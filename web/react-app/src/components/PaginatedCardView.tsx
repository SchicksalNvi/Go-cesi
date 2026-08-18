import React, { useState, useMemo } from 'react';
import { Row, Col, Pagination, Card, Tag, Button, Space, Alert } from 'antd';
import {
  CheckCircle2,
  Eye,
} from 'lucide-react';
import { Node } from '@/types';
import { useStore } from '@/store';

interface PaginatedCardViewProps {
  nodes: Node[];
  onNodeClick: (nodeName: string) => void;
  searchQuery?: string;
  pageSize?: number;
}

const DEFAULT_PAGE_SIZE = 50;
const PAGINATION_THRESHOLD = 20; // Show pagination when more than 20 nodes

export const PaginatedCardView: React.FC<PaginatedCardViewProps> = ({
  nodes,
  onNodeClick,
  searchQuery = '',
  pageSize = DEFAULT_PAGE_SIZE,
}) => {
  const { t } = useStore();
  const [currentPage, setCurrentPage] = useState(1);
  const [currentPageSize, setCurrentPageSize] = useState(pageSize);

  // Calculate pagination
  const shouldPaginate = nodes.length > PAGINATION_THRESHOLD;
  
  const paginatedNodes = useMemo(() => {
    if (!shouldPaginate) {
      return nodes;
    }
    
    const start = (currentPage - 1) * currentPageSize;
    return nodes.slice(start, start + currentPageSize);
  }, [nodes, currentPage, currentPageSize, shouldPaginate]);

  // Reset to first page when nodes change (e.g., after filtering)
  React.useEffect(() => {
    setCurrentPage(1);
  }, [nodes.length]);

  // Highlight search matches
  const highlightText = (text: string) => {
    if (!searchQuery) return text;
    
    const regex = new RegExp(`(${searchQuery.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
    const parts = text.split(regex);
    
    return parts.map((part, index) => 
      regex.test(part) ? (
        <mark key={index} style={{ backgroundColor: '#fff566', padding: 0 }}>
          {part}
        </mark>
      ) : part
    );
  };

  const handlePageChange = (page: number, size?: number) => {
    setCurrentPage(page);
    if (size && size !== currentPageSize) {
      setCurrentPageSize(size);
    }
  };

  return (
    <div>
      {/* Performance info for large datasets */}
      {shouldPaginate && (
        <Alert
          message={t.paginatedCard.largeDatasetInfo.replace('{count}', String(nodes.length)).replace('{pageSize}', String(currentPageSize))}
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
        />
      )}

      {/* Cards Grid */}
      <Row gutter={[16, 16]}>
        {paginatedNodes.map((node) => (
          <Col xs={24} sm={12} lg={8} key={node.name}>
            <NodeCard 
              node={node} 
              onView={() => onNodeClick(node.name)}
              searchQuery={searchQuery}
            />
          </Col>
        ))}
      </Row>

      {/* Pagination Controls */}
      {shouldPaginate && (
        <div style={{ 
          marginTop: 24, 
          textAlign: 'center',
          padding: '16px 0',
          borderTop: '1px solid #f0f0f0'
        }}>
          <Pagination
            current={currentPage}
            total={nodes.length}
            pageSize={currentPageSize}
            showSizeChanger
            showQuickJumper
            showTotal={(total, range) => 
              t.paginatedCard.ofNodes.replace('{start}', String(range[0])).replace('{end}', String(range[1])).replace('{total}', String(total))
            }
            pageSizeOptions={['20', '50', '100', '200']}
            onChange={handlePageChange}
            onShowSizeChange={handlePageChange}
          />
        </div>
      )}
    </div>
  );
};

// Node Card Component
function NodeCard({ 
  node, 
  onView, 
  searchQuery = '' 
}: { 
  node: Node; 
  onView: () => void;
  searchQuery?: string;
}) {
  const { t } = useStore();
  const isOnline = node.is_connected;

  // Highlight search matches
  const highlightText = (text: string) => {
    if (!searchQuery) return text;
    
    const regex = new RegExp(`(${searchQuery.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
    const parts = text.split(regex);
    
    return parts.map((part, index) => 
      regex.test(part) ? (
        <mark key={index} style={{ backgroundColor: '#fff566', padding: 0 }}>
          {part}
        </mark>
      ) : part
    );
  };

  return (
    <Card
      hoverable
      style={{ height: '100%', cursor: 'pointer' }}
      onClick={onView}
    >
      <Space direction="vertical" size="middle" style={{ width: '100%' }}>
        {/* Header */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <div
              style={{
                width: 48,
                height: 48,
                borderRadius: 8,
                background: '#1890ff20',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}
            >
              <CheckCircle2 size={24} strokeWidth={1.7} style={{ color: '#1890ff' }} />
            </div>
            <div>
              <h3 style={{ margin: 0, fontSize: 18 }}>
                {highlightText(node.name)}
              </h3>
              <Tag color="blue" style={{ marginTop: 4 }}>
                {highlightText(node.environment || 'default')}
              </Tag>
            </div>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <div
              style={{
                width: 10,
                height: 10,
                borderRadius: '50%',
                background: isOnline ? '#52c41a' : '#ff4d4f',
              }}
            />
            <span style={{ fontSize: 12, color: isOnline ? '#52c41a' : '#ff4d4f', fontWeight: 500 }}>
              {isOnline ? t.nodes.online : t.nodes.offline}
            </span>
          </div>
        </div>

        {/* Info */}
        <div>
          <div style={{ color: '#666', fontSize: 14, marginBottom: 4 }}>
            {highlightText(`${node.host}:${node.port}`)}
          </div>
          {node.username && (
            <div style={{ color: '#666', fontSize: 14 }}>
              User: {highlightText(node.username)}
            </div>
          )}
        </div>

        {/* Stats */}
        <div
          style={{
            display: 'flex',
            paddingTop: 16,
            borderTop: '1px solid #f0f0f0',
          }}
        >
          <div style={{ flex: 1, textAlign: 'center' }}>
            <div style={{ fontSize: 12, color: '#999' }}>{t.nodes.processes}</div>
            <div style={{ fontSize: 22, fontWeight: 'bold', color: '#1890ff' }}>
              {isOnline ? (node.process_count || 0) : '-'}
            </div>
          </div>
          <div style={{ flex: 1, textAlign: 'center', borderLeft: '1px solid #f0f0f0' }}>
            <div style={{ fontSize: 12, color: '#999' }}>{t.dashboard.runningProcesses}</div>
            <div style={{ fontSize: 22, fontWeight: 'bold', color: '#52c41a' }}>
              {isOnline ? (node.running_count || 0) : '-'}
            </div>
          </div>
        </div>

        {/* Action Button */}
        <Button
          type="primary"
          block
          icon={<Eye size={14} strokeWidth={1.7} />}
          onClick={(e) => {
            e.stopPropagation();
            onView();
          }}
        >
          {t.nodes.viewDetails}
        </Button>
      </Space>
    </Card>
  );
}

export default PaginatedCardView;