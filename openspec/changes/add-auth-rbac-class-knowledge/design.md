## Context

迭代 1 需要在单机上交付一个可复现的教研材料系统。系统面对两个主要风险：客户端伪造角色或班级，以及上传过程留下数据库孤立记录或无归属文件。因此，身份与租户信息必须由服务端掌握，上传过程必须能够整体成功或整体清理。

## Goals

- 第三方仅依赖 README 和 Docker Compose 即可启动完整系统。
- 教师与学生使用服务端会话登录，所有权限判断发生在 API。
- 班级隔离覆盖列表、详情、原文件下载和上传数据归属。
- 上传成功后材料与知识库正文同时可查，失败后不留下脏数据。
- 留下能够复核主路径和失败路径的运行证据。

## Non-goals

本设计不覆盖 RAG 问答、开放 API、移动端认证、管理员后台、多副本部署与生产级对象存储。

## Decisions

### Decision 1：浏览器只访问统一入口

浏览器仅访问 Nginx 暴露的 web 端口。Nginx 提供前端静态文件，并把 `/api` 与 `/health` 转发给 Go 服务；Go 再访问 MySQL。API 和数据库不映射宿主端口，上传目录也不作为静态目录暴露。

备选方案是分别暴露 Vite、Go 和 MySQL 端口，调试更直接，但会扩大攻击面并制造跨域 Cookie 差异，因此不采用。

### Decision 2：使用服务端 Session Cookie

登录成功后生成不可预测的随机会话 ID，只把该 ID 放进 `HttpOnly`、`SameSite=Lax` Cookie。会话记录保存在 MySQL；每次请求通过会话关联用户表，取得 `user_id`、`role`、`class_id`。登录会换发会话 ID，登出删除服务端记录并清除 Cookie。

备选方案是 JWT。当前系统是同源浏览器应用，JWT 会增加前端令牌存储、主动失效和权限声明过期问题；服务端 Session 能即时登出且不把角色和班级交给客户端，所以不采用 JWT。

### Decision 3：班级只来自服务端，跨班统一返回 404

材料归属班级来自已认证用户的 `class_id`。请求参数、表单字段和前端状态都不能覆盖它。列表 SQL 必须带班级条件，详情和文件查询同时按 `id` 与 `class_id` 查询。跨班请求与不存在记录使用相同的 404 响应。

备选方案是跨班返回 403。403 更直观，却会确认某个材料 ID 确实存在；404 减少资源枚举信息，因此采用 404，并要求 README、规约和实现保持一致。

### Decision 4：上传采用白名单和补偿清理

上传只接受非空 UTF-8 的 `.txt`、`.md`，大小上限来自环境变量。服务端生成随机存储名，原文件名只作展示标题。文件通过校验后落盘，随后在一个数据库事务中写入 `materials` 与 `knowledge_entries`；数据库失败时回滚事务并删除刚写入的文件。

备选方案是先写一张表、再异步解析另一张表。它会引入中间状态和重试队列，超出本次单实例迭代范围，因此采用同步事务加文件补偿清理。

### Decision 5：`/health` 只表示进程存活

`GET /health` 不要求登录，也不访问数据库。数据库不可用时，依赖数据库的业务接口返回 503；存活探针仍可用于判断 Go 进程是否能响应。

备选方案是在 `/health` 中查询数据库。数据库短暂故障会把正常进程判成死亡并诱发反复重启，因此不把存活与就绪混为同一语义。

### Decision 6：口令与密钥不进入源码

预置账号的口令仅从环境变量读取，使用 bcrypt 哈希后入库；数据库口令和会话密钥同样只来自环境变量。缺少必需配置时 API 启动失败。仓库仅提交 `.env.example`。

备选方案是提供开发默认密钥和明文种子口令，虽然启动方便，但任何获得源码的人都能伪造会话或复用凭据，因此不采用。

## Architecture

```text
Browser
   |
   v
Nginx web :8080
   |-- static React assets
   `-- /api, /health --> Go api :8081
                              |-- MySQL :3306
                              `-- /app/uploads
```

## API

| Method | Path | Authentication | Behavior |
|---|---|---|---|
| POST | `/api/login` | none | Verify credentials and issue a session cookie |
| POST | `/api/logout` | session | Delete the server session and clear the cookie |
| GET | `/api/me` | session | Return current user, role and class |
| GET | `/api/materials` | session | Return only the current class materials |
| GET | `/api/materials/{id}` | session + class | Return metadata and knowledge text; cross-class is 404 |
| GET | `/api/materials/{id}/file` | session + class | Download original file; cross-class is 404 |
| POST | `/api/materials` | teacher session | Validate, store, parse and insert material |
| GET | `/health` | none | Return process liveness |

`POST /api/login` consumes JSON:

```json
{"username":"teacher_a","password":"value from SEED_TEACHER_PASSWORD"}
```

Successful responses use JSON and set a `campusclaw_session` cookie. Credential failure and login throttling both return HTTP 401 with the same body:

```json
{"error":"用户名或密码错误"}
```

## Data Model

- `classes(id, name)`
- `users(id, username, display_name, role, class_id, password_hash)`
- `sessions(id_hash, user_id, expires_at, created_at)`
- `login_attempts(username, ip, failed_count, locked_until, updated_at)`
- `materials(id, class_id, uploader_id, title, original_name, stored_name, media_type, size_bytes, created_at)`
- `knowledge_entries(id, material_id, class_id, content, created_at)`
- `lessons`、`assignments`、`assistants`、`skills`：为后续迭代保留的六类核心数据结构中的其余实体。

## Configuration

API 必需配置：

- `DB_HOST`、`DB_PORT`、`DB_NAME`、`DB_USER`、`DB_PASSWORD`
- `SESSION_SECRET`、`SESSION_TTL_HOURS`
- `UPLOAD_DIR`、`MAX_UPLOAD_BYTES`
- `SEED_TEACHER_PASSWORD`、`SEED_STUDENT_PASSWORD`
- `LOGIN_MAX_FAILURES`、`LOGIN_LOCK_MINUTES`

Compose 只让 web 映射宿主端口。MySQL 数据使用命名卷，上传目录使用命名卷挂载到 API。当前部署固定为单个 API 副本。

## Verification Strategy

- 结构：`openspec validate add-auth-rbac-class-knowledge --strict`。
- 主路径：教师登录、上传、本班列表与知识库正文可查；学生可查看与下载。
- 失败路径：未登录 401、学生上传 403、跨班详情与文件 404、非法扩展名 400。
- 持久化：Compose `down` 后再次 `up`，已上传记录与文件仍在。
- 安全：仓库无 `.env` 和真实密钥；数据库口令字段为 bcrypt 哈希；`/uploads/...` 无法直接访问。
