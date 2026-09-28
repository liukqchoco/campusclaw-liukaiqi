## ADDED Requirements

### Requirement: 班级范围内的知识库检索

系统 MUST 允许已登录教师和学生检索其当前班级中已就绪的知识切片。检索班级 MUST 仅来自服务端 Session；请求 query、JSON 或 header 中的 `class_id` MUST 被忽略。

#### Scenario: 教师与学生均可检索本班

- **WHEN** A 班教师或学生以有效会话提交同一条本班材料查询
- **THEN** 两种角色 MUST 均能收到 A 班的有效命中
- **AND** 上传权限仍 MUST 只属于教师

#### Scenario: 伪造班级字段无效

- **WHEN** A 班用户在检索请求中提交 B 班 `class_id`
- **THEN** 服务端 MUST 仍按 A 班执行检索
- **AND** 响应 MUST 不含 B 班材料标题、正文、切片、标识或分数

#### Scenario: 跨班专属词表现为无命中

- **WHEN** A 班用户检索一个只存在于 B 班切片中的词
- **THEN** 响应状态 MUST 为 200
- **AND** `hits` MUST 为空
- **AND** 响应 MUST NOT 使用 403 或 404 暗示 B 班材料存在

#### Scenario: 未登录用户不能检索

- **WHEN** 未携带有效 Session Cookie 调用检索或问答接口
- **THEN** API MUST 返回 401
- **AND** 响应 MUST 不含任何材料或切片数据

### Requirement: 知识正文切片

系统 MUST 将 `knowledge_entries` 中的正文切分为 `knowledge_chunks`。每个切片 MUST 记录班级、来源材料、来源知识记录、切片序号、字符区间、切片正文、策略和索引状态。切片序号 MUST 在同一知识记录内从 0 连续递增。

#### Scenario: 默认自动窗口切分

- **WHEN** 上传或补齐索引时没有指定策略或明确选择 `auto`
- **THEN** 系统 MUST 使用 `auto`
- **AND** 最大窗口 MUST 约为 800 字
- **AND** 相邻窗口 MUST 重叠约 80 字
- **AND** 系统 SHOULD 优先在空行、换行或句号处断开
- **AND** 请求中同时携带的自定义长度、重叠、分隔符及预处理参数 MUST 被忽略

#### Scenario: 自定义切分参数受限

- **WHEN** 教师选择 `custom`
- **THEN** 最大长度 MUST 限制在 100 至 2000 字
- **AND** 重叠比例 MUST 限制在 0% 至 50%
- **AND** 分隔符 MUST 仅允许换行、空行或句号
- **AND** 无可用断点时 MUST 按最大长度强制切分

#### Scenario: Markdown 标题层级切分

- **WHEN** 教师对含 `#`、`##` 或 `###` 标题的材料选择 `hierarchy`
- **THEN** 系统 MUST 按标题层级分章
- **AND** 标题 MUST 保留在对应章节切片中
- **AND** 超长章节 MUST 再按窗口规则切分

#### Scenario: 预处理不改写原文

- **WHEN** 自定义策略启用移除链接或折叠空白
- **THEN** 预处理 MUST 只作用于待切分文本
- **AND** `knowledge_entries` 中的原始正文 MUST 保持不变
- **AND** 切片偏移 MUST 明确表示预处理后文本中的位置

### Requirement: 原文、切片与向量可追溯关联

系统 MUST 将切片正文保存在 MySQL，并将向量保存在 Qdrant collection `campusclaw_chunks`。Qdrant point ID、MySQL `knowledge_chunks.id` 与 payload `chunk_id` MUST 相同。向量 payload MUST NOT 包含正文。

#### Scenario: 两侧主键一致

- **WHEN** 一个切片完成嵌入与向量写入
- **THEN** Qdrant point ID MUST 等于该行 `knowledge_chunks.id`
- **AND** payload `chunk_id` MUST 等于同一 ID
- **AND** payload 的 `class_id`、`material_id`、`knowledge_entry_id`、`chunk_index` MUST 与 MySQL 行一致

#### Scenario: 摘录只取自 MySQL

- **WHEN** 向量检索命中一个 Qdrant point
- **THEN** 服务端 MUST 使用 point ID 和会话班级回 MySQL 查询
- **AND** 结果 `snippet` MUST 来自 `knowledge_chunks.chunk_text`
- **AND** Qdrant payload MUST 不含 `chunk_text`、完整材料正文或磁盘路径

### Requirement: 关键字检索模式

`keyword` 模式 MUST 只查询 MySQL 中当前班级且 `index_status = ready` 的切片，使用适合中文的全文索引并按全文相关度排序。该模式 MUST NOT 调用嵌入网关或 Qdrant。

#### Scenario: 原词关键字命中

- **WHEN** 本班切片包含查询中的原词且用户选择 `keyword`
- **THEN** 匹配切片 MUST 出现在 `hits`
- **AND** `score_keyword` 与 `rank_keyword` MUST 可用于解释排序
- **AND** 响应 MUST 不包含本次生成的 `query_vector`

#### Scenario: Qdrant 故障不影响关键字路径

- **WHEN** Qdrant 不可用而 MySQL 正常
- **THEN** `keyword` 检索 MUST 仍能返回本班正文结果
- **AND** 服务端 MUST NOT 伪造向量分数

### Requirement: 向量检索模式

`vector` 模式 MUST 在服务端把问句转换为与 Qdrant collection 相同维度的向量，使用余弦距离并按 Session 班级过滤。余弦相似度低于 `0.35` 的候选 MUST 被丢弃，命中后 MUST 以同一班级条件回 MySQL 读取正文。`vector` 与 `hybrid` 响应 MUST 提供本次 `query_vector` 的维度、L2 范数与有限分量预览；`keyword` MUST 不生成该向量。

#### Scenario: 同义改写可以语义命中

- **WHEN** 查询未复用原文措辞但语义与本班切片相近
- **THEN** 达到阈值的切片 MAY 由向量路径命中
- **AND** 命中 MUST 提供 `score_vector` 与 `rank_vector`
- **AND** 摘录 MUST 仍来自 MySQL
- **AND** 响应 MUST 提供问句向量维度、L2 范数和有限预览

#### Scenario: 低相似候选不凑数

- **WHEN** 所有向量候选的余弦相似度均低于 `0.35`
- **THEN** `hits` MUST 为空
- **AND** 系统 MUST 返回「资料中未找到相关内容」

#### Scenario: 向量依赖故障

- **WHEN** 嵌入网关或 Qdrant 不可用
- **THEN** `vector` 模式 MUST 返回 503
- **AND** MUST NOT 静默降级、编造向量或返回未验证正文

### Requirement: 混合检索模式

`hybrid` 模式 MUST 分别执行关键字和向量路径，各自先按绝对分数过滤，再使用 RRF、`k = 60` 融合切片名次。缺席某一路的切片 MUST 只计算其实际存在路径的贡献。

#### Scenario: 两路命中优先

- **WHEN** 一个切片同时获得关键字与向量排名，另一个切片只在单路出现
- **THEN** 系统 MUST 按 RRF 公式计算每个候选的 `score_final`
- **AND** 两路均命中的候选 SHOULD 排在相近单路候选之前
- **AND** 响应 MUST 保留各路分数与名次供核对

#### Scenario: 不直接相加异构分数

- **WHEN** 服务端融合全文相关度与余弦相似度
- **THEN** MUST NOT 直接把 `score_keyword` 与 `score_vector` 相加
- **AND** `score_final` MUST 只由过滤后的名次按照 RRF 计算

#### Scenario: 混合依赖故障

- **WHEN** Qdrant 或嵌入网关不可用
- **THEN** `hybrid` 模式 MUST 返回 503
- **AND** MUST NOT 在未告知调用方的情况下改为 `keyword`

### Requirement: 可追溯检索结果

每条检索结果 MUST 至少返回材料标识、材料标题、切片标识、切片序号、字符区间和摘录，并支持用户打开其有权访问的原材料。结果还 MUST 返回与所选模式一致的分数、名次或融合解释。

#### Scenario: 命中结果可定位原文

- **WHEN** 检索返回一个切片
- **THEN** 响应 MUST 包含 `material_id`、`material_title`、`chunk_id`、`chunk_index`
- **AND** MUST 包含 `start_offset`、`end_offset` 与取自 MySQL 的 `snippet`
- **AND** 用户 MUST 能通过 `material_id` 打开本班材料详情

#### Scenario: 跨班材料链接仍受原有规则保护

- **WHEN** A 班用户篡改结果链接并打开 B 班材料 ID
- **THEN** 材料详情 MUST 返回 404
- **AND** 响应 MUST 不含 B 班标题、正文或路径

### Requirement: 无依据处理

任一检索模式经过其规定的过滤后没有候选时，系统 MUST 返回 HTTP 200、`status = no_evidence`、固定文案「资料中未找到相关内容」和空 `hits`。系统 MUST NOT 以弱相关候选凑数。

#### Scenario: 与资料无关的问题

- **WHEN** 用户检索天气、比分或其他本班资料不存在的内容
- **THEN** 响应 MUST 为 200
- **AND** `hits` MUST 为空
- **AND** `message` MUST 为「资料中未找到相关内容」

#### Scenario: 空查询被拒绝

- **WHEN** `q` 为空或仅包含空白
- **THEN** 检索接口 MUST 返回 400
- **AND** MUST NOT 调用数据库全文检索、嵌入网关或 Qdrant

### Requirement: 有引用的简短回答

系统 MUST 提供问答接口，并只使用最新问题在当前会话班级执行混合检索。只有取得至少一个切片后，系统才可调用生成模型。此前的 `user` / `assistant` 历史 MAY 在清理后提供给生成模型以支持连续追问，但 MUST NOT 参与本轮检索。回答中的 `[1]`、`[2]` MUST 与 `citations` 顺序一一对应。

#### Scenario: 有依据时生成引用回答

- **WHEN** 本班混合检索取得一个或多个有效切片
- **THEN** 问答模块 MUST 最多取前 4 条作为依据
- **AND** 生成模型 MUST 接收材料标题、切片序号、切片正文、本轮问题及清理后的 `user` / `assistant` 历史
- **AND** 回答中的引用编号 MUST 与 citations 顺序一致
- **AND** 每个 citation MUST 包含材料、切片、偏移和摘录

#### Scenario: 只有最新问题用于检索

- **WHEN** 请求 history 含此前对话且本轮提交新的 `q`
- **THEN** 检索模块 MUST 只使用最新 `q` 生成关键字与问句向量
- **AND** 历史内容 MUST NOT 扩大本轮候选集合

#### Scenario: 无依据时不调用生成模型

- **WHEN** 混合检索没有有效切片
- **THEN** 服务端 MUST 直接返回 `answer = 资料中未找到相关内容`
- **AND** `citations` MUST 为空
- **AND** 生成模型 MUST NOT 被调用

#### Scenario: 客户端 system 消息被丢弃

- **WHEN** 请求历史中包含客户端构造的 `system` 消息
- **THEN** 服务端 MUST 丢弃该消息
- **AND** 生成模型使用的 system prompt MUST 由服务端构造

#### Scenario: 生成网关故障

- **WHEN** 本班检索已有有效切片但生成网关不可用
- **THEN** 问答接口 MUST 返回 503
- **AND** MUST NOT 返回半成品、无引用回答或伪造 citations
- **AND** 独立的检索接口 MUST 仍可按其依赖状态正常工作

### Requirement: 索引失败与显式重建

原始材料入库成功后，检索索引失败 MUST NOT 删除材料或 `knowledge_entries` 原文。失败切片 MUST 标记为 `failed` 且不得写入不完整向量。教师 MUST 能对本班材料选择新策略并显式重建索引。

#### Scenario: 嵌入失败保留原文

- **WHEN** 材料原文已保存但某切片嵌入失败
- **THEN** 材料与 `knowledge_entries` MUST 保留
- **AND** 失败切片 MUST 标记为 `failed`
- **AND** Qdrant MUST 不含该切片的不完整 point

#### Scenario: 教师重建索引

- **WHEN** 教师对本班材料提交合法的新切分策略
- **THEN** 系统 MUST 删除该材料旧切片及对应旧向量
- **AND** MUST 按新策略创建新切片与向量
- **AND** 响应 MUST 返回新 `chunk_count` 与 `chunk_strategy`

#### Scenario: 学生或跨班重建被拒绝

- **WHEN** 学生请求重建索引
- **THEN** API MUST 返回 403
- **AND** 旧切片与向量 MUST 不变
- **WHEN** 教师请求重建其他班级材料
- **THEN** API MUST 返回 404
- **AND** MUST 不泄露目标材料是否存在

### Requirement: 向量服务与密钥不对外暴露

Qdrant、嵌入网关和生成网关 MUST 仅由服务端检索模块访问。Compose MUST NOT 将 Qdrant 端口映射到宿主机。所有模型密钥 MUST 仅来自服务端环境变量，MUST NOT 进入浏览器、源码或版本库。

#### Scenario: 浏览器只能访问本站 API

- **WHEN** 系统通过 Compose 运行
- **THEN** 只有 web 服务 MUST 映射宿主端口
- **AND** 浏览器 MUST NOT 能直接连接 Qdrant 或模型网关
- **AND** 前端响应与构建产物 MUST 不含模型 API key

#### Scenario: 配置示例不含真实密钥

- **WHEN** 第三方检查公开仓库
- **THEN** `.env.example` MUST 只包含占位说明
- **AND** 仓库历史 MUST 不含可用的嵌入或生成模型密钥
