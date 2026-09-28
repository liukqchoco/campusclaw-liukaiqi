## ADDED Requirements

### Requirement: 用户登录与服务端会话

系统 MUST 允许预置教师和学生使用账号密码登录。口令 MUST 使用 bcrypt 校验；登录成功 MUST 换发新的服务端会话，并通过 `HttpOnly`、`SameSite=Lax` Cookie 返回不透明会话 ID。服务端 MUST 能从会话判定 `user_id`、`role` 与 `class_id`。

#### Scenario: 教师登录成功

- **WHEN** 教师使用正确账号和口令请求 `POST /api/login`
- **THEN** 响应状态 MUST 为 200
- **AND** 响应 MUST 设置 `HttpOnly`、`SameSite=Lax` 的会话 Cookie
- **AND** 随后请求 `GET /api/me` MUST 返回教师身份、`teacher` 角色与所属班级

#### Scenario: 登录失败响应统一

- **WHEN** 用户名不存在、口令错误或账号处于登录限流期
- **THEN** 响应状态 MUST 为 401
- **AND** 三种情况的 JSON 错误正文 MUST 相同
- **AND** 响应 MUST NOT 说明账号是否存在

#### Scenario: 未登录访问受保护接口

- **WHEN** 未携带有效会话 Cookie 请求材料列表、详情或文件
- **THEN** API MUST 返回 401
- **AND** 响应体 MUST NOT 包含材料标题、正文、原文件名或磁盘路径

#### Scenario: 登出使旧会话失效

- **WHEN** 已登录用户调用 `POST /api/logout` 后继续使用原 Cookie 请求 `GET /api/me`
- **THEN** 服务端会话记录 MUST 已删除
- **AND** 后续请求 MUST 返回 401

### Requirement: 角色授权

系统 MUST 在服务端依据会话用户的角色授权。教师可上传、查看和下载本班材料；学生只可查看和下载本班材料。客户端提交的角色声明 MUST 被忽略。

#### Scenario: 学生上传被拒绝

- **WHEN** 有效学生会话向 `POST /api/materials` 提交合法 `.txt` 或 `.md` 文件
- **THEN** 响应状态 MUST 为 403
- **AND** `materials` 与 `knowledge_entries` 行数 MUST 不变
- **AND** 上传目录 MUST 不新增文件

#### Scenario: 教师上传被允许

- **WHEN** 有效教师会话提交符合校验要求的文件
- **THEN** 系统 MUST 接受上传并返回 201
- **AND** 响应 MUST 包含新材料标识

### Requirement: 班级数据隔离

系统 MUST 将班级作为数据边界。业务查询 MUST 使用服务端会话关联用户得到的 `class_id`；客户端 query、body 或 header 中的班级值 MUST NOT 改变查询范围或数据归属。

#### Scenario: 列表只包含本班材料

- **WHEN** A 班用户请求 `GET /api/materials`
- **THEN** 返回列表中的每条材料 MUST 归属 A 班
- **AND** 响应 MUST NOT 包含 B 班材料的标题、正文、原文件名或存储路径

#### Scenario: 跨班详情返回 404

- **WHEN** A 班用户按 ID 请求一条 B 班材料的详情
- **THEN** 响应状态 MUST 为 404
- **AND** 响应 MUST 与请求不存在材料的响应同形
- **AND** 响应体 MUST NOT 暴露 B 班材料是否存在

#### Scenario: 客户端班级参数无效

- **WHEN** A 班用户在列表或上传请求中伪造 B 班 `class_id`
- **THEN** 列表仍 MUST 只返回 A 班数据
- **AND** 教师上传产生的记录仍 MUST 归属 A 班

### Requirement: 材料上传与知识库入库

教师上传材料时，系统 MUST 仅接受 `.txt` 与 `.md` 白名单扩展名、限制请求大小并要求非空 UTF-8 内容。存储名 MUST 由服务端生成。系统 MUST 保存原文件，并在同一数据库事务中写入材料记录与知识库正文记录。

#### Scenario: 上传后知识库与列表可查

- **WHEN** A 班教师上传一份合法 UTF-8 `.md` 文件
- **THEN** 上传目录 MUST 出现一个由服务端命名的文件
- **AND** `materials` MUST 新增一条归属 A 班的记录
- **AND** `knowledge_entries` MUST 新增一条关联该材料且归属 A 班的正文记录
- **AND** A 班材料列表与详情 MUST 能查询到该标题和正文

#### Scenario: 非白名单扩展名被拒绝

- **WHEN** 教师上传 `.exe`、`.pdf` 或伪装为 `.md.exe` 的文件
- **THEN** 响应状态 MUST 为 400
- **AND** 数据库与上传目录 MUST 不发生变化

#### Scenario: 内容非法或为空被拒绝

- **WHEN** 教师上传空文件或不是合法 UTF-8 的 `.txt` / `.md`
- **THEN** 响应状态 MUST 为 400
- **AND** 数据库与上传目录 MUST 不发生变化

#### Scenario: 文件过大被拒绝

- **WHEN** 上传请求超过 `MAX_UPLOAD_BYTES`
- **THEN** 响应状态 MUST 为 413
- **AND** 服务端 MUST 在保留完整超限文件之前终止处理
- **AND** 数据库与上传目录 MUST 不发生变化

#### Scenario: 写库失败后无脏数据

- **WHEN** 文件通过校验并写入磁盘后，任一数据库写入失败
- **THEN** 数据库事务 MUST 回滚
- **AND** 已写入的文件 MUST 被删除
- **AND** 系统 MUST 不留下孤立材料、孤立知识库正文或未关联文件

### Requirement: 材料详情与文件访问

材料详情和原文件下载 MUST 通过鉴权 API 提供。上传目录 MUST NOT 由 Nginx 作为静态资源目录暴露。详情响应 MUST NOT 返回服务器绝对路径或内部存储名。

#### Scenario: 本班用户查看与下载

- **WHEN** 本班教师或学生请求本班材料详情与 `/api/materials/{id}/file`
- **THEN** 详情 MUST 返回标题、原文件名、创建时间和知识库正文
- **AND** 文件接口 MUST 返回原文件内容并使用安全的下载文件名
- **AND** 详情 JSON MUST NOT 包含磁盘绝对路径或内部存储名

#### Scenario: 跨班文件返回 404

- **WHEN** A 班用户请求一条 B 班材料的文件接口
- **THEN** 响应状态 MUST 为 404
- **AND** 响应 MUST NOT 包含文件内容、文件名或路径

#### Scenario: 静态上传路径不可访问

- **WHEN** 用户直接请求 `/uploads/{猜测文件名}`
- **THEN** Nginx MUST NOT 返回上传文件

### Requirement: 预置核心数据

系统 MUST 建立班级、用户、讲义、作业、助手、技能、材料与知识库数据结构，并预置两个班级、一个教师账号、两个不同班级的学生账号和两班可区分的样本材料。

#### Scenario: 双班种子数据可验收

- **WHEN** 全新数据库首次启动完成
- **THEN** MUST 存在 A 班与 B 班
- **AND** MUST 存在 A 班教师、A 班学生和 B 班学生
- **AND** A、B 两班 MUST 各有标题可区分的样本材料
- **AND** 预置用户口令字段 MUST 为 bcrypt 哈希而非明文

### Requirement: 密钥与配置安全

数据库凭据、会话密钥、预置账号口令和上传上限 MUST 从服务端环境变量读取。源码 MUST NOT 包含可用的默认密钥或真实口令；`.env` MUST NOT 入库，`.env.example` MUST 列出复现所需配置。

#### Scenario: 缺少会话密钥时启动失败

- **WHEN** 启动 API 时没有设置 `SESSION_SECRET`
- **THEN** API MUST 报错退出
- **AND** MUST NOT 回退到内置默认密钥

#### Scenario: 仓库不含真实密钥

- **WHEN** 第三方克隆并检查完整仓库
- **THEN** 仓库 MUST 不含 `.env`、真实数据库口令或可用会话密钥
- **AND** `.env.example` MUST 包含全部必需变量的占位说明

### Requirement: Web 页面与身份恢复

系统 MUST 提供登录页和材料页。页面刷新后 MUST 调用 `GET /api/me` 恢复身份；上传入口仅在该接口返回教师角色时显示。前端显示规则 MUST NOT 替代服务端授权。

#### Scenario: 刷新后通过服务端恢复身份

- **WHEN** 已登录用户刷新页面或新开同源标签页
- **THEN** 前端 MUST 通过 `/api/me` 读取当前身份
- **AND** MUST NOT 使用 `localStorage` 中的角色或班级作为授权依据

#### Scenario: 学生界面只读且绕过仍失败

- **WHEN** 学生登录材料页
- **THEN** 页面 MUST 不显示上传表单
- **AND** 学生 MUST 可以查看和下载本班材料
- **AND** 学生绕过页面直接调用上传接口仍 MUST 得到 403

#### Scenario: Markdown 正文安全展示

- **WHEN** 用户查看包含 Markdown 或 HTML 标签的 `.md` 材料
- **THEN** 页面 MUST 格式化展示 Markdown
- **AND** 材料中的脚本 MUST NOT 被执行

### Requirement: Compose 部署与健康检查

系统 MUST 通过 Docker Compose 启动 web、api、db 三个服务。只有 web 映射宿主端口；API、数据库和上传目录不得直接对外暴露。数据库与上传文件 MUST 持久化。`GET /health` MUST 无需登录且仅表示 API 进程存活。

#### Scenario: 第三方按 README 从零启动

- **WHEN** 第三方复制 `.env.example`、填写必需值并执行 README 的 Compose 命令
- **THEN** web、api、db MUST 成功启动
- **AND** 浏览器访问 web 地址 MUST 显示登录页
- **AND** 预置账号 MUST 可以登录

#### Scenario: 仅 web 端口暴露

- **WHEN** Compose 服务全部运行
- **THEN** 宿主机 MUST 只映射 web 端口
- **AND** api 端口与 MySQL 3306 MUST 仅在 Compose 网络内可达

#### Scenario: health 无需会话且不查数据库

- **WHEN** 未登录用户请求 `GET /health`
- **THEN** 响应状态 MUST 为 200
- **AND** 响应 MUST 表示 API 进程存活
- **AND** 停止数据库后该端点仍 MUST 返回 200

#### Scenario: 数据跨容器重启保留

- **WHEN** 已有上传记录后执行 `docker compose down`，随后执行 `docker compose up` 且未删除 volumes
- **THEN** 预置账号 MUST 仍可登录
- **AND** 已上传材料记录、知识库正文与原文件 MUST 仍可访问
