import React from 'react';
import { Space, Button, Divider, Typography, Alert } from 'antd';
import {
  RefreshCw,
  PlayCircle,
  Square,
  RefreshCcw,
  Trash2,
} from 'lucide-react';
import { ViewToggle, ViewMode } from './ViewToggle';
import { SearchBox } from './SearchBox';
import { FilterBar, NodeFilters } from './FilterBar';
import { useStore } from '@/store';

const { Text } = Typography;

export interface BulkAction {
  type: 'restart_all' | 'stop_all' | 'start_all' | 'refresh_all' | 'delete_selected';
  nodeIds: string[];
}

interface NodesToolbarProps {
  // View mode
  viewMode: ViewMode;
  onViewModeChange: (mode: ViewMode) => void;
  
  // Search
  searchQuery: string;
  onSearchChange: (query: string) => void;
  
  // Filters
  filters: NodeFilters;
  onFiltersChange: (filters: NodeFilters) => void;
  environments: string[];
  
  // Node counts
  totalNodes: number;
  filteredNodes: number;
  
  // Selection and bulk actions
  selectedNodes: string[];
  onBulkAction: (action: BulkAction) => void;
  
  // General actions
  onRefreshAll: () => void;
  loading?: boolean;
  
  // Responsive
  isMobile?: boolean;
}

export const NodesToolbar: React.FC<NodesToolbarProps> = ({
  viewMode,
  onViewModeChange,
  searchQuery,
  onSearchChange,
  filters,
  onFiltersChange,
  environments,
  totalNodes,
  filteredNodes,
  selectedNodes,
  onBulkAction,
  onRefreshAll,
  loading = false,
  isMobile = false,
}) => {
  const { t } = useStore();
  const hasSelection = selectedNodes.length > 0;
  const isFiltered = filteredNodes !== totalNodes;

  const handleBulkAction = (type: BulkAction['type']) => {
    onBulkAction({
      type,
      nodeIds: selectedNodes,
    });
  };

  return (
    <div style={{ marginBottom: 16 }}>
      {/* Main Toolbar */}
      <div style={{ 
        display: 'flex', 
        justifyContent: 'space-between', 
        alignItems: 'flex-start',
        flexWrap: 'wrap',
        gap: 16,
        marginBottom: 16,
      }}>
        {/* Left Side - Title and Stats */}
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 8 }}>
            <div>
              <h2 style={{ margin: 0, fontSize: 24 }}>{t.nodesToolbar.nodes}</h2>
              <Text type="secondary" style={{ fontSize: 14 }}>
                {isFiltered ? (
                  <>{t.nodesToolbar.showingOf.replace('{filtered}', String(filteredNodes)).replace('{total}', String(totalNodes))}</>
                ) : (
                  <>{t.nodesToolbar.nodesTotal.replace('{total}', String(totalNodes))}</>
                )}
              </Text>
            </div>
            
            {/* View Toggle - Hide on mobile */}
            {!isMobile && (
              <>
                <Divider type="vertical" style={{ height: 32 }} />
                <ViewToggle
                  value={viewMode}
                  onChange={onViewModeChange}
                  disabled={loading}
                />
              </>
            )}
          </div>
        </div>

        {/* Right Side - Actions */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
          <Button
            type="primary"
            icon={<RefreshCw size={14} strokeWidth={1.7} />}
            onClick={onRefreshAll}
            loading={loading}
          >
            {isMobile ? '' : t.nodesToolbar.refreshAll}
          </Button>
        </div>
      </div>

      {/* Search and Filters */}
      <div style={{ 
        display: 'flex', 
        flexDirection: isMobile ? 'column' : 'row',
        gap: 16, 
        alignItems: isMobile ? 'stretch' : 'flex-start',
        marginBottom: hasSelection ? 16 : 0,
      }}>
        {/* Search Box */}
        <div style={{ flex: isMobile ? 'none' : '0 0 300px' }}>
          <SearchBox
            value={searchQuery}
            onChange={onSearchChange}
            placeholder={t.nodesToolbar.searchPlaceholder}
            size={isMobile ? 'large' : 'middle'}
          />
        </div>

        {/* Filter Bar */}
        <div style={{ flex: 1, minWidth: 0 }}>
          <FilterBar
            filters={filters}
            onFiltersChange={onFiltersChange}
            environments={environments}
            totalNodes={totalNodes}
            filteredNodes={filteredNodes}
            size={isMobile ? 'large' : 'middle'}
          />
        </div>
      </div>

      {/* Bulk Actions Toolbar */}
      {hasSelection && (
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message={
            <div style={{ 
              display: 'flex', 
              justifyContent: 'space-between', 
              alignItems: 'center',
              flexWrap: 'wrap',
              gap: 12,
            }}>
              <Text strong>
                {t.nodesToolbar.nodesSelected.replace('{count}', String(selectedNodes.length))}
              </Text>
              
              <Space size="small" wrap>
                <Button
                  size="small"
                  icon={<PlayCircle size={14} strokeWidth={1.7} />}
                  onClick={() => handleBulkAction('start_all')}
                >
                  {t.nodesToolbar.startAllProcesses}
                </Button>
                <Button
                  size="small"
                  icon={<Square size={14} strokeWidth={1.7} />}
                  onClick={() => handleBulkAction('stop_all')}
                >
                  {t.nodesToolbar.stopAllProcesses}
                </Button>
                <Button
                  size="small"
                  icon={<RefreshCcw size={14} strokeWidth={1.7} />}
                  onClick={() => handleBulkAction('restart_all')}
                >
                  {t.nodesToolbar.restartAllProcesses}
                </Button>
                <Button
                  size="small"
                  icon={<RefreshCw size={14} strokeWidth={1.7} />}
                  onClick={() => handleBulkAction('refresh_all')}
                >
                  {t.nodesToolbar.refreshStatus}
                </Button>
                <Divider type="vertical" />
                <Button
                  size="small"
                  danger
                  icon={<Trash2 size={14} strokeWidth={1.7} />}
                  onClick={() => handleBulkAction('delete_selected')}
                >
                  {t.nodesToolbar.removeSelected}
                </Button>
              </Space>
            </div>
          }
        />
      )}
    </div>
  );
};

export default NodesToolbar;