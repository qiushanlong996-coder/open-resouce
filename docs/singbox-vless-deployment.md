# sing-box VLESS over TLS 部署文档（156.225.28.10）

厂商确认该服务器**不开放 UDP 端口**，WireGuard 无法使用（排查过程见 [wireguard-deployment.md](./wireguard-deployment.md)）。本文档记录改用 TCP 承载的实际可用方案。

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
| 协议 | VLESS + TLS，flow `xtls-rprx-vision` |
| 端口 | **TCP 8444** |
| 证书 | `/etc/open-resouce/tls/openresource.cn.pem`（Let's Encrypt，与网站共用，只读引用） |
| SNI | `openresource.cn` |
| 服务 | `sing-box.service`，已 `enable`，`Restart=on-failure` |
| 配置 | `/etc/sing-box/config.json`（600），UUID 单独存 `/etc/sing-box/uuid` |
| 客户端配置 | `/root/sing-box-clients/`（`link.txt`、`desktop-tun.json`、`README.md`） |

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

用真实 sing-box 客户端（macOS arm64）连公网 IP 做端到端验证，不是只看端口监听：

| 测试项 | 结果 |
| --- | --- |
| 出口 IP（直连基线 `106.11.31.1`） | 经隧道变为 **156.225.28.10** |
| `www.google.com` | 200（修 DNS 前为 000） |
| `github.com` / `www.youtube.com` / `www.wikipedia.org` | 均 200 |
| 10MB 下载吞吐 | 约 906 KB/s |
| uTLS `chrome` 指纹 | 可用，已写入分享链接 |

## 5. 客户端接入

分享链接在服务器 `/root/sing-box-clients/link.txt`：

```
vless://<UUID>@156.225.28.10:8444?encryption=none&flow=xtls-rprx-vision&security=tls&sni=openresource.cn&type=tcp&fp=chrome#openresource-hk
```

**server 填 IP、SNI 单独填域名**是刻意的：既不依赖 DNS 解析，也绕开域名被接入商备案拦截的问题（见 git 历史中关于备案拦截的记录）。

| 平台 | 做法 |
| --- | --- |
| iOS | sing-box 官方 App 或 Shadowrocket，粘贴上面的 vless:// 链接导入。两者都支持 Vision 和 chrome uTLS 指纹 |
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

新增用户：在 `config.json` 的 `inbounds[0].users` 里加一项（各自独立 UUID），`check` 通过后 `restart`。

### 需要留意

1. **证书 2026-11-03 到期**。sing-box 在启动时读取证书文件，续期后必须 `systemctl restart sing-box`，否则会继续用旧证书。建议把这一步加进证书续期流程，和 nginx 的 reload 放在一起。
2. 客户端的 `flow` 必须是 `xtls-rprx-vision`，与服务端一致；留空会握手失败。
3. CentOS 7 已 EOL，sing-box 是静态二进制不受影响，但升级需手动替换 `/usr/local/bin/sing-box` 后重启服务。
4. WireGuard 已于 2026-08-06 从服务器上完全清理（服务、接口、内核模块、密钥、源码、构建依赖，并把 `ip_forward` 恢复为 0），清理明细见 [wireguard-deployment.md](./wireguard-deployment.md) 第 9 节。现在这台机器上的出网方案只有 sing-box 一套，不存在两套并存的情况。
