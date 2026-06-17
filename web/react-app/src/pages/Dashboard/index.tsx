import { useEffect, useState } from 'react';
import { Alert, Button, Card, Col, Empty, List, Row, Space, Statistic, Table, Tag, Typography } from 'antd';
import {
  ArrowRightOutlined,
  BugOutlined,
  CheckCircleOutlined,
  CloseCircleOutlined,
  ExclamationCircleOutlined,
  InfoCircleOutlined,
  PlayCircleOutlined,
  WarningOutlined,
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';
import { useNavigate } from 'react-router-dom';
import dayjs from 'dayjs';
import { activityLogsAPI, type ActivityLog, type LogStatistics } from '@/api/activityLogs';
import { nodesApi } from '@/api/nodes';
import { useStore } from '@/store';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAutoRefresh } from '@/hooks/useAutoRefresh';
import { Node } from '@/types';

const { Text } = Typography;

export default function Dashboard() {
  const navigate = useNavigate();
  const { nodes, setNodes, systemStats, setSystemStats, t } = useStore();
  const [nodesLoading, setNodesLoading] = useState(false);
  const [activityLoading, setActivityLoading] = useState(false);
  const [recentLogs, setRecentLogs] = useState<ActivityLog[]>([]);
  const [logStatistics, setLogStatistics] = useState<LogStatistics | null>(null);
  const [activityError, setActivityError] = useState<string | null>(null);

  async function loadNodes() {
    setNodesLoading(true);
    try {
      const response = await nodesApi.getNodes();
      setNodes(response.nodes || []);
    } catch (error) {
      console.error('Failed to load nodes:', error);
    } finally {
      setNodesLoading(false);
    }
  }

  async function loadActivityOverview() {
    setActivityLoading(true);
    setActivityError(null);

    try {
      const [logs, stats] = await Promise.all([
        activityLogsAPI.getRecentLogs(8),
        activityLogsAPI.getLogStatistics(7),
      ]);
      setRecentLogs(logs);
      setLogStatistics(stats);
    } catch (error) {
      console.error('Failed to load dashboard activity overview:', error);
      setActivityError(t.dashboard.activityUnavailable);
    } finally {
      setActivityLoading(false);
    }
  }

  useEffect(() => {
    void loadNodes();
    void loadActivityOverview();
  }, []);

  // WebSocket real-time updates
  useWebSocket({
    onMessage: (message) => {
      if (message.type === 'nodes_update') {
        setNodes(message.data);
      } else if (message.type === 'system_stats') {
        setSystemStats(message.data);
      }
    },
  });

  useAutoRefresh(async () => {
    await Promise.all([loadNodes(), loadActivityOverview()]);
  });

  const totalNodes = nodes.length;
  const onlineNodes = nodes.filter(n => n.is_connected).length;
  const offlineNodes = totalNodes - onlineNodes;
  const totalProcesses = systemStats?.running_processes || 0;
  const recentAlerts = recentLogs.filter((log) => ['WARNING', 'ERROR'].includes(log.level.toUpperCase()));

  const getLevelColor = (level: string) => {
    switch (level.toUpperCase()) {
      case 'ERROR':
        return 'red';
      case 'WARNING':
        return 'orange';
      case 'INFO':
        return 'blue';
      case 'DEBUG':
        return 'default';
      default:
        return 'default';
    }
  };

  const formatActionLabel = (action: string) => action.replace(/_/g, ' ').toUpperCase();

  const columns: ColumnsType<Node> = [
    {
      title: t.nodes.nodeName,
      dataIndex: 'name',
      key: 'name',
      render: (text) => <a onClick={() => navigate(`/nodes/${text}`)}>{text}</a>,
    },
    {
      title: t.nodes.environment,
      dataIndex: 'environment',
      key: 'environment',
      render: (env) => <Tag color="blue">{env}</Tag>,
    },
    {
      title: t.nodes.nodeHost,
      key: 'host',
      render: (_, record) => `${record.host}:${record.port}`,
    },
    {
      title: t.common.status,
      dataIndex: 'is_connected',
      key: 'status',
      render: (connected) =>
        connected ? (
          <Tag icon={<CheckCircleOutlined />} color="success">
            {t.nodes.online}
          </Tag>
        ) : (
          <Tag icon={<CloseCircleOutlined />} color="error">
            {t.nodes.offline}
          </Tag>
        ),
    },
    {
      title: t.nodes.processes,
      dataIndex: 'process_count',
      key: 'process_count',
      render: (count) => count || 0,
    },
    {
      title: t.common.actions,
      key: 'action',
      render: (_, record) => (
        <Button
          type="link"
          size="small"
          onClick={() => navigate(`/nodes/${record.name}`)}
        >
          {t.nodes.viewDetails}
        </Button>
      ),
    },
  ];

  return (
    <div>
      {/* Statistics Cards */}
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title={t.dashboard.totalNodes}
              value={totalNodes}
              prefix={null}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title={t.dashboard.onlineNodes}
              value={onlineNodes}
              prefix={<CheckCircleOutlined />}
              valueStyle={{ color: '#3f8600' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title={t.dashboard.offlineNodes}
              value={offlineNodes}
              prefix={<CloseCircleOutlined />}
              valueStyle={{ color: offlineNodes > 0 ? '#cf1322' : '#999' }}
            />
          </Card>
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <Card>
            <Statistic
              title={t.dashboard.runningProcesses}
              value={totalProcesses}
              prefix={<PlayCircleOutlined />}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
      </Row>

      {/* Nodes Table */}
      <Card
        title={t.dashboard.nodeStatus}
        extra={
          <Button type="primary" onClick={() => {
            void loadNodes();
            void loadActivityOverview();
          }} loading={nodesLoading || activityLoading}>
            {t.common.refresh}
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={nodes}
          rowKey="name"
          loading={nodesLoading}
          pagination={{ pageSize: 10 }}
        />
      </Card>

      <Row gutter={16} style={{ marginTop: 24 }}>
        <Col xs={24} xl={12}>
          <Card
            title={t.dashboard.activityOverview}
            loading={activityLoading}
            extra={
              <Button type="link" onClick={() => navigate('/logs')}>
                {t.dashboard.viewAllLogs}
              </Button>
            }
          >
            {activityError ? (
              <Alert type="warning" showIcon message={activityError} />
            ) : (
              <Space direction="vertical" size="large" style={{ width: '100%' }}>
                <Row gutter={[16, 16]}>
                  <Col xs={12} md={6}>
                    <Statistic
                      title={t.dashboard.infoLogs}
                      value={logStatistics?.info_count || 0}
                      prefix={<InfoCircleOutlined />}
                    />
                  </Col>
                  <Col xs={12} md={6}>
                    <Statistic
                      title={t.dashboard.warningLogs}
                      value={logStatistics?.warning_count || 0}
                      prefix={<WarningOutlined />}
                      valueStyle={{ color: '#d48806' }}
                    />
                  </Col>
                  <Col xs={12} md={6}>
                    <Statistic
                      title={t.dashboard.errorLogs}
                      value={logStatistics?.error_count || 0}
                      prefix={<ExclamationCircleOutlined />}
                      valueStyle={{ color: '#cf1322' }}
                    />
                  </Col>
                  <Col xs={12} md={6}>
                    <Statistic
                      title={t.dashboard.debugLogs}
                      value={logStatistics?.debug_count || 0}
                      prefix={<BugOutlined />}
                    />
                  </Col>
                </Row>

                <Row gutter={16}>
                  <Col xs={24} md={12}>
                    <Text strong>{t.dashboard.topActions}</Text>
                    <List
                      size="small"
                      locale={{ emptyText: t.common.noData }}
                      dataSource={logStatistics?.top_actions || []}
                      renderItem={(item) => (
                        <List.Item>
                          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                            <Tag>{formatActionLabel(item.action)}</Tag>
                            <Text type="secondary">{item.count}</Text>
                          </Space>
                        </List.Item>
                      )}
                    />
                  </Col>
                  <Col xs={24} md={12}>
                    <Text strong>{t.dashboard.topUsers}</Text>
                    <List
                      size="small"
                      locale={{ emptyText: t.common.noData }}
                      dataSource={logStatistics?.top_users || []}
                      renderItem={(item) => (
                        <List.Item>
                          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                            <Text>{item.username || t.dashboard.systemActor}</Text>
                            <Text type="secondary">{item.count}</Text>
                          </Space>
                        </List.Item>
                      )}
                    />
                  </Col>
                </Row>
              </Space>
            )}
          </Card>
        </Col>

        <Col xs={24} xl={12}>
          <Card
            title={t.dashboard.recentAlerts}
            loading={activityLoading}
            extra={
              <Button type="link" icon={<ArrowRightOutlined />} onClick={() => navigate('/logs')}>
                {t.common.view}
              </Button>
            }
          >
            {activityError ? (
              <Alert type="warning" showIcon message={activityError} />
            ) : recentAlerts.length === 0 ? (
              <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={t.dashboard.noRecentAlerts} />
            ) : (
              <List
                itemLayout="vertical"
                dataSource={recentAlerts}
                renderItem={(log) => (
                  <List.Item
                    key={log.id}
                    style={{ cursor: 'pointer' }}
                    onClick={() => navigate('/logs')}
                  >
                    <Space direction="vertical" size={4} style={{ width: '100%' }}>
                      <Space wrap>
                        <Tag color={getLevelColor(log.level)}>{log.level.toUpperCase()}</Tag>
                        <Tag>{formatActionLabel(log.action)}</Tag>
                        <Text type="secondary">{dayjs(log.created_at).format('YYYY-MM-DD HH:mm:ss')}</Text>
                      </Space>
                      <Text strong>{log.message}</Text>
                      <Text type="secondary">
                        {(log.username || t.dashboard.systemActor)} · {log.resource || '-'} · {log.target || '-'}
                      </Text>
                    </Space>
                  </List.Item>
                )}
              />
            )}
          </Card>
        </Col>
      </Row>
    </div>
  );
}
