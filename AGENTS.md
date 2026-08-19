# Nebula 项目级 Agent 研发约束

## 1. 项目目标

Nebula 是从零重构的现代 AI API 中转站。首期只实现三类用户、组织/项目、模型与供应商、API Key 双级审批、一次性领取、状态事件和标准模型网关。

只读参考项目为 `D:\VScodeProjects\NebulaCloud\nebula-ai`。参考项目用于核对真实业务和协议行为，禁止修改、复制或直接兼容其代码、旧表、旧路由和历史技术债。

首期明确排除计费、余额、额度、RPM、Token 限制、用量记录、通知中心、模型价格、智能路由、软删除、旧路由别名和 internal usage/monitor API。

## 2. 权威文档与强制同步

以下四份文档与代码共同构成项目事实来源：

- `README.md`：实际能力、技术栈、目录结构、启动和验证命令。
- `docs/DATABASE.md`：数据库表、字段、索引、枚举、关系和事务规则。
- `docs/API_DOCUMENTATION.md`：HTTP/SSE/WebSocket 路由、鉴权、DTO、响应和错误码。
- `docs/TESTING_AND_TROUBLESHOOTING.md`：测试策略、常见问题、问题根源、解决方案和排查步骤。

每个 Task 开始前必须：

1. 阅读与任务相关的源码、测试和上述四份文档。
2. 涉及迁移功能时，检查只读参考项目的真实 Migration、Model、Controller、Schema、Service 和路由，不得只参考其 Markdown。
3. 检查 Git 状态，保留用户已有修改。

每个 Task 结束前必须：

1. 代码、测试、四份权威文档保持 100% 一致。
2. 新增或修改功能时，同步补充测试文档中的测试范围、常见失败、根因、解决方案和可执行排查步骤。
3. API、DTO、错误码或鉴权变化时同步更新 API 文档。
4. 表、字段、索引、枚举、关系或事务变化时同步更新数据库文档。
5. 能力、依赖、目录、配置、启动或验证命令变化时同步更新 README。
6. 删除已被新需求取代的旧说明、旧路由、旧配置、旧测试和兼容逻辑。

禁止仅修改代码而把文档留给后续 Task。

## 3. 架构边界

- `internal/domain`：领域错误、值对象、状态机和端口，不依赖 Gin、Ent、Redis 或供应商 SDK。
- `internal/controlplane`：认证、组织、项目、模型、供应商和审批用例；负责事务编排，不处理 HTTP。
- `internal/dataplane`：API Key 鉴权、白名单、binding 选择、协议代理、流式转发和故障切换。
- `internal/infrastructure`：Ent/PostgreSQL、Redis、SMTP、加密和外部服务实现。
- `internal/api/http`：Gin 路由、DTO 绑定、认证上下文、统一 envelope 和错误映射；禁止直接编写数据库业务。
- `cmd/server`：依赖装配、bootstrap 和优雅停机。
- `cmd/migrate`：显式数据库 schema 初始化；服务启动不得隐式执行迁移。
- `Dockerfile`：backend/migrate 共用的多阶段、非 root 应用镜像。
- `compose.yaml`：本地 PostgreSQL、Redis、迁移和 backend 的权威编排定义。
- `scripts/compose.ps1`：Doppler `nebula-api/dev_personal` 注入与安全的容器 DSN 改写入口。

依赖方向必须从接口层和基础设施层指向应用/领域抽象，禁止领域层反向依赖框架。

## 4. 数据与事务规则

- PostgreSQL + Ent，UUIDv7，无 `nebula_` 表前缀。
- 资源通过 `active/inactive/disabled` 停用，不提供业务 DELETE API，不使用 `deleted_at`。
- API Key 状态迁移必须使用数据库事务和带旧状态条件的更新，禁止“先查后无条件写”。
- 老师终审通过必须在同一事务中完成模型就绪校验、组织成员幂等创建、项目成员幂等创建、审计写入和状态更新。
- API Key 明文只在首次领取响应中出现；数据库只保存带 pepper 的 HMAC/hash 和 prefix。
- 驳回、撤销和审核记录不可删除；驳回与撤销原因必填。
- 验证码、refresh session、老师邀请和 SSE 事件只存 Redis，不新增 PostgreSQL 表。
- 不执行自动 fallback、双写、旧字段映射或旧路径兼容。

## 5. Docker 与本地运行规则

- Compose 项目名固定为 `nebula-api`；一个 service 只运行一个职责明确的进程。
- `frontend` 是 Vue 3/Vite/TypeScript 官方控制台，由非 root Nginx 提供 SPA 与同源 `/api`、`/v1` 反向代理；不得创建绕过类型化 API client 的页面直连逻辑。
- 应用镜像版本只从根目录 `VERSION` 读取，格式必须为完整 SemVer `X.X.X`。发布可观察行为、API、数据库或运行时变更时按 SemVer 更新版本。
- Dockerfile 基础镜像和 Compose 第三方镜像必须使用可验证的精确 tag，禁止 `latest`、浮动 major/minor tag 和无版本本地镜像。
- secret 只允许通过 Doppler 项目 `nebula-api` 的 `dev_personal` config 在运行时注入。禁止写入 Dockerfile、Compose YAML、`.env`、build arg、镜像 layer、日志或文档。
- PostgreSQL、Redis 不暴露宿主机端口；backend 只绑定 `127.0.0.1:8080`，frontend 只绑定 `127.0.0.1:8081`。修改 service、image、tag、port、network、volume、healthcheck、启动命令或 Doppler 变量映射时，必须同步 README 和故障排查文档；影响 API/数据库时继续同步对应契约文档。
- `migrate` 必须是显式的一次性 service，backend 依赖其成功退出；禁止 backend 启动时隐式迁移。
- 普通停止使用 `.\scripts\compose.ps1 down` 并保留命名卷。`down -v` 属于破坏性数据删除，必须获得用户明确授权。

## 6. API 与安全规则

- 控制面仅使用 `/api/v1` 和统一响应 envelope；网关仅使用标准 `/v1` 原生协议响应。
- 所有用户输入必须做长度、枚举、格式和资源归属校验；JSON 未知字段必须拒绝。
- 所有受保护操作必须同时校验 JWT、数据库用户状态、角色和资源范围。
- Refresh token 使用 Secure/HttpOnly/SameSite Cookie，Redis 只保存 hash，并实现轮换与重用检测。
- 密码使用 bcrypt；供应商凭据使用 AES-256-GCM；随机 token 和 API Key 使用 `crypto/rand`。
- 禁止记录或返回密码、验证码、完整 API Key、refresh token、JWT、供应商凭据、加密主密钥或数据库 DSN。
- 不得把 secret 放在 URL 查询参数；Realtime 浏览器鉴权使用 WebSocket subprotocol。
- 上游代理不得透传客户端对供应商的 Authorization；必须覆盖为解密后的供应商凭据。
- 流式响应一旦向客户端写出字节，禁止切换供应商。

## 7. 实现流程

### 阶段一：只读调查

- 优先使用 `rg` 和 `rg --files`。
- 阅读适用指令、权威文档、相关源码、测试和参考项目真实实现。
- 此阶段不构建、不测试、不启动服务。

### 阶段二：批量实现

- 一次性完成完整调用链、配置、测试和文档修改。
- 未完成全部实现前不运行 formatter、生成器、测试、构建或服务。
- 使用 `apply_patch` 修改文本文件；中文文件使用 UTF-8。
- 不新增生产依赖，除非现有技术栈无法正确完成需求；新增时必须说明用途和安全边界。

### 阶段三：统一验证

按风险集中执行：

1. `go generate ./internal/infrastructure/db/ent`
2. `gofmt`
3. `go test ./...`
4. `go vet ./...`
5. 检查最终 Git diff/status、敏感信息、乱码、冲突标记和文档一致性。

禁止连接或修改生产数据库，禁止使用真实供应商密钥进行测试。

## 8. 测试要求

- 状态机、认证、安全边界、事务、协议转换、流式代理和持久化契约必须有测试。
- API 改动优先使用少量契约/集成测试，避免为每层重复测试。
- 修复真实问题时必须添加回归测试，并在故障排查文档记录症状、根因和验证命令。
- 不测试简单 getter、框架行为或无逻辑字段映射。
- 测试不得依赖真实 SMTP、真实供应商、生产 Redis 或生产 PostgreSQL。

## 9. Git 与参考项目安全

- 不修改 `D:\VScodeProjects\NebulaCloud\nebula-ai`。
- 不覆盖、回滚或删除用户已有修改。
- 未经要求不创建分支、不提交、不 amend、不 push、不创建 PR。
- 禁止 `git reset --hard`、强制 checkout 和强制清理。
- 不提交 `.env`、凭据、令牌、数据库内容或测试生成的 secret。

## 10. 当前命令

```powershell
go generate ./internal/infrastructure/db/ent
go run ./cmd/migrate
go run ./cmd/server
go test ./...
go vet ./...
cd frontend
npm run typecheck
npm run test
npm run build
.\scripts\compose.ps1 config --quiet
.\scripts\compose.ps1 up -d --build
.\scripts\compose.ps1 ps
```
