package xmlrpc

import (
	"fmt"
	"strconv"
	"strings"
)

// ProcessInfo 符合 Supervisor XML-RPC API 规范的进程信息
type ProcessInfo struct {
	Name           string `json:"name"`           // 进程名称
	Group          string `json:"group"`          // 进程组名称  
	Start          int64  `json:"start"`          // UNIX 启动时间戳
	Stop           int64  `json:"stop"`           // UNIX 停止时间戳 (0 表示从未停止)
	Now            int64  `json:"now"`            // 当前 UNIX 时间戳
	State          int    `json:"state"`          // 状态码 (见 Supervisor 文档)
	StateName      string `json:"statename"`      // 状态名称
	SpawnErr       string `json:"spawnerr"`       // 启动错误描述
	ExitStatus     int    `json:"exitstatus"`     // 退出状态码
	StdoutLogfile  string `json:"stdout_logfile"` // stdout 日志文件路径
	StderrLogfile  string `json:"stderr_logfile"` // stderr 日志文件路径
	PID            int    `json:"pid"`            // 进程 PID (0 表示未运行)
	
	// 计算字段 (非 API 返回)
	Uptime         int64  `json:"uptime"`         // 运行时间 (秒)
	UptimeHuman    string `json:"uptime_human"`   // 人类可读的运行时间
}

type SupervisorClient struct {
	client *Client
}

func NewSupervisorClient(host string, port int, username, password string) (*SupervisorClient, error) {
	client, err := NewClient(host, port, username, password)
	if err != nil {
		return nil, err
	}
	return &SupervisorClient{client: client}, nil
}

// GetAllProcessInfo 获取所有进程信息 - 符合官方 API 规范
func (s *SupervisorClient) GetAllProcessInfo() ([]ProcessInfo, error) {
	result, err := s.client.Call("supervisor.getAllProcessInfo", nil)
	if err != nil {
		return nil, fmt.Errorf("XML-RPC call failed: %v", err)
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", result)
	}

	// 检查是否有fault
	if strings.Contains(xmlResponse, "<fault>") {
		return nil, fmt.Errorf("XML-RPC fault in response")
	}

	// 使用正确的 XML 解析
	processes, err := parseProcessInfoXML(xmlResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %v", err)
	}
	
	return processes, nil
}

// parseProcessInfoXML 使用正确的方法解析 XML 响应
func parseProcessInfoXML(xmlResponse string) ([]ProcessInfo, error) {
	var processes []ProcessInfo
	
	// 简单但有效的方法：逐个查找每个字段
	// 查找所有 <value><struct> 块
	structs := extractStructBlocks(xmlResponse)
	
	for _, structXML := range structs {
		process := ProcessInfo{}
		
		// 提取字符串字段
		process.Name = extractFieldValue(structXML, "name")
		process.Group = extractFieldValue(structXML, "group")
		process.StateName = extractFieldValue(structXML, "statename")
		process.SpawnErr = extractFieldValue(structXML, "spawnerr")
		process.StdoutLogfile = extractFieldValue(structXML, "stdout_logfile")
		process.StderrLogfile = extractFieldValue(structXML, "stderr_logfile")
		
		// 提取整数字段
		if stateStr := extractFieldValue(structXML, "state"); stateStr != "" {
			if state, err := strconv.Atoi(stateStr); err == nil {
				process.State = state
			}
		}
		
		if pidStr := extractFieldValue(structXML, "pid"); pidStr != "" {
			if pid, err := strconv.Atoi(pidStr); err == nil {
				process.PID = pid
			}
		}
		
		if exitStr := extractFieldValue(structXML, "exitstatus"); exitStr != "" {
			if exit, err := strconv.Atoi(exitStr); err == nil {
				process.ExitStatus = exit
			}
		}
		
		// 提取时间戳字段
		if startStr := extractFieldValue(structXML, "start"); startStr != "" {
			if start, err := strconv.ParseInt(startStr, 10, 64); err == nil {
				process.Start = start
			}
		}
		
		if stopStr := extractFieldValue(structXML, "stop"); stopStr != "" {
			if stop, err := strconv.ParseInt(stopStr, 10, 64); err == nil {
				process.Stop = stop
			}
		}
		
		if nowStr := extractFieldValue(structXML, "now"); nowStr != "" {
			if now, err := strconv.ParseInt(nowStr, 10, 64); err == nil {
				process.Now = now
			}
		}
		
		// 计算运行时间 - 直接使用API提供的时间戳
		if process.State == 20 && process.Start > 0 && process.Now > 0 { // RUNNING state
			process.Uptime = process.Now - process.Start
			process.UptimeHuman = formatUptime(process.Uptime)
		}
		
		if process.Name != "" {
			processes = append(processes, process)
		}
	}
	
	return processes, nil
}

// extractStructBlocks 提取所有 struct 块
func extractStructBlocks(xmlResponse string) []string {
	var blocks []string
	start := 0
	
	for {
		structStart := strings.Index(xmlResponse[start:], "<value><struct>")
		if structStart == -1 {
			break
		}
		structStart += start + 15 // len("<value><struct>")
		
		structEnd := strings.Index(xmlResponse[structStart:], "</struct></value>")
		if structEnd == -1 {
			break
		}
		
		block := xmlResponse[structStart : structStart+structEnd]
		blocks = append(blocks, block)
		start = structStart + structEnd + 16 // len("</struct></value>")
	}
	
	return blocks
}

// extractFieldValue 提取指定字段的值 - 更精确的匹配
func extractFieldValue(structXML, fieldName string) string {
	// 构建精确的模式：<member><name>fieldName</name><value>...
	memberStart := 0
	for {
		// 查找下一个 member 块
		memberPos := strings.Index(structXML[memberStart:], "<member>")
		if memberPos == -1 {
			break
		}
		memberPos += memberStart
		
		// 查找这个 member 的结束
		memberEnd := strings.Index(structXML[memberPos:], "</member>")
		if memberEnd == -1 {
			break
		}
		memberEnd += memberPos
		
		memberContent := structXML[memberPos:memberEnd]
		
		// 检查这个 member 是否包含我们要找的字段名
		namePattern := fmt.Sprintf("<name>%s</name>", fieldName)
		if strings.Contains(memberContent, namePattern) {
			// 提取值
			valueStart := strings.Index(memberContent, "<value>")
			if valueStart == -1 {
				break
			}
			valueStart += 7 // len("<value>")
			
			valueEnd := strings.Index(memberContent[valueStart:], "</value>")
			if valueEnd == -1 {
				break
			}
			
			valueContent := memberContent[valueStart : valueStart+valueEnd]
			
			// 提取具体类型的值
			for _, valueType := range []string{"string", "int", "boolean"} {
				startTag := fmt.Sprintf("<%s>", valueType)
				endTag := fmt.Sprintf("</%s>", valueType)
				
				if start := strings.Index(valueContent, startTag); start != -1 {
					start += len(startTag)
					if end := strings.Index(valueContent[start:], endTag); end != -1 {
						return valueContent[start : start+end]
					}
				}
			}
		}
		
		memberStart = memberEnd
	}
	
	return ""
}



// parseBooleanResponse 解析 XML-RPC 布尔响应
// 从 XML 中提取 <boolean> 标签的值
// 返回: (success bool, err error)
func parseBooleanResponse(xmlResponse string) (bool, error) {
	// 检查是否是 fault 响应
	if strings.Contains(xmlResponse, "<fault>") {
		_, faultString, _ := parseFaultResponse(xmlResponse)
		return false, fmt.Errorf("XML-RPC fault: %s", faultString)
	}

	// 查找 <boolean> 标签
	// 格式: <value><boolean>1</boolean></value> 或 <value><boolean>0</boolean></value>
	if strings.Contains(xmlResponse, "<boolean>1</boolean>") {
		return true, nil
	}
	if strings.Contains(xmlResponse, "<boolean>0</boolean>") {
		return false, nil
	}

	// 尝试更精确的解析
	boolStart := strings.Index(xmlResponse, "<boolean>")
	if boolStart == -1 {
		return false, fmt.Errorf("no boolean value found in response")
	}
	boolStart += 9 // len("<boolean>")

	boolEnd := strings.Index(xmlResponse[boolStart:], "</boolean>")
	if boolEnd == -1 {
		return false, fmt.Errorf("malformed boolean response")
	}

	value := strings.TrimSpace(xmlResponse[boolStart : boolStart+boolEnd])
	switch value {
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value: %s", value)
	}
}

// parseFaultResponse 解析 XML-RPC Fault 响应
// 从 XML 中提取 faultCode 和 faultString
// 返回: (faultCode int, faultString string, isFault bool)
func parseFaultResponse(xmlResponse string) (int, string, bool) {
	if !strings.Contains(xmlResponse, "<fault>") {
		return 0, "", false
	}

	faultCode := 0
	faultString := ""

	// 提取 faultCode
	if codeStr := extractValue(xmlResponse, "faultCode"); codeStr != "" {
		if code, err := strconv.Atoi(codeStr); err == nil {
			faultCode = code
		}
	}

	// 提取 faultString
	faultString = extractValue(xmlResponse, "faultString")

	return faultCode, faultString, true
}

// formatUptime 格式化运行时间为人类可读格式
func formatUptime(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm %ds", minutes, seconds%60)
	}
	
	hours := minutes / 60
	if hours < 24 {
		return fmt.Sprintf("%dh %dm", hours, minutes%60)
	}
	
	days := hours / 24
	return fmt.Sprintf("%dd %dh", days, hours%24)
}

// StartProcess 启动进程
// 官方签名:supervisor.startProcess(name, wait=True) -> boolean
// 显式传 wait=true(同步):supervisord 阻塞到进程完全启动(进入 RUNNING/SRC 等)
// 才返回,因此返回结果是真实准确的。HTTP 客户端超时已在 client.go 提升到
// 35s(大于 SingleOperationTimeout 30s),由上层 timeoutManager 优先负责超时,
// 避免慢启动进程在 HTTP 层被 5s 软超时误截断。
func (s *SupervisorClient) StartProcess(name string) error {
	result, err := s.client.Call("supervisor.startProcess", []interface{}{name, true})
	if err != nil {
		return err
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return fmt.Errorf("unexpected response type: %T", result)
	}

	// 检查是否是 fault 响应
	if faultCode, faultString, isFault := parseFaultResponse(xmlResponse); isFault {
		// ALREADY_STARTED 视为成功（幂等操作）
		if strings.Contains(faultString, "ALREADY_STARTED") {
			return nil
		}
		return fmt.Errorf("supervisor fault [%d]: %s", faultCode, faultString)
	}

	// 解析布尔响应
	success, err := parseBooleanResponse(xmlResponse)
	if err != nil {
		return fmt.Errorf("failed to parse response for process %s: %v", name, err)
	}

	if !success {
		return fmt.Errorf("supervisor rejected start request for process %s", name)
	}

	return nil
}

// StopProcess 停止进程
func (s *SupervisorClient) StopProcess(name string) error {
	// 先检查进程状态，如果已经停止则直接返回成功
	info, err := s.GetProcessInfo(name)
	if err != nil {
		return err
	}
	
	// 如果进程已经停止，直接返回成功
	// Supervisor 状态码: 0=STOPPED, 10=STARTING, 20=RUNNING, 30=BACKOFF, 40=STOPPING, 100=EXITED, 200=FATAL, 1000=UNKNOWN
	if info.State == 0 || info.State == 100 || info.State == 200 {
		return nil // 进程已经停止或退出，无需再停止
	}
	
	result, err := s.client.Call("supervisor.stopProcess", []interface{}{name, true})
	if err != nil {
		// 如果错误信息表明进程已经停止，也返回成功
		if strings.Contains(err.Error(), "NOT_RUNNING") || strings.Contains(err.Error(), "not running") {
			return nil
		}
		return err
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return fmt.Errorf("unexpected response type: %T", result)
	}

	// 检查是否是 fault 响应
	if faultCode, faultString, isFault := parseFaultResponse(xmlResponse); isFault {
		// NOT_RUNNING 视为成功（幂等操作）
		if strings.Contains(faultString, "NOT_RUNNING") {
			return nil
		}
		return fmt.Errorf("supervisor fault [%d]: %s", faultCode, faultString)
	}

	// 解析布尔响应
	success, err := parseBooleanResponse(xmlResponse)
	if err != nil {
		return fmt.Errorf("failed to parse response for process %s: %v", name, err)
	}

	if !success {
		return fmt.Errorf("supervisor rejected stop request for process %s", name)
	}

	return nil
}

// GetProcessInfo 获取单个进程信息
func (s *SupervisorClient) GetProcessInfo(name string) (*ProcessInfo, error) {
	result, err := s.client.Call("supervisor.getProcessInfo", []interface{}{name})
	if err != nil {
		return nil, err
	}

	// 处理 XML 响应
	xmlResponse, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", result)
	}

	// 检查是否有 fault
	if strings.Contains(xmlResponse, "<fault>") {
		return nil, fmt.Errorf("XML-RPC fault in response")
	}

	// 解析单个进程信息
	pi := &ProcessInfo{}
	
	// 解析 name
	if nameMatch := extractValue(xmlResponse, "name"); nameMatch != "" {
		pi.Name = nameMatch
	}
	
	// 解析 group
	if groupMatch := extractValue(xmlResponse, "group"); groupMatch != "" {
		pi.Group = groupMatch
	}
	
	// 解析 state
	if stateMatch := extractValue(xmlResponse, "state"); stateMatch != "" {
		if state, err := strconv.Atoi(stateMatch); err == nil {
			pi.State = state
		}
	}
	
	// 解析 statename
	if statenameMatch := extractValue(xmlResponse, "statename"); statenameMatch != "" {
		pi.StateName = statenameMatch
	}
	
	// 解析 pid
	if pidMatch := extractValue(xmlResponse, "pid"); pidMatch != "" {
		if pid, err := strconv.Atoi(pidMatch); err == nil {
			pi.PID = pid
		}
	}

	return pi, nil
}

// extractValue 从 XML 响应中提取指定字段的值
func extractValue(xml, fieldName string) string {
	// 查找 <name>fieldName</name> 后面的 <value> 标签
	nameTag := "<name>" + fieldName + "</name>"
	idx := strings.Index(xml, nameTag)
	if idx == -1 {
		return ""
	}
	
	// 从 nameTag 位置开始查找 <value> 标签
	remaining := xml[idx+len(nameTag):]
	
	// 尝试解析 <int> 类型
	if intStart := strings.Index(remaining, "<int>"); intStart != -1 && intStart < 100 {
		intEnd := strings.Index(remaining[intStart:], "</int>")
		if intEnd != -1 {
			return remaining[intStart+5 : intStart+intEnd]
		}
	}
	
	// 尝试解析 <i4> 类型
	if i4Start := strings.Index(remaining, "<i4>"); i4Start != -1 && i4Start < 100 {
		i4End := strings.Index(remaining[i4Start:], "</i4>")
		if i4End != -1 {
			return remaining[i4Start+4 : i4Start+i4End]
		}
	}
	
	// 尝试解析 <string> 类型
	if strStart := strings.Index(remaining, "<string>"); strStart != -1 && strStart < 100 {
		strEnd := strings.Index(remaining[strStart:], "</string>")
		if strEnd != -1 {
			return remaining[strStart+8 : strStart+strEnd]
		}
	}
	
	// 尝试解析空 <value></value> 或 <value/>
	if valueStart := strings.Index(remaining, "<value>"); valueStart != -1 && valueStart < 50 {
		valueEnd := strings.Index(remaining[valueStart:], "</value>")
		if valueEnd != -1 {
			content := remaining[valueStart+7 : valueStart+valueEnd]
			// 去除可能的标签
			content = strings.TrimSpace(content)
			if !strings.HasPrefix(content, "<") {
				return content
			}
		}
	}
	
	return ""
}

// TailProcessStdoutLog 获取进程 stdout 日志尾部 - 符合官方 API 规范
// 返回: [string bytes, int offset, bool overflow]
func (s *SupervisorClient) TailProcessStdoutLog(name string, offset, length int) (string, int, bool, error) {
	result, err := s.client.Call("supervisor.tailProcessStdoutLog", []interface{}{name, offset, length})
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to tail stdout log: %w", err)
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return "", 0, false, fmt.Errorf("unexpected response type: %T", result)
	}

	// 检查是否有fault
	if strings.Contains(xmlResponse, "<fault>") {
		return "", 0, false, fmt.Errorf("XML-RPC fault in response")
	}

	// 解析 tailProcessStdoutLog 返回的数组: [bytes, offset, overflow]
	logData, nextOffset, overflow := parseTailLogResponse(xmlResponse)
	formattedLog := formatLogContent(logData)
	return formattedLog, nextOffset, overflow, nil
}

// TailProcessStderrLog 获取进程 stderr 日志尾部 - 符合官方 API 规范  
// 返回: [string bytes, int offset, bool overflow]
func (s *SupervisorClient) TailProcessStderrLog(name string, offset, length int) (string, int, bool, error) {
	result, err := s.client.Call("supervisor.tailProcessStderrLog", []interface{}{name, offset, length})
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to tail stderr log: %w", err)
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return "", 0, false, fmt.Errorf("unexpected response type: %T", result)
	}

	// 检查是否有fault
	if strings.Contains(xmlResponse, "<fault>") {
		return "", 0, false, fmt.Errorf("XML-RPC fault in response")
	}

	// 解析 tailProcessStderrLog 返回的数组: [bytes, offset, overflow]
	logData, nextOffset, overflow := parseTailLogResponse(xmlResponse)
	formattedLog := formatLogContent(logData)
	return formattedLog, nextOffset, overflow, nil
}

// ReadProcessStdoutLog 按偏移量读取进程 stdout 日志 - 符合官方 API 规范
// 官方签名: readProcessStdoutLog(name, offset, length) -> [string bytes, int offset, bool overflow]
// 与 tail 不同,read 从指定 offset 读取,用于历史日志分页加载。
func (s *SupervisorClient) ReadProcessStdoutLog(name string, offset, length int) (string, int, bool, error) {
	result, err := s.client.Call("supervisor.readProcessStdoutLog", []interface{}{name, offset, length})
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to read stdout log: %w", err)
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return "", 0, false, fmt.Errorf("unexpected response type: %T", result)
	}

	if strings.Contains(xmlResponse, "<fault>") {
		return "", 0, false, fmt.Errorf("XML-RPC fault in response")
	}

	logData, nextOffset, overflow := parseTailLogResponse(xmlResponse)
	formattedLog := formatLogContent(logData)
	return formattedLog, nextOffset, overflow, nil
}

// ReadProcessStderrLog 按偏移量读取进程 stderr 日志 - 符合官方 API 规范
// 官方签名: readProcessStderrLog(name, offset, length) -> [string bytes, int offset, bool overflow]
func (s *SupervisorClient) ReadProcessStderrLog(name string, offset, length int) (string, int, bool, error) {
	result, err := s.client.Call("supervisor.readProcessStderrLog", []interface{}{name, offset, length})
	if err != nil {
		return "", 0, false, fmt.Errorf("failed to read stderr log: %w", err)
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return "", 0, false, fmt.Errorf("unexpected response type: %T", result)
	}

	if strings.Contains(xmlResponse, "<fault>") {
		return "", 0, false, fmt.Errorf("XML-RPC fault in response")
	}

	logData, nextOffset, overflow := parseTailLogResponse(xmlResponse)
	formattedLog := formatLogContent(logData)
	return formattedLog, nextOffset, overflow, nil
}

// ClearProcessLogs 清空并重开指定进程的 stdout/stderr 日志 - 符合官方 API 规范
// 官方签名: clearProcessLogs(name) -> boolean
func (s *SupervisorClient) ClearProcessLogs(name string) error {
	result, err := s.client.Call("supervisor.clearProcessLogs", []interface{}{name})
	if err != nil {
		return fmt.Errorf("failed to clear process logs: %w", err)
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return fmt.Errorf("unexpected response type: %T", result)
	}

	if faultCode, faultString, isFault := parseFaultResponse(xmlResponse); isFault {
		return fmt.Errorf("supervisor fault [%d]: %s", faultCode, faultString)
	}

	success, err := parseBooleanResponse(xmlResponse)
	if err != nil {
		return fmt.Errorf("failed to parse response for clearProcessLogs %s: %v", name, err)
	}
	if !success {
		return fmt.Errorf("supervisor rejected clearProcessLogs request for process %s", name)
	}
	return nil
}

// ProcessConfigInfo 符合官方 getAllConfigInfo 返回的进程配置信息结构
type ProcessConfigInfo struct {
	Name             string `json:"name"`
	Group            string `json:"group"`
	Command          string `json:"command"`
	StdoutLogfile    string `json:"stdout_logfile"`
	StderrLogfile    string `json:"stderr_logfile"`
	Directory        string `json:"directory"`
	Priority         int    `json:"priority"`
	Autostart        bool   `json:"autostart"`
	Autorestart      bool   `json:"autorestart"`
	Startsecs        int    `json:"startsecs"`
	Startretries     int    `json:"startretries"`
	Stopsignal       string `json:"stopsignal"`
	Stopwaitsecs     int    `json:"stopwaitsecs"`
	Killasgroup      bool   `json:"killasgroup"`
	Exitcodes        string `json:"exitcodes"`
	Environment      string `json:"environment"`
	ProcessName      string `json:"process_name"`
}

// GetAllConfigInfo 获取所有进程配置信息 - 符合官方 API 规范
// 官方签名: getAllConfigInfo() -> array of process config info structs
func (s *SupervisorClient) GetAllConfigInfo() ([]ProcessConfigInfo, error) {
	result, err := s.client.Call("supervisor.getAllConfigInfo", nil)
	if err != nil {
		return nil, fmt.Errorf("XML-RPC call failed: %v", err)
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", result)
	}

	if strings.Contains(xmlResponse, "<fault>") {
		return nil, fmt.Errorf("XML-RPC fault in response")
	}

	// 复用 extractStructBlocks 解析 struct 数组
	structs := extractStructBlocks(xmlResponse)
	var configs []ProcessConfigInfo
	for _, structXML := range structs {
		cfg := ProcessConfigInfo{
			Name:          extractFieldValue(structXML, "name"),
			Group:         extractFieldValue(structXML, "group"),
			Command:       extractFieldValue(structXML, "command"),
			StdoutLogfile: extractFieldValue(structXML, "stdout_logfile"),
			StderrLogfile: extractFieldValue(structXML, "stderr_logfile"),
			Directory:     extractFieldValue(structXML, "directory"),
			Stopsignal:    extractFieldValue(structXML, "stopsignal"),
			Environment:   extractFieldValue(structXML, "environment"),
		}
		cfg.ProcessName = cfg.Name

		if v := extractFieldValue(structXML, "priority"); v != "" {
			cfg.Priority, _ = strconv.Atoi(v)
		}
		if v := extractFieldValue(structXML, "startsecs"); v != "" {
			cfg.Startsecs, _ = strconv.Atoi(v)
		}
		if v := extractFieldValue(structXML, "startretries"); v != "" {
			cfg.Startretries, _ = strconv.Atoi(v)
		}
		if v := extractFieldValue(structXML, "stopwaitsecs"); v != "" {
			cfg.Stopwaitsecs, _ = strconv.Atoi(v)
		}
		// 布尔字段:autostart/autorestart/killasgroup
		cfg.Autostart = extractFieldValue(structXML, "autostart") == "true"
		cfg.Autorestart = extractFieldValue(structXML, "autorestart") == "true" ||
			extractFieldValue(structXML, "autorestart") == "unexpected"
		cfg.Killasgroup = extractFieldValue(structXML, "killasgroup") == "true"

		if cfg.Name != "" {
			configs = append(configs, cfg)
		}
	}
	return configs, nil
}

// ReloadResult 符合官方 reloadConfig 返回的结构
type ReloadResult struct {
	Added   []string `json:"added"`
	Changed []string `json:"changed"`
	Removed []string `json:"removed"`
}

// ReloadConfig 重载配置 - 符合官方 API 规范
// 官方签名: reloadConfig() -> struct { added: array, changed: array, removed: array }
func (s *SupervisorClient) ReloadConfig() (*ReloadResult, error) {
	result, err := s.client.Call("supervisor.reloadConfig", nil)
	if err != nil {
		return nil, fmt.Errorf("XML-RPC call failed: %v", err)
	}

	xmlResponse, ok := result.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", result)
	}

	if strings.Contains(xmlResponse, "<fault>") {
		return nil, fmt.Errorf("XML-RPC fault in response")
	}

	// reloadConfig 返回单个 struct,内含 added/changed/removed 三个字符串数组
	// 格式: <struct><member><name>added</name>...<member><name>changed</name>...<member><name>removed</name>...
	res := &ReloadResult{}
	res.Added = extractStringArrayField(xmlResponse, "added")
	res.Changed = extractStringArrayField(xmlResponse, "changed")
	res.Removed = extractStringArrayField(xmlResponse, "removed")
	return res, nil
}

// extractStringArrayField 从 XML-RPC struct 响应中提取字符串数组字段
// 兼容 reloadConfig 的 added/changed/removed 数组结构。
func extractStringArrayField(xmlResponse, fieldName string) []string {
	var out []string
	nameTag := "<name>" + fieldName + "</name>"
	idx := strings.Index(xmlResponse, nameTag)
	if idx == -1 {
		return out
	}
	// 从 nameTag 后找 <array><data>...
	remaining := xmlResponse[idx+len(nameTag):]
	dataStart := strings.Index(remaining, "<array><data>")
	if dataStart == -1 {
		// 兼容 <array>\n<data> 形式
		dataStart = strings.Index(remaining, "<array>")
		if dataStart == -1 {
			return out
		}
		innerData := strings.Index(remaining[dataStart:], "<data>")
		if innerData == -1 {
			return out
		}
		dataStart += innerData
	}
	dataStart += len("<array><data>")

	// 收集所有 <value><string>xxx</string></value>
	cur := remaining[dataStart:]
	for {
		strStart := strings.Index(cur, "<value><string>")
		if strStart == -1 {
			break
		}
		strStart += len("<value><string>")
		strEnd := strings.Index(cur[strStart:], "</string></value>")
		if strEnd == -1 {
			break
		}
		out = append(out, cur[strStart:strStart+strEnd])
		cur = cur[strStart+strEnd+len("</string></value>"):]
	}
	return out
}

// parseTailLogResponse 解析 tailProcessLog 的响应
// Supervisor API 返回格式: [string bytes, int offset, bool overflow]
// XML-RPC 数组格式: <array><data><value>...</value><value>...</value><value>...</value></data></array>
func parseTailLogResponse(xmlResponse string) (string, int, bool) {
	logContent := ""
	nextOffset := 0
	overflow := false
	
	// 查找 <array><data> 结构
	dataStart := strings.Index(xmlResponse, "<data>")
	dataEnd := strings.Index(xmlResponse, "</data>")
	if dataStart == -1 || dataEnd == -1 {
		return logContent, nextOffset, overflow
	}
	
	dataContent := xmlResponse[dataStart:dataEnd]
	
	// 提取所有 <value>...</value> 元素
	values := extractValues(dataContent)
	
	// 第一个值: 日志内容 (string)
	if len(values) > 0 {
		logContent = extractStringValue(values[0])
	}
	
	// 第二个值: 偏移量 (int)
	if len(values) > 1 {
		nextOffset = extractIntValue(values[1])
	}
	
	// 第三个值: 溢出标志 (boolean)
	if len(values) > 2 {
		overflow = extractBoolValue(values[2])
	}
	
	return logContent, nextOffset, overflow
}

// extractValues 从 <data> 内容中提取所有 <value> 元素
func extractValues(dataContent string) []string {
	var values []string
	remaining := dataContent
	
	for {
		start := strings.Index(remaining, "<value>")
		if start == -1 {
			break
		}
		
		// 找到对应的 </value>，需要处理嵌套
		end := findMatchingValueEnd(remaining[start:])
		if end == -1 {
			break
		}
		
		valueContent := remaining[start : start+end+8] // 8 = len("</value>")
		values = append(values, valueContent)
		remaining = remaining[start+end+8:]
	}
	
	return values
}

// findMatchingValueEnd 找到匹配的 </value> 位置
func findMatchingValueEnd(s string) int {
	depth := 0
	i := 0
	for i < len(s) {
		if strings.HasPrefix(s[i:], "<value>") {
			depth++
			i += 7
		} else if strings.HasPrefix(s[i:], "</value>") {
			depth--
			if depth == 0 {
				return i
			}
			i += 8
		} else {
			i++
		}
	}
	return -1
}

// extractStringValue 从 <value> 元素中提取字符串值
func extractStringValue(valueXML string) string {
	// 尝试 <string>...</string>
	if start := strings.Index(valueXML, "<string>"); start != -1 {
		start += 8
		if end := strings.Index(valueXML[start:], "</string>"); end != -1 {
			return valueXML[start : start+end]
		}
	}
	// 有些实现直接在 <value> 中放字符串
	if start := strings.Index(valueXML, "<value>"); start != -1 {
		start += 7
		if end := strings.Index(valueXML[start:], "</value>"); end != -1 {
			content := valueXML[start : start+end]
			// 如果没有其他标签，就是纯字符串
			if !strings.Contains(content, "<") {
				return content
			}
		}
	}
	return ""
}

// extractIntValue 从 <value> 元素中提取整数值
func extractIntValue(valueXML string) int {
	// 尝试 <int>...</int>
	if start := strings.Index(valueXML, "<int>"); start != -1 {
		start += 5
		if end := strings.Index(valueXML[start:], "</int>"); end != -1 {
			if val, err := strconv.Atoi(valueXML[start : start+end]); err == nil {
				return val
			}
		}
	}
	// 尝试 <i4>...</i4>
	if start := strings.Index(valueXML, "<i4>"); start != -1 {
		start += 4
		if end := strings.Index(valueXML[start:], "</i4>"); end != -1 {
			if val, err := strconv.Atoi(valueXML[start : start+end]); err == nil {
				return val
			}
		}
	}
	return 0
}

// extractBoolValue 从 <value> 元素中提取布尔值
func extractBoolValue(valueXML string) bool {
	if strings.Contains(valueXML, "<boolean>1</boolean>") {
		return true
	}
	if strings.Contains(valueXML, "<boolean>true</boolean>") {
		return true
	}
	return false
}

// formatLogContent 格式化日志内容
func formatLogContent(content string) string {
	// 移除XML转义字符
	content = strings.ReplaceAll(content, "&lt;", "<")
	content = strings.ReplaceAll(content, "&gt;", ">")
	content = strings.ReplaceAll(content, "&amp;", "&")
	content = strings.ReplaceAll(content, "&quot;", "\"")
	content = strings.ReplaceAll(content, "&#39;", "'")
	
	// 日志内容应该保持原样，不要按逗号分割
	// 只需要清理首尾空白
	return strings.TrimSpace(content)
}
