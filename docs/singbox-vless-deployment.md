# sing-box VLESS over TLS 部署文档（103.149.91.140）

厂商确认该服务器**不开放 UDP 端口**，WireGuard 无法使用（排查过程见 [wireguard-deployment.md](./wireguard-deployment.md)）。本文档记录改用 TCP 承载的实际可用方案。

> **公网 IP 已于 2026-08-07 由云厂商变更：`156.225.28.10` → `103.149.91.140`**（旧 IP 已完全失效）。定位过程与客户端更新见第 7 节。本文中的 IP 均已更新为新值。

## 1. 为什么选这个方案

需求是「全局出网、日常上网用」，客户端为 iOS 手机和 macOS/Windows 电脑，且只能走 TCP。

| 方案 | 结论 |
| --- | --- |
| **sing-box VLESS + TLS + Vision** | **采用**。单个静态 Go 二进制，不依赖 CentOS 7 的老 glibc；复用已部署的 openresource.cn 正式证书，链路上就是普通 HTTPS；iOS 有官方 App 和 Shadowrocket 可用 |
| OpenVPN over TCP | 否。TCP-over-TCP 在跨境丢包时劣化明显，且握手特征易被识别限速 |
| WireGuard + udp2raw | 否。性能最好，但 udp2raw 没有 iOS 客户端，手机用不了 |
| SoftEther SSL-VPN | 否。手机端要么依赖 L2TP（UDP，不可用），要么退回 OpenVPN 兼容模式，不如直接用前者 |

前置事实：实测该机**任意 TCP 端口入向都放通**（验证了 1194 / 8444 / 9443 / 2087 / 40443，服务端均收到连接，源 `106.11.31.1`）。因此不必和网站抢 443，给 VPN 独立端口即可，现有 nginx 与站点完全不用改动。

## 2. 部署结果

| 项 | 值 |
| --- | --- |
| 程序 | sing-box 1.13.16，静态二进制 `/usr/local/bin/sing-box` |
| 协议 | VLESS + TLS |
| 端口 | **TCP 8444**（`listen: "::"`，与公网 IP 无绑定关系） |
| 证书 | `/etc/open-resouce/tls/openresource.cn.pem`（Let's Encrypt，与网站共用，只读引用） |
| SNI | `openresource.cn` |
| 服务 | `sing-box.service`，已 `enable`，`Restart=on-failure` |
| 配置 | `/etc/sing-box/config.json`（600） |
| 客户端配置 | `/root/sing-box-clients/`（`link.txt`、`desktop-tun.json`、`README.md`） |

服务端配了**两个用户**，共用同一个 inbound：

| name | UUID 存放位置 | flow | 用途 |
| --- | --- | --- | --- |
| `main` | `/etc/sing-box/uuid` | `xtls-rprx-vision` | 主链路，性能更好，优先用 |
| `compat` | `/etc/sing-box/uuid-compat` | 无 | 给不支持 Vision 的客户端兜底 |

加 `compat` 的原因：部分 iOS 客户端不支持 `xtls-rprx-vision`，此时导入主链接会直接握手失败，且失败现象（连上但不通）不容易和网络问题区分。两个 UUID 都持久化在独立文件里，重跑 `singbox_deploy.sh` 不会重新生成、不会让已分发的链接失效。

nginx 的 80 / 443 / 8443 一律未改动，部署后复测：443 → 301（原有跳转）、8443 → 200、80 → 200，站点不受影响。

## 3. 一个必须修的坑：服务器 DNS 被污染

首次验证时 `github.com` 能通，但 `google.com` 失败。原因是服务器 `/etc/resolv.conf` 首选 `114.114.114.114`（国内 DNS）：

```
114.114.114.114 -> www.google.com = 157.240.7.20    # 这是 Facebook 的 IP，污染结果
8.8.8.8         -> www.google.com = 142.251.150.119 # 正确
1.1.1.1         -> www.google.com = 142.251.153.119 # 正确
```

sing-box 日志里对应的报错是 `dial tcp 69.171.235.22:443: i/o timeout`——同样是 Facebook 段的地址。如果不管，日常上网会大面积踩污染。

修法是让 sing-box 用自己的 DNS，不跟随系统 resolver。**没有改 `/etc/resolv.conf`**，避免影响 nginx、gateway、yum 等既有服务：

```json
"dns": {
  "servers": [
    { "type": "https", "tag": "cf-doh",     "server": "1.1.1.1" },
    { "type": "udp",   "tag": "google-udp", "server": "8.8.8.8" }
  ],
  "strategy": "ipv4_only"
}
```

两点说明：

1. 主用 DoH（TCP 443），实测服务器可达，既绕开 UDP 限制也避免解析被篡改。
2. `strategy` 必须是 `ipv4_only`。这台机器 eth0 上没有 IPv6 地址，若解析到 AAAA 会直接连不上。

## 4. 验证结果

用真实 sing-box 客户端（macOS arm64）连公网 IP 做端到端验证，不是只看端口监听。下表为 2026-08-07 IP 变更后的复测结果，`main` 与 `compat` 两个用户各起一个 socks 客户端同时测：

| 测试项 | `main`（Vision） | `compat`（无 flow） |
| --- | --- | --- |
| 出口 IP（直连基线 `106.11.31.5`） | **103.149.91.140** | **103.149.91.140** |
| `www.google.com` | 200 | 200 |
| `github.com` | 200 | 200 |
| `www.youtube.com` | 200 | 200 |
| 客户端日志 error/fatal | 无 | 无 |

| 其它 | 结果 |
| --- | --- |
| 10MB 下载吞吐 | 约 2.1 MB/s（IP 变更前为 906 KB/s） |
| uTLS `chrome` 指纹 | 可用，已写入分享链接 |
| 网站 80 / 443 / 8443（外部访问新 IP） | 200 / 200 / 200；带正确 SNI 访问 `https://openresource.cn/` 为 301（原有跳转） |
| 证书（经新 IP，SNI `openresource.cn`） | Let's Encrypt，`notAfter=Nov 3 15:46:21 2026 GMT`，链正常 |

## 5. 客户端接入

分享链接在服务器 `/root/sing-box-clients/link.txt`（两条，UUID 见服务器文件，不进仓库）：

```
# 主链路，客户端需支持 xtls-rprx-vision
vless://<UUID-main>@103.149.91.140:8444?encryption=none&flow=xtls-rprx-vision&security=tls&sni=openresource.cn&type=tcp&fp=chrome#openresource-hk

# 兼容链路，无 flow，给不支持 Vision 的客户端
vless://<UUID-compat>@103.149.91.140:8444?encryption=none&security=tls&sni=openresource.cn&type=tcp&fp=chrome#openresource-hk-compat
```

先用主链路；如果客户端能连上但网页打不开、或日志里出现握手相关报错，换兼容链路。**两条链接不要同时在一个客户端里启用**，否则分不清是哪条在工作。

**server 填 IP、SNI 单独填域名**是刻意的：既不依赖 DNS 解析，也绕开域名被接入商备案拦截的问题（见 git 历史中关于备案拦截的记录）。代价是公网 IP 变更时必须重发链接——2026-08-07 就发生了一次，见第 7 节。

| 平台 | 做法 |
| --- | --- |
| iOS | sing-box 官方 App 或 Shadowrocket，粘贴 vless:// 链接导入。两者都支持 Vision 和 chrome uTLS 指纹；若手上的客户端不支持 Vision，用兼容链路 |
| macOS / Windows | GUI 客户端（sing-box GUI、Clash Verge、v2rayN）粘贴链接；或用官方 CLI 跑 `desktop-tun.json`：`sudo sing-box run -c desktop-tun.json` |

`desktop-tun.json` 的策略：TUN 全局模式，内网地址直连，其余全部走代理，DNS 走隧道内的 DoH，避免本地污染。

UUID 属于凭据，只存在服务器上，不进仓库。

## 6. 运维

```bash
systemctl status sing-box                          # 状态
journalctl -u sing-box -n 50 --no-pager            # 日志
sing-box check -c /etc/sing-box/config.json        # 改配置后先检查再重启
systemctl restart sing-box
bash /root/singbox_deploy.sh                       # 可重复执行，UUID 不会被覆盖
```

新增用户：在 `config.json` 的 `inbounds[0].users` 里加一项（各自独立 UUID），`check` 通过后 `restart`。加 `compat` 用户用的 `/root/add_compat.sh` 是这个流程的样板——先写临时配置、`sing-box check` 通过才覆盖，失败则从 `config.json.bak` 回滚，避免把服务改崩。

### 需要留意

1. **证书 2026-11-03 到期**。sing-box 在启动时读取证书文件，续期后必须 `systemctl restart sing-box`，否则会继续用旧证书。建议把这一步加进证书续期流程，和 nginx 的 reload 放在一起。
2. 客户端的 `flow` 必须是 `xtls-rprx-vision`，与服务端一致；留空会握手失败。
3. CentOS 7 已 EOL，sing-box 是静态二进制不受影响，但升级需手动替换 `/usr/local/bin/sing-box` 后重启服务。
4. WireGuard 已于 2026-08-06 从服务器上完全清理（服务、接口、内核模块、密钥、源码、构建依赖，并把 `ip_forward` 恢复为 0），清理明细见 [wireguard-deployment.md](./wireguard-deployment.md) 第 9 节。现在这台机器上的出网方案只有 sing-box 一套，不存在两套并存的情况。
5. 从这台开发 Mac 直接访问 `https://www.openresource.cn/` 会拿到 **403 加纯中文提示**，那是**本机云壳的域名拦截 sinkhole**（域名被解析到 `59.82.113.122`，0.26s 就返回，根本没回源），与源站状态无关。要放行得去「云壳-防护记录-域名拦截」加白。VPN 不受影响，因为客户端链接是 IP + 单独填 SNI。排查网站问题时别把这个 403 当成源站故障。

## 7. 公网 IP 变更事件（2026-08-07）

云厂商把这台机器的公网 IP 从 `156.225.28.10` 换成了 `103.149.91.140`，没有事先通知。现象是网站和 sing-box 同时全断。

### 7.1 定位过程

一上手要回答的问题是「服务挂了，还是机器没了」。判据：

| 探测 | 结果 |
| --- | --- |
| 旧 IP 的 22 / 80 / 443 / 8443 / 8444 / 18080 / 3389 / 8080 | **全部超时**，无一例外 |
| 旧 IP 的 ICMP | 5 轮全丢 |
| 8 个海外节点（6 国）→ 旧 IP TCP 443 | 全部 Connection timed out |
| 6 个海外节点 → 旧 IP ICMP | 0/4 |
| 6 个海外节点 → **同网段网关 `156.225.28.1`** ICMP | **4/4 全通** |
| 正对照：海外节点 → `8.8.8.8:443` | 成功（证明探测方法本身有效） |

三步排除：

1. **不是服务故障**。SSH 22 和 ICMP 与 nginx / sing-box 无关，它们一起消失，说明问题在这两个服务之下的层级。
2. **不是本机或云壳**。8 个分布在 6 个国家的独立节点看到完全一致的黑洞。
3. **不是机房或线路**。同一 `/24` 的网关从全球都稳定应答，说明包能进机房、路由正常，只有旧 IP 这一个地址是黑洞。

结论锁定在「实例的公网地址层」，随后厂商侧确认为 IP 变更。

### 7.2 两条值得留存的方法

1. **拿同网段的网关做对照**。目标 IP 不通时，ping 同 `/24` 里的网关（通常是 `.1`）：网关通而目标不通，就能把「线路/路由问题」和「这个地址本身的问题」分开，不需要任何服务端权限。
2. **用多地节点排除本地因素**。本机有云壳这类安全软件时，「连不上」的解释空间很大。`check-host.net` 的 API 可以直接用 `curl` 调（`check-tcp` / `check-ping` 拿 `request_id`，再取 `check-result/<id>`），几秒钟拿到多国视角，比在本机反复试有效得多。**记得同时打一个正对照**（比如 `8.8.8.8:443`），否则无法区分「目标真的不通」和「探测服务本身坏了」。

### 7.3 IP 变更后的处理清单

服务端**不需要改任何配置**：inbound 是 `listen: "::"`，nginx 也没绑具体地址，重启后两个服务都随 systemd 自启（本次实测 `active/enabled`，`systemctl --failed` 为空）。要改的只有客户端和外部引用：

| 项 | 处理 |
| --- | --- |
| `/root/sing-box-clients/link.txt` | 按新 IP 重生成两条链接（旧文件备份为 `link.txt.oldip.bak`） |
| `/root/sing-box-clients/desktop-tun.json` | `outbounds[].server` 换新 IP，改完跑 `sing-box check` 确认仍是合法配置 |
| `/root/sing-box-clients/README.md` | 同步替换 |
| 各客户端 | 必须重新导入链接。IP 写死在链接里，这是「不依赖 DNS」的代价 |
| 域名 A 记录 | 本次 `openresource.cn` 与 `www` 已跟着指向新 IP，实测无需手工改；换 IP 后仍应确认一次 |
| 证书 | 无需重签，证书绑域名不绑 IP |

验证 A 记录时注意：本机 DNS 被云壳劫持，直接 `dig` 拿到的是 sinkhole 地址。要经隧道查 DoH 才拿得到真实结果：

```bash
curl -s --socks5-hostname 127.0.0.1:<port> "https://dns.google/resolve?name=openresource.cn&type=A"
```
