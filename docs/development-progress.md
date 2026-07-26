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
- 待完成：部署后请求 `http://127.0.0.1:18080/healthz`

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

1. 完成 Go 1.24 环境中的测试。
2. 增加 Gateway 多阶段 Dockerfile。
3. 将 Gateway 隔离部署到服务器 `18080`。
4. 验证健康检查后，再考虑新增独立 API 子域名或 Nginx 路由。
