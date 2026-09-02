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
  Tabs,
} from 'antd';
import {
  Save,
  Database,
  RefreshCw,
  FileArchive,
  Inbox,
} from 'lucide-react';
import { useStore } from '@/store';
import { settingsApi, SystemSettings } from '@/api/settings';
import {
  dataManagementApi,
  exportFormatOptions,
  type ExportFormat,
  type ExportRecord,
  type ExportType,
  type ImportRecord,
} from '@/api/dataManagement';
import type { UploadFile } from 'antd/es/upload/interface';
import type { TablePaginationConfig } from 'antd/es/table';
import dayjs from 'dayjs';
import { POLL_INTERVAL } from './helpers';
import { buildExportColumns, buildImportColumns } from './columns';

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
  const [exports, setExports] = useState<ExportRecord[]>([]);
  const [exportPage, setExportPage] = useState(1);
  const [exportPageSize, setExportPageSize] = useState(10);
  const [exportTotal, setExportTotal] = useState(0);
  const [imports, setImports] = useState<ImportRecord[]>([]);
  const [importPage, setImportPage] = useState(1);
  const [importPageSize, setImportPageSize] = useState(10);
  const [importTotal, setImportTotal] = useState(0);
  const [importFileList, setImportFileList] = useState<UploadFile[]>([]);
  const [systemForm] = Form.useForm();
  const [exportForm] = Form.useForm();
  const [importForm] = Form.useForm();

  const hasActiveExports = exports.some(
    (record) => record.status === 'pending' || record.status === 'running'
  );
  const hasActiveImports = imports.some(
    (record) => record.status === 'pending' || record.status === 'running'
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
    }
  }, [user, systemForm]);

  useEffect(() => {
    if (!user?.is_admin || (!hasActiveExports && !hasActiveImports)) {
      return;
    }

    const timer = window.setInterval(() => {
      if (hasActiveExports) {
        loadExports(exportPage, exportPageSize);
      }
      if (hasActiveImports) {
        loadImports(importPage, importPageSize);
      }
    }, POLL_INTERVAL);

    return () => window.clearInterval(timer);
  }, [user?.is_admin, hasActiveExports, hasActiveImports, exportPage, exportPageSize, importPage, importPageSize]);

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

  const openExportModal = () => {
    exportForm.setFieldsValue({
      export_type: 'configs',
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

  const exportColumns = useMemo(
    () => buildExportColumns(t, { onDownload: handleDownloadExport, onDelete: handleDeleteExport }),
    [t]
  );

  const importColumns = useMemo(
    () => buildImportColumns(t, { onDelete: handleDeleteImport }),
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
                icon={<Save size={14} strokeWidth={1.7} />}
                onClick={handleSystemUpdate}
                loading={loading}
              >
                {t.settings.saveSettings}
              </Button>
              
            </Space>
          </Form.Item>
        </Form>
      </Card>

      <Card
        title={t.settings.dataManagementTitle}
        style={{ marginBottom: 24 }}
      >
        <Tabs
          defaultActiveKey="exports"
          items={[
            {
              key: 'exports',
              label: t.settings.exportManagementTitle,
              children: (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                    <Button
                      icon={<RefreshCw size={14} strokeWidth={1.7} />}
                      onClick={() => loadExports(exportPage, exportPageSize)}
                      loading={exportsLoading}
                    >
                      {t.common.refresh}
                    </Button>
                    <Button
                      type="primary"
                      icon={<FileArchive size={14} strokeWidth={1.7} />}
                      onClick={openExportModal}
                    >
                      {t.settings.createExport}
                    </Button>
                  </div>

                  <Alert
                    message={t.settings.exportData}
                    description={t.settings.exportManagementDesc}
                    type="info"
                    showIcon
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
                </div>
              ),
            },
            {
              key: 'imports',
              label: t.settings.importManagementTitle,
              children: (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                  <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
                    <Button
                      icon={<RefreshCw size={14} strokeWidth={1.7} />}
                      onClick={() => loadImports(importPage, importPageSize)}
                      loading={importsLoading}
                    >
                      {t.common.refresh}
                    </Button>
                    <Button
                      type="primary"
                      icon={<Inbox size={14} strokeWidth={1.7} />}
                      onClick={openImportModal}
                    >
                      {t.settings.createImport}
                    </Button>
                  </div>

                  <Alert
                    message={t.settings.importData}
                    description={t.settings.importManagementDesc}
                    type="info"
                    showIcon
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
                </div>
              ),
            },
          ]}
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
                { label: t.settings.configs, value: 'configs' },
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
              const exportType = (getFieldValue('export_type') || 'configs') as ExportType;
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
              <Button icon={<Inbox size={14} strokeWidth={1.7} />}>{t.settings.selectImportFile}</Button>
            </Upload>
          </Form.Item>
        </Form>
      </Modal>

    </div>
  );
};

export default Settings;
