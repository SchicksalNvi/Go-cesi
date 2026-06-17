import { Button, Space, Tag, Popconfirm, Typography } from 'antd';
import { DownloadOutlined, DeleteOutlined } from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import type { TranslationKeys } from '@/i18n';
import type {
  BackupRecord,
  ExportFormat,
  ExportRecord,
  ExportType,
  ImportRecord,
} from '@/api/dataManagement';
import { buildExportLabel, formatFileSize } from './helpers';

const { Text } = Typography;

interface ExportColumnHandlers {
  onDownload: (record: ExportRecord) => void;
  onDelete: (id: string) => void;
}

interface ImportColumnHandlers {
  onDelete: (id: string) => void;
}

interface BackupColumnHandlers {
  onDownload: (record: BackupRecord) => void;
  onDelete: (id: string) => void;
}

export function buildExportColumns(
  t: TranslationKeys,
  handlers: ExportColumnHandlers
): ColumnsType<ExportRecord> {
  return [
    {
      title: t.settings.exportName,
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: t.settings.exportType,
      dataIndex: 'export_type',
      key: 'export_type',
      width: 140,
      render: (exportType: ExportType) =>
        t.settings[buildExportLabel(exportType) as keyof typeof t.settings] || exportType,
    },
    {
      title: t.settings.exportFormat,
      dataIndex: 'format',
      key: 'format',
      width: 120,
      render: (format: ExportFormat) => format.toUpperCase(),
    },
    {
      title: t.common.status,
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: ExportRecord['status']) => {
        const colorMap: Record<ExportRecord['status'], string> = {
          pending: 'default',
          running: 'processing',
          completed: 'success',
          failed: 'error',
        };
        const textMap: Record<ExportRecord['status'], string> = {
          pending: t.settings.exportPending,
          running: t.settings.exportRunning,
          completed: t.settings.exportCompleted,
          failed: t.settings.exportFailed,
        };

        return <Tag color={colorMap[status]}>{textMap[status]}</Tag>;
      },
    },
    {
      title: t.settings.exportRecords,
      dataIndex: 'record_count',
      key: 'record_count',
      width: 120,
    },
    {
      title: t.settings.exportSize,
      dataIndex: 'file_size',
      key: 'file_size',
      width: 120,
      render: (fileSize: number) => formatFileSize(fileSize),
    },
    {
      title: t.settings.exportCreatedAt,
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (createdAt: string) => dayjs(createdAt).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: t.settings.exportCompletedAt,
      dataIndex: 'completed_at',
      key: 'completed_at',
      width: 180,
      render: (completedAt?: string | null) =>
        completedAt ? dayjs(completedAt).format('YYYY-MM-DD HH:mm:ss') : '-',
    },
    {
      title: t.settings.exportError,
      dataIndex: 'error_msg',
      key: 'error_msg',
      ellipsis: true,
      render: (errorMsg: string) => errorMsg || '-',
    },
    {
      title: t.common.actions,
      key: 'actions',
      width: 160,
      render: (_value, record) => (
        <Space>
          <Button
            size="small"
            icon={<DownloadOutlined />}
            onClick={() => handlers.onDownload(record)}
            disabled={record.status !== 'completed'}
          >
            {t.common.export}
          </Button>
          <Popconfirm
            title={t.settings.confirmDeleteExport}
            onConfirm={() => handlers.onDelete(record.id)}
            okText={t.common.delete}
            cancelText={t.common.cancel}
          >
            <Button
              size="small"
              danger
              icon={<DeleteOutlined />}
              disabled={record.status === 'running'}
            >
              {t.common.delete}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];
}

export function buildImportColumns(
  t: TranslationKeys,
  handlers: ImportColumnHandlers
): ColumnsType<ImportRecord> {
  return [
    {
      title: t.settings.importName,
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: t.settings.importSourceFile,
      dataIndex: 'source_file',
      key: 'source_file',
      render: (sourceFile: string) => sourceFile.split('/').pop() || sourceFile,
    },
    {
      title: t.common.status,
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: ImportRecord['status']) => {
        const colorMap: Record<ImportRecord['status'], string> = {
          pending: 'default',
          running: 'processing',
          completed: 'success',
          failed: 'error',
          partial: 'warning',
        };
        const textMap: Record<ImportRecord['status'], string> = {
          pending: t.settings.importPending,
          running: t.settings.importRunning,
          completed: t.settings.importCompleted,
          failed: t.settings.importFailed,
          partial: t.settings.importPartial,
        };

        return <Tag color={colorMap[status]}>{textMap[status]}</Tag>;
      },
    },
    {
      title: t.settings.importRecords,
      dataIndex: 'total_records',
      key: 'total_records',
      width: 120,
    },
    {
      title: t.settings.importSuccess,
      dataIndex: 'success_count',
      key: 'success_count',
      width: 120,
    },
    {
      title: t.settings.importFailure,
      dataIndex: 'failure_count',
      key: 'failure_count',
      width: 120,
    },
    {
      title: t.settings.importCreatedAt,
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (createdAt: string) => dayjs(createdAt).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: t.settings.importCompletedAt,
      dataIndex: 'completed_at',
      key: 'completed_at',
      width: 180,
      render: (completedAt?: string | null) =>
        completedAt ? dayjs(completedAt).format('YYYY-MM-DD HH:mm:ss') : '-',
    },
    {
      title: t.settings.importValidation,
      dataIndex: 'validation_log',
      key: 'validation_log',
      ellipsis: true,
      render: (validationLog: string) => validationLog || '-',
    },
    {
      title: t.settings.importError,
      dataIndex: 'error_msg',
      key: 'error_msg',
      ellipsis: true,
      render: (errorMsg: string) => errorMsg || '-',
    },
    {
      title: t.common.actions,
      key: 'actions',
      width: 120,
      render: (_value, record) => (
        <Popconfirm
          title={t.settings.confirmDeleteImport}
          onConfirm={() => handlers.onDelete(record.id)}
          okText={t.common.delete}
          cancelText={t.common.cancel}
        >
          <Button
            size="small"
            danger
            icon={<DeleteOutlined />}
            disabled={record.status === 'running'}
          >
            {t.common.delete}
          </Button>
        </Popconfirm>
      ),
    },
  ];
}

export function buildBackupColumns(
  t: TranslationKeys,
  handlers: BackupColumnHandlers
): ColumnsType<BackupRecord> {
  return [
    {
      title: t.settings.backupName,
      dataIndex: 'name',
      key: 'name',
      render: (_value, record) => (
        <div>
          <div>{record.name}</div>
          {record.description ? <Text type="secondary">{record.description}</Text> : null}
        </div>
      ),
    },
    {
      title: t.settings.backupType,
      dataIndex: 'backup_type',
      key: 'backup_type',
      width: 140,
      render: (backupType: BackupRecord['backup_type']) =>
        backupType === 'config_only' ? t.settings.configOnlyBackup : t.settings.fullBackup,
    },
    {
      title: t.common.status,
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: BackupRecord['status']) => {
        const colorMap: Record<BackupRecord['status'], string> = {
          pending: 'default',
          running: 'processing',
          completed: 'success',
          failed: 'error',
        };
        const textMap: Record<BackupRecord['status'], string> = {
          pending: t.settings.backupPending,
          running: t.settings.backupRunning,
          completed: t.settings.backupCompleted,
          failed: t.settings.backupFailed,
        };

        return <Tag color={colorMap[status]}>{textMap[status]}</Tag>;
      },
    },
    {
      title: t.settings.backupSize,
      dataIndex: 'file_size',
      key: 'file_size',
      width: 120,
      render: (fileSize: number) => formatFileSize(fileSize),
    },
    {
      title: t.settings.backupCreatedAt,
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (createdAt: string) => dayjs(createdAt).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: t.settings.backupCompletedAt,
      dataIndex: 'completed_at',
      key: 'completed_at',
      width: 180,
      render: (completedAt?: string | null) =>
        completedAt ? dayjs(completedAt).format('YYYY-MM-DD HH:mm:ss') : '-',
    },
    {
      title: t.settings.backupError,
      dataIndex: 'error_msg',
      key: 'error_msg',
      ellipsis: true,
      render: (errorMsg: string) => errorMsg || '-',
    },
    {
      title: t.common.actions,
      key: 'actions',
      width: 160,
      render: (_value, record) => (
        <Space>
          <Button
            size="small"
            icon={<DownloadOutlined />}
            onClick={() => handlers.onDownload(record)}
            disabled={record.status !== 'completed'}
          >
            {t.common.export}
          </Button>
          <Popconfirm
            title={t.settings.confirmDeleteBackup}
            onConfirm={() => handlers.onDelete(record.id)}
            okText={t.common.delete}
            cancelText={t.common.cancel}
          >
            <Button
              size="small"
              danger
              icon={<DeleteOutlined />}
              disabled={record.status === 'running'}
            >
              {t.common.delete}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];
}
