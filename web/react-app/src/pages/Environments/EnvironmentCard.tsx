import { Card, Space, Tag } from 'antd';
import { LayoutGrid, CheckCircle2, XCircle } from 'lucide-react';
import { Environment } from '@/types';
import { useStore } from '@/store';

interface EnvironmentCardProps {
  environment: Environment;
  onClick: () => void;
}

export default function EnvironmentCard({ environment, onClick }: EnvironmentCardProps) {
  const { t } = useStore();
  const totalNodes = environment.members.length;
  const onlineNodes = environment.members.filter((node) => node.is_connected).length;
  const offlineNodes = totalNodes - onlineNodes;

  return (
    <Card
      hoverable
      style={{
        height: '100%',
        cursor: 'pointer',
      }}
      onClick={onClick}
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        {/* Header */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <div
            style={{
              width: 48,
              height: 48,
              borderRadius: 8,
              background: '#52c41a20',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <LayoutGrid size={24} strokeWidth={1.7} style={{ fontSize: 24, color: '#52c41a' }} />
          </div>
          <div>
            <h3 style={{ margin: 0, fontSize: 18 }}>{environment.name}</h3>
            <p style={{ margin: 0, color: '#999', fontSize: 14 }}>
              {totalNodes} {t.nav.nodes.toLowerCase()}
            </p>
          </div>
        </div>

        {/* Stats */}
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-around',
            paddingTop: 16,
            borderTop: '1px solid #f0f0f0',
          }}
        >
          <div style={{ textAlign: 'center' }}>
            <div style={{ fontSize: 24, fontWeight: 'bold', color: '#52c41a' }}>
              {onlineNodes}
            </div>
            <div style={{ fontSize: 12, color: '#999', marginTop: 4 }}>
              <CheckCircle2 size={14} strokeWidth={1.7} style={{ marginRight: 4 }} />
              {t.nodes.online}
            </div>
          </div>
          <div
            style={{
              width: 1,
              background: '#f0f0f0',
            }}
          />
          <div style={{ textAlign: 'center' }}>
            <div style={{ fontSize: 24, fontWeight: 'bold', color: '#ff4d4f' }}>
              {offlineNodes}
            </div>
            <div style={{ fontSize: 12, color: '#999', marginTop: 4 }}>
              <XCircle size={14} strokeWidth={1.7} style={{ marginRight: 4 }} />
              {t.nodes.offline}
            </div>
          </div>
        </div>

        {/* Status Badge */}
        <div style={{ textAlign: 'center' }}>
          {onlineNodes === totalNodes ? (
            <Tag color="success" style={{ margin: 0 }}>
              {t.environmentCard.allNodesOnline}
            </Tag>
          ) : onlineNodes === 0 ? (
            <Tag color="error" style={{ margin: 0 }}>
              {t.environmentCard.allNodesOffline}
            </Tag>
          ) : (
            <Tag color="warning" style={{ margin: 0 }}>
              {t.environmentCard.partialConnectivity}
            </Tag>
          )}
        </div>
      </Space>
    </Card>
  );
}
