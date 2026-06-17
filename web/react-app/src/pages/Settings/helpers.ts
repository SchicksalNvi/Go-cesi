import dayjs from 'dayjs';
import type { ExportType } from '@/api/dataManagement';

export const BACKUP_POLL_INTERVAL = 5000;

export function buildExportLabel(exportType: ExportType) {
  const labels: Record<ExportType, string> = {
    users: 'users',
    logs: 'logs',
    configs: 'configs',
    processes: 'processes',
    all: 'all',
  };

  return labels[exportType];
}

export function buildBackupName(backupType: 'full' | 'config_only') {
  const prefix = backupType === 'config_only' ? 'config-backup' : 'full-backup';
  return `${prefix}-${dayjs().format('YYYYMMDD-HHmmss')}`;
}

export function formatFileSize(fileSize: number) {
  if (!fileSize) {
    return '-';
  }

  const units = ['B', 'KB', 'MB', 'GB'];
  let size = fileSize;
  let unitIndex = 0;

  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex += 1;
  }

  return `${size.toFixed(unitIndex === 0 ? 0 : 1)} ${units[unitIndex]}`;
}
