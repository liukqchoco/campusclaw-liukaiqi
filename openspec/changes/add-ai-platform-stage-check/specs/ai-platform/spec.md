## ADDED Requirements

### Requirement: AI 应用与提示词模板

系统 SHALL 在班级范围内提供多个可配置 AI 应用，并在每次对话中渲染和传递该应用绑定的提示词模板。

#### Scenario: 班内存在多个 AI 应用

- **Given** 当前班级已预置至少三个启用的 AI 应用
- **When** 已登录用户查询应用列表
- **Then** 返回应用名称、说明、模板和绑定数量
- **And** 不返回其他班级应用

#### Scenario: 提示词模板参与 LLM 请求

- **Given** 教师为应用选择了含变量的提示词模板
- **When** 用户使用该应用发送消息
- **Then** 服务端渲染模板变量并把完整结果放入 LLM 系统消息
- **And** 验收轨迹同时保存原模板、变量与渲染结果

#### Scenario: 学生不能修改模板

- **Given** 当前用户角色为学生
- **When** 学生调用模板写接口
- **Then** 返回 403
- **And** 模板内容不改变

### Requirement: 知识库检索

系统 SHALL 使用课程向量接口，只在当前应用绑定的本班材料中检索，并只向 LLM 传递有限数量的命中切片。

#### Scenario: 向量请求与维度校验

- **Given** 服务端配置了课程向量模型和预期维度 2048
- **When** 系统为知识切片或查询生成向量
- **Then** `/embeddings` 请求只包含 `model` 与 `input`
- **And** 服务端拒绝维度不是 2048 的响应
- **And** 切片正文保存在 MySQL，Qdrant payload 只保存班级、材料、知识记录和切片标识
- **And** API Key 不进入前端、日志或轨迹

#### Scenario: 只发送 Top-K 命中切片

- **Given** 当前应用绑定了多份本班材料
- **When** 用户提出与其中一份材料相关的问题
- **Then** 服务端过滤余弦低于 0.35 的向量候选
- **And** 关键字排名与 Qdrant 向量排名通过 `k=60` 的 RRF 融合
- **And** 服务端返回至多 4 个切片
- **And** LLM 用户消息只含命中切片而非知识库全部正文
- **And** 引用包含材料、切片与字符区间

#### Scenario: 应用没有绑定材料

- **Given** 当前应用未绑定材料
- **When** 用户发送消息
- **Then** 检索结果为空
- **And** LLM 请求不包含伪造的资料或引用编号

#### Scenario: 跨班材料不进入检索

- **Given** 另一班级材料中含有唯一标记
- **When** 当前用户用该标记提问
- **Then** LLM 请求与回答均不包含该标记

### Requirement: Skill 上传、绑定与执行

系统 SHALL 同时提供内置 Skill 和教师上传 Skill，并以可复现规则为每次消息选择至多一个 Skill。

#### Scenario: 上传合法 Skill

- **Given** 教师上传符合课程格式的 UTF-8 文本文件
- **When** 文件包含名称、适用条件、启用状态和非空说明正文
- **Then** 服务端保存原文及解析字段
- **And** 技能来源标记为上传

#### Scenario: 上传非法 Skill

- **Given** 文件缺少必填字段、重复字段、正文为空或超过长度限制
- **When** 教师提交文件
- **Then** 返回 400 和可读错误
- **And** 不保存任何 Skill 记录

#### Scenario: 一次消息至多选择一个 Skill

- **Given** 应用绑定了多个启用 Skill
- **When** 用户问题命中多个 Skill 的适用条件
- **Then** 服务端按词法得分选择得分最高的一条
- **And** 同分时选择编号较小者
- **And** 只有被选 Skill 的完整说明正文进入系统消息

#### Scenario: 未绑定或停用 Skill 不参与执行

- **Given** 某 Skill 未绑定到当前应用或已停用
- **When** 用户问题命中其适用条件
- **Then** 该 Skill 不进入候选集
- **And** 其说明正文不出现在 LLM 请求中

### Requirement: MCP 工具接入

系统 SHALL 通过受鉴权的 MCP Server 发现和执行外部工具，并由宿主实施最小权限、参数校验、调用上限和审计。

#### Scenario: 连接并发现工具

- **Given** 教师登记了可访问的 MCP Server 和服务端 Token
- **When** 教师执行连接
- **Then** 宿主依次调用 `initialize` 和 `tools/list`
- **And** 两步都成功后整体替换发现清单并标记已连接
- **And** API 响应只返回 `has_token`，不返回 Token

#### Scenario: 模型决定调用已授权工具

- **Given** 被选 Skill 引用了一个已发现且已连接的 MCP 工具
- **When** LLM 返回该工具的名称和参数
- **Then** 宿主按发现的输入结构校验参数
- **And** 携带服务端 Token 调用 `tools/call`
- **And** 将安全检查后的结果作为 `tool` 消息回传 LLM
- **And** 保存成功调用记录

#### Scenario: 模型请求候选集外工具

- **Given** 某工具未被当前 Skill 引用
- **When** 模型要求调用该工具
- **Then** 宿主拒绝执行
- **And** 保存状态为“被拒”的调用记录

#### Scenario: MCP 失败时优雅降级

- **Given** MCP Server 超时、断开或返回错误
- **When** 对话尝试执行工具
- **Then** 工具事件记录失败原因
- **And** 最终回答使用已有信息继续生成或给出可读错误

### Requirement: LLM 与验收轨迹

系统 SHALL 保存不含密钥的真实 LLM 与工具调用轨迹，使教师指定的五项问题均可由运行数据复核。

#### Scenario: 真实记录请求与返回

- **Given** LLM 配置有效
- **When** 用户完成一次对话
- **Then** 轨迹保存实际发送的模型、消息、温度、流式标志和工具声明
- **And** 保存模型返回正文、工具决策与用量信息
- **And** 不保存任何密钥或认证头

#### Scenario: 五项验证可从轨迹复核

- **Given** 对话使用了模板、知识库、Skill 和 MCP 工具
- **When** 有权限用户打开轨迹详情
- **Then** 页面分别展示模板是否传递、知识库传递范围、Skill 传递范围、Skill 执行逻辑和 MCP 参数
- **And** 每项结论均链接到本次实际请求或事件数据

#### Scenario: 轨迹遵守班级与用户边界

- **Given** 当前用户不是轨迹所属班级的教师或触发该轨迹的学生
- **When** 用户按编号读取轨迹
- **Then** 返回 404
- **And** 响应与不存在编号一致
