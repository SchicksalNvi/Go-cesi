import { useState } from 'react';
import {
  Table,
  Tag,
  Button,
  Space,
  Popconfirm,
  message,
} from 'antd';
import {
  PlayCircle,
  Square,
  RefreshCw,
  FileText,
  Settings2,
} from 'lucide-react';
import { ProcessInstance } from '@/types';
import { nodesApi, ProcessConfigInfo } from '@/api/nodes';
import LogViewer from '@/components/LogViewer';
import { useStore } from '@/store';
import { Modal, Descriptions, Spin } from 'antd';

interface ProcessInstanceListProps {
  instances: ProcessInstance[];
  processName: string;
  onRefresh: () => void;
}

const ProcessInstanceList: React.FC<ProcessInstanceListProps> = ({
  instances,
  processName,
  onRefresh,
}) => {
  const { t } = useStore();
  const [actionLoading, setActionLoading] = useState<Record<string, boolean>>({});
  const [logViewerVisible, setLogViewerVisible] = useState(false);
  const [selectedInstance, setSelectedInstance] = useState<ProcessInstance | null>(null);
  const [configVisible, setConfigVisible] = useState(false);
  const [configLoading, setConfigLoading] = useState(false);
  const [configData, setConfigData] = useState<ProcessConfigInfo | null>(null);

  const getProcessStateColor = (state: number) => {
    switch (state) {
      case 20:
        return 'success'; // RUNNING
      case 0:
        return 'default'; // STOPPED
      case 10:
        return 'processing'; // STARTING
      case 30:
        return 'warning'; // BACKOFF
      case 100:
        return 'error'; // FATAL
      default:
        return 'default';
    }
  };

  const handleInstanceAction = async (
    instance: ProcessInstance,
    action: 'start' | 'stop' | 'restart'
  ) => {
    const actionKey = `${instance.node_name}-${processName}-${action}`;
    setActionLoading((prev) => ({ ...prev, [actionKey]: true }));

    try {
      switch (action) {
        case 'start':
          await nodesApi.startProcess(instance.node_name, processName);
          message.success(`Started ${processName} on ${instance.node_name}`);
          break;
        case 'stop':
          await nodesApi.stopProcess(instance.node_name, processName);
          message.success(`Stopped ${processName} on ${instance.node_name}`);
          break;
        case 'restart':
          await nodesApi.restartProcess(instance.node_name, processName);
          message.success(`Restarted ${processName} on ${instance.node_name}`);
          break;
      }
      onRefresh();
    } catch (error) {
      console.error(`Failed to ${action} process:`, error);
      message.error(`Failed to ${action} process on ${instance.node_name}`);
    } finally {
      setActionLoading((prev) => ({ ...prev, [actionKey]: false }));
    }
  };

  const handleViewLogs = (instance: ProcessInstance) => {
    setSelectedInstance(instance);
    setLogViewerVisible(true);
  };

  // 查看进程配置:拉取节点所有配置,按 name/process_name 匹配当前进程
  const handleViewConfig = async (instance: ProcessInstance) => {
    setSelectedInstance(instance);
    setConfigVisible(true);
    setConfigLoading(true);
    setConfigData(null);
    try {
      const res = await nodesApi.getAllConfigInfo(instance.node_name);
      const configs = res.data || [];
      const found = configs.find(
        (c) => c.name === processName || c.process_name === processName || c.name === `${instance.group}:${processName}`
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

  const columns = [
    {
      title: t.processInstance.node,
      dataIndex: 'node_name',
      key: 'node_name',
      render: (name: string, record: ProcessInstance) => (
        <div>
          <strong>{name}</strong>
          <br />
          <small style={{ color: '#888' }}>
            {record.node_host}:{record.node_port}
          </small>
        </div>
      ),
    },
    {
      title: t.processInstance.state,
      dataIndex: 'state_string',
      key: 'state_string',
      render: (state: string, record: ProcessInstance) => (
        <Tag color={getProcessStateColor(record.state)}>{state}</Tag>
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
      title: t.processInstance.group,
      dataIndex: 'group',
      key: 'group',
    },
    {
      title: t.common.actions,
      key: 'actions',
      render: (_: any, record: ProcessInstance) => (
        <Space>
          {record.state !== 20 && (
            <Button
              type="primary"
              size="small"
              icon={<PlayCircle size={14} strokeWidth={1.7} />}
              onClick={() => handleInstanceAction(record, 'start')}
              loading={actionLoading[`${record.node_name}-${processName}-start`]}
            >
              {t.processes.start}
            </Button>
          )}
          {record.state === 20 && (
            <Popconfirm
              title={t.nodeDetail.stopProcess}
              onConfirm={() => handleInstanceAction(record, 'stop')}
              okText={t.common.yes}
              cancelText={t.common.no}
            >
              <Button
                size="small"
                icon={<Square size={14} strokeWidth={1.7} />}
                loading={actionLoading[`${record.node_name}-${processName}-stop`]}
                danger
              >
                {t.processes.stop}
              </Button>
            </Popconfirm>
          )}
          <Button
            size="small"
            icon={<RefreshCw size={14} strokeWidth={1.7} />}
            onClick={() => handleInstanceAction(record, 'restart')}
            loading={actionLoading[`${record.node_name}-${processName}-restart`]}
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
            loading={configLoading}
          >
            {t.processInstance.viewConfig}
          </Button>
        </Space>
      ),
    },
  ];

  return (
    <>
      <Table
        columns={columns}
        dataSource={instances}
        rowKey={(record) => `${record.node_name}-${processName}`}
        pagination={false}
        size="small"
      />

      {selectedInstance && (
        <LogViewer
          visible={logViewerVisible}
          onClose={() => {
            setLogViewerVisible(false);
            setSelectedInstance(null);
          }}
          nodeName={selectedInstance.node_name}
          processName={processName}
        />
      )}

      <Modal
        open={configVisible}
        onCancel={() => {
          setConfigVisible(false);
          setSelectedInstance(null);
        }}
        title={`${processName} - ${t.processInstance.node}: ${selectedInstance?.node_name || ''}`}
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
            <Descriptions.Item label="Autorestart">
              {configData.autorestart ? 'Yes' : 'No'}
            </Descriptions.Item>
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
    </>
  );
};

export default ProcessInstanceList;
