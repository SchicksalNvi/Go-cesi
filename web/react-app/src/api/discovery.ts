import apiClient from './client';

// Discovery task status constants
export const DiscoveryStatus = {
  PENDING: 'pending',
  RUNNING: 'running',
  COMPLETED: 'completed',
  CANCELLED: 'cancelled',
  FAILED: 'failed',
} as const;

export type DiscoveryStatusType = typeof DiscoveryStatus[keyof typeof DiscoveryStatus];

// Discovery result status constants
export const ResultStatus = {
  SUCCESS: 'success',
  TIMEOUT: 'timeout',
  CONNECTION_REFUSED: 'connection_refused',
  AUTH_FAILED: 'auth_failed',
  ERROR: 'error',
} as const;

export type ResultStatusType = typeof ResultStatus[keyof typeof ResultStatus];

// DiscoveryTask represents a network discovery scan operation
export interface DiscoveryTask {
  id: number;
  created_at: string;
  updated_at: string;
  cidr: string;
  port: number;
  username: string;
  status: DiscoveryStatusType;
  total_ips: number;
  scanned_ips: number;
  found_nodes: number;
  failed_ips: number;
  started_at?: string;
  completed_at?: string;
  error_msg?: string;
  created_by: string;
}

// DiscoveryResult represents the outcome of probing a single IP address
export interface DiscoveryResult {
  id: number;
  created_at: string;
  task_id: number;
  ip: string;
  port: number;
  status: ResultStatusType;
  node_id?: number;
  node_name?: string;
  version?: string;
  error_msg?: string;
  duration_ms: number;
}

// Request types
export interface StartDiscoveryRequest {
  cidr: string;
  port: number;
  username: string;
  password: string;
  timeout_seconds?: number;  // optional, default 3
  max_workers?: number;      // optional, default 50
}

export interface ValidateCIDRRequest {
  cidr: string;
}

export interface ListTasksParams {
  page?: number;
  limit?: number;
  status?: DiscoveryStatusType;
}

// Response types
interface StartDiscoveryResponse {
  status: string;
  task: DiscoveryTask;
}

interface ListTasksResponse {
  status: string;
  tasks: DiscoveryTask[];
  total: number;
  page: number;
  limit: number;
}

interface GetTaskResponse {
  status: string;
  task: DiscoveryTask;
  results: DiscoveryResult[];
}

interface CancelTaskResponse {
  status: string;
  message: string;
}

interface DeleteTaskResponse {
  status: string;
  message: string;
}

interface GetTaskProgressResponse {
  status: string;
  progress: {
    task_id: number;
    status: DiscoveryStatusType;
    total_ips: number;
    scanned_ips: number;
    found_nodes: number;
    failed_ips: number;
    percent: number;
  };
}

interface ValidateCIDRResponse {
  status: string;
  valid: boolean;
  cidr: string;
  count: number;
}

// Discovery API client
// Requirements: 2.1, 2.4, 6.4
export const discoveryApi = {
  // Start a new discovery task
  // POST /api/discovery/tasks
  startDiscovery: (req: StartDiscoveryRequest) =>
    apiClient.post<StartDiscoveryResponse>('/discovery/tasks', req),

  // List discovery tasks with pagination and optional status filter
  // GET /api/discovery/tasks
  getTasks: (params?: ListTasksParams, config?: { signal?: AbortSignal }) =>
    apiClient.get<ListTasksResponse>('/discovery/tasks', { params, ...config }),

  // Get task details with results
  // GET /api/discovery/tasks/:id
  getTask: (taskId: number, config?: { signal?: AbortSignal }) =>
    apiClient.get<GetTaskResponse>(`/discovery/tasks/${taskId}`, config),

  // Cancel a running discovery task
  // POST /api/discovery/tasks/:id/cancel
  cancelTask: (taskId: number) =>
    apiClient.post<CancelTaskResponse>(`/discovery/tasks/${taskId}/cancel`),

  // Delete a discovery task and its results
  // DELETE /api/discovery/tasks/:id
  deleteTask: (taskId: number) =>
    apiClient.delete<DeleteTaskResponse>(`/discovery/tasks/${taskId}`),

  // Get current progress of a discovery task (polling endpoint)
  // GET /api/discovery/tasks/:id/progress
  getTaskProgress: (taskId: number) =>
    apiClient.get<GetTaskProgressResponse>(`/discovery/tasks/${taskId}/progress`),

  // Validate a CIDR string and get IP count
  // POST /api/discovery/validate-cidr
  validateCIDR: (cidr: string) =>
    apiClient.post<ValidateCIDRResponse>('/discovery/validate-cidr', { cidr }),
};
