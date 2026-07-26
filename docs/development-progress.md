# 新猿译码开发进度

本文档是跨电脑开发的持续交接记录。每完成并推送一个小功能，都必须同步更新本文件。

## 当前基线

- 当前分支：`main`
- 当前阶段：一期第 1 周，后端基础骨架
- 前端：`apps/web` React Demo，可构建、可本地预览
- 后端：开始实现 `services/gateway`
- 生产服务器：`www.lovenuaa.xyz`，CentOS 7

## 2026-07-26：Gateway 健康检查

### 已完成

- 新增根 Go Module，目标版本为 Go 1.24。
- 新增 Gateway 服务入口。
- 新增 `GET /healthz` 与 `GET /readyz`。
- 两个接口均返回：

```json
{"service":"gateway","status":"ok"}
```

- 新增优雅停机处理，响应 `SIGINT` 和 `SIGTERM`。
- 新增健康检查及未知路由测试。
- 更新开发部署文档中的 Gateway 启动与验证命令。

### 验证

- 已使用 Go 1.24.6 完成 Linux/amd64 测试二进制编译。
- 已在生产服务器隔离目录运行完整测试，健康检查和未知路由测试全部通过。
- Windows 本机应用控制策略禁止执行临时编译的测试程序，因此采用交叉编译后在 Linux 服务器运行的验证方式。
- 已部署并验证 `http://127.0.0.1:18080/healthz`。

### 服务器现状与注意事项

- 服务器已有业务正在使用 `www.lovenuaa.xyz`、Nginx 和本机 `8080` 端口。
- `8080` 当前由 Java 服务监听，禁止占用或停止。
- 现有 Nginx 配置指向 `/opt/more-offer/current/frontend/dist`，禁止覆盖。
- 新猿译码使用独立目录 `/opt/open-resouce`。
- Gateway 预留宿主机端口 `18080`，未完成隔离部署前不要修改现有 Nginx。
- 服务器没有安装 Go；Docker CLI 已安装，但初次检查时守护进程未运行。
- Docker Hub 当前连接不稳定，国内镜像源也出现过 TLS 握手超时；部署脚本不能假设每次都能在线拉取构建镜像。
- 不得把服务器密码、JWT 密钥或其他敏感配置写入仓库。

### 下一步

- [x] 完成 Go 1.24 环境中的测试。
- [ ] 增加 Gateway 多阶段 Dockerfile。
- [x] 将 Gateway 隔离部署到服务器 `18080`。
- [ ] 确定独立 API 子域名或 Nginx 路由。

## 2026-07-26：Gateway systemd 隔离部署

### 已完成

- Gateway 默认监听地址调整为 `127.0.0.1:8080`。
- 新增 `HOST` 与 `PORT` 环境变量，支持按环境覆盖监听地址。
- 新增监听地址配置测试。
- 新增 `deploy/systemd/open-resouce-gateway.service`。
- 使用 Go 1.24.6 交叉编译静态 Linux/amd64 二进制。
- 部署目录：`/opt/open-resouce/current/bin/gateway`。
- systemd 服务名：`open-resouce-gateway`。
- 服务已设置为开机启动。

### 验证

- 更新后的全量 Gateway 测试在服务器运行通过。
- `systemctl is-active open-resouce-gateway` 返回 `active`。
- `curl http://127.0.0.1:18080/healthz` 返回：

```json
{"service":"gateway","status":"ok"}
```

- `ss -lntp` 确认服务只监听 `127.0.0.1:18080`。

### 运维命令

```bash
systemctl status open-resouce-gateway
journalctl -u open-resouce-gateway -n 100 --no-pager
systemctl restart open-resouce-gateway
curl http://127.0.0.1:18080/healthz
```

### 注意事项

- 当前 Gateway 仅供服务器本机访问，公网和现有域名不会转发到该端口。
- 未确定独立 API 域名之前，不修改现有 `www.lovenuaa.xyz` 的 Nginx 路由。
- 后续部署必须先通过测试，再替换二进制并重启 systemd 服务。
- 若部署失败，检查 `journalctl -u open-resouce-gateway`，不要停止占用 `8080` 的既有 Java 服务。

### 下一步

1. 新增统一 JSON 响应、请求 ID 和访问日志中间件。
2. 增加 API 版本入口 `/api/v1`。
3. 为前端准备可配置的 Gateway 基础地址。
