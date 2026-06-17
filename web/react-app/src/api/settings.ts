import apiClient from './client';
import { ApiResponse } from '@/types';

export interface UserPreferences {
  id?: string;
  user_id?: string;
  theme?: string;
  language?: string;
  timezone?: string;
  date_format?: string;
  time_format?: string;
  page_size?: number;
  auto_refresh?: boolean;
  refresh_interval?: number;
  email_notifications?: boolean;
  process_alerts?: boolean;
  system_alerts?: boolean;
  node_status_changes?: boolean;
  weekly_report?: boolean;
  notifications?: string;
  dashboard_layout?: string;
  created_at?: string;
  updated_at?: string;
}

export interface SystemSettings {
  refresh_interval: number;
  log_retention_days: number;
  max_concurrent_connections: number;
  enable_websocket: boolean;
  enable_activity_logging: boolean;
}

export interface SystemSettingsResponse {
  settings: Record<string, string>;
  count: number;
}

export const settingsApi = {
  // 用户偏好设置
  getUserPreferences: () =>
    apiClient.get<UserPreferences>('/system-settings/user-preferences'),

  updateUserPreferences: (data: UserPreferences) =>
    apiClient.put<UserPreferences>('/system-settings/user-preferences', data),

  // 系统设置（仅管理员）
  getSystemSettings: () =>
    apiClient.get<SystemSettingsResponse>('/system-settings'),

  updateSystemSettings: (data: Partial<SystemSettings>) =>
    apiClient.put<{ message: string }>('/system-settings/batch', { settings: data }),
};
