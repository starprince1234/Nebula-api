# ?? 现代化 AI 中转站项目（Greenfield Replatforming）- Codex 指令与架构规范

## 1. 项目概览与重构愿景 (Project Overview & Mission)
本项目是一个**从零搭建的高性能、高扩展、无冗余代码的现代化 AI API 中转站/网关系统**。
项目的核心使命是：在彻底清除历史技术债务的前提下，重构出一套具备整洁架构（Clean Architecture）、极高吞吐量与极佳维护性的现代中转站系统[1][5]。

### ?? 唯一参考项目源 (Single Source of Truth Reference)
* **参考项目路径**：`D:\VScodeProjects\NebulaCloud\nebula-ai`
* **参考规则**：
  1. 该参考项目已线上运行，包含完整的业务逻辑与前端界面形式。
  2. **只读约束**：严禁修改该路径下的任何文件，仅将其作为业务逻辑与UI交互的参考源。
  3. **拒绝冗余**：绝不照搬原项目中的冗余代码、过时架构设计或硬编码逻辑[5][8]。

---

## 2. 迁移与研发决策三步法工作流 (Migration & Decision Workflow)

当用户/开发者提出任何功能模块的开发需求时，Codex 必须严格执行以下**“三步决策流程”**：

```
[用户提出功能需求]
       │
       ▼
[步骤 1: 查阅原项目] ───> 检索并分析 `D:\VScodeProjects\NebulaCloud\nebula-ai` 的实现（后端逻辑 + 前端 UI）
       │
       ▼
[步骤 2: 顶尖思路评估] ──> 评估原实现是否符合现代最佳实践（高性能、低耦合、易维护、整洁架构）
       │
       ├─── [符合顶尖标准] ──> 【步骤 3A: 精准迁移】保留业务逻辑与 UI 精髓，用干净规范的新代码落地。
       │
       └─── [存在冗余/过时] ──> 【步骤 3B: 替代重构】提炼核心业务规则，用业界顶尖架构/设计模式重新设计并替代[1]。
```

### 决策评估维度：
- **后端评估**：接口设计是否符合 RESTful/gRPC 标准？流式代理 SSE/WebSocket 是否高效无内存泄漏？DAO/Service 职责是否分层解耦？是否存在死代码或重写轮子？
- **前端评估**：组件拆分是否合理？状态管理（Pinia/Zustand等）是否轻量规范？样式与响应式布局是否现代化？性能开销是否合理？

---

## 3. 技术栈与整洁架构规约 (Architecture & Technology Standards)

本项目采用**整洁架构 (Clean Architecture)** 思想划分分层结构，确保依赖单向流动[1][4]：

### 核心分层设计
1. **Domain / Entities (领域实体层)**：纯粹的业务实体与核心规则，不依赖任何外部框架。
2. **Use Cases / Services (应用用例层)**：编排业务流程，处理中转站的核心逻辑（如按 token 计费、路由分发、限流策略）。
3. **Interface Adapters / Controllers (接口适配层)**：处理 HTTP/gRPC 请求映射、数据传输对象（DTO）转换。
4. **Infrastructure / Drivers (基础设施层)**：数据库持久化、Redis 缓存、第三方 AI 模型服务 SDK 适配。

### 关键业务性能要求
- **流式代理 (Streaming Proxy)**：AI 响应中转必须采用 Zero-Copy 或低内存开销的高性能流式转发（Stream Pipeline）。
- **统一响应与异常处理**：全局统一的 Result 包装器与标准的错误码定义。
- **高并发与缓存优化**：API Key 鉴权、路由映射与配额扣减需具备 Redis/内存二级缓存能力。

---

## 4. Codex 行为准则与操作红线 (Operational Guardrails)

### 文档驱动开发规约 (Documentation-Driven Rule)

⚠️ 【强制规则】文档驱动同步规约：

- 每次进行任何 Backend/Frontend 功能编写前，必须优先查阅 `docs/DATABASE.md` 与 `docs/API_DOCUMENTATION.md`。
- 任何代码层面增加/修改的数据库表结构、字段、枚举值，或新增/修改的 API 接口路由、请求/响应结构，必须同步更新这两个文档。
- 每次 Task 结束时，代码实现必须与这两个文档 100% 保持一致。

### ? ALWAYS (必须遵守)
- 编写简洁、自解释且类型安全的代码，遵循 SOLID 设计原则[1]。
- 每次实现新功能前，主动检索并告知用户原项目 `D:\VScodeProjects\NebulaCloud\nebula-ai` 中的对应实现及你的改进评估[5]。
- 保证前后端代码具备完善的接口类型定义（TypeScript / Strong Type Schema）。
- 模块变更必须保证独立的自测性与高内聚[1]。

### ?? ASK FIRST (需明确确认)
- 引入新的重型第三方依赖库时。
- 涉及数据库 Schema 的破坏性变更（Break Changes）或重大结构调整时。
- 决定采用与原项目完全不同的业务数据模型时。

### ? NEVER (严禁触碰)
- **严禁直接修改或覆盖原项目文件**（原项目仅作为只读参考路径）。
- **严禁直接复制原项目已识别的冗余代码**（如无用的多重封装、遗留死代码、硬编码配置）。
- **严禁在代码中明文写入 API Key、数据库密码等敏感密钥**。

---

## 5. 项目命令集预留 (Commands Reference)
*(请在确定技术栈后补充具体命令，例如 npm/pnpm/go/cargo 等)*

- **启动开发环境**: `[TBD: e.g. pnpm dev / go run main.go]`
- **构建生产包**: `[TBD: e.g. pnpm build / go build]`
- **代码检查与 Lint**: `[TBD: e.g. pnpm lint]`
- **单元测试**: `[TBD: e.g. pnpm test]`
```