import type { User } from '@/types';

export const PERMISSIONS = {
  systemManage: 'system:manage',
  systemConfig: 'system:config',
  userRead: 'user:read',
  userWrite: 'user:write',
  userDelete: 'user:delete',
  logRead: 'log:read',
  logDelete: 'log:delete',
} as const;

export function hasPermission(user: User | null | undefined, permission: string): boolean {
  if (!user) {
    return false;
  }

  if (user.is_admin) {
    return true;
  }

  return user.permissions?.includes(permission) ?? false;
}

export function hasAnyPermission(user: User | null | undefined, permissions: string[]): boolean {
  return permissions.some((permission) => hasPermission(user, permission));
}
