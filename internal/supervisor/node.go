package supervisor

import (
	"fmt"
	"superview/internal/errors"
	"superview/internal/logger"
	"strings"
	"sync"
	"time"

	"superview/internal/supervisor/xmlrpc"
	"go.uber.org/zap"
)

// ErrNodeNotConnected 节点未连接错误
var ErrNodeNotConnected = errors.NewConnectionError("node", nil)

// LogTimezone 日志解析使用的时区，默认为本地时区
// 可通过 SetLogTimezone 设置
var LogTimezone = time.Local

// SetLogTimezone 设置日志解析使用的时区
func SetLogTimezone(tzName string) error {
	if tzName == "" || tzName == "Local" {
		LogTimezone = time.Local
		return nil
	}
	
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		logger.Warn("Invalid timezone, using local timezone",
			zap.String("timezone", tzName),
			zap.Error(err))
		LogTimezone = time.Local
		return err
	}
	
	LogTimezone = loc
	logger.Info("Log timezone set", zap.String("timezone", tzName))
	return nil
}

type Node struct {
	Name         string
	Environment  string
	Host         string
	Port         int
	Username     string
	Password     string
	
	// 需要同步保护的字段
	mu           sync.RWMutex
	IsConnected  bool
	LastPing     time.Time
	Processes    []Process
	
	client       *xmlrpc.SupervisorClient
}

// Process 表示一个进程 - 符合 Supervisor API 规范
type Process struct {
	Name          string        `json:"name"`           // 进程名称
	Group         string        `json:"group"`          // 进程组名称
	State         int           `json:"state"`          // 状态码
	StateString   string        `json:"state_string"`   // 状态名称
	StartTime     time.Time     `json:"start_time"`     // 启动时间
	StopTime      time.Time     `json:"stop_time"`      // 停止时间
	PID           int           `json:"pid"`            // 进程 PID
	ExitStatus    int           `json:"exit_status"`    // 退出状态码
	StdoutLogfile string        `json:"stdout_logfile"` // stdout 日志文件
	StderrLogfile string        `json:"stderr_logfile"` // stderr 日志文件
	Uptime        time.Duration `json:"uptime"`         // 运行时间
	UptimeHuman   string        `json:"uptime_human"`   // 人类可读的运行时间
	SpawnErr      string        `json:"spawn_err"`      // 启动错误
	Now           time.Time     `json:"now"`            // 当前时间
}

type LogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Level       string    `json:"level"`
	Message     string    `json:"message"`
	Source      string    `json:"source"`
	ProcessName string    `json:"process_name"`
	NodeName    string    `json:"node_name"`
}

// LogStream 表示日志流 - 符合 Supervisor API 规范
type LogStream struct {
	ProcessName string     `json:"process_name"`
	NodeName    string     `json:"node_name"`
	LogType     string     `json:"log_type"`     // "stdout" 或 "stderr"
	Entries     []LogEntry `json:"entries"`
	LastOffset  int        `json:"last_offset"`  // 下一次读取的偏移量
	Overflow    bool       `json:"overflow"`     // 是否有日志溢出
}

func NewNode(name, environment, host string, port int, username, password string) (*Node, error) {
	client, err := xmlrpc.NewSupervisorClient(host, port, username, password)
	if err != nil {
		return nil, err
	}

	return &Node{
		Name:        name,
		Environment: environment,
		Host:        host,
		Port:        port,
		Username:    username,
		Password:    password,
		client:      client,
		Processes:   make([]Process, 0),
	}, nil
}

func (n *Node) Connect() error {
	// 尝试获取进程信息来测试连接
	_, err := n.client.GetAllProcessInfo()
	
	n.mu.Lock()
	defer n.mu.Unlock()
	
	if err != nil {
		n.IsConnected = false
		return err
	}

	n.IsConnected = true
	n.LastPing = time.Now()
	return nil
}

func (n *Node) RefreshProcesses() error {
	n.mu.RLock()
	connected := n.IsConnected
	n.mu.RUnlock()
	
	if !connected {
		return ErrNodeNotConnected
	}

	logger.Debug("Refreshing processes for node",
		zap.String("node", n.Name),
		zap.String("host", n.Host),
		zap.Int("port", n.Port))

	processInfos, err := n.client.GetAllProcessInfo()
	if err != nil {
		logger.Error("Failed to get process info from node",
			zap.String("node", n.Name),
			zap.String("host", n.Host),
			zap.Int("port", n.Port),
			zap.Error(err))
		return err
	}

	logger.Debug("Retrieved process info from node",
		zap.String("node", n.Name),
		zap.Int("process_count", len(processInfos)))

	processes := make([]Process, len(processInfos))
	for i, info := range processInfos {
		// 转换时间戳
		var startTime, stopTime, nowTime time.Time
		if info.Start > 0 {
			startTime = time.Unix(info.Start, 0)
		}
		if info.Stop > 0 {
			stopTime = time.Unix(info.Stop, 0)
		}
		if info.Now > 0 {
			nowTime = time.Unix(info.Now, 0)
		}

		// 计算运行时长
		var uptime time.Duration
		var uptimeHuman string
		
		if info.State == 20 && info.Start > 0 && info.Now > 0 { // RUNNING状态
			// 使用当前时间和启动时间计算实际运行时间
			actualUptime := info.Now - info.Start
			uptime = time.Duration(actualUptime) * time.Second
			uptimeHuman = formatDuration(uptime)
		} else {
			// 对于非运行状态的进程，uptime为0
			uptime = 0
			uptimeHuman = "0s"
		}

		// 状态字符串映射 - 符合 Supervisor 规范
		stateString := getStateNameFromCode(info.State)

		processes[i] = Process{
			Name:          info.Name,
			Group:         info.Group,
			State:         info.State,
			StateString:   stateString,
			StartTime:     startTime,
			StopTime:      stopTime,
			PID:           getPIDForState(info.State, info.PID),
			ExitStatus:    info.ExitStatus,
			StdoutLogfile: info.StdoutLogfile,
			StderrLogfile: info.StderrLogfile,
			SpawnErr:      info.SpawnErr,
			Uptime:        uptime,
			UptimeHuman:   uptimeHuman,
			Now:           nowTime,
		}
	}

	n.mu.Lock()
	n.Processes = processes
	n.mu.Unlock()

	logger.Info("Successfully refreshed processes for node",
		zap.String("node", n.Name),
		zap.Int("process_count", len(processes)))

	return nil
}

// getStateNameFromCode 根据状态码获取状态名称 - 符合 Supervisor 规范
func getStateNameFromCode(state int) string {
	switch state {
	case 0:
		return "STOPPED"
	case 10:
		return "STARTING"
	case 20:
		return "RUNNING"
	case 30:
		return "BACKOFF"
	case 40:
		return "STOPPING"
	case 100:
		return "EXITED"
	case 200:
		return "FATAL"
	case 1000:
		return "UNKNOWN"
	default:
		return fmt.Sprintf("STATE_%d", state)
	}
}

// getPIDForState 根据进程状态返回正确的PID
// 只有运行状态的进程才有有效的PID
func getPIDForState(state int, originalPID int) int {
	switch state {
	case 20: // RUNNING
		return originalPID
	case 10: // STARTING - 可能有PID
		return originalPID
	default:
		// STOPPED, EXITED, FATAL等状态下PID应该为0
		return 0
	}
}

func (n *Node) StartProcess(name string) error {
	n.mu.RLock()
	connected := n.IsConnected
	n.mu.RUnlock()
	
	if !connected {
		return ErrNodeNotConnected
	}

	return n.client.StartProcess(name)
}

func (n *Node) StopProcess(name string) error {
	n.mu.RLock()
	connected := n.IsConnected
	n.mu.RUnlock()
	
	if !connected {
		return ErrNodeNotConnected
	}

	return n.client.StopProcess(name)
}

func (n *Node) RestartProcess(name string) error {
	n.mu.RLock()
	connected := n.IsConnected
	n.mu.RUnlock()
	
	if !connected {
		return ErrNodeNotConnected
	}

	// 先停止进程
	err := n.client.StopProcess(name)
	if err != nil {
		return err
	}
	
	// 等待一小段时间
	time.Sleep(100 * time.Millisecond)
	
	// 再启动进程
	return n.client.StartProcess(name)
}

func (n *Node) GetProcessLogs(name string) (map[string][]string, error) {
	n.mu.RLock()
	connected := n.IsConnected
	n.mu.RUnlock()
	
	if !connected {
		return nil, ErrNodeNotConnected
	}

	// 使用新的 API 签名 - 返回 (content, offset, overflow, error)
	stdout, _, _, err := n.client.TailProcessStdoutLog(name, 0, 500)
	if err != nil {
		return nil, err
	}

	stderr, _, _, err := n.client.TailProcessStderrLog(name, 0, 500)
	if err != nil {
		return nil, err
	}

	// 分割日志行，过滤空行
	stdoutLines := make([]string, 0)
	for _, line := range strings.Split(stdout, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			stdoutLines = append(stdoutLines, line)
		}
	}

	stderrLines := make([]string, 0)
	for _, line := range strings.Split(stderr, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			stderrLines = append(stderrLines, line)
		}
	}

	return map[string][]string{
		"stdout": stdoutLines,
		"stderr": stderrLines,
	}, nil
}

// GetProcessLogSize 获取进程日志文件当前大小（字节偏移量）
func (n *Node) GetProcessLogSize(name string) (int, error) {
	n.mu.RLock()
	connected := n.IsConnected
	n.mu.RUnlock()
	
	if !connected {
		return 0, ErrNodeNotConnected
	}

	// 调用 tailProcessStdoutLog(name, 0, 0) 获取当前文件大小
	_, fileSize, _, err := n.client.TailProcessStdoutLog(name, 0, 0)
	if err != nil {
		return 0, err
	}
	
	return fileSize, nil
}

// GetProcessLogStreamTail 从文件末尾读取最新日志
func (n *Node) GetProcessLogStreamTail(name string, maxLines int) (*LogStream, error) {
	n.mu.RLock()
	connected := n.IsConnected
	n.mu.RUnlock()
	
	if !connected {
		return nil, ErrNodeNotConnected
	}

	bytesToRead := maxLines * 200
	
	// 先获取文件大小
	_, fileSize, _, err := n.client.TailProcessStdoutLog(name, 0, 0)
	if err != nil {
		return nil, err
	}
	
	// 从文件末尾读取
	startOffset := fileSize - bytesToRead
	if startOffset < 0 {
		startOffset = 0
	}
	
	stdout, nextOffset, overflow, err := n.client.TailProcessStdoutLog(name, startOffset, bytesToRead)
	if err != nil {
		return nil, err
	}

	entries := n.parseLogEntries(stdout, "stdout", name)
	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
	}

	return &LogStream{
		ProcessName: name,
		NodeName:    n.Name,
		LogType:     "stdout",
		Entries:     entries,
		LastOffset:  nextOffset,
		Overflow:    overflow,
	}, nil
}

// GetProcessLogStream 获取结构化的日志流 - 从指定偏移量读取
func (n *Node) GetProcessLogStream(name string, offset int, maxLines int) (*LogStream, error) {
	n.mu.RLock()
	connected := n.IsConnected
	n.mu.RUnlock()
	
	if !connected {
		return nil, ErrNodeNotConnected
	}

	bytesToRead := maxLines * 200 // 估算每行 200 字节
	
	// 直接从指定偏移量读取
	stdout, nextOffset, overflow, err := n.client.TailProcessStdoutLog(name, offset, bytesToRead)
	if err != nil {
		return nil, err
	}

	// 解析日志为结构化条目
	entries := n.parseLogEntries(stdout, "stdout", name)

	// 限制返回的条目数
	if len(entries) > maxLines {
		entries = entries[len(entries)-maxLines:]
	}

	return &LogStream{
		ProcessName: name,
		NodeName:    n.Name,
		LogType:     "stdout",
		Entries:     entries,
		LastOffset:  nextOffset,
		Overflow:    overflow,
	}, nil
}

// parseLogEntries 解析日志文本为结构化条目
func (n *Node) parseLogEntries(logText, logType, processName string) []LogEntry {
	var entries []LogEntry
	
	lines := strings.Split(logText, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		
		entry := LogEntry{
			Timestamp:   time.Now(), // 默认当前时间，后续可以解析日志中的时间戳
			Level:       extractLogLevel(line),
			Message:     line,
			Source:      logType,
			ProcessName: processName,
			NodeName:    n.Name,
		}
		
		// 尝试解析时间戳
		if timestamp := extractTimestamp(line); !timestamp.IsZero() {
			entry.Timestamp = timestamp
		}
		
		entries = append(entries, entry)
	}
	
	return entries
}

// extractLogLevel 从日志行中提取日志级别
// 采用"词元边界"匹配而非子串匹配,避免把消息文本中的
// BasicErrorController.errorHtml / handleError / error( 等含子串的行误判为 ERROR。
// 支持的常见日志级别格式:
//   - 独立词元(空格分隔): "... INFO ..." / "...\tERROR\t..." / "Level=ERROR"
//   - 方括号形式: "[INFO]" "[WARN]" "[ERROR]"
// 级别按其严重程度降序判断,命中即返回(保证子串包含低优先级级别时不误报)。
func extractLogLevel(line string) string {
	upper := strings.ToUpper(line)

	// 按严重程度降序的级别列表(ERROR 与 WARNING 同义,统一为 ERROR)
	levels := []string{"FATAL", "ERROR", "WARN", "INFO", "DEBUG", "TRACE"}

	for _, level := range levels {
		// 普通词元匹配:级别前后须为单词边界(空格/制表/冒号/等号/左括号或行首行尾)
		if matchLogLevelToken(upper, level) {
			// 归一化别名
			if level == "WARN" {
				return "WARNING"
			}
			return level
		}
	}
	return "INFO" // 默认级别
}

// matchLogLevelToken 判断 word 是否为 line 中以单词边界分隔的独立词元,
// 同时兼容 "LEVEL=" 键值写法(如 "Level=ERROR")。
func matchLogLevelToken(line, word string) bool {
	// 键值形式:支持 "LEVEL=" 前无单词边界
	kv := word + "="
	if strings.Contains(line, kv) {
		return true
	}
	// 带前导字符(键值前可能有空格),如 " Level=",已由上面覆盖;这里再兼容 "=LEVEL" 后置写法
	kv2 := "=" + word
	if strings.Contains(line, kv2) {
		return true
	}

	for i := 0; i+len(word) <= len(line); i++ {
		if line[i:i+len(word)] != word {
			continue
		}
		beforeOK := i == 0 || isLogLevelBoundary(line[i-1])
		afterOK := i+len(word) == len(line) || isLogLevelAfterBoundary(line[i+len(word)])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

// isLogLevelBoundary 判断字符 c 是否可作为日志级别词元的"前"边界。
// 前边界为:空白符、方括号、圆括号、大括号、分号、等号、管道、行首等非字母数字字符。
// 注意:点号(.)、冒号(:)、连字符(-)、斜杠(/)故意**不作为**前边界——
// 它们多属于 Java 类名/方法调用(如 BasicErrorController.errorActually、ssl.errorLog、
// report:ERROR、/error),若放宽会把这类含子串的行误判为 ERROR。
func isLogLevelBoundary(c byte) bool {
	switch {
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9':
		return false
	case c == '.', c == ':', c == '-', c == '/':
		return false
	default:
		return true
	}
}

// isLogLevelAfterBoundary 判断字符 c 是否可作为日志级别词元的"后"边界。
// 级别后紧邻左括号( 表示方法调用(如 error(、handleError(),不算独立级别词元。
// 点号(.)也不作为后边界(如 error.foo 是成员访问)。
func isLogLevelAfterBoundary(c byte) bool {
	switch c {
	case '(':
		return false
	case '.':
		return false
	default:
		return isLogLevelBoundary(c)
	}
}

// extractTimestamp 从日志行中提取时间戳
func extractTimestamp(line string) time.Time {
	// 常见的时间戳格式
	formats := []string{
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006/01/02 15:04:05",
		"Jan 02 15:04:05",
	}
	
	// 使用配置的日志时区(而非本地时区)
	loc := LogTimezone
	
	for _, format := range formats {
		// 尝试从行首提取时间戳
		if len(line) >= len(format) {
			if t, err := time.ParseInLocation(format, line[:len(format)], loc); err == nil {
				// 返回本地时间，不转换为 UTC
				return t
			}
		}
		
		// 尝试查找时间戳模式
		for i := 0; i <= len(line)-len(format); i++ {
			if t, err := time.ParseInLocation(format, line[i:i+len(format)], loc); err == nil {
				return t
			}
		}
	}
	
	return time.Time{} // 返回零值表示未找到
}

func (n *Node) Serialize() map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	runningCount := 0
	for _, p := range n.Processes {
		if p.State == 20 { // RUNNING state
			runningCount++
		}
	}
	
	var lastPing interface{}
	if !n.LastPing.IsZero() {
		lastPing = n.LastPing
	}

	return map[string]interface{}{
		"name":           n.Name,
		"environment":    n.Environment,
		"is_connected":   n.IsConnected,
		"host":           n.Host,
		"port":           n.Port,
		"username":       n.Username,
		"last_ping":      lastPing,
		"process_count":  len(n.Processes),
		"running_count":  runningCount,
	}
}

func (n *Node) SerializeProcesses() []map[string]interface{} {
	n.mu.RLock()
	defer n.mu.RUnlock()
	
	var processes []map[string]interface{}
	for _, p := range n.Processes {
		processes = append(processes, map[string]interface{}{
			"name":           p.Name,
			"group":          p.Group,
			"state":          p.State,
			"state_string":   p.StateString,
			"start_time":     p.StartTime,
			"stop_time":      p.StopTime,
			"pid":            p.PID,
			"exit_status":    p.ExitStatus,
			"stdout_logfile": p.StdoutLogfile,
			"stderr_logfile": p.StderrLogfile,
			"uptime":         p.Uptime.Seconds(),
			"uptime_human":   formatDuration(p.Uptime),
			"now":            p.Now,
		})
	}
	return processes
}

// GetConnectionStatus 安全地获取连接状态
func (n *Node) GetConnectionStatus() (bool, time.Time) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.IsConnected, n.LastPing
}

// SetConnectionStatus 安全地设置连接状态和最后ping时间
func (n *Node) SetConnectionStatus(isConnected bool, lastPing time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.IsConnected = isConnected
	n.LastPing = lastPing
}

// SetConnected 安全地设置连接状态
func (n *Node) SetConnected(isConnected bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.IsConnected = isConnected
}

// GetProcessCount 安全地获取进程数量
func (n *Node) GetProcessCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.Processes)
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.1fh", d.Hours())
	}
	return fmt.Sprintf("%.1fd", d.Hours()/24)
}