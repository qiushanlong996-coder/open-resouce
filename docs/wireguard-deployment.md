# WireGuard 部署文档（156.225.28.10）

本文档记录在香港服务器 `156.225.28.10`（CentOS 7.6，内核 3.10.0-957.1.3.el7）上部署 WireGuard 的过程、验证结果和当前阻塞点。

监听端口为 **UDP 8443**，与 nginx 占用的 **TCP 8443** 不冲突：两者是不同协议，可以共存。

> **本文是历史排查记录，所描述的部署已于 2026-08-06 从服务器上完全移除。**
>
> 已向云厂商确认，**该服务器不开放 UDP 端口**，第 6 节的抓包判断得到证实，WireGuard 在这台机器上无法使用。实际投入使用的是改走 TCP 的方案，见
> [singbox-vless-deployment.md](./singbox-vless-deployment.md)。
>
> 清理明细见第 9 节。保留本文的价值在于：第 2.1 节的 `compat.h` 补丁（在 RHEL 7.6 老内核上编译 WireGuard 的必要修改）和第 6 节的 UDP 阻断诊断方法，日后若换机器或厂商放通 UDP 可直接复用。
>
> 另注：本文中的公网 IP `156.225.28.10` 已于 2026-08-07 被云厂商变更为 `103.149.91.140`，旧 IP 已失效。文中保留旧 IP 是为了忠实记录当时的排查现场。

## 1. 部署结果概览

| 项 | 状态 |
| --- | --- |
| 内核模块 `wireguard` | 已编译、已安装、已加载（version 1.0.20220627） |
| 用户态工具 `wg` / `wg-quick` | 已安装（wireguard-tools 1.0.20210914） |
| 接口 `wg0` | 已启动，监听 `0.0.0.0:8443/udp`，地址 `10.8.0.1/24` |
| 开机自启 | `systemctl enable wg-quick@wg0` 已开启 |
| IP 转发 | 已开启并持久化到 `/etc/sysctl.d/99-wireguard.conf` |
| NAT 出网 | `POSTROUTING -s 10.8.0.0/24 -o eth0 -j MASQUERADE` |
| 隧道握手 + 出网 | **本机隔离测试已通过**（详见第 5 节） |
| 公网可达性 | **未通过**：公网 UDP 报文到不了这台机器，原因在云厂商网络侧（详见第 6 节） |

结论：服务端本身已经完全就绪且功能验证通过，唯一待解决的是云厂商侧不放通 UDP。

## 2. 为什么需要自行编译内核模块

WireGuard 从 Linux 5.6 才进入内核主线。本机内核是 3.10，且：

1. ELRepo 的 el7 仓库已不再提供 `kmod-wireguard`。
2. EPEL 7 归档中只剩用户态的 `wireguard-tools`，没有内核模块。

因此使用官方为 3.10–5.5 内核提供的向后移植分支 `wireguard-linux-compat` 现场编译。这样无需重启、无需更换内核，数据面仍然走内核态（性能优于 wireguard-go 用户态方案）。

编译依赖与运行内核**版本必须完全一致**，`kernel-devel-3.10.0-957.1.3.el7` 不在已配置的 7.9 vault 中，从 7.6.1810 的 updates 目录单独取得：

```
https://vault.centos.org/7.6.1810/updates/x86_64/Packages/kernel-devel-3.10.0-957.1.3.el7.x86_64.rpm
```

### 2.1 一个必须打的补丁

首次编译在 `src/socket.c` 失败：

```
error: 'const struct ipv6_stub' has no member named 'ipv6_dst_lookup_flow'
```

原因是上游 `compat/compat.h` 假定**所有** RHEL 7 内核都具备 `ipv6_dst_lookup_flow`。实际上该成员是 RHEL 7.9（kernel-3.10.0-1160）才回合进 `ipv6_stub` 的，本机是 RHEL 7.6（`RHEL_MINOR 6`），只有 4 参数形式的 `ipv6_dst_lookup(net, sk, dst, fl6)`——正好是上游 fallback 宏所需要的签名。

补丁（已应用，原文件备份为 `compat/compat.h.orig`）：

```c
/* RHEL 7.9 (kernel-3.10.0-1160) backported ipv6_dst_lookup_flow into ipv6_stub,
 * but earlier RHEL 7 minors only expose ipv6_dst_lookup. Upstream compat.h assumes
 * the newer API for all of RHEL 7, so cover the older minors here. */
#if defined(ISRHEL7) && RHEL_MINOR < 9
#define ipv6_dst_lookup_flow(a, b, c, d) ipv6_dst_lookup(a, b, &dst, c) + (void *)0 ?: dst
#endif
```

编译期的 `Can't read private key` 与 `objtool: chacha20_avx512vl()` 属于模块签名和 objtool 的无害告警，本机未启用 Secure Boot，可忽略。

## 3. 服务端文件清单

| 路径 | 内容 |
| --- | --- |
| `/etc/wireguard/wg0.conf` | 服务端配置（含服务端私钥、三个 peer） |
| `/etc/wireguard/server.key` / `server.pub` | 服务端密钥对 |
| `/etc/wireguard/client-{phone,laptop,spare}.{key,pub,psk}` | 客户端密钥对与预共享密钥 |
| `/root/wireguard-clients/{phone,laptop,spare}.conf` | 可直接导入的客户端配置 |
| `/etc/sysctl.d/99-wireguard.conf` | `net.ipv4.ip_forward = 1` |
| `/root/setup_wg.sh` | 生成密钥与配置的脚本，可重复执行（已存在的密钥不会被覆盖） |
| `/root/patch_and_build.sh` | 打补丁并编译安装内核模块 |
| `/root/netns_test.sh` | 隔离的端到端隧道自测脚本 |

**密钥不进仓库**：所有私钥和预共享密钥只保存在服务器上，权限 600，本文档不记录其内容。

## 4. 网络规划

| 项 | 值 |
| --- | --- |
| 隧道网段 | `10.8.0.0/24`（服务端 `10.8.0.1`） |
| 客户端地址 | phone `10.8.0.2`、laptop `10.8.0.3`、spare `10.8.0.4` |
| 监听端口 | `8443/udp` |
| 出口网卡 | `eth0`，内网地址 `10.0.227.2/24`，公网 `156.225.28.10`（1:1 映射） |
| 客户端 DNS | `1.1.1.1`, `8.8.8.8` |
| 客户端路由 | `AllowedIPs = 0.0.0.0/0`（全局代理） |

隧道网段特意选 `10.8.0.0/24`，避开 VPC 自身的 `10.0.227.0/24`，防止路由冲突。

每个 peer 都配了 `PresharedKey`，在 Noise 握手之上再加一层对称密钥，提升抗量子预计算的余量。

## 5. 端到端验证（已通过）

公网 UDP 不通，但隧道本身是否正确可以在本机用独立 network namespace 验证——这条路径不经过云厂商网络，能把「配置对不对」和「公网通不通」两个问题分开。

`/root/netns_test.sh` 的做法：建一个 `wgtest` netns，用 veth 与宿主相连，在 netns 内用 phone 的密钥起 `wg1`，Endpoint 指向服务端内网地址 `10.0.227.2:8443`，默认路由走 `wg1`。

结果：

```
latest handshake: Now
transfer: 476 B received, 564 B sent          # 客户端侧
peer rXQCg+... endpoint: 10.99.0.2:39215      # 服务端侧已识别 peer
http_code=301                                 # 经隧道 + MASQUERADE 访问 http://1.1.1.1 成功
```

即：握手、预共享密钥、AllowedIPs、转发与 NAT 出网**全链路正确**。脚本结束时会自动清理 netns 和 veth，不影响生产的 `wg0`。

## 6. 当前阻塞点：公网 UDP 进不来

安全组已放通全部 UDP 端口后，问题依旧。抓包与探测结果：

| 测试 | 结果 |
| --- | --- |
| 本地 → `156.225.28.10:8443/tcp`（nginx） | HTTP 200，**TCP 通** |
| 本地 → 该 IP 的 UDP 8443 / 51820 / 4443 / 443 / 53 / 123 / 500 | 服务器 `tcpdump -i eth0` **一个包都没抓到** |
| 以本机公网出口 IP `47.246.98.55` 为源做全端口 UDP 抓包 | 同样为 0，排除端口被改写的可能 |
| 本地 → STUN（`stun.l.google.com:19302/udp`） | 成功，说明**本地 UDP 出网正常** |
| 服务器 → STUN（同上） | 超时，服务器**高端口 UDP 出网也不通** |
| 服务器 → `1.1.1.1:53/udp`、`114.114.114.114:53/udp` | 正常，DNS 可解析 |

判断依据：`tcpdump` 在 eth0 上抓包的位置**早于 netfilter**，而且宿主的 iptables 是空规则、`INPUT` 策略为 `ACCEPT`、SELinux 为 Disabled。既然连网卡都没看到包，说明丢包发生在**实例之外的上游**（云厂商 EIP/NAT 网关或云防火墙），机器内部任何配置都不可能是原因。

再结合「出方向 UDP 只有 53 能通、高端口不通」，最可能的情况是该公网 IP 的映射只对 TCP（及 UDP/53）生效，UDP 没有对应的映射条目。

### 6.1 建议的处理顺序

1. 检查公网 IP 是不是通过 **NAT 网关 / EIP 端口映射**接入的。若是，需要为 `8443/udp` 单独添加 DNAT 条目——安全组放通不等于映射存在。
2. 确认没有额外的**云防火墙**实例挡在安全组之前。
3. 若厂商本身不承载 UDP（部分香港线路为防滥用会全局丢弃非 53 的 UDP），则 WireGuard 无法直接使用，需要改走 TCP 承载：`udp2raw`、`phantun` 或 `wstunnel` 把 UDP 封进 TCP。注意 TCP 8443 已被 nginx 占用，需另选端口或用 nginx `stream` 模块按 SNI 分流。

## 7. 若日后需要重建

前提是厂商放通 UDP，否则重建也不可用。关键信息都在本文里：第 2 节的编译方式与 `kernel-devel` 取得途径、第 2.1 节的 `compat.h` 补丁、第 4 节的网络规划、第 5 节的 netns 自测方法。

注意内核绑定问题：模块与编译时的内核版本一一对应，换内核必须用对应版本的 `kernel-devel` 重新编译。若希望自动跟随内核升级，用 DKMS 管理而不是手工编译。

## 8. 排查过程中值得留存的两点方法

1. **用 netns 把「配置对不对」和「网络通不通」分开**。公网不通时，在本机建独立 network namespace 起一个客户端连服务端内网地址，可以在不受上游网络干扰的前提下验证隧道本身。这个方法对任何 VPN/代理排查都适用。
2. **`tcpdump` 的抓包点早于 netfilter**。因此「eth0 上抓不到包」可以直接排除本机所有防火墙、SELinux、服务配置因素，把问题定位到实例之外，避免在机器内部反复试错。

## 9. 清理记录（2026-08-06）

确认厂商不放通 UDP 后，按要求把 WireGuard 从服务器上完全移除，逐项复查通过：

| 对象 | 处理 |
| --- | --- |
| `wg-quick@wg0` 服务 | 已 `disable --now`，systemd 单元已不存在 |
| `wg0` 接口 | 已移除 |
| iptables 规则 | `FORWARD` 与 `nat POSTROUTING` 均已回到只有默认策略、无自定义规则 |
| 内核模块 | 已 `rmmod`，`extra/wireguard.ko` 已删除并 `depmod -a`，`modinfo` 已找不到；连带的 `udp_tunnel`、`ip6_udp_tunnel` 也已卸载 |
| 配置与密钥 | `/etc/wireguard`、`/root/wireguard-clients` 已删除（含所有私钥与预共享密钥） |
| 源码与脚本 | `wireguard-linux-compat` 源码及 `setup_wg.sh`、`patch_and_build.sh`、`netns_test.sh` 已删除 |
| `ip_forward` | 删除 `/etc/sysctl.d/99-wireguard.conf` 并恢复为 `0`（原值）。已先确认无 Docker、无网桥、无其他服务依赖转发，sing-box 是用户态代理不需要 |
| 软件包 | `wireguard-tools`、`kernel-devel`、`elrepo-release` 已卸载，`elrepo.repo` 已删除 |

清理后复测：sing-box `active/enabled`，经隧道出口 IP 仍为 `156.225.28.10`，google/youtube 均 200；网站 443/8443/80 分别 200/200/200，nginx 与 gateway 均未受影响。

仍留在机器上的 `gcc`、`tcpdump`、`kernel-headers`、`libmnl` 是本次排查期间安装的通用工具，本身与 WireGuard 无关，故保留；如需一并卸载可另行处理。
