# openresource.cn TLS 证书

`openresource.cn.pem` 是**公钥证书链**（叶证书 + 中间证书，2 层）。
证书本身是公开信息 —— 任何人握手就能拿到，入库没有风险。

| 项 | 值 |
| --- | --- |
| 覆盖域名 | `www.openresource.cn` + `openresource.cn`（SAN 两个都有） |
| 签发 | 阿里云免费 DV（DigiCert Encryption Everywhere DV TLS CA - G2） |
| 有效期 | 2026-08-05 → **2026-11-02**（只有 3 个月） |
| 阿里云订单号 | `26454481` |

## 私钥不在这里，也不会入库

`.key` **故意没有提交**。私钥进 git 会永久留在历史里，且对任何有仓库权限的人可见；
而这个证书是可以重新下载的，所以入库只有代价没有收益。

**换电脑后怎么拿私钥**：登录阿里云控制台 → 数字证书管理 → 找订单 `26454481`
→ 下载 Nginx 格式，得到 `26454481_www.openresource.cn_nginx.zip`，
里面就是这份 `.pem` 加对应的 `.key`。

部署位置（应用服务器）：

```sh
install -m 0644 www.openresource.cn.pem /etc/open-resouce/tls/openresource.cn.pem
install -m 0600 www.openresource.cn.key /etc/open-resouce/tls/openresource.cn.key
nginx -t && systemctl reload nginx
```

校验证书与私钥是否配对（两个 md5 必须一致）：

```sh
openssl x509 -in openresource.cn.pem -noout -pubkey | openssl md5
openssl pkey -in openresource.cn.key -pubout | openssl md5
```

## 到期提醒

免费 DV 只有 3 个月。到期前重新下载覆盖上面两个文件即可。
旧机上装过一个周检脚本（剩余 <21 天写 syslog），新机还没装，见
`docs/development-progress.md` 里的交接清单。
