## 1. 仓库、规则与配置

- [ ] 1.1 完成 README 三行说明、项目规则、OpenSpec 四件套 — verify: `test -f README.md && test -f AGENTS.md && openspec status --change add-auth-rbac-class-knowledge`
- [ ] 1.2 建立 Go、React、Nginx 与 Compose 目录骨架 — verify: `test -f backend/go.mod && test -f frontend/package.json && test -f frontend/nginx.conf && test -f docker-compose.yml`
- [ ] 1.3 实现严格的环境变量配置加载，缺少密钥或数据库配置时退出 — verify: 在缺少 `SESSION_SECRET` 的环境启动 API，进程非零退出且输出缺失配置错误
- [ ] 1.4 添加 `.env.example`、`.gitignore`、`.dockerignore`，排除真实配置和上传文件 — verify: 文件存在，且忽略规则包含 `.env`、`uploads/`、数据库数据与前端构建目录

## 2. 数据模型与种子

- [ ] 2.1 创建班级、用户、会话、登录失败、讲义、作业、助手、技能、材料、知识库表 — verify: 全新 MySQL 启动后查询 `information_schema.tables` 可见全部表
- [ ] 2.2 预置 A/B 班、教师 A、学生 A1/B1 与两班可区分材料 — verify: 查询 `users` 与 `materials`，账号与 A/B 班材料标题均存在
- [ ] 2.3 使用 bcrypt 保存预置口令 — verify: 查询 `users.password_hash`，值为 bcrypt 哈希且不等于环境变量中的明文口令

## 3. 登录、会话与限流

- [ ] 3.1 实现 `POST /api/login`，成功换发服务端会话和安全 Cookie — verify: 正确口令返回 200，响应含 `HttpOnly; SameSite=Lax`，随后 `/api/me` 返回 user、role、class
- [ ] 3.2 实现鉴权中间件、`GET /api/me` 与 `POST /api/logout` — verify: 未登录 `/api/me` 返回 401；登出后使用旧 Cookie 再请求仍返回 401
- [ ] 3.3 实现用户名与 IP 组合的失败限流和同形错误 — verify: 不存在用户、错误口令和触发限流三种请求均返回相同状态码与 JSON 正文

## 4. 班级隔离

- [ ] 4.1 实现按会话班级过滤的材料列表 — verify: A 班账号请求列表只出现 A 班标题，B 班账号只出现 B 班标题
- [ ] 4.2 实现详情查询的 `id + class_id` 双条件与跨班 404 — verify: A 班 Cookie 请求 B 班材料 ID 返回 404，响应不含 B 班标题或正文
- [ ] 4.3 忽略客户端伪造的角色和班级字段 — verify: A 班账号附带 B 班 `class_id` 后仍只看见 A 班数据

## 5. 角色授权、上传与下载

- [ ] 5.1 实现教师专属 `POST /api/materials` 与学生 403 — verify: 学生上传合法文件返回 403，数据库行数与上传目录文件数均不变
- [ ] 5.2 实现 `.txt` / `.md`、大小、非空 UTF-8 白名单校验和服务端存储名 — verify: 合法文件通过；`.exe`、`.md.exe`、空文件、非法 UTF-8 返回 400，超限返回 413，均无残留
- [ ] 5.3 实现文件落盘及 `materials`、`knowledge_entries` 同事务写入和失败清理 — verify: 教师上传后两表各增一行且归属会话班级；制造写库失败后两表和目录均无新增
- [ ] 5.4 实现鉴权详情与文件下载，不暴露内部路径 — verify: 本班用户可下载；跨班文件为 404；详情 JSON 无存储名和绝对路径；`/uploads/...` 不可访问

## 6. 前端页面

- [ ] 6.1 实现登录页、统一错误提示、登出和 `/api/me` 身份恢复 — verify: 教师与学生均可登录；刷新页面保持身份；登出后返回登录页
- [ ] 6.2 实现本班材料列表、搜索、详情和安全 Markdown 展示 — verify: 两班各自只见本班数据；含脚本标签的 Markdown 不执行脚本
- [ ] 6.3 实现教师上传进度和学生只读界面 — verify: 教师上传后列表新增；学生无上传入口且直接调用接口仍为 403
- [ ] 6.4 完成北大红视觉、深浅色、列表/网格切换、toast、响应式布局 — verify: 桌面与 390px 宽度均可操作，主题与视图切换有效

## 7. Docker Compose 与文档

- [ ] 7.1 构建 web、api、db 三服务且仅 web 映射宿主端口 — verify: `docker compose config` 显示 api/db 无宿主端口，`docker compose up --build -d` 后三服务运行
- [ ] 7.2 配置数据库与上传 volume、数据库就绪等待 — verify: 全新启动无持续 502；上传后 `down` 再 `up`，记录与文件仍存在
- [ ] 7.3 实现无需登录且不查数据库的 `GET /health` — verify: 未登录请求返回 200；停止 db 后 `/health` 仍为 200，材料接口返回 503
- [ ] 7.4 扩充 README 的配置、启动、账号、接口、范围和设计决策 — verify: 第三方只读 README 能从零启动，并能回答单实例边界与跨班返回码

## 8. 规约与行为验收

- [ ] 8.1 运行 OpenSpec 严格结构校验 — verify: `openspec validate add-auth-rbac-class-knowledge --strict` 退出码为 0 且无 error
- [ ] 8.2 验收主路径：教师登录上传、本班列表和知识库可查、学生查看下载 — verify: 按顺序运行并保存 HTTP 输出和页面截图，四步全部通过
- [ ] 8.3 验收失败路径：未登录、学生上传、跨班访问 — verify: 分别得到 401、403、404，响应无受保护数据且数据库和目录无脏数据
- [ ] 8.4 验收仓库与镜像不含真实密钥 — verify: 仓库文件与容器镜像均无 `.env` 和真实密钥，数据库口令列均为 bcrypt 哈希

## 9. 发布与归档

- [ ] 9.1 记录每条关键 Scenario 的通过/失败结论及可复核证据 — verify: `docs/iteration-1-verification.md` 包含运行命令、状态码、关键输出和截图
- [ ] 9.2 在干净环境按 README 复现 Compose 启动 — verify: 仅使用入库文件和新建 `.env` 即可打开登录页并通过 `/health`
- [ ] 9.3 完成 OpenSpec 归档并核对基线 — verify: `openspec list` 无活动 change，`openspec/changes/archive/` 有带日期目录，`openspec/specs/auth-upload/spec.md` 存在
- [ ] 9.4 由提交人亲自创建版本提交、tag 和远程仓库 — verify: 提交历史作者只有提交人，远程公开仓库可按 README 复现
