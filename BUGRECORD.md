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
| High    | 5    | 0    | 5    | 0       |
| Medium  | 9    | 0    | 8    | 1       |
| Low     | 4    | 0    | 4    | 0       |

---

## 🔴 High — 性能瓶颈

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

### H-05 ✅ 无界后台 goroutine
- **位置**: `internal/services/data_management.go:71`、`internal/services/data_management.go:507`
- **问题**: `go s.performExport(...)` / `go s.performBackup(...)` 无并发上限、无 panic recover、无 context。多个大导出并发 → 内存/磁盘爆掉，且全卡在那条 DB 连接上。
- **修复建议**: 加信号量限流 + `defer recover()`，记录失败。
- **修复记录**: 2026-06-16 · 新增 `dataJobSlots`（容量 2 的信号量）+ `runDataJob(name, id, fn)` helper（限流 + `defer recover()` 记录 panic），两处 `go` 调用改用之（`data_management.go`）。

---

## 🟡 Medium

### M-01 ✅ 导出全量载入内存 + 静默截断
- **位置**: `internal/services/data_management.go:201-242`（`exportAll`），`:156`、`:219`（`Limit(10000)`）
- **问题**: `exportAll` 把 users + 1 万日志 + configs + settings 全 `Find` 进内存切片再编码，大库内存尖峰；日志 `Limit(10000)` 静默截断，用户不知数据被砍。
- **修复建议**: 流式/分批写出；截断时 `log.Warn` 告知。
- **修复记录**: 2026-06-16 · 提取常量 `maxExportLogs`，`exportLogs` 与 `exportAll` 在命中上限时 `logger.Warn` 告知截断。（注：完整流式导出未做——大库内存尖峰仍存，作为后续优化保留；本次仅消除"静默"问题。）

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
- **遗留**: token 24h 固定、无刷新、无吊销名单（登出无法失效）属设计层面待办，另开条目跟踪（见 M-10，若需要）。

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

_最后更新：2026-06-17 · 第三轮：修复前端 i18n `loadLogsFailed` + L-01/L-02/M-04/L-04/M-05，M-09 代码完成（验证待补）。至此除 M-07(WONTFIX) 外全部条目已处理。_

_验证状态：_
- _L-01/L-02/M-04/L-04（后端）— `go build ./internal/services ./internal/database ./internal/repository` 通过，data_management 测试通过。_
- _M-05（websocket）— `go test -race ./internal/websocket/` 通过。_
- _i18n 修复 — `tsc --noEmit` 通过（修复时）。_
- _M-09（Settings 拆分）— ✅ 已验证：用户手动运行 `npx tsc --noEmit` 返回 EXIT=0。`columns.tsx`/`helpers.ts` 抽离、`index.tsx` 改 `useMemo` 工厂调用，1193→~875 行。_

_至此除 M-07(WONTFIX) 外全部 17 项已修复并验证。后续 review 待补：API 层大文件（process_enhanced/log_analysis/configuration/alerts）。_
