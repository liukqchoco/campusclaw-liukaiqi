# CampusClaw

- **价值主张**：为中小学教师和学生提供有权限边界的教研材料管理与知识库底座。
- **核心场景**：教师上传本班教学材料，教师和学生登录后查看、搜索与下载本班内容。
- **迭代 1 非目标**：当时不做检索问答、AI 对话、作业提交与批改、注册找回密码、SSO 与生产高可用；第四课 change 现单独描述可追溯检索和仅基于证据的简短回答。

## 迭代 1

本迭代交付 CampusClaw 的教研材料底座：账号登录、教师/学生角色权限、班级数据隔离、材料上传、正文入库、列表查询、详情和原文件下载。所有身份、角色和班级判断都在服务端完成。

### 系统链路

```text
浏览器 → Nginx web → Go API → MySQL
                         └→ 上传文件卷
```

只有 Nginx 的 web 端口映射到宿主机。API、MySQL 及上传目录不直接对外暴露；浏览器通过同源 `/api` 访问后端。

## 从零启动

### 环境要求

- Docker Engine 或 Docker Desktop
- Docker Compose v2

### 1. 准备本地配置

```bash
cp .env.example .env
```

编辑 `.env`，至少替换以下占位值：

- `MYSQL_ROOT_PASSWORD`
- `DB_PASSWORD`
- `SESSION_SECRET`（不少于 32 个字符）
- `SEED_TEACHER_PASSWORD`
- `SEED_STUDENT_PASSWORD`

`.env` 已被 Git 和 Docker 构建上下文排除，不得提交。

### 2. 构建并启动

```bash
docker compose up --build -d
docker compose ps
```

首次启动需要下载镜像并初始化 MySQL，通常需要几十秒。三个服务均为 healthy 后访问：

- Web 登录页：<http://localhost:8080>
- 存活检查：<http://localhost:8080/health>

如 8080 被占用，可修改 `.env` 中的 `WEB_PORT`。

### 3. 预置账号

| 用户名 | 角色 | 班级 | 密码来源 |
|---|---|---|---|
| `teacher_a` | 教师 | 高一（1）班 | `.env` 的 `SEED_TEACHER_PASSWORD` |
| `student_a1` | 学生 | 高一（1）班 | `.env` 的 `SEED_STUDENT_PASSWORD` |
| `student_b1` | 学生 | 高一（2）班 | `.env` 的 `SEED_STUDENT_PASSWORD` |

教师可以上传本班材料；学生只能查看和下载本班材料。

### 4. 停止与重新启动

```bash
docker compose down
docker compose up -d
```

上述操作不会删除 MySQL 和上传文件卷，已有数据应当保留。仅在主动重置实验数据时使用：

```bash
docker compose down -v
```

## 上传与下载规则

- 上传接口：`POST /api/materials`，仅教师可用。
- 只允许 `.txt`、`.md`，内容必须为非空 UTF-8 文本。
- 默认大小上限为 1 MiB，可通过 `MAX_UPLOAD_BYTES` 调整。
- 文件存储名由服务端生成；原文件名仅用于展示和下载。
- 材料记录与知识库正文在同一数据库事务中写入，失败时回滚并清理文件。
- 下载通过 `GET /api/materials/{id}/file`；必须先验证会话与班级。
- `/uploads/...` 不作为静态路径开放。

## 认证、授权与隔离

- 登录成功后，服务端创建 Session；浏览器仅持有 `HttpOnly`、`SameSite=Lax` 的不透明 Cookie。
- `user_id`、`role`、`class_id` 均由服务端会话关联用户表取得，不采信请求参数或 `localStorage`。
- 未登录访问受保护 API 返回 401。
- 学生调用上传接口返回 403，且数据库和上传目录均不改变。
- 跨班详情和文件访问统一返回 404，与记录不存在的响应一致。
- 数据库不可用时业务接口返回 503；`/health` 仍只表示 API 进程存活。

## 主要接口

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/login` | 登录并签发服务端会话 |
| `POST` | `/api/logout` | 删除服务端会话并清 Cookie |
| `GET` | `/api/me` | 获取当前用户、角色与班级 |
| `GET` | `/api/materials` | 查询当前班级材料 |
| `GET` | `/api/materials/{id}` | 查看本班材料与知识库正文 |
| `GET` | `/api/materials/{id}/file` | 鉴权下载本班原文件 |
| `POST` | `/api/materials` | 教师上传并入库 |
| `GET` | `/health` | 无需登录的进程存活检查 |

## 快速验收

### 健康检查和未登录失败路径

```bash
curl -i http://localhost:8080/health
curl -i http://localhost:8080/api/materials
```

预期分别为 200 和 401，第二个响应不得含任何材料数据。

### 查看服务状态和日志

```bash
docker compose ps
docker compose logs --no-color api
```

### 检查数据库未暴露

```bash
docker compose config
```

`ports` 只应出现在 `web` 服务；`api` 和 `db` 只能通过 Compose 内部网络访问。

## 设计决策

1. **会话与班级来源**：选择数据库中的服务端 Session，而不是 JWT。这样登出可以立即失效，角色和班级不会落到可被客户端修改的令牌或本地存储中。
2. **跨班使用 404**：403 会确认目标材料存在；404 能让跨班 ID 与不存在 ID 表现一致，减少资源枚举信息。
3. **上传失败无脏数据**：扩展名、大小和编码先校验；文件落盘后，两张业务表在同一事务中写入。写库失败会回滚并删除文件。
4. **`/health` 不查数据库**：它只回答 API 进程是否存活。数据库故障由业务接口用 503 表达，避免数据库抖动触发无意义的 API 重启。

## 部署边界

本迭代按单实例交付。登录失败限流和本地文件存储尚未设计为多 API 副本共享；多副本、对象存储、就绪探针和生产 HTTPS 留待后续迭代。

## OpenSpec

迭代 1 的认证、班级隔离与材料入库规约位于：

```text
openspec/changes/add-auth-rbac-class-knowledge/
├── proposal.md
├── design.md
├── tasks.md
└── specs/auth-upload/spec.md
```

结构校验：

```bash
npx --yes @fission-ai/openspec@latest validate add-auth-rbac-class-knowledge --strict
```

### 第四课：可追溯知识库检索变更

第四课作业要求的 OpenSpec change 已写入：

```text
openspec/changes/add-traceable-vector-retrieval/
├── proposal.md
├── design.md
├── tasks.md
└── specs/knowledge-retrieval/spec.md
```

该 change 描述班级范围内的正文切片、关键字/向量/混合检索、MySQL 与 Qdrant 主键关联、出处标注、无依据处理和引用式回答。它是待实施的变更描述，因此 `tasks.md` 的 checkbox 均保持 `[ ]`。

严格校验：

```bash
npx --yes @fission-ai/openspec@latest validate add-traceable-vector-retrieval --strict
```
