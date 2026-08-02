package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// IP 归属地测试。
//
// 重点覆盖三件事：
//  1. normalizeIPRegion 的归一化规则，尤其是民族自治区简称——
//     「去掉自治区后缀」的朴素写法会得到「新疆维吾尔」这种错误结果。
//  2. 内网/回环地址不产生归属地（本地开发与容器内网的常态）。
//  3. 只落库归属地、不落库 IP——这是隐私上的关键约束，必须由测试守住。

// 以下用例的原始串均取自真实 ip2region_v4.xdb 的输出，
// 字段布局为：国家|省份|城市|ISP|国家代码。
func TestNormalizeIPRegionChinaProvinces(t *testing.T) {
	for name, testCase := range map[string]struct {
		raw  string
		want string
	}{
		"直辖市无后缀":    {"中国|北京|北京市|0|CN", "北京"},
		"上海":        {"中国|上海|上海市|联通|CN", "上海"},
		"普通省份":      {"中国|江苏省|南京市|0|CN", "江苏"},
		"三字省份":      {"中国|黑龙江省|哈尔滨市|移动|CN", "黑龙江"},
		"宁夏已是简称":    {"中国|宁夏|银川市|电信|CN", "宁夏"},
		"新疆已是简称":    {"中国|新疆|克拉玛依市|电信|CN", "新疆"},
		"内蒙古自治区全称":  {"中国|内蒙古自治区|呼和浩特市|电信|CN", "内蒙古"},
		"广西壮族自治区全称": {"中国|广西壮族自治区|南宁市|联通|CN", "广西"},
		"西藏自治区":     {"中国|西藏自治区|拉萨市|电信|CN", "西藏"},
		"澳门特别行政区":   {"中国|澳门特别行政区|0|澳门电讯|CN", "澳门"},
		"香港特别行政区":   {"中国|香港特别行政区|0|0|CN", "香港"},
		"台湾省":       {"中国|台湾省|0|中华电信|CN", "台湾"},
	} {
		if got := normalizeIPRegion(testCase.raw); got != testCase.want {
			t.Errorf("%s: normalizeIPRegion(%q) = %q, want %q", name, testCase.raw, got, testCase.want)
		}
	}
}

// TestNormalizeIPRegionNeverLeaksCity 是隐私约束的直接守卫。
//
// 历史教训：v4 数据的字段布局是「国家|省份|城市|ISP|代码」，旧版是
// 「国家|区域|省份|城市|ISP」。按旧版取第 3 段会拿到城市名，
// 把「南京市」发到公开评论区。这条测试存在的意义就是拦住这个退化。
func TestNormalizeIPRegionNeverLeaksCity(t *testing.T) {
	known := make(map[string]struct{}, len(chinaProvinceNames))
	for _, name := range chinaProvinceNames {
		known[name] = struct{}{}
	}

	// 这些都是真实数据行，省份段有效、城市段包含具体市名。
	for _, raw := range []string{
		"中国|江苏省|南京市|0|CN",
		"中国|浙江省|杭州市|阿里|CN",
		"中国|广东省|广州市|电信|CN",
		"中国|陕西省|西安市|中国科技网|CN",
		"中国|四川省|甘孜|电信|CN",
		"中国|吉林省|延边|电信|CN",
		"中国|青海省|海西|电信|CN",
	} {
		got := normalizeIPRegion(raw)
		// 关键断言：必须解出具体省份，不能是「中国」。
		//
		// 为何不能只断言「在省级白名单内」：若字段取错（拿到城市名），
		// 城市名匹配不到省份表，会落到「中国」这个更安全的兜底值——
		// 而「中国」也在白名单里，于是测试会静静放过。
		// （这正是反向验证时发现的：第一版断言完全没拦住该 bug。）
		if got == "中国" {
			t.Errorf("normalizeIPRegion(%q) = 中国，省份段明确有值却没解出来（字段位置取错？）", raw)
			continue
		}
		if _, ok := known[got]; !ok {
			t.Errorf("normalizeIPRegion(%q) = %q，不在省级白名单内（泄露了更细的位置）", raw, got)
		}
		// 双重保险：结果不能包含原始串里的城市段。
		city := strings.Split(raw, "|")[2]
		if city != "0" && got == city {
			t.Errorf("normalizeIPRegion(%q) 直接返回了城市名 %q", raw, city)
		}
	}
}

// TestNormalizeIPRegionUnknownProvinceFallsBackToCountry 省份对不上时退到国家级，
// 而不是把未知内容原样吐出。
func TestNormalizeIPRegionUnknownProvinceFallsBackToCountry(t *testing.T) {
	for _, raw := range []string{
		"中国|某个新行政区|某市|电信|CN",
		"中国|0|0|0|CN",
	} {
		if got := normalizeIPRegion(raw); got != "中国" {
			t.Errorf("normalizeIPRegion(%q) = %q, want 中国", raw, got)
		}
	}
}

func TestNormalizeIPRegionOverseasUsesCountry(t *testing.T) {
	// 境外只到国家级：v4 数据里国家名为英文，省/州与城市不得泄露。
	for raw, want := range map[string]string{
		"United States|California|0|Google LLC|US": "United States",
		"Australia|Queensland|Brisbane|0|AU":       "Australia",
		"Japan|Tokyo|Tokyo|0|JP":                   "Japan",
		"Singapore|Singapore|Singapore|0|SG":       "Singapore",
	} {
		got := normalizeIPRegion(raw)
		if got != want {
			t.Errorf("normalizeIPRegion(%q) = %q, want %q", raw, got, want)
		}
		// 不能退化成州/省或城市。
		if parts := strings.Split(raw, "|"); got == parts[1] && parts[1] != parts[0] {
			t.Errorf("normalizeIPRegion(%q) 返回了州/省 %q，应为国家名", raw, got)
		}
	}
}

func TestNormalizeIPRegionPlaceholdersYieldEmpty(t *testing.T) {
	// ip2region 用 0 / 内网IP 表示无数据，这些都不该展示出去。
	// 注意这些行的国家代码不是 CN，因此走境外分支的占位符判定。
	for _, raw := range []string{
		"0|0|0|内网IP|0",
		"0|0|0|0|0",
		"内网IP|内网IP|内网IP|内网IP|0",
		"局域网|局域网|0|0|0",
		"",
		"格式不对",
	} {
		if got := normalizeIPRegion(raw); got != "" {
			t.Errorf("normalizeIPRegion(%q) = %q, want 空字符串", raw, got)
		}
	}
}

// TestNormalizeIPRegionKnownChinaWithoutProvince 能确定在境内但省份缺失时，
// 展示国家名比什么都不显示更有信息量。
func TestNormalizeIPRegionKnownChinaWithoutProvince(t *testing.T) {
	if got := normalizeIPRegion("中国|0|0|0|CN"); got != "中国" {
		t.Fatalf("normalizeIPRegion = %q, want 中国", got)
	}
}

func TestIsPublicIPRejectsNonRoutable(t *testing.T) {
	// 这些地址查归属地只会得到无意义结果，本地开发时几乎全是这种。
	for _, ip := range []string{
		"127.0.0.1", "::1", // 回环
		"10.0.0.5", "172.16.3.4", "192.168.1.100", // 私有
		"169.254.1.1", // 链路本地
		"0.0.0.0",
		"100.64.0.1", "100.127.255.254", // 运营商级 NAT（IsPrivate 不覆盖）
		"fd00::1", // IPv6 唯一本地地址
	} {
		if got := parseAndCheckPublic(ip); got {
			t.Errorf("isPublicIP(%s) = true, want false", ip)
		}
	}
	// 公网地址必须放行，否则功能等于没开。
	for _, ip := range []string{
		"1.2.4.8", "114.114.114.114", "103.236.98.166",
		"2400:3200::1",
		"100.63.255.255", "100.128.0.1", // 紧邻 100.64/10 边界的公网地址
	} {
		if got := parseAndCheckPublic(ip); !got {
			t.Errorf("isPublicIP(%s) = false, want true", ip)
		}
	}
}

func parseAndCheckPublic(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	return isPublicIP(parsed)
}

func TestNoopResolverReturnsEmpty(t *testing.T) {
	// 未配置数据文件时必须静默返回空，而不是报错或返回「未知」。
	if got := (noopIPRegionResolver{}).Resolve("114.114.114.114"); got != "" {
		t.Fatalf("noop resolver = %q, want 空字符串", got)
	}
}

// stubIPRegionResolver 让 HTTP 层测试不依赖真实 xdb 数据文件。
type stubIPRegionResolver struct {
	region string
	mutex  sync.Mutex
	seen   []string
}

func (stub *stubIPRegionResolver) Resolve(ip string) string {
	stub.mutex.Lock()
	stub.seen = append(stub.seen, ip)
	stub.mutex.Unlock()
	return stub.region
}

func (stub *stubIPRegionResolver) observed() []string {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()
	return append([]string(nil), stub.seen...)
}

// TestCommentStoresRegionNotIP 是隐私约束的守卫测试：
// 评论落库与响应里都必须只有归属地，绝不能出现原始 IP。
func TestCommentStoresRegionNotIP(t *testing.T) {
	originalAuth, originalComments := authRepositoryStore, commentRepositoryStore
	originalResolver, originalLimiter := ipRegionResolverStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	commentRepositoryStore = newMemoryCommentRepository()
	authRateLimiter = newFixedWindowLimiter()
	stub := &stubIPRegionResolver{region: "广东"}
	ipRegionResolverStore = stub
	t.Cleanup(func() {
		// 先排空在途的 best-effort 任务（评论会触发异步加经验），
		// 否则恢复包级依赖时会与那些 goroutine 的读取发生数据竞争。
		waitForBackgroundTasks()
		authRepositoryStore, commentRepositoryStore = originalAuth, originalComments
		ipRegionResolverStore, authRateLimiter = originalResolver, originalLimiter
	})

	cookie, _ := registerTestUser(t, "region-user@example.com", "归属地用户")
	const clientIP = "114.114.114.114"

	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"","quote":"","body":"验证归属地"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Forwarded-For", clientIP)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create comment status = %d, want 201: %s", response.Code, response.Body)
	}

	body := response.Body.String()
	if !strings.Contains(body, `"author_region":"广东"`) {
		t.Fatalf("响应缺少归属地: %s", body)
	}
	// 关键断言：响应里不能出现原始 IP。
	if strings.Contains(body, clientIP) {
		t.Fatalf("响应泄漏了原始 IP：%s", body)
	}

	// 解析器确实收到了 X-Forwarded-For 里的 IP（说明取 IP 的链路是通的）。
	// 注意：registerTestUser 的注册自动登录也会触发一次解析，
	// 因此要看的是最后一次（本次评论）。
	seen := stub.observed()
	if len(seen) == 0 || seen[len(seen)-1] != clientIP {
		t.Fatalf("resolver 收到的最后一个 IP = %#v, want 以 %s 结尾", seen, clientIP)
	}

	// 列表接口同样只返回归属地。
	listRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments", nil)
	listResponse := httptest.NewRecorder()
	newHandler().ServeHTTP(listResponse, listRequest)
	listBody := listResponse.Body.String()
	if !strings.Contains(listBody, `"author_region":"广东"`) {
		t.Fatalf("列表缺少归属地: %s", listBody)
	}
	if strings.Contains(listBody, clientIP) {
		t.Fatalf("列表泄漏了原始 IP：%s", listBody)
	}
}

// TestCommentOmitsRegionWhenUnresolvable 内网来源不该显示归属地。
// omitempty 让空归属地在 JSON 里整个字段消失，前端天然不渲染。
func TestCommentOmitsRegionWhenUnresolvable(t *testing.T) {
	originalAuth, originalComments := authRepositoryStore, commentRepositoryStore
	originalResolver, originalLimiter := ipRegionResolverStore, authRateLimiter
	authRepositoryStore = newMemoryAuthRepository()
	commentRepositoryStore = newMemoryCommentRepository()
	authRateLimiter = newFixedWindowLimiter()
	ipRegionResolverStore = &stubIPRegionResolver{region: ""}
	t.Cleanup(func() {
		waitForBackgroundTasks()
		authRepositoryStore, commentRepositoryStore = originalAuth, originalComments
		ipRegionResolverStore, authRateLimiter = originalResolver, originalLimiter
	})

	cookie, _ := registerTestUser(t, "intranet-user@example.com", "内网用户")
	request := httptest.NewRequest(http.MethodPost,
		"/api/v1/projects/atlas-agent/documents/quick-start/comments",
		strings.NewReader(`{"block_id":"","quote":"","body":"内网评论"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newHandler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", response.Code, response.Body)
	}
	if strings.Contains(response.Body.String(), "author_region") {
		t.Fatalf("归属地为空时字段应整个省略：%s", response.Body)
	}
}

// TestIP2RegionResolverRealDatabase 用真实 xdb 验证解析结果与并发安全。
//
// 数据文件不入仓（IPv4 就有 11MB），因此未提供时跳过。
// 本地跑：IP_REGION_V4_DB_PATH=... go test -race -run RealDatabase ./services/gateway/
func TestIP2RegionResolverRealDatabase(t *testing.T) {
	dbPath := os.Getenv("IP_REGION_V4_DB_PATH")
	if dbPath == "" {
		t.Skip("IP_REGION_V4_DB_PATH 未配置，跳过真实数据库用例")
	}
	resolver, err := newIP2RegionResolver(dbPath, "")
	if err != nil {
		t.Fatalf("加载 IP 库失败：%v", err)
	}

	// 已知归属的公网地址：必须解出具体省份。
	// 不能只断言「在白名单内」：字段取错时会落到「中国」兜底值，
	// 那样测试会静静放过。也不写死具体省名：IP 段归属会随数据库更新变动。
	known := make(map[string]struct{}, len(chinaProvinceNames))
	for _, name := range chinaProvinceNames {
		known[name] = struct{}{}
	}
	for _, ip := range []string{"114.114.114.114", "223.5.5.5", "1.2.4.8", "202.96.128.86"} {
		region := resolver.Resolve(ip)
		if region == "" {
			t.Errorf("Resolve(%s) 为空，已知公网地址应当能解出归属", ip)
			continue
		}
		if region == "中国" {
			t.Errorf("Resolve(%s) = 中国，应解出具体省份（字段位置取错？）", ip)
			continue
		}
		if _, ok := known[region]; !ok {
			t.Errorf("Resolve(%s) = %q，不在省级白名单内（可能泄露了城市级）", ip, region)
		}
		t.Logf("Resolve(%s) = %s", ip, region)
	}

	// 内网地址即使有真实数据库也不应产生归属地。
	if region := resolver.Resolve("127.0.0.1"); region != "" {
		t.Errorf("Resolve(127.0.0.1) = %q, want 空", region)
	}

	// 并发安全：xdb.Searcher 自己标注了 Not thread safe，且 Search 会写 ioCount。
	// 这段在 -race 下能直接暴露“忘了加锁”这类退化。
	const workers = 16
	const rounds = 40
	var group sync.WaitGroup
	results := make([]string, workers)
	for worker := range workers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			var last string
			for range rounds {
				last = resolver.Resolve("114.114.114.114")
			}
			results[index] = last
		}(worker)
	}
	group.Wait()
	// 同一个 IP 并发查询必须得到一致结果；不一致就意味着读到了被其他
	// goroutine 改动的 seek 位置或缓冲区。
	for index, got := range results {
		if got != results[0] {
			t.Fatalf("并发结果不一致：worker %d = %q, worker 0 = %q", index, got, results[0])
		}
	}
	if results[0] == "" {
		t.Fatal("并发查询全部返回空，不符合预期")
	}
}

// TestRecordLoginStoresRegion 登录后管理后台能看到归属地与时间。
func TestRecordLoginStoresRegion(t *testing.T) {
	originalAuth, originalResolver := authRepositoryStore, ipRegionResolverStore
	originalLimiter := authRateLimiter
	repository := newMemoryAuthRepository()
	authRepositoryStore = repository
	authRateLimiter = newFixedWindowLimiter()
	ipRegionResolverStore = &stubIPRegionResolver{region: "浙江"}
	t.Cleanup(func() {
		waitForBackgroundTasks()
		authRepositoryStore, ipRegionResolverStore = originalAuth, originalResolver
		authRateLimiter = originalLimiter
	})

	before := time.Now().UTC().Add(-time.Second)
	// registerTestUser 走的也是 createLoginSession，因此注册即算一次登录。
	_, user := registerTestUser(t, "login-region@example.com", "登录用户")

	summaries, total, err := repository.ListUsers(t.Context(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(summaries) != 1 {
		t.Fatalf("total=%d len=%d, want 1/1", total, len(summaries))
	}
	summary := summaries[0]
	if summary.ID != user.ID {
		t.Fatalf("summary.ID = %q, want %q", summary.ID, user.ID)
	}
	if summary.LastLoginRegion != "浙江" {
		t.Fatalf("LastLoginRegion = %q, want 浙江", summary.LastLoginRegion)
	}
	if summary.LastLoginAt == nil {
		t.Fatal("LastLoginAt 不应为空")
	}
	if summary.LastLoginAt.Before(before) {
		t.Fatalf("LastLoginAt = %v, 早于本次测试开始时间 %v", summary.LastLoginAt, before)
	}
	// 管理后台同样不该拿到原始 IP：结构体里根本没有这个字段，
	// 这里断言的是「不存在存 IP 的地方」这一设计。
}

// TestListUsersLeavesLastLoginEmptyWithoutLogin 从未登录过的用户两个字段都为空。
func TestListUsersLeavesLastLoginEmptyWithoutLogin(t *testing.T) {
	repository := newMemoryAuthRepository()
	if err := repository.CreateUser(t.Context(), authUser{
		ID: "user-never", Email: "never@example.com", DisplayName: "从未登录",
	}); err != nil {
		t.Fatal(err)
	}
	summaries, _, err := repository.ListUsers(t.Context(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len = %d, want 1", len(summaries))
	}
	if summaries[0].LastLoginRegion != "" || summaries[0].LastLoginAt != nil {
		t.Fatalf("从未登录过的用户应留空，got region=%q at=%v",
			summaries[0].LastLoginRegion, summaries[0].LastLoginAt)
	}
}

// TestRecordLoginOverwritesPreviousEntry 「上一次登录」语义要求覆盖而非追加。
func TestRecordLoginOverwritesPreviousEntry(t *testing.T) {
	repository := newMemoryAuthRepository()
	if err := repository.CreateUser(t.Context(), authUser{
		ID: "user-1", Email: "u1@example.com", DisplayName: "用户一",
	}); err != nil {
		t.Fatal(err)
	}
	first := time.Now().UTC().Add(-time.Hour)
	second := time.Now().UTC()
	if err := repository.RecordLogin(t.Context(), "user-1", "北京", first); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordLogin(t.Context(), "user-1", "上海", second); err != nil {
		t.Fatal(err)
	}
	summaries, _, err := repository.ListUsers(t.Context(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if summaries[0].LastLoginRegion != "上海" {
		t.Fatalf("LastLoginRegion = %q, want 上海（应覆盖为最近一次）", summaries[0].LastLoginRegion)
	}
	if !summaries[0].LastLoginAt.Equal(second) {
		t.Fatalf("LastLoginAt = %v, want %v", summaries[0].LastLoginAt, second)
	}
}
