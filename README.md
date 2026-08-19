# Nebula

Nebula 是使用 Go 重构的 AI API 中转站。首期围绕学生、导师、老师三类用户，提供组织与项目管理、API Key 双级审批、模型与供应商配置、一次性 Key 领取、状态事件以及 OpenAI/Anthropic 兼容代理。

只读参考项目位于 `D:\VScodeProjects\NebulaCloud\nebula-ai`。本仓库重新实现业务，不兼容参考项目旧表、旧路由或历史数据。

## 首期能力

- 学生和导师邮箱验证码注册，三种角色统一登录，JWT access token 与 Redis refresh token 轮换。
- 老师 bootstrap、邀请与邀请激活。
- 学生选择组织、项目和模型白名单提交 Key 申请。
- 项目任一负责导师完成首次初审；老师完成终审。
- 老师终审通过后，学生自动加入目标组织和项目。
- 学生首次领取时生成 API Key，明文只返回一次。
- 导师可撤销负责项目中的 ACTIVE Key。
- 老师管理组织、项目、导师项目申请、供应商、模型和模型 binding。
- 认证 SSE 推送 Key 状态与全局常用模型变化。
- 标准 `/v1` OpenAI-compatible、Anthropic Messages 和 Realtime WebSocket 网关。

首期不包含计费、余额、额度、RPM、Token 限制、用量记录、通知中心、模型价格、智能路由、资源删除 API 或旧路由兼容层。

## 技术栈

| 领域 | 实现 |
| --- | --- |
| 语言 | Go 1.23+ |
| HTTP | Gin |
| ORM | Ent |
| 数据库 | PostgreSQL，UUIDv7，`citext` |
| Redis | 验证码、refresh session、老师邀请、SSE stream、视频任务路由 |
| 安全 | bcrypt、HMAC-SHA256、AES-256-GCM、JWT HS256 |
| 网关 | `net/http` 流式转发、multipart 重放、WebSocket 双向代理 |
| 本地编排 | Docker Compose、Doppler `nebula-api/dev_personal` 运行时注入 |

## Docker 本地架构

Compose 项目名固定为 `nebula-api`。Compose 项目是容器、网络和卷的逻辑分组，不是“一个容器包含多个镜像”。前端是独立的 Vue 静态应用，由 Nginx 提供同源入口并反向代理控制面与网关。

| Service | Image | 作用 | 宿主机暴露 |
| --- | --- | --- | --- |
| `postgres` | `docker.m.daocloud.io/library/postgres:17.11-alpine3.24` | PostgreSQL 与 `citext` 持久化 | 不暴露 |
| `redis` | `docker.m.daocloud.io/library/redis:8.2.8-alpine3.22` | 验证码、会话、邀请、SSE 与视频路由 | 不暴露 |
| `migrate` | `nebula-api-backend:0.3.2` | 一次性执行 `cmd/migrate`，成功后退出 | 不暴露 |
| `backend` | `nebula-api-backend:0.3.2` | 控制面、SSE 和模型网关 | `127.0.0.1:8080` |
| `cloudflared` | `cloudflare/cloudflared:2025.8.1` | Cloudflare Tunnel 到后端内部端口 | 不暴露宿主机端口 |

PostgreSQL 和 Redis 只加入内部 `data` 网络；backend 同时加入 `edge` 和 `data` 网络。数据分别保存在 Docker 命名卷 `nebula-api_postgres-data` 与 `nebula-api_redis-data`。应用容器以非 root 用户运行，丢弃 Linux capabilities，启用 `no-new-privileges` 和只读根文件系统。

项目 Docker 构建使用 `docker.m.daocloud.io/library` 与 `dockerproxy.net/library` 国内镜像前缀（前端 Node/Nginx 使用后者，已验证大层下载速度）；Go 依赖使用 `goproxy.cn`，前端 npm 依赖使用 `registry.npmmirror.com`。前端构建上下文通过 `frontend/.dockerignore` 排除本地 `node_modules` 与 `dist`。如需更换镜像，只修改两个 Dockerfile 与 Compose 中的镜像地址，不需要修改 Docker Desktop 全局 daemon 配置。

应用镜像版本由仓库根目录 `VERSION` 唯一维护，必须为完整 SemVer `X.X.X`；Compose 禁止 `latest`。第三方镜像保持上游精确 patch tag，不重新包装成无意义的本地镜像。

## 目录

```text
cmd/
  migrate/                         显式创建 citext extension 与 Ent schema
  server/                          服务入口、依赖装配、bootstrap、优雅停机
docs/
  API_DOCUMENTATION.md            控制面与网关契约
  DATABASE.md                     表结构与事务规则
  TESTING_AND_TROUBLESHOOTING.md  测试策略与支持排障
frontend/
  src/api/                        类型化控制面 API client
  src/store/                      Pinia 内存会话状态
  src/views/                      双栏认证、学生分步申请/模型广场、导师与老师队列和管理页面
  Dockerfile                      Vue 构建与非 root Nginx 镜像
  nginx.conf                      SPA 与同源 API 反向代理
scripts/
  ci-deploy.sh                    GitHub runner 的主机校验、源码同步和远端 Doppler 注入
  compose.ps1                     固定 Doppler config、改写容器内 DSN 并调用 Compose
  deploy.sh                       服务器端生产配置校验、镜像构建、迁移和健康检查
internal/
  api/http/                        Gin 路由、DTO、middleware、响应映射
  controlplane/                    认证及学生/导师/老师业务用例
  dataplane/                       Key 鉴权、binding 选择和协议代理
  domain/                          领域错误与状态规则
  infrastructure/
    cache/                         Redis 状态与事件
    crypto/                        密码、JWT、HMAC、凭据加密
    db/                            Ent Schema、生成代码与 driver
    mail/                          SMTP
Dockerfile                        Go 多阶段构建与非 root 运行镜像
compose.yaml                      本地 PostgreSQL、Redis、迁移、backend 和 frontend 编排
compose.production.yaml            生产 PostgreSQL、Redis、迁移、backend 和 Cloudflare Tunnel 编排
VERSION                           应用镜像 SemVer
```

## 配置

以 `.env.example` 为变量清单，通过 shell、IDE 或部署平台注入运行环境。本项目不会自动读取 `.env`。至少替换以下值：

- `DATABASE_URL`、`REDIS_URL`
- `JWT_SIGNING_KEY`、`AUTH_STATE_HASH_PEPPER`
- `API_KEY_HASH_PEPPER`
- `PROVIDER_CREDENTIAL_ENCRYPTION_KEY`
- 首个老师信息和 SMTP 配置

禁止把真实 `.env` 或任何凭据提交到仓库。

Docker 启动固定从 Doppler 项目 `nebula-api` 的 `dev_personal` config 注入真实配置。`scripts/compose.ps1` 只在当前进程内将 Doppler 的本地 `DATABASE_URL`/`REDIS_URL` 主机改为 `postgres`/`redis` Compose DNS，并从数据库 URL 派生 PostgreSQL 初始化参数；其余应用配置原样注入。它不创建 `.env`，也不输出 secret。为防止误连外部数据库，脚本拒绝非 loopback/Compose service host，并要求 `HTTP_ADDRESS=:8080` 与本地端口契约一致。

## 初始化与运行

首次使用需要显式创建数据库结构。迁移命令会修改 `DATABASE_URL` 指向的数据库，执行前必须确认目标环境：

```powershell
go run ./cmd/migrate
go run ./cmd/server
```

服务启动不会自动迁移数据库。默认监听 `:8080`。

前端本地开发使用 Node.js 22 与 npm：

```powershell
cd frontend
npm install
npm run dev
```

Vite 将 `/api` 和 `/v1` 代理到 `127.0.0.1:8080`。access token 只保存在 Pinia 内存中，页面刷新通过 HttpOnly refresh Cookie 恢复会话。

## Docker 启动与运维

前置条件：Docker Desktop 正在运行，Doppler CLI 已登录，并可访问 `nebula-api/dev_personal`。在仓库根目录执行：

```powershell
# 构建镜像、初始化本地数据库并后台启动全部服务
.\scripts\compose.ps1 up -d --build

# 查看状态、日志与最终解析后的镜像名称（命令不会回显 secret）
.\scripts\compose.ps1 ps
.\scripts\compose.ps1 logs --tail 100 backend
.\scripts\compose.ps1 images

# 停止并删除容器/网络，保留数据库和 Redis 数据卷
.\scripts\compose.ps1 down
```

启动顺序由健康条件约束：PostgreSQL、Redis 就绪后运行 `migrate`；迁移退出码为 0 后启动 backend。服务探针：

```powershell
Invoke-RestMethod http://127.0.0.1:8080/health/live
Invoke-RestMethod http://127.0.0.1:8080/health/ready
```

浏览器控制台入口为 `http://127.0.0.1:8081`。

`docker compose down -v` 会永久删除本地 PostgreSQL 和 Redis 命名卷，仅在用户明确要求完全重置本地数据时执行。

## 生产自动部署

`.github/workflows/deploy.yml` 在代码 push 到 `main` 后触发 `production` job。GitHub 只保存 Doppler `nebula-api/prd` 的只读 service token；服务器地址、端口、用户、密码、固定主机公钥和全部应用配置均从 Doppler 动态注入，不写入 workflow、仓库、构建参数或 `.env`。

部署顺序固定为：校验 SSH 变量与主机公钥、通过 SSH 管道将当前 commit 精确同步到 `/opt/nebula-api`、在远端进程内注入 Doppler 配置、构建 backend 镜像、启动 PostgreSQL/Redis、显式执行 migrate、重建 backend 和 Cloudflare Tunnel，并检查 backend readiness 与 Tunnel 运行状态。workflow 使用 concurrency 串行化生产部署，不会让两个 main push 同时修改部署目录。

生产使用 `compose.production.yaml`，frontend 静态资源由 Vercel 托管，backend 不映射宿主机端口，Cloudflare Tunnel 通过内部网络访问 `http://backend:8080`。Cloudflare Public Hostname 配置为 `api.lyn91r.cn`，Service 配置为 `http://backend:8080`。Vercel 项目的 Root Directory 必须为 `frontend`，`VITE_API_BASE_URL` 配置为 `https://api.lyn91r.cn`，正式前端地址为 `https://www.lyn91r.cn`。`frontend/vercel.json` 将所有前端 history 路由回退到 `/index.html`，保证 `/login`、`/teacher/...` 等地址可直接访问和刷新；静态资源仍由 Vercel 文件系统正常提供。

```powershell
& "D:\PuTTY\plink.exe" -ssh -P 22 -L 8081:127.0.0.1:8081 root@<server-host>
```

随后可打开 `http://localhost:8081` 检查页面；生产登录和 API Key 传输必须经过 HTTPS 反向代理，因为 production refresh Cookie 强制使用 `Secure`。不得直接开放 8080/8081，也不得在没有 TLS 的公网入口上进行登录或传输 API Key。服务器变更时同时更新 Doppler 中的 `DEPLOY_SSH_HOST`、`DEPLOY_SSH_PORT`、`DEPLOY_SSH_USER`、`DEPLOY_SSH_PASSWORD` 和 `DEPLOY_SSH_KNOWN_HOSTS`；workflow 无需修改。

## 生成与验证

```powershell
go generate ./internal/infrastructure/db/ent
go test ./...
go vet ./...
cd frontend
npm run typecheck
npm run test
npm run build
.\scripts\compose.ps1 config --quiet
```

API、数据库、目录、配置或功能发生变化时，必须在同一 Task 同步更新本 README、`docs/API_DOCUMENTATION.md`、`docs/DATABASE.md` 和 `docs/TESTING_AND_TROUBLESHOOTING.md`。
