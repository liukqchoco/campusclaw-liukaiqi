# CampusClaw 个人 AI 应用验证平台

CampusClaw 是面向班级教学场景的个人 AI 应用平台。它把多个 AI 应用、提示词模板、班级知识库、内置与上传 Skill、外部工具和 MCP 接入同一条真实对话链路，并保存脱敏的运行轨迹，用于验证发送给 LLM 的具体内容。

## 阶段验收功能

| 教师要求 | 本项目实现 |
|---|---|
| 内置多个 AI 应用 | 预置“科学知识助手”“解题引导助手”“作业反馈助手”，每个应用独立绑定模板、材料与 Skill |
| 支持提示词模板 | 教师可维护模板；服务端渲染班级、用户和应用变量后放入 LLM `system` 消息 |
| 支持知识库检索 | 正文切片存 MySQL、2048 维向量存 Qdrant；关键字与向量排名经 RRF 融合，只发送 Top-4 命中切片和可追溯出处 |
| 内置 Skill 并支持上传 | 预置三项 Skill；教师可上传第 5 课规定格式的 `.txt` / `.md` Skill 文件 |
| 外部工具及 MCP 接入 | 同仓 MCP Server 提供 `formula_hint` 和 `unit_convert`，宿主实现握手、发现、最小候选集、模型决策、参数校验、执行和审计 |

平台不提供伪造的本地“AI 回答”作为降级结果。真实 LLM 未配置时，对话接口明确返回 503；完成验收必须填写课程网关的真实地址、密钥和模型名。

## 课程链路

```text
登录与班级隔离（第 3 课）
  → 班级材料入库与可追溯检索（第 4 课）
  → AI 应用、提示词、Skill 路由与 SSE 对话（第 5 课）
  → MCP 连接、工具发现、模型决策、宿主执行与调用记录（第 6 课）
  → LLM 请求 / 返回 / 检索 / Skill / MCP 验收轨迹
```

## 系统架构

```text
浏览器
  │  Bearer Token + SSE
  ▼
Nginx Web :8080
  │
  ▼
Go API :8081
  ├─ MySQL：正文切片、应用、模板、Skill、会话、轨迹和审计
  ├─ Qdrant：2048 维向量与最小关联标识
  ├─ 课程网关：course-embedding + course-chat
  └─ MCP Host
       │ JSON-RPC 2.0 + Bearer Token
       ▼
     MCP Server :8090
       ├─ formula_hint
       └─ unit_convert
```

只有 Web 端口映射到宿主机。API、MySQL、Qdrant 与 MCP Server 只在 Compose 内部网络通信，课程网关 API Key 和 MCP Token 均只存在于服务端环境变量。

## 从零启动

### 1. 环境要求

- Docker Engine 或 Docker Desktop
- Docker Compose v2
- 一个课程网关 API Key 和可用模型名

### 2. 配置

```bash
cp .env.example .env
```

至少替换以下值：

- `MYSQL_ROOT_PASSWORD`
- `DB_PASSWORD`
- `SESSION_SECRET`
- `JWT_SECRET`
- `SEED_TEACHER_PASSWORD`
- `SEED_STUDENT_PASSWORD`
- `LLM_API_KEY`
- `LLM_MODEL`
- `EMBEDDING_API_KEY`
- `EMBEDDING_MODEL`
- `EMBEDDING_DIM`
- `MCP_AUTH_TOKEN`

模型配置示例：

```dotenv
LLM_BASE_URL=https://ai-gateway.devops.hello1023.com/v1
LLM_API_KEY=仅填写在本机
LLM_MODEL=course-chat
LLM_TIMEOUT_SECONDS=90
EMBEDDING_BASE_URL=https://ai-gateway.devops.hello1023.com/v1
EMBEDDING_API_KEY=仅填写在本机
EMBEDDING_MODEL=course-embedding
EMBEDDING_DIM=2048
QDRANT_URL=http://qdrant:6333
QDRANT_COLLECTION=campusclaw_chunks
VECTOR_SCORE_THRESHOLD=0.35
RRF_K=60
```

`.env` 已被 Git 和 Docker 构建上下文排除。不得把密钥写入代码、前端、截图、报告或版本历史。
`EMBEDDING_DIM` 只用于校验返回向量，向量请求本身只发送 `model` 和 `input`。

### 3. 启动

```bash
docker compose up --build -d
docker compose ps
```

服务均为 healthy 后访问：

- 平台：<http://localhost:8080>
- 健康检查：<http://localhost:8080/health>

### 4. 预置账号

| 用户名 | 角色 | 班级 | 密码来源 |
|---|---|---|---|
| `teacher_a` | 教师 | 高一（1）班 | `.env` 的 `SEED_TEACHER_PASSWORD` |
| `student_a1` | 学生 | 高一（1）班 | `.env` 的 `SEED_STUDENT_PASSWORD` |
| `student_b1` | 学生 | 高一（2）班 | `.env` 的 `SEED_STUDENT_PASSWORD` |

## 验收操作

### 1. 多个 AI 应用与提示词

1. 教师登录并打开“AI 应用”。
2. 查看三个内置应用及其不同模板、材料和 Skill 绑定。
3. 修改一个模板并保存。
4. 使用对应应用完成对话。
5. 在“调用验证”中查看原模板、变量、渲染结果和真实 LLM `messages`。

### 2. 知识库传递范围

1. 在“知识库”中确认存在多份材料。
2. 使用“科学知识助手”提问“磁铁能吸木块吗？”。
3. 打开本次调用轨迹。
4. 核对 `method: hybrid_rrf`、`qdrant_collection: campusclaw_chunks`、`embedding_model: course-embedding`、两路排名、`chunks_sent` 和 `full_knowledge_base_sent: false`。
5. 在实际 LLM 请求中确认用户消息只包含编号后的命中切片，不包含全部知识库。

### 3. 内置与上传 Skill

示例文件位于：

```text
examples/skills/单位换算检查.txt
```

课程格式：

```text
名称：单位换算检查
适用条件：单位,换算,厘米,米,千米
资料：
工具：unit_convert@campus-tools
启用：是

先让学生明确原单位、目标单位和量纲，再调用单位换算工具核对换算关系……
```

先在“MCP 工具”页面连接 `campus-tools`，再在“Skill 技能”页面上传。服务端会拒绝缺字段、重复字段、空正文、超长内容和未发现的工具引用。

一次消息的 Skill 执行逻辑：

1. 读取当前应用绑定且启用的 Skill；
2. 将适用条件切词，与问题做不区分大小写的词法匹配；
3. 取最高分的一条，同分取编号较小者，零命中不选；
4. 把被选 Skill 的完整说明正文放入系统消息；
5. 元数据用于路由、资料过滤和工具候选集，不把整个 Skill 原文件传给 LLM。

### 4. MCP 工具

1. 教师进入“MCP 工具”，点击 `campus-tools` 的“连接并发现”。
2. 宿主依次调用 `initialize` 与 `tools/list`。
3. 页面应显示 `formula_hint@campus-tools` 和 `unit_convert@campus-tools`。
4. 使用“解题引导助手”询问“250 厘米等于多少米，请帮我检查换算”。
5. LLM 收到当前 Skill 允许的 `tools` 声明并决定调用。
6. 宿主校验参数后携带服务端 Token 执行 `tools/call`，再把结果作为 `tool` 消息回传 LLM。
7. “调用记录”和“调用验证”页面能够查看工具、状态、轮次、参数和结果；Token 始终不可见。

## 教师指定的五个验证问题

1. **每次对话传给 LLM 什么？**  
   轨迹保存模型名、`messages`、`tools`、`temperature`、`stream` 与真实返回。模板渲染结果在第一条 `system` 消息中。

2. **是否传递知识库全部内容？**  
   否。只在当前应用绑定的本班材料中执行关键字与 Qdrant 向量混合检索，并传递最多 4 个命中切片。

3. **是否传递 Skill 全部内容？**  
   不传整个原文件。只在该 Skill 被路由选中时，完整传递其说明正文；名称、适用条件、资料范围和工具字段由宿主执行。

4. **Skill 如何执行？**  
   绑定与启用过滤 → 词法计分 → 至多选择一个 → 限定检索范围和工具候选集 → 将说明正文注入系统消息。

5. **MCP 接入后传给 LLM 什么？**  
   首次工具决策请求的 `tools` 中包含名称、说明和 JSON Schema；模型返回工具名及 JSON 参数。MCP URL、Bearer Token 和 JSON-RPC 认证信息不传给 LLM。

上述答案不是静态页面文案；“调用验证”页面会同时展示对应的真实请求、返回与执行事件。

## 主要接口

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/login` | 登录并签发 Bearer Token |
| `GET/POST` | `/api/apps` | 查询或创建 AI 应用 |
| `PUT` | `/api/apps/{id}` | 修改模板及材料、Skill 绑定 |
| `GET/PUT` | `/api/prompt-templates`、`/api/prompt-templates/{id}` | 提示词模板 |
| `GET` | `/api/materials` | 本班知识库材料 |
| `POST` | `/api/materials` | 教师上传材料 |
| `GET` | `/api/skills` | 内置与上传 Skill |
| `POST` | `/api/skills/upload` | 上传并解析 Skill |
| `GET/POST` | `/api/conversations` | 会话列表与创建 |
| `POST` | `/api/conversations/{id}/messages` | SSE 对话 |
| `GET/POST` | `/api/mcp/servers` | MCP Server 查询与登记 |
| `POST` | `/api/mcp/servers/{id}/connect` | 握手并发现工具 |
| `POST` | `/api/mcp/servers/{id}/disconnect` | 断开并清空工具 |
| `GET` | `/api/mcp/tools` | 已发现工具 |
| `GET` | `/api/mcp/logs` | MCP 调用记录 |
| `GET` | `/api/traces`、`/api/traces/{id}` | 脱敏 LLM 验收轨迹 |

## 测试

```bash
cd backend
go test ./...

cd ../mcp-server
go test ./...

cd ../frontend
npm ci
npm run build
```

严格规约校验：

```bash
npx --yes @fission-ai/openspec@latest validate add-ai-platform-stage-check --strict
```

本次 OpenSpec 位于：

```text
openspec/changes/add-ai-platform-stage-check/
├── proposal.md
├── design.md
├── tasks.md
└── specs/ai-platform/spec.md
```

阶段验收报告 Markdown 位于：

```text
作业提交材料/2026-10-28/个人项目阶段性验收/个人项目阶段性验收报告.md
```
