import React, { useMemo, useState, useCallback } from 'react';
import * as ReactWindow from 'react-window';
import { Table, Tag, Button, Space, Tooltip, Typography, Checkbox } from 'antd';
import {
  CheckCircle2,
  XCircle,
  Eye,
  RefreshCw,
} from 'lucide-react';
import { Node } from '@/types';

const { Text } = Typography;

interface VirtualizedNodesTableProps {
  nodes: Node[];
  loading?: boolean;
  selectedNodes: string[];
  onSelectionChange: (nodeIds: string[]) => void;
  onNodeClick: (nodeName: string) => void;
  onRefreshNode?: (nodeName: string) => void;
  searchQuery?: string;
  height?: number;
}

interface NodeListItem extends Node {
  key: string;
  statusColor: string;
  displayName: string;
}

const ROW_HEIGHT = 48;
const HEADER_HEIGHT = 40;

export const VirtualizedNodesTable: React.FC<VirtualizedNodesTableProps> = ({
  nodes,
  loading = false,
  selectedNodes,
  onSelectionChange,
  onNodeClick,
  onRefreshNode,
  searchQuery = '',
  height = 400,
}) => {
  const VirtualList = ReactWindow.List as any;
  const [sortedInfo, setSortedInfo] = useState<any>({});

  // Transform nodes for table display
  const tableData: NodeListItem[] = useMemo(() => {
    let data = nodes.map(node => ({
      ...node,
      key: node.name,
      statusColor: node.is_connected ? '#52c41a' : '#ff4d4f',
      displayName: node.name,
    }));

    // Apply sorting
    if (sortedInfo.columnKey) {
      data = data.sort((a, b) => {
        const aVal = a[sortedInfo.columnKey as keyof NodeListItem];
        const bVal = b[sortedInfo.columnKey as keyof NodeListItem];
        
        if (typeof aVal === 'string' && typeof bVal === 'string') {
          const result = aVal.localeCompare(bVal);
          return sortedInfo.order === 'descend' ? -result : result;
        }
        
        if (typeof aVal === 'number' && typeof bVal === 'number') {
          const result = aVal - bVal;
          return sortedInfo.order === 'descend' ? -result : result;
        }
        
        if (typeof aVal === 'boolean' && typeof bVal === 'boolean') {
          const result = Number(aVal) - Number(bVal);
          return sortedInfo.order === 'descend' ? -result : result;
        }
        
        return 0;
      });
    }

    return data;
  }, [nodes, sortedInfo]);

  // Highlight search matches in text
  const highlightText = (text: string, query: string) => {
    if (!query) return text;
    
    const regex = new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi');
    const parts = text.split(regex);
    
    return parts.map((part, index) => 
      regex.test(part) ? (
        <mark key={index} style={{ backgroundColor: '#fff566', padding: 0 }}>
          {part}
        </mark>
      ) : part
    );
  };

  // Handle row selection
  const handleRowSelect = useCallback((nodeKey: string, checked: boolean) => {
    if (checked) {
      onSelectionChange([...selectedNodes, nodeKey]);
    } else {
      onSelectionChange(selectedNodes.filter(key => key !== nodeKey));
    }
  }, [selectedNodes, onSelectionChange]);

  // Handle select all
  const handleSelectAll = useCallback((checked: boolean) => {
    if (checked) {
      onSelectionChange(tableData.map(node => node.key));
    } else {
      onSelectionChange([]);
    }
  }, [tableData, onSelectionChange]);

  // Handle column sorting
  const handleSort = useCallback((columnKey: string) => {
    const order = sortedInfo.columnKey === columnKey && sortedInfo.order === 'ascend' 
      ? 'descend' 
      : 'ascend';
    setSortedInfo({ columnKey, order });
  }, [sortedInfo]);

  // Row renderer for virtual list
  const Row = useCallback(({ index, style }: { index: number; style: React.CSSProperties }) => {
    const node = tableData[index];
    const isSelected = selectedNodes.includes(node.key);

    return (
      <div
        style={{
          ...style,
          display: 'flex',
          alignItems: 'center',
          borderBottom: '1px solid #f0f0f0',
          backgroundColor: isSelected ? '#e6f7ff' : 'white',
          cursor: 'pointer',
        }}
        onClick={() => onNodeClick(node.name)}
      >
        {/* Selection Checkbox */}
        <div style={{ width: 50, textAlign: 'center', paddingLeft: 16 }}>
          <Checkbox
            checked={isSelected}
            onChange={(e) => {
              e.stopPropagation();
              handleRowSelect(node.key, e.target.checked);
            }}
          />
        </div>

        {/* Status */}
        <div style={{ width: 80, textAlign: 'center' }}>
          <div
            style={{
              width: 12,
              height: 12,
              borderRadius: '50%',
              backgroundColor: node.statusColor,
              margin: '0 auto',
            }}
          />
        </div>

        {/* Name */}
        <div style={{ width: 200, paddingLeft: 8 }}>
          <Button
            type="link"
            onClick={(e) => {
              e.stopPropagation();
              onNodeClick(node.name);
            }}
            style={{ padding: 0, height: 'auto', fontWeight: 500 }}
          >
            {highlightText(node.name, searchQuery)}
          </Button>
        </div>

        {/* Environment */}
        <div style={{ width: 120, paddingLeft: 8 }}>
          <Tag color="blue">
            {highlightText(node.environment || 'default', searchQuery)}
          </Tag>
        </div>

        {/* Host:Port */}
        <div style={{ width: 180, paddingLeft: 8 }}>
          <Text code style={{ fontSize: 12 }}>
            {highlightText(`${node.host}:${node.port}`, searchQuery)}
          </Text>
        </div>

        {/* User */}
        <div style={{ width: 100, paddingLeft: 8 }}>
          {node.username ? (
            <Text type="secondary" style={{ fontSize: 12 }}>
              {highlightText(node.username, searchQuery)}
            </Text>
          ) : (
            <Text type="secondary" style={{ fontSize: 12 }}>-</Text>
          )}
        </div>

        {/* Processes */}
        <div style={{ width: 100, textAlign: 'center' }}>
          <Tag color={node.process_count > 0 ? 'blue' : 'default'}>
            {node.process_count || 0}
          </Tag>
        </div>

        {/* Status Tag */}
        <div style={{ width: 100, paddingLeft: 8 }}>
          <Tag
            icon={node.is_connected ? <CheckCircle2 size={14} strokeWidth={1.7} /> : <XCircle size={14} strokeWidth={1.7} />}
            color={node.is_connected ? 'success' : 'error'}
          >
            {node.is_connected ? 'Online' : 'Offline'}
          </Tag>
        </div>

        {/* Actions */}
        <div style={{ width: 120, textAlign: 'center' }}>
          <Space size="small">
            <Tooltip title="View Details">
              <Button
                type="text"
                size="small"
                icon={<Eye size={14} strokeWidth={1.7} />}
                onClick={(e) => {
                  e.stopPropagation();
                  onNodeClick(node.name);
                }}
              />
            </Tooltip>
            {onRefreshNode && (
              <Tooltip title="Refresh Node">
                <Button
                  type="text"
                  size="small"
                  icon={<RefreshCw size={14} strokeWidth={1.7} />}
                  onClick={(e) => {
                    e.stopPropagation();
                    onRefreshNode(node.name);
                  }}
                />
              </Tooltip>
            )}
          </Space>
        </div>
      </div>
    );
  }, [tableData, selectedNodes, searchQuery, onNodeClick, onRefreshNode, handleRowSelect]);

  // Header component
  const Header = () => {
    const allSelected = tableData.length > 0 && selectedNodes.length === tableData.length;
    const indeterminate = selectedNodes.length > 0 && selectedNodes.length < tableData.length;

    return (
      <div
        style={{
          height: HEADER_HEIGHT,
          display: 'flex',
          alignItems: 'center',
          borderBottom: '2px solid #f0f0f0',
          backgroundColor: '#fafafa',
          fontWeight: 'bold',
        }}
      >
        {/* Selection Header */}
        <div style={{ width: 50, textAlign: 'center', paddingLeft: 16 }}>
          <Checkbox
            checked={allSelected}
            indeterminate={indeterminate}
            onChange={(e) => handleSelectAll(e.target.checked)}
          />
        </div>

        {/* Status Header */}
        <div 
          style={{ width: 80, textAlign: 'center', cursor: 'pointer' }}
          onClick={() => handleSort('is_connected')}
        >
          Status {sortedInfo.columnKey === 'is_connected' && (sortedInfo.order === 'ascend' ? '↑' : '↓')}
        </div>

        {/* Name Header */}
        <div 
          style={{ width: 200, paddingLeft: 8, cursor: 'pointer' }}
          onClick={() => handleSort('name')}
        >
          Name {sortedInfo.columnKey === 'name' && (sortedInfo.order === 'ascend' ? '↑' : '↓')}
        </div>

        {/* Environment Header */}
        <div 
          style={{ width: 120, paddingLeft: 8, cursor: 'pointer' }}
          onClick={() => handleSort('environment')}
        >
          Environment {sortedInfo.columnKey === 'environment' && (sortedInfo.order === 'ascend' ? '↑' : '↓')}
        </div>

        {/* Host:Port Header */}
        <div style={{ width: 180, paddingLeft: 8 }}>
          Host:Port
        </div>

        {/* User Header */}
        <div style={{ width: 100, paddingLeft: 8 }}>
          User
        </div>

        {/* Processes Header */}
        <div 
          style={{ width: 100, textAlign: 'center', cursor: 'pointer' }}
          onClick={() => handleSort('process_count')}
        >
          Processes {sortedInfo.columnKey === 'process_count' && (sortedInfo.order === 'ascend' ? '↑' : '↓')}
        </div>

        {/* Status Tag Header */}
        <div style={{ width: 100, paddingLeft: 8 }}>
          Status
        </div>

        {/* Actions Header */}
        <div style={{ width: 120, textAlign: 'center' }}>
          Actions
        </div>
      </div>
    );
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: 50 }}>
        Loading...
      </div>
    );
  }

    return (
      <div style={{ border: '1px solid #f0f0f0', borderRadius: 6 }}>
      <Header />
      <VirtualList
        height={height - HEADER_HEIGHT}
        itemCount={tableData.length}
        itemSize={ROW_HEIGHT}
        width="100%"
      >
        {Row}
      </VirtualList>
      
      {/* Footer with pagination info */}
      <div style={{ 
        padding: '8px 16px', 
        borderTop: '1px solid #f0f0f0', 
        backgroundColor: '#fafafa',
        fontSize: 14,
        color: '#666'
      }}>
        Total: {tableData.length} nodes
        {selectedNodes.length > 0 && ` | Selected: ${selectedNodes.length}`}
      </div>
    </div>
  );
};

export default VirtualizedNodesTable;
