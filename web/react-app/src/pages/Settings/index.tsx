import { useState, useEffect, useMemo } from 'react';
import {
  Card,
  Form,
  Button,
  Switch,
  InputNumber,
  Input,
  message,
  Space,
  Divider,
  Alert,
  Modal,
  Select,
  Table,
  Typography,
  Upload,
  Spin,
} from 'antd';
import {
  SaveOutlined,
  DatabaseOutlined,
  ReloadOutlined,
  PlusOutlined,
  FileZipOutlined,
  InboxOutlined,
} from '@ant-design/icons';
import { useStore } from '@/store';
import { settingsApi, SystemSettings } from '@/api/settings';
import {
  dataManagementApi,
  exportFormatOptions,
  type BackupRecord,
  type ExportFormat,
  type ExportRecord,
  type ExportType,
  type ImportRecord,
} from '@/api/dataManagement';
import type { UploadFile } from 'antd/es/upload/interface';
import type { TablePaginationConfig } from 'antd/es/table';
import dayjs from 'dayjs';
import { BACKUP_POLL_INTERVAL, buildBackupName } from './helpers';
import { buildExportColumns, buildImportColumns, buildBackupColumns } from './columns';

const { Paragraph } = Typography;
const { TextArea } = Input;

const Settings: React.FC = () => {
  const { user, t } = useStore();
  const [loading, setLoading] = useState(false);
  const [exportsLoading, setExportsLoading] = useState(false);
  const [exportSubmitting, setExportSubmitting] = useState(false);
  const [exportModalOpen, setExportModalOpen] = useState(false);
  const [importsLoading, setImportsLoading] = useState(false);
  const [importSubmitting, setImportSubmitting] = useState(false);
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [backupsLoading, setBackupsLoading] = useState(false);
  const [backupSubmitting, setBackupSubmitting] = useState(false);
  const [backupModalOpen, setBackupModalOpen] = useState(false);
  const [exports, setExports] = useState<ExportRecord[]>([]);
  const [exportPage, setExportPage] = useState(1);
  const [exportPageSize, setExportPageSize] = useState(10);
  const [exportTotal, setExportTotal] = useState(0);
  const [imports, setImports] = useState<ImportRecord[]>([]);
  const [importPage, setImportPage] = useState(1);
  const [importPageSize, setImportPageSize] = useState(10);
  const [importTotal, setImportTotal] = useState(0);
  const [importFileList, setImportFileList] = useState<UploadFile[]>([]);
  const [backups, setBackups] = useState<BackupRecord[]>([]);
  const [backupPage, setBackupPage] = useState(1);
  const [backupPageSize, setBackupPageSize] = useState(10);
  const [backupTotal, setBackupTotal] = useState(0);
  const [systemForm] = Form.useForm();
  const [exportForm] = Form.useForm();
  const [importForm] = Form.useForm();
  const [backupForm] = Form.useForm();

  const hasActiveExports = exports.some(
    (record) => record.status === 'pending' || record.status === 'running'
  );
  const hasActiveImports = imports.some(
    (record) => record.status === 'pending' || record.status === 'running'
  );
  const hasActiveBackups = backups.some(
    (backup) => backup.status === 'pending' || backup.status === 'running'
  );

  // 加载系统设置
  useEffect(() => {
    const loadSystemSettings = async () => {
      if (!user?.is_admin) return;
      
      try {
        const response = await settingsApi.getSystemSettings();
        const settings = response.settings || {};
        const wsEnabled = settings.enable_websocket !== 'false';
        systemForm.setFieldsValue({
          refresh_interval: parseInt(settings.refresh_interval) || 30,
          log_retention_days: parseInt(settings.log_retention_days) || 30,
          max_concurrent_connections: parseInt(settings.max_concurrent_connections) || 100,
          enable_websocket: wsEnabled,
          auto_refresh: settings.enable_activity_logging !== 'false',
        });
        // Sync settings to store
        const { setWebsocketEnabled, setAutoRefreshEnabled, setRefreshInterval } = useStore.getState();
        setWebsocketEnabled(wsEnabled);
        setAutoRefreshEnabled(settings.enable_activity_logging !== 'false');
        setRefreshInterval(parseInt(settings.refresh_interval) || 30);
      } catch (error) {
        console.error('Failed to load system settings:', error);
        systemForm.setFieldsValue({
          refresh_interval: 30,
          log_retention_days: 30,
          max_concurrent_connections: 100,
          enable_websocket: true,
          auto_refresh: true,
        });
      }
    };

    if (user?.is_admin) {
      loadSystemSettings();
      loadExports(1, exportPageSize);
      loadImports(1, importPageSize);
      loadBackups(1, backupPageSize);
    }
  }, [user, systemForm]);

  useEffect(() => {
    if (!user?.is_admin || (!hasActiveBackups && !hasActiveExports && !hasActiveImports)) {
      return;
    }

    const timer = window.setInterval(() => {
      if (hasActiveExports) {
        loadExports(exportPage, exportPageSize);
      }
      if (hasActiveImports) {
        loadImports(importPage, importPageSize);
      }
      loadBackups(backupPage, backupPageSize);
    }, BACKUP_POLL_INTERVAL);

    return () => window.clearInterval(timer);
  }, [user?.is_admin, hasActiveBackups, hasActiveExports, hasActiveImports, backupPage, backupPageSize, exportPage, exportPageSize, importPage, importPageSize]);

  async function loadExports(page: number = exportPage, pageSize: number = exportPageSize) {
    setExportsLoading(true);
    try {
      const response = await dataManagementApi.getExportRecords(page, pageSize);
      setExports(response.data || []);
      setExportPage(response.page || page);
      setExportPageSize(response.page_size || pageSize);
      setExportTotal(response.total || 0);
    } catch (error) {
      console.error('Failed to load exports:', error);
      message.error(t.settings.exportLoadFailed);
    } finally {
      setExportsLoading(false);
    }
  }

  async function loadImports(page: number = importPage, pageSize: number = importPageSize) {
    setImportsLoading(true);
    try {
      const response = await dataManagementApi.getImportRecords(page, pageSize);
      setImports(response.data || []);
      setImportPage(response.page || page);
      setImportPageSize(response.page_size || pageSize);
      setImportTotal(response.total || 0);
    } catch (error) {
      console.error('Failed to load imports:', error);
      message.error(t.settings.importLoadFailed);
    } finally {
      setImportsLoading(false);
    }
  }

  async function loadBackups(page: number = backupPage, pageSize: number = backupPageSize) {
    setBackupsLoading(true);
    try {
      const response = await dataManagementApi.getBackupRecords(page, pageSize);
      setBackups(response.data || []);
      setBackupPage(response.page || page);
      setBackupPageSize(response.page_size || pageSize);
      setBackupTotal(response.total || 0);
    } catch (error) {
      console.error('Failed to load backups:', error);
      message.error(t.settings.backupLoadFailed);
    } finally {
      setBackupsLoading(false);
    }
  }

  const handleSystemUpdate = async () => {
    try {
      const values = await systemForm.validateFields();
      setLoading(true);
      
      const systemSettings: Partial<SystemSettings> = {
        refresh_interval: values.refresh_interval,
        log_retention_days: values.log_retention_days,
        max_concurrent_connections: values.max_concurrent_connections,
        enable_websocket: values.enable_websocket,
        enable_activity_logging: values.auto_refresh,
      };
      
      await settingsApi.updateSystemSettings(systemSettings);
      
      // Update local store
      const { setWebsocketEnabled, setAutoRefreshEnabled, setRefreshInterval } = useStore.getState();
      setWebsocketEnabled(values.enable_websocket);
      setAutoRefreshEnabled(values.auto_refresh);
      setRefreshInterval(values.refresh_interval);
      
      message.success(t.settings.systemSettingsUpdated);
    } catch (error: any) {
      console.error('Failed to update system settings:', error);
      message.error(error.response?.data?.message || 'Failed to update system settings');
    } finally {
      setLoading(false);
    }
  };

  const openBackupModal = () => {
    backupForm.setFieldsValue({
      backup_type: 'full',
      name: buildBackupName('full'),
      description: '',
    });
    setBackupModalOpen(true);
  };

  const openExportModal = () => {
    exportForm.setFieldsValue({
      export_type: 'users',
      format: 'json',
    });
    setExportModalOpen(true);
  };

  const openImportModal = () => {
    importForm.setFieldsValue({
      overwrite_existing: false,
    });
    setImportFileList([]);
    setImportModalOpen(true);
  };

  const handleExportTypeChange = (exportType: ExportType) => {
    const formats = exportFormatOptions[exportType];
    const currentFormat = exportForm.getFieldValue('format') as ExportFormat | undefined;
    if (!currentFormat || !formats.includes(currentFormat)) {
      exportForm.setFieldValue('format', formats[0]);
    }
  };

  const handleCreateExport = async () => {
    try {
      const values = await exportForm.validateFields();
      setExportSubmitting(true);

      await dataManagementApi.createExport(values);
      message.success(t.settings.exportStarted);
      setExportModalOpen(false);
      exportForm.resetFields();
      loadExports(1, exportPageSize);
    } catch (error: any) {
      if (error?.errorFields) {
        return;
      }

      console.error('Failed to create export:', error);
      message.error(error.response?.data?.error || t.settings.exportCreateFailed);
    } finally {
      setExportSubmitting(false);
    }
  };

  const handleCreateImport = async () => {
    try {
      const values = await importForm.validateFields();
      const file = importFileList[0]?.originFileObj;
      if (!file) {
        message.error(t.settings.selectImportFile);
        return;
      }

      setImportSubmitting(true);
      await dataManagementApi.importConfigurations(file, values.overwrite_existing ?? false);
      message.success(t.settings.importStarted);
      setImportModalOpen(false);
      importForm.resetFields();
      setImportFileList([]);
      loadImports(1, importPageSize);
    } catch (error: any) {
      if (error?.errorFields) {
        return;
      }

      console.error('Failed to create import:', error);
      message.error(error.message || t.settings.importCreateFailed);
    } finally {
      setImportSubmitting(false);
    }
  };

  const handleBackupTypeChange = (backupType: 'full' | 'config_only') => {
    const currentName = backupForm.getFieldValue('name');
    if (!currentName || currentName.startsWith('full-backup-') || currentName.startsWith('config-backup-')) {
      backupForm.setFieldValue('name', buildBackupName(backupType));
    }
  };

  const handleCreateBackup = async () => {
    try {
      const values = await backupForm.validateFields();
      setBackupSubmitting(true);

      await dataManagementApi.createBackup(values);
      message.success(t.settings.databaseBackupStarted);
      setBackupModalOpen(false);
      backupForm.resetFields();
      loadBackups(1, backupPageSize);
    } catch (error: any) {
      if (error?.errorFields) {
        return;
      }

      console.error('Failed to create backup:', error);
      message.error(error.response?.data?.error || t.settings.backupCreateFailed);
    } finally {
      setBackupSubmitting(false);
    }
  };

  const handleDownloadBackup = async (record: BackupRecord) => {
    try {
      const blob = await dataManagementApi.downloadBackupFile(record.id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = record.file_path?.split('/').pop() || `${record.name}.zip`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Failed to download backup:', error);
      message.error(t.settings.backupDownloadFailed);
    }
  };

  const handleDeleteBackup = async (id: string) => {
    try {
      await dataManagementApi.deleteBackupRecord(id);
      message.success(t.settings.backupDeleteSuccess);
      loadBackups(backupPage, backupPageSize);
    } catch (error: any) {
      console.error('Failed to delete backup:', error);
      message.error(error.response?.data?.error || t.settings.backupDeleteFailed);
    }
  };

  const handleDownloadExport = async (record: ExportRecord) => {
    try {
      const blob = await dataManagementApi.downloadExportFile(record.id);
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = record.file_path?.split('/').pop() || `${record.name}.${record.format}`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Failed to download export:', error);
      message.error(t.settings.exportDownloadFailed);
    }
  };

  const handleDeleteExport = async (id: string) => {
    try {
      await dataManagementApi.deleteExportRecord(id);
      message.success(t.settings.exportDeleteSuccess);
      loadExports(exportPage, exportPageSize);
    } catch (error: any) {
      console.error('Failed to delete export:', error);
      message.error(error.response?.data?.error || t.settings.exportDeleteFailed);
    }
  };

  const handleDeleteImport = async (id: string) => {
    try {
      await dataManagementApi.deleteImportRecord(id);
      message.success(t.settings.importDeleteSuccess);
      loadImports(importPage, importPageSize);
    } catch (error: any) {
      console.error('Failed to delete import:', error);
      message.error(error.response?.data?.error || t.settings.importDeleteFailed);
    }
  };

  const handleExportTableChange = (pagination: TablePaginationConfig) => {
    const nextPage = pagination.current || 1;
    const nextPageSize = pagination.pageSize || exportPageSize;
    loadExports(nextPage, nextPageSize);
  };

  const handleImportTableChange = (pagination: TablePaginationConfig) => {
    const nextPage = pagination.current || 1;
    const nextPageSize = pagination.pageSize || importPageSize;
    loadImports(nextPage, nextPageSize);
  };

  const handleBackupTableChange = (pagination: TablePaginationConfig) => {
    const nextPage = pagination.current || 1;
    const nextPageSize = pagination.pageSize || backupPageSize;
    loadBackups(nextPage, nextPageSize);
  };

  const exportColumns = useMemo(
    () => buildExportColumns(t, { onDownload: handleDownloadExport, onDelete: handleDeleteExport }),
    [t]
  );

  const importColumns = useMemo(
    () => buildImportColumns(t, { onDelete: handleDeleteImport }),
    [t]
  );

  const backupColumns = useMemo(
    () => buildBackupColumns(t, { onDownload: handleDownloadBackup, onDelete: handleDeleteBackup }),
    [t]
  );

  if (!user) {
    // User data not loaded yet (e.g. token validation in progress) - show loading state
    return (
      <div style={{ padding: 24, display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '50vh' }}>
        <Spin size="large" />
      </div>
    );
  }

  if (!user.is_admin) {
    return (
      <div style={{ padding: 24 }}>
        <Alert
          message={t.common.error}
          description={t.settings.adminOnlyAccess}
          type="error"
          showIcon
        />
      </div>
    );
  }

  return (
    <div style={{ padding: 24 }}>
      <h1 style={{ marginBottom: 16 }}>{t.settings.title}</h1>

      <Card style={{ marginBottom: 24 }}>
        <Alert
          message={t.settings.system}
          description={t.settings.systemSettingsDesc}
          type="warning"
          showIcon
          style={{ marginBottom: 24 }}
        />
        
        <Form
          form={systemForm}
          layout="vertical"
          style={{ maxWidth: 600 }}
          initialValues={{
            refresh_interval: 30,
            log_retention_days: 30,
            max_concurrent_connections: 100,
            enable_websocket: true,
            auto_refresh: true,
          }}
        >
          <Form.Item
            name="refresh_interval"
            label={t.settings.refreshInterval + ' (seconds)'}
            help={t.settings.refreshIntervalHelp}
            rules={[{ required: true }]}
          >
            <InputNumber min={5} max={300} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="log_retention_days"
            label={t.settings.dataRetention + ' (days)'}
            rules={[{ required: true }]}
          >
            <InputNumber min={1} max={365} style={{ width: '100%' }} />
          </Form.Item>

          <Form.Item
            name="max_concurrent_connections"
            label={t.settings.maxConcurrentConnections}
            rules={[{ required: true }]}
          >
            <InputNumber min={10} max={1000} style={{ width: '100%' }} />
          </Form.Item>

          <Divider />

          <Form.Item
            name="enable_websocket"
            label={t.settings.websocketEnabled}
            valuePropName="checked"
            help={t.settings.websocketHelp}
          >
            <Switch />
          </Form.Item>

          <Form.Item
            name="auto_refresh"
            label={t.settings.autoRefresh}
            valuePropName="checked"
            help={t.settings.autoRefreshHelp}
          >
            <Switch />
          </Form.Item>

          <Form.Item>
            <Space>
              <Button
                type="primary"
                icon={<SaveOutlined />}
                onClick={handleSystemUpdate}
                loading={loading}
              >
                {t.settings.saveSettings}
              </Button>
              <Button
                icon={<DatabaseOutlined />}
                onClick={openBackupModal}
              >
                {t.settings.backupNow}
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card
        title={t.settings.exportManagementTitle}
        extra={
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => loadExports(exportPage, exportPageSize)}
              loading={exportsLoading}
            >
              {t.common.refresh}
            </Button>
            <Button
              type="primary"
              icon={<FileZipOutlined />}
              onClick={openExportModal}
            >
              {t.settings.createExport}
            </Button>
          </Space>
        }
        style={{ marginBottom: 24 }}
      >
        <Alert
          message={t.settings.exportData}
          description={t.settings.exportManagementDesc}
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />

        {hasActiveExports ? (
          <Paragraph type="secondary" style={{ marginTop: 0 }}>
            {t.settings.exportPollingHint}
          </Paragraph>
        ) : null}

        <Table
          columns={exportColumns}
          dataSource={exports}
          rowKey="id"
          loading={exportsLoading}
          pagination={{
            current: exportPage,
            pageSize: exportPageSize,
            total: exportTotal,
            showSizeChanger: true,
            pageSizeOptions: ['10', '20', '50'],
          }}
          locale={{
            emptyText: t.settings.exportNoData,
          }}
          onChange={handleExportTableChange}
          scroll={{ x: 1250 }}
        />
      </Card>

      <Card
        title={t.settings.importManagementTitle}
        extra={
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => loadImports(importPage, importPageSize)}
              loading={importsLoading}
            >
              {t.common.refresh}
            </Button>
            <Button
              type="primary"
              icon={<InboxOutlined />}
              onClick={openImportModal}
            >
              {t.settings.createImport}
            </Button>
          </Space>
        }
        style={{ marginBottom: 24 }}
      >
        <Alert
          message={t.settings.importData}
          description={t.settings.importManagementDesc}
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />

        {hasActiveImports ? (
          <Paragraph type="secondary" style={{ marginTop: 0 }}>
            {t.settings.importPollingHint}
          </Paragraph>
        ) : null}

        <Table
          columns={importColumns}
          dataSource={imports}
          rowKey="id"
          loading={importsLoading}
          pagination={{
            current: importPage,
            pageSize: importPageSize,
            total: importTotal,
            showSizeChanger: true,
            pageSizeOptions: ['10', '20', '50'],
          }}
          locale={{
            emptyText: t.settings.importNoData,
          }}
          onChange={handleImportTableChange}
          scroll={{ x: 1450 }}
        />
      </Card>

      <Card
        title={t.settings.backupManagementTitle}
        extra={
          <Space>
            <Button
              icon={<ReloadOutlined />}
              onClick={() => loadBackups(backupPage, backupPageSize)}
              loading={backupsLoading}
            >
              {t.common.refresh}
            </Button>
            <Button
              type="primary"
              icon={<PlusOutlined />}
              onClick={openBackupModal}
            >
              {t.settings.createBackup}
            </Button>
          </Space>
        }
      >
        <Alert
          message={t.settings.backup}
          description={t.settings.backupManagementDesc}
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />

        {hasActiveBackups ? (
          <Paragraph type="secondary" style={{ marginTop: 0 }}>
            {t.settings.backupPollingHint}
          </Paragraph>
        ) : null}

        <Table
          columns={backupColumns}
          dataSource={backups}
          rowKey="id"
          loading={backupsLoading}
          pagination={{
            current: backupPage,
            pageSize: backupPageSize,
            total: backupTotal,
            showSizeChanger: true,
            pageSizeOptions: ['10', '20', '50'],
          }}
          locale={{
            emptyText: t.settings.backupNoData,
          }}
          onChange={handleBackupTableChange}
          scroll={{ x: 1100 }}
        />
      </Card>

      <Modal
        title={t.settings.createExport}
        open={exportModalOpen}
        onOk={handleCreateExport}
        onCancel={() => setExportModalOpen(false)}
        confirmLoading={exportSubmitting}
        okText={t.common.create}
        cancelText={t.common.cancel}
        destroyOnClose
      >
        <Form form={exportForm} layout="vertical">
          <Form.Item
            name="export_type"
            label={t.settings.exportType}
            rules={[{ required: true, message: t.settings.selectExportType }]}
          >
            <Select
              options={[
                { label: t.settings.users, value: 'users' },
                { label: t.settings.logs, value: 'logs' },
                { label: t.settings.configs, value: 'configs' },
                { label: t.settings.processes, value: 'processes' },
                { label: t.settings.all, value: 'all' },
              ]}
              onChange={handleExportTypeChange}
            />
          </Form.Item>

          <Form.Item
            shouldUpdate={(prevValues, currentValues) => prevValues.export_type !== currentValues.export_type}
            noStyle
          >
            {({ getFieldValue }) => {
              const exportType = (getFieldValue('export_type') || 'users') as ExportType;
              return (
                <Form.Item
                  name="format"
                  label={t.settings.exportFormat}
                  rules={[{ required: true, message: t.settings.selectExportFormat }]}
                >
                  <Select
                    options={exportFormatOptions[exportType].map((format) => ({
                      label: format.toUpperCase(),
                      value: format,
                    }))}
                  />
                </Form.Item>
              );
            }}
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t.settings.createImport}
        open={importModalOpen}
        onOk={handleCreateImport}
        onCancel={() => setImportModalOpen(false)}
        confirmLoading={importSubmitting}
        okText={t.common.create}
        cancelText={t.common.cancel}
        destroyOnClose
      >
        <Form form={importForm} layout="vertical" initialValues={{ overwrite_existing: false }}>
          <Form.Item label={t.settings.importSupportedType}>
            <Input value={t.settings.configs} disabled />
          </Form.Item>

          <Form.Item
            name="overwrite_existing"
            label={t.settings.overwriteExisting}
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>

          <Form.Item
            label={t.settings.importFile}
            required
            extra={t.settings.importFileHelp}
          >
            <Upload
              accept=".json,application/json"
              beforeUpload={() => false}
              maxCount={1}
              fileList={importFileList}
              onChange={({ fileList }) => setImportFileList(fileList)}
            >
              <Button icon={<InboxOutlined />}>{t.settings.selectImportFile}</Button>
            </Upload>
          </Form.Item>
        </Form>
      </Modal>

      <Modal
        title={t.settings.createBackup}
        open={backupModalOpen}
        onOk={handleCreateBackup}
        onCancel={() => setBackupModalOpen(false)}
        confirmLoading={backupSubmitting}
        okText={t.common.create}
        cancelText={t.common.cancel}
        destroyOnClose
      >
        <Form form={backupForm} layout="vertical">
          <Form.Item
            name="backup_type"
            label={t.settings.backupType}
            rules={[{ required: true, message: t.settings.selectBackupType }]}
          >
            <Select
              options={[
                { label: t.settings.fullBackup, value: 'full' },
                { label: t.settings.configOnlyBackup, value: 'config_only' },
              ]}
              onChange={handleBackupTypeChange}
            />
          </Form.Item>

          <Form.Item
            name="name"
            label={t.settings.backupName}
            rules={[{ required: true, message: t.settings.enterBackupName }]}
          >
            <Input maxLength={100} />
          </Form.Item>

          <Form.Item
            name="description"
            label={t.settings.backupDescription}
          >
            <TextArea rows={3} maxLength={500} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default Settings;
