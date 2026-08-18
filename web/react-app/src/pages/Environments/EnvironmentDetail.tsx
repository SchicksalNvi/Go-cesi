import { useEffect, useState, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
  Card,
  Table,
  Button,
  Space,
  Spin,
  Tag,
  message,
  Result,
  Radio,
} from 'antd';
import {
  ArrowLeft,
  RefreshCw,
  CheckCircle2,
  XCircle,
} from 'lucide-react';
import type { RadioChangeEvent } from 'antd';
import { environmentsApi } from '@/api/environments';
import { useWebSocket } from '@/hooks/useWebSocket';
import { EnvironmentDetail as EnvironmentDetailType, NodeDetail } from '@/types';
import type { ColumnsType } from 'antd/es/table';
import { useStore } from '@/store';

type StatusFilter = 'all' | 'online' | 'offline';

export default function EnvironmentDetail() {
  const { environmentName } = useParams<{ environmentName: string }>();
  const navigate = useNavigate();
  const { t } = useStore();
  const [environment, setEnvironment] = useState<EnvironmentDetailType | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');

  useEffect(() => {
    if (environmentName) {
      loadEnvironmentDetail();
    }
  }, [environmentName]);

  const loadEnvironmentDetail = async () => {
    if (!environmentName) return;

    setLoading(true);
    setError(null);
    try {
      const response = await environmentsApi.getEnvironmentDetail(environmentName);
      if (response.environment) {
        setEnvironment(response.environment);
      } else {
        setError('Environment not found');
      }
    } catch (err: any) {
      console.error('Failed to load environment detail:', err);
      if (err.response?.status === 404) {
        setError('Environment not found');
      } else {
        message.error('Failed to load environment details. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  };

  // WebSocket real-time updates
  useWebSocket({
    onMessage: (msg) => {
      if (msg.type === 'nodes_update' || msg.type === 'node_status_change') {
        loadEnvironmentDetail();
      }
    },
  });

  // Filter nodes based on status
  const filteredNodes = useMemo(() => {
    if (!environment) return [];
    
    switch (statusFilter) {
      case 'online':
        return environment.members.filter((node) => node.is_connected);
      case 'offline':
        return environment.members.filter((node) => !node.is_connected);
      default:
        return environment.members;
    }
  }, [environment, statusFilter]);

  const handleStatusFilterChange = (e: RadioChangeEvent) => {
    setStatusFilter(e.target.value);
  };

  const columns: ColumnsType<NodeDetail> = [
    {
      title: t.nodes.nodeName,
      dataIndex: 'name',
      key: 'name',
      render: (name: string) => (
        <a
          onClick={(e) => {
            e.preventDefault();
            navigate(`/nodes/${name}`);
          }}
          style={{ fontWeight: 500 }}
        >
          {name}
        </a>
      ),
    },
    {
      title: t.nodeDetail.host,
      dataIndex: 'host',
      key: 'host',
    },
    {
      title: t.nodeDetail.port,
      dataIndex: 'port',
      key: 'port',
    },
    {
      title: t.common.status,
      dataIndex: 'is_connected',
      key: 'status',
      render: (isConnected: boolean) => (
        <Tag
          icon={isConnected ? <CheckCircle2 size={14} strokeWidth={1.7} /> : <XCircle size={14} strokeWidth={1.7} />}
          color={isConnected ? 'success' : 'error'}
        >
          {isConnected ? t.nodes.online : t.nodes.offline}
        </Tag>
      ),
    },
    {
      title: t.nodes.processes,
      dataIndex: 'processes',
      key: 'processes',
      render: (count: number) => count || 0,
    },
    {
      title: t.nodeDetail.lastPing,
      dataIndex: 'last_ping',
      key: 'last_ping',
      render: (lastPing: string) => {
        if (!lastPing) return '-';
        const date = new Date(lastPing);
        return date.toLocaleString();
      },
    },
  ];

  if (loading && !environment) {
    return (
      <div style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    );
  }

  if (error) {
    return (
      <Result
        status="404"
        title={t.environmentDetail.envNotFound}
        subTitle={t.environmentDetail.envNotExist.replace('{name}', environmentName || '')}
        extra={
          <Button type="primary" onClick={() => navigate('/environments')}>
            {t.environmentDetail.backToEnv}
          </Button>
        }
      />
    );
  }

  if (!environment) {
    return null;
  }

  return (
    <div>
      <div
        style={{
          marginBottom: 24,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
        }}
      >
        <div>
          <Space>
            <Button
              icon={<ArrowLeft size={14} strokeWidth={1.7} />}
              onClick={() => navigate('/environments')}
            >
              {t.common.back}
            </Button>
            <h2 style={{ margin: 0, fontSize: 24 }}>
              {t.nav.environments}: {environment.name}
            </h2>
          </Space>
          <p style={{ color: '#666', marginTop: 8, marginLeft: 40 }}>
            {t.environmentDetail.nodesInEnv.replace('{count}', String(environment.members.length))}
          </p>
        </div>
        <Button
          type="primary"
          icon={<RefreshCw size={14} strokeWidth={1.7} />}
          onClick={loadEnvironmentDetail}
          loading={loading}
        >
          {t.common.refresh}
        </Button>
      </div>

      <Card>
        {environment.members.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 40 }}>
            <p style={{ color: '#999', fontSize: 16 }}>
              {t.environmentDetail.noNodesFound}
            </p>
          </div>
        ) : (
          <>
            <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Radio.Group value={statusFilter} onChange={handleStatusFilterChange}>
                <Radio.Button value="all">
                  {t.common.all} ({environment.members.length})
                </Radio.Button>
                <Radio.Button value="online">
                  <CheckCircle2 size={14} strokeWidth={1.7} style={{ color: '#52c41a' }} /> {t.nodes.online} ({environment.members.filter(n => n.is_connected).length})
                </Radio.Button>
                <Radio.Button value="offline">
                  <XCircle size={14} strokeWidth={1.7} style={{ color: '#ff4d4f' }} /> {t.nodes.offline} ({environment.members.filter(n => !n.is_connected).length})
                </Radio.Button>
              </Radio.Group>
            </div>
            <Table
              columns={columns}
              dataSource={filteredNodes}
              rowKey="name"
              pagination={false}
              loading={loading}
            />
          </>
        )}
      </Card>
    </div>
  );
}
