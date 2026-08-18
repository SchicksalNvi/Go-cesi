import { Suspense, lazy, useEffect, useState } from 'react';
import { Lightbulb } from 'lucide-react';
import { Card, Button, Spin, Empty, message } from 'antd';
import { useNavigate } from 'react-router-dom';
import { nodesApi } from '@/api/nodes';
import { useStore } from '@/store';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAutoRefresh } from '@/hooks/useAutoRefresh';
import { useLocalStorage } from '@/hooks/useLocalStorage';
import { useNodeFiltering } from '@/hooks/useNodeFiltering';
import { useIsMobile } from '@/hooks/useResponsive';
import NodesToolbar from '@/components/NodesToolbar';
import ErrorBoundary from '@/components/ErrorBoundary';
import type { ViewMode } from '@/components/ViewToggle';
import type { NodeFilters } from '@/components/FilterBar';
import type { BulkAction } from '@/components/NodesToolbar';
import { useListPerformance } from '@/hooks/usePerformanceMonitor';

const NodesListView = lazy(() => import('@/components/NodesListView'));
const PaginatedCardView = lazy(() => import('@/components/PaginatedCardView'));

function ViewFallback() {
  return (
    <div style={{ textAlign: 'center', padding: 32 }}>
      <Spin />
    </div>
  );
}

function NodeList() {
  const navigate = useNavigate();
  const { nodes, setNodes } = useStore();
  const [loading, setLoading] = useState(false);
  const isMobile = useIsMobile();

  // Performance monitoring
  const { shouldOptimize, recommendations } = useListPerformance(nodes.length);

  // View mode and search/filter state
  const [viewMode, setViewMode] = useLocalStorage<ViewMode>('nodes-view-mode', 'card');
  const [searchQuery, setSearchQuery] = useState('');
  const [filters, setFilters] = useState<NodeFilters>({});
  const [selectedNodes, setSelectedNodes] = useState<string[]>([]);

  // Force list view on mobile
  const effectiveViewMode = isMobile ? 'list' : viewMode;

  // Filter nodes
  const { filteredNodes, availableEnvironments, stats } = useNodeFiltering(
    nodes,
    searchQuery,
    filters
  );

  // Load nodes
  async function loadNodes() {
    setLoading(true);
    try {
      const response = await nodesApi.getNodes();
      setNodes(response.nodes || []);
    } catch (error) {
      console.error('Failed to load nodes:', error);
      message.error('Failed to load nodes');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    loadNodes();
  }, []);

  // WebSocket real-time updates
  useWebSocket({
    onMessage: (message) => {
      if (message.type === 'nodes_update') {
        setNodes(message.data);
      }
    },
  });

  // Auto refresh
  useAutoRefresh(loadNodes);

  const handleBulkAction = async (action: BulkAction) => {
    const nodeIds = action.nodeIds;
    if (nodeIds.length === 0) return;

    try {
      switch (action.type) {
        case 'refresh_all':
          message.loading({ content: `Refreshing ${nodeIds.length} nodes...`, key: 'bulk' });
          await loadNodes();
          message.success({ content: `Refreshed ${nodeIds.length} nodes`, key: 'bulk' });
          break;
        case 'restart_all':
          message.loading({ content: `Restarting all processes on ${nodeIds.length} nodes...`, key: 'bulk' });
          await Promise.all(nodeIds.map(nodeName => nodesApi.restartAllProcesses(nodeName)));
          message.success({ content: `Restarted all processes on ${nodeIds.length} nodes`, key: 'bulk' });
          break;
        case 'start_all':
          message.loading({ content: `Starting all processes on ${nodeIds.length} nodes...`, key: 'bulk' });
          await Promise.all(nodeIds.map(nodeName => nodesApi.startAllProcesses(nodeName)));
          message.success({ content: `Started all processes on ${nodeIds.length} nodes`, key: 'bulk' });
          break;
        case 'stop_all':
          message.loading({ content: `Stopping all processes on ${nodeIds.length} nodes...`, key: 'bulk' });
          await Promise.all(nodeIds.map(nodeName => nodesApi.stopAllProcesses(nodeName)));
          message.success({ content: `Stopped all processes on ${nodeIds.length} nodes`, key: 'bulk' });
          break;
        case 'delete_selected':
          message.warning('Node deletion is not supported - nodes are defined in config.toml');
          break;
      }
      setSelectedNodes([]);
    } catch (error) {
      console.error('Bulk action failed:', error);
      message.error({ content: 'Bulk action failed', key: 'bulk' });
    }
  };

  if (loading && nodes.length === 0) {
    return (
      <div style={{ textAlign: 'center', padding: 50 }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <ErrorBoundary>
      <div>
        <NodesToolbar
          viewMode={effectiveViewMode}
          onViewModeChange={setViewMode}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          filters={filters}
          onFiltersChange={setFilters}
          environments={availableEnvironments}
          totalNodes={stats.total}
          filteredNodes={stats.filtered}
          selectedNodes={selectedNodes}
          onBulkAction={handleBulkAction}
          onRefreshAll={loadNodes}
          loading={loading}
          isMobile={isMobile}
        />

        {/* Performance recommendations */}
        {shouldOptimize && recommendations.length > 0 && process.env.NODE_ENV === 'development' && (
          <div style={{ marginBottom: 16 }}>
            {recommendations.map((rec, index) => (
              <div key={index} style={{ 
                padding: 8, 
                backgroundColor: '#fff7e6', 
                border: '1px solid #ffd591',
                borderRadius: 4,
                marginBottom: 4,
                fontSize: 12,
                color: '#d46b08'
              }}>
                <Lightbulb size={15} strokeWidth={1.7} /> {rec}
              </div>
            ))}
          </div>
        )}

        {filteredNodes.length === 0 ? (
          <Card>
            <Empty
              description={
                stats.total === 0 
                  ? "No nodes configured"
                  : "No nodes match your search criteria"
              }
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            >
              {stats.total === 0 ? (
                <p style={{ color: '#999' }}>
                  Add nodes in your config.toml file
                </p>
              ) : (
                <Button onClick={() => {
                  setSearchQuery('');
                  setFilters({});
                }}>
                  Clear filters
                </Button>
              )}
            </Empty>
          </Card>
        ) : effectiveViewMode === 'list' ? (
          <ErrorBoundary fallback={
            <div style={{ padding: 20, textAlign: 'center' }}>
              <p>Failed to load list view. Please try refreshing the page.</p>
            </div>
          }>
            <Suspense fallback={<ViewFallback />}>
              <NodesListView
                nodes={filteredNodes}
                loading={loading}
                selectedNodes={selectedNodes}
                onSelectionChange={setSelectedNodes}
                onNodeClick={(nodeName) => navigate(`/nodes/${nodeName}`)}
                onRefreshNode={async (nodeName) => {
                  message.loading({ content: `Refreshing node: ${nodeName}`, key: 'refresh' });
                  await loadNodes();
                  message.success({ content: `Node ${nodeName} refreshed`, key: 'refresh' });
                }}
                searchQuery={searchQuery}
              />
            </Suspense>
          </ErrorBoundary>
        ) : (
          <ErrorBoundary fallback={
            <div style={{ padding: 20, textAlign: 'center' }}>
              <p>Failed to load card view. Please try refreshing the page.</p>
            </div>
          }>
            <Suspense fallback={<ViewFallback />}>
              <PaginatedCardView
                nodes={filteredNodes}
                onNodeClick={(nodeName) => navigate(`/nodes/${nodeName}`)}
                searchQuery={searchQuery}
              />
            </Suspense>
          </ErrorBoundary>
        )}
      </div>
    </ErrorBoundary>
  );
}

export default NodeList;
