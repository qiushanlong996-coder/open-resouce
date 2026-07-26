# 新猿译码开发进度

本文档是跨电脑开发的持续交接记录。每完成并推送一个小功能，都必须同步更新本文件。

## 环境与人工安装事项

- 自动安装或自动配置环境失败时，必须立即在本节记录，并通知项目负责人手动处理。
- 记录内容必须包含：目标机器、软件名称、准确版本、用途、安装或验证命令、是否阻塞当前功能。
- 当前本机没有系统级 Go。开发流程临时使用工作区内 Go 1.24.6，不阻塞当前开发。
- 当前服务器没有安装 Go。Gateway 使用本地交叉编译的静态二进制部署，不阻塞当前开发。
- 服务器 Docker 镜像源连接不稳定。进入数据库与中间件 Compose 部署前需要再次验证；若仍失败，需要项目负责人协助手动配置稳定镜像源。
- 已解决：HTTPS 连接 GitHub 443 连续超时。2026-07-26 已生成并添加专用 ed25519 密钥，GitHub SSH 认证成功，仓库远程地址已切换为 SSH。

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

- [x] 新增统一 JSON 响应、请求 ID 和访问日志中间件。
- [ ] 增加 API 版本入口 `/api/v1`。
- [ ] 为前端准备可配置的 Gateway 基础地址。

## 2026-07-26：Gateway 请求追踪与统一错误

### 已完成

- 新增 `X-Request-ID` 中间件。
- 客户端提供合法长度的请求 ID 时原样透传；未提供或超过 128 字符时自动生成。
- 新增结构化访问日志，记录请求 ID、方法、路径、状态码和耗时。
- 未知路由统一返回 JSON 错误 `route_not_found`。
- 不支持的请求方法统一返回 JSON 错误 `method_not_allowed`，并设置 `Allow` 响应头。
- 抽取统一 JSON 响应方法，为后续 API 接口复用。

### 验证

- Go 1.24.6 Linux/amd64 编译成功。
- 更新后的 8 组测试及子测试全部通过。
- 已原子替换服务器 Gateway 二进制并重启服务。
- 带 `X-Request-ID: deploy-check-001` 请求健康检查，响应头正确透传。
- 请求 `/missing` 返回 HTTP 404 和统一 JSON 错误。
- `journalctl` 已确认访问日志包含请求 ID、方法、路径、状态码和耗时。

### 卡点与注意事项

- 非阻塞：服务器 `journalctl` 的历史日志起止时间显示顺序异常，可能经历过系统时间或时区调整。当前实时日志时间正确，暂不影响 Gateway；后续进行日志聚合或证书自动化前应检查 `timedatectl` 和 NTP。
- 客户端应保存响应中的 `X-Request-ID`，以便后续排查问题。

### 下一步

- [x] 新增 `/api/v1` 版本入口和服务信息接口。
- [ ] 增加安全响应头与 CORS 基础策略。
- [ ] 为前端准备统一 API Client。

## 2026-07-26：API v1 版本入口

### 已完成

- 新增 `GET /api/v1` 服务信息入口。
- 返回 Gateway 服务名、API 版本和运行状态。
- `/api/v1/` 下尚未实现的资源继续使用统一 JSON 404 错误。
- 新增版本入口和未知版本化路由测试。

### 验证

- Go 1.24.6 Linux/amd64 编译成功。
- 更新后的 10 组测试及子测试全部通过。
- 已部署到服务器并重启 `open-resouce-gateway`。
- `GET http://127.0.0.1:18080/api/v1` 返回：

```json
{"service":"gateway","api_version":"v1","status":"ok"}
```

- systemd 服务状态为 `active`。

### 注意事项

- `/api/v1` 当前是服务发现入口，不代表项目、用户等业务接口已经实现。
- 新增业务接口必须放在 `/api/v1/` 命名空间内，避免未来版本升级破坏客户端。

### 下一步

- [x] 增加安全响应头与 CORS 基础策略。
- [ ] 为前端准备统一 API Client 和开发代理。
- [ ] 开始项目列表只读 API 的数据契约设计。

## 2026-07-26：安全响应头与 CORS

### 已完成

- 新增 API 安全响应头中间件。
- 增加 CSP、Permissions Policy、Referrer Policy、`nosniff` 和禁止 iframe 嵌入策略。
- 新增基于 `CORS_ALLOWED_ORIGINS` 的精确来源白名单。
- 支持逗号分隔配置多个前端来源。
- 允许跨域请求携带 `Authorization`、`Content-Type` 和 `X-Request-ID`。
- 向浏览器暴露 `X-Request-ID`，预检缓存 600 秒。
- 未配置白名单时，跨域预检统一返回 `origin_not_allowed` JSON 错误。

### 验证

- Go 1.24.6 Linux/amd64 编译成功。
- 更新后的 13 组测试及子测试全部通过。
- 已部署并重启 `open-resouce-gateway`。
- `/api/v1` 响应包含全部预期安全响应头。
- 未授权来源 `https://untrusted.example` 的 OPTIONS 预检返回 HTTP 403。

### 配置与注意事项

- 本地前端联调时设置：

```text
CORS_ALLOWED_ORIGINS=http://127.0.0.1:5173,http://localhost:5173
```

- 生产环境必须填写准确域名，不使用 `*`。
- 服务器当前未配置允许来源，因为 Gateway 尚未通过 Nginx 暴露，不影响现有服务。
- 本功能没有新增人工安装事项。

### 下一步

1. 为前端准备统一 API Client 和开发代理。
2. 在页面展示 Gateway 连通状态。
3. 开始项目列表只读 API 的数据契约设计。
