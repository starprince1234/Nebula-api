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

- 模型倍率解析、项目/Key 月度 credit 聚合、调用日志与输入监控 DTO；新增 PostgreSQL migration、额度门禁、失败返还和权限集成测试应在发布前执行。

### 限额、调用日志与输入监控

- 学生 `/student/usage`、导师项目用量、老师项目花费必须通过真实 HTTP JSON envelope 验证 snake_case 字段，禁止只构造前端对象。Go usage DTO 缺少 `json` tag 时，接口会返回 `ID/Quota/Models`，前端按 `id/quota/models` 读取后表现为空白或 `undefined.map`。
- 模型广场必须展示 `credit_multiplier`；老师模型配置在倍率变化时必须要求 `multiplier_change_reason`。Key 申请、导师初审、老师终审分别验证三阶段额度字段。
- 老师终审成功路径必须验证项目月 bucket 更新同时绑定 `project_id` 与当前上海月份；SQL 占位符与实参数量不一致时，pgx 会在事务内拒绝执行并完整回滚。前端遇到终审依赖错误时必须关闭确认弹窗，明确说明额度和申请状态未变更，并展示响应 `request_id` 供管理员排查。
- 调用日志无数据时先区分真实空集和渲染失败：检查 `/mentor/call-logs` 的 `items`，再通过 fake upstream 发一笔测试调用，确认 `gateway_calls`、`monthly_usage_cube` 和适用协议的 `monitored_inputs` 同时更新。
- 生产 Playwright E2E 仅使用专用测试身份与测试 Key；凭据不得写入源码、报告或截图。额度/倍率写操作只针对专用测试资源，并保留永久审计。
- 导师项目详情的视觉验收必须检查：两张环图都有可见颜色图例；额度分配按成员的全部 Key 额度之和分段；成员明细采用横向卡片，每成员一张，左侧为成员身份，中间纵向排列多个 Key 的额度轨道/已用填充/右侧百分比，右侧显示该成员的 0x 免费模型请求次数表并保留 Key 状态；逐模型验证所有成员 `free_models.calls` 之和等于项目顶层 `free_models.calls`；窄屏应依次降为两列和单列。老师项目花费必须是按项目分行的表格，行内提供 mini-donut，展开后显示按模型拆分且带可见图例的大图。学生个人用量的每个 Key 环图也必须有可见模型/剩余额度图例。
- 所有 `CreditDonut` 环图的 tooltip 必须限制在各自图表视区内；将鼠标移至左右边缘扇区时，文本不得落到图表容器外而被父级裁切。该规则同时覆盖导师项目额度分配/实际开销、老师项目花费和学生个人用量图。
- 导师调用日志和输入监控的资源筛选必须为同一套级联下拉：项目仅列负责项目；选择项目后成员、Key、模型按项目收窄；选择成员后 Key、模型继续收窄；选择 Key 后模型按白名单收窄；修改上级必须清空全部下级；任一下级为空表示当前上级范围内全部。关键词和日期时间不是枚举，分别保留文本框与日期时间控件。验证只有 0x 调用的已撤销 Key 仍可作为当月筛选项。

- 测试范围：模型倍率定点解析、上海时区月桶、Key/项目额度门禁、供应商故障切换、调用状态结算、日志分页、导师项目范围和提示词访问审计。
- 正常路径：请求解析模型后创建 pending call，成功结算 charged 与月度聚合；提示词仅在 charged/outcome_unknown 后可见。
- 权限/安全边界：老师只看项目按模型聚合，导师只看当前负责项目的调用日志和输入监控；完整提示词读取必须先成功写访问审计。
- 并发或事务边界：项目月桶和 Key 月桶必须在固定锁序下条件更新；重复 finalize/recovery 不得重复计费；撤销与 reserve 使用同一事务规则。
- 常见现象：额度耗尽返回 `429`；日志出现 `outcome_unknown`；输入监控列表无全文。
- 根因：Key/项目月桶已达到额度、客户端在上游发送后中断、或输入文本仍需点击详情读取。
- 解决方案：等待北京时间下月刷新或使用 0 倍率模型；按 request ID 检查调用尝试；在导师项目范围内打开详情。
- 排查命令或观察点：`go test ./...`、`go vet ./...`、`cd frontend; npm run typecheck`；查看 `gateway_calls`、`gateway_call_attempts` 的脱敏状态和 `monthly_usage_cube` 聚合。
- 不得记录的敏感信息：完整 API Key、供应商凭据、数据库 DSN、refresh token、JWT、提示词原文和上游原始响应 body。

- API Key 合法/非法状态迁移及驳回、撤销原因规则；
- bcrypt、JWT audience/issuer、API Key HMAC、AES-GCM 防篡改；
- 标准网关路由与旧路由拒绝；
- 控制面完整路由注册，以及不存在 DELETE 和旧网关前缀；
- 供应商 URL 拼接、公开模型 ID 到上游模型名重写；
- JSON/multipart body 重放、文件内容保真和请求大小限制；
- Adapter 协议隔离，以及 Images/Audio/Videos/Realtime/Moderations 的 route-to-adapter 映射；
- Codex Responses HTTP/compact/WebSocket 的未知字段、加密 compaction item、prompt cache、service tier、`OpenAI-Beta` 与 model 改写保真；
- Realtime client secret 的 `session.model`、WebRTC calls multipart `session.model` 改写和 SDP 保真；
- Video resource 创建后的 Redis binding 关联，以及查询、content、remix 返回同一上游；
- 控制面结构化错误码到 HTTP 状态码映射；
- 安全配置长度、加密主密钥、开发 Cookie 行为，以及固定测试账号 Secret 的成套解析和角色校验。
- 前端类型检查、生产构建、关键 API client 行为，以及初次加载/刷新状态分离、并发请求计数、相同操作去重和可访问骨架渲染。
- 生产 Dockerfile 的 BuildKit 契约：部署显式启用 BuildKit，`go mod download` 使用持久化 module cache，三个 Go 二进制的串行编译同时使用 module cache 和 build cache。
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

生产测试账号凭据保存在 Doppler `nebula-api/prd`，每个角色各三组：`TEST_STUDENT_1..3_{NAME,EMAIL,PASSWORD}` 和 `TEST_MENTOR_1..3_{NAME,EMAIL,PASSWORD}`。生产部署要求六组三元组全部存在，并将其作为 backend 运行时环境注入；服务启动时按邮箱幂等创建缺失用户。这些值不写入仓库或 backend 日志。已有同邮箱、同角色用户不会更新密码或状态；同邮箱角色冲突会导致 backend 启动失败。

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

本地 Compose 期望：`postgres`、`redis`、`backend` 为 healthy，`frontend` 为 running，`migrate` 为 exited (0)。生产 Compose 使用 `compose.production.yaml`，期望 `postgres`、`redis`、`backend`、`maintenance`、`model-catalog`、`cloudflared` 运行，`migrate` exited (0)，且不运行 frontend。健康响应不得包含 DSN、secret 或异常文本。

### 2.5 生产部署验证

push 到 `main` 后，在 GitHub Actions 的 `Deploy production` workflow 中确认源码已同步到服务器、第三方镜像依次拉取、单核 Go 构建、分阶段 service 启动、显式 migrate 和两个 HTTP 健康检查均成功。服务器端只读检查：

```bash
cd /opt/nebula-api
docker compose --project-name nebula-api --file compose.production.yaml ps
curl --fail http://127.0.0.1:8080/health/ready
curl --fail https://api.lyn91r.cn/health/ready
```

期望 `/opt/nebula-api` 保存当前部署源码，`postgres`、`redis`、`backend`、`maintenance`、`cloudflared` 正在运行，`migrate` 成功退出，内部和公网 readiness 均返回 200。GitHub 日志不得出现 SSH 密码、Doppler token、DSN 或应用 secret。公网访问通过 Cloudflare Tunnel 的 `https://api.lyn91r.cn`，production 登录和 API Key 传输必须经过 HTTPS。

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

1. 分别用学生、导师、老师账号登录，确认默认落点、侧栏权限、手机导航抽屉和退出登录；导师从“审核密钥”点击“项目管理”后必须保留完整工作台并显示项目列表或明确空状态。
2. 学生选择组织后确认停用/无导师项目可见但禁选；切换组织确认会清空后续草稿。
3. 在模型步骤解析一个既有模型，确认采用平台权威卡片；再填写一个不存在模型的完整元数据并提交。
4. 导师在最早优先队列完成通过和带原因驳回；老师检查逐模型路由后终审。
5. 学生观察 SSE 状态变化：只有已有 Key 的状态真正变化时自动打开该 Key 详情；首次进入、普通刷新、SSE 重连和新提交不会自动弹窗。领取时验证未复制关闭二次确认、复制失败手动选择及关闭后 secret 清空。
6. 学生模型广场点击“申请新模型贴士”，确认提示申请应在申请密钥阶段添加，审批通过后自动更新模型广场；申请密钥阶段直接填写新模型卡片，不再先调用模型 ID 检查接口。
6. 老师验证导师候选搜索分页、组织/项目启停、凭据独立替换、模型 nullable 数值和 Binding 编辑。
7. 缩窄至 375px 宽度并仅使用键盘完成认证、申请、审核和弹层关闭；开启 reduced motion 后确认无位移、缩放、旋转或 shimmer 动画，但加载文字、禁用态和错误信息仍然可见。
8. 对每个角色依次打开所有侧栏页面：GET 期间应显示顶部加载进度和数据骨架；使用局部 `LoadingRegion` 的复杂列表在初次加载时不得先闪现误导性的空状态，刷新时继续保留已有内容；登录、注册、退出及使用局部 pending 的写操作按钮应显示处理中并禁止重复提交。

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
| Doppler 中学生/导师测试账号仍登录 401 | 同邮箱用户在引入 bootstrap 前已经存在，启动初始化按幂等契约不会覆盖其密码或状态 | 使用现有密码登录，或通过正式重置密码流程同步到目标密码；不要直接改 `password_hash` | 只核对用户是否存在、角色和状态，不输出邮箱全文或密码；新邮箱应在首次部署后创建为 `active` |
| Refresh 突然全部失效 | token 被轮换后重用，session family 被撤销 | 清除 Cookie 后重新登录 | 检查是否多标签页并发 refresh；确认 Redis 未被清空 |
| 浏览器没有 refresh Cookie | 非 HTTPS 环境却设置 Secure，或跨站 Cookie 被阻止 | 本地环境关闭 Secure；生产使用 HTTPS 同源 | 检查 Set-Cookie、SameSite、Path 和浏览器存储策略 |
| SSE 连接 401 | 浏览器 EventSource 无法附加 Authorization Header | 使用 fetch streaming 并携带 Bearer token | 检查请求 Header；禁止把 token 放在 URL |
| SSE 已连接但状态不更新 | Redis stream 不可用、事件过期或客户端只监听未刷新 REST | 恢复 Redis并重新拉取权威 REST 状态 | 查看 heartbeat、Last-Event-ID 和用户 stream；事件不是权威数据 |
| 学生看得到项目但提交失败 | 项目没有 ACTIVE 负责导师 | 导师申请该项目并由老师批准 | 查看项目 `has_mentor` 与 ACTIVE project member |
| 申请提示 Key 名称冲突 | 同一学生已有同名进行中/生效 Key，citext 不区分大小写 | 更换名称，或等待原申请进入 rejected/revoked | 查询自己的 Key 列表，核对状态和大小写 |
| 自定义模型提交后无法终审 | 新模型处于 `pending_configuration` | 老师补全模型、至少一个 ACTIVE binding 和 ACTIVE provider | 终审返回的 `model_ids` 是未就绪集合 |
| 老师新增模型搜不到 Matrix 候选 | `model-catalog` 未启动、`MATRIX_APIKEY` 缺失或最近同步失败 | 检查生产 worker 状态和日志；失败保留上次快照，不清表、不把目录暴露给学生 | `docker compose -p nebula-api -f compose.production.yaml ps model-catalog`；日志不得包含 API Key |
| 一键配置拒绝测试地址 | 手写 Base URL 解析到 localhost、私网、链路本地或云元数据地址 | 使用公网 HTTP/HTTPS 上游，或先注册经审核的供应商 | 不得关闭连接阶段 DNS/IP 校验或允许重定向绕过 |
| 学生 `0x` 模型显示 0 次而老师有记录 | 学生查询遗漏 `zero_cost_count` | 学生调用数使用 `charged_count + zero_cost_count`，0 credits 行仍返回 | `go test ./internal/usage`；对照学生个人用量与老师/导师免费模型汇总 |
| 导师看不到待审 Key | 导师不是目标项目的 ACTIVE 负责成员，或已被其他导师处理 | 完成项目申请，或刷新状态 | 查项目成员关系和 Key 当前状态；首个有效初审胜出 |
| 导师/老师重复审批得到 409 | 条件更新阻止并发重复状态迁移 | 刷新详情，不要重复提交 | 检查审计时间线和当前状态，不要直接改数据库 |
| 老师终审返回 `503 DEPENDENCY_UNAVAILABLE`，页面提示终审未完成 | 终审事务的数据库操作失败；曾出现项目月 bucket 更新 SQL 使用 `$1..$4` 但只传 3 个参数，pgx 报 `mismatched param and argument count` | 保持终审事务向 bucket 更新完整传入额度、时间、项目 ID 和月份；失败时事务必须回滚，修复后重新终审 | 从响应复制 `request_id`，在 backend 日志中按该 ID 定位真实错误；运行 `go test ./internal/usage -run TestApproveKeyBindsProjectBucketMonth` 和 `cd frontend; npm run test -- src/api/client.test.ts` |
| 老师驳回申请返回原因缺失 | 驳回请求没有填写非空审核意见；通过请求的审核意见可以省略 | 在驳回弹窗填写具体原因；通过时可以留空意见 | 期望错误码为 `REJECTION_REASON_REQUIRED`；通过请求空 body 应返回 `204` |
| 老师审批导师项目申请提示导师关系异常 | 审批时发现导师未加入项目所属组织，或导师已经是项目成员；空 body 则可能是请求体解析错误 | `MENTOR_NOT_IN_ORGANIZATION`：在“组织管理”重新分配导师；`MENTOR_ALREADY_PROJECT_MEMBER`：无需重复审批并刷新列表 | 查看响应 `error.code`；核对 `organization_members`、`project_members` 和申请状态 |
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
| 网关返回 `model_not_allowed`，但模型已在 Key 白名单 | 模型没有与当前入口协议匹配的 ACTIVE adapter Binding | 按入口创建 Responses、Embeddings、Images、Audio、Videos、Realtime、Moderations、Rerank 等独立 Binding | 不要用 `openai_compatible` 代替其他协议；检查 Binding、provider 和 model 状态 |
| Gemini `v1beta` 返回 400/401 | 使用了禁止的 `?key=`、缺少 `x-goog-api-key`/Bearer，或没有 `google_gemini_v1beta` Binding | 使用 Header 携带 Nebula Key，并为模型创建 Gemini Binding | 核对路径 `/v1beta/models/{public_model}:operation`、adapter 和供应商 Base URL；不要记录 key |
| Gemini batch embeddings 上游提示 model 不匹配 | Binding 上游模型名错误，或批量请求内的 model 不是合法 Gemini embedding 模型 | 修正 Binding 的上游模型名 | 网关会统一改写路径和 `requests[*].model`；检查上游返回但不要输出供应商凭据 |
| Realtime WebSocket 连接失败 | token 放在 query、模型不允许、subprotocol 不匹配 | 使用 Authorization 或约定 subprotocol | 检查 Upgrade 响应、模型 query 和白名单；禁止记录 token |
| Codex CLI 普通请求可用但 compact 或 WebSocket 失败 | 缺少 `/v1/responses/compact`、Responses WebSocket Upgrade 未转发、`OpenAI-Beta` 被删除，或 compact output 被代理裁剪 | 使用 `openai_responses` Binding，确认 HTTP 与 WS 都指向支持 Responses 的上游 | compact 必须保留完整 output/`encrypted_content`；检查 `prompt_cache_key`、`previous_response_id` 与 Upgrade Header，不要打印加密内容 |
| Realtime WebSocket 生成失败 | 连接未使用 `/v1/realtime` WebSocket、模型不在 Key 白名单或额度耗尽 | 使用 WebSocket 并在连接 query 指定允许模型；额度耗尽时等待下月或使用 0 倍率模型 | 检查协议 error event 与 request ID；不再提供 WebRTC client secret/calls 路由 |
| Video 创建成功但查询、下载或 remix 404 | Redis 资源路由过期、Video 创建响应没有标准 `id`，或原 Binding/provider 已停用 | 在 TTL 内使用标准 video ID，恢复对应 Binding/provider；必要时重新创建 Video | 检查 `gateway:resource-route:video:{id}` 是否存在；不要把 video ID 当模型重新路由 |
| Backend 启动时报 `GATEWAY_RESOURCE_ROUTE_TTL_HOURS` 配置错误 | Doppler 仍只保存旧视频任务 TTL 字段或值不是正整数小时 | 在 `nebula-api` 对应 config 中写入 `GATEWAY_RESOURCE_ROUTE_TTL_HOURS=24` 并删除旧字段 | 使用按名配置读取确认新字段存在；不要输出整份 secret 配置 |
| `/v1/files`、`/v1/uploads` 或 `/v1/batches` 返回 404 | 这些是有状态资源 API，当前按模型 Binding 的首期网关未实现 | 不要映射到任意默认模型；如需该能力先设计资源归属、权限和生命周期契约 | NewAPI 参考实现同样把多项 Files 路由标为未实现；透明代理无法解决后续 resource ID 应回到哪个上游的问题 |
| 供应商凭据配置后仍不可用 | 加密主密钥不一致或密文损坏 | 恢复原主密钥，无法恢复时重新录入凭据 | 不输出密文/明文；检查 AEAD 解密错误和 key 长度 |
| 修改常用模型后学生页面不刷新 | 全局 SSE 事件丢失或客户端未重新请求模型广场 | 收到事件后重新 GET 模型广场 | 检查 global stream revision；不要把 SSE payload 当完整数据 |
| API 返回 404 但资源存在 | 为防越权，资源不可见与不存在统一返回 404 | 使用正确角色和资源范围 | 检查 JWT user ID、角色、组织/项目成员关系 |
| API 返回未知字段错误 | 控制面拒绝未声明 JSON 字段 | 按 API 文档删除错误字段 | 核对字段拼写和 DTO 版本，不要关闭严格解码 |
| Doppler 启动提示 config 不存在 | config 名拼写错误或当前账号无权限 | 使用真实配置 `nebula-api/dev_personal` 并重新登录 | 运行 `doppler configs --project nebula-api`；只共享 config 名，不粘贴变量值 |
| Doppler 启动提示 placeholder values | `dev_personal` 中仍保存 `.env.example` 占位符或必填值为空 | 在 Doppler 中替换提示的变量；不要写入仓库或聊天 | 只核对变量名与占位状态，不输出真实值；修复后重新运行启动脚本 |
| Doppler 启动提示密钥长度/格式不合格 | JWT/HMAC pepper 少于 32 UTF-8 bytes，或供应商主密钥不是恰好 32 bytes 的 Base64 | 重新生成满足契约的随机值并保存到 Doppler | 只输出长度/格式布尔结果，不输出密钥；三个 JWT/HMAC 值均至少 32 bytes，供应商 key 解码后必须恰好 32 bytes |
| Compose 提示必须通过脚本运行 | 未设置 `NEBULA_VERSION` 或未完成 DSN 安全改写 | 使用 `.\scripts\compose.ps1 ...` 作为唯一入口 | 检查根目录 `VERSION` 是否为 `X.X.X`，不要手工复制 secret 到 shell |
| 启动脚本拒绝 DATABASE_URL | Doppler DSN 指向外部 host、缺少账号密码或数据库名 | 将 `dev_personal` 配置为本地 loopback DSN，或明确确认后调整本地配置 | 只检查 host 类别和字段是否存在，不输出完整 DSN |
| `postgres` 一直 unhealthy | 命名卷中的初始化账号与当前 Doppler DSN 不一致、卷损坏，或旧宿主机 libseccomp 将 PostgreSQL 新 syscall 错误返回为 `EPERM` | 恢复创建该卷时的 Doppler 数据库配置；生产必须保留版本化 ENOSYS seccomp profile；需要清空卷时先获得明确授权 | 查看 postgres 日志；若出现 `pwritev2`/`Operation not permitted`，运行隔离的 `pg_test_fsync` 验证 profile，禁止使用 `seccomp=unconfined` |
| `redis` 一直 unhealthy | Redis 启动失败、AOF 损坏或 Docker 磁盘异常 | 根据 Redis 日志修复；不要擅自删除数据卷 | `.\scripts\compose.ps1 logs --tail 100 redis` 和 `docker system df` |
| `migrate` exited (1) | PostgreSQL 未就绪、凭据不匹配、`citext`/Ent schema 创建失败 | 修正本地 DSN或数据库权限后重建一次性 service | `.\scripts\compose.ps1 logs --tail 100 migrate`；确认目标是 Docker 本地卷 |
| `backend` 不启动 | migrate 未成功、Redis 不健康或 Doppler 必填配置错误 | 先恢复依赖，再重新 `up -d` backend | 按 postgres -> redis -> migrate -> backend 顺序查看 `ps` 和日志 |
| `maintenance` 反复记录 `last_success_at` 类型错误，或一直处于 `health: starting` | heartbeat SQL 未显式约束 text/timestamptz 参数类型，或 maintenance 继承了只适用于 backend 的 HTTP healthcheck | heartbeat 参数显式使用 `::text`/`::timestamptz`；maintenance service 禁用继承的 HTTP healthcheck并由 Docker running 状态监控 | 运行 `go test ./internal/usage -run TestRunMaintenanceUsesTypedHeartbeatParameters`；查看 maintenance 日志确认不再出现 `SQLSTATE 42804` |
| backend 报测试账号配置或角色冲突 | 某组 `TEST_*_{NAME,EMAIL,PASSWORD}` 未成套填写、格式不合法，或该邮箱已属于另一角色 | 补齐 Doppler 三元组或使用角色正确且唯一的测试邮箱；禁止修改既有用户角色 | 根据错误中的变量前缀或通用角色冲突定位，不打印 Secret；生产必须配置三组学生和三组导师 |
| `/health/live` 成功但 `/health/ready` 503 | 进程存活但 PostgreSQL 或 Redis ping 失败 | 修复显示为 unavailable 的依赖 | 响应只给依赖类别；进一步查看对应容器日志 |
| 公网 API 返回 Cloudflare `530`，Tunnel 控制台显示 `Active replicas: 0 / Down`，浏览器同时报告 CORS 缺少 `Access-Control-Allow-Origin` | backend 正常但 cloudflared 到 Cloudflare edge 的长连接不可达；CORS 报错只是 530 页面没有业务 CORS Header 的二次表现 | 恢复服务器到 Cloudflare edge 的直连网络；服务器等待 edge 注册，GitHub Runner 从公网验证 readiness | 检查 `docker compose -p nebula-api -f compose.production.yaml logs --tail 100 cloudflared` 和 `https://api.lyn91r.cn/health/live`；Tunnel 日志应出现 `Registered tunnel connection`，不得只检查容器 running |
| 8080 端口被占用 | 本机其他程序已监听 `127.0.0.1:8080` | 停止冲突程序；端口契约变更需同步 Compose 和文档 | `Get-NetTCPConnection -LocalPort 8080 -State Listen` |
| 修改代码后仍运行旧行为 | 未重建镜像或 `VERSION`/image tag 与预期不一致 | 执行 `.\scripts\compose.ps1 up -d --build`，发布时按 SemVer 更新 `VERSION` | `.\scripts\compose.ps1 images` 与 `docker image inspect nebula-api-backend:<version>` |
| 2C2G 服务器部署时 CPU、内存或磁盘 I/O 突增 | 首次冷构建、Go 版本或依赖变化导致缓存失效，Go 编译仍需计算资源，或 Compose 同时 pull/start 多个 service | 生产 Dockerfile 固定单核 Go 构建；BuildKit 使用稳定命名的 module/build cache 复用未变化 package；部署脚本以 `COMPOSE_PARALLEL_LIMIT=1` 逐个 pull，并按 postgres -> redis -> migrate -> backend -> maintenance -> cloudflared 串行启动，每阶段间隔十秒 | 运行 `go test ./scripts` 检查缓存契约；Actions 日志应显示 `persistent BuildKit caches` 且同一时间只有一个 pull/build/start 操作；服务器可用 `docker stats --no-stream`、`uptime` 和 `iostat -xz 1 3` 观察，不要并发重跑 workflow |
| 后续发布仍反复下载全部 Go module 或全量编译 | Docker builder 首次运行、BuildKit 未启用、Go 版本/`go.mod`/`go.sum` 变化，或 builder cache 被人工清理 | 保持 `DOCKER_BUILDKIT=1` 和 Dockerfile 中的 `nebula-go-mod`/`nebula-go-build` cache mount；允许首次冷构建完成，后续构建自动复用 | 使用 `docker system df -v` 只读检查 Build Cache；不得在部署脚本中自动执行 `docker builder prune`，磁盘确有压力时必须先确认清理范围并获得授权 |
| main push 后没有部署 | workflow 未进入 `production` environment，或 GitHub 缺少 `DOPPLER_TOKEN` | 检查 Actions 运行记录和 environment secret；不要把 token 改写到 workflow | `gh run list --workflow deploy.yml` 和 `gh secret list --env production`，只核对名称 |
| 部署在读取 Doppler 配置时超时 | GitHub Runner 或云服务器到 Doppler API 的网络波动；旧的 CLI 完整配置下载特别容易受 dynamic secrets 影响 | 两端均使用 `curl` 的按名 REST 查询并对同一请求最多重试五次；生产服务器必须能直连 Doppler HTTPS API | 查看 `Sync source and deploy` 日志并在服务器执行不输出 secret 的 Doppler API 连通性检查；不要将生产配置复制到 GitHub secret |
| 部署在 SSH 校验阶段失败 | Doppler 主机信息已更新但 `DEPLOY_SSH_KNOWN_HOSTS` 仍是旧主机公钥，或密码认证被禁用 | 在可信通道核对新服务器公钥并同时更新五个 `DEPLOY_SSH_*` 变量 | 不得关闭 `StrictHostKeyChecking`；不要在日志打印密码或完整 Doppler 配置 |
| 远端镜像未更新 | 源码同步失败、服务器本地构建失败、磁盘不足或 Docker 服务异常 | 修正 SSH/Docker/构建网络问题后重新 push 新 commit | 检查 workflow 的 `Sync source and deploy` 日志、`docker image inspect nebula-api-backend:<version>`、`df -h /opt` 和 `docker system df`；不得删除无关项目镜像或卷 |
| migrate 成功但应用健康检查超时 | backend 配置错误、依赖未健康、端口冲突或 frontend 未启动 | 按 postgres -> redis -> migrate -> backend -> frontend 顺序定位 | `cd /opt/nebula-api && docker compose -p nebula-api ps`；日志中不得输出 DSN 或 secret |
| 云服务器 8080/8081 从公网不可达 | Compose 按安全契约只绑定 loopback | 使用 SSH tunnel；配置域名、TLS 和反向代理应作为独立部署变更 | 服务器执行 `ss -lntp '( sport = :8080 or sport = :8081 )'`，应只看到 `127.0.0.1` |
| DataGrip 无法连接生产 PostgreSQL | PostgreSQL 未加入专用 `management` 网络导致 Docker 未创建 loopback 端口转发，或 SSH tunnel 配置错误 | 保持 PostgreSQL 同时加入 `data` 和仅该服务使用的 `management` 网络，映射服务器本机 `127.0.0.1:15432`，并在 DataGrip 启用 SSH tunnel；不要连接公网 5432 | 服务器执行 `ss -lntp '( sport = :15432 )'`，应看到 `127.0.0.1:15432`；DataGrip General 使用 `127.0.0.1:15432`，SSH 使用服务器 22 端口 |
| 前端打开后白屏 | 前端生产构建失败、静态资源未进入镜像或路由脚本异常 | 先完成 typecheck/build，再重建 frontend 镜像 | `cd frontend; npm run typecheck; npm run build`，浏览器查看 Console 与 Network，不粘贴 token |
| 页面或表格请求期间没有加载反馈，或骨架跑到页面右上角 | 页面绕过官方 API client、局部初次加载状态被提前标记为完成，或把骨架误放入全局网络层 | 所有控制面请求使用类型化 API client；全局层只允许顶部进度条，页面/表格骨架必须由对应内容区域的 `LoadingRegion` 承载；不要用空列表冒充加载完成 | 浏览器 Network 节流后依次访问所有侧栏页面，确认骨架位于对应列表/表单容器；运行 `cd frontend; npm run test -- src/components/NetworkLoadingLayer.test.ts src/components/LoadingRegion.test.ts src/composables/useLoadState.test.ts` |
| 请求完成后加载动画一直不消失 | 请求计数没有在异常路径 `finally` 中递减，或页面启动了未结束的普通 HTTP 请求 | 保证成功、失败和 `401` 重试都成对维护计数；持续连接必须使用独立 SSE/WebSocket 通道，不能计入普通请求 | 检查 Network 中未结束请求及 `networkActivity` 调用链；运行 API client 回归测试，不通过吞错或强制归零掩盖问题 |
| 快速重复点击造成多次提交 | 写操作没有局部 pending key，按钮只依赖全局提示但仍可点击 | 使用 `LoadingButton` 和 keyed pending action，在 promise 完成前禁用同一操作 | Network 中同一资源操作只能有一个在途请求；运行 `cd frontend; npm run test -- src/composables/usePendingActions.test.ts` |
| 导师从“审核密钥”点击“项目管理”后整个工作台变空 | 导师与老师的项目页面使用了相同 Vue Router `name`，后注册路由移除了先注册的 `/mentor/projects` 记录 | 所有工作区路由使用全局唯一技术名称，中文标题通过 `meta.title` 展示；禁止复用展示文案作为路由名 | 运行 `cd frontend; npm run test -- src/router/index.test.ts`，并在浏览器确认 `/mentor/projects` 的 `route.matched` 非空且项目接口已发出 |
| 老师模型管理的新增模型弹窗中 checkbox 与文字错位或纵向间距异常 | 全局普通输入框样式覆盖了 checkbox 的宽度、显示方式和上边距 | 使用模型表单专用 checkbox 选项布局，并重新部署前端 | 打开老师 `模型管理 -> 新增模型`，确认输入/输出模态及“全局常用模型”的 checkbox 与文字同一行、间距一致 |
| 老师配置 Binding 后模型卡片仍显示“待配置”且同时显示“路由就绪”小泡泡 | 卡片直接渲染数据库 `status`，又重复渲染派生的 `route_ready`；`pending_configuration + route_ready=true` 的展示语义没有统一 | 卡片将该组合展示为“已就绪”，移除“路由就绪/未就绪”小泡泡；数据库状态仍由模型启用/停用契约控制 | 配置 ACTIVE Binding 和 ACTIVE provider 后刷新老师模型列表，确认卡片只有“已就绪”状态徽标；运行 `cd frontend; npm run test -- src/utils/models.test.ts` |
| 直接打开或刷新 `/login`、`/teacher/...` 返回 Vercel `404: NOT_FOUND` | Vercel 未读取 `frontend/vercel.json`，通常是 Root Directory 不是 `frontend` 或新配置尚未部署 | 将 Vercel Root Directory 设置为 `frontend` 并重新部署当前版本 | 直接请求 `/login` 和 `/teacher/key-reviews`，确认 HTTP 200 且响应为 `index.html`；不要改用 hash 路由或增加后端旧路由 |
| 页面刷新后回到登录页 | refresh Cookie 不存在、Path/Secure/SameSite 不匹配，或 Redis session 已失效 | 修正 Cookie 与同源部署配置后重新登录 | 检查 `/api/v1/auth/refresh` 状态和 Set-Cookie 属性；前端不会从 JS 读取 refresh token |
| 登录后刷新页面被重定向到登录页，且 refresh Cookie 原本有效 | 路由守卫与 `App.vue` 同时调用 bootstrap，单次轮换 refresh token 被并发使用，后端将第二次请求识别为重用并撤销 session family | 由 auth store 提供 single-flight bootstrap，并让路由守卫复用同一个 Promise；不要在多个启动入口重复调用 refresh | 运行 `cd frontend; npm run test -- src/store/auth.test.ts`，浏览器 Network 中刷新页面应只有一个 `/api/v1/auth/refresh` 请求 |
| 多个请求同时 401 导致反复刷新 | API client 未共享 refresh 请求，或 refresh token 被并发轮换重用 | 使用前端内置 single-flight client，禁止页面直接调用 fetch | Network 中应只有一个并发 refresh；若 session family 被撤销则重新登录 |
| 前端提示接口 404 | 未通过 Vite/Nginx 代理访问、路径写成旧路由或 backend 未启动 | 使用相对 `/api/v1` 路径并恢复 backend | 检查浏览器请求 URL、`frontend/nginx.conf` 和 Compose 网络，不增加旧路由别名 |
| 注册成功收到验证码但提交返回 `VALIDATION_ERROR` 且提示未知字段 | 注册页面把 `role`、密码确认字段等 UI 状态一并发送；后端注册 DTO 只接受 `name`、`email`、`password`、`verification_code` 并严格拒绝未知 JSON 字段 | API client 现在显式构造四个允许字段；更新前端构建后重新注册 | 浏览器 Network 查看请求 JSON 的字段名，不要提交 `role`、`confirm` 或其他表单状态；错误提示会指出具体字段 |
| 前端错误提示出现 `body: 包含不支持字段` 或英文原始错误 | 客户端直接展示 Gin/JSON 解析器原文，未按错误码和字段原因翻译 | 使用统一 API error 文案映射；校验错误显示具体字段和可执行处理方式 | 检查响应 `error.code`、`error.details` 和 `request_id`；不要把完整请求头或 token 粘贴到工单 |
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
