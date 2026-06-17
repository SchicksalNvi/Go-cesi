import { useState, useEffect, useCallback } from 'react';
import {
  Card,
  Form,
  Input,
  InputNumber,
  Button,
  Table,
  Progress,
  Tag,
  Space,
  message,
  Modal,
  Tooltip,
  Empty,
  Spin,
  Tabs,
  Badge,
  Typography,
  Popconfirm,
  Alert,
  Select,
} from 'antd';
import type { TabsProps } from 'antd';
import {
  SearchOutlined,
  StopOutlined,
  DeleteOutlined,
  ReloadOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ClockCircleOutlined,
  ExclamationCircleOutlined,
  EyeOutlined,
  RadarChartOutlined,
  LinkOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { useNavigate } from 'react-router-dom';
import {
  discoveryApi,
  DiscoveryTask,
  DiscoveryResult,
  DiscoveryStatusType,
  StartDiscoveryRequest,
} from '@/api/discovery';
import { useWebSocket } from '@/hooks/useWebSocket';
import { WebSocketMessage } from '@/types';
import { useStore } from '@/store';

const { Text, Title } = Typography;

// CIDR validation regex
const CIDR_REGEX = /^(\d{1,3}\.){3}\d{1,3}\/\d{1,2}$/;

// Status tag colors
const statusColors: Record<string, string> = {
  pending: 'default',
  running: 'processing',
  completed: 'success',
  cancelled: 'warning',
  failed: 'error',
};

const resultStatusColors: Record<string, string> = {
  success: 'success',
  timeout: 'warning',
  connection_refused: 'default',
  auth_failed: 'error',
  error: 'error',
};

interface DiscoveryProgress {
  task_id: number;
  scanned_ips: number;
  total_ips: number;
  found_nodes: number;
  failed_ips: number;
  percent: number;
}

interface DiscoveredNode {
  task_id: number;
  ip: string;
  port: number;
  node_name: string;
  version: string;
}

// Helper to extract error message from various error formats
const getErrorMessage = (error: any, fallback: string): string => {
  if (!error) return fallback;
  const data = error.response?.data;
  if (!data) return fallback;
  if (typeof data === 'string') return data;
  if (typeof data.error === 'string') return data.error;
  if (typeof data.message === 'string') return data.message;
  if (data.error?.message) return data.error.message;
  return fallback;
};

export default function DiscoveryPage() {
  const navigate = useNavigate();
  const { t } = useStore();
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);
  const [tasks, setTasks] = useState<DiscoveryTask[]>([]);
  const [tasksLoading, setTasksLoading] = useState(false);
  const [pagination, setPagination] = useState({ page: 1, limit: 10, total: 0 });
  const [statusFilter, setStatusFilter] = useState<DiscoveryStatusType | ''>('');
  const [activeTask, setActiveTask] = useState<DiscoveryTask | null>(null);
  const [activeResults, setActiveResults] = useState<DiscoveryResult[]>([]);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [detailLoading, setDetailLoading] = useState(false);
  const [cidrValidation, setCidrValidation] = useState<{ valid: boolean; count: number; error?: string } | null>(null);
  const [validatingCidr, setValidatingCidr] = useState(false);
  const [recentlyDiscovered, setRecentlyDiscovered] = useState<DiscoveredNode[]>([]);
  const [activeTab, setActiveTab] = useState('new');

  // Load tasks on mount
  useEffect(() => {
    loadTasks();
  }, [pagination.page, pagination.limit, statusFilter]);

  // WebSocket for real-time updates
  useWebSocket({
    onMessage: useCallback((msg: WebSocketMessage) => {
      if (msg.type === 'discovery_progress') {
        const progress = msg.data as DiscoveryProgress;
        setTasks(prev => prev.map(t => 
          t.id === progress.task_id 
            ? { ...t, scanned_ips: progress.scanned_ips, found_nodes: progress.found_nodes, failed_ips: progress.failed_ips }
            : t
        ));
        if (activeTask?.id === progress.task_id) {
          setActiveTask(prev => prev ? { ...prev, scanned_ips: progress.scanned_ips, found_nodes: progress.found_nodes, failed_ips: progress.failed_ips } : null);
        }
      } else if (msg.type === 'node_discovered') {
        const node = msg.data as DiscoveredNode;
        setRecentlyDiscovered(prev => [node, ...prev.slice(0, 9)]);
        message.success(t.discovery.nodeDiscovered.replace('{node}', node.node_name));
      } else if (msg.type === 'discovery_completed') {
        const data = msg.data as { task_id: number; status: string };
        setTasks(prev => prev.map(t => 
          t.id === data.task_id ? { ...t, status: data.status as any } : t
        ));
        if (activeTask?.id === data.task_id) {
          loadTaskDetail(data.task_id);
        }
        message.info(t.discovery.taskCompleted.replace('{id}', String(data.task_id)));
      }
    }, [activeTask, t.discovery.nodeDiscovered, t.discovery.taskCompleted]),
  });

  const loadTasks = async () => {
    setTasksLoading(true);
    try {
      const response = await discoveryApi.getTasks({
        page: pagination.page,
        limit: pagination.limit,
        status: statusFilter || undefined,
      });
      setTasks(response.tasks || []);
      setPagination(prev => ({ ...prev, total: response.total }));
    } catch (error) {
      console.error('Failed to load tasks:', error);
      message.error(t.discovery.loadTasksFailed);
    } finally {
      setTasksLoading(false);
    }
  };

  const loadTaskDetail = async (taskId: number) => {
    setDetailLoading(true);
    try {
      const response = await discoveryApi.getTask(taskId);
      setActiveTask(response.task);
      setActiveResults(response.results || []);
    } catch (error) {
      console.error('Failed to load task detail:', error);
      message.error(t.discovery.loadTaskDetailsFailed);
    } finally {
      setDetailLoading(false);
    }
  };

  // CIDR validation with debounce
  const validateCidr = async (cidr: string) => {
    if (!cidr) {
      setCidrValidation(null);
      return;
    }
    if (!CIDR_REGEX.test(cidr)) {
      setCidrValidation({
        valid: false,
        count: 0,
        error: t.discovery.invalidCidrFormat,
      });
      return;
    }
    setValidatingCidr(true);
    try {
      const response = await discoveryApi.validateCIDR(cidr);
      setCidrValidation({ valid: response.valid, count: response.count });
    } catch (error: any) {
      const errorMsg = getErrorMessage(error, t.discovery.validateCidrFailed);
      setCidrValidation({ valid: false, count: 0, error: errorMsg });
    } finally {
      setValidatingCidr(false);
    }
  };

  // Start discovery
  const handleStartDiscovery = async (values: StartDiscoveryRequest) => {
    if (!cidrValidation?.valid) {
      message.error(t.discovery.enterValidCidr);
      return;
    }
    setLoading(true);
    try {
      await discoveryApi.startDiscovery(values);
      message.success(t.discovery.startedFor.replace('{cidr}', values.cidr));
      form.resetFields();
      setCidrValidation(null);
      setRecentlyDiscovered([]);
      setActiveTab('history');
      loadTasks();
    } catch (error: any) {
      const errorMsg = getErrorMessage(error, t.discovery.startFailed);
      message.error(errorMsg);
    } finally {
      setLoading(false);
    }
  };

  // Cancel task
  const handleCancelTask = async (taskId: number) => {
    try {
      await discoveryApi.cancelTask(taskId);
      message.success(t.discovery.taskCancelled);
      loadTasks();
    } catch (error) {
      message.error(t.discovery.cancelTaskFailed);
    }
  };

  // Delete task
  const handleDeleteTask = async (taskId: number) => {
    try {
      await discoveryApi.deleteTask(taskId);
      message.success(t.discovery.taskDeleted);
      loadTasks();
    } catch (error) {
      message.error(t.discovery.deleteTaskFailed);
    }
  };

  // View task details
  const handleViewTask = (task: DiscoveryTask) => {
    setActiveTask(task);
    setDetailModalVisible(true);
    loadTaskDetail(task.id);
  };

  // Task table columns
  const taskColumns: ColumnsType<DiscoveryTask> = [
    {
      title: 'ID',
      dataIndex: 'id',
      key: 'id',
      width: 60,
    },
    {
      title: t.discovery.cidrRange,
      dataIndex: 'cidr',
      key: 'cidr',
      render: (cidr: string) => <Text code>{cidr}</Text>,
    },
    {
      title: t.nodes.nodePort,
      dataIndex: 'port',
      key: 'port',
      width: 80,
    },
    {
      title: t.common.status,
      dataIndex: 'status',
      key: 'status',
      width: 120,
      render: (status: string) => (
        <Tag color={statusColors[status] || 'default'}>
          {status.toUpperCase()}
        </Tag>
      ),
    },
    {
      title: t.discovery.progress,
      key: 'progress',
      width: 200,
      render: (_, record) => {
        const percent = record.total_ips > 0 ? Math.round((record.scanned_ips / record.total_ips) * 100) : 0;
        return (
          <Space direction="vertical" size={0} style={{ width: '100%' }}>
            <Progress percent={percent} size="small" status={record.status === 'running' ? 'active' : undefined} />
            <Text type="secondary" style={{ fontSize: 12 }}>
              {t.discovery.progressSummary
                .replace('{scanned}', String(record.scanned_ips))
                .replace('{total}', String(record.total_ips))
                .replace('{found}', String(record.found_nodes))}
            </Text>
          </Space>
        );
      },
    },
    {
      title: t.users.createdAt,
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (date: string) => new Date(date).toLocaleString(),
    },
    {
      title: t.discovery.completedAt,
      dataIndex: 'completed_at',
      key: 'completed_at',
      width: 160,
      render: (date?: string) => (date ? new Date(date).toLocaleString() : '-'),
    },
    {
      title: t.discovery.errorMessage,
      dataIndex: 'error_msg',
      key: 'error_msg',
      ellipsis: true,
      render: (errorMsg?: string) => errorMsg || '-',
    },
    {
      title: t.common.actions,
      key: 'actions',
      width: 150,
      render: (_, record) => (
        <Space>
          <Tooltip title={t.common.view}>
            <Button type="text" icon={<EyeOutlined />} onClick={() => handleViewTask(record)} />
          </Tooltip>
          {record.status === 'running' && (
            <Tooltip title={t.common.cancel}>
              <Button type="text" danger icon={<StopOutlined />} onClick={() => handleCancelTask(record.id)} />
            </Tooltip>
          )}
          {['completed', 'cancelled', 'failed'].includes(record.status) && (
            <Popconfirm title={t.discovery.confirmDeleteTask} onConfirm={() => handleDeleteTask(record.id)}>
              <Tooltip title={t.common.delete}>
                <Button type="text" danger icon={<DeleteOutlined />} />
              </Tooltip>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ];

  // Result table columns
  const resultColumns: ColumnsType<DiscoveryResult> = [
    {
      title: 'IP',
      dataIndex: 'ip',
      key: 'ip',
      render: (ip: string) => <Text code>{ip}</Text>,
    },
    {
      title: t.nodes.nodePort,
      dataIndex: 'port',
      key: 'port',
      width: 80,
    },
    {
      title: t.common.status,
      dataIndex: 'status',
      key: 'status',
      width: 160,
      render: (status: string, record: DiscoveryResult) => {
        const tag = (
          <Tag color={resultStatusColors[status] || 'default'}>
            {status === 'success' && <CheckCircleOutlined />}
            {status === 'timeout' && <ClockCircleOutlined />}
            {status === 'connection_refused' && <CloseCircleOutlined />}
            {status === 'auth_failed' && <ExclamationCircleOutlined />}
            {status === 'error' && <ExclamationCircleOutlined />}
            {' '}{status.replace('_', ' ').toUpperCase()}
          </Tag>
        );
        // Show error message in tooltip for failed statuses
        if (record.error_msg && status !== 'success') {
          return <Tooltip title={record.error_msg}>{tag}</Tooltip>;
        }
        return tag;
      },
    },
    {
      title: t.nodes.nodeName,
      dataIndex: 'node_name',
      key: 'node_name',
      render: (name: string, record: DiscoveryResult) =>
        name && record.status === 'success' ? (
          <Button type="link" icon={<LinkOutlined />} onClick={() => navigate(`/nodes/${name}`)}>
            {name}
          </Button>
        ) : (
          name || '-'
        ),
    },
    {
      title: t.discovery.version,
      dataIndex: 'version',
      key: 'version',
      render: (version: string) => version || '-',
    },
    {
      title: t.discovery.duration,
      dataIndex: 'duration_ms',
      key: 'duration_ms',
      width: 100,
      render: (ms: number) => t.discovery.durationMs.replace('{ms}', String(ms)),
    },
  ];

  // New Discovery tab content
  const newDiscoveryContent = (
    <>
      <Card title={t.discovery.startScan} style={{ marginBottom: 24 }}>
        <Form
          form={form}
          layout="vertical"
          onFinish={handleStartDiscovery}
          initialValues={{ port: 9001, timeout_seconds: 3, max_workers: 50 }}
        >
          <Form.Item
            name="cidr"
            label={t.discovery.cidrRange}
            rules={[{ required: true, message: t.discovery.enterCidrRange }]}
            help={
              cidrValidation ? (
                cidrValidation.valid ? (
                  <Text type="success">
                    {t.discovery.validCidr.replace('{count}', String(cidrValidation.count))}
                  </Text>
                ) : (
                  <Text type="danger">{cidrValidation.error || t.discovery.invalidCidr}</Text>
                )
              ) : undefined
            }
            validateStatus={cidrValidation ? (cidrValidation.valid ? 'success' : 'error') : undefined}
          >
            <Input
              placeholder={t.discovery.cidrPlaceholder}
              onChange={(e) => validateCidr(e.target.value)}
              suffix={validatingCidr ? <Spin size="small" /> : <span />}
            />
          </Form.Item>

          <Space size="large" style={{ width: '100%' }}>
            <Form.Item
              name="port"
              label={t.nodes.nodePort}
              rules={[{ required: true, message: t.discovery.enterPort }]}
              style={{ width: 150 }}
            >
              <InputNumber min={1} max={65535} style={{ width: '100%' }} />
            </Form.Item>

            <Form.Item
              name="username"
              label={t.users.username}
              rules={[{ required: true, message: t.discovery.enterUsername }]}
              style={{ width: 200 }}
            >
              <Input placeholder={t.discovery.usernamePlaceholder} />
            </Form.Item>

            <Form.Item
              name="password"
              label={t.users.password}
              rules={[{ required: true, message: t.discovery.enterPassword }]}
              style={{ width: 200 }}
            >
              <Input.Password placeholder={t.users.password} />
            </Form.Item>
          </Space>

          <Space size="large">
            <Form.Item name="timeout_seconds" label={t.discovery.timeoutSeconds} style={{ width: 150 }}>
              <InputNumber min={1} max={30} style={{ width: '100%' }} />
            </Form.Item>

            <Form.Item name="max_workers" label={t.discovery.maxWorkers} style={{ width: 150 }}>
              <InputNumber min={1} max={200} style={{ width: '100%' }} />
            </Form.Item>
          </Space>

          <Form.Item>
            <Button
              type="primary"
              htmlType="submit"
              loading={loading}
              icon={<SearchOutlined />}
              disabled={!cidrValidation?.valid}
            >
              {t.discovery.startScan}
            </Button>
          </Form.Item>
        </Form>
      </Card>

      {recentlyDiscovered.length > 0 && (
        <Card title={t.discovery.discoveredNodes} size="small">
          <Space direction="vertical" style={{ width: '100%' }}>
            {recentlyDiscovered.map((node, index) => (
              <Alert
                key={`${node.ip}-${index}`}
                type="success"
                message={
                  <Space>
                    <CheckCircleOutlined />
                    <Text strong>{node.node_name}</Text>
                    <Text type="secondary">({node.ip}:{node.port})</Text>
                    <Tag color="blue">v{node.version}</Tag>
                    <Button type="link" size="small" onClick={() => navigate(`/nodes/${node.node_name}`)}>
                      {t.nodes.viewDetails}
                    </Button>
                  </Space>
                }
                showIcon={false}
              />
            ))}
          </Space>
        </Card>
      )}
    </>
  );

  // History tab content
  const historyContent = (
    <Card
      title={t.discovery.scanHistory}
      extra={
        <Space>
          <Select
            value={statusFilter || undefined}
            placeholder={t.discovery.filterByStatus}
            allowClear
            style={{ width: 180 }}
            onChange={(value) => {
              setPagination((prev) => ({ ...prev, page: 1 }));
              setStatusFilter(value || '');
            }}
            options={[
              { label: t.common.all, value: '' },
              { label: t.discovery.statusPending, value: 'pending' },
              { label: t.discovery.statusRunning, value: 'running' },
              { label: t.discovery.statusCompleted, value: 'completed' },
              { label: t.discovery.statusCancelled, value: 'cancelled' },
              { label: t.discovery.statusFailed, value: 'failed' },
            ]}
          />
          <Button icon={<ReloadOutlined />} onClick={loadTasks} loading={tasksLoading}>
            {t.common.refresh}
          </Button>
        </Space>
      }
    >
      <Table
        columns={taskColumns}
        dataSource={tasks}
        rowKey="id"
        loading={tasksLoading}
        pagination={{
          current: pagination.page,
          pageSize: pagination.limit,
          total: pagination.total,
          showSizeChanger: true,
          showTotal: (total) => `${t.common.total} ${total}`,
          onChange: (page, pageSize) => setPagination({ page, limit: pageSize, total: pagination.total }),
        }}
        locale={{
          emptyText: <Empty description={t.common.noData} />,
        }}
      />
    </Card>
  );

  // Tab items (new API)
  const tabItems: TabsProps['items'] = [
    {
      key: 'new',
      label: t.discovery.startScan,
      children: newDiscoveryContent,
    },
    {
      key: 'history',
      label: (
        <Badge count={tasks.filter(t => t.status === 'running').length} offset={[10, 0]}>
          {t.discovery.scanHistory}
        </Badge>
      ),
      children: historyContent,
    },
  ];

  return (
    <div>
      <Title level={4} style={{ marginBottom: 24 }}>
        <RadarChartOutlined /> {t.discovery.title}
      </Title>

      <Tabs activeKey={activeTab} onChange={setActiveTab} items={tabItems} />

      {/* Task Detail Modal */}
      <Modal
        title={t.discovery.taskDetails.replace('{id}', String(activeTask?.id || ''))}
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={null}
        width={900}
      >
        {detailLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <Spin size="large" />
          </div>
        ) : activeTask ? (
          <div>
            <Card size="small" style={{ marginBottom: 16 }}>
              <Space size="large" wrap>
                <div>
                  <Text type="secondary">{t.discovery.cidrRange}:</Text> <Text code>{activeTask.cidr}</Text>
                </div>
                <div>
                  <Text type="secondary">{t.nodes.nodePort}:</Text> <Text>{activeTask.port}</Text>
                </div>
                <div>
                  <Text type="secondary">{t.common.status}:</Text>{' '}
                  <Tag color={statusColors[activeTask.status]}>{activeTask.status.toUpperCase()}</Tag>
                </div>
                <div>
                  <Text type="secondary">{t.discovery.createdBy}:</Text> <Text>{activeTask.created_by}</Text>
                </div>
              </Space>
            </Card>

            <Card size="small" style={{ marginBottom: 16 }}>
              <Progress
                percent={activeTask.total_ips > 0 ? Math.round((activeTask.scanned_ips / activeTask.total_ips) * 100) : 0}
                status={activeTask.status === 'running' ? 'active' : undefined}
              />
              <Space size="large" style={{ marginTop: 8 }}>
                <Text>
                  <Text type="secondary">{t.discovery.scanned}:</Text> {activeTask.scanned_ips}/{activeTask.total_ips}
                </Text>
                <Text type="success">
                  <CheckCircleOutlined /> {t.discovery.found}: {activeTask.found_nodes}
                </Text>
                <Text type="danger">
                  <CloseCircleOutlined /> {t.discovery.failed}: {activeTask.failed_ips}
                </Text>
              </Space>
            </Card>

            <Table
              columns={resultColumns}
              dataSource={activeResults}
              rowKey="id"
              size="small"
              pagination={{ pageSize: 10 }}
              locale={{
                emptyText: <Empty description={t.discovery.noResultsYet} />,
              }}
            />
          </div>
        ) : null}
      </Modal>
    </div>
  );
}
