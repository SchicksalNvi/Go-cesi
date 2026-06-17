import apiClient from './client';
import { ApiResponse, User } from '@/types';

export interface LoginRequest {
  username: string;
  password: string;
}

export interface LoginResponse {
  token: string;
  user: User;
}

export interface ChangeOwnPasswordRequest {
  old_password: string;
  new_password: string;
}

export const authApi = {
  // Login
  login: (data: LoginRequest) =>
    apiClient.post<ApiResponse<LoginResponse>>('/auth/login', data),

  // Logout
  logout: () => apiClient.post('/auth/logout'),

  // Get current user
  getCurrentUser: () => apiClient.get<ApiResponse<{ user: User }>>('/auth/user'),

  // Get current profile
  getProfile: () => apiClient.get<ApiResponse<{ user: User }>>('/profile'),

  // Update profile
  updateProfile: (data: Partial<User>) => apiClient.put<ApiResponse<{ user: User }>>('/profile', data),

  // Change current user password
  changeOwnPassword: (data: ChangeOwnPasswordRequest) =>
    apiClient.put<ApiResponse<{ message: string }>>('/profile/password', data),
};
