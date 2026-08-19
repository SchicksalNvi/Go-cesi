import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Table,
  Tag,
  Button,
  Space,
  Descriptions,
  Modal,
  Tabs,
  message,
  Popconfirm,
  Spin,
  Row,
  Col,
  Input,
  Statistic,
} from 'antd';
import {
  ArrowLeft,
  PlayCircle,
  Square,
  RefreshCw,
  FileText,
  Settings2,
  Info,
  CheckCircle2,
  XCircle,
  Pencil,
  Save,
} from 'lucide-react';
import { nodesApi, ProcessConfigInfo } from '@/api/nodes';
import { Node, Process } from '@/types';
import LogViewer from '@/components/LogViewer';

import { useStore } from '@/store';

const NodeDetail: React.FC = () => {
  const { nodeName } = useParams<{ nodeName: string }>();
  const navigate = useNavigate();
  const { t } = useStore();
  const [node, setNode] = useState<Node | null>(null);
  const [processes, setProcesses] = useState<Process[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<Record<string, boolean>>({});
  const [logViewerVisible, setLogViewerVisible] = useState(false);
  const [selectedProcess, setSelectedProcess] = useState<Process | null>(null);
  const [configVisible, setConfigVisible] = useState(false);
  const [configLoading, setConfigLoading] = useState(false);
  const [configData, setConfigData] = useState<ProcessConfigInfo | null>(null);
  const [selectedRowKeys, setSelectedRowKeys] = useState<React.Key[]>([]);
  const [batchRestartLoading, setBatchRestartLoading] = useState(false);
  const [batchStopLoading, setBatchStopLoading] = useState(false);
  const [pageSize, setPageSize] = useState(10);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState('');
  const [editEnvironment, setEditEnvironment] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (nodeName) {
      loadNodeDetail();
    }
  }, [nodeName]);

  const loadNodeDetail = async () => {
    if (!nodeName) return;
    
    setLoading(true);
    try {
      // 加载节点信息
      const nodesResponse = await nodesApi.getNodes();
      // 后端直接返回 { status, nodes }，不是嵌套在 data 里
      const foundNode = (nodesResponse as any).nodes?.find((n: Node) => n.name === nodeName);
      
      if (foundNode) {
        setNode(foundNode);
        
        // 加载进程列表
        if (foundNode.is_connected) {
          const processResponse = await nodesApi.getNodeProcesses(nodeName);
          // 后端直接返回 { status, processes }
          setProcesses((processResponse as any).processes || []);
        }
      } else {
        message.error('Node not found');
        navigate('/nodes');
      }
    } catch (error) {
      console.error('Failed to load node detail:', error);
      message.error('Failed to load node detail');
    } finally {
      setLoading(false);
    }
  };

  const handleProcessAction = async (
    processName: string,
    action: 'start' | 'stop' | 'restart'
  ) => {
    if (!nodeName) return;
    
    const actionKey = `${processName}-${action}`;
    setActionLoading(prev => ({ ...prev, [actionKey]: true }));
    
    try {
      switch (action) {
        case 'start':
          await nodesApi.startProcess(nodeName, processName);
          message.success(`Started ${processName}`);
          break;
        case 'stop':
          await nodesApi.stopProcess(nodeName, processName);
          message.success(`Stopped ${processName}`);
          break;
        case 'restart':
          await nodesApi.restartProcess(nodeName, processName);
          message.success(`Restarted ${processName}`);
          break;
      }
      // 重新加载进程列表
      await loadNodeDetail();
    } catch (error) {
      console.error(`Failed to ${action} process:`, error);
      message.error(`Failed to ${action} process`);
    } finally {
      setActionLoading(prev => ({ ...prev, [actionKey]: false }));
    }
  };

  // 批量执行勾选进程的动作 (restart / stop)
  const handleBatchAction = async (action: 'restart' | 'stop') => {
    if (selectedRowKeys.length === 0) {
      message.warning(t.processInstance.selectAtLeastOne || '请至少选择一个进程');
      return;
    }
    if (action === 'restart') setBatchRestartLoading(true);
    else setBatchStopLoading(true);
    let ok = 0;
    let fail = 0;
    const nn = nodeName || '';
    for (const key of selectedRowKeys as string[]) {
      try {
        if (action === 'restart') {
          await nodesApi.restartProcess(nn, key);
        } else {
          await nodesApi.stopProcess(nn, key);
        }
        ok++;
      } catch (error: any) {
        fail++;
        console.error(`Failed to ${action} ${key}:`, error);
      }
    }
    if (action === 'restart') setBatchRestartLoading(false);
    else setBatchStopLoading(false);
    if (ok > 0)
      message.success(
        action === 'restart'
          ? (t.processInstance.batchRestartDone || `已重启 ${ok} 个进程`)
          : (t.processInstance.batchStopDone || `已停止 ${ok} 个进程`)
      );
    if (fail > 0)
      message.error(
        action === 'restart'
          ? (t.processInstance.batchRestartFail || `${fail} 个进程重启失败`)
          : (t.processInstance.batchStopFail || `${fail} 个进程停止失败`)
      );
    setSelectedRowKeys([]);
    await loadNodeDetail();
  };

  const handleViewLogs = (process: Process) => {
    setSelectedProcess(process);
    setLogViewerVisible(true);
  };

  // 查看进程配置:拉取节点所有配置,按 name/process_name/group:name 匹配当前进程
  const handleViewConfig = async (process: Process) => {
    const nodeNameParam = nodeName || '';
    setSelectedProcess(process);
    setConfigVisible(true);
    setConfigLoading(true);
    setConfigData(null);
    try {
      const res = await nodesApi.getAllConfigInfo(nodeNameParam);
      const configs = res.data || [];
      const found = configs.find(
        (c) => c.name === process.name || c.process_name === process.name || c.name === `${process.group}:${process.name}`
      ) || null;
      if (!found) {
        message.info(t.processInstance.configNotFound || '未找到该进程的配置信息');
      }
      setConfigData(found);
    } catch (error: any) {
      console.error('Failed to load process config:', error);
      message.error(error.response?.data?.message || '加载进程配置失败');
    } finally {
      setConfigLoading(false);
    }
  };

  const handleEditStart = () => {
    if (node) {
      setEditName(node.name);
      setEditEnvironment(node.environment || '');
      setEditing(true);
    }
  };

  const handleEditSave = async () => {
    if (!nodeName || !editName.trim()) return;
    setSaving(true);
    try {
      await nodesApi.updateNode(nodeName, {
        name: editName.trim(),
        environment: editEnvironment.trim(),
      });
      message.success('Node updated');
      setEditing(false);
      // 如果名称变了，跳转到新名称的页面
      if (editName.trim() !== nodeName) {
        navigate(`/nodes/${editName.trim()}`);
      } else {
        loadNodeDetail();
      }
    } catch (error) {
      console.error('Failed to update node:', error);
      message.error('Failed to update node');
    } finally {
      setSaving(false);
    }
  };

  const getProcessStateColor = (state: number) => {
    switch (state) {
      case 20: return 'success'; // RUNNING
      case 0: return 'default'; // STOPPED
      case 10: return 'processing'; // STARTING
      case 30: return 'warning'; // BACKOFF
      case 100: return 'error'; // FATAL
      default: return 'default';
    }
  };

  const processColumns = [
    {
      title: t.common.name,
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => <strong>{name}</strong>,
    },
    {
      title: t.processes.processGroup,
      dataIndex: 'group',
      key: 'group',
    },
    {
      title: t.processes.processState,
      dataIndex: 'state_string',
      key: 'state_string',
      render: (state: string, record: Process) => (
        <Tag color={getProcessStateColor(record.state)}>
          {state}
        </Tag>
      ),
    },
    {
      title: t.processes.pid,
      dataIndex: 'pid',
      key: 'pid',
      render: (pid: number) => pid || '-',
    },
    {
      title: t.processes.uptime,
      dataIndex: 'uptime_human',
      key: 'uptime_human',
      render: (uptime: string) => uptime || '-',
    },
    {
      title: t.common.actions,
      key: 'actions',
      render: (_: any, record: Process) => (
        <Space>
          {record.state !== 20 && (
            <Button
              type="primary"
              size="small"
              icon={<PlayCircle size={14} strokeWidth={1.7} />}
              onClick={() => handleProcessAction(record.name, 'start')}
              loading={actionLoading[`${record.name}-start`]}
            >
              {t.processes.start}
            </Button>
          )}
          {record.state === 20 && (
            <Popconfirm
              title={t.nodeDetail.stopProcess}
              onConfirm={() => handleProcessAction(record.name, 'stop')}
              okText={t.common.yes}
              cancelText={t.common.no}
            >
              <Button
                size="small"
                icon={<Square size={14} strokeWidth={1.7} />}
                loading={actionLoading[`${record.name}-stop`]}
                danger
              >
                {t.processes.stop}
              </Button>
            </Popconfirm>
          )}
          <Button
            size="small"
            icon={<RefreshCw size={14} strokeWidth={1.7} />}
            onClick={() => handleProcessAction(record.name, 'restart')}
            loading={actionLoading[`${record.name}-restart`]}
          >
            {t.processes.restart}
          </Button>
          <Button
            size="small"
            icon={<FileText size={14} strokeWidth={1.7} />}
            onClick={() => handleViewLogs(record)}
          >
            {t.logs.title}
          </Button>
          <Button
            size="small"
            icon={<Settings2 size={14} strokeWidth={1.7} />}
            onClick={() => handleViewConfig(record)}
          >
            {t.processInstance.viewConfig}
          </Button>
        </Space>
      ),
    },
  ];

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '50px' }}>
        <Spin size="large" />
      </div>
    );
  }

  if (!node) {
    return <div>Node not found</div>;
  }

  const runningProcesses = processes.filter(p => p.state === 20).length;
  const stoppedProcesses = processes.filter(p => p.state === 0).length;
  const totalProcesses = processes.length;

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space>
          <Button icon={<ArrowLeft size={14} strokeWidth={1.7} />} onClick={() => navigate('/nodes')}>
            {t.common.back}
          </Button>
          <h1 style={{ margin: 0 }}>{t.nav.nodes}: {node.name}</h1>
          {node.is_connected ? (
            <Tag icon={<CheckCircle2 size={14} strokeWidth={1.7} />} color="success">
              {t.nodeDetail.connected}
            </Tag>
          ) : (
            <Tag icon={<XCircle size={14} strokeWidth={1.7} />} color="error">
              {t.nodeDetail.disconnected}
            </Tag>
          )}
        </Space>
        <Button
          type="primary"
          icon={<RefreshCw size={14} strokeWidth={1.7} />}
          onClick={loadNodeDetail}
          loading={loading}
        >
          {t.common.refresh}
        </Button>
      </div>

      {/* 统计卡片 */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={8}>
          <Card>
            <Statistic
              title={t.nodeDetail.totalProcesses}
              value={totalProcesses}
              prefix={<Info size={14} strokeWidth={1.7} />}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card>
            <Statistic
              title={t.nodeDetail.running}
              value={runningProcesses}
              prefix={<PlayCircle size={14} strokeWidth={1.7} />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={8}>
          <Card>
            <Statistic
              title={t.nodeDetail.stopped}
              value={stoppedProcesses}
              prefix={<Square size={14} strokeWidth={1.7} />}
              valueStyle={{ color: '#ff4d4f' }}
            />
          </Card>
        </Col>
      </Row>

      <Tabs
        defaultActiveKey="processes"
        items={[
          {
            key: 'processes',
            label: t.processes.title,
            children: (
              <Card
                extra={
                  <Space>
                    <Button
                      size="small"
                      icon={<RefreshCw size={14} strokeWidth={1.7} />}
                      onClick={() => handleBatchAction('restart')}
                      loading={batchRestartLoading}
                      disabled={selectedRowKeys.length === 0}
                      type="primary"
                    >
                      {t.processInstance.batchRestart || '批量重启'}({selectedRowKeys.length})
                    </Button>
                    <Popconfirm
                      title={t.nodeDetail.stopProcess}
                      onConfirm={() => handleBatchAction('stop')}
                      okText={t.common.yes}
                      cancelText={t.common.no}
                      disabled={selectedRowKeys.length === 0}
                    >
                      <Button
                        size="small"
                        icon={<Square size={14} strokeWidth={1.7} />}
                        loading={batchStopLoading}
                        disabled={selectedRowKeys.length === 0}
                        danger
                      >
                        {t.processInstance.batchStop || '批量停止'}({selectedRowKeys.length})
                      </Button>
                    </Popconfirm>
                  </Space>
                }
              >
                <Table
                  columns={processColumns}
                  dataSource={processes}
                  rowKey="name"
                  rowSelection={{
                    selectedRowKeys,
                    onChange: (keys) => setSelectedRowKeys(keys),
                  }}
                  pagination={{
                    pageSize: pageSize,
                    showSizeChanger: true,
                    pageSizeOptions: ['10', '20', '50', '100'],
                    onShowSizeChange: (_, size) => setPageSize(size),
                    showTotal: (total) => `${t.common.total} ${total} ${t.processes.title}`,
                  }}
                  scroll={{ x: 800 }}
                />
              </Card>
            ),
          },
          {
            key: 'info',
            label: t.nodeDetail.nodeInfo,
            children: (
              <Card extra={
                editing ? (
                  <Space>
                    <Button onClick={() => setEditing(false)}>
                      {t.common.cancel}
                    </Button>
                    <Button type="primary" icon={<Save size={14} strokeWidth={1.7} />} onClick={handleEditSave} loading={saving}>
                      {t.common.save}
                    </Button>
                  </Space>
                ) : (
                  <Button icon={<Pencil size={14} strokeWidth={1.7} />} onClick={handleEditStart}>
                    {t.common.edit}
                  </Button>
                )
              }>
                <Descriptions bordered column={2}>
                  <Descriptions.Item label={t.common.name}>
                    {editing ? (
                      <Input value={editName} onChange={e => setEditName(e.target.value)} />
                    ) : node.name}
                  </Descriptions.Item>
                  <Descriptions.Item label={t.nodeDetail.host}>{node.host}</Descriptions.Item>
                  <Descriptions.Item label={t.nodeDetail.port}>{node.port}</Descriptions.Item>
                  <Descriptions.Item label={t.nodes.environment}>
                    {editing ? (
                      <Input value={editEnvironment} onChange={e => setEditEnvironment(e.target.value)} placeholder="e.g. production, staging" />
                    ) : (
                      <Tag color={node.environment === 'production' ? 'red' : 'blue'}>
                        {node.environment || 'default'}
                      </Tag>
                    )}
                  </Descriptions.Item>
                  <Descriptions.Item label={t.nodeDetail.username}>{node.username}</Descriptions.Item>
                  <Descriptions.Item label={t.common.status}>
                    <Tag color={node.is_connected ? 'success' : 'error'}>
                      {node.is_connected ? t.nodeDetail.connected : t.nodeDetail.disconnected}
                    </Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label={t.nodeDetail.lastPing} span={2}>
                    {node.last_ping ? new Date(node.last_ping).toLocaleString() : '-'}
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            ),
          },
        ]}
      />

      {/* Enhanced Log Viewer */}
      {selectedProcess && (
        <LogViewer
          visible={logViewerVisible}
          onClose={() => {
            setLogViewerVisible(false);
            setSelectedProcess(null);
          }}
          nodeName={nodeName || ''}
          processName={selectedProcess.name}
        />
      )}

      {/* Process Config Modal */}
      <Modal
        open={configVisible}
        onCancel={() => {
          setConfigVisible(false);
          setSelectedProcess(null);
        }}
        title={`${selectedProcess?.name || ''} - ${t.processInstance.node}: ${nodeName || ''}`}
        width={640}
        footer={null}
      >
        {configLoading ? (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <Spin />
          </div>
        ) : configData ? (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="Name">{configData.name}</Descriptions.Item>
            <Descriptions.Item label="Group">{configData.group}</Descriptions.Item>
            <Descriptions.Item label="Command">
              <code style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{configData.command}</code>
            </Descriptions.Item>
            <Descriptions.Item label="Directory">{configData.directory || '-'}</Descriptions.Item>
            <Descriptions.Item label="stdout_logfile"><code style={{ fontFamily: 'var(--font-mono)' }}>{configData.stdout_logfile || '-'}</code></Descriptions.Item>
            <Descriptions.Item label="stderr_logfile"><code style={{ fontFamily: 'var(--font-mono)' }}>{configData.stderr_logfile || '-'}</code></Descriptions.Item>
            <Descriptions.Item label="Autostart">{configData.autostart ? 'Yes' : 'No'}</Descriptions.Item>
            <Descriptions.Item label="Autorestart">{configData.autorestart ? 'Yes' : 'No'}</Descriptions.Item>
            <Descriptions.Item label="Priority">{String(configData.priority ?? '-')}</Descriptions.Item>
            <Descriptions.Item label="Startsecs">{String(configData.startsecs ?? '-')}</Descriptions.Item>
            <Descriptions.Item label="Startretries">{String(configData.startretries ?? '-')}</Descriptions.Item>
            <Descriptions.Item label="Stopsignal">{configData.stopsignal || '-'}</Descriptions.Item>
            <Descriptions.Item label="Stopwaitsecs">{String(configData.stopwaitsecs ?? '-')}</Descriptions.Item>
            <Descriptions.Item label="Kill as group">{configData.killasgroup ? 'Yes' : 'No'}</Descriptions.Item>
            <Descriptions.Item label="Exitcodes">{configData.exitcodes || '-'}</Descriptions.Item>
            <Descriptions.Item label="Environment">
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: 11, whiteSpace: 'pre-wrap' }}>
                {configData.environment || '-'}
              </span>
            </Descriptions.Item>
          </Descriptions>
        ) : (
          <div style={{ textAlign: 'center', padding: 40, color: 'var(--text-low)' }}>
            {t.processInstance.configNotFound || '未找到该进程的配置信息'}
          </div>
        )}
      </Modal>
    </div>
  );
};

export default NodeDetail;
