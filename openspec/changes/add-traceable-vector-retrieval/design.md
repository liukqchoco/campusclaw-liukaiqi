## Context

现有系统已经把材料原文写入 MySQL，并用服务端 Session 强制执行角色权限与班级隔离。本变更要在这条可信边界之后加入切片、嵌入、向量库、检索和引用式回答。设计重点不是“能返回一段文字”，而是每个结果都能证明来自哪个班级、哪份材料和哪一个切片，并在没有依据时停止生成。

## Goals

- 教师和学生均可检索本班材料，任何客户端字段都不能改变班级范围。
- 同时支持原词检索、语义检索和混合检索，并保留各自的失败语义。
- 每条结果可追溯到 MySQL 中的切片正文和原材料。
- 检索无依据时返回确定结果，不调用生成模型。
- 教师可以选择切分策略并显式重建索引。
- Qdrant 或模型网关不成为浏览器可访问的入口，密钥不进入前端和仓库。

## Non-goals

本设计不覆盖流式聊天、跨班管理员检索、重排序模型、对象存储、多副本检索、生产网关和模型训练。

## Decisions

### Decision 1：MySQL 保存正文，Qdrant 只保存向量与标识

`knowledge_chunks` 保存 `chunk_text`、切片序号、字符区间、班级与材料外键。Qdrant collection `campusclaw_chunks` 只保存固定维度向量，payload 仅含 `class_id`、`material_id`、`knowledge_entry_id`、`chunk_id`、`chunk_index`。Qdrant point ID、`knowledge_chunks.id` 与 `payload.chunk_id` 三者相同。

备选方案是把切片正文也写进 Qdrant。这样少一次回表，但会产生双份正文和更新一致性问题；本课强调可追溯事实来源，因此摘录始终回 MySQL 读取。

### Decision 2：提供关键字、向量和混合三种模式

`keyword` 只访问 MySQL 全文索引；`vector` 将问句嵌入后在 Qdrant 中执行余弦检索；`hybrid` 同时运行两条路径，再以 RRF（`k = 60`）融合名次。默认模式为 `hybrid`。

关键字分数和余弦相似度不在同一量纲，不能直接相加。备选方案是归一化后加权，但更换嵌入模型后必须重新标定权重，因此选择只依赖名次的 RRF。

### Decision 3：绝对阈值先于名次融合

向量候选的余弦相似度低于 `0.35` 时直接丢弃；关键字路径也先移除无有效全文相关度的记录。过滤完成后才执行 RRF。RRF 分值只表达名次融合，不充当相关性阈值。

备选方案是始终返回固定数量候选。它会在无关问题上强行提供出处，违反“无依据不生成”，因此不采用。

### Decision 4：班级只来自 Session，并在两层复核

检索模块只读取认证中间件放入请求上下文的 `class_id`。请求 query、JSON 或 header 中的 `class_id` 解析后丢弃。

关键字 SQL 必须包含会话班级；Qdrant 查询必须包含同一班级 payload filter；向量命中回 MySQL 时再次使用 `chunk_id + class_id` 查询。即便向量侧漏掉过滤，回表仍不能取得其他班级正文。

备选方案是按班级建立独立 collection，隔离更强但 collection 数量随班级增长。本迭代使用单 collection 加双层过滤。

### Decision 5：三种切分策略共用统一输出

- `auto`：固定使用服务端默认的最大 800 字、重叠 80 字，并优先在空行、换行和句号处断开；请求中同时携带的自定义分隔符、长度、重叠和预处理参数全部忽略。
- `custom`：最大长度 100～2000 字、重叠 0%～50%，可选择换行、空行或句号，并可移除 URL/邮箱、折叠连续空白。
- `hierarchy`：按 Markdown `#`、`##`、`###` 分章，标题保留在章节内；超长章节再按窗口切分。

切分输出统一包含 `chunk_index`、`start_offset`、`end_offset` 和 `chunk_text`。预处理仅影响待切分文本，不能覆盖 `knowledge_entries` 中的原文。

备选方案是仅采用固定窗口，逻辑更少但无法验证不同资料结构下的切分取舍，因此保留三种策略。

### Decision 6：上传原文成功与检索索引成功分开

材料和原文仍按迭代 1 的事务保存。随后创建切片并逐条嵌入；嵌入失败时保留材料和切片，将 `index_status` 标为 `failed`，不写入残缺向量。列表可显示索引状态，但失败材料不会进入向量结果。

备选方案是索引失败时回滚整个上传。模型网关的瞬时故障不应导致原始教学材料丢失，因此采用可重建的索引状态。

### Decision 7：重建索引是教师的显式操作

已有材料不会因页面切换策略而自动变化。教师调用重建接口后，服务端删除该材料的旧向量与旧切片，再按新策略生成。操作只允许处理当前会话班级中的材料。

备选方案是后台自动重建全部材料，成本与失败范围不可控，且不利于判断哪种策略产生了当前结果，因此不采用。

### Decision 8：无命中不调用生成模型

`POST /api/ask` 只以请求中的最新 `q` 对本班执行混合检索并取前 4 条切片。若无候选，直接返回 HTTP 200、`status = no_evidence`、`answer = 资料中未找到相关内容`、空 `citations`。只有存在切片时才把标题、序号、正文、最新问题及清理后的既往 `user` / `assistant` 消息交给生成模型，以支持连续追问。

客户端传入的 `system` 消息被丢弃；服务端自行构造限制性 system prompt。备选方案是让模型在无依据时自行回答“不知道”，提示约束可能失效，因此不采用。

### Decision 9：依赖故障按模式隔离

Qdrant 不可用时，`keyword` 仍可从 MySQL 返回结果；`vector` 与 `hybrid` 返回 503。嵌入网关不可用时行为相同。数据库不可用时所有检索模式返回 503。生成网关不可用时，已完成的 `/api/search` 不受影响；有检索依据的 `/api/ask` 返回 503，且不得返回半成品或伪造回答。任何路径都不得伪造向量分数或静默改成另一种模式。

备选方案是混合检索自动降级为关键字。无提示降级会让客户端误判当前模式与分数含义，因此本迭代返回明确错误。

## Architecture

```text
Browser
   |
   v
Nginx web
   |
   v
Go API ── auth/session ── retrieval
   |                         | \
   |                         |  └── embedding gateway
   |                         |
   v                         v
MySQL                    Qdrant
materials                campusclaw_chunks
knowledge_entries        vector + identifiers only
knowledge_chunks
```

浏览器仍只访问 Nginx 的 `/api`。Qdrant 和模型网关仅允许 Go API 访问，不映射宿主端口。

## Data Model

### `knowledge_chunks`

| 字段 | 含义 |
|---|---|
| `id` | 切片主键，同时作为 Qdrant point ID |
| `knowledge_entry_id` | 原文知识库记录 |
| `material_id` | 来源材料 |
| `class_id` | 班级隔离字段 |
| `chunk_index` | 从 0 开始的切片序号 |
| `start_offset` / `end_offset` | 在待切分文本中的字符区间 |
| `chunk_text` | 事实正文与结果摘录来源 |
| `chunk_strategy` | `auto` / `custom` / `hierarchy` |
| `index_status` | `pending` / `ready` / `failed` |
| `embedding_error` | 失败原因，仅服务端诊断 |
| `created_at` / `updated_at` | 时间 |

约束：

- `(knowledge_entry_id, chunk_index)` 唯一。
- 关键字检索只查询 `index_status = ready`。
- 中文全文索引使用 ngram parser，token size 为 2。

### Qdrant payload

```json
{
  "class_id": 1,
  "material_id": 8,
  "knowledge_entry_id": 8,
  "chunk_id": 42,
  "chunk_index": 0
}
```

payload 不含 `chunk_text`、材料正文、文件路径或用户信息。

## API

### `POST /api/search`

请求：

```json
{
  "q": "自然语言问题",
  "mode": "hybrid",
  "limit": 10,
  "offset": 0,
  "class_id": 999
}
```

`class_id` 被忽略。`q` 为空返回 400；`mode` 非法返回 400；`limit` 在服务端限制范围内截断。

成功响应中的每条 `hit`：

```json
{
  "chunk_id": 42,
  "material_id": 8,
  "material_title": "课堂规范",
  "chunk_index": 0,
  "start_offset": 0,
  "end_offset": 218,
  "snippet": "……",
  "score_keyword": 4.21,
  "score_vector": 0.78,
  "rank_keyword": 1,
  "rank_vector": 2,
  "score_final": 0.0325
}
```

`keyword` 模式不生成 `query_vector`；`vector` 与 `hybrid` MUST 返回问句向量的维度、范数与有限预览，便于核对问句确实经过嵌入且维度与 collection 一致。

### `POST /api/ask`

请求：

```json
{
  "q": "本班材料中的问题",
  "history": []
}
```

有依据时返回简短回答和 citations；引用序号与回答 `[1]`、`[2]` 对齐。无依据时返回：

```json
{
  "status": "no_evidence",
  "answer": "资料中未找到相关内容",
  "citations": []
}
```

### `POST /api/materials/{id}/reindex`

仅教师可用，且材料必须属于当前班级。请求包含切分策略参数。成功返回 `chunk_count`、`chunk_strategy`；学生返回 403，跨班或不存在返回 404。

### `GET /api/materials/{id}/vectors`

仅教师可查看本班材料的切片与向量关联，用于验收 MySQL 主键、Qdrant point ID 和 payload `chunk_id` 是否一致。响应不返回其他班级信息和服务端密钥。

## Retrieval Algorithms

### Keyword

MySQL 在当前班级、`ready` 状态切片中执行全文查询，按全文相关度降序，返回数量受 `limit + offset` 和内部候选上限约束。不调用嵌入网关和 Qdrant。

### Vector

服务端对问句生成与 collection 维度一致的向量；Qdrant 查询附加当前班级过滤，取回候选后丢弃余弦小于 `0.35` 的记录，再以相同班级回 MySQL读取正文。

### Hybrid

分别取得已过滤的关键字和向量排名。对每个切片计算：

```text
score_final =
  (存在关键字排名 ? 1 / (60 + rank_keyword) : 0)
  + (存在向量排名 ? 1 / (60 + rank_vector) : 0)
```

按 `score_final` 降序；稳定排序以最佳单路名次、`chunk_id` 作为后续条件。

## Configuration

新增服务端变量：

- `QDRANT_URL`
- `QDRANT_COLLECTION=campusclaw_chunks`
- `EMBEDDING_BASE_URL`
- `EMBEDDING_API_KEY`
- `EMBEDDING_MODEL`
- `EMBEDDING_DIMENSIONS`
- `CHAT_BASE_URL`
- `CHAT_API_KEY`
- `CHAT_MODEL`
- `VECTOR_SCORE_THRESHOLD=0.35`
- `RRF_K=60`

`.env.example` 只提供占位值，真实密钥不得入库或进入前端构建产物。

## Verification Strategy

- OpenSpec：严格校验四件套和所有 Scenario。
- 切分：同一材料分别以三种策略重建，核对条数、序号、偏移和旧索引删除。
- 数据关联：抽查 MySQL 切片 ID、Qdrant point ID、payload `chunk_id` 三者相同。
- 模式差异：原词验证 keyword；同义改写验证 vector；交叉内容验证 hybrid RRF。
- 隔离：A 班查询 B 班专属词，以及伪造 `class_id`，均 200 且无命中。
- 无依据：天气问题返回固定文案和空引用，并证明未调用生成模型。
- 故障：停止 Qdrant，keyword 正常，vector/hybrid 为 503。
- 安全：浏览器无法直连 Qdrant，仓库和前端均无模型密钥。
