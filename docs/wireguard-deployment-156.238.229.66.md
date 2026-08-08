# WireGuard 部署记录 — 156.238.229.66（美国）

部署日期：2026-08-08

这台是替换 `38.60.78.93` 的新机器。和前两台最大的区别：**厂商把 UDP 全端口都放进来了**，所以不用再折腾 sing-box / TCP 伪装，直接上 WireGuard。

## 1. 机器概况

| 项 | 值 |
|---|---|
| 系统 | CentOS Stream 9 |
| 内核（部署前） | 5.14.0-80.el9 |
| 内核（部署后） | 5.14.0-731.el9 |
| CPU / 内存 / 磁盘 | 4 核 Xeon E5-2696 v4 / 3.6 GB / 30 GB |
| 网卡 | eth0 = `10.0.181.4/24`，MTU 1500 |
| 公网 IP | `156.238.229.66`（厂商 1:1 NAT，机器上看不到公网地址） |
| 防火墙 | 无（firewalld / nftables / iptables 全部 inactive，且 **nft 二进制都没装**） |
| SELinux | Disabled |
| WireGuard 内核模块 | `wireguard.ko` v1.0.0 内核自带，不需要编译 kmod |

部署前只有 sshd(22) 和 **rpcbind(TCP+UDP 111)** 在对公网监听。

## 2. UDP 连通性实测（部署前必做）

用 python3 在服务器上一次绑定 10 个 UDP 端口做回显，从本地打过去数回包。**不装任何软件**，所以可以在动系统之前先测。

脚本思路（`udpecho.py` / `probe.py`）：服务端 `select` 多端口收包后原样回 `ECHO:<port>:<payload>`；客户端每个端口发 4 包，统计回显数。

结果：

| 端口 | 51820 | 8443 | 443 | 53 | 123 | 4500 | 1194 | 2408 | 8000 | 39281 |
|---|---|---|---|---|---|---|---|---|---|---|
| 回显 | 4/4 | 4/4 | 4/4 | 4/4 | 4/4 | 4/4 | 4/4 | 4/4 | 4/4 | 4/4 |

包长测试（同样 3/3 全通）：148 / 512 / 1200 / 1372 / 1420 / 1472 / 1500 / 2000 字节载荷。1500 B 载荷即 1528 B 的 IP 包也能过，说明路径 MTU 不是瓶颈，分片也正常。

**结论：UDP 入站出站双向全通、零丢包、不限包长。**

## 3. 端口选 UDP 443 的理由

既然全通，选哪个都行，选 443 是因为：

- 这台机器上没有 nginx，443 是空的（不像 `38.60.78.93` 那台 443 被 nginx 占着）
- UDP/443 在链路上看起来就是 QUIC / HTTP3，是当下最常见的 UDP 流量，比 51820 不容易被识别和限速
- 51820 是 WireGuard 默认端口，扫段的人第一个就扫它

## 4. 装包前的坑：系统滞后

`dnf history` 显示**最后一次事务是 2022 年 5 月**。CentOS Stream 9 是滚动仓库，直接 `dnf install wireguard-tools` 会拉 2026 年的依赖去替换 2022 年的 glibc/openssl —— 这正是之前把某台机器 sshd 弄坏的原因。

安全顺序：

1. **先让 dnf 脱离 SSH 独立跑**（`setsid nohup`），SSH 断了也不会中途打断事务 —— 事务被打断才是真正弄坏系统的原因
2. 同时放一个**看门狗**在后台循环（已在内存里运行的 bash 不受 glibc 替换影响），发现 22 端口不监听就把 sshd 拉起来
3. 全量 `dnf -y update` → 331 个包，`UPDATE_EXIT=0`
4. 重启换新内核，确认 sshd active、`wireguard.ko` 在新内核里也在
5. **然后**才装 wireguard-tools

## 5. 部署内容

脚本：`/tmp/wgdeploy/wg_deploy_229.sh`（幂等，密钥一旦生成不再覆盖，重跑不会让已分发的配置失效）。

```
/etc/wireguard/
  server.key / server.pub        服务端密钥
  phone.key / phone.pub / phone.psk
  pc.key    / pc.pub    / pc.psk
  wg0.conf                       主配置（600）
  wg0-nat.nft                    NAT + MSS clamp 规则
/etc/sysctl.d/99-wireguard.conf  ip_forward=1, src_valid_mark=1
/root/wg-clients/{phone,pc}.conf 客户端配置（600）
```

网络参数：

- 隧道网段 `10.66.66.0/24`，服务端 `.1`，手机 `.2`，电脑 `.3`
- MTU 1420（1500 − 80 WireGuard 开销）
- 服务端公钥 `BXQ7xEAGx4Nc1WEV0lLEYP8e74wWs40ygV8z7VTiiUM=`

nft 规则（由 wg-quick 的 PostUp/PostDown 挂载和卸载，不依赖 nftables.service）：

```
table ip wgnat {
  chain post {
    type nat hook postrouting priority srcnat; policy accept;
    ip saddr 10.66.66.0/24 oifname "eth0" masquerade
  }
  chain wgfwd {
    type filter hook forward priority filter; policy accept;
    iifname "wg0" tcp flags syn tcp option maxseg size set rt mtu
    oifname "wg0" tcp flags syn tcp option maxseg size set rt mtu
  }
}
```

客户端统一带 `PersistentKeepalive = 15`——上一台机器手机空置一两小时就连不上，是 NAT 映射被回收，keepalive 是对症的。

### 踩到的两个坑

1. **`nft` 命令不存在**。部署前我跑 `nft list ruleset` 得到空输出，读成了"没有规则"，实际是没装 nftables 包。wg-quick 的 PostUp 直接 `nft: command not found`。装 `nftables` 即可。
2. **`fwd` 是 nft 保留字**。链名叫 `fwd` 会报 `syntax error, unexpected fwd, expecting string or last`，而报错行号指向下一行，容易看错地方。改名 `wgfwd`。

## 6. 端到端验证

本机（macOS）没有 wg 工具，不想为测试改动本机，所以**在服务器内起一个网络命名空间当客户端**：veth 连到宿主，命名空间里建 wg0，用临时密钥+临时 peer，测完自动清理（trap EXIT）。这样能完整跑通握手、加解密、转发、masquerade；公网那一段已由第 2 节的 UDP 回显证明。

第一次测试失败过一次，是我自己脚手架的问题：命名空间里加了 `ip rule not fwmark 51820 table 51820` 策略路由，rp_filter 反查源地址时算出应走 wg0，而包实际从 veth 进来，被丢掉。表现是**握手成功（客户端只收到 92 B，正好一个握手响应）但数据面不通**。改成默认路由留在 veth 上、只把测试目标 IP 路由进隧道，就不需要 fwmark，问题消失。

验证结果：

| 项 | 结果 |
|---|---|
| 隧道内 ping 服务端 10.66.66.1 | 3/3，0.55 ms |
| 隧道内 ping 1.1.1.1（经服务器出网） | 4/4，1.8 ms |
| 隧道内访问 `https://1.1.1.1/cdn-cgi/trace` | **`ip=156.238.229.66`** ← 转发+masquerade 正确的决定性证据 |
| 服务器自身直拉 Cloudflare 10 MB | 7861635 B/s（约 63 Mbps），完整 10000000 B |
| 重启后 | wg-quick@wg0 自动 active，443 监听，2 peer，nft 表在，ip_forward=1 |
| 向 UDP 443 发无效握手包 | 无响应（WireGuard 静默丢弃，不暴露端口） |

本机到服务器的路径质量（40 包 ICMP）：

| | 156.238.229.66（新） | 38.60.78.93（旧） |
|---|---|---|
| RTT avg | **168.6 ms**（min 167.7 / max 171.1，stddev 0.86） | 之前测得 185 ms |
| 丢包 | **0 %** | 之前 1.7 %，2026-08-08 复测 100 % 不通 |

新机 stddev 只有 0.86 ms，抖动比旧机小一个量级。

## 7. 运维备忘

- 客户端配置在 `/root/wg-clients/`，**含私钥和 PSK，不进仓库**。本地副本放 `/tmp`。
- 加设备：生成密钥 → `wg set wg0 peer <pub> preshared-key <(...) allowed-ips 10.66.66.N/32`，同时写进 `wg0.conf` 才能扛重启。
- 看状态：`wg show`；看握手时间和 transfer 计数判断对端是否活着。
- `qrencode` 不在 CentOS Stream 9 基础仓库里（在 EPEL）。为了不给这台机器加第三方仓库，二维码是在本地用 python 的 `segno`（纯 python，装在 /tmp）生成的。

## 8. 待办 / 未处理

- **rpcbind 仍在 TCP+UDP 111 上对公网监听**，没有任何业务用到它。关掉是 `systemctl disable --now rpcbind rpcbind.socket`，**尚未执行，等确认**。
- 旧机 `38.60.78.93` 上的 sing-box（VLESS+TLS，TCP 8443）和 WireGuard 都还在，本次没有动它们。该机 2026-08-08 复测 ICMP 100% 不通。
