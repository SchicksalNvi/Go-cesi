import { useEffect, useState } from 'react';
import { Card, Row, Col, Button, Spin, Empty, message } from 'antd';
import { RefreshCw } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { environmentsApi } from '@/api/environments';
import { useWebSocket } from '@/hooks/useWebSocket';
import { Environment } from '@/types';
import { useStore } from '@/store';
import EnvironmentCard from './EnvironmentCard';

export default function EnvironmentList() {
  const navigate = useNavigate();
  const { t } = useStore();
  const [environments, setEnvironments] = useState<Environment[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    loadEnvironments();
  }, []);

  const loadEnvironments = async () => {
    setLoading(true);
    try {
      const response = await environmentsApi.getEnvironments();
      setEnvironments(response.environments || []);
    } catch (error) {
      console.error('Failed to load environments:', error);
      message.error('Failed to load environments. Please try again.');
    } finally {
      setLoading(false);
    }
  };

  // WebSocket real-time updates
  useWebSocket({
    onMessage: (msg) => {
      if (msg.type === 'nodes_update' || msg.type === 'node_status_change') {
        loadEnvironments();
      }
    },
  });

  if (loading && environments.length === 0) {
    return (
      <div style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    );
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
          <h2 style={{ margin: 0, fontSize: 24 }}>{t.environments.title}</h2>
          <p style={{ color: '#666', marginTop: 8 }}>
            Manage nodes by environment
          </p>
        </div>
        <Button
          type="primary"
          icon={<RefreshCw size={14} strokeWidth={1.7} />}
          onClick={loadEnvironments}
          loading={loading}
        >
          {t.common.refresh}
        </Button>
      </div>

      {environments.length === 0 ? (
        <Card>
          <Empty
            description={t.common.noData}
            image={Empty.PRESENTED_IMAGE_SIMPLE}
          >
            <p style={{ color: '#999' }}>
              Add nodes with environment labels in your config.toml file
            </p>
          </Empty>
        </Card>
      ) : (
        <Row gutter={[16, 16]}>
          {environments.map((env) => (
            <Col xs={24} sm={12} lg={8} key={env.name}>
              <EnvironmentCard
                environment={env}
                onClick={() => navigate(`/environments/${env.name}`)}
              />
            </Col>
          ))}
        </Row>
      )}
    </div>
  );
}
