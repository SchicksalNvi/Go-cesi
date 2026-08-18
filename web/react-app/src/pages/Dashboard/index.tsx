import { useEffect, useState } from 'react';
import { Button, Card, Col, Empty, List, Row, Space, Table, Tag, Typography, Progress } from 'antd';
import {
  ArrowRight,
  Activity,
  CheckCircle2,
  XCircle,
  Play,
  Server,
  Zap,
  ShieldAlert,
  ScrollText,
  CircleAlert,
  Bug,
  Info,
} from 'lucide-react';
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

// Immersive stat card
function StatCard({
  icon: Icon,
  label,
  value,
  accent,
  delay,
}: {
  icon: typeof Server;
  label: string;
  value: number | string;
  accent: string;
  delay?: string;
}) {
  return (
    <div className="glass glass-interact sv-reveal" style={{ padding: '22px 24px', animationDelay: delay, height: '100%' }}>
      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
        <div>
          <div className="hero-kicker" style={{ fontSize: 10.5, marginBottom: 10 }}>{label.toUpperCase()}</div>
          <div className="display" style={{ fontSize: 34, lineHeight: 1 }}>
            {value}
          </div>
        </div>
        <div
          style={{
            width: 46,
            height: 46,
            borderRadius: 14,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background: `linear-gradient(135deg, ${accent}26, ${accent}0d)`,
            border: `1px solid ${accent}40`,
            color: accent,
          }}
        >
          <Icon size={21} strokeWidth={1.7} />
        </div>
      </div>
    </div>
  );
}

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
      case 'ERROR': return '#fb7185';
      case 'WARNING': return '#fbbf24';
      case 'INFO': return '#38bdf8';
      case 'DEBUG': return '#6b7694';
      default: return '#6b7694';
    }
  };

  const formatActionLabel = (action: string) => action.replace(/_/g, ' ').toUpperCase();

  const columns: ColumnsType<Node> = [
    {
      title: t.nodes.nodeName,
      dataIndex: 'name',
      key: 'name',
      render: (text) => (
        <a
          style={{ color: 'var(--text-hi)', fontWeight: 500, fontFamily: 'var(--font-mono)' }}
          onClick={() => navigate(`/nodes/${text}`)}
        >
          {text}
        </a>
      ),
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
      render: (_, record) => (
        <span className="mono" style={{ color: 'var(--text-mid)', fontSize: 12.5 }}>
          {record.host}:{record.port}
        </span>
      ),
    },
    {
      title: t.common.status,
      dataIndex: 'is_connected',
      key: 'status',
      render: (connected) =>
        connected ? (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--ok)', boxShadow: '0 0 12px var(--ok)', display: 'inline-block' }} />
            <span style={{ color: 'var(--ok)', fontSize: 12.5, fontFamily: 'var(--font-mono)' }}>{t.nodes.online}</span>
          </span>
        ) : (
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 7 }}>
            <span style={{ width: 8, height: 8, borderRadius: '50%', background: 'var(--err)', boxShadow: '0 0 12px var(--err)', display: 'inline-block' }} />
            <span style={{ color: 'var(--err)', fontSize: 12.5, fontFamily: 'var(--font-mono)' }}>{t.nodes.offline}</span>
          </span>
        ),
    },
    {
      title: t.nodes.processes,
      dataIndex: 'process_count',
      key: 'process_count',
      render: (count) => (
        <span className="mono" style={{ color: 'var(--text-mid)' }}>{count || 0}</span>
      ),
    },
    {
      title: t.common.actions,
      key: 'action',
      render: (_, record) => (
        <Button
          type="link"
          size="small"
          style={{ color: 'var(--acc-violet)' }}
          onClick={() => navigate(`/nodes/${record.name}`)}
        >
          {t.nodes.viewDetails}
        </Button>
      ),
    },
  ];

  return (
    <div style={{ maxWidth: 1440, margin: '0 auto' }}>
      {/* Page hero */}
      <div className="sv-reveal" style={{ marginBottom: 26 }}>
        <div className="hero-kicker" style={{ marginBottom: 8 }}>CONTROL PLANE / OVERVIEW</div>
        <div style={{ display: 'flex', alignItems: 'flex-end', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12 }}>
          <h1 className="display" style={{ fontSize: 32 }}>
            <span className="gradient-text">{t.dashboard.title}</span>
          </h1>
          <Button
            className="sv-btn sv-btn-primary"
            style={{ height: 40 }}
            onClick={() => {
              void loadNodes();
              void loadActivityOverview();
            }}
            loading={nodesLoading || activityLoading}
          >
            <Zap size={16} strokeWidth={2} />
            {t.common.refresh}
          </Button>
        </div>
      </div>

      {/* Statistics Cards */}
      <Row gutter={[18, 18]} style={{ marginBottom: 24 }}>
        <Col xs={24} sm={12} lg={6}>
          <StatCard icon={Server} label={t.dashboard.totalNodes} value={totalNodes} accent="#38bdf8" delay="0.05s" />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard icon={CheckCircle2} label={t.dashboard.onlineNodes} value={onlineNodes} accent="#34d399" delay="0.12s" />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard icon={XCircle} label={t.dashboard.offlineNodes} value={offlineNodes} accent={offlineNodes > 0 ? '#fb7185' : '#6b7694'} delay="0.19s" />
        </Col>
        <Col xs={24} sm={12} lg={6}>
          <StatCard icon={Play} label={t.dashboard.runningProcesses} value={totalProcesses} accent="#c084fc" delay="0.26s" />
        </Col>
      </Row>

      {/* Node status + health ring */}
      <Row gutter={[18, 18]} style={{ marginBottom: 24 }}>
        <Col xs={24} xl={18}>
          <div className="glass sv-reveal sv-reveal-d2" style={{ padding: 8, overflow: 'hidden' }}>
            <Table
              columns={columns}
              dataSource={nodes}
              rowKey="name"
              loading={nodesLoading}
              pagination={{ pageSize: 8, showSizeChanger: false }}
              size="middle"
            />
          </div>
        </Col>
        <Col xs={24} xl={6}>
          <div className="glass sv-reveal sv-reveal-d3" style={{ padding: '24px 22px', height: '100%', display: 'flex', flexDirection: 'column', gap: 22 }}>
            <div>
              <div className="hero-kicker" style={{ fontSize: 10.5, marginBottom: 12 }}>FLEET HEALTH</div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 18 }}>
                <Progress
                  type="circle"
                  percent={totalNodes ? Math.round((onlineNodes / totalNodes) * 100) : 0}
                  size={92}
                  strokeColor={{ '0%': '#22d3ee', '100%': '#c084fc' }}
                  trailColor="rgba(255,255,255,0.07)"
                />
                <div>
                  <div className="display" style={{ fontSize: 24 }}>
                    {totalNodes ? Math.round((onlineNodes / totalNodes) * 100) : 0}%
                  </div>
                  <div style={{ color: 'var(--text-low)', fontSize: 12 }}>{t.dashboard.onlineNodes}</div>
                </div>
              </div>
            </div>

            <div style={{ flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', gap: 12 }}>
              {[
                { label: 'RUNNING PROCESSES', value: totalProcesses, accent: '#c084fc' },
                { label: 'WARNING LOGS / 7D', value: logStatistics?.warning_count || 0, accent: '#fbbf24' },
                { label: 'ERROR LOGS / 7D', value: logStatistics?.error_count || 0, accent: '#fb7185' },
              ].map((row) => (
                <div key={row.label} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <span className="mono" style={{ fontSize: 10.5, letterSpacing: '0.08em', color: 'var(--text-low)' }}>
                    {row.label}
                  </span>
                  <span className="display" style={{ fontSize: 17, color: row.accent }}>{row.value}</span>
                </div>
              ))}
            </div>
          </div>
        </Col>
      </Row>

      {/* Activity + Alerts */}
      <Row gutter={[18, 18]}>
        <Col xs={24} xl={12}>
          <div className="glass sv-reveal sv-reveal-d3" style={{ padding: 20, height: '100%' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <div className="display" style={{ fontSize: 15 }}>{t.dashboard.activityOverview}</div>
              <Button type="link" style={{ color: 'var(--acc-violet)', padding: 0 }} onClick={() => navigate('/logs')}>
                {t.dashboard.viewAllLogs}
                <ArrowRight size={14} strokeWidth={1.8} />
              </Button>
            </div>

            {activityError ? (
              <div style={{ color: 'var(--warn)', fontSize: 13 }}>{activityError}</div>
            ) : (
              <Space direction="vertical" size="large" style={{ width: '100%' }}>
                <Row gutter={[10, 10]}>
                  {[
                    { icon: Info, label: t.dashboard.infoLogs, value: logStatistics?.info_count || 0, accent: '#38bdf8' },
                    { icon: ShieldAlert, label: t.dashboard.warningLogs, value: logStatistics?.warning_count || 0, accent: '#fbbf24' },
                    { icon: CircleAlert, label: t.dashboard.errorLogs, value: logStatistics?.error_count || 0, accent: '#fb7185' },
                    { icon: Bug, label: t.dashboard.debugLogs, value: logStatistics?.debug_count || 0, accent: '#a78bfa' },
                  ].map((row) => (
                    <Col xs={12} md={6} key={row.label}>
                      <div style={{
                        padding: '13px 14px',
                        borderRadius: 12,
                        background: 'var(--glass)',
                        border: `1px solid ${row.accent}22`,
                      }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 7 }}>
                          <row.icon size={13} strokeWidth={1.8} style={{ color: row.accent }} />
                          <span style={{ fontSize: 11, color: 'var(--text-low)' }}>{row.label}</span>
                        </div>
                        <span className="display" style={{ fontSize: 21, color: row.accent }}>{row.value}</span>
                      </div>
                    </Col>
                  ))}
                </Row>

                <Row gutter={16}>
                  <Col xs={24} md={12}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 8 }}>
                      <Activity size={13} strokeWidth={1.8} style={{ color: 'var(--acc-violet)' }} />
                      <Text style={{ color: 'var(--text-hi)', fontSize: 13 }}>{t.dashboard.topActions}</Text>
                    </div>
                    <List
                      size="small"
                      locale={{ emptyText: t.common.noData }}
                      dataSource={logStatistics?.top_actions || []}
                      renderItem={(item) => (
                        <List.Item style={{ padding: '7px 0' }}>
                          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                            <Tag color="default">{formatActionLabel(item.action)}</Tag>
                            <Text className="mono" style={{ color: 'var(--text-mid)' }}>{item.count}</Text>
                          </Space>
                        </List.Item>
                      )}
                    />
                  </Col>
                  <Col xs={24} md={12}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: 7, marginBottom: 8 }}>
                      <Activity size={13} strokeWidth={1.8} style={{ color: 'var(--acc-cyan)' }} />
                      <Text style={{ color: 'var(--text-hi)', fontSize: 13 }}>{t.dashboard.topUsers}</Text>
                    </div>
                    <List
                      size="small"
                      locale={{ emptyText: t.common.noData }}
                      dataSource={logStatistics?.top_users || []}
                      renderItem={(item) => (
                        <List.Item style={{ padding: '7px 0' }}>
                          <Space style={{ width: '100%', justifyContent: 'space-between' }}>
                            <Text style={{ color: 'var(--text-mid)' }}>{item.username || t.dashboard.systemActor}</Text>
                            <Text className="mono" style={{ color: 'var(--text-mid)' }}>{item.count}</Text>
                          </Space>
                        </List.Item>
                      )}
                    />
                  </Col>
                </Row>
              </Space>
            )}
          </div>
        </Col>

        <Col xs={24} xl={12}>
          <div className="glass sv-reveal sv-reveal-d4" style={{ padding: 20, height: '100%' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <ScrollText size={16} strokeWidth={1.7} style={{ color: 'var(--err)' }} />
                <span className="display" style={{ fontSize: 15 }}>{t.dashboard.recentAlerts}</span>
              </div>
              <Button type="link" style={{ color: 'var(--acc-violet)', padding: 0 }} onClick={() => navigate('/logs')}>
                {t.common.view}
                <ArrowRight size={14} strokeWidth={1.8} />
              </Button>
            </div>

            {activityError ? (
              <div style={{ color: 'var(--warn)', fontSize: 13 }}>{activityError}</div>
            ) : recentAlerts.length === 0 ? (
              <Empty
                image={Empty.PRESENTED_IMAGE_SIMPLE}
                description={<span style={{ color: 'var(--text-low)' }}>{t.dashboard.noRecentAlerts}</span>}
                style={{ padding: '40px 0' }}
              />
            ) : (
              <List
                itemLayout="vertical"
                dataSource={recentAlerts}
                renderItem={(log) => (
                  <List.Item
                    key={log.id}
                    style={{ cursor: 'pointer', padding: '12px 6px' }}
                    onClick={() => navigate('/logs')}
                  >
                    <Space direction="vertical" size={4} style={{ width: '100%' }}>
                      <Space wrap style={{ gap: 8 }}>
                        <span
                          style={{
                            display: 'inline-flex', alignItems: 'center', gap: 5,
                            padding: '2px 9px', borderRadius: 6,
                            fontFamily: 'var(--font-mono)', fontSize: 10.5, letterSpacing: '0.05em',
                            color: getLevelColor(log.level),
                            background: `${getLevelColor(log.level)}1a`,
                            border: `1px solid ${getLevelColor(log.level)}33`,
                          }}
                        >
                          <span style={{ width: 6, height: 6, borderRadius: '50%', background: getLevelColor(log.level) }} />
                          {log.level.toUpperCase()}
                        </span>
                        <Tag color="default">{formatActionLabel(log.action)}</Tag>
                        <Text className="mono" style={{ color: 'var(--text-low)', fontSize: 11.5 }}>
                          {dayjs(log.created_at).format('YYYY-MM-DD HH:mm:ss')}
                        </Text>
                      </Space>
                      <Text style={{ color: 'var(--text-hi)', fontWeight: 500, fontSize: 13 }}>{log.message}</Text>
                      <Text style={{ color: 'var(--text-low)', fontSize: 12 }}>
                        {(log.username || t.dashboard.systemActor)} · {log.resource || '-'} · {log.target || '-'}
                      </Text>
                    </Space>
                  </List.Item>
                )}
              />
            )}
          </div>
        </Col>
      </Row>
    </div>
  );
}