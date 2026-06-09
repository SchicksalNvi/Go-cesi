import { useEffect, useState } from 'react';
import {
  Card,
  Input,
  Select,
  Button,
  Space,
  Table,
  Tag,
  DatePicker,
  message,
  Alert,
} from 'antd';
import {
  SearchOutlined,
  ReloadOutlined,
  DownloadOutlined,
  DeleteOutlined,
} from '@ant-design/icons';
import { Modal } from 'antd';
import type { ColumnsType, TablePaginationConfig } from 'antd/es/table';
import dayjs, { Dayjs } from 'dayjs';
import { activityLogsAPI } from '../../api/activityLogs';
import type { ActivityLog, ActivityLogsFilters, PaginationInfo } from '../../api/activityLogs';
import { useAutoRefresh } from '../../hooks/useAutoRefresh';
import { useStore } from '../../store';

const { RangePicker } = DatePicker;

const Logs: React.FC = () => {
  const { t, user } = useStore();
  const [activityLogs, setActivityLogs] = useState<ActivityLog[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searchText, setSearchText] = useState('');
  const [levelFilter, setLevelFilter] = useState<string>('');
  const [actionFilter, setActionFilter] = useState<string>('');
  const [resourceFilter, setResourceFilter] = useState<string>('');
  const [usernameFilter, setUsernameFilter] = useState<string>('');
  const [dateRange, setDateRange] = useState<[Dayjs | null, Dayjs | null] | null>(null);
  const [pagination, setPagination] = useState<PaginationInfo>({
    page: 1,
    page_size: 20,
    total: 0,
    total_pages: 0,
    has_next: false,
    has_prev: false,
  });

  useEffect(() => {
    loadLogs();
  }, []);

  // Auto refresh (only on first page)
  useAutoRefresh(() => {
    if (pagination.page === 1) {
      loadLogs();
    }
  });

  const buildFilters = (): ActivityLogsFilters => {
    const filters: ActivityLogsFilters = {
      page: pagination.page,
      page_size: pagination.page_size,
    };

    if (levelFilter) filters.level = levelFilter;
    if (actionFilter) filters.action = actionFilter;
    if (resourceFilter) filters.resource = resourceFilter;
    if (usernameFilter) filters.username = usernameFilter;
    
    if (dateRange && dateRange[0] && dateRange[1]) {
      filters.start_time = dateRange[0].toISOString();
      filters.end_time = dateRange[1].toISOString();
    }

    return filters;
  };

  async function loadLogs(customFilters?: Partial<ActivityLogsFilters>) {
    setLoading(true);
    setError(null);
    
    try {
      const filters = customFilters ? { ...buildFilters(), ...customFilters } : buildFilters();
      const response = await activityLogsAPI.getActivityLogs(filters);
      
      setActivityLogs(response.data.logs);
      setPagination(response.data.pagination);
    } catch (err) {
      console.error('Failed to load logs:', err);
      const errorMessage = err instanceof Error ? err.message : 'Failed to load activity logs';
      setError(errorMessage);
      message.error(errorMessage);
    } finally {
      setLoading(false);
    }
  }

  const handleSearch = () => {
    setPagination(prev => ({ ...prev, page: 1 }));
    loadLogs({ page: 1 });
  };

  const handleExport = async () => {
    try {
      message.loading({ content: 'Exporting logs...', key: 'export' });
      const filters = buildFilters();
      const blob = await activityLogsAPI.exportLogs(filters);
      
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `activity-logs-${dayjs().format('YYYY-MM-DD-HHmmss')}.csv`;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      URL.revokeObjectURL(url);
      
      message.success({ content: 'Logs exported successfully', key: 'export' });
    } catch (err) {
      console.error('Failed to export logs:', err);
      message.error({ content: 'Failed to export logs', key: 'export' });
    }
  };

  const hasFilters = () => {
    return !!(levelFilter || actionFilter || resourceFilter || usernameFilter || (dateRange && dateRange[0] && dateRange[1]));
  };

  const handleClearLogs = () => {
    const filtered = hasFilters();
    Modal.confirm({
      title: filtered ? 'Clear Filtered Logs' : 'Clear All Logs',
      content: filtered 
        ? 'Are you sure you want to delete all logs matching the current filters? This action cannot be undone.'
        : 'Are you sure you want to delete ALL activity logs? This action cannot be undone.',
      okText: 'Delete',
      okType: 'danger',
      cancelText: 'Cancel',
      onOk: async () => {
        try {
          message.loading({ content: 'Deleting logs...', key: 'delete' });
          const filters = buildFilters();
          // Remove pagination from delete filters
          delete filters.page;
          delete filters.page_size;
          
          const result = await activityLogsAPI.deleteLogs(filters);
          message.success({ content: `Deleted ${result.deleted} logs`, key: 'delete' });
          loadLogs({ page: 1 });
        } catch (err) {
          console.error('Failed to delete logs:', err);
          message.error({ content: 'Failed to delete logs', key: 'delete' });
        }
      },
    });
  };

  const getLevelColor = (level: string) => {
    const upperLevel = level.toUpperCase();
    switch (upperLevel) {
      case 'ERROR': return 'red';
      case 'WARNING': return 'orange';
      case 'INFO': return 'blue';
      case 'DEBUG': return 'default';
      default: return 'default';
    }
  };

  const getActionColor = (action: string) => {
    if (action.includes('create')) return 'green';
    if (action.includes('delete')) return 'red';
    if (action.includes('update') || action.includes('edit')) return 'orange';
    if (action.includes('start')) return 'blue';
    if (action.includes('stop')) return 'volcano';
    return 'default';
  };

  const handleTableChange = (newPagination: TablePaginationConfig) => {
    const newPage = newPagination.current || 1;
    const newPageSize = newPagination.pageSize || 20;
    
    setPagination(prev => ({
      ...prev,
      page: newPage,
      page_size: newPageSize,
    }));
    
    loadLogs({ page: newPage, page_size: newPageSize });
  };

  const activityLogColumns: ColumnsType<ActivityLog> = [
    {
      title: t.logs.logTime,
      dataIndex: 'created_at',
      key: 'created_at',
      width: 180,
      render: (timestamp: string) => dayjs(timestamp).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: t.logs.logLevel,
      dataIndex: 'level',
      key: 'level',
      width: 100,
      render: (level: string) => (
        <Tag color={getLevelColor(level)}>
          {level.toUpperCase()}
        </Tag>
      ),
    },
    {
      title: t.users.username,
      dataIndex: 'username',
      key: 'username',
      width: 120,
      render: (username: string) => username || 'system',
    },
    {
      title: t.common.operation,
      dataIndex: 'action',
      key: 'action',
      width: 150,
      render: (action: string) => (
        <Tag color={getActionColor(action)}>
          {action.replace(/_/g, ' ').toUpperCase()}
        </Tag>
      ),
    },
    {
      title: t.logs.logSource,
      dataIndex: 'resource',
      key: 'resource',
      width: 120,
    },
    {
      title: 'Target',
      dataIndex: 'target',
      key: 'target',
      width: 150,
      ellipsis: true,
    },
    {
      title: t.logs.logMessage,
      dataIndex: 'message',
      key: 'message',
      ellipsis: true,
    },
    {
      title: 'IP',
      dataIndex: 'ip_address',
      key: 'ip_address',
      width: 140,
    },
  ];

  // Client-side search filter for display purposes
  const displayedLogs = searchText
    ? activityLogs.filter(log =>
        log.message.toLowerCase().includes(searchText.toLowerCase()) ||
        log.username?.toLowerCase().includes(searchText.toLowerCase()) ||
        log.action.toLowerCase().includes(searchText.toLowerCase()) ||
        log.target?.toLowerCase().includes(searchText.toLowerCase())
      )
    : activityLogs;

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h1 style={{ margin: 0 }}>{t.logs.activityLogs}</h1>
        <Space>
          <Button
            icon={<DownloadOutlined />}
            onClick={handleExport}
            disabled={loading}
          >
            {t.common.export}
          </Button>
          {user?.is_admin && (
            <Button
              danger
              icon={<DeleteOutlined />}
              onClick={handleClearLogs}
              disabled={loading}
            >
              {t.logs.clearLogs}
            </Button>
          )}
          <Button
            type="primary"
            icon={<ReloadOutlined />}
            onClick={() => loadLogs()}
            loading={loading}
          >
            {t.common.refresh}
          </Button>
        </Space>
      </div>

      {error && (
        <Alert
          message="Error"
          description={error}
          type="error"
          closable
          onClose={() => setError(null)}
          style={{ marginBottom: 16 }}
        />
      )}

      {/* 筛选器 */}
      <Card style={{ marginBottom: 16 }}>
        <Space wrap style={{ width: '100%' }}>
          <Input
            placeholder={t.common.search + '...'}
            prefix={<SearchOutlined />}
            style={{ width: 300 }}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            allowClear
          />
          <Select
            style={{ width: 150 }}
            placeholder={t.logs.logLevel}
            value={levelFilter || undefined}
            onChange={setLevelFilter}
            allowClear
            options={[
              { label: t.logs.info, value: 'INFO' },
              { label: t.logs.warn, value: 'WARNING' },
              { label: t.logs.error, value: 'ERROR' },
              { label: t.logs.debug, value: 'DEBUG' },
            ]}
          />
          <Select
            style={{ width: 150 }}
            placeholder={t.common.operation}
            value={actionFilter || undefined}
            onChange={setActionFilter}
            allowClear
            options={[
              { label: t.processes.start, value: 'start_process' },
              { label: t.processes.stop, value: 'stop_process' },
              { label: t.processes.restart, value: 'restart_process' },
              { label: 'Login', value: 'login' },
              { label: t.nav.logout, value: 'logout' },
              { label: t.users.addUser, value: 'create_user' },
              { label: t.users.deleteUser, value: 'delete_user' },
              { label: t.users.resetPassword, value: 'change_password' },
            ]}
          />
          <Select
            style={{ width: 150 }}
            placeholder={t.logs.logSource}
            value={resourceFilter || undefined}
            onChange={setResourceFilter}
            allowClear
            options={[
              { label: t.nav.processes, value: 'process' },
              { label: t.nav.nodes, value: 'node' },
              { label: t.nav.users, value: 'user' },
              { label: 'Auth', value: 'auth' },
              { label: 'System', value: 'system' },
            ]}
          />
          <Input
            placeholder={t.users.username}
            style={{ width: 150 }}
            value={usernameFilter}
            onChange={(e) => setUsernameFilter(e.target.value)}
            allowClear
          />
          <RangePicker
            showTime
            format="YYYY-MM-DD HH:mm"
            placeholder={['Start Time', 'End Time']}
            value={dateRange}
            onChange={setDateRange}
          />
          <Button
            type="primary"
            icon={<SearchOutlined />}
            onClick={handleSearch}
            loading={loading}
          >
            {t.common.search}
          </Button>
        </Space>
      </Card>

      {/* 日志表格 */}
      <Card>
        <Table
          columns={activityLogColumns}
          dataSource={displayedLogs}
          rowKey="id"
          loading={loading}
          pagination={{
            current: pagination.page,
            pageSize: pagination.page_size,
            total: pagination.total,
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total, range) => `${range[0]}-${range[1]} of ${total} logs`,
            pageSizeOptions: ['10', '20', '50', '100'],
          }}
          onChange={handleTableChange}
          scroll={{ x: 1200 }}
          size="small"
        />
      </Card>
    </div>
  );
};

export default Logs;
