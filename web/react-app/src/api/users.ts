import client from './client';

export interface User {
  id: string;
  username: string;
  email: string;
  full_name?: string;
  is_active: boolean;
  is_admin: boolean;
  last_login?: string;
  created_at: string;
  updated_at: string;
}

export interface AvailableNode {
  id: number;
  name: string;
  environment?: string;
  host: string;
  port: number;
}

export interface UserNodeAccess {
  id?: string;
  node_id: number;
  can_read: boolean;
  can_write: boolean;
  can_delete: boolean;
  node?: AvailableNode;
}

export interface UserNodeAccessResponse {
  status: string;
  data: {
    user_id: string;
    node_access: UserNodeAccess[];
    available_nodes: AvailableNode[];
  };
}

export interface CreateUserRequest {
  username: string;
  email: string;
  password: string;
  full_name?: string;
  is_admin?: boolean;
}

export interface UpdateUserRequest {
  email?: string;
  full_name?: string;
  is_admin?: boolean;
  is_active?: boolean;
}

export interface UsersResponse {
  status: string;
  data: {
    users: User[];
    total: number;
    page: number;
    page_size: number;
  };
}

export interface UserResponse {
  status: string;
  data: User;
  message?: string;
}

export const usersApi = {
  getUsers: async (page = 1, pageSize = 20): Promise<UsersResponse> => {
    return client.get(`/users?page=${page}&page_size=${pageSize}`);
  },

  getUser: async (id: string): Promise<UserResponse> => {
    return client.get(`/users/${id}`);
  },

  createUser: async (data: CreateUserRequest): Promise<UserResponse> => {
    return client.post('/users', data);
  },

  updateUser: async (id: string, data: UpdateUserRequest): Promise<UserResponse> => {
    return client.put(`/users/${id}`, data);
  },

  deleteUser: async (id: string): Promise<{ status: string; message: string }> => {
    return client.delete(`/users/${id}`);
  },

  toggleUserStatus: async (id: string, isActive: boolean): Promise<{ status: string; message: string }> => {
    return client.patch(`/users/${id}/toggle`, { is_active: isActive });
  },

  resetPassword: async (id: string, newPassword: string): Promise<{ status: string; message: string }> => {
    return client.put(`/users/${id}/password`, { new_password: newPassword });
  },

  getUserPreferences: async (userId: string): Promise<UserPreferencesData> => {
    return client.get(`/system-settings/users/${userId}/preferences`);
  },

  updateUserPreferences: async (userId: string, data: UserPreferencesData): Promise<UserPreferencesData> => {
    return client.put(`/system-settings/users/${userId}/preferences`, data);
  },

  getUserNodeAccess: async (userId: string): Promise<UserNodeAccessResponse> => {
    return client.get(`/users/${userId}/node-access`);
  },

  updateUserNodeAccess: async (
    userId: string,
    data: { node_access: Array<Pick<UserNodeAccess, 'node_id' | 'can_read' | 'can_write' | 'can_delete'>> }
  ): Promise<UserNodeAccessResponse> => {
    return client.put(`/users/${userId}/node-access`, data);
  },
};

export interface UserPreferencesData {
  timezone?: string;
  theme?: string;
  language?: string;
  email_notifications?: boolean;
  process_alerts?: boolean;
  system_alerts?: boolean;
  node_status_changes?: boolean;
  weekly_report?: boolean;
}
