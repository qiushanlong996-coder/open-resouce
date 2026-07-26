# 新猿译码开发部署文档

本文档用于指导新电脑上的环境准备、项目启动、构建部署和 Git 协作。

当前仓库处于一期基础骨架阶段。`apps/web` 前端可以直接运行，`services/gateway` 已提供首个可执行的 Go 服务和健康检查接口。MySQL、MongoDB、Elasticsearch、Redis、MinIO、NATS 等基础设施已写入技术设计，后续将按功能逐步接入。

## 1. 当前版本基线

以下版本在当前开发机上验证通过，建议其他电脑优先保持一致。

| 工具 | 推荐版本 | 用途 |
| --- | --- | --- |
| Node.js | 20.20.2，Node 20 LTS | 前端运行时 |
| npm | 10.8.2 | 依赖安装和脚本执行 |
| Vite | 8.1.5，随项目依赖安装 | 前端开发服务器和构建 |
| TypeScript | 6.0.x | 类型检查和构建 |
| Git | 2.52.0 或更新版本 | 代码和文档版本管理 |
| 浏览器 | Chrome、Edge、Safari 或 Firefox 最新两个主版本 | 页面调试和验收 |

### 1.1 操作系统

支持 Windows 10/11、macOS 和主流 Linux 发行版。Windows 推荐使用以下任一终端：

1. Git Bash。
2. PowerShell 7 或 Windows Terminal。

### 1.2 仅运行前端 Demo 不需要的环境

运行当前前端 Demo 暂时不需要安装：

1. Go。
2. MySQL。
3. MongoDB。
4. Redis。
5. Elasticsearch。
6. MinIO。
7. NATS。
8. Docker Desktop。

这些工具属于后端实现阶段的环境。开发 Gateway 需要 Go 1.24.x；如果只运行前端 Demo，则仍然不需要安装它们。

### 1.3 Gateway 健康检查

在仓库根目录启动 Gateway：

```bash
go run ./services/gateway
```

默认监听 `127.0.0.1:8080`，可通过 `HOST` 和 `PORT` 环境变量覆盖。服务提供：

```text
GET /healthz
GET /readyz
GET /api/v1
GET /api/v1/projects
GET /api/v1/projects/{slug}
GET /api/v1/projects/{slug}/documents
GET /api/v1/projects/{slug}/documents/{documentSlug}
GET /api/v1/projects/{slug}/documents/{documentSlug}/comments
POST /api/v1/projects/{slug}/documents/{documentSlug}/comments
PATCH /api/v1/projects/{slug}/documents/{documentSlug}/comments/{commentID}
```

项目列表支持 `page`、`page_size`、`q`、`category` 和 `sort` 查询参数。
`sort` 可选值为 `updated`、`downloads` 或 `stars`。

验证命令：

```bash
go test ./services/gateway
curl http://127.0.0.1:8080/healthz
```

本地前端需要跨域访问 Gateway 时，按逗号分隔精确配置允许来源：

```text
CORS_ALLOWED_ORIGINS=http://127.0.0.1:5173,http://localhost:5173
```

未配置时不向任何跨域来源返回授权响应头。不要在生产环境使用 `*`。

### 1.4 Gateway 容器镜像

从仓库根目录执行：

```bash
docker build -f services/gateway/Dockerfile -t open-resouce-gateway:dev .
docker run --rm -p 127.0.0.1:18080:8080 open-resouce-gateway:dev
```

镜像使用 Go 1.24 Alpine 构建阶段和 `scratch` 运行阶段，运行用户为
`65532:65532`。根目录 `.dockerignore` 会排除前端依赖、构建产物、文档和
Codex 临时运行时，避免把无关文件或本地敏感数据发送到 Docker 构建上下文。

## 2. 获取代码

### 2.1 使用 SSH 克隆

推荐使用 SSH，避免每次推送输入账号密码：

```bash
git clone git@github.com:qiushanlong996-coder/open-resouce.git
cd open-resouce
```

验证 SSH：

```bash
ssh -T git@github.com
```

看到类似下面的提示，说明认证成功：

```text
Hi qiushanlong996-coder! You've successfully authenticated, but GitHub does not provide shell access.
```

如果还没有 SSH 密钥，可以参考 GitHub 官方文档生成并添加公钥：

<https://docs.github.com/en/authentication/connecting-to-github-with-ssh>

### 2.2 配置 Git 用户信息

每台新电脑都需要配置提交身份：

```bash
git config --global user.name "你的 GitHub 用户名"
git config --global user.email "你的 GitHub 邮箱"
```

检查配置：

```bash
git config --global --list
git remote -v
```

远程仓库应为：

```text
git@github.com:qiushanlong996-coder/open-resouce.git
```

## 3. 前端开发环境

进入前端目录：

```bash
cd apps/web
```

### 3.1 安装 Node.js

安装 Node.js 20 LTS，并确认版本：

```bash
node --version
npm --version
```

推荐输出至少满足：

```text
v20.20.2
10.8.2
```

如果使用 nvm、fnm 或 Volta，请固定到 Node 20 LTS，避免不同电脑使用不同 Node 大版本。

### 3.2 安装依赖

项目已经提交 `package-lock.json`，新电脑必须优先使用 `npm ci`：

```bash
npm ci
```

不要在没有明确需求时删除或手动修改 `package-lock.json`。如果确实修改了依赖，必须同时提交 `package.json` 和 `package-lock.json`。

### 3.3 启动开发服务器

```bash
npm run dev
```

默认访问地址：

<http://127.0.0.1:5173>

需要让同一局域网内的其他设备访问时：

```bash
npm run dev -- --host 0.0.0.0
```

然后使用开发电脑的局域网 IP 加端口访问，例如：

```text
http://192.168.1.100:5173
```

如果 5173 端口被占用，Vite 会自动尝试下一个可用端口，请以终端输出为准。

## 4. 常用命令

在 `apps/web` 目录执行：

| 命令 | 说明 |
| --- | --- |
| `npm ci` | 按锁文件安装完整依赖 |
| `npm run dev` | 启动 Vite 开发服务器 |
| `npm run build` | 执行 TypeScript 检查并生成生产构建 |
| `npm run preview` | 预览 `dist` 构建产物 |
| `npm run lint` | 执行 Oxlint 检查 |

提交代码前至少执行：

```bash
npm run build
```

修改 Gateway API 后还必须校验 OpenAPI 契约：

```bash
node scripts/validate-openapi.mjs
```

## 5. 前端配置说明

前端默认通过同源 `/api` 访问 Gateway。开发服务器会将 `/api` 代理到
`http://127.0.0.1:18080`，一般不需要创建 `.env`。如需覆盖配置，复制
`apps/web/.env.example` 为 `apps/web/.env.local`。

可用的前端环境变量：

```text
VITE_API_BASE_URL=
VITE_API_PROXY_TARGET=http://127.0.0.1:18080
```

`VITE_API_BASE_URL` 用于浏览器实际请求地址；留空表示同源。`VITE_API_PROXY_TARGET`
仅供 Vite 开发代理使用，不会进入生产构建。

Vite 只会把以 `VITE_` 开头的变量暴露给浏览器，因此以下内容绝对不能放进前端 `.env`：

1. 数据库密码。
2. JWT 签名密钥。
3. GitHub OAuth Client Secret。
4. 微信 App Secret。
5. MinIO/S3 Secret Key。
6. OpenAI 或其他模型服务 API Key。

敏感配置必须由后端服务通过服务器环境变量或密钥管理系统读取。

## 6. 生产部署前端 Demo

### 6.1 生成生产文件

```bash
cd apps/web
npm ci
npm run build
```

构建产物位于：

```text
apps/web/dist
```

### 6.2 本地预览生产构建

```bash
npm run preview -- --host 0.0.0.0
```

### 6.3 Nginx 部署示例

将 `apps/web/dist` 上传到服务器，例如 `/var/www/xinyuan-web`，然后配置：

```nginx
server {
    listen 80;
    server_name example.com;

    root /var/www/xinyuan-web;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    location ~* \.(js|css|png|jpg|jpeg|webp|svg|woff2)$ {
        expires 7d;
        add_header Cache-Control "public, max-age=604800, immutable";
    }
}
```

当前 Demo 使用前端路由状态切换，部署为静态站点时必须保留 `try_files ... /index.html`，否则刷新非首页路径可能返回 404。

### 6.4 Gateway 服务器部署

生产服务器没有安装 Go 时，可在开发机交叉编译 Linux 二进制：

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o gateway ./services/gateway
```

将二进制上传到 `/opt/open-resouce/current/bin/gateway`，将
`deploy/systemd/open-resouce-gateway.service` 安装到 `/etc/systemd/system/`，然后执行：

```bash
systemctl daemon-reload
systemctl enable --now open-resouce-gateway
systemctl status open-resouce-gateway
curl http://127.0.0.1:18080/healthz
```

Gateway 仅监听项目预留的 `18080` 端口。为避免影响同机已有业务，在独立域名或 Nginx 路由确认前，不要修改现有站点配置。

## 7. 后端阶段环境规划

以下是技术设计中的一期基础设施规划，不代表当前 Demo 已经接入。真正开始后端开发时，应将具体镜像版本固化到 `docker-compose.yml`，并同步更新本文档。

| 服务 | 规划版本 | 主要用途 |
| --- | --- | --- |
| Go | 1.24.x | 后端微服务 |
| MySQL | 8.0.x | 用户、项目、权限、评论、审核和事务数据 |
| MongoDB | 8.0.x | Markdown 正文、内容块、文档树和版本快照 |
| Redis | 7.x | 缓存、会话、限流和实时状态 |
| Elasticsearch | 8.x | 项目、文档和代码文件名搜索 |
| MinIO | 与 S3 API 兼容的稳定版本 | 文件、图片、代码包和文档存储 |
| NATS JetStream | 2.10.x | 事件、异步任务和实时消息扇出 |

后端开发阶段至少需要配置：

```text
APP_ENV=local
HOST=127.0.0.1
PORT=8080
CORS_ALLOWED_ORIGINS=http://127.0.0.1:5173,http://localhost:5173
DATABASE_URL=mysql://...
MONGO_URI=mongodb://...
REDIS_URL=redis://...
ELASTICSEARCH_URL=http://...
NATS_URL=nats://...
S3_ENDPOINT=http://...
S3_ACCESS_KEY=...
S3_SECRET_KEY=...
JWT_SECRET=...
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
WECHAT_APP_ID=...
WECHAT_APP_SECRET=...
```

这些配置只放在本地 `.env`、服务器环境变量或密钥管理系统中，禁止提交到 GitHub。

## 8. Git 协作流程

开始工作前：

```bash
git pull --rebase origin main
```

完成代码或文档改动后：

```bash
git status
npm run build
git add apps docs README.md
git commit -m "描述本次改动"
git push origin main
```

当前仓库中的 `design-md/` 是已有的未跟踪设计资料目录，除非明确需要，否则不要自动加入提交。

推荐提交信息：

```text
feat: 新增功能
fix: 修复问题
docs: 更新文档
style: 调整样式
chore: 工具或依赖调整
```

## 9. 新电脑最短启动流程

```bash
git clone git@github.com:qiushanlong996-coder/open-resouce.git
cd open-resouce/apps/web
npm ci
npm run dev
```

浏览器打开：

<http://127.0.0.1:5173>

## 10. 常见问题

### 10.1 `npm ci` 报 Node 版本问题

确认使用 Node 20 LTS：

```bash
node --version
```

切换 Node 版本后，删除 `apps/web/node_modules`，再次执行：

```bash
npm ci
```

### 10.2 端口被占用

直接让 Vite 使用其他端口：

```bash
npm run dev -- --port 5174
```

### 10.3 GitHub SSH 认证失败

确认远程地址是 SSH：

```bash
git remote -v
```

确认 SSH：

```bash
ssh -T git@github.com
```

如果提示 `Permission denied (publickey)`，说明当前电脑的公钥没有添加到有仓库权限的 GitHub 账号，或 SSH 使用了错误的密钥。

### 10.4 PowerShell 出现 profile 执行策略提示

这通常是本机 PowerShell profile 的执行策略提示，不是项目错误。可以使用 Git Bash，或者用不加载 profile 的 PowerShell 执行命令：

```powershell
powershell -NoProfile
```

### 10.5 页面显示旧样式

先停止并重新启动 Vite；如果仍未更新，浏览器执行强制刷新。主题配置保存在浏览器 `localStorage` 中，必要时可以清理站点数据后重新查看默认主题。

## 11. 环境变更规则

以下改动必须同步更新本文档，并提交到 GitHub：

1. Node.js、npm 或 Go 版本变化。
2. 新增或移除依赖。
3. 新增环境变量。
4. 新增数据库或中间件。
5. 修改启动、构建或部署命令。
6. 修改 Git 分支和发布流程。

每次环境变更后，至少记录变更原因、影响范围和验证命令，避免跨电脑开发时出现隐式配置。
