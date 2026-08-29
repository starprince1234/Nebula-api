# Nebula 首期 API 契约

## 1. 文档范围与状态

`0.7.0` credits 字段统一返回最多三位小数的定点字符串，usage/call/input DTO 统一使用 snake_case。学生申请、导师 approve、老师 approve 的 `requested_monthly_credits` / `monthly_credits` 均为当前必填契约，不提供旧 approve body 兼容。

导师项目用量响应同时返回顶层 `free_models` 项目汇总和 `members[].free_models` 成员明细；两者的每个模型均包含 `id`、`name`、固定 `credits: "0.000"` 与 `calls`。同一模型的项目 `calls` 必须等于全部成员对应 `calls` 之和。

`members[].keys[]` 同时返回 `models` 白名单摘要（`id`、`name`），供调用日志和输入监控按“项目 → 成员 → Key → 模型”进行级联筛选；下级为空表示当前上级范围内全部。

本文档定义首期控制面 `/api/v1` 与模型网关 `/v1` 的已实现契约。Gin Handler、控制面 Service、Redis SSE、审批事务和网关运行逻辑均已落盘；实际运行依赖已初始化的 PostgreSQL、Redis 和有效环境配置。

契约依据参考项目真实 Controller、Schema 与 Gateway 路由重新设计，不兼容旧 `/api/nebula/gateway`、无 `/v1` 路径别名或 internal usage/monitor 路由。

## 2. 通用约定

### 2.0 运维健康检查

| 方法 | 路由 | 鉴权 | 成功 | 用途 |
| --- | --- | --- | --- | --- |
| GET | `/health/live` | 无 | `200` | 进程存活探针，不访问依赖 |
| GET | `/health/ready` | 无 | `200` | PostgreSQL 与 Redis 均可用时就绪 |

`/health/ready` 依赖不可用时返回 `503`。健康响应不使用控制面 envelope，且只暴露 `ok/unavailable`，不得返回 DSN、host、账号、异常栈或其他内部连接细节。

### 2.1 传输与格式

- 控制面请求和响应使用 HTTPS + UTF-8 JSON；SSE 除外。
- 控制面时间字段使用 RFC 3339 UTC，例如 `2026-08-18T09:30:00Z`。
- 资源 ID 为 UUID 字符串；模型的对外 `model_id` 为字符串。
- 未知 JSON 字段应拒绝并返回 `VALIDATION_ERROR`。
- 首期列表按文档指定的稳定字段排序并全量返回，`meta.next_cursor` 固定为 `null`；引入分页时必须同时更新 Handler、Service、本文档和测试。

### 2.2 控制面鉴权

- 受保护接口使用 `Authorization: Bearer <access_token>`。
- Access token 为短期 JWT；Refresh token 为不透明随机值，只通过 `Secure; HttpOnly; SameSite=Lax` Cookie 传输。
- Refresh token 每次刷新都轮换；Redis 中保存 token hash、session family 和 TTL。复用已轮换 token 时撤销整个 family。
- 禁用用户、退出登录、重置密码后，其相关 refresh session 必须撤销。
- RBAC 以服务端 JWT 身份和数据库中的最新用户状态为准，客户端不得提交或覆盖角色。

### 2.3 成功响应

控制面统一使用 envelope：

```json
{
  "data": {},
  "meta": {
    "next_cursor": null
  },
  "request_id": "01K2..."
}
```

单资源响应可省略 `meta`。创建成功使用 `201 Created`；无响应体的成功操作使用 `204 No Content`。

### 2.4 错误响应

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "request validation failed",
    "details": [
      {"field": "email", "reason": "invalid_format"}
    ]
  },
  "request_id": "01K2..."
}
```

| HTTP | 错误码 | 场景 |
| --- | --- | --- |
| 400 | `VALIDATION_ERROR`, `REJECTION_REASON_REQUIRED` | JSON、字段或业务输入不合法；驳回申请必须填写原因，通过意见可以省略 |
| 401 | `AUTHENTICATION_REQUIRED`, `INVALID_CREDENTIALS`, `TOKEN_EXPIRED` | 未登录或凭据失效 |
| 403 | `FORBIDDEN`, `ACCOUNT_DISABLED` | 角色或资源范围不允许 |
| 404 | `RESOURCE_NOT_FOUND` | 资源不存在或调用者不可见 |
| 409 | `EMAIL_ALREADY_REGISTERED`, `RESOURCE_NAME_CONFLICT`, `INVALID_STATE_TRANSITION`, `PROJECT_HAS_NO_MENTOR`, `MENTOR_NOT_IN_ORGANIZATION`, `MENTOR_ALREADY_PROJECT_MEMBER`, `MODEL_NOT_READY`, `KEY_ALREADY_CLAIMED` | 唯一性或状态冲突；老师审批导师项目申请时会区分导师未加入组织和已加入项目 |
| 422 | `VERIFICATION_CODE_INVALID`, `VERIFICATION_CODE_EXPIRED`, `INVITATION_INVALID` | 可解析但无法完成认证流程 |
| 429 | `RATE_LIMITED`, `VERIFICATION_LOCKED` | 冷却或失败锁定 |
| 500 | `INTERNAL_ERROR` | 未预期服务端错误，不泄露内部细节 |
| 503 | `DEPENDENCY_UNAVAILABLE` | PostgreSQL、Redis 或邮件服务不可用 |

### 2.5 核心资源形状

```json
{
  "user": {
    "id": "uuid",
    "name": "Alice",
    "email": "alice@example.com",
    "role": "student",
    "status": "active",
    "created_at": "2026-08-18T09:30:00Z"
  },
  "organization": {
    "id": "uuid",
    "name": "AI Lab",
    "description": null,
    "status": "active"
  },
  "project": {
    "id": "uuid",
    "organization_id": "uuid",
    "name": "Research Assistant",
    "description": null,
    "status": "active",
    "has_mentor": true
  },
  "model": {
    "id": "uuid",
    "model_id": "gpt-4.1-mini",
    "display_name": "GPT-4.1 mini",
    "description": null,
    "category": "text",
    "capabilities": ["chat", "tools"],
    "input_modalities": ["text", "image"],
    "output_modalities": ["text"],
    "context_window": 1048576,
    "max_output_tokens": 32768,
    "is_common": true,
    "status": "active"
  }
}
```

## 3. 认证与当前用户

| 方法 | 路由 | 鉴权 | 用途 |
| --- | --- | --- | --- |
| POST | `/api/v1/auth/verification-codes` | 无 | 发送学生/导师注册验证码 |
| POST | `/api/v1/auth/register/student` | 无 | 学生注册 |
| POST | `/api/v1/auth/register/mentor` | 无 | 导师注册 |
| POST | `/api/v1/auth/login` | 无 | 三种角色统一登录 |
| POST | `/api/v1/auth/refresh` | Refresh Cookie | 轮换 refresh token 并签发 access token |
| POST | `/api/v1/auth/logout` | Refresh Cookie，可带 Access token | 撤销当前 refresh session 并清除 Cookie |
| POST | `/api/v1/auth/password/forgot` | 无 | 向已注册邮箱发送重置验证码；始终返回同一结果以避免枚举用户 |
| POST | `/api/v1/auth/password/reset` | 无 | 使用验证码重置密码并撤销全部 refresh session |
| POST | `/api/v1/auth/teacher-invitations/activate` | 无 | 受邀老师设置姓名和密码并激活账户 |
| GET | `/api/v1/me` | 任意登录用户 | 获取当前用户资料与角色 |

### 3.1 请求与响应

发送注册验证码：

```json
{"email":"alice@example.com","purpose":"student_registration"}
```

`purpose` 只能为 `student_registration` 或 `mentor_registration`。成功返回 `202 Accepted`，响应不包含验证码。

学生/导师注册请求：

```json
{
  "name": "Alice",
  "email": "alice@example.com",
  "password": "user-provided-password",
  "verification_code": "123456"
}
```

注册成功返回 `201 Created` 和 `user`，不自动改变客户端提交的角色；路由本身决定角色。

部署可通过运行时 Secret 配置固定学生/导师测试账号。该初始化不增加 HTTP 路由，也不绕过公开注册接口的验证码契约；backend 只在启动时为尚不存在的邮箱创建对应角色的 `active` 用户。已有同邮箱用户不会被 Secret 覆盖，角色冲突会阻止服务启动。

统一登录请求：

```json
{"email":"alice@example.com","password":"user-provided-password"}
```

登录和 refresh 成功均设置新的 Refresh Cookie，并返回：

```json
{
  "data": {
    "access_token": "jwt",
    "token_type": "Bearer",
    "expires_in": 900,
    "user": {"id":"uuid","name":"Alice","email":"alice@example.com","role":"student","status":"active"}
  },
  "request_id": "01K2..."
}
```

忘记密码请求只包含 `email`。重置请求包含 `email`、`verification_code`、`new_password`。老师邀请激活请求包含 `token`、`name`、`password`。

## 4. 实时事件

| 方法 | 路由 | 鉴权 | Content-Type |
| --- | --- | --- | --- |
| GET | `/api/v1/events` | 任意登录用户 | `text/event-stream` |

客户端使用支持自定义 Header 的 fetch streaming 方式携带 Bearer token。服务端发送心跳注释，并支持标准 `Last-Event-ID`。事件只用于提示刷新，不作为权威状态。

```text
id: 01K2...
event: api_key.status_changed
data: {"api_key_id":"uuid","status":"pending_teacher","updated_at":"2026-08-18T09:30:00Z"}

id: 01K2...
event: models.common_changed
data: {"revision":"01K2..."}
```

`api_key.status_changed` 仅发给该 Key 的学生；`models.common_changed` 发给全部已认证用户。断线重连、事件过期或 revision 变化后，客户端必须重新调用对应 REST 列表接口。

## 5. 学生接口

以下接口要求 `role=student` 且用户为 `active`。

| 方法 | 路由 | 用途 |
| --- | --- | --- |
| GET | `/api/v1/student/organizations` | 列出平台全部 ACTIVE 组织 |
| GET | `/api/v1/student/organizations/{organization_id}/projects` | 列出该 ACTIVE 组织全部项目，并返回真实状态与 `has_mentor` |
| GET | `/api/v1/student/models` | 模型广场 |
| POST | `/api/v1/student/api-keys` | 提交 Key 申请 |
| GET | `/api/v1/student/api-keys` | 列出自己的申请和 Key |
| GET | `/api/v1/student/api-keys/{api_key_id}` | 查看自己的审核详情与进度 |
| POST | `/api/v1/student/api-keys/{api_key_id}/claim` | 一次性领取已批准 Key |

组织和项目列表不要求学生已是成员。项目没有导师或已停用时仍返回并由界面禁选；无导师项目提交返回 `409 PROJECT_HAS_NO_MENTOR`，停用项目提交返回资源不存在。

模型广场集合为：全局 `is_common=true AND status=active` 模型，与当前学生 `approved/active` Key 关联模型的去重并集。后者即使后来停用仍返回真实状态，便于解释 Key 可用性。

提交申请：

```json
{
  "name": "research-key",
  "organization_id": "uuid",
  "project_id": "uuid",
  "model_ids": ["gpt-4.1-mini"],
  "requested_models": [{"model_id":"new-lab-model","display_name":"Lab Model","description":null,"category":"text","capabilities":["chat"],"input_modalities":["text"],"output_modalities":["text"],"context_window":null,"max_output_tokens":null}]
}
```

- `model_ids` 与 `requested_models` 合计为 1 至 100 个大小写不敏感去重后的模型 ID。新模型必须在 `requested_models` 中提交完整模型卡片字段。
- 服务端校验项目确实属于 `organization_id`，但只持久化 `project_id`，避免组织归属冗余。
- 新 `model_id` 大小写不敏感幂等创建为 `pending_configuration`。
- 创建申请、候选模型和 `api_key_models` 必须在一个事务完成。
- 同一学生进行中或生效 Key 名称大小写不敏感唯一；`rejected/revoked` 后可复用。

创建成功返回 `201 Created`：

```json
{
  "data": {
    "id": "uuid",
    "name": "research-key",
    "status": "pending_mentor",
    "progress": {"current":"mentor_review","completed_steps":["submitted"]},
    "organization": {"id":"uuid","name":"AI Lab"},
    "project": {"id":"uuid","name":"Research Assistant"},
    "models": [{"model_id":"gpt-4.1-mini","status":"active"}],
    "key_prefix": null,
    "claimed_at": null,
    "created_at": "2026-08-18T09:30:00Z"
  },
  "request_id": "01K2..."
}
```

详情额外返回按时间升序的 `audits`，每项含 `action`、`actor_role`、`comment` 和 `created_at`。不向学生返回审核人邮箱。

领取仅允许 `approved` 状态。成功时原子地切换到 `active`，并且只在本次响应返回完整 secret：

```json
{
  "data": {
    "api_key": "neb_sk_live_once_only",
    "key_prefix": "neb_sk_live_abc1",
    "claimed_at": "2026-08-18T09:30:00Z"
  },
  "request_id": "01K2..."
}
```

重复领取返回 `409 KEY_ALREADY_CLAIMED`，不会再次返回明文。老师批准事务会幂等创建学生的组织和项目成员关系；Key 撤销不移除这些关系。

## 6. 导师接口

以下接口要求 `role=mentor` 且用户为 `active`。

| 方法 | 路由 | 用途 |
| --- | --- | --- |
| GET | `/api/v1/mentor/organizations` | 列出导师所属 ACTIVE 组织 |
| GET | `/api/v1/mentor/organizations/{organization_id}/projects` | 列出该组织全部 ACTIVE 项目并标明负责状态和申请状态 |
| POST | `/api/v1/mentor/project-applications` | 申请负责项目 |
| GET | `/api/v1/mentor/project-applications` | 查看自己的项目申请历史 |
| GET | `/api/v1/mentor/api-key-reviews` | 列出负责项目中待初审申请 |
| GET | `/api/v1/mentor/api-key-reviews/{api_key_id}` | 查看待初审申请详情 |
| POST | `/api/v1/mentor/api-key-reviews/{api_key_id}/approve` | 初审通过 |
| POST | `/api/v1/mentor/api-key-reviews/{api_key_id}/reject` | 初审驳回 |
| GET | `/api/v1/mentor/projects/{project_id}/api-keys` | 列出负责项目的 ACTIVE Key |
| POST | `/api/v1/mentor/api-keys/{api_key_id}/revoke` | 撤销负责项目的 ACTIVE Key |

项目申请请求：

```json
{"project_id":"uuid"}
```

只能申请导师所属组织中的 ACTIVE 项目；同项目已有负责关系或 pending 申请时返回 `409 INVALID_STATE_TRANSITION`。

初审列表和详情展示申请学生的 `id/name/email`、组织、项目、申请模型及创建时间，不返回 Key hash。任一负责导师可对 `pending_mentor` 申请执行首次有效决策；并发后续请求返回 `409 INVALID_STATE_TRANSITION`。

通过请求可省略 body，或传 `{"comment":"optional"}`。驳回和撤销必须传非空 `{"comment":"reason"}`。撤销不会移除学生组织或项目成员关系。

## 7. 老师接口

以下接口要求 `role=teacher` 且用户为 `active`。老师拥有平台级管理权限，但没有项目成员或 ACTIVE Key 浏览接口。

### 7.1 路由

| 模块 | 方法 | 路由 | 用途 |
| --- | --- | --- | --- |
| 邀请 | POST | `/api/v1/teacher/invitations` | 邀请老师 |
| 组织 | GET | `/api/v1/teacher/organizations` | 组织列表 |
| 组织 | POST | `/api/v1/teacher/organizations` | 添加组织 |
| 组织 | PATCH | `/api/v1/teacher/organizations/{organization_id}` | 修改名称、说明或状态 |
| 组织 | GET | `/api/v1/teacher/organizations/{organization_id}/mentor-candidates` | 搜索 ACTIVE 导师候选 |
| 组织 | POST | `/api/v1/teacher/organizations/{organization_id}/mentors/{mentor_id}` | 幂等分配导师到组织 |
| 项目 | GET | `/api/v1/teacher/projects` | 项目列表，可按 `organization_id/status` 筛选 |
| 项目 | POST | `/api/v1/teacher/projects` | 添加项目 |
| 项目 | PATCH | `/api/v1/teacher/projects/{project_id}` | 修改名称、说明或状态 |
| 项目申请 | GET | `/api/v1/teacher/mentor-project-applications` | 导师项目申请列表 |
| 项目申请 | POST | `/api/v1/teacher/mentor-project-applications/{application_id}/approve` | 批准并幂等加入项目 |
| 项目申请 | POST | `/api/v1/teacher/mentor-project-applications/{application_id}/reject` | 驳回，原因必填 |
| 供应商 | GET | `/api/v1/teacher/providers` | 供应商列表 |
| 供应商 | POST | `/api/v1/teacher/providers` | 添加供应商并加密凭据 |
| 供应商 | GET | `/api/v1/teacher/providers/{provider_id}` | 供应商详情，不返回凭据密文或明文 |
| 供应商 | PATCH | `/api/v1/teacher/providers/{provider_id}` | 修改名称、URL、状态；可替换凭据 |
| 模型 | GET | `/api/v1/teacher/models` | 全部模型去重视图与 `route_ready` |
| 模型 | POST | `/api/v1/teacher/models` | 主动添加模型 |
| 模型 | GET | `/api/v1/teacher/models/{model_id}` | 模型及 binding 详情 |
| 模型 | PATCH | `/api/v1/teacher/models/{model_id}` | 配置模型元数据、常用标记和状态 |
| Binding | POST | `/api/v1/teacher/models/{model_id}/bindings` | 添加供应商 binding |
| Binding | PATCH | `/api/v1/teacher/model-bindings/{binding_id}` | 修改上游名、adapter、priority 或状态 |
| Key 终审 | GET | `/api/v1/teacher/api-key-reviews` | 只列出 `pending_teacher` 摘要 |
| Key 终审 | GET | `/api/v1/teacher/api-key-reviews/{api_key_id}` | 待终审摘要详情 |
| Key 终审 | POST | `/api/v1/teacher/api-key-reviews/{api_key_id}/approve` | 终审通过并自动加入成员关系 |
| Key 终审 | POST | `/api/v1/teacher/api-key-reviews/{api_key_id}/reject` | 终审驳回，原因必填 |

首期不提供任何 DELETE 接口。资源通过 `status=inactive/disabled` 停用。

### 7.2 管理请求

组织创建/修改字段为 `name`、`description`、`status`；项目创建字段为 `organization_id`、`name`、`description`，修改字段为 `name`、`description`、`status`。

老师邀请请求：

```json
{"email":"teacher2@example.com"}
```

供应商创建：

```json
{
  "name": "Provider A",
  "base_url": "https://api.provider.example/v1",
  "credential": "provider-secret"
}
```

`credential` 只作为写入字段；服务端加密后保存，所有读取响应只返回 `credential_configured: true/false`。

模型创建/修改使用核心模型字段。将模型改为 `active` 前必须至少存在一个 ACTIVE binding，且对应 provider 为 ACTIVE。Binding 请求形状：

`context_window` 和 `max_output_tokens` 在 PATCH 中支持三态：省略保持不变、`null` 清空、正整数更新。`model_id` 创建后只读。激活模型、停用 provider 或停用 binding 会在事务内锁定相关路由记录；若操作会令 ACTIVE 模型失去最后可用路由，返回 `409 MODEL_ROUTING_REQUIRED`，并在 `details.model_ids` 给出受影响模型。

```json
{
  "provider_id": "uuid",
  "upstream_model_name": "upstream-model",
  "adapter": "openai_compatible",
  "priority": 100,
  "status": "active"
}
```

`adapter` 可选值及职责：

| Adapter | 公共协议入口 | 上游协议 |
| --- | --- | --- |
| `openai_compatible` | `/v1/chat/completions`、`/v1/completions` | OpenAI Chat/Completions |
| `openai_responses` | `/v1/responses`、`/v1/responses/compact` | OpenAI Responses |
| `openai_embeddings` | `/v1/embeddings` | OpenAI Embeddings |
| `openai_images` | `/v1/images/generations`、`/v1/images/edits`、`/v1/images/variations` | OpenAI Images |
| `openai_audio` | `/v1/audio/transcriptions`、`/v1/audio/translations`、`/v1/audio/speech` | OpenAI Audio |
| `openai_video` | `/v1/videos`、资源查询、内容下载和 remix | OpenAI Videos |
| `openai_realtime` | `/v1/realtime` | OpenAI Realtime WebSocket |
| `openai_moderations` | `/v1/moderations` | OpenAI Moderations |
| `anthropic` | `/v1/messages` | Anthropic Messages |
| `cohere_rerank_v2` | `/v2/rerank` | Cohere Rerank v2 `/v2/rerank` |
| `google_gemini_v1beta` | `/v1beta/models/{model}:...` | Google Gemini 原生 `v1beta` |

同一模型需要支持多个协议时，应分别创建对应 adapter 的 Binding；网关不会在协议之间自动转换请求或响应。

老师模型列表的集合是所有 `models`，即平台用户曾申请模型的大小写不敏感去重并集加老师主动添加的模型；不返回申请次数或申请用户身份。

学生在申请 Key 阶段直接填写新模型完整卡片字段（`model_id`、`display_name`、`description`、`category`、`capabilities`、`input_modalities`、`output_modalities`、`context_window`、`max_output_tokens`）。服务端在提交事务内按 `model_id` 查询：已有模型直接复用平台权威配置；不存在时以 `pending_configuration` 创建。并发创建同一 ID 时首个成功记录为权威，后续申请复用该记录。模型申请审批通过并完成配置后，自动出现在学生模型广场。

### 7.3 Key 终审可见范围

老师只能读取 `pending_teacher` 申请。摘要包含申请学生的 `name/email`、组织、项目、模型状态和导师审核记录；不包含项目成员列表、历史 ACTIVE Key 或 Key hash。

终审通过请求可省略 body，或传 `{"comment":"optional"}`。通过前必须再次校验全部模型及 ACTIVE binding；不满足时返回 `409 MODEL_NOT_READY` 和未就绪 `model_ids`。通过事务幂等创建学生的组织与项目成员关系，追加审核记录，并转为 `approved`。事务依赖失败返回 `503 DEPENDENCY_UNAVAILABLE`，`details` 固定包含 `{"operation":"api_key_approval","state_changed":false}`，表示本次终审已完整回滚；前端必须同时展示 `request_id` 供管理员定位日志。

## 8. 网关 API

网关不使用控制面 envelope，成功和失败响应保持对应 OpenAI、Anthropic、Cohere 或 Google Gemini 协议原生格式。OpenAI 与 Anthropic 入口位于标准 `/v1`，Cohere Rerank 保持官方 `/v2`，Google Gemini 保持官方 `/v1beta` 原生路径。

### 8.1 路由

| 协议 | 方法 | 路由 | 请求形态 |
| --- | --- | --- | --- |
| OpenAI | GET | `/v1/models` | JSON |
| OpenAI | POST | `/v1/chat/completions` | JSON，支持 `stream=true` SSE |
| OpenAI | POST | `/v1/completions` | JSON，支持 `stream=true` SSE |
| OpenAI | POST/WS | `/v1/responses` | JSON SSE 或 Responses WebSocket；Codex CLI `response.create` 原样透传并改写 model |
| OpenAI | POST | `/v1/responses/compact` | JSON |
| OpenAI | POST | `/v1/embeddings` | JSON |
| Cohere Rerank | POST | `/v2/rerank` | JSON |
| OpenAI | POST | `/v1/images/generations` | JSON |
| OpenAI | POST | `/v1/images/edits` | multipart/form-data |
| OpenAI | POST | `/v1/images/variations` | multipart/form-data |
| OpenAI | POST | `/v1/audio/transcriptions` | multipart/form-data |
| OpenAI | POST | `/v1/audio/translations` | multipart/form-data |
| OpenAI | POST | `/v1/audio/speech` | JSON，响应音频流 |
| OpenAI | POST | `/v1/videos` | JSON 或 multipart/form-data；返回异步 Video resource |
| OpenAI | GET | `/v1/videos/{video_id}` | JSON |
| OpenAI | GET | `/v1/videos/{video_id}/content` | 视频二进制流 |
| OpenAI | POST | `/v1/videos/{video_id}/remix` | JSON |
| OpenAI | POST | `/v1/moderations` | JSON |
| Anthropic | POST | `/v1/messages` | JSON，支持 `stream=true` SSE |
| Google Gemini | POST | `/v1beta/models/{model}:generateContent` | JSON |
| Google Gemini | POST | `/v1beta/models/{model}:streamGenerateContent` | JSON；`alt=sse` 时为 SSE |
| Google Gemini | POST | `/v1beta/models/{model}:embedContent` | JSON |
| Google Gemini | POST | `/v1beta/models/{model}:batchEmbedContents` | JSON |
| OpenAI Realtime | WS | `/v1/realtime` | WebSocket 双向事件 |

不提供旧 `/api/nebula/gateway/*`、`/models`、`/chat/completions`、`/messages` 等别名，也不提供 health、usage、monitor 或 provider internal 路由作为公共 API。

协议实现依据：

- OpenAI Responses：[Create a model response](https://developers.openai.com/api/reference/resources/responses/methods/create/) 与 [Compact a response](https://developers.openai.com/api/reference/resources/responses/methods/compact/)
- OpenAI Embeddings：[Create embeddings](https://developers.openai.com/api/reference/resources/embeddings/methods/create/)
- OpenAI Images：[Images API](https://developers.openai.com/api/reference/resources/images/)
- OpenAI Audio：[Audio and speech](https://developers.openai.com/api/docs/guides/audio)
- OpenAI Videos：[Videos API](https://developers.openai.com/api/reference/resources/videos/)
- OpenAI Realtime：[Realtime and audio](https://developers.openai.com/api/docs/guides/realtime) 与 [WebRTC](https://developers.openai.com/api/docs/guides/realtime-webrtc)
- OpenAI Moderations：[Moderation](https://developers.openai.com/api/docs/guides/moderation)
- Anthropic Messages：[Messages API](https://docs.anthropic.com/en/api/messages)
- Cohere Rerank v2：[Rerank API](https://docs.cohere.com/reference/rerank)
- Google Gemini：[Generating content](https://ai.google.dev/api/generate-content)、[Embeddings](https://ai.google.dev/api/embeddings) 与 [All methods](https://ai.google.dev/api/all-methods)

### 8.2 鉴权与授权

- OpenAI-compatible HTTP 使用 `Authorization: Bearer <nebula_api_key>`。
- Anthropic Messages 同时接受协议原生 `x-api-key: <nebula_api_key>`；要求并透传合法 `anthropic-version`。
- Google Gemini 原生路由接受 `x-goog-api-key: <nebula_api_key>` 或 `Authorization: Bearer <nebula_api_key>`；禁止使用 `?key=`，网关向上游覆盖为供应商凭据。
- Realtime 使用 `Authorization` Header；浏览器无法设置 Header 时可使用标准 `Sec-WebSocket-Protocol` 承载凭据，不接受查询参数中的 API Key。
- Responses WebSocket 使用同一个 `/v1/responses`，在首个 `response.create` frame 中提取公开 model 并改写为上游 model；`OpenAI-Beta`、Codex metadata Header、`previous_response_id`、`prompt_cache_key`、加密 reasoning/compaction item 和未知字段均透明保留。
- Key 必须为 `active`，所请求 `model` 必须位于 `api_key_models` 白名单，模型、binding 和 provider 均须为 `active`。
- `/v1/models` 仅返回当前 Key 白名单中当前可路由的模型。

网关按模型倍率执行月度 credit 额度检查与调用记录；目录查询、视频状态查询和内容下载不计费。

### 8.3 代理与故障切换

- 保持协议原生 request/response、状态码、SSE 事件、multipart 文件和 WebSocket frame，不使用控制面 envelope。每类协议只选择同协议 adapter，不做跨协议 DTO 转换。
- `/v1/responses/compact` 是独立无状态 compact：网关不得裁剪返回 `output`，也不得解析或替换 opaque `encrypted_content`；Codex CLI 使用的 tools、reasoning、text、prompt cache 与 service tier 字段仅改写顶层 model。
- Responses 与 Realtime WebSocket 的每个 `response.create` 都是独立计费操作；额度不足时发送协议内 error event 并保持连接。
- Videos 创建成功后，Redis 保存 `video_id -> binding_id`；查询、内容下载和 remix 必须回到创建该资源的上游，不能按 priority 重新选择供应商。
- OpenAI 和 Cohere 请求体中的公共 `model` 会替换为 Binding 的上游模型名；Gemini 的路径模型会替换为上游模型名，`batchEmbedContents.requests[*].model` 同步改写为 `models/{upstream_model_name}`。
- 候选 binding 按 `priority ASC, id ASC` 排序。
- 仅在连接错误、连接/响应超时、HTTP `429` 或 `5xx` 时尝试下一 binding。
- `4xx` 参数或鉴权错误除 `429` 外不得跨供应商重试。
- 流式响应在任何字节已发给客户端后不得切换供应商，以免拼接两个上游响应。
- 所有候选失败时，以目标协议的原生错误结构返回最后一个可解释错误，并附服务端 `request_id` 响应 Header；不得泄露供应商凭据或内部 URL。

### 8.4 协议错误

OpenAI-compatible 示例：

```json
{
  "error": {
    "message": "The requested model is not allowed for this API key.",
    "type": "invalid_request_error",
    "param": "model",
    "code": "model_not_allowed"
  }
}
```

Anthropic-compatible 示例：

```json
{
  "type": "error",
  "error": {
    "type": "permission_error",
    "message": "The requested model is not allowed for this API key."
  }
}
```

## 9. 用量与监控接口

学生使用 `GET /api/v1/student/usage?month=YYYY-MM` 查看每个 API Key 的模型 credit 分解。

导师使用 `GET /api/v1/mentor/projects/{project_id}/usage` 查看项目额度分配、成员 Key 进度与实际开销；`GET /api/v1/mentor/call-logs` 和 `GET /api/v1/mentor/input-monitor` 使用游标分页与项目范围授权。完整输入仅在详情读取时返回并写访问审计。

导师可通过 `PATCH /api/v1/mentor/api-keys/{api_key_id}/monthly-credit-quota` 调整负责项目内 Key 的月额度，请求包含 `monthly_credits` 与必填 `reason`。

老师使用 `GET /api/v1/teacher/project-spend` 查看按模型聚合的项目花费和 0 倍率模型调用次数。额度拒绝返回 429 以及 `api_key_quota_exceeded` 或 `project_quota_exceeded`。

## 10. 明确排除项

- 不实现余额、RPM、Token 限制、通知中心或模型价格。
- 不提供老师浏览项目成员或 ACTIVE Key 的接口。
- 不提供资源删除接口。
- 不代理 Files、Uploads、Batches、Fine-tuning 或 Assistants/Threads 等有状态资源 API；这些接口没有可在每个请求中稳定取得的公开 model，若未来纳入必须新增资源 ID 到 Binding 的完整归属状态与授权契约，不能按默认 Binding 猜测上游。
- 不提供旧路由兼容层、别名、双写或 internal usage/monitor 路由。

## 10. 官方 Web 控制台消费约定

仓库内 `frontend/` 是上述契约的官方 Vue 客户端。Vercel 项目的 Root Directory 为 `frontend`，生产页面使用 `https://www.lyn91r.cn`，并通过 `VITE_API_BASE_URL=https://api.lyn91r.cn` 调用 API；`frontend/vercel.json` 将 Vue Router history 路由统一回退到 `/index.html`，因此 `/login`、`/teacher/...` 等前端路由支持直接访问和刷新。本地 Vite 仍使用 `/api` 与 `/v1` 代理。生产 API 由 Cloudflare Tunnel 暴露为 `https://api.lyn91r.cn`，Tunnel Service 指向内部 `http://backend:8080`；cloudflared 通过生产 `edge` 网络直接访问 backend 并连接 Cloudflare edge，不改变公开路由或 HTTP 契约。控制面 CORS 只允许正式前端 Origin，并允许 credentials、Authorization、Content-Type 和 Last-Event-ID；Realtime WebSocket 同样校验正式前端 Origin。

- access token 只保存在 Pinia 内存中，不写入 localStorage、sessionStorage 或 URL。
- refresh token 由 HttpOnly Cookie 管理，所有请求使用 `credentials: include`；并发 `401` 共享一次 refresh 请求，失败后返回登录页。
- 页面启动时路由守卫与应用壳共享同一个 bootstrap refresh 请求；不会因重复调用轮换同一个 refresh token。刷新成功后重新建立内存 access token 和当前用户，刷新失败才进入登录态失效流程。
- 当前用户与角色以 `/api/v1/me`/session 响应为准，路由守卫不能替代服务端权限校验。
- 工作区路由使用全局唯一的技术名称，中文页面标题存放在独立的路由元数据中；导师与老师均可使用“项目管理”标题，但 `/mentor/projects` 与 `/teacher/projects` 必须同时保留独立路由记录。
- 官方 API client 对控制面 GET 与写请求维护仅存在于内存的活动计数：GET 活动只驱动全局不占布局的顶部进度条，学生、导师、老师页面及候选弹窗通过局部 `LoadingRegion` 在自身内容区域显示骨架；刷新保留已渲染数据，写请求显示提交状态并由具体操作按钮阻止重复提交。计数必须在成功、结构化错误、网络错误和 `401` refresh 重试后清零；该 UI 状态不改变请求、响应 envelope、鉴权或错误处理契约。
- 页面切换、弹层、状态和加载反馈遵循 `prefers-reduced-motion`；关闭位移、缩放、旋转和 shimmer 后仍必须保留文字、禁用态与 `aria-busy` 等可访问状态。
- 老师模型卡片将 `status=pending_configuration` 且 `route_ready=true` 的模型展示为“已就绪”；`route_ready` 只作为权威派生条件，不再重复渲染为卡片小泡泡。该展示映射不修改 API 返回的模型状态，启用/停用仍遵循模型状态契约。
- `/api/v1/events` 使用带 `Authorization` Header 的 fetch streaming，不使用原生 EventSource；收到状态事件后重新获取 REST 权威数据。
- 一次性 API Key 仅在领取弹窗中显示和复制，关闭后不写入浏览器持久存储，也不支持再次展示。
- 老师控制台不提供项目成员和 ACTIVE Key 浏览入口，前端不得扩大服务端可见范围。
