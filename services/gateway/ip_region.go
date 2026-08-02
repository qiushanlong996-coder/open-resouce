package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

// 评论 IP 归属地。
//
// 设计取舍（按重要性排序）：
//
//  1. 只落库「归属地」，绝不落库 IP。
//     原始 IP 是个人信息，存下来就是长期负担：库一旦泄漏，每条评论都能关联到
//     具体网络位置。而「IP 属地」本质是「发表这条评论时人在哪个省」——一个
//     时点事实，算完即可丢弃 IP。这样做同时让读路径完全不依赖 ip2region。
//
//  2. 只到省级（境外只到国家），不显示城市与运营商。
//     省级已足够达到「标注发言来源」的目的，城市级则显著提升可识别度。
//
//  3. xdb 数据文件不入库，由 IP_REGION_V4_DB_PATH / IP_REGION_V6_DB_PATH 指定。
//     11MB + 37MB 的二进制不该进 git。未配置时整个功能静默降级（归属地为空，
//     前端不显示），与 Elasticsearch 的降级策略一致。
//
//  4. 用 vector-index 模式而非全量载入内存。
//     归属地只在评论写入时算一次，查询频率极低，几次页缓存内的文件读毫无影响；
//     换来的是内存占用从约 48MB 降到约 1MB。
//
//  5. 必须加锁。xdb.Searcher 源码自己标注了 "Not thread safe"，且 Search 每次
//     调用都会写 s.ioCount，文件模式还共享 Seek 位置。评论创建是并发的，
//     不加锁会触发数据竞争。锁的代价在这个调用频率下可以忽略。

type ipRegionResolver interface {
	// Resolve 返回省级（境外为国家级）归属地。
	// 无法判定、内网地址、未配置数据文件时统一返回空字符串，
	// 由调用方决定「空」怎么展示，而不是在这里编造「未知」。
	Resolve(ip string) string
}

// noopIPRegionResolver 在未配置 xdb 时使用。
type noopIPRegionResolver struct{}

func (noopIPRegionResolver) Resolve(string) string { return "" }

var ipRegionResolverStore ipRegionResolver = noopIPRegionResolver{}

type ip2regionResolver struct {
	// searcher 非线程安全，所有访问都必须持 mutex。
	mutex sync.Mutex
	v4    *xdb.Searcher
	v6    *xdb.Searcher
}

// newIP2RegionResolver 按需加载 IPv4 / IPv6 数据文件。
// 两个路径都可以为空：只配 v4 时，IPv6 来源的评论归属地为空。
func newIP2RegionResolver(v4Path, v6Path string) (*ip2regionResolver, error) {
	resolver := &ip2regionResolver{}
	if v4Path != "" {
		searcher, err := openXDBSearcher(xdb.IPv4, v4Path)
		if err != nil {
			return nil, fmt.Errorf("load ipv4 region database: %w", err)
		}
		resolver.v4 = searcher
	}
	if v6Path != "" {
		searcher, err := openXDBSearcher(xdb.IPv6, v6Path)
		if err != nil {
			return nil, fmt.Errorf("load ipv6 region database: %w", err)
		}
		resolver.v6 = searcher
	}
	if resolver.v4 == nil && resolver.v6 == nil {
		return nil, errors.New("no ip region database configured")
	}
	return resolver, nil
}

// openXDBSearcher 以 vector-index 模式打开 xdb。
func openXDBSearcher(version *xdb.Version, path string) (*xdb.Searcher, error) {
	// 先校验文件结构，避免把损坏或截断的文件当成可用数据源。
	if err := xdb.VerifyFromFile(path); err != nil {
		return nil, fmt.Errorf("verify %s: %w", path, err)
	}
	vectorIndex, err := xdb.LoadVectorIndexFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load vector index from %s: %w", path, err)
	}
	searcher, err := xdb.NewWithVectorIndex(version, path, vectorIndex)
	if err != nil {
		return nil, fmt.Errorf("open searcher for %s: %w", path, err)
	}
	return searcher, nil
}

func (resolver *ip2regionResolver) Resolve(ip string) string {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil || !isPublicIP(parsed) {
		return ""
	}

	// 按地址族选库：v4 地址查 v4 库，v6 地址查 v6 库。
	// 注意 parsed.To4() 对 IPv4-mapped IPv6（::ffff:1.2.3.4）也返回非 nil，
	// 这正是我们想要的——这类地址应当走 v4 库。
	searcher := resolver.v6
	lookup := parsed.String()
	if v4 := parsed.To4(); v4 != nil {
		searcher = resolver.v4
		lookup = v4.String()
	}
	if searcher == nil {
		return ""
	}

	resolver.mutex.Lock()
	raw, err := searcher.Search(lookup)
	resolver.mutex.Unlock()
	if err != nil {
		// 查不到不是错误路径的重点：归属地缺失只影响一个展示字段。
		slog.Debug("resolve ip region failed", "error", err)
		return ""
	}
	return normalizeIPRegion(raw)
}

// isPublicIP 判断是否为需要（也值得）查询归属地的公网地址。
// 内网、回环、链路本地地址查出来只会是「内网IP」这类无意义结果，
// 本地开发时几乎全是这种情况。
func isPublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return false
	}
	// 100.64.0.0/10（运营商级 NAT）不在 IsPrivate 覆盖范围内，但同样查不出归属地。
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return false
	}
	return true
}

// chinaProvinceNames 是省级行政区的展示名，按「以它为前缀」匹配 ip2region 的省份字段。
//
// 不用「去掉省/市/自治区后缀」的写法：那样会把「新疆维吾尔自治区」变成
// 「新疆维吾尔」、「广西壮族自治区」变成「广西壮族」。这些名字里的民族名
// 属于全称的一部分，简称需要显式列出。
// 这些短名之间不存在前缀重叠（河北/河南、湖北/湖南、山西/山东、广东/广西
// 都能互相区分），因此顺序无关。
var chinaProvinceNames = []string{
	"北京", "天津", "上海", "重庆",
	"河北", "山西", "辽宁", "吉林", "黑龙江", "江苏", "浙江", "安徽",
	"福建", "江西", "山东", "河南", "湖北", "湖南", "广东", "海南",
	"四川", "贵州", "云南", "陕西", "甘肃", "青海",
	"内蒙古", "广西", "西藏", "宁夏", "新疆",
	"香港", "澳门", "台湾",
}

// ipRegionPlaceholders 是 ip2region 表示「无数据」的取值。
var ipRegionPlaceholders = map[string]struct{}{
	"":          {},
	"0":         {},
	"内网IP":      {},
	"局域网":       {},
	"未分配或者内网IP": {},
}

// normalizeIPRegion 把 ip2region 的原始记录压成一个展示用的归属地。
//
// v4 数据的字段布局是：国家|省份|城市|ISP|国家代码
//
//	中国|江苏省|南京市|0|CN
//	United States|California|0|Google LLC|US
//
// 注意这与旧版（v2）的「国家|区域|省份|城市|ISP」不同：省份在第 2 段而不是第 3 段。
// 把两者搞反会直接把城市名发到公开评论区，违反「只到省级」的约束。
//
// 境外只取国家名。v4 数据里境外国家名是英文（United States），这里原样展示：
// 维护一张两百多条的中英对照表成本高，而部分翻译、部分英文比统一英文更糟。
func normalizeIPRegion(raw string) string {
	parts := strings.Split(raw, "|")
	// 至少要有「国家|省份」两段才能判定。
	if len(parts) < 2 {
		return ""
	}
	country := strings.TrimSpace(parts[0])
	province := strings.TrimSpace(parts[1])
	countryCode := ""
	if len(parts) >= 5 {
		countryCode = strings.TrimSpace(parts[4])
	}

	// 境内判定优先用国家代码：比匹配中文串稳定。
	isChina := countryCode == "CN" || country == "中国"
	if !isChina {
		if _, placeholder := ipRegionPlaceholders[country]; placeholder {
			return ""
		}
		return country
	}

	for _, name := range chinaProvinceNames {
		if strings.HasPrefix(province, name) {
			return name
		}
	}
	// 能确定在境内但省份对不上（数据异常或新增行政区）：退到国家级。
	// 绝不直接把原始字段吐出去——那可能是城市名或其他意外内容。
	return "中国"
}

// resolveIPRegion 把请求 IP 转成归属地，供评论公开展示与登录审计共用。
func resolveIPRegion(ip string) string {
	return ipRegionResolverStore.Resolve(ip)
}
