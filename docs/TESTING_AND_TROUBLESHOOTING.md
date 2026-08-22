# Nebula 测试与故障排查

## 1. 文档职责

本文档用于统一测试入口和支持排查口径，避免通过反复问答才能定位常见问题。新增、修改或删除任何功能时，必须在同一 Task 更新：

- 相关测试范围和验证命令；
- 用户可观察到的失败现象；
- 已知根因；
- 解决方案；
- 从低成本到高成本的排查步骤。

本文档不得记录真实邮箱验证码、JWT、refresh token、API Key、供应商凭据、加密密钥、数据库 DSN 或用户隐私数据。日志和工单只允许使用 request ID、资源 ID、Key prefix 和脱敏邮箱。

## 2. 标准验证流程

### 2.1 静态与单元验证

```powershell
go generate ./internal/infrastructure/db/ent
go test ./...
go vet ./...
cd frontend
npm run typecheck
npm run test
npm run build
```

当前自动化测试覆盖：

- API Key 合法/非法状态迁移及驳回、撤销原因规则；
- bcrypt、JWT audience/issuer、API Key HMAC、AES-GCM 防篡改；
- 标准网关路由与旧路由拒绝；
- 控制面完整路由注册，以及不存在 DELETE 和旧网关前缀；
- 供应商 URL 拼接、公开模型 ID 到上游模型名重写；
- JSON/multipart body 重放、文件内容保真和请求大小限制；
- 控制面结构化错误码到 HTTP 状态码映射；
- 安全配置长度、加密主密钥和开发 Cookie 行为。
- 前端类型检查、生产构建和关键 API client 行为。
- 导师候选游标编解码、学生新模型元数据校验与并发复用规则、nullable 模型字段三态解析、老师驳回进度映射、模型 ID 去重和 1–100 数量校验。

### 2.2 本地依赖检查

```powershell
go version
docker version
docker compose version
doppler --version
doppler configure get project --plain
.\scripts\compose.ps1 config --quiet
```

PostgreSQL 和 Redis 按设计不映射宿主机端口，不能再用 `Test-NetConnection localhost -Port 5432/6379` 判断它们。使用 `.\scripts\compose.ps1 ps` 查看容器健康状态，并通过 backend readiness 统一判断依赖。

生产测试账号凭据保存在 Doppler `nebula-api/prd`，每个角色各三组：`TEST_STUDENT_1..3_{NAME,EMAIL,PASSWORD}` 和 `TEST_MENTOR_1..3_{NAME,EMAIL,PASSWORD}`。这些值不写入仓库、不注入 backend 容器日志；登录前需确认对应用户已存在于生产数据库。

### 2.3 数据库初始化

```powershell
.\scripts\compose.ps1 up -d --build
```

该命令使用 Doppler `nebula-api/dev_personal` 的真实配置，创建/更新 Docker 命名卷中的本地数据库结构。启动脚本拒绝外部数据库 host，防止把生产 DSN 带入本地迁移。

### 2.4 Docker 验证

```powershell
.\scripts\compose.ps1 ps
.\scripts\compose.ps1 images
Invoke-RestMethod http://127.0.0.1:8080/health/live
Invoke-RestMethod http://127.0.0.1:8080/health/ready
.\scripts\compose.ps1 logs --tail 100 backend
```

本地 Compose 期望：`postgres`、`redis`、`backend` 为 healthy，`frontend` 为 running，`migrate` 为 exited (0)。生产 Compose 使用 `compose.production.yaml`，期望 `postgres`、`redis`、`backend`、`cloudflared` 运行，`migrate` exited (0)，且不运行 frontend。健康响应不得包含 DSN、secret 或异常文本。

### 2.5 生产部署验证

push 到 `main` 后，在 GitHub Actions 的 `Deploy production` workflow 中确认源码已同步到服务器、服务器本地镜像构建、显式 migrate 和两个 HTTP 健康检查均成功。服务器端只读检查：

```bash
cd /opt/nebula-api
docker compose --project-name nebula-api --file compose.production.yaml ps
curl --fail http://127.0.0.1:8080/health/ready
```

期望 `/opt/nebula-api` 保存当前部署源码，`postgres`、`redis`、`backend`、`cloudflared` 正在运行，`migrate` 成功退出。GitHub 日志不得出现 SSH 密码、Doppler token、DSN 或应用 secret。公网访问通过 Cloudflare Tunnel 的 `https://api.lyn91r.cn`，production 登录和 API Key 传输必须经过 HTTPS。

Vercel 项目必须将 Root Directory 设置为 `frontend`。前端部署完成后，直接请求 history 路由应返回控制台 HTML，而不是 Vercel 404：

```powershell
(Invoke-WebRequest -UseBasicParsing https://www.lyn91r.cn/login).StatusCode
(Invoke-WebRequest -UseBasicParsing https://www.lyn91r.cn/teacher/key-reviews).StatusCode
```

两项均应返回 `200`，随后由 Vue Router 和认证守卫决定页面内容及登录跳转。

### 2.6 首期业务验收

1. 学生和导师分别完成验证码注册，老师由 bootstrap 或邀请激活。
2. 老师创建组织和项目，并将导师加入组织。
3. 导师申请项目，老师批准。
4. 学生选择组织、项目和至少一个模型申请 Key。
5. 导师初审后，状态从 `pending_mentor` 变为 `pending_teacher`。
6. 老师确保所有模型和 binding ACTIVE 后终审，状态变为 `approved`。
7. 确认学生已自动加入组织和项目。
8. 学生首次领取，取得一次性明文并转为 `active`；重复领取必须失败。
9. 使用 Key 调用 `GET /v1/models` 和一个允许模型；调用白名单外模型必须失败。
10. 导师撤销 Key，后续网关调用必须失败，但学生成员关系保留。

### 2.7 控制台人工验收

1. 分别用学生、导师、老师账号登录，确认默认落点、侧栏权限、手机导航抽屉和退出登录。
2. 学生选择组织后确认停用/无导师项目可见但禁选；切换组织确认会清空后续草稿。
3. 在模型步骤解析一个既有模型，确认采用平台权威卡片；再填写一个不存在模型的完整元数据并提交。
4. 导师在最早优先队列完成通过和带原因驳回；老师检查逐模型路由后终审。
5. 学生观察 SSE 状态变化，领取时验证未复制关闭二次确认、复制失败手动选择及关闭后 secret 清空。
6. 老师验证导师候选搜索分页、组织/项目启停、凭据独立替换、模型 nullable 数值和 Binding 编辑。
7. 缩窄至 375px 宽度并仅使用键盘完成认证、申请、审核和弹层关闭；开启 reduced motion 后确认无位移或缩放动画。

## 3. 常见问题矩阵

| 现象 | 常见根因 | 解决方案 | 排查建议 |
| --- | --- | --- | --- |
| 服务启动时报缺少环境变量 | 未加载 `.env`，或关键 secret 仍为空 | 按 `.env.example` 配置运行环境 | 先核对变量名，不要在日志/工单粘贴变量值 |
| `go` 命令不存在 | 新终端未加载用户 PATH，或 Go 未安装 | 重新打开终端，确认 Go bin 在 PATH | 运行 `go version`；本机约定安装在 `D:\dev\caches\go` |
| Ent 生成结果与 Schema 不一致 | 修改 Schema 后未运行生成器 | 运行 Ent generate 并提交生成代码 | 运行 `go generate ./internal/infrastructure/db/ent`，再检查 Git diff |
| 数据库提示 `type citext does not exist` | PostgreSQL 未启用 `citext` extension | 对正确数据库执行迁移命令 | 先确认 DSN，再运行 `go run ./cmd/migrate` |
| 数据库连接失败 | DSN、网络、账号、TLS 参数错误 | 修正 `DATABASE_URL` 或启动 PostgreSQL | 检查 5432 端口、数据库名和 `sslmode`；不要输出密码 |
| Redis 连接失败 | Redis 未启动、URL 错误或 ACL 不允许 | 启动 Redis或修正 `REDIS_URL` | 检查 6379 端口；确认 DB index 和 TLS scheme |
| 验证码一直收不到 | SMTP 配置错误、冷却中、垃圾邮件拦截 | 修正 SMTP，等待冷却后重发 | 用 request ID 查邮件发送错误；不要索取验证码 |
| 验证码提示无效或锁定 | code 过期、用途不一致、失败次数超过上限 | 重新发送对应用途验证码，等待锁定过期 | 核对 email 规范化结果、purpose、TTL 和 Redis key TTL |
| 注册提示邮箱已存在 | `users.email` 使用 citext 唯一，大小写视为相同 | 使用登录/重置密码，不要重复注册 | 用脱敏邮箱查询用户状态；检查是否 `disabled` |
| 登录一直 401 | 密码错误、账户禁用、JWT 配置变化 | 重置密码或由老师恢复账户；重新登录 | 不区分“邮箱不存在”和“密码错误”；检查服务端 request ID |
| Refresh 突然全部失效 | token 被轮换后重用，session family 被撤销 | 清除 Cookie 后重新登录 | 检查是否多标签页并发 refresh；确认 Redis 未被清空 |
| 浏览器没有 refresh Cookie | 非 HTTPS 环境却设置 Secure，或跨站 Cookie 被阻止 | 本地环境关闭 Secure；生产使用 HTTPS 同源 | 检查 Set-Cookie、SameSite、Path 和浏览器存储策略 |
| SSE 连接 401 | 浏览器 EventSource 无法附加 Authorization Header | 使用 fetch streaming 并携带 Bearer token | 检查请求 Header；禁止把 token 放在 URL |
| SSE 已连接但状态不更新 | Redis stream 不可用、事件过期或客户端只监听未刷新 REST | 恢复 Redis并重新拉取权威 REST 状态 | 查看 heartbeat、Last-Event-ID 和用户 stream；事件不是权威数据 |
| 学生看得到项目但提交失败 | 项目没有 ACTIVE 负责导师 | 导师申请该项目并由老师批准 | 查看项目 `has_mentor` 与 ACTIVE project member |
| 申请提示 Key 名称冲突 | 同一学生已有同名进行中/生效 Key，citext 不区分大小写 | 更换名称，或等待原申请进入 rejected/revoked | 查询自己的 Key 列表，核对状态和大小写 |
| 自定义模型提交后无法终审 | 新模型处于 `pending_configuration` | 老师补全模型、至少一个 ACTIVE binding 和 ACTIVE provider | 终审返回的 `model_ids` 是未就绪集合 |
| 导师看不到待审 Key | 导师不是目标项目的 ACTIVE 负责成员，或已被其他导师处理 | 完成项目申请，或刷新状态 | 查项目成员关系和 Key 当前状态；首个有效初审胜出 |
| 导师/老师重复审批得到 409 | 条件更新阻止并发重复状态迁移 | 刷新详情，不要重复提交 | 检查审计时间线和当前状态，不要直接改数据库 |
| 老师看不到 ACTIVE Key | 这是首期权限边界，不是数据丢失 | 由负责导师查看和撤销 | 老师接口只暴露 `pending_teacher` 摘要 |
| 终审后学生未加入组织/项目 | 终审事务回滚或数据库约束失败 | 修复根因后重新执行合法终审；禁止手工只改 Key 状态 | 同时检查 Key、两张 member 表和 audit；四者必须原子一致 |
| 领取接口没有再次返回完整 Key | 明文只在首次领取返回一次 | 创建新申请，不支持找回 | 只能使用 prefix 定位；任何人都不应从数据库恢复明文 |
| 领取时报 `KEY_ALREADY_CLAIMED` | Key 已是 ACTIVE 或并发请求已有一个成功 | 使用首次响应中的明文 | 检查 `claimed_at` 和 prefix，不要要求后台重显 |
| 网关返回 invalid API key | Key 不存在、HMAC pepper 变化、状态非 ACTIVE | 使用正确 Key；恢复原 pepper 或重新申请 | 只记录 prefix；检查部署间 `API_KEY_HASH_PEPPER` 是否一致 |
| 网关返回 model_not_allowed | 请求模型不在该 Key 的规范化白名单 | 使用 `GET /v1/models` 返回的模型 | 核对 `api_key_models`，不要依赖客户端缓存 |
| 网关返回 model unavailable | 模型、binding 或 provider 被停用 | 老师启用完整路由链或添加 binding | 按 model -> binding -> provider 顺序排查状态和 priority |
| 上游 429/5xx 后没有切换 | 没有下一个 ACTIVE binding，或流已经开始写出 | 添加低优先级候选；流开始后只能返回当前错误 | 查看候选排序、adapter、priority 和首字节是否已发送 |
| multipart 请求失败 | Content-Type boundary 丢失、文件过大或客户端重复读取 body | 保留原始 Content-Type，调整允许大小 | 检查请求大小和 boundary；不要把文件内容写入日志 |
| Anthropic Messages 返回鉴权错误 | 缺少 `x-api-key` 或 `anthropic-version` | 使用协议原生 Header | 确认网关覆盖上游 key，不透传客户端 key |
| Realtime WebSocket 连接失败 | token 放在 query、模型不允许、subprotocol 不匹配 | 使用 Authorization 或约定 subprotocol | 检查 Upgrade 响应、模型 query 和白名单；禁止记录 token |
| 供应商凭据配置后仍不可用 | 加密主密钥不一致或密文损坏 | 恢复原主密钥，无法恢复时重新录入凭据 | 不输出密文/明文；检查 AEAD 解密错误和 key 长度 |
| 修改常用模型后学生页面不刷新 | 全局 SSE 事件丢失或客户端未重新请求模型广场 | 收到事件后重新 GET 模型广场 | 检查 global stream revision；不要把 SSE payload 当完整数据 |
| API 返回 404 但资源存在 | 为防越权，资源不可见与不存在统一返回 404 | 使用正确角色和资源范围 | 检查 JWT user ID、角色、组织/项目成员关系 |
| API 返回未知字段错误 | 控制面拒绝未声明 JSON 字段 | 按 API 文档删除错误字段 | 核对字段拼写和 DTO 版本，不要关闭严格解码 |
| Doppler 启动提示 config 不存在 | config 名拼写错误或当前账号无权限 | 使用真实配置 `nebula-api/dev_personal` 并重新登录 | 运行 `doppler configs --project nebula-api`；只共享 config 名，不粘贴变量值 |
| Doppler 启动提示 placeholder values | `dev_personal` 中仍保存 `.env.example` 占位符或必填值为空 | 在 Doppler 中替换提示的变量；不要写入仓库或聊天 | 只核对变量名与占位状态，不输出真实值；修复后重新运行启动脚本 |
| Doppler 启动提示密钥长度/格式不合格 | JWT/HMAC pepper 少于 32 UTF-8 bytes，或供应商主密钥不是恰好 32 bytes 的 Base64 | 重新生成满足契约的随机值并保存到 Doppler | 只输出长度/格式布尔结果，不输出密钥；三个 JWT/HMAC 值均至少 32 bytes，供应商 key 解码后必须恰好 32 bytes |
| Compose 提示必须通过脚本运行 | 未设置 `NEBULA_VERSION` 或未完成 DSN 安全改写 | 使用 `.\scripts\compose.ps1 ...` 作为唯一入口 | 检查根目录 `VERSION` 是否为 `X.X.X`，不要手工复制 secret 到 shell |
| 启动脚本拒绝 DATABASE_URL | Doppler DSN 指向外部 host、缺少账号密码或数据库名 | 将 `dev_personal` 配置为本地 loopback DSN，或明确确认后调整本地配置 | 只检查 host 类别和字段是否存在，不输出完整 DSN |
| `postgres` 一直 unhealthy | 命名卷中的初始化账号与当前 Doppler DSN 不一致，或卷损坏 | 恢复创建该卷时的 Doppler 数据库配置；需要清空卷时先获得明确授权 | `.\scripts\compose.ps1 logs --tail 100 postgres`；禁止在工单粘贴密码 |
| `redis` 一直 unhealthy | Redis 启动失败、AOF 损坏或 Docker 磁盘异常 | 根据 Redis 日志修复；不要擅自删除数据卷 | `.\scripts\compose.ps1 logs --tail 100 redis` 和 `docker system df` |
| `migrate` exited (1) | PostgreSQL 未就绪、凭据不匹配、`citext`/Ent schema 创建失败 | 修正本地 DSN或数据库权限后重建一次性 service | `.\scripts\compose.ps1 logs --tail 100 migrate`；确认目标是 Docker 本地卷 |
| `backend` 不启动 | migrate 未成功、Redis 不健康或 Doppler 必填配置错误 | 先恢复依赖，再重新 `up -d` backend | 按 postgres -> redis -> migrate -> backend 顺序查看 `ps` 和日志 |
| `/health/live` 成功但 `/health/ready` 503 | 进程存活但 PostgreSQL 或 Redis ping 失败 | 修复显示为 unavailable 的依赖 | 响应只给依赖类别；进一步查看对应容器日志 |
| 公网 API 返回 Cloudflare `530`，浏览器同时报告 CORS 缺少 `Access-Control-Allow-Origin` | Cloudflare Tunnel 没有可用的 edge connection；CORS 报错是上游不可达后的二次表现，不是控制面 CORS 配置本身 | 生产 Compose 的 `cloudflared` 固定使用 `--protocol http2`，重新部署并确认 Tunnel 日志出现 Registered tunnel connection | 先请求 `https://api.lyn91r.cn/health/live`；再检查 `docker compose -p nebula-api -f compose.production.yaml logs --tail 100 cloudflared`，确认没有 QUIC timeout 且公网 health 返回 200 |
| 8080 端口被占用 | 本机其他程序已监听 `127.0.0.1:8080` | 停止冲突程序；端口契约变更需同步 Compose 和文档 | `Get-NetTCPConnection -LocalPort 8080 -State Listen` |
| 修改代码后仍运行旧行为 | 未重建镜像或 `VERSION`/image tag 与预期不一致 | 执行 `.\scripts\compose.ps1 up -d --build`，发布时按 SemVer 更新 `VERSION` | `.\scripts\compose.ps1 images` 与 `docker image inspect nebula-api-backend:<version>` |
| main push 后没有部署 | workflow 未进入 `production` environment，或 GitHub 缺少 `DOPPLER_TOKEN` | 检查 Actions 运行记录和 environment secret；不要把 token 改写到 workflow | `gh run list --workflow deploy.yml` 和 `gh secret list --env production`，只核对名称 |
| GitHub runner 获取 Doppler 部署 SSH 配置超时 | Doppler API 到 runner 的网络波动 | workflow 使用 Runner 自带 `curl` 调用按名 REST 接口读取五个 `DEPLOY_SSH_*`，并对同一请求最多重试五次；仍失败时检查 Doppler 服务状态和 `DOPPLER_TOKEN` 的访问权限 | 查看 `Sync source and deploy` 日志；不安装 Doppler CLI，也不要将 `DEPLOY_SSH_*` 复制到 GitHub secret |
| 部署在 SSH 校验阶段失败 | Doppler 主机信息已更新但 `DEPLOY_SSH_KNOWN_HOSTS` 仍是旧主机公钥，或密码认证被禁用 | 在可信通道核对新服务器公钥并同时更新五个 `DEPLOY_SSH_*` 变量 | 不得关闭 `StrictHostKeyChecking`；不要在日志打印密码或完整 Doppler 配置 |
| 远端镜像未更新 | 源码同步失败、服务器本地构建失败、磁盘不足或 Docker 服务异常 | 修正 SSH/Docker/构建网络问题后重新 push 新 commit | 检查 workflow 的 `Sync source and deploy` 日志、`docker image inspect nebula-api-backend:<version>`、`df -h /opt` 和 `docker system df`；不得删除无关项目镜像或卷 |
| migrate 成功但应用健康检查超时 | backend 配置错误、依赖未健康、端口冲突或 frontend 未启动 | 按 postgres -> redis -> migrate -> backend -> frontend 顺序定位 | `cd /opt/nebula-api && docker compose -p nebula-api ps`；日志中不得输出 DSN 或 secret |
| 云服务器 8080/8081 从公网不可达 | Compose 按安全契约只绑定 loopback | 使用 SSH tunnel；配置域名、TLS 和反向代理应作为独立部署变更 | 服务器执行 `ss -lntp '( sport = :8080 or sport = :8081 )'`，应只看到 `127.0.0.1` |
| 前端打开后白屏 | 前端生产构建失败、静态资源未进入镜像或路由脚本异常 | 先完成 typecheck/build，再重建 frontend 镜像 | `cd frontend; npm run typecheck; npm run build`，浏览器查看 Console 与 Network，不粘贴 token |
| 老师模型管理的新增模型弹窗中 checkbox 与文字错位或纵向间距异常 | 全局普通输入框样式覆盖了 checkbox 的宽度、显示方式和上边距 | 使用模型表单专用 checkbox 选项布局，并重新部署前端 | 打开老师 `模型管理 -> 新增模型`，确认输入/输出模态及“全局常用模型”的 checkbox 与文字同一行、间距一致 |
| 直接打开或刷新 `/login`、`/teacher/...` 返回 Vercel `404: NOT_FOUND` | Vercel 未读取 `frontend/vercel.json`，通常是 Root Directory 不是 `frontend` 或新配置尚未部署 | 将 Vercel Root Directory 设置为 `frontend` 并重新部署当前版本 | 直接请求 `/login` 和 `/teacher/key-reviews`，确认 HTTP 200 且响应为 `index.html`；不要改用 hash 路由或增加后端旧路由 |
| 页面刷新后回到登录页 | refresh Cookie 不存在、Path/Secure/SameSite 不匹配，或 Redis session 已失效 | 修正 Cookie 与同源部署配置后重新登录 | 检查 `/api/v1/auth/refresh` 状态和 Set-Cookie 属性；前端不会从 JS 读取 refresh token |
| 多个请求同时 401 导致反复刷新 | API client 未共享 refresh 请求，或 refresh token 被并发轮换重用 | 使用前端内置 single-flight client，禁止页面直接调用 fetch | Network 中应只有一个并发 refresh；若 session family 被撤销则重新登录 |
| 前端提示接口 404 | 未通过 Vite/Nginx 代理访问、路径写成旧路由或 backend 未启动 | 使用相对 `/api/v1` 路径并恢复 backend | 检查浏览器请求 URL、`frontend/nginx.conf` 和 Compose 网络，不增加旧路由别名 |
| 前端 SSE 无状态更新 | 使用 EventSource 无法带 Bearer，或 fetch stream 中断 | 使用内置 fetch streaming，重连后重新拉取列表 | 检查 `/api/v1/events` Authorization 与响应 Content-Type；不要把 token 放 query |
| 领取弹窗关闭后找不到完整 Key | 明文按契约只在首次响应与当前内存弹窗出现 | 使用首次领取时复制的值；遗失后只能重新申请 | 数据库和浏览器存储都不应存在明文，禁止增加“再次显示”功能 |
| 8081 端口被占用 | 本机其他进程占用前端入口 | 停止冲突进程或按正式变更流程修改端口和文档 | `Get-NetTCPConnection -LocalPort 8081 -State Listen` |
| `down` 后数据仍存在 | 命名卷按设计持久化 | 重新启动即可恢复；不要把持久化误判为清理失败 | `docker volume ls --filter name=nebula-api_`；`down -v` 会永久删除数据 |

## 4. 分层排查顺序

1. 使用 response Header 或 envelope 中的 `request_id` 关联日志。
2. 检查请求方法、标准路由、Content-Type 和认证 Header。
3. 检查当前用户状态、角色和资源归属。
4. 检查 PostgreSQL 权威状态及合法状态迁移。
5. 检查 Redis 中的短期状态、TTL 和 stream。
6. 检查供应商/模型/binding 三层启用状态。
7. 最后检查上游连接、超时、429/5xx 和协议响应。

禁止通过关闭权限校验、吞掉错误、手工修改单张表、返回完整 secret 或启用旧路由来“临时解决”问题。

## 5. 新功能同步模板

每个新增功能至少在本文件补充以下内容：

```markdown
### 功能名称

- 测试范围：
- 正常路径：
- 权限/安全边界：
- 并发或事务边界：
- 常见现象：
- 根因：
- 解决方案：
- 排查命令或观察点：
- 不得记录的敏感信息：
```
