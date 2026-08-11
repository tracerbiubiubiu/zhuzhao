# 文档目录

> 本项目所有文档的组织入口。按文档类型分目录存放。

## 目录结构

```
docs/
├── README.md                    # 本文件，文档索引
│
├── design/                      # 设计文档（架构与方案）
│   ├── architecture.md          # 系统架构（What：系统是什么）
│   ├── design-decisions.md      # 设计决策（Why：为什么这么选）
│   └── implementation-plan.md   # 实施计划（How：怎么搭）
│
├── api/                         # API 文档
│   └── (Swagger 生成)            # Phase 1 后期由 swag 自动生成
│
├── ops/                         # 运维文档
│   ├── deployment.md            # 部署指南
│   └── runbook.md               # 故障处理手册
│
└── adr/                         # 架构决策记录（ADR）
    └── README.md                # ADR 索引与模板说明
```

## 文档分类说明

### design/ — 设计文档

项目"设计阶段"产出的文档，描述系统的架构、决策和计划。

| 文档 | 定位 | 读者 |
|------|------|------|
| `architecture.md` | 系统全貌：模块边界、数据库 schema、API 总表、分阶段计划 | 所有开发者 |
| `design-decisions.md` | 决策推理：方案对比、Q&A 讨论、故障场景分析 | 架构 review |
| `implementation-plan.md` | 实施步骤：Phase 1 的逐步搭建计划与验收标准 | 执行开发者 |

### api/ — API 文档

接口规格说明，供前后端联调使用。

- Phase 1 后期接入 `swag`，从代码注解自动生成 OpenAPI 文档
- 如需手写接口文档（如非 RESTful 接口），放在此目录

### ops/ — 运维文档

部署、配置、监控、故障处理等"上线后"所需的文档。

| 文档 | 内容 | 产出时机 |
|------|------|---------|
| `deployment.md` | 环境要求、配置项说明、部署步骤、Docker/K8s 配置 | Phase 1 |
| `runbook.md` | 常见故障现象、排查步骤、应急操作 | Phase 3 |

### adr/ — 架构决策记录

轻量级的决策日志，每个 ADR 记录一个独立的架构决策。

与 `design-decisions.md` 的区别：
- `design-decisions.md`：讨论过程的完整记录（含方案对比、推理细节）
- `adr/`：决策的最终结论摘要，编号管理，便于追溯

> ADR 格式适合在项目稳定后，从 `design-decisions.md` 中提取关键决策形成独立记录。Phase 1 暂不需要，目录预埋。

## 文档规范

### 命名

- 全小写，连字符分隔：`deployment-guide.md` 而非 `DeploymentGuide.md`
- 语义优先：`architecture.md` 而非 `doc1.md`

### 关联引用

文档间引用使用相对路径：

```markdown
详见 [设计决策](./design/design-decisions.md#1-jwt-无状态策略与权限缓存)。
```

### 更新时机

| 文档 | 更新时机 |
|------|---------|
| `architecture.md` | 架构变更时（新增模块、调整边界） |
| `design-decisions.md` | 每次有新的设计讨论 |
| `implementation-plan.md` | 每个 Phase 开始时 |
| `api/` | 接口变更时 |
| `ops/` | 部署配置变更、新增故障案例时 |
