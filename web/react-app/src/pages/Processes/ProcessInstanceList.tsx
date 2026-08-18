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
} from 'lucide-react';
import { ProcessInstance } from '@/types';
import { nodesApi } from '@/api/nodes';
import LogViewer from '@/components/LogViewer';
import { useStore } from '@/store';

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
    </>
  );
};

export default ProcessInstanceList;
