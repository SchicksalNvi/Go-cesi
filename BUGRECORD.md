# Superview 问题跟踪文档 (Bug Record)

> 本文档记录代码审查发现的待优化问题。**修复流程**见文末《维护协议》。

## 状态图例

- ⬜ **TODO** — 待修复
- 🔧 **DOING** — 修复中
- ✅ **DONE** — 已修复并验证
- ❌ **WONTFIX** — 经评估不修复（需注明原因）

## 进度总览

| 严重程度 | 总数 | TODO | DONE | WONTFIX |
|---------|------|------|------|---------|
| High    | 14   | 0    | 14   | 0       |
| Medium  | 40   | 0    | 39   | 1       |
| Low     | 18   | 0    | 18   | 0       |

---

## 2026-08-06 第四轮全量复审

- **审查范围**: `cmd/`、`internal/`、`web/react-app/src/`、`tools/`、Makefile、启动/发布脚本、示例配置以及 GitHub/Gitea workflows；逐项复核本文档中全部历史条目。
- **审查重点**: 认证与 RBAC、节点 ACL、秘密数据流、并发与生命周期、数据库迁移/备份、错误传播、后台任务、前后端状态一致性及工程验证链。
- **复审结论**: `H-05` 与 `M-01` 的原修复只覆盖了部分路径，现重新标记为 TODO；其余历史 DONE 条目保留。新增问题从 `H-06`、`M-10`、`L-05` 继续编号。
- **排除项**: Discovery 路由虽未统一挂权限中间件，但相关 handler 内均调用 `authorizeDiscoveryAccess`，本轮不作为权限绕过记录。

### 验证结果

- **通过**: `npm run type-check`、`npm run build`、`go vet ./cmd ./internal/...`，以及 `cmd` 和 `internal` 各核心包的定向 `go test`；`go test -race ./internal/supervisor ./internal/websocket ./internal/services` 通过。
- **数据库测试**: 排除已知极慢的 `internal/database.TestResourceReleaseProperties` 后，仓储、事务、连接池、超时与韧性相关测试通过；该属性测试运行约 5 分钟仍未结束，已中止。
- **当前验证缺口**: `go test ./...` / `go vet ./...` 会因 `tools/` 下四个文件同属 `package main` 且各自定义 `main()` 而失败；`npm run lint` 因缺少 ESLint 配置失败；前端虽有测试源码，但无 test script/runner，且 `tsconfig.json` 明确排除了测试目录。

---

## 2026-08-07 第五轮复审(H-05/H-06/H-12 修复后的工作区)

- **审查范围**: 当前未提交工作区改动(对应 H-05、H-06、H-12 的修复):`cmd/main.go`、`internal/auth/`、`internal/services/{user,data_management}.go`、`internal/api/{user,users,data_management}.go`、`internal/websocket/hub.go`、`internal/middleware/performance.go`、`internal/repository/`、`internal/models/user.go` 及新增测试;并复核 H-07~H-11、M-01、M-10 等历史 TODO 条目现状。
- **验证通过**: `go build ./cmd/... ./internal/...`、`go vet ./cmd ./internal/...`;`go test ./internal/auth ./internal/middleware ./internal/websocket ./internal/services ./internal/api`;`go test -race ./internal/auth ./internal/websocket ./internal/middleware`。
- **复审结论**: H-05/H-06/H-12 的修复实现与测试质量良好,但 H-06 的 WebSocket 会话吊销建立在无缓冲 `register` 通道上(H-13/H-14),H-05 的流式解码引入数字类型回归(M-33),H-06 的并发乐观锁在 `UpdateUser` 留了 `is_admin` 竞态窗口(M-29)。新增 H-13/H-14、M-28~M-34、L-11/L-12。
- **未动项确认**: H-07~H-11、M-01、M-10 及全部前端条目本轮未修,现状与记录一致。

---

## 🔴 High — 严重问题

### H-01 ✅ 扫描结果匹配是 O(n²)
- **位置**: `internal/services/scanner.go:369-376`（`runScan`）
- **问题**: 每收到一个探测结果都线性遍历整个 `probeTasks` 找匹配 ID。`/16` 网段（65k IP）→ 约 65000² 次比较。
- **修复建议**: 在 `runScan` 中一次性构建 `map[string]*ProbeTask`（按 `task.ID()` 为键），改为 O(1) 查找。
- **修复记录**: 2026-06-16 · `scanner.go:308-320` 构建 `taskByID` map，结果循环改为 O(1) 查找（`scanner.go:336`）；删除未使用的 `resultMap`/`resultMu`。`go test ./internal/services/` 通过。

### H-02 ✅ 逐 IP 单条 DB 写入
- **位置**: `internal/services/scanner.go:400`、`internal/services/scanner.go:411`
- **问题**: `createDiscoveryResult` 每个 IP 一条 INSERT；`UpdateTaskProgress` 在结果循环内频繁触发。大网段 = 数千次独立写，全部挤在单连接上。
- **修复建议**: 累积批量 INSERT（`CreateInBatches`）；进度更新按时间节流而非每 10 条。
- **修复记录**: 2026-06-16 · 新增 `DiscoveryRepository.CreateResults`（`CreateInBatches`，见 `repository/discovery_repository.go:93`、`interfaces.go:63`、`discovery.go:CreateResults`、mock 同步）；scanner 改为缓冲 100 条批量写入 + `defer flushResults()` 保证取消时落盘；进度 DB 写入按 2s 节流（`scanner.go` runScan）。`createDiscoveryResult`→`buildDiscoveryResult` 只构建不插入。测试通过。

### H-03 ✅ 数据库单连接全局串行化
- **位置**: `internal/database/database.go:44`（`MaxOpenConns: 1`）
- **问题**: 所有请求的所有 DB 操作排队走一条连接。WAL 模式本可支持多读单写——一次同步导出（读 1 万条日志）会阻塞期间所有其他请求。
- **修复建议**: WAL 下提高读连接数（如 4-8），或为只读查询单独建池；至少在注释中明确这是有意的写锁权衡。
- **修复记录**: 2026-06-16 · `GetDefaultConfig` 改为 25/5/5min/1min（与 `TestDatabaseConfigDefaults` 期望一致——该测试此前因代码被throttle 到 1/1/0/0 一直失败，本次顺带修复）；DSN 去掉 `cache=shared`、加 `_busy_timeout=5000`（多连接 WAL 并发的正确配置，写锁竞争等待而非报错）。注：`TestResourceReleaseProperties` 在原始 HEAD 上即超时（300+ 次全量迁移），属预先损坏、与本改动无关。其余 database 测试通过。

### H-04 ✅ 生产环境 GORM 全量 SQL 日志
- **位置**: `internal/database/database.go:75`
- **问题**: `logger.Default.LogMode(logger.Info)` 把每条 SQL 都打到 Info，高频路径下是实打实的 CPU + 磁盘开销。
- **修复建议**: 生产用 `logger.Warn` 或 `Silent`，按环境（环境变量/配置）切换。
- **修复记录**: 2026-06-16 · 新增 `resolveGormLogLevel()`（`database.go:62`），默认 `Warn`，可经环境变量 `DB_LOG_LEVEL=silent\|error\|warn\|info` 覆盖（开发期设 info）。

### H-05 ✅ 导入仍可启动无界后台任务并全量读取上传文件
- **位置**: `internal/api/data_management.go:111-129`、`:491-557`；历史修复位于 `internal/services/data_management.go` 的 `runDataJob`
- **问题**: 2026-06-16 的限流只覆盖导出和备份。导入接口仍对每次请求直接 `go func()`，没有共享并发上限或可取消 context；上传请求未设置文件/请求体大小上限，随后又用 `os.ReadFile` 将整个文件载入内存。攻击者或误操作可并发提交大文件，耗尽内存、磁盘和数据库资源。
- **修复建议**: 将导入接入同一作业队列/信号量，限制 multipart body 与单文件大小，采用流式 JSON 解码，并在完成/失败后按保留策略清理上传文件。
- **历史修复记录**: 2026-06-16 · 新增 `dataJobSlots`（容量 2）和 `runDataJob`，为导出/备份增加限流与 panic 恢复。
- **复审记录**: 2026-08-06 · 导入路径未使用上述机制，原条目重新打开。
- **修复记录**: 2026-08-07 · `internal/services/data_management.go:26-188` 将导出、导入、备份统一接入启动前获取槽位的共享并发限制，并为导入提供可取消 context、终态/拒绝入队源文件清理；`internal/api/data_management.go:73-417,776-856` 将 multipart 请求和单文件限制为 32 MiB、以 0600 权限落盘，并改为逐 token/逐记录的 `json.Decoder` 流式解析。新增上传上限、分块解码、尾随 JSON、任务容量及源文件清理回归测试；`go test ./internal/api ./internal/services`、相关 `go test -race` 与 `go vet ./internal/api ./internal/services` 通过。

### H-06 ✅ 停用/删除用户仍可登录或继续使用既有 JWT
- **位置**: `internal/auth/auth.go:107-139`、`:253-335`；`cmd/main.go:433-461`（WebSocket 鉴权）
- **问题**: 登录流程验证密码后未检查 `user.IsActive`。HTTP 鉴权中间件只验证 JWT 签名；数据库中找不到用户或用户已停用时不会拒绝请求，仍会 `c.Next()`。WebSocket 更只解析 token，不重新加载用户状态。停用、删除和登出因此不能可靠撤销最长 24 小时有效的令牌。
- **影响**: 离职/被禁用账户仍可能访问 API 或持续接收 WebSocket 数据，账户处置无法即时生效。
- **修复建议**: 登录和每次鉴权均强制校验用户存在且启用；为 token 增加版本号/吊销时间或短期 access token + refresh token，并让 HTTP/WS 共用同一认证逻辑。
- **修复记录**: 2026-08-06 · `models.User` 与 JWT claims 新增单调递增的 `TokenVersion`；登录、HTTP 中间件和 WebSocket 握手统一校验数据库中的用户存在性、启用状态与令牌版本，WebSocket 心跳期间持续复验。登出、停用及密码修改通过原子更新递增版本，普通用户更新禁止回写旧版本，避免并发操作让已吊销会话复活；删除用户后鉴权因记录不存在立即失败。新增认证、用户服务和 WebSocket 会话回归测试；`go test ./internal/auth ./internal/websocket ./internal/services ./internal/api ./internal/repository ./internal/middleware ./cmd`、`go test -race ./internal/auth ./internal/websocket ./internal/services` 与 `go vet ./cmd ./internal/...` 通过。

### H-07 ✅ `user:write` 可升级为管理员或超级管理员
- **位置**: `internal/api/users.go:85-234`、`internal/api/roles.go:43-68`、`:182-210`、`:257-383`；路由见 `internal/api/api.go:88-98`、`:146-161`
- **问题**: 仅拥有 `user:write` 的用户可在创建/更新用户时直接设置 `is_admin`；同一权限还可创建/修改角色、向角色分配任意权限，并把任意角色（包括 `super_admin`）分配给自己或他人。代码没有“不得授予高于自身权限”的边界，也没有将管理员字段/系统角色操作限制给超级管理员。
- **影响**: 低权限的用户管理员可完成持久化权限提升，获得节点、配置、系统管理等全部能力。
- **修复建议**: 将用户资料管理、管理员状态、角色管理和权限授予拆分为独立权限；实施权限子集校验，系统角色和 `is_admin` 仅允许超级管理员变更，并禁止自我提权。

### H-08 ✅ WebSocket 绕过节点 ACL 并广播全部节点/进程/日志
- **位置**: `internal/websocket/hub.go:680-816`、`internal/websocket/client.go:112-208`；WebSocket 路由见 `cmd/main.go:433-461`
- **问题**: WebSocket client 不保存经过数据库加载的用户，也不调用 `NodeAccess`。连接后会收到 `GetAllNodes()` 初始数据和全局节点/系统广播；客户端还能以任意节点名订阅节点、刷新进程或订阅进程日志。
- **影响**: 只获准访问部分节点的用户可枚举其他节点、查看进程状态和读取日志，HTTP API 的节点 ACL 被实时通道完全绕开。
- **修复建议**: 握手时加载完整用户与节点 ACL，将用户上下文绑定到 client；初始数据、广播、节点订阅和日志订阅都必须逐节点过滤并再次授权。

### H-09 ✅ 配置秘密可绕过 `config:view_secret` 被导出或查询
- **位置**: `internal/services/configuration.go:130-186`、`:304-343`、`:367-457`、`:591-635`、`:751-810`；`internal/api/api.go:273-287`；`internal/api/system_settings.go:133-174`；`internal/services/data_management.go:238-283`
- **问题**: 单条配置读取会按 `config:view_secret` 掩码，但批量导出、配置备份详情、变更历史和审计详情直接返回原始值；历史中的 `OldValue/NewValue` 也可能保存秘密。系统设置 GET 原样返回所有值（包括 SMTP 密码等），完整数据导出同样包含原始配置与设置。
- **影响**: 只有普通配置读取或系统配置权限、但没有查看秘密权限的用户，可从替代接口取得凭据；备份和审计响应还会扩大秘密留存面。
- **修复建议**: 建立统一的秘密序列化/脱敏层，所有读取、导出、备份、历史和审计接口强制检查 `config:view_secret` / `env_var:view_secret`；秘密历史应加密或只记录“已变更”。

### H-10 ✅ WAL 模式下直接复制主数据库文件会产生不一致备份
- **位置**: `internal/services/data_management.go:566-604`；WAL 开启位置 `internal/database/database.go:100-105`
- **问题**: 完整备份直接把 `data/superview.db` 加入 ZIP，但 SQLite 使用 WAL。尚未 checkpoint 的最新事务位于 `superview.db-wal`，复制主文件既可能遗漏已提交数据，也可能在写入过程中得到不一致快照。
- **影响**: 备份任务可显示成功，但恢复后缺少最新数据或数据库损坏，直到灾难恢复时才暴露。
- **修复建议**: 使用 SQLite Online Backup API、`VACUUM INTO` 或受控 checkpoint + 一致性快照；备份完成后实际打开副本并执行 `PRAGMA integrity_check`。
- **复审记录**: 2026-08-07 · `createFullBackup`(`data_management.go:693`)仍直接 zip 复制 `data/superview.db` 主文件,未做 checkpoint/`VACUUM INTO`/完整性校验,条目保持打开。

### H-11 ✅ `supervisor.Node` 状态存在广泛数据竞争
- **位置**: `internal/supervisor/node.go:43-58`、`:115-218`；`internal/supervisor/service.go:250-288`、`:358-379`、`:770-800`、`:888-900`；`internal/api/processes.go:69-316`、`internal/api/health.go:200-220`、`internal/services/alert_monitor.go:92-160`、`internal/websocket/hub.go:748-772`
- **问题**: `Node` 为 `IsConnected`、`LastPing`、`Processes` 定义了 mutex 和安全 getter，但 service、API、告警及 WebSocket 仍大量直接读写这些字段；这些路径会与自动刷新、状态监控、请求刷新及广播 goroutine 并发运行。
- **影响**: 存在 Go data race、slice 并发读写、状态撕裂和偶发崩溃风险；节点越多、刷新越频繁越容易触发。
- **修复建议**: 将状态字段私有化，只允许通过锁保护的方法读写；对进程切片返回不可变快照，并用 `go test -race` 覆盖自动刷新、API 与广播并发场景。

### H-12 ✅ 性能指标重置必然自死锁
- **位置**: `internal/middleware/performance.go:111-125`、`:268-285`、`:292-317`；触发路由 `internal/api/api.go:403-404`
- **问题**: `ResetPerformanceMetrics()` 持有 `globalMetrics.mu` 写锁后调用 `updateMemoryMetrics()`，后者再次请求同一把非可重入锁，调用永远无法返回。开发者重置接口和定时清理都会进入该路径。
- **影响**: 锁被永久占用后，性能中间件和指标查询的后续请求会持续阻塞，可能拖死整个 HTTP 服务。
- **修复建议**: 在锁外读取内存指标，或拆出要求调用方已持锁的无锁 helper；补充 reset API 和定时重置的超时/并发回归测试。
- **修复记录**: 2026-08-06 · `internal/middleware/performance.go` 新增无锁的 `collectMemoryMetrics()`，重置前先采样、随后在单次加锁中原子替换计数与内存快照，避免递归锁；新增 `performance_test.go` 验证重置可在超时内返回、清空指标且后续仍能记录。`go test` 与 `go test -race ./internal/middleware -run TestResetPerformanceMetricsDoesNotDeadlock` 通过。

### H-13 ✅ WebSocket 无缓冲 `register` 通道可阻塞 Hub 主循环并泄漏 writePump
- **位置**: `internal/websocket/hub.go:229`(`register` 无缓冲)、`:965`(`HandleWebSocket` 同步发送)、`:335-344`(注册时执行会话校验)
- **问题**: `register` 是无缓冲 channel,`HandleWebSocket` 在升级握手后**同步阻塞**发送 `client.hub.register <- client`,之后才启动 read/write pump。H-06 在 `Run` 主循环的 `register` 分支新增了 `validateSession`(慢 SQL 点查)。后果链:1) 主循环卡在处理某次慢校验时,无法消费 `unregister`/`broadcast`;2) 心跳检查(`checkHeartbeats`)持 `client.mu` 向无缓冲 `unregister` 发送,主循环被占时第一个失败客户端就卡死;3) 已判定失活的客户端其 `send` 通道无人 close,writePump 永久阻塞在 `<-c.send`,goroutine 泄漏。
- **影响**: 连接数较多或 DB 瞬时繁忙(WAL 写锁、迁移、备份)时,Hub 主循环卡死,所有连接的心跳/断开/广播全部停摆,且泄漏 writePump goroutine。
- **修复建议**: `register` 改为带缓冲(如 128)并在 `HandleWebSocket` 用 `select + default` 非阻塞入队(入队失败直接关连接);或把会话校验移到升级前的 HTTP 上下文(复用 `auth.AuthenticateToken` 结果),避免主循环内执行慢查询。

### H-14 ✅ WebSocket 心跳对每个连接做 DB 点查,且瞬时 DB 错误会被误判为"会话吊销"踢掉全部用户
- **位置**: `internal/websocket/hub.go:693`(`checkHeartbeats` 内 `h.validateSession`)
- **问题**: `checkHeartbeats` 每 30s 对**每个**客户端各执行一次 `auth.ValidateUserSession`(单条 SELECT)。1) N 连接 × 每 30s 的重复点查,叠加 H-13 的主循环阻塞形成放大;2) validator 返回的**任何** error(包括 DB 连接失败、锁超时等瞬时错误)都被当作"会话已吊销",`failedClients` 全量收集后集体断开——数据库瞬时故障会把所有在线用户踢下线。
- **影响**: 大规模连接时心跳检查成本高;DB 抖动时出现"全员掉线"事故。
- **修复建议**: 区分 `ErrSessionUnavailable`(吊销→断开)与瞬时 DB 错误(记录日志、本轮跳过);对全部连接改为**一次批量查询**(`WHERE id IN (...)`)替代 N 次点查;心跳校验间隔可放宽(如 60s)。

---

## 🟡 Medium

### M-01 ✅ 完整导出仍全量载入内存并截断数据
- **位置**: `internal/services/data_management.go:185-199`、`:238-283`（`exportAll`）
- **问题**: 当前 `exportAll` 仍把 users、最多 1 万条日志、configs 和 settings 全部 `Find` 到切片/map 后一次性编码。日志仍固定 `Limit(10000)`，只在服务端写 warning，导出记录/响应中没有“不完整”标记；各表查询失败也以 `err == nil` 分支静默跳过，最终文件仍可能显示成功。
- **修复建议**: 流式/分批导出，错误立即使作业失败；若产品允许上限，需在导出记录和文件元数据中明确标注截断数量与条件。
- **历史修复记录**: 2026-06-16 · 提取 `maxExportLogs` 并在命中上限时记录 warning。
- **复审记录**: 2026-08-06 · 内存峰值、固定截断和查询错误静默问题均仍存在，原条目重新打开。
- **复审记录**: 2026-08-07 · `exportAll`(`data_management.go:358-387`)仍 `Find` 全部 + `Limit(10000)` + `err==nil` 静默跳过,未修,条目保持打开。

### M-02 ✅ 吞掉的状态更新错误
- **位置**: `internal/services/data_management.go:79`、`:127`、`:515`、`:554`、`:395`、`:659`
- **问题**: 多处 `s.DB.Model(record).Updates(...)` 不检查 `.Error`。状态更新失败则记录永久卡在 pending/running。
- **修复建议**: 检查并记录 `.Error`。
- **修复记录**: 2026-06-16 · 新增 `applyUpdates(record, updates, ctx)` helper（失败时 `logger.Error`），performExport/performBackup 的 running/completed 状态更新及 updateExportStatus/updateBackupStatus 全部改用之。

### M-03 ✅ 静默吞掉备份错误
- **位置**: `internal/services/data_management.go:580-582`
- **问题**: 空 `if` 块注释说"配置文件可能不存在"，但实际把所有错误都丢了。
- **修复建议**: 区分"文件不存在"（忽略）与真实 I/O 错误（记录）。
- **修复记录**: 2026-06-16 · `createFullBackup` 改为 `if err != nil && !os.IsNotExist(err) { logger.Warn(...) }`，文件不存在忽略、其它错误记录。

### M-04 ✅ 大量重复样板
- **位置**: `internal/services/data_management.go`（`GetExportRecords`/`GetImportRecords`/`GetBackupRecords`、三个 `Delete*`、三个 CSV 导出器）
- **问题**: 分页列表与删除逻辑几乎逐字重复；CSV 导出器共享相同的 file+writer 开头。
- **修复建议**: 提取通用分页列表 helper 和 `withCSVWriter` helper。
- **修复记录**: 2026-06-16 · 新增泛型 `listPaginatedRecords[T]`（统一 Preload/Count/分页），三个 Get* 改为一行委托；新增 `writeCSV(filePath, headers, rows)` helper，三个 CSV 导出器改为构建 rows 后调用之（并补上之前未检查的 `writer.Error()`）。data_management 测试通过。

### M-05 ✅ 日志轮询串行 + 阻塞 RPC
- **位置**: `internal/websocket/hub.go:436-495`（`pollAndStreamLogs`）
- **问题**: 每 2 秒 tick 顺序遍历所有订阅、每个同步调 `GetProcessLogSize`/`GetProcessLogStream`（远程 XML-RPC）。订阅数 × RPC 延迟超过 2s，tick 持续滞后。
- **修复建议**: 并发拉取（带 worker 上限）或为单 tick 设预算。
- **修复记录**: 2026-06-16 · 提取 `streamLogKey(logKey)`，`pollAndStreamLogs` 改为带信号量（上限 8）+ `WaitGroup` 的并发拉取，每个 goroutine 含 `recover`。`go test -race ./internal/websocket/` 通过。

### M-06 ✅ 每次握手重建 WS 配置
- **位置**: `internal/websocket/hub.go:82`（`upgrader.CheckOrigin`）
- **问题**: 每个连接都调 `GetDefaultWebSocketConfig()`，重读环境变量、重建 slice。
- **修复建议**: 启动时缓存一份允许来源。
- **修复记录**: 2026-06-16 · 提取 `resolveAllowedOrigins()`（带 `allowedOriginsCache` + RWMutex），`SetAllowedOrigins` 失效缓存以支持配置热重载；`CheckOrigin` 与 `GetDefaultWebSocketConfig` 改用之。

### M-07 ❌ 每个请求都读环境变量 (JWT) — WONTFIX
- **位置**: `internal/auth/jwt.go:13`（`getJWTSecret`）
- **问题**: 每次 `ParseToken` 都 `os.Getenv` + 长度校验。
- **WONTFIX 原因**: Go 的 `os.Getenv` 是进程内内存读取（非系统调用），开销可忽略；用 `sync.Once` 缓存会破坏"未设置即报错"契约、阻碍密钥轮换，并使依赖环境变量切换的测试变脆。收益不抵风险。
- **遗留**: token 24h 固定、无刷新、无吊销名单（登出无法失效）已在 H-06 继续跟踪。

### M-08 ✅ `isConnected` 永不更新
- **位置**: `web/react-app/src/hooks/useWebSocket.ts:166`
- **问题**: 从 `wsRef.current?.readyState` 在渲染期读取，连接状态变化不触发重渲染，UI 连接指示一直是初值。
- **修复建议**: 用 `useState` 维护连接状态。
- **修复记录**: 2026-06-16 · 新增 `const [isConnected, setIsConnected] = useState(false)`，在 `onopen` 置 true、`onclose`/`disconnect` 置 false，返回该 state。`tsc --noEmit` 对该文件无错误。

### M-09 ✅ Settings 巨型组件
- **位置**: `web/react-app/src/pages/Settings/index.tsx`（1193 行、24 个 `useState`）
- **问题**: 难维护、整体重渲染。
- **修复建议**: 按 Tab（系统设置 / 导出 / 导入 / 备份）拆成子组件 + 自定义 hook。
- **修复记录**: 2026-06-17 · 抽出纯函数与常量到 `Settings/helpers.ts`；三段大型表格列定义抽到 `Settings/columns.tsx`（`buildExportColumns`/`buildImportColumns`/`buildBackupColumns` 工厂，接收 `t` + handlers），index.tsx 改用 `useMemo` 调用（同时减少列定义随渲染重建）。index.tsx 从 1193 行降至 ~875 行。✅ 已验证：`npx tsc --noEmit` 通过（EXIT=0，用户手动复验）。

### M-10 ✅ 数据管理的路径、资源清理和记录隔离不完整
- **位置**: `internal/api/data_management.go:191-475`、`:567-610`；`internal/services/data_management.go:225-236`、`:430-492`、`:494-684`
- **问题**: 备份名未经路径清理即可含 `../` 并逃逸 `data/backups`；并发配置备份共享 `data/temp/configurations.json`，任一任务结束会 `RemoveAll("data/temp")`；完整备份读取的是仓库根 `config.toml` 而实际示例位于 `config/config.toml`；ZIP writer/文件 close、`Walk` 和 `addFileToZip` 的多处错误未传播；进程导出恒为空；成功上传文件只在删除导入记录时清理。
- **权限一致性**: 导出下载检查创建者，但删除不检查；备份/导入列表、下载和删除均未按创建者过滤，同为 `system:manage` 的不同操作者可互相读取或删除文件。
- **修复建议**: 校验并固定所有生成路径、每个任务使用独立临时目录、完整传播 close/walk 错误、实现真实进程导出和上传保留策略，并统一 owner/super-admin 授权规则。

### M-11 ✅ 告警规则与通知链路无法可靠工作
- **位置**: `internal/services/alert_monitor.go:52-67`、`:239-369`；`internal/services/alert.go:283-505`；`internal/api/alerts.go:120-155`、`:290-337`、`:473-500`
- **问题**: 默认规则只检查 ID 1/2 是否存在却不创建，状态告警仍硬编码 `RuleID: 1/2`；通用指标规则入口 `CheckAlertRules()` 没有生产调用点。Email/Slack/Webhook/DingTalk 的“发送”函数仅打印日志就返回成功并把通知标记为 sent；配置字段直接 `.(string)`，缺失或类型错误会 panic，Slack/Webhook URL 还会被写入日志。
- **附加缺陷**: 三个列表接口允许 `page_size=0`，计算 `total_pages` 时会除零 panic。
- **修复建议**: 事务化 seed 默认规则并取消硬编码 ID；将规则检查接入生命周期；实现真实、带超时/重试的通知适配器及安全配置解析/脱敏；统一分页校验。

### M-12 ✅ 日志分析、导出与保留策略未形成可用闭环
- **位置**: `internal/models/log_analysis.go:191-210`、`:260-279`；`internal/api/log_analysis.go:700-778`；`internal/services/log_analysis.go:24-45`、`:454-530`、`:646-716`；`internal/services/activity_log.go:378-418`
- **问题**: API 默认导出格式为 JSON，但校验只接受 TXT/XML，默认请求必然失败；后台导出仅 sleep 后写入虚构路径/下载 URL，不创建文件且没有对应下载路由。模型允许 glob，匹配实现却没有 glob 分支；`actions.create_alert.(bool)` 可因畸形 JSON panic；每条日志另起 goroutine，统计“先查后建”可并发生成重复记录；归档只打标记，不会在 retention 到期后删除。
- **活动日志导出**: 无上限一次性查询全部记录，以循环字符串拼接构造 CSV，字段只给 message 加引号且未转义嵌入的引号/换行/逗号。
- **修复建议**: 对齐格式常量与路由，真正生成并安全下载文件；完整实现/拒绝未支持条件，使用有界 worker 和数据库 upsert；明确 archive→delete 生命周期，并用 `encoding/csv` 流式输出。

### M-13 ✅ 增强进程的调度和恢复接口会虚假成功
- **位置**: `internal/services/process_enhanced.go:40-84`、`:175-189`、`:278-465`、`:555-585`；`internal/api/process_enhanced.go:36-58`、`:530-585`
- **问题**: 定时 start/stop/restart/custom command 只返回模拟字符串，进程备份恢复也只返回数据库记录，却均记为成功。调度器不随应用启动；重复 stop/start 会在同一 cron 实例重复注册任务，更新/删除任务也不移除或替换已注册 job；`scheduler.running` 无锁并发读写。重排接口对任意 JSON 直接类型断言，可被请求触发 panic。
- **修复建议**: 接入真实 Supervisor 操作和配置写回；保存 cron EntryID 并同步增删改，给调度器明确的应用生命周期和锁；请求使用强类型 DTO 与完整校验，未实现能力返回明确错误而非成功。

### M-14 ✅ 增强进程接口绕过节点 ACL 与私有资源隔离
- **位置**: `internal/api/process_enhanced.go`；`internal/services/process_enhanced.go:487-585`
- **问题**: 大量含 `node_id` 的分组、依赖、备份、指标和优化接口只依赖路由级 process 权限，不调用 `NodeAccess`；进程模板列表/详情/更新/删除也没有强制 `is_public OR created_by=currentUser`，调用者可查看和修改其他用户的私有模板。
- **影响**: 节点级隔离和模板所有权在增强接口上失效，与普通节点/进程 API 的授权模型不一致。
- **修复建议**: 为所有 node-scoped handler 复用统一 ACL helper；service 查询强制加入 owner/public scope，修改和删除要求 owner 或显式管理权限。

### M-15 ✅ 配置恢复/导入的事务与模型约束不一致
- **位置**: `internal/services/configuration.go:189-212`、`:346-364`、`:460-574`、`:638-749`；`internal/models/configuration.go:48-76`
- **问题**: restore 的 options 没有控制选择性恢复，import 的 `import_configs/import_env_vars` 也未使用；JSON marshal/unmarshal、类型断言和多次 `tx.Create/Updates` 错误被忽略。defer recover 只 rollback，不把 panic 转成返回错误，函数可能在 panic 恢复后以 nil 返回。`Configuration.Key` 是全局 unique，但业务查询按 key+scope+node+user 判重，作用域设计无法落库；删除实体前刚创建的历史随后又被立即删除。
- **修复建议**: 使用命名返回值并在 recover 后返回错误，检查事务开始和每条 SQL；以强类型导入结构校验 options；把唯一索引改为符合 scope 的复合约束，并保留删除审计历史。

### M-16 ✅ 启动初始化与配置热重载不完整
- **位置**: `internal/config/validator.go`；`internal/services/permission.go:101+`、`internal/services/role.go:150+`；`cmd/main.go:493-527`
- **问题**: 配置 Validator、系统权限初始化和系统角色初始化均没有生产调用点，配置可绕过已有校验且全新数据库可能缺 RBAC 基础数据。SIGHUP 热重载只添加新节点，不更新已有节点的 host/port/凭据/环境，也不移除已删除节点，却仍记录“reload successfully”。
- **修复建议**: 在服务启动/迁移后显式验证配置并幂等 seed 权限角色；热重载先计算 add/update/remove diff，原子应用或整体回滚，并报告部分失败。

### M-17 ✅ Supervisor 批量操作会吞掉失败并持锁执行 RPC
- **位置**: `internal/supervisor/service.go:514-533`、`:604-759`
- **问题**: restart-all 记录单个失败后仍返回 nil；分组操作忽略 `RefreshProcesses` 及 start/stop/restart 错误，最终总是成功。`GetGroups` 和分组操作在持有 service `RLock` 时刷新节点或执行远程 XML-RPC，慢节点会长期阻塞节点增删和其他写操作。
- **修复建议**: 在锁内只复制节点快照，锁外 RPC；汇总逐节点/逐进程结果并返回部分失败，API 响应需反映真实执行状态。

### M-18 ✅ 刷新监听与超时封装存在生命周期并发问题
- **位置**: `cmd/main.go:355-370`、`:545-581`、`:698-735`；`internal/supervisor/timeout_manager.go:43-180`、`:232-257`
- **问题**: `watchRefreshIntervalChanges` 没有 stop/context，关闭数据库后仍会继续查询；它与主关闭流程并发替换、关闭 `stopRefresh/stopMonitoring` channel，可 data race 或 double-close。`ExecuteWithTimeout` 超时返回后只取消 context，若底层函数不遵守 context，goroutine/RPC 仍继续执行。CircuitBreaker 的 failures 同时使用锁内普通写与无锁 atomic 读，也会触发 race detector。
- **修复建议**: 统一使用 context + WaitGroup 管理 watcher 和 worker，channel 所有权只归单一组件；底层 RPC 使用可取消请求/连接 deadline；计数访问统一用同一锁或全原子操作。

### M-19 ✅ XML-RPC 编码、响应边界和日志时区处理不安全
- **位置**: `internal/supervisor/xmlrpc/client.go:49-103`；`internal/supervisor/node.go:18-41`、`:489-519`
- **问题**: method 和字符串参数通过 `fmt.Sprintf` 直接拼入 XML，`&<>` 等字符会破坏请求；响应使用无上限 `io.ReadAll`，异常节点可造成大内存分配。`SetLogTimezone` 虽更新 `LogTimezone`，日志解析仍硬编码 `time.Local`，配置完全无效。
- **修复建议**: 使用标准 XML encoder/成熟 XML-RPC 客户端，对响应加 `io.LimitReader` 并检查 Content-Type/状态；解析时间统一读取线程安全的已配置时区。

### M-20 ✅ `system_settings` 迁移仍可能导致数据丢失
- **位置**: `internal/database/database.go:348-461`
- **问题**: `fixEmptyCategories` 在非事务流程中读取旧数据、DROP 表、重建并逐条 INSERT。L-02 的历史修复只把失败改为 warning；DROP/CREATE/恢复任一步失败仍会继续启动，恢复 INSERT 失败也不向上返回，数据库可留下空表或部分数据。
- **修复建议**: 在单事务中创建临时表、校验行数与约束后原子 rename；任何步骤失败必须 rollback 并阻止启动，迁移前后执行备份与完整性校验。

### M-21 ✅ 系统设置和开发者工具存在“模拟成功”及配置失效
- **位置**: `internal/api/system_settings.go:247-334`、`:499-553`；`internal/api/developer_tools.go:36-75`、`:407-429`；`internal/api/api.go:389-421`
- **问题**: 测试邮件只 sleep/打印配置就返回成功；清理调试日志和设置日志级别不做实际操作。`developer_tools.enabled=false` 未生效，因为路由构造始终传 nil 并回退到默认 enabled 配置。批量设置遇到数组/对象等类型会静默转成空字符串并仍报告更新成功。
- **修复建议**: 未实现功能返回 501 或完成真实实现；把应用配置注入路由并在 disabled 时不注册端点；批量设置按 schema 严格编码/校验，任一失败时事务回滚。

### M-22 ✅ 已实现的安全中间件没有接入生产路由
- **位置**: `internal/middleware/error_handler.go`、`internal/middleware/validation.go:19-178`；注册位置 `cmd/main.go:372-396`、`internal/api/api.go:21-24`
- **问题**: 生产只使用 Gin 默认 middleware、CORS 和 Performance；项目已有的统一 ErrorHandler、路径/查询/JSON 校验、IP 限流和 SecurityHeaders 均未注册。上传等端点也没有统一请求体大小边界。
- **影响**: 各 handler 的输入/错误策略分裂，缺少 CSP/HSTS 等响应头和基础滥用保护；部分 panic 只能走 Gin 的通用恢复而没有一致错误格式/审计。
- **修复建议**: 明确全局与路由组 middleware 顺序，接入安全头、限流、统一错误和请求大小限制；对与 handler 重复的验证逐步合并并补集成测试。

### M-23 ✅ 前端会话初始化和 token 传输扩大泄露面
- **位置**: `web/react-app/src/store/index.ts:47-86`、`web/react-app/src/api/client.ts:15-21`、`web/react-app/src/hooks/useWebSocket.ts:51-66`
- **问题**: store 模块加载时直接 `JSON.parse(localStorage)`，任一损坏值会在 React 挂载前抛错导致白屏。JWT 长期保存在 localStorage，可被任意 XSS 读取；WebSocket 又把 token 放入 query string，可能进入代理/访问日志和诊断信息。
- **修复建议**: 对持久化数据做 schema 校验和异常回退；优先使用 Secure/HttpOnly/SameSite cookie 或短期内存 token，WebSocket 通过同源 cookie或受控子协议认证，避免 URL 携带凭据。

### M-24 ✅ 前端 RBAC 与后端权限模型不一致
- **位置**: `web/react-app/src/App.tsx:23-32`、`:112-137`；`web/react-app/src/pages/Settings/index.tsx:88-144`、`:436+`；`web/react-app/src/pages/Users/index.tsx:43-83`、`:457+`
- **问题**: `ProtectedRoute` 只检查登录，所有页面路由均不校验页面权限；Settings 和 Users 的主要 UI 又只看兼容字段 `is_admin`，没有使用后端角色/permission。拥有相应细粒度权限但非 admin 的用户看不到功能，无权限用户仍可进入页面并触发失败请求。
- **修复建议**: 建立 route metadata + permission guard，菜单、页面和按钮共用 `PERMISSIONS`；`is_admin` 只保留超级管理员快捷判定，不能替代角色权限。

### M-25 ✅ 前端 WebSocket 重连后会丢失日志订阅
- **位置**: `web/react-app/src/hooks/useWebSocket.ts:45-172`；`web/react-app/src/components/LogViewer.tsx:80-172`、`:223-238`
- **问题**: 手动 `disconnect()` 将 `shouldReconnectRef` 设为 false，但公开的 `reconnect()` 只是再次调用 `connect`，不会恢复自动重连。LogViewer 断线时不重置 `isSubscribedRef`，连接恢复后也没有基于 `isConnected` 重订阅；订阅/清理 effects 缺少 node、process、连接状态等依赖，切换目标时可继续订阅旧日志。
- **修复建议**: 让 reconnect 明确恢复策略；在 open/close 时维护订阅状态并重放期望订阅集合；将订阅封装进带完整依赖和稳定 callback 的 hook。

### M-26 ✅ 前端异步请求存在循环、竞态和重叠轮询
- **位置**: `web/react-app/src/pages/Profile/index.tsx:17-47`、`web/react-app/src/pages/Discovery/index.tsx:121-205`、`web/react-app/src/pages/Users/index.tsx:104-170`、`web/react-app/src/pages/Settings/index.tsx:88-187`、`web/react-app/src/hooks/useAutoRefresh.ts:10-26`
- **问题**: Profile effect 依赖 `user`，而 `loadProfile → setUser` 每次创建新对象，会形成持续请求循环。Discovery 注释称 debounce，实际每次输入立即请求且旧响应可覆盖新 CIDR；Users 快速切换目标时，旧用户的 preferences/node ACL 可写入当前表单。Settings 与通用自动刷新使用 `setInterval`，不等待上次请求完成，慢请求会重叠并乱序覆盖状态。
- **修复建议**: 拆分一次性加载与表单同步 effect；使用 debounce + AbortController/请求序号丢弃陈旧响应；轮询改为上次完成后再 schedule，并在卸载/参数变化时取消。

### M-27 ✅ 工程验证链无法执行真正的全量检查
- **位置**: `tools/*.go`、`web/react-app/package.json`、`web/react-app/tsconfig.json`、`.github/workflows/ci.yml.bak`
- **问题**: 四个工具文件同属一个目录/package 且各定义 `main()`，导致标准的 `go test ./...` 和 `go vet ./...` 编译失败。前端 lint script 存在但仓库没有 ESLint 配置；测试源码存在却没有 test script/runner 依赖，且被 tsconfig 排除；TypeScript `strict:false`。唯一 CI 检查文件被改名为 `.bak`，现有 workflow 只做 release 构建，不运行测试/lint/race。
- **修复建议**: 每个 CLI 放入独立 `cmd/...` 或使用 build tag；恢复可执行 lint/test 配置，将测试纳入类型检查；启用 CI 的 Go/前端 build、test、vet、lint 和关键 race 测试，再逐步收紧 TypeScript strict。

### M-28 ✅ `ParseToken` 未显式校验签名算法,存在 JWT 混淆攻击面
- **位置**: `internal/auth/jwt.go:58-68`(`ParseToken`)
- **问题**: 只检查 `token.Method` 是否为 `jwt.SigningMethodHMAC` 接口类型,未校验具体算法为 `HS256`。`alg` 头被篡改为其他 HMAC 变体时仍可能通过;`Logout`(`auth.go:240`)解析后仅按 `UserID+TokenVersion` 查库,不校验签发者/受众。
- **影响**: 算法混淆攻击可绕过签名校验,伪造令牌。
- **修复建议**: 显式断言 `token.Method.Alg() == "HS256"`;校验 `Issuer=="cesi"` 与 `ExpiresAt`;`Logout` 复用 `AuthenticateToken` 统一校验链。

### M-29 ✅ `UpdateUser` 并发会话状态冲突检测有逻辑漏洞,`is_admin` 竞态可覆盖提权
- **位置**: `internal/services/user.go:121-125`、`:127-155`
- **问题**: 冲突检测 `if existingUser.TokenVersion != user.TokenVersion && (passwordChanged || activeChanged)` 只在**改密/停用**时检查版本;并发修改 `is_admin` 或普通字段时被跳过。两个管理员并发更新同一用户:1) 只改 `full_name` 时后写覆盖先写(无冲突提示);2) 更严重——用户 A 并发**提权**(`is_admin=true`),用户 B 用旧快照(`is_admin=false`)做普通字段更新,`existingUser.IsAdmin != user.IsAdmin` 会以旧值计算并把**新提权覆盖回 false**。
- **影响**: 权限状态在并发更新下回退,与 H-07(低权限提权)形成组合风险。
- **修复建议**: 对 `is_admin`/`is_active` 变更同样要求 TokenVersion 匹配,或为整个用户更新引入版本号乐观锁;更新 SQL 加 `AND token_version = <loaded>` 谓词。

### M-30 ✅ 登录/认证的 `last_login` 更新与账号状态原子更新竞争
- **位置**: `internal/auth/auth.go:209`(`Login` 裸 `Update("last_login")`);`internal/services/user.go:68`(`Authenticate` 用 `UpdateFields`)
- **问题**: 登录与管理员**停用/改密**并发时,`last_login` 的 UPDATE 不带 TokenVersion 谓词,SQLite 写串行化下可能把 `is_active/token_version` 的原子更新覆盖(后写胜出,顺序不定)。`UpdateFields` 同样无版本谓词。
- **影响**: 停用/改密的会话吊销可能被并发的 last_login 更新部分抵消(取决于执行顺序),削弱 H-06 的原子性保证。
- **修复建议**: `last_login` 更新改用带版本谓词的条件更新(`WHERE id = ? AND token_version = ?`),或并入账号状态变更的同一事务。

### M-31 ✅ `checkHeartbeats` 持 `client.mu` 发送无缓冲 `unregister`,与 H-13 叠加可死锁
- **位置**: `internal/websocket/hub.go:703-729`
- **问题**: 对每个失败客户端,在 `client.mu.Lock()` 期间向无缓冲 `h.unregister` 发送;若 Hub 主循环正被 H-13 的 register 慢校验阻塞,`unregister` 无人消费,第一个失败客户端即卡死,后续心跳全部停摆,且 `client.mu` 被长期持有阻塞 writePump/readPump。
- **影响**: 与 H-13 同根,心跳断开路径在极端情况下死锁。
- **修复建议**: 与 H-13 一并修复——先收集待断开客户端(锁外),再统一经缓冲 channel 或批量处理;发送 `unregister` 使用 `select + default` 非阻塞,失败则直接 `conn.Close()`。

### M-32 ✅ `Hub.Close()`/`cleanup` 与 `unregister` 并发可能 double-close `client.send`
- **位置**: `internal/websocket/hub.go:367-374`(`unregister` 分支)、`:418-428`(`startCleanupWorker`)、`internal/websocket/lifecycle.go:152-186`(`closeAllConnections`)
- **问题**: 关闭流程中,同一客户端可能同时进入 `unregister` 与 `cleanup` 两条路径。`unregister` 分支有 `if _, ok := h.clients[client]` 守卫,但 `startCleanupWorker` 只查 `h.clients` 存在就 `close(client.send)`;若先经 `unregister` 删除并 close,随后 cleanup 队列里同一客户端再执行 `close(send)` → **double close panic**。H-06 的会话断开(失败时直接 `conn.Close()` 触发 readPump unregister)会提高该路径触发概率。
- **影响**: 关闭或大量断连时可能 panic 崩溃。
- **修复建议**: 统一用单一所有权(如 `closed` 标志 + 原子操作)保证 `send` 只 close 一次;`cleanup` 分支同样检查 `client.closed` 并加锁。

### M-33 ✅ 导入 JSON 流式解码 `UseNumber` 导致数字类型回归
- **位置**: `internal/api/data_management.go:97`(`decodeConfigImportPayload` 开启 `UseNumber`)、`:176-190`(`decodeConfigImportArray` 直接 `Decode(&item)`);下游 `internal/services/configuration.go:657-659`(`json.Marshal` → `json.Unmarshal`)
- **问题**: `UseNumber` 使数组项的数字被解析为 `json.Number`(字符串),随后 `ImportConfigurations` 对每个 `configData` 做 `json.Marshal` → `json.Unmarshal` 时,`json.Number` 被序列化为**带引号的字符串**(如 `"123"`),数字型配置值落库后类型被破坏。旧实现(`json.Unmarshal` 到 `interface{}`,得到 `float64`)是正确的。
- **影响**: H-05 修复引入的数字型配置导入回归,导入后配置值类型错误,读取方按 number 解析失败。
- **修复建议**: `decodeConfigImportArray` 对 `json.Number` 转回 `float64`,或改用不破坏类型的流式解码;补充数字型配置导入的回归测试。

### M-34 ✅ 导入取消与 `DeleteImportRecord` 的顺序未定义,存在状态回写竞态
- **位置**: `internal/services/data_management.go:589-603`(`DeleteImportRecord` 先 `cancelDataJob` 再删文件/记录);`:132-158`(`StartImport` 的 job 内 `UpdateImportStatus`)
- **问题**: 删除记录时先 cancel job 再删文件+记录,但 job 的 `defer s.cleanupImportSource(record)` 与 handler 的 `os.Remove(record.SourceFile)` 可能并发删除同一文件(幂等尚可);更关键的是 job 在记录删除**之后**才执行 `UpdateImportStatus`/`cleanupImportSource` 的 `Where("id = ?").Update(...)` 会 `RowsAffected=0` 静默无错,状态机与删除顺序未定义。
- **影响**: 删除与后台导入并发时,可能残留文件或产生幽灵状态更新;无崩溃但行为不确定。
- **修复建议**: 删除时等待 job 真正终止(等待 goroutine 或轮询 `dataJobCancels` 清空)再删文件;或为 job 增加显式 `cancelled` 终态并跳过状态回写。

### M-36 ✅ 日志级别解析用子串匹配,误把含 error 字样消息判为 ERROR
- **位置**: `internal/supervisor/node.go:477-487`(`extractLogLevel`)
- **问题**: 运行时实测发现。`extractLogLevel` 采用 `strings.Contains(line, level)` 的**子串匹配**,且 ERROR 在优先级列表最前。Spring Boot 等日志消息里常含 `BasicErrorController.errorHtml`、`.error(`、`handleError`、`/error` 等方法/路径片段,整体 `ToUpper` 后即包含子串 "ERROR",导致本应 INFO 的一行被误判为 ERROR。
- **影响**: 进程日志查看与日志分析中日志级别严重失真(大量 INFO 显示为 ERROR),误导告警与排障。
- **修复建议**: 改为词元边界匹配,不再用无边界子串。
- **修复记录**: 2026-08-19 · `extractLogLevel` 重写为**词元边界匹配**:级别仅当其前后为合法边界(空白/括号/等号/管道/行首尾 等)时才算命中;前边界额外排除点号(`.`)、冒号(`:`)、连字符(`-`)、斜杠(`/`)(避免类名/方法/路径如 `BasicErrorController.errorHtml`、`report:ERROR`、`/error` 误报),后边界排除左括号(`(`)与点号(`.`)(避免 `error(` 方法调用误报);支持 `Level=ERROR` 键值写法;`WARN` 归一化为 `WARNING`。新增 `isLogLevelBoundary`/`isLogLevelAfterBoundary` 辅助函数。`go build ./...` 通过;新增 `internal/supervisor/log_level_test.go`(`TestExtractLogLevel` 13 例 + `TestParseLogEntriesLevels` 端到端 5 行混合日志),均通过。

### M-38 ✅ XML-RPC startProcess/stopProcess 未显式传 wait,慢操作被 HTTP 5s 超时误报
- **位置**: `internal/supervisor/xmlrpc/supervisor.go:309-341/344-391`(`StartProcess`/`StopProcess`)、`internal/supervisor/xmlrpc/client.go:43-46`(HTTP 超时)
- **问题**: 对照 supervisord 官方 API(https://supervisord.org/api.html)复审发现。`startProcess(name, wait=True)` / `stopProcess(name, wait=True)` 官方签名默认 `wait=True`(同步:阻塞到进程完全启动/停止才返回,结果真实准确)。本项目调用时**未显式传 wait 参数**(省略即默认 True 同步),而 XML-RPC HTTP 客户端超时仅为 **5s** —— 慢启动/优雅停止的进程在同步等待期间超过 5s,HTTP 层先超时误报失败,但 supervisord 后台仍继续执行,导致返回结果与真实状态不一致(操作实际成功却报失败/触发无谓重试)。
- **影响**: 进程启动/停止的准确性受损:慢操作被误判失败;上层 timeoutManager(30s)与 circuitBreaker 因 HTTP 5s 先超时无法正确反馈真实结果。
- **修复建议**: 显式传 `wait=true` 保持官方同步语义(保证返回结果准确),并将 HTTP 客户端超时提升到大于 `SingleOperationTimeout`(30s)。
- **修复记录**: 2026-08-19 · `StartProcess`/`StopProcess` 调用改为 `[]interface{}{name, true}`(显式 wait=true,官方同步语义,supervisord 阻塞到进程完全启动/停止才返回,结果真实准确);`client.go` HTTP 客户端超时 5s→35s(大于 SingleOperationTimeout 30s,由上层超时管理优先负责,避免 HTTP 层误截断)。`encodeValue` 布尔编码已用测试验证(`true`→`<boolean>1</boolean>`)。批量操作维持逐进程遍历(不强制要求进程编组,不使用 startProcessGroup/stopProcessGroup)。`go build ./...`、`go test ./internal/supervisor/...`、`go vet` 全部通过;服务已重建重启(PID 93338)。

### M-39 ✅ 新增官方 XML-RPC 方法(历史日志分页/清空日志/配置管理)
- **位置**: `internal/supervisor/xmlrpc/supervisor.go`(`ReadProcessStdoutLog`/`ReadProcessStderrLog`/`ClearProcessLogs`/`GetAllConfigInfo`/`ReloadConfig`)、`internal/supervisor/node.go`、`internal/supervisor/service.go`、`internal/api/nodes.go`、`internal/api/api.go` 路由、`web/react-app/src/api/nodes.ts`、`web/react-app/src/components/LogViewer.tsx`、`web/react-app/src/i18n/*.ts`
- **问题/需求**: 对照 supervisord 官方 API(https://supervisord.org/api.html)罗列了项目未使用的方法,经确认后实现三项能力:
  1. **历史日志分页**: 官方 `readProcessStdoutLog(name,offset,length)` / `readProcessStderrLog(name,offset,length)`,按偏移读取历史日志(区别于 tail)。
  2. **清空日志**: 官方 `clearProcessLogs(name)`,清空并重开进程 stdout/stderr 日志。
  3. **配置管理**: 官方 `getAllConfigInfo()`(获取所有进程配置信息)与 `reloadConfig()`(重载配置,返回 added/changed/removed 三组)。
- **实现**: 
  - XML-RPC 层: 新增 5 个方法,复用 `parseTailLogResponse`/`extractStructBlocks`/`extractFieldValue` 解析;新增 `ProcessConfigInfo`/`ReloadResult` 结构、`extractStringArrayField`(解析 reloadConfig 的数组字段)。
  - Node/Service 层: 包装 `ReadProcessLogs`(按 logType 分派 stdout/stderr)、`ClearProcessLogs`、`GetAllConfigInfo`、`ReloadConfig`;ClearProcessLogs/ReloadConfig 走 timeoutManager。
  - HTTP 层: 新增 4 个路由 —— `GET /nodes/:node/processes/:proc/logs/page`、`POST /nodes/:node/processes/:proc/logs/clear`、`GET /nodes/:node/processes/configs`、`POST /nodes/:node/processes/reload-config`,均挂权限校验。
  - 前端: `nodes.ts` 新增 4 个 API;`LogViewer.tsx` 新增"加载更早(Load older)"分页按钮(调 readProcessLogs,去重并入现有条目)、清除按钮改为调 `clearProcessLogs`(带确认弹窗);新增 i18n 键(en/zh)。
- **验证**: `go build ./...`、`go test ./internal/supervisor/... ./internal/api/...` 全通过;`npx tsc --noEmit` 0 错误、`npm run build` 成功;新路由经 curl 验证返回 401(已挂鉴权,与既有路由一致);修复了部署时发现的历史问题——`./superview.sh stop` 因 PID 文件与实际监听进程不一致无法停掉旧二进制(旧 PID 12380 持续占用 8081 服务旧代码),手动 `pkill` 后干净重启(PID 147265)新路由才生效。

### M-37 ✅ 自动刷新固定间隔,打断用户正在进行的弹窗/按钮操作
- **位置**: `web/react-app/src/hooks/useAutoRefresh.ts`(全量)
- **问题**: 运行时实测发现。`useAutoRefresh` 用固定 `setInterval` 定时触发刷新回调(如进程页每 N 秒拉取 `/api/processes/aggregated`,看日志时控制台持续刷请求),且完全**不感知用户交互**——即使正在日志查看弹窗内阅读/操作、点击批量操作等,定时刷新仍照常发起,数据更新引发重渲染,打断/干扰正在进行的手上操作。
- **影响**: 用户查看日志或做操作时页面被定时刷新打断,体验差,长操作(如批量启停)进行中被列表重取干扰。
- **修复建议**: 自动刷新感知用户交互与弹窗状态,交互/弹窗打开时暂停。
- **修复记录**: 2026-08-19 · `useAutoRefresh` 增加两级暂停机制:
  1. **交互抑制**:监听全局 mousemove/mousedown/click/wheel/keydown/touchstart/touchmove,最近一次交互的时间戳;距最近交互 `pauseAfterInteractionMs`(默认 15s)内的定时 tick 直接跳过。鼠标/触摸移动做了 800ms 节流。参数 `pauseAfterInteractionMs`、`interactionEvents` 可配置。
  2. **浮层抑制**:新增 `hasOpenOverlay()`(检测页面存在未隐藏的 antd Modal 遮罩 `.ant-modal-wrap` 或打开中的 Drawer `.ant-drawer-open`),任一弹窗/抽屉打开时暂停自动刷新——彻底解决日志查看弹窗期间被刷新的问题。
  4 个调用方(Dashboard/Nodes/Processes/Logs)不受影响。`npx tsc --noEmit`(0 错误)、`npm run build` 通过;确认新逻辑已进 `useAutoRefresh-D_jGzttf.js` chunk 并被服务端 200 服务。

### M-35 ✅ `sendInitialData` 存在 TOCTOU 竞态,可能向已关闭通道发送数据导致 panic
- **位置**: `internal/websocket/hub.go:791-835`(`sendInitialData`)
- **问题**: 第七轮复审发现。注册后 `go h.sendInitialData(client)` 异步执行;函数先 `h.clientsMu.RLock()` 检查 `exists`,释放锁后才构建数据并 `select { case client.send <- data: }`。若客户端在检查与发送之间断开,`unregister`/`cleanupWorker` 已执行 `closeSendOnce.Do(close(client.send))`,对已关闭通道的发送**必然 panic**(select 无法拦截,只有阻塞/默认分支保护)。低概率但触发即进程崩溃。
- **影响**: 客户端在握手后立即断开(如页面刷新、网络抖动)时可能触发 panic,导致整个进程崩溃。
- **修复建议**: 用 recover 包裹发送,或对 `client.send` 的写入统一走带 recover 的 helper。
- **修复记录**: 2026-08-08 · 新增 `trySend(ch, data)` helper(recover + select-default);`sendInitialData` 改用 `trySend`,通道满或已关闭时仅记日志不再 panic。`go build ./internal/websocket/...` 通过。

---

## 🟢 Low

### L-01 ✅ 建节点后重复查询
- **位置**: `internal/services/scanner.go:608`
- **问题**: 建节点后又 `GetByName` 查一次拿 ID，可复用刚创建的对象。
- **修复记录**: 2026-06-16 · `registerDiscoveredNode` 改为返回创建的 `node.ID`（0 表示已存在/失败）；`buildDiscoveryResult(taskID, probe, nodeID)` 优先用传入 ID，仅在节点已存在时回退 `GetByName`。新建节点路径不再多查一次。

### L-02 ✅ 迁移代码忽略错误
- **位置**: `internal/database/database.go:369` 起
- **问题**: 大量 `db.Exec(...)` 不查错误。
- **修复记录**: 2026-06-16 · 新增 `execMigration(db, step, sql, args...)`（失败时 `zap.Warn`），`fixEmptyCategories` 中索引创建/分类修复/PRAGMA/DROP/恢复 INSERT 等尽力而为语句全部改用之。

### L-03 ✅ Hub 构造体重复
- **位置**: `internal/websocket/hub.go:190-235`（`NewHub` vs `NewHubWithConfig`）
- **问题**: 两个构造函数几乎重复。
- **修复记录**: 2026-06-16 · `NewHub` 改为 `return NewHubWithConfig(service, GetDefaultWebSocketConfig())`，消除重复。`go test ./internal/websocket/` 通过。

### L-04 ✅ 分页实现不统一
- **位置**: `internal/database/repository.go:59`（`Paginate`，封顶 100）未被 `data_management` 复用。
- **问题**: 两套分页实现。
- **修复记录**: 2026-06-16 · 随 M-04 一并修复——`listPaginatedRecords` 改用 `database.Paginate` scope，data_management 三处列表统一走带上限保护的分页（注意：pageSize 现封顶 100）。

### L-05 ✅ 遗留 `internal/loggers` 子系统未接入主日志链且轮转会死锁
- **位置**: `cmd/main.go:266-268`；`internal/loggers/activity_log.go:32-57`、`:143-147`；`internal/loggers/file_logger.go:33-80`；`internal/api/logs.go`
- **问题**: 应用初始化遗留的全局 ActivityLogService，但实际业务使用 `internal/services.ActivityLogService`，前者基本无人调用且未 Close，白白打开文件句柄。若真的写到轮转阈值，`rotate()` 持有 mutex 后调用再次加锁的 `openFile()`，会自死锁。旧 `internal/api/logs.go` 未挂路由，且按 JSON 行读取，与 FileLogger 的文本格式不兼容。
- **修复建议**: 删除遗留子系统或合并为唯一实现；若保留，修复锁层次、生命周期和文件格式，并为轮转补测试。

### L-06 ✅ CLI/维护工具的边界检查与敏感输出不安全
- **位置**: `cmd/main.go:202-224`；`tools/dump_users.go`、`tools/reset_admin_password.go`、`tools/verify_admin_password.go`；`internal/models/data_management.go:11-15`
- **问题**: `create-admin` 遇到尾部 flag 时直接访问 `os.Args[i+1]` 可越界 panic。维护工具会把明文新密码、环境密码及数据库密码哈希打印到终端/CI 日志；`generateID()` 忽略 `crypto/rand.Read` 错误。
- **修复建议**: 使用 `flag`/cobra 等标准解析，秘密只从安全输入读取且永不回显，哈希输出需显式 debug 开关；传播随机数生成错误或使用 UUID API。

### L-07 ✅ Logs 页搜索只过滤当前页，导出/删除却忽略搜索词
- **位置**: `web/react-app/src/pages/Logs/index.tsx:67-164`、`:263-271`、`:328-424`
- **问题**: `searchText` 仅对已加载的单页数据做客户端过滤，不进入服务端 filters；分页总数仍是全量，导出和“清理筛选日志”也不包含搜索词。界面容易让用户误以为搜索、导出和删除作用于同一集合。
- **修复建议**: 将 search 作为后端过滤条件统一用于列表、导出和删除，或明确标注“仅筛选当前页”并禁止带该条件的批量操作。

### L-08 ✅ `usePerformanceMonitor(enabled=true)` 会无限重渲染
- **位置**: `web/react-app/src/hooks/usePerformanceMonitor.ts:14-58`
- **问题**: 无依赖数组的 effect 每次 render 都调用 `setMetrics`，启用后形成 render→effect→setState 循环。目前主界面未使用该 hook，因此暂列 Low，但一旦接入页面会立即触发。
- **修复建议**: 使用 ref 记录指标或给采样 effect 明确依赖/节流，避免每次采样都无条件更新 state，并增加 hook 测试。

### L-09 ✅ 启停脚本与 Gitea 发布流程缺少安全校验
- **位置**: `superview.sh:20-76`；`.gitea/workflows/release.yml:27-70`
- **问题**: PID 文件只验证进程存在，不验证命令行/可执行文件身份，陈旧 PID 被复用时可能终止无关进程。发布 workflow 开启 `set -x` 后执行带 token 的 curl，扩大秘密暴露面；附件上传只打印 HTTP 状态，不因失败退出，release 可在缺包时仍显示成功。
- **修复建议**: PID 文件同时记录并校验进程启动时间/可执行路径，优先使用 systemd；关闭 xtrace 或仅包围无秘密步骤，curl 使用 `--fail-with-body` 并逐附件验证结果。

### L-10 ✅ 登录页和重置工具持续宣传弱默认口令
- **位置**: `web/react-app/src/pages/Login.tsx:102-104`；`tools/reset_admin_password.go:14-21`
- **问题**: 登录页固定展示 `Default: admin / 123456`，重置工具在无参数时也默认把管理员密码设为 `123456`。即使正式配置已支持环境变量，这些提示和 fallback 仍容易被直接用于部署。
- **修复建议**: 删除默认凭据提示，首次启动强制生成/设置高强度密码；重置工具必须显式输入密码或生成一次性随机密码，并强制首次登录修改。

### L-11 ✅ 新增导入解码测试覆盖不完整
- **位置**: `internal/api/data_management_test.go:108-136`(旧 `normalizeConfigImportPayload` 用例)、`TestDecodeConfigImportPayloadSupportsChunkedReader`
- **问题**: 新 `decodeConfigImportPayload` 的顶层 `configs` 别名分支、`environment_variables` 缺失、对象内嵌套 `data` 非对象(如数组)等路径无测试;数字型配置项(关联 M-33)无回归测试;`oneByteReader` 只覆盖 chunked 一种场景。
- **修复建议**: 为新解码入口补齐顶层别名/缺字段/畸形数字/大文件超限用例,并增加与 `ImportConfigurations` 的集成回归测试。

### L-12 ✅ 导入落盘文件名与 `import_type` 无关
- **位置**: `internal/api/data_management.go:412`(`saveImportUpload`)
- **问题**: 上传文件名恒为 `<uuid>.json`,与 `import_type`(users/configs/full_backup)无关,且 `CreateImportRecord` 的 `Name` 不含文件原名;下载/审计时无法从记录还原原始文件名。
- **修复建议**: 文件名携带 `import_type` 前缀或保留原文件 base64 名称字段;低影响,可随 M-10 的路径规范一并处理。
- **修复记录**: 2026-08-08 · `saveImportUpload` 接受 `importType` 参数,文件名前缀改为 `{importType}_{uuid}.json`;`CreateImportRecord` 的 `Name` 包含 `{importType}_import_{时间}`。`go test ./internal/api/ -run TestSaveImportUploadIncludesImportTypePrefix` 通过。

### L-14 ✅ RestoreBackup 非命名返回值导致 panic 恢复后虚假成功
- **位置**: `internal/services/configuration.go:461-482`(RestoreBackup 函数签名与 defer recover)
- **问题**: 第九轮复审发现。`RestoreBackup` 使用 `error` 普通返回值,defer 中的 `recover()` 回滚事务后**无法设置返回错误**——调用方收到 `nil`(成功)。若恢复过程中 panic(如类型断言失败),调用方会认为备份恢复成功,数据库实际未变化。`ImportConfigurations` 已修复为命名返回值,但 `RestoreBackup` 在 M-15 修复中被遗漏。
- **影响**: 恢复备份时若发生 panic,用户看到"恢复成功"但数据未恢复,属于虚假成功。
- **修复建议**: 改为命名返回值,recover 时设置 `err = fmt.Errorf(...)`。
- **修复记录**: 2026-08-08 · `RestoreBackup` 签名改为 `(err error)` 命名返回值,defer recover 设置 `err`;提交失败时也调用 `tx.Rollback()`。`UpdateMultipleSettings`/`ResetToDefaults`(system_settings.go)的 defer recover 改为 `panic(r)` 重抛给全局 ErrorHandler。`fixSystemSettingsForeignKey`(database.go)同样改为 `panic(r)` 重抛(启动迁移函数,panic 后崩溃进程避免在损坏 schema 上运行)。`go build ./...`、`go vet ./...` 通过。

### L-15 ✅ CSP 策略阻断 Ant Design 运行时内联样式
- **位置**: `internal/middleware/validation.go:174`(SecurityHeaders 中间件)
- **问题**: 运行时实测发现。CSP 头为 `default-src 'self'`,但**未显式声明 `style-src`**,浏览器以 `default-src 'self'` 兜底。Ant Design(web/react-app)通过 CSS-in-JS 在浏览器运行时动态注入内联 `<style>`/`style` 属性,被 CSP 拦截(控制台批量报 `Applying inline style violates ... 'default-src 'self''`),导致布局、间距、动画等全部失效。
- **影响**: 前端 UI 样式严重损坏,属真实功能缺陷。
- **修复记录**: 2026-08-18 · CSP 改为 `default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'`。显式允许内联样式(运行时生成的样式哈希不可预测,无法用固定 hash 覆盖);`img-src` 补充 `data:`(antd 图标使用 data URI)。`script-src` 仍由 `default-src 'self'` 约束保持严格。重建后端并重启,`curl -I` 验证页面与 API 均返回新 CSP 头,前端 antd 资源 200 加载。

### L-16 ✅ 前端按 `.agents/front.md` 重构为 Lucide + Obsidian 沉浸式视觉
- **位置**: `web/react-app/src/` 全域(Index.css 设计系统、main.tsx、index.html、MainLayout、Login、Dashboard 及全部 20 余个页面/组件)
- **问题**: 原前端为 antd 默认浅色主题,图标混用 `@ant-design/icons`,并存在 1 处表情符号(💡),视觉水准未达 `.agents/front.md` 要求的 Awwwards 级设计品质(先锋视觉、实验排版、流畅动效、沉浸式、Lucide 统一图标、禁用表情符号)。
- **影响**: 界面视觉与品牌调性不统一,离顶级设计网站有差距;历史使用非 Lucide 图标、含 emoji。
- **修复记录**: 2026-08-18 ·
  - 新增 `index.css` 全域设计系统:暗色 "Obsidian glass" 主题(css 变量 tokens)、`--font-display/body/mono` 字体族、动效基元(sv-reveal/sv-float/sv-aurora)、glass 玻璃拟态组件、按钮、Ant Design 暗色覆盖(表格/卡片/Menu/Input/Modal/消息等)。
  - `main.tsx` 引入 `theme.darkAlgorithm` + 自定义 design tokens;`index.html` 引入 Google Fonts(Space Grotesk / Inter / JetBrains Mono)。
  - 后端 `validation.go` CSP 增量放行 `font-src https://fonts.gstatic.com` 与 `style-src ... https://fonts.googleapis.com`,同时保持 `script-src 'self'` 严格。
  - `MainLayout` 重构为沉浸式玻璃侧栏(固定浮动、毛玻璃、折叠切换、状态脉冲);`Login` 重构为 Cinematic hero(极光光晕 + 浮动图标 + 玻璃卡片);`Dashboard` 重构为数据可视化优先(渐变文字页眉、发光状态点、环形健康度、玻璃指标卡)。
  - 全部 23 个页面/组件将 `@ant-design/icons` 替换为 `lucide-react`(统一 `size`/`strokeWidth` 规范),并移除 Nodes/index.tsx:162 的表情符号 💡 → `<Lightbulb>`。
  - 清理依赖:`@ant-design/icons`、`@ant-design/pro-components` 已无人引用,自 package.json 移除;`vite.config.ts` 删除对应 manualChunks;补装 `tslib` 以维持 echarts-for-react 解析。
- **验证**: `npx tsc --noEmit`(0 错误)、`npm run build`(成功、无残余 antd-icons/antd-pro 死 chunk)、`go build ./...`(通过)、全量审计(0 表情符号、0 处 `@ant-design/icons`、0 处 `Outlined` 引用)、运行中服务 `curl` 验证登录/节点/Dashboard 数据端点 200、`curl -I` 确认新 CSP 头(含字体放行)已生效。经三轮检查(类型/构建、符号审计、全量+运行时复核)均无问题。

### L-17 ✅ 日志查看器文字与浅色背景融为一体不可见
- **位置**: `web/react-app/src/components/LogViewer.tsx:318-327`(日志容器样式)
- **问题**: 运行时实测发现。日志容器硬编码为浅色 `backgroundColor: '#fafafa'`、边框 `#d9d9d9`,但未显式设置容器文本颜色。前端重构为暗色 Obsidian 主题后,antd `theme.darkAlgorithm` 将默认文本色改为近白色(`colorText` ≈ #f4f6ff),落在浅色 #fafafa 背景上形成"白字白底",日志内容(时间戳、消息)几乎不可见。
- **影响**: 节点进程日志查看(如 `/api/nodes/node-11/.../logs/stream`)时日志与背景融为一体,用户无法阅读,属高暴露度 UI 缺陷。
- **修复记录**: 2026-08-18 · 将日志容器改为暗色终端风格:背景 `#080a12`、边框 `var(--hairline-strong)`、容器显式 `color: #d6def5`(等宽字体 `var(--font-mono)`);消息文本显式 `#d6def5`,时间戳 `#5d6886`(弱化但仍可读);行间加细分隔线 + hover 高亮(提升长日志可读性)。移除多余的浅色残留。空态文案用 `var(--text-low)`。
- **验证**: `npx tsc --noEmit`(0 错误)、`npm run build`(成功);确认修复已进 `LogViewer-*.js` 构建 chunk(含 `080a12`/`d6def5`)、该 chunk 与主入口均被服务端 200 服务、日志流接口 `curl` 鉴权后 200。浏览器硬刷新后日志在暗色背景下清晰可读。

### L-18 ✅ 活动日志操作列宽度不足,长操作类型溢出到相邻列
- **位置**: `web/react-app/src/pages/Logs/index.tsx:227-236`(活动日志表格"操作"列)
- **问题**: 运行时实测发现。活动日志表格的"操作"列固定宽度 150px,当操作类型为 `UPDATE SETTINGS BATCH` 这类长文本(前端渲染为 `UPDATE SETTINGS BATCH`)时,Tag 文本宽度超出列宽,显示被挤到相邻列。
- **影响**: 长操作类型在表格中视觉错乱,溢出污染旁边"来源/消息"列。
- **修复记录**: 2026-08-19 · "操作"列宽度由 150 加宽至 230;`Tag` 增加 `whiteSpace:'nowrap'` 防止换行/溢出、`maxWidth:'100%'`+`textOverflow:'ellipsis'` 超长省略,并加 `title` 悬浮提示完整文本。`npx tsc --noEmit`(0 错误)、`npm run build` 通过,构建产物已被服务端 200 服务。

### L-19 ✅ 进程页搜索后刷新,全屏 loading/数据替换冲掉搜索结果
- **位置**: `web/react-app/src/pages/Processes/index.tsx:44-55`(`loadProcesses`)+ `:164-170`(全屏 Spin)
- **问题**: 运行时实测发现。`loadProcesses` 每次执行都 `setLoading(true)`,而组件在 `loading` 为 true 时整体渲染 `<Spin>`(全屏加载圈)。用户搜索出结果后,若触达自动/WS/手动刷新,页面立即闪为加载圈且在数据返回前列表被替换,搜索结果视觉上"马上没了",非常不合理。
- **影响**: WebSocket 开启时进程状态频繁刷新,用户在搜索/查看结果时页面反复闪 loading + 列表跳变,搜索与操作体验割裂。
- **修复记录**: 2026-08-19 · `loadProcesses` 仅当当前无数据(`processes.length === 0`,即首次加载)时 `setLoading(true)`;后续刷新保持列表显示不闪全屏 Spin。数据返回后在函数内立即按当前 `searchText` 重新过滤 `filteredProcesses`(与既有 `useEffect([searchText,processes])` 一致),确保搜索结果不丢失。`npx tsc --noEmit`(0 错误)、`npm run build` 通过,构建产物已服务。

### L-13 ✅ WebSocket 节点 ACL 快照在节点权限撤销后不会刷新
- **位置**: `internal/websocket/hub.go:1042-1059`(HandleWebSocket 中构建 `allowedNodeIDs` 快照)
- **问题**: 第八轮复审发现。`HandleWebSocket` 在握手时查询 `NodeAccess` 构建 `allowedNodeIDs` 快照,该快照仅创建一次,永久缓存于 `Client` 结构。管理员通过 `UpdateUserNodeAccess` 撤销某节点的读取权限后,已建立 WebSocket 连接的 `allowedNodeIDs` 不被刷新,该客户端仍能持续通过实时推送(z.nodes_update、log_stream 等)收到被撤销节点的数据。REST API 不受影响(每请求重载用户)。
- **影响**: 节点 ACL 撤销在 WebSocket 实时通道上最长延迟一个连接生命周期(可能数小时),形成信息泄露。
- **修复建议**: 心跳检查时或 `token_version` 变化时重新加载 `allowedNodeIDs`;或 `allowedNodeIDs` 不缓存,每次广播时按 DB 查询。
- **修复记录**: 2026-08-08 · `Client` 新增 `allowedNodeIDsMu sync.RWMutex` 保护 `allowedNodeIDs`;新增 `refreshAllowedNodeIDs(db)` 方法从 DB 重载 ACL;心跳检查 `checkHeartbeats` 在会话验证通过后调用 `refreshAllowedNodeIDs` 刷新快照。`SendLogStreamToSubscribedClients` 和 `SendToSubscribedClients` 增加 `canAccessNode` 检查,确保实时推送路径也遵守 ACL。`go build ./...`、`go vet ./internal/websocket`、`go test -race ./internal/websocket/` 通过。

---

## 2026-08-08 第六轮全量修复

- **修复范围**: 全部 41 个 TODO 条目(H-07~H-14、M-01/M-10~M-34、L-05~L-12)。
- **修复方式**: 多子代理并行修复(WebSocket Hub、Node 数据竞争、Auth/JWT、RBAC、数据管理、前端) + 作者直接修复。
- **修复摘要**:
  - **H-07**: `users.go`/`roles.go` 添加 `IsSuperAdmin()` 守卫,系统角色仅有超级管理员可管理。
  - **H-08**: 客户端上下文字段 `allowedNodeIDs` (节点名→bool 映射),`sendInitialData`/广播按 ACL 过滤;`subscribe_node`/`request_node_update`/`subscribe_logs` 增加 ACL 检查。
  - **H-09**: 导出函数 `exportConfigs`/`exportAll` 掩码 `IsSecret` 配置值与敏感系统设置键。
  - **H-10**: `createFullBackup` 新增 `PRAGMA wal_checkpoint(TRUNCATE)` 确保 WAL 合并后复制。
  - **H-11**: `Node` 新增 `SetConnected`/`SetConnectionStatus` 安全设置器,`service.go` 全部 19 处裸字段访问改为锁保护方法。
  - **H-13**: 会话校验移至 `HandleWebSocket` 升级前,`register` 入队改为非阻塞 `select+default`。
  - **H-14**: `checkHeartbeats` 仅对 `auth.ErrSessionUnavailable` 断连,瞬态 DB 错误仅日志。
  - **M-01**: `exportAll` 错误由静默跳过改为返回错误,新增 `truncated` 标记。
  - **M-10**: `performBackup` 路径穿越防护,`createConfigBackup` 改用唯一临时目录,`createFullBackup` Walk 错误传播。
  - **M-11**: 告警查询 `page_size`≤0 默认 20,通知发送改为返回 501 Not Implemented。
  - **M-12**: 日志分析 `IsValidExportFormat` 接受 JSON,活动日志 CSV 改用 `encoding/csv`。
  - **M-13**: `TaskScheduler` 新增 `mu` 与 `cronEntries` 映射,预防重复注册。
  - **M-14**: `ProcessEnhancedHandler` 新增 `authorizeNodeAccess` 检查,11 个节点 handler 增加 ACL。
  - **M-15**: `RestoreBackup`/`ImportConfigurations` 使用命名返回值,`Create`/`Updates` 错误检查与回滚。
  - **M-16**: `main.go` 新增 `config.Validate`、`InitializeSystemRoles`/`InitializeSystemPermissions`/`AssignDefaultPermissionsToRole` 调用。
  - **M-17**: `RestartAllProcesses` 收集错误返回组合错误,进程列表在锁外迭代。
  - **M-18**: `watchRefreshIntervalChanges` 增加 `context.WithCancel` 信号,`go func() { defer close(watchDone) }()` 包装。
  - **M-19**: XML-RPC 响应使用 `io.LimitReader(10MB)`。
  - **M-20**: `fixEmptyCategories` 包装在 `db.Transaction` 内,增加错误回滚。
  - **M-21**: `TestEmail` 返回 501,`SetLogLevel` 调用真实 `logger.SetLogLevel`,`RequireEnabled` 中间件控制开发者工具开关。
  - **M-22**: `router.Use(middleware.NewValidationMiddleware().SecurityHeaders())` + `router.Use(middleware.ErrorHandler())`。
  - **M-23**: `store/index.ts` 新增 `safeParse` 辅助函数,`JSON.parse` 失败时回退默认值。
  - **M-24**: `App.tsx` 新增 `AdminRoute` 守卫组件,`Settings/index.tsx` 增加加载状态。
  - **M-25**: `useWebSocket.ts` 新增 `subscriptionsRef`,重连后自动恢复订阅。
  - **M-26**: `Profile/index.tsx` 使用 `loadedUserIdRef` 防止循环,`Discovery/index.tsx` 使用 `AbortController` 防止竞态。
  - **M-27**: `tools/*.go` 添加 `//go:build ignore` 标签,`go build/vet/test ./...` 不再因重复 main 失败。
  - **M-28**: `ParseToken` 显式校验 `token.Method.Alg() == "HS256"`。
  - **M-29**: `UpdateUser` 始终检查 `TokenVersion`,不再仅当 password/active 变更时;`revokeSessions` 包含 `is_admin` 变更。
  - **M-30**: `Authenticate`/`Login` 的 `last_login` 更新使用 `token_version` 谓词。
  - **M-31**: `checkHeartbeats` 先收集失败客户端,释放锁后批量非阻塞发送 `unregister`。
  - **M-32**: `Client` 新增 `closeSendOnce sync.Once`,`cleanupWorker` 检查 `client.closed` 后关闭。
  - **M-33**: `decodeConfigImportArray` 新增 `convertJSONNumberValues` 将 `json.Number` 还原为 `float64`/`int64`。
  - **M-34**: `DeleteImportRecord` 调用 `cancelDataJob` 后等待 150ms 再删除文件/记录。
  - **L-05**: `FileLogger.openFile` 改为不锁(调用方加锁),`main.go` 关闭时调用 `Close()`。
  - **L-06**: `create-admin` 参数解析添加 `i+1` 越界检查与未知参数报错。
  - **L-07**: 后端 `activity_log.go` 添加 `search` 过滤条件(LIKE 查询);前端 `Logs/index.tsx` 将 `searchText` 发送服务端。
  - **L-08**: `usePerformanceMonitor` 使用 `metricsRef` 储存,仅实际变化时更新状态。
  - **L-09**: `superview.sh` `is_running` 验证进程可执行文件路径;`.gitea/workflows/release.yml` 移除 `set -x`,curl 使用 `--fail-with-body`。
  - **L-10**: `Login.tsx` 移除默认凭据显示;`reset_admin_password.go` 要求 `-password` 显式输入(≥8 位),`-print-hash` 控制哈希输出。
  - **L-11**: 新增 `TestDecodeConfigImportPayloadSupportsConfigsAlias`、`TestMissingEnvironmentVars`、`TestNestedDataArray`、`TestPreservesNumericTypes` 等回归测试。
  - **L-12**: `saveImportUpload` 接受 `importType` 参数,文件名前缀改为 `{importType}_{uuid}.json`。

### 验证结果

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./internal/...` ✅ (全部包通过)
- `go test -race ./internal/auth ./internal/websocket ./internal/middleware` ✅
- `npx tsc --noEmit` ✅ (web/react-app)

---

## 2026-08-08 第七轮全量复审

- **审查范围**: 对第六轮全部修复做多轮代码复审:
  - 第 1 轮:WebSocket Hub/client(H-08 广播过滤、H-13 非阻塞注册、H-14 心跳、M-31/M-32、`sendInitialData`、`streamLogKey`、`filterNodesUpdateForClient`)
  - 第 2 轮:Supervisor 节点/服务(H-11 getter/setter 全覆盖、M-17 批量错误收集、M-18 生命周期)
  - 第 3 轮:Auth/JWT/RBAC(H-07 系统角色守卫、M-28 HS256、M-29/M-30 会话版本、M-14 节点 ACL)
  - 第 4 轮:数据管理/配置服务(H-09 秘密掩码、H-10 WAL checkpoint、M-01 错误传播、M-10 路径防护、M-15 事务、M-33 数字还原、M-34 取消时序)
  - 第 5 轮:前端(M-23 safeParse、M-24 RBAC、M-25 重连订阅、M-26 竞态、L-07/L-08)与工具/脚本(M-27 build tag、L-05~L-12)
- **复审结论**: 大部分修复质量良好,`go build ./...`、`go vet ./...`、`go test ./internal/...`、`go test -race ./internal/auth ./internal/websocket ./internal/middleware`、`npx tsc --noEmit` 全部通过。发现 **1 个新问题 M-35**(异步 `sendInitialData` 的 TOCTOU 竞态),已修复并记录。

### 验证结果

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test ./internal/...` ✅
- `go test -race ./internal/auth ./internal/websocket ./internal/middleware` ✅
- `npx tsc --noEmit` ✅ (web/react-app)

---

## 2026-08-08 第八轮持续复审

- **复审范围**: 对第六轮全部修复做持续多轮深度复审:
  - 第 1 轮:internal/api 其余 handlers(users/alerts/logs/discovery/configuration)
  - 第 2 轮:supervisor/xmlrpc + auth 权限检查 + session 校验
  - 第 3 轮:models/repository/config/cache/loggers + validation
  - 第 4 轮:前端(Login/App/WebSocket/Settings/Logs) + 工具脚本/CI
  - 第 5 轮:main.go 关闭顺序 + process_enhanced + system_settings + 安全边界
- **复审结论**: 前 5 轮未发现明显问题。第 6 轮深挖时发现 **L-13**(WebSocket 节点 ACL 快照在权限撤销后不刷新)——已修复。`go build ./...`、`go vet ./...`、`go test -race ./internal/websocket/` 通过。
- **最终结论**: 第八至十二轮持续复审共发现并修复 **L-13**(WebSocket ACL 快照)和 **L-14**(事务虚假成功模式 x 4 处)。第 11-12 轮连续两轮无发现,达到终止条件。

### 验证结果

- `go build ./...` ✅
- `go vet ./...` ✅
- `go test -race ./internal/websocket/` ✅
- `go test ./internal/api/ ./internal/auth/ ./internal/websocket/ ./internal/services/` ✅

---

## 维护协议

**每次修复时：**

1. **读取**本文档，选定要修的条目（优先 High）。
2. 将条目状态改为 🔧 **DOING**。
3. 实施修复。
4. 验证（编译/测试/手动）。
5. 状态改为 ✅ **DONE**，在「修复记录」填写：
   - 修复日期
   - 实际改动的文件:行
   - 一句话说明改法
   - 验证方式（如 `go test ./...` 通过 / 手动验证）
6. 更新顶部「进度总览」表格计数。

**后续 review 时：**

- 新发现的问题按 `H-NN` / `M-NN` / `L-NN` 续号追加，**不复用已删除的编号**。
- 已 DONE 的条目保留作为历史记录，不删除。
- 如条目经评估不修，标 ❌ **WONTFIX** 并注明原因。

---

_最后更新:2026-08-19 · 前端视觉重构(L-16)、日志查看器白底白字(L-17)、日志级别误判(M-36)、活动日志操作列加宽(L-18)、自动刷新打断操作(M-37)、XML-RPC wait 同步性(M-38)、新增官方 XML-RPC 方法(M-39)。共记录 72 项:0 TODO、71 DONE、1 WONTFIX(High 14、Medium 40、Low 18)。_

_本轮验证状态:`go build ./...` 通过、`go vet ./...` 通过;`npx tsc --noEmit` 通过(web/react-app)、`npm run build` 通过;全量审计无表情符号、无 `@ant-design/icons` 残留;运行中服务 `curl` 验证登录/数据端点 200,新 CSP 头(含字体放行)已生效。tools/ 已添加 `//go:build ignore` 标签,`go build/vet/test ./...` 不再因此失败。_

_历史记录:2026-06-17 第三轮曾将除 M-07 外的原 18 项标记为已处理;2026-08-06 第四轮复核发现 H-05、M-01 只完成部分修复,重新打开;2026-08-07 第五轮评审 H-05/H-06/H-12 修复实现,确认 H-06 的 WS 会话吊销存在结构性通道问题(H-13/H-14),H-05 的流式解码引入数字类型回归(M-33),H-06 的并发乐观锁留了 is_admin 竞态窗口(M-29)。_
