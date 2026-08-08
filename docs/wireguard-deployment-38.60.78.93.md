# WireGuard 部署文档（38.60.78.93）

本文记录在美东服务器 `38.60.78.93`（CentOS Stream 9）上部署 WireGuard 的过程与验证结果。**已部署完成并验证通过，可直接投入使用。**

这台机器与香港那台（`103.149.91.140`，原 `156.225.28.10`）的关键差别是**公网 UDP 出入双向全通**，因此不必像香港那样退化成 TCP 承载的 sing-box 方案，见 [wireguard-deployment.md](./wireguard-deployment.md) 与 [singbox-vless-deployment.md](./singbox-vless-deployment.md)。

## 1. 结果概览

| 项 | 状态 |
| --- | --- |
| 内核模块 `wireguard` | **主线内核自带**（内核 5.14.0-731.el9），`modprobe wireguard` 直接成功，无需编译 |
| 用户态工具 | `wireguard-tools 1.0.20210914-4.el9`、`nftables 1.0.9`、`qrencode`（EPEL） |
| 接口 `wg0` | active，监听 `0.0.0.0:443/udp` + `[::]:443`，地址 `10.8.0.1/24` |
| 开机自启 | `systemctl enable wg-quick@wg0` 已开启 |
| IP 转发 | `/etc/sysctl.d/99-wireguard.conf` → `net.ipv4.ip_forward = 1` |
| NAT 出网 | nft 表 `ip wg0nat`，挂在 wg-quick 的 PostUp/PostDown 上，随接口生死 |
| 隧道握手 + 转发 + NAT | **netns 隔离测试通过**（第 5 节） |
| 公网 UDP 443 入向 | **抓包确认可达**（第 6 节） |

## 2. 为什么这样选参数

| 项 | 值 | 理由 |
| --- | --- | --- |
| 监听端口 | **UDP 443** | 比默认 51820 更容易穿过限制性网络（酒店/公司 Wi‑Fi 常只放 443）。这台机器没有 nginx，且 TCP 443 与 UDP 443 是不同协议、互不冲突 |
| 隧道网段 | `10.8.0.0/24`（服务端 `10.8.0.1`） | 避开 VPC 自身的 `10.0.21.0/24`，防止路由冲突 |
| 客户端地址 | phone `10.8.0.2`、laptop `10.8.0.3`、spare `10.8.0.4` | 一机一 peer，出问题可单独吊销 |
| PresharedKey | 每个 peer 各一份 | 在 Noise 握手之上再加一层对称密钥，提升抗量子预计算余量 |
| 客户端 DNS | `1.1.1.1`, `8.8.8.8` | 走隧道解析，避免本地 DNS 污染/劫持（香港那台曾遇到云壳把域名解析到 `59.82.113.122` 返 403） |
| 客户端 AllowedIPs | `0.0.0.0/0, ::/0` | 见下方说明 |
| NAT 实现 | nft 规则文件 + PostUp/PostDown | 不留持久规则，接口停掉就干净；规则文件用 `add table` / `delete table` / 再定义的写法，重复启停不会堆叠 |

**为什么 AllowedIPs 要带 `::/0`**：服务端只有 IPv4，隧道内也没有 IPv6。如果只写 `0.0.0.0/0`，客户端的 IPv6 流量会**绕过隧道直连**——国内移动网络 IPv6 覆盖很广，大量站点会被直连命中，全局代理就形同虚设。加上 `::/0` 后 IPv6 默认路由被吸进隧道并被服务端丢弃，应用经 Happy Eyeballs 回落到 IPv4 走隧道。代价是访问纯 IPv6 站点会失败，并且首次连接有约 250ms 的回落延迟——为了不泄漏，这个代价值得付。

## 3. 服务端文件清单

| 路径 | 内容 |
| --- | --- |
| `/etc/wireguard/wg0.conf` | 服务端配置（含服务端私钥、三个 peer） |
| `/etc/wireguard/server.key` / `server.pub` | 服务端密钥对 |
| `/etc/wireguard/client-{phone,laptop,spare}.{key,pub,psk}` | 客户端密钥对与预共享密钥 |
| `/etc/wireguard/wg0-nat.nft` | NAT 规则，由 PostUp 加载 |
| `/root/wireguard-clients/{phone,laptop,spare}.conf` | 可直接导入的客户端配置 |
| `/root/wireguard-clients/{phone,laptop,spare}.png` | 对应二维码（手机扫码导入） |
| `/etc/sysctl.d/99-wireguard.conf` | `net.ipv4.ip_forward = 1` |
| `/root/setup_wg.sh` | 生成密钥与配置，**可重复执行**（已存在的密钥不会被覆盖） |
| `/root/netns_test.sh` | 隔离的端到端隧道自测脚本 |

**密钥不进仓库**：所有私钥与预共享密钥只存在服务器上，权限 600、目录 700，本文档不记录其内容。

## 4. 部署前必须先做的事（这次踩过坑）

首次部署时直接 `dnf install -y wireguard-tools nftables` **把 sshd 弄坏了，最终只能重装系统**。系统内核是 2022 年 8 月的 `5.14.0-80.el9`，落后约 4 年、有 356 个待更新包；依赖求解把 `openssl-libs` 从 3.0.1 升到 3.5.7，而 `openssh-server` 仍是 8.7p1，版本错配后 sshd 的每连接子进程全崩。

重装后按下面的顺序重做，一次通过：

1. **先判断滞后程度**：`uname -r`、`dnf check-update | wc -l`、对比 `rpm -q openssl-libs` 与仓库版本。
2. **全量对齐再装东西**，且更新要**脱离 SSH 会话**跑，掉线不会留下半更新的系统：
   ```bash
   setsid bash -c 'dnf -y update > /root/update.log 2>&1; echo "EXIT=$?" > /root/update.done' < /dev/null > /dev/null 2>&1 &
   ```
   本次 636 个包，`EXIT=0`。
3. **重启前先用新连接验证 sshd 健康**：`ldd /usr/sbin/sshd | grep -c 'not found'` 应为 0，`sshd -t` 无输出，`rpm -q openssh-server openssl-libs` 版本互相匹配。
4. 重启（35s 回来），确认 `modprobe wireguard` 成功。
5. **装包时挂一个死人开关**，万一又断连能自动回滚：
   ```bash
   printf '#!/bin/bash\ndnf -y history undo last\nsystemctl restart sshd\n' > /root/deadman.sh
   chmod +x /root/deadman.sh
   systemd-run --on-active=300 --unit=deadman /root/deadman.sh
   dnf install -y wireguard-tools nftables
   # 用**新连接**确认能登录后再取消：
   systemctl stop deadman.timer
   ```
   这次只带了一个 `systemd-resolved-252-73.el9`，没碰 openssl/openssh/glibc。
6. **`qrencode` 单独装**——它在 EPEL 不在 base 仓库，混在同一事务里会让整个事务失败（连必需包都装不上）。

另：确认 `systemd-resolved` 保持 `disabled`/`inactive`，没有 `127.0.0.53` 监听，`/etc/resolv.conf` 仍是 `1.1.1.1` / `8.8.8.8`——它是被上一步连带装上的，一旦启用会接管 DNS。

## 5. 端到端验证（已通过）

`/root/netns_test.sh` 建一个 `wgtest` netns，用 veth 与宿主相连，在 netns 内用 phone 的密钥起 `wg1`，Endpoint 指向服务端**内网**地址 `10.0.21.3:443`，默认路由走 `wg1`。这条路径不经过厂商公网，能把「配置对不对」和「公网通不通」分开。

```
客户端侧 wg1: latest handshake: 3 seconds ago; transfer: 92 B received, 180 B sent
服务端侧 wg0: peer 3vs9Kw... endpoint: 10.99.0.3:36574, handshake 4 seconds ago
经隧道 ping 10.8.0.1      : 2/2, rtt avg 0.444 ms
经隧道 + NAT 出口 IP      : 38.60.78.93        ← MASQUERADE 生效
http://1.1.1.1            : 301
https://www.google.com    : 200
```

即握手、PresharedKey、AllowedIPs、转发、NAT 出网**全链路正确**。脚本结束自动清理 netns 与 veth，不影响生产的 `wg0`。

另外验证了幂等性：`systemctl restart wg-quick@wg0` 后 `nft list table ip wg0nat` 里 masquerade 规则仍只有 1 条，不会随重启堆叠。

## 6. 公网可达性验证

从本地（出口 `119.130.58.147`）向 `38.60.78.93:443/udp` 发 5 个 148 字节包，服务器 `tcpdump -i eth0 'udp port 443'` 全部抓到：

```
119.130.58.147.56674 > 10.0.21.3.443: UDP, length 148   × 5
```

目的地址是 `10.0.21.3` 而不是公网 IP，说明厂商是 1:1 NAT 映射，映射条目对 UDP 有效。`wg0` 静默丢弃这些无效包属正确行为（WireGuard 对非法报文不回任何东西，这也是它难被主动探测的原因）。

部署前已单独实测过：入向 UDP 53/443/1194/4500/8443/51820/40000 **全部**收到 echo 回包；出向 STUN（`19302`、`3478`）均成功映射到 `38.60.78.93`。

**hairpin 不通**：在服务器自己的 netns 里把 Endpoint 指向公网 IP `38.60.78.93:443` 握手失败。这是厂商 1:1 NAT 不支持回环，**不影响真实客户端**，只是意味着无法在服务器上自测公网端点。

## 7. 客户端使用

| 客户端 | 做法 |
| --- | --- |
| iOS | App Store 装官方 **WireGuard**，「添加隧道 → 从二维码创建」，扫 `/root/wireguard-clients/phone.png` |
| macOS | App Store 装官方 **WireGuard**，「从文件导入隧道」，导入 `laptop.conf` |
| Windows | 官网客户端，「Import tunnel(s) from file」，导入 `laptop.conf` |

`spare.conf`（`10.8.0.4`）留作备用，先不要启用——**同一份配置不能在两台设备上同时使用**，两边会互相抢同一个 peer 的握手，表现为频繁掉线。要加第三台设备就用 spare，再多就在 `/root/setup_wg.sh` 的 `CLIENTS` 变量里加名字后重跑脚本（已有密钥不会被覆盖）。

导入后在服务器上确认握手：

```bash
wg show wg0 | grep -B2 'latest handshake'
```

客户端侧自查：访问 `https://ifconfig.me/ip` 应返回 `38.60.78.93`。

## 8. 运维要点

- **公网 IP 是配置里的硬编码值**。香港那台在 2026-08-07 被厂商换过 IP，如果这台也换，需要改所有客户端 `Endpoint`。若厂商给的 IP 不稳定，建议挂个域名到 `Endpoint`（WireGuard 支持域名，且会在握手失败时重新解析）。
- **`PersistentKeepalive = 25`** 让客户端每 25 秒发一个保活包，维持 NAT 映射，手机切换网络后恢复更快。
- **换密钥**：删掉 `/etc/wireguard/client-<name>.*` 后重跑 `/root/setup_wg.sh`，会生成新密钥并重写 `wg0.conf` 与客户端配置，然后 `systemctl restart wg-quick@wg0`。
- **MTU** 用默认 1420，未做调整。若在某些网络下出现「能握手、能 ping，但网页打不开」，是典型的 PMTU 黑洞，在客户端 `[Interface]` 里加 `MTU = 1380` 试。
- **`rpcbind` 在 UDP/TCP 111 对公网暴露**（外网 `rpcinfo -T udp -p` 能拿到完整程序列表）。在一台 UDP 全开又没有防火墙的机器上，这是可被利用的放大反射源。建议 `systemctl disable --now rpcbind rpcbind.socket`——本次**未执行**，待确认没有 NFS 依赖后再动。

## 9. 排查方法沉淀

1. **用 netns 把「配置对不对」和「网络通不通」分开**。在本机建独立 network namespace 起客户端连服务端内网地址，可以不受上游网络干扰地验证隧道本身。对任何 VPN/代理排查都适用。
2. **`tcpdump` 的抓包点早于 netfilter**。「eth0 上抓不到包」可以直接排除本机所有防火墙、SELinux、服务配置因素，把问题定位到实例之外。反过来，抓到了就说明厂商映射没问题。
3. **UDP 通不通要用回包来判**：在服务端起多端口 UDP echo，同时抓包 + 本地探测，可以三向区分「包没到 eth0」（上游丢）/「到了但没回包」（本机丢）/「完整回环」（全通）。单向发包看不出任何结论。
4. **改包/改网络前留退路**：死人开关（`systemd-run --on-active`）+ 长驻已认证会话 + 厂商控制台，三者至少有一个。已建立的 SSH 会话不受共享库替换影响，是最后的抢救手段。
