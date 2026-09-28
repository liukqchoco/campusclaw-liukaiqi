## 1. 规约与配置

- [ ] 1.1 确认迭代 1 的登录、班级隔离、材料入库能力可复用，并严格校验本 change — verify: `npx --yes @fission-ai/openspec@latest validate add-traceable-vector-retrieval --strict` 退出码为 0
- [ ] 1.2 在 `.env.example` 增加 Qdrant、嵌入、生成模型、阈值与 RRF 配置占位，不填真实密钥 — verify: `cd backend && go test ./internal/config -run TestLoadRetrievalConfig` 通过，且项目根执行 `! rg -n 'sk-[A-Za-z0-9_-]{16,}|(EMBEDDING|CHAT)_API_KEY[[:space:]]*=[[:space:]]*[^[:space:]]+' --hidden --glob '!.git/**' --glob '!.env.example'`
- [ ] 1.3 更新 README 的检索范围、三种模式、无依据行为、启动依赖和故障语义 — verify: `rg -q 'keyword.*vector.*hybrid' README.md && rg -q '资料中未找到相关内容' README.md && rg -q 'Qdrant.*503' README.md`
- [ ] 1.4 建立第四课后端/前端测试入口及 `scripts/verify-week04.sh` 场景脚本，再进入后续实现 — verify: `cd backend && go test ./...`、`cd frontend && npm test -- --run`、`bash -n scripts/verify-week04.sh` 均可执行

## 2. 数据模型与迁移

- [ ] 2.1 新增 `knowledge_chunks` 表及班级、材料、知识记录、状态索引和唯一约束 — verify: `cd backend && go test ./internal/store -run TestKnowledgeChunksMigration` 通过，并回到根目录执行 `docker compose exec -T db mysql -e 'SHOW CREATE TABLE knowledge_chunks\\G'`
- [ ] 2.2 为中文关键字检索建立 ngram 全文索引并把 token size 固定为 2 — verify: `cd backend && go test ./internal/store -run TestChineseFullTextSearch` 通过，且 `SHOW INDEX FROM knowledge_chunks` 可见 FULLTEXT 索引
- [ ] 2.3 为现有种子材料补齐默认 `auto` 切片，重复启动保持幂等 — verify: `cd backend && go test ./internal/store -run TestBackfillChunksIsIdempotent -count=1` 连续初始化两次后断言数量及唯一键不变

## 3. 切分模块

- [ ] 3.1 实现 `auto`：约 800 字、重叠 80 字，忽略请求中的自定义参数并优先自然断点 — verify: `cd backend && go test ./internal/retrieval/chunk -run 'TestAuto(Windows|IgnoresCustomOptions|NaturalBoundary)'`
- [ ] 3.2 实现 `custom`：分隔符、100～2000 字、0%～50% 重叠及可选预处理 — verify: `cd backend && go test ./internal/retrieval/chunk -run 'TestCustom|TestValidateCustomOptions'`，非法参数 API 用例均断言 400
- [ ] 3.3 实现 `hierarchy`：保留 Markdown 标题，超长章节再按窗口切分 — verify: `cd backend && go test ./internal/retrieval/chunk -run TestHierarchy` 断言标题、序号、偏移与正文
- [ ] 3.4 确认预处理不修改 `knowledge_entries.body_text` — verify: `cd backend && go test ./internal/retrieval -run TestReindexPreservesBodyText` 对比重建前后原文字节完全相同

## 4. 嵌入与 Qdrant

- [ ] 4.1 实现服务端嵌入客户端，校验返回维度与 Qdrant collection 一致 — verify: `cd backend && go test ./internal/retrieval/embed -run 'TestEmbed(Success|Timeout|Non2xx|Empty|WrongDimension)'`
- [ ] 4.2 初始化 `campusclaw_chunks` collection，使用余弦距离和配置维度 — verify: `bash scripts/verify-week04.sh qdrant-collection` 断言 distance=Cosine 且 size=`EMBEDDING_DIMENSIONS`
- [ ] 4.3 写入 point ID 与 `knowledge_chunks.id` 相同的向量和最小 payload — verify: `bash scripts/verify-week04.sh vector-linkage` 抽查至少三条并断言三方 ID 一致、payload 不含正文
- [ ] 4.4 实现嵌入失败状态，保留原文并确保 Qdrant 无残缺 point — verify: `cd backend && go test ./internal/retrieval -run TestEmbeddingFailureLeavesSourceWithoutPoint -count=1`

## 5. 上传集成与重建索引

- [ ] 5.1 材料原文入库后按请求策略切分、嵌入并返回 `chunk_count`、`chunk_strategy` — verify: `bash scripts/verify-week04.sh upload-three-strategies` 断言三次响应与数据库切片一致
- [ ] 5.2 实现教师专属 `POST /api/materials/{id}/reindex` — verify: `cd backend && go test ./internal/httpapi -run TestReindexReplacesOldChunksAndPoints -count=1`
- [ ] 5.3 服务端忽略上传或重建请求中的伪造 `class_id` — verify: `cd backend && go test ./internal/httpapi -run TestUploadAndReindexIgnoreClientClassID`
- [ ] 5.4 学生重建返回 403，跨班重建返回 404 且状态不变 — verify: `bash scripts/verify-week04.sh reindex-denials` 对比两种失败前后的切片数、point 数和状态

## 6. 关键字检索

- [ ] 6.1 实现 `POST /api/search` 的参数校验、分页限制和 Session 认证 — verify: `cd backend && go test ./internal/httpapi -run 'TestSearch(Validation|RequiresSession|Limit)'`
- [ ] 6.2 在 MySQL 按会话班级与 ready 状态执行关键字检索 — verify: `cd backend && go test ./internal/retrieval -run TestKeywordSearchScopesClassAndReadyStatus`
- [ ] 6.3 关键字响应返回来源字段、全文分与排名，不生成问句向量 — verify: `cd backend && go test ./internal/httpapi -run TestKeywordResponseHasTraceWithoutQueryVector` 并断言 embed mock 调用数为 0
- [ ] 6.4 Qdrant 停止时关键字模式仍正常 — verify: `bash scripts/verify-week04.sh keyword-with-qdrant-down` 断言 HTTP 200 与预期摘录

## 7. 向量与混合检索

- [ ] 7.1 实现问句嵌入和带会话班级过滤的 Qdrant 余弦查询 — verify: `cd backend && go test ./internal/retrieval -run TestVectorQueryUsesSessionClassFilter`
- [ ] 7.2 丢弃余弦低于 0.35 的候选，并以 `chunk_id + class_id` 回 MySQL — verify: `cd backend && go test ./internal/retrieval -run 'TestVectorThreshold|TestVectorReadbackScopesClass'`
- [ ] 7.3 实现 RRF `k=60`，按过滤后排名融合且不直接相加原始分数 — verify: `cd backend && go test ./internal/retrieval -run TestRRF` 覆盖双路、单路、并列和缺席
- [ ] 7.4 在响应中返回各路分数、名次、最终分、query_vector 信息和可追溯字段 — verify: `cd backend && go test ./internal/httpapi -run TestSearchSchemasByMode` 断言 vector/hybrid 有维度、范数和预览，keyword 无向量
- [ ] 7.5 Qdrant 或嵌入网关故障时 vector/hybrid 返回 503 — verify: `bash scripts/verify-week04.sh vector-dependency-failures` 断言均为 503 且无降级或伪造分数

## 8. 隔离、无依据与引用回答

- [ ] 8.1 伪造 `class_id` 和检索 B 班专属词均只得到 A 班范围结果 — verify: `bash scripts/verify-week04.sh class-isolation` 断言两次均为 200 且无 B 班标题、正文、ID 或分数
- [ ] 8.2 所有模式无候选时返回固定文案和空 hits — verify: `bash scripts/verify-week04.sh no-evidence-all-modes` 用天气问题断言三种模式均不返回弱相关切片
- [ ] 8.3 实现 `POST /api/ask`：只以最新 q 做本班 hybrid，最多取前 4 条，再结合清理后的历史生成带编号回答 — verify: `cd backend && go test ./internal/httpapi -run 'TestAskUsesLatestQueryOnly|TestAskCitationsAlign'`
- [ ] 8.4 无切片时 ask 直接返回固定文案，不调用生成模型 — verify: `cd backend && go test ./internal/httpapi -run TestAskNoEvidenceSkipsGenerator` 断言生成 mock 调用数为 0
- [ ] 8.5 丢弃客户端 system 消息，生成提示只由服务端构造 — verify: `cd backend && go test ./internal/httpapi -run TestAskDropsClientSystemMessages`
- [ ] 8.6 生成网关故障时有依据的 ask 返回 503 且不影响独立检索 — verify: `cd backend && go test ./internal/httpapi -run TestAskGeneratorFailureReturns503WithoutAffectingSearch`

## 9. 前端检索与可追溯展示

- [ ] 9.1 新增检索页，提供 keyword/vector/hybrid 切换并默认 hybrid — verify: `cd frontend && npm test -- --run search-page` 覆盖三种请求，且项目根执行 `rg 'localStorage.*(role|class)' frontend/src` 无命中
- [ ] 9.2 展示材料标题、切片序号、字符区间、摘录、命中路径和排序解释 — verify: `cd frontend && npm test -- --run traceable-hit-card` 断言所有字段与本班材料链接
- [ ] 9.3 新增引用式问答页，无依据固定文案与有依据 citations 展示正确 — verify: `cd frontend && npm test -- --run ask-page` 覆盖有依据引用顺序和天气无依据文案
- [ ] 9.4 教师上传与材料页提供切分策略、重建及向量关联查看；学生不显示写入口 — verify: `cd frontend && npm test -- --run retrieval-role-controls`，并回到根目录执行 `bash scripts/verify-week04.sh student-reindex-403`

## 10. Compose 与发布验收

- [ ] 10.1 Compose 增加 Qdrant 和持久化卷，不映射宿主端口 — verify: `docker compose config > /tmp/week04-compose.yml && ! rg -n '6333:|6334:' /tmp/week04-compose.yml`，且只有 web 含宿主 `ports`
- [ ] 10.2 从零启动后自动创建 collection 并补齐种子材料索引 — verify: `bash scripts/verify-week04.sh clean-start-backfill` 断言 collection、切片、向量及主键对应
- [ ] 10.3 运行严格 OpenSpec 校验、后端测试、前端构建和依赖审计 — verify: `npx --yes @fission-ai/openspec@latest validate add-traceable-vector-retrieval --strict && (cd backend && go test ./... && go vet ./...) && (cd frontend && npm audit && npm run build)`
- [ ] 10.4 编写并执行 `scripts/verify-week04.sh`，验收三种模式、隔离、无依据、引用、重建和故障语义 — verify: `bash scripts/verify-week04.sh all | tee docs/iteration-2-verification.txt` 退出码为 0
- [ ] 10.5 检查公开仓库、完整历史与镜像无 `.env`、模型密钥、数据库数据和上传文件 — verify: 在干净克隆执行 `! rg -n 'sk-[A-Za-z0-9_-]{16,}|(EMBEDDING|CHAT)_API_KEY[[:space:]]*=[[:space:]]*[^[:space:]]+' --hidden --glob '!.git/**' --glob '!.env.example'`；由提交人执行 `! git log -p --all -- . ':(exclude).env.example' | rg 'sk-[A-Za-z0-9_-]{16,}|(EMBEDDING|CHAT)_API_KEY[[:space:]]*=[[:space:]]*[^[:space:]]+'`；并确认镜像内不存在 `.env`
