# Nebula AI Gateway NextGen

## 1. 项目简介与重构迁移背景

本项目是对原中转站系统（只读参考源：`D:\VScodeProjects\NebulaCloud\nebula-ai`）的从零现代化重构版本。

- **核心目标**：继承参考项目已验证的业务能力与控制台交互，采用数据面与控制面分离的高性能整洁架构，移除旧实现中的服务内部分层耦合。
- **架构选择**：采用方案 A，即 Go + Gin + Ent + Redis + PostgreSQL 作为后端，Vue 3 + Vite + TypeScript + Tailwind CSS + Pinia + Vue Router 作为前端。
- **边界约束**：参考项目仅用于核对业务规则与 UI；不得修改、复制其代码，或将其历史架构直接迁入本仓库。

### 核心业务模块

- **身份与租户**：用户认证、组织、项目、成员关系与 RBAC。
- **令牌与鉴权**：API Key 创建、轮换、吊销、配额、过期时间、IP 白名单、模型白名单与分组权限。
- **智能路由与负载均衡**：供应商、渠道、模型映射、权重轮询、健康检查、熔断和故障切换。
- **高性能模型代理**：OpenAI 与 Anthropic 兼容接口、SSE 流式转发、请求归因、实时 Token 计费与配额扣减。
- **财务与用量**：充值/兑换、成本核算、消费明细、账单与聚合统计。
- **治理与运营**：控制台看板、调用日志、审计与风控、通知、渠道健康监控。

---

## 2. 技术栈选型

### 后端

| 领域 | 选型 | 用途 |
| --- | --- | --- |
| 语言与 HTTP 框架 | Go 1.22+ / Gin | 独立部署的数据面和控制面 HTTP 服务 |
| ORM 与迁移 | Ent | PostgreSQL 业务实体、类型安全查询与 schema migration |
| 缓存与协调 | Redis | L2 缓存、分布式限流、配额原子扣减、并发锁与事件流 |
| 主数据存储 | PostgreSQL | 配置、用户、订单、账单及控制面业务数据 |
| 观测性 | Zap + OpenTelemetry | 结构化日志、TraceID、指标和链路追踪 |

### 前端

| 领域 | 选型 | 用途 |
| --- | --- | --- |
| 框架与构建 | Vue 3 / Vite / TypeScript | 独立管理控制台 |
| UI 与样式 | Tailwind CSS + Naive UI | 高密度运营后台与可复用组件 |
| 状态与路由 | Pinia + Vue Router | 用户会话、权限状态和动态路由 |

---

## 3. 架构规约

### 控制面与数据面分离

- **数据面 (`internal/dataplane`)** 负责对外模型请求。它只能依赖内存缓存、Redis、上游 HTTP 客户端及抽象端口；代理热路径禁止同步读取 PostgreSQL。
- **控制面 (`internal/controlplane`)** 负责身份、组织、项目、Key、渠道、充值、统计、审计和通知等管理业务，并通过事件或缓存失效机制向数据面发布配置变更。
- **领域层 (`internal/domain`)** 只定义实体、值对象、业务规则和仓储端口，不依赖 Gin、Ent、Redis 或供应商 SDK。
- **基础设施层 (`internal/infrastructure`)** 实现领域端口，集中接入 PostgreSQL、Redis、日志、观测与外部服务。
- **接口层 (`internal/api`)** 只处理 HTTP 路由、DTO 校验、认证上下文和统一错误响应，禁止承载业务编排与存储访问。

### 三层缓存

1. **L1 本地内存 LRU**：缓存热点 API Key 校验结果、渠道运行状态与模型路由快照，避免每个请求访问网络缓存。
2. **L2 Redis**：承载跨实例限流、实时并发控制、配额扣减原子操作、分布式锁及数据面缓存一致性。
3. **L3 PostgreSQL**：只持久化配置、用户、组织、项目、充值、账单与异步落盘日志；不进入代理热路径。

### 数据面约束

- 流式响应使用 `io.Reader`/`io.Writer` 管道转发，避免聚合完整响应后再写出。
- 供应商协议转换必须由 `dataplane/adapter` 实现；代理主流程不得针对具体供应商写条件分支。
- 路由策略只从缓存读取渠道与模型快照；故障切换、熔断与健康状态由 `dataplane/router` 管理。
- 每个请求必须携带 TraceID，并可追溯至 API Key、用户、组织、项目、模型与渠道。

### 前端约束

- `frontend/src/views` 禁止直接调用 `fetch`、`axios` 或其他 HTTP 客户端；所有请求必须经由 `frontend/src/api` 的类型化函数。
- 前端权限、会话和应用级状态仅由 Pinia store 管理；页面不得维护可跨页面复用的全局状态副本。
- 路由守卫集中于 `frontend/src/router`，页面组件不得自行实现权限绕过逻辑。

---

## 4. 目录结构

```text
Nebula-api/
├── cmd/
│   ├── server/                 # HTTP/API Gateway 主服务入口
│   └── cron/                   # 异步定时任务入口：额度同步、日志清理、健康检查
├── internal/
│   ├── dataplane/              # 高性能 AI API 代理引擎
│   │   ├── proxy/              # SSE、Chunked HTTP 流式转发
│   │   ├── router/             # 选渠、权重策略、熔断与故障切换
│   │   ├── quota/              # 实时计费、额度与并发控制
│   │   └── adapter/            # OpenAI、Anthropic、Gemini 等协议适配器
│   ├── controlplane/           # 管理后台与用户业务用例
│   │   ├── auth/               # 登录、JWT/OAuth2 和身份管理
│   │   ├── user/               # 用户资料与额度查询
│   │   ├── key/                # API Key 生命周期与权限
│   │   ├── channel/            # 供应商渠道、密钥池与健康探针
│   │   └── redemption/         # 兑换码、充值与订单
│   ├── domain/
│   │   ├── model/              # 领域实体与值对象
│   │   └── repository/         # 持久化与外部资源端口
│   ├── infrastructure/
│   │   ├── db/                 # Ent 客户端、PostgreSQL 与迁移
│   │   ├── redis/              # Redis 缓存、限流与分布式锁
│   │   └── logger/             # 结构化日志与观测实现
│   └── api/
│       ├── http/               # REST Controller、DTO 与路由注册
│       └── middleware/         # CORS、RateLimit、TraceID、统一错误处理
├── frontend/
│   └── src/
│       ├── api/                # 类型化 REST 客户端
│       ├── assets/             # 图标、图片与全局样式
│       ├── components/         # 通用、无业务耦合 UI 组件
│       ├── views/
│       │   ├── auth/           # 登录、注册、重置密码
│       │   ├── dashboard/      # 总览与统计看板
│       │   ├── keys/           # API Key 管理
│       │   ├── channels/       # 管理员渠道配置
│       │   └── logs/           # 用量与调用明细
│       ├── store/              # Pinia 状态
│       ├── router/             # 路由与权限守卫
│       └── utils/              # 格式化、时间与 SSE 工具
├── config/                     # YAML 与环境变量模板
├── docs/                       # 架构、OpenAPI 与设计文档
├── scripts/                    # Docker、数据库初始化与运维脚本
├── AGENTS.md                   # Agent 研发约束
└── README.md                   # 当前文档
```

---

## 5. 开发与快速开始

代码骨架、依赖清单、容器编排和启动指令将在对应实现完成后补充。未落地的命令不得作为可执行命令记录在本文档中。
