// API Response Types
export interface ApiResponse<T = any> {
  status: string;
  data?: T;
  message?: string;
  error?: string;
}

// Node Types
export interface Node {
  name: string;
  environment: string;
  host: string;
  port: number;
  username?: string;
  is_connected: boolean;
  process_count?: number;
  running_count?: number;
  last_ping?: string;
}

// Environment Types
export interface NodeSummary {
  name: string;
  host: string;
  port: number;
  is_connected: boolean;
  last_ping: string;
}

export interface NodeDetail extends NodeSummary {
  processes: number;
}

export interface Environment {
  name: string;
  members: NodeSummary[];
}

export interface EnvironmentDetail {
  name: string;
  members: NodeDetail[];
}

// Process Types
export interface Process {
  name: string;
  group: string;
  state: number;
  state_string: string;
  start_time: string;
  stop_time: string;
  pid: number;
  exit_status: number;
  uptime: number;
  uptime_human: string;
}

// User Types
export interface User {
  id: string;
  username: string;
  email: string;
  full_name?: string;
  is_admin: boolean;
  is_active: boolean;
  role?: string;
  roles?: string[];
  permissions?: string[];
  created_at: string;
  updated_at: string;
}

// Alert Types
export interface AlertRule {
  id: number;
  name: string;
  description: string;
  metric: string;
  condition: string;
  threshold: number;
  duration: number;
  severity: 'low' | 'medium' | 'high' | 'critical';
  enabled: boolean;
  node_id?: number;
  process_name?: string;
  tags?: string;
  created_by: number;
  created_at: string;
  updated_at: string;
}

export interface Alert {
  id: number;
  rule_id: number;
  node_id?: number;
  process_name?: string;
  message: string;
  severity: 'low' | 'medium' | 'high' | 'critical';
  status: 'active' | 'acknowledged' | 'resolved';
  value: number;
  start_time: string;
  end_time?: string;
  acked_by?: number;
  acked_at?: string;
  resolved_by?: number;
  resolved_at?: string;
  created_at: string;
}

// Activity Log Types
export interface ActivityLog {
  id: number;
  created_at: string;
  updated_at: string;
  level: string;
  message: string;
  action: string;
  resource: string;
  target: string;
  user_id: string;
  username: string;
  ip_address: string;
  user_agent: string;
  details: string;
  status: string;
  duration: number;
}

export interface ActivityLogsFilters {
  level?: string;
  action?: string;
  resource?: string;
  username?: string;
  search?: string;
  start_time?: string;
  end_time?: string;
  page?: number;
  page_size?: number;
}

export interface PaginationInfo {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
  has_next: boolean;
  has_prev: boolean;
}

export interface ActivityLogsResponse {
  status: string;
  data: {
    logs: ActivityLog[];
    pagination: PaginationInfo;
  };
}

export interface LogStatistics {
  total_logs: number;
  info_count: number;
  warning_count: number;
  error_count: number;
  debug_count: number;
  top_actions: Array<{ action: string; count: number }>;
  top_users: Array<{ username: string; count: number }>;
}

// System Stats Types
export interface SystemStats {
  total_nodes: number;
  connected_nodes: number;
  running_processes: number;
  stopped_processes: number;
  active_alerts: number;
  timestamp: string;
}

// WebSocket Message Types
export interface WebSocketMessage {
  type: string;
  data?: any;
  payload?: any;
  timestamp?: number;
}

// Log Stream WebSocket Message
export interface LogStreamMessage {
  node_name: string;
  process_name: string;
  log_type: string;
  entries: LogEntry[];
  timestamp: string;
}

// Chart Data Types
export interface ChartDataPoint {
  timestamp: string;
  value: number;
  label?: string;
}

// Role and Permission Types
export interface Role {
  id: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
}

export interface Permission {
  id: string;
  name: string;
  description: string;
  resource: string;
  action: string;
}

// Configuration Types
export interface Configuration {
  id: number;
  key: string;
  value: string;
  category: string;
  description: string;
  is_sensitive: boolean;
  created_at: string;
  updated_at: string;
}

// Log Entry Types
export interface LogEntry {
  timestamp: string;
  level: string;
  message: string;
  source: string;
  process_name: string;
  node_name: string;
}

// Log Stream Types
export interface LogStream {
  process_name: string;
  node_name: string;
  log_type: string;
  entries: LogEntry[];
  last_offset: number;
  overflow: boolean;
}

// Process Group Types
export interface ProcessGroup {
  id: number;
  name: string;
  description: string;
  environment: string;
  startup_order: number;
  created_at: string;
  updated_at: string;
}

// Scheduled Task Types
export interface ScheduledTask {
  id: number;
  name: string;
  description: string;
  node_id: number;
  process_name: string;
  schedule: string;
  action: 'start' | 'stop' | 'restart';
  enabled: boolean;
  last_run?: string;
  next_run?: string;
  created_at: string;
  updated_at: string;
}

// Process Aggregation Types
export interface AggregatedProcess {
  name: string;
  total_instances: number;
  running_instances: number;
  stopped_instances: number;
  instances: ProcessInstance[];
}

export interface ProcessInstance {
  node_name: string;
  node_host: string;
  node_port: number;
  state: number;
  state_string: string;
  pid: number;
  uptime: number;
  uptime_human: string;
  group: string;
}

export interface BatchOperationResult {
  process_name: string;
  total_instances: number;
  success_count: number;
  failure_count: number;
  results: InstanceOperationResult[];
}

export interface InstanceOperationResult {
  node_name: string;
  success: boolean;
  error?: string;
}
