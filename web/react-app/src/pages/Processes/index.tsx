import { Suspense, lazy, useEffect, useState } from 'react';
import {
  Card,
  Input,
  Spin,
  Empty,
  Space,
  Button,
  message,
  Collapse,
  Tag,
  Row,
  Col,
  Statistic,
  Popconfirm,
  Modal,
} from 'antd';
import {
  RefreshCw,
  Search as SearchIcon,
  PlayCircle,
  Square,
  Info,
} from 'lucide-react';
import { processesApi } from '@/api/processes';
import { AggregatedProcess, BatchOperationResult } from '@/types';
import { useWebSocket } from '@/hooks/useWebSocket';
import { useAutoRefresh } from '@/hooks/useAutoRefresh';
import { useStore } from '@/store';

const { Search } = Input;
const ProcessInstanceList = lazy(() => import('./ProcessInstanceList'));

const ProcessesPage: React.FC = () => {
  const { t } = useStore();
  const [processes, setProcesses] = useState<AggregatedProcess[]>([]);
  const [filteredProcesses, setFilteredProcesses] = useState<AggregatedProcess[]>([]);
  const [loading, setLoading] = useState(true);
  const [searchText, setSearchText] = useState('');
  const [actionLoading, setActionLoading] = useState<Record<string, boolean>>({});
  const [expandedProcesses, setExpandedProcesses] = useState<Record<string, boolean>>({});

  // function declaration — hoisted, avoids TDZ in minified bundle
  async function loadProcesses() {
    // 仅在首次加载(尚无数据)时显示全屏 loading;后续刷新保持列表显示,
    // 避免搜索结果被 loading 闪烁"冲掉"。
    if (processes.length === 0) {
      setLoading(true);
    }
    try {
      const response = await processesApi.getAggregated();
      setProcesses(response.processes || []);

      // 数据更新后立即重新应用当前搜索过滤,保证搜索结果不丢失。
      const next = response.processes || [];
      if (searchText.trim()) {
        setFilteredProcesses(next.filter((proc) =>
          proc.name.toLowerCase().includes(searchText.trim().toLowerCase())
        ));
      } else {
        setFilteredProcesses(next);
      }
    } catch (error) {
      console.error('Failed to load processes:', error);
      message.error('Failed to load processes');
    } finally {
      setLoading(false);
    }
  }

  function filterProcesses() {
    if (!searchText.trim()) {
      setFilteredProcesses(processes);
      return;
    }

    const filtered = processes.filter((proc) =>
      proc.name.toLowerCase().includes(searchText.toLowerCase())
    );
    setFilteredProcesses(filtered);
  };

  useEffect(() => {
    loadProcesses();
  }, []);

  useEffect(() => {
    filterProcesses();
  }, [searchText, processes]);

  // WebSocket 实时更新
  useWebSocket({
    onMessage: (message) => {
      if (
        message.type === 'process_status_change' ||
        message.type === 'node_status_change' ||
        message.type === 'nodes_update'
      ) {
        loadProcesses();
      }
    },
  });

  // Auto refresh
  useAutoRefresh(loadProcesses);

  const handleSearch = (value: string) => {
    setSearchText(value);
  };

  const handleDetailsToggle = (processName: string, keys: string | string[]) => {
    const activeKeys = Array.isArray(keys) ? keys : [keys];
    setExpandedProcesses((prev) => ({
      ...prev,
      [processName]: activeKeys.includes('details'),
    }));
  };

  const handleBatchOperation = async (
    processName: string,
    operation: 'start' | 'stop' | 'restart'
  ) => {
    const actionKey = `${processName}-${operation}`;
    setActionLoading((prev) => ({ ...prev, [actionKey]: true }));

    try {
      let response;
      switch (operation) {
        case 'start':
          response = await processesApi.batchStart(processName);
          break;
        case 'stop':
          response = await processesApi.batchStop(processName);
          break;
        case 'restart':
          response = await processesApi.batchRestart(processName);
          break;
      }

      const result: BatchOperationResult = response.result;

      // 显示操作结果
      if (result.failure_count === 0) {
        message.success(
          `Successfully ${operation}ed ${result.success_count} instance(s) of ${processName}`
        );
      } else {
        Modal.warning({
          title: `Batch ${operation} completed with errors`,
          content: (
            <div>
              <p>
                Success: {result.success_count} / {result.total_instances}
              </p>
              <p>Failed: {result.failure_count}</p>
              {result.results
                .filter((r) => !r.success)
                .map((r, idx) => (
                  <div key={idx}>
                    <strong>{r.node_name}:</strong> {r.error}
                  </div>
                ))}
            </div>
          ),
        });
      }

      // 重新加载进程列表
      await loadProcesses();
    } catch (error) {
      console.error(`Failed to ${operation} process:`, error);
      message.error(`Failed to ${operation} process`);
    } finally {
      setActionLoading((prev) => ({ ...prev, [actionKey]: false }));
    }
  };

  if (loading) {
    return (
      <div style={{ textAlign: 'center', padding: '50px' }}>
        <Spin size="large" />
      </div>
    );
  }

  return (
    <div>
      <div style={{ marginBottom: 16, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <h1 style={{ margin: 0 }}>{t.processes.title}</h1>
        <Button
          type="primary"
          icon={<RefreshCw size={14} strokeWidth={1.7} />}
          onClick={loadProcesses}
          loading={loading}
        >
          {t.common.refresh}
        </Button>
      </div>

      <Card style={{ marginBottom: 16 }}>
        <Search
          placeholder={t.common.search + '...'}
          allowClear
          enterButton={<SearchIcon size={14} strokeWidth={1.7} />}
          size="large"
          onSearch={handleSearch}
          onChange={(e) => handleSearch(e.target.value)}
          value={searchText}
        />
      </Card>

      {filteredProcesses.length === 0 ? (
        <Card>
          <Empty
            description={
              searchText
                ? `${t.common.noData}: "${searchText}"`
                : t.common.noData
            }
          />
        </Card>
      ) : (
        <Space direction="vertical" size="middle" style={{ width: '100%' }}>
          {filteredProcesses.map((proc) => (
            <Card
              key={proc.name}
              title={
                <Space>
                  <strong>{proc.name}</strong>
                  <Tag color="blue">{proc.total_instances} instances</Tag>
                </Space>
              }
              extra={
                <Space>
                  <Button
                    type="primary"
                    size="small"
                    icon={<PlayCircle size={14} strokeWidth={1.7} />}
                    onClick={() => handleBatchOperation(proc.name, 'start')}
                    loading={actionLoading[`${proc.name}-start`]}
                    disabled={proc.running_instances === proc.total_instances}
                  >
                    {t.nodes.startAll}
                  </Button>
                  <Popconfirm
                    title={t.processes.confirmStop}
                    onConfirm={() => handleBatchOperation(proc.name, 'stop')}
                    okText={t.common.yes}
                    cancelText={t.common.no}
                  >
                    <Button
                      size="small"
                      icon={<Square size={14} strokeWidth={1.7} />}
                      loading={actionLoading[`${proc.name}-stop`]}
                      disabled={proc.running_instances === 0}
                      danger
                    >
                      {t.nodes.stopAll}
                    </Button>
                  </Popconfirm>
                  <Button
                    size="small"
                    icon={<RefreshCw size={14} strokeWidth={1.7} />}
                    onClick={() => handleBatchOperation(proc.name, 'restart')}
                    loading={actionLoading[`${proc.name}-restart`]}
                  >
                    {t.nodes.restartAll}
                  </Button>
                </Space>
              }
            >
              <Row gutter={16} style={{ marginBottom: 16 }}>
                <Col xs={24} sm={8}>
                  <Statistic
                    title={t.common.total}
                    value={proc.total_instances}
                    prefix={<Info size={14} strokeWidth={1.7} />}
                  />
                </Col>
                <Col xs={24} sm={8}>
                  <Statistic
                    title={t.processes.running}
                    value={proc.running_instances}
                    prefix={<PlayCircle size={14} strokeWidth={1.7} />}
                    valueStyle={{ color: '#52c41a' }}
                  />
                </Col>
                <Col xs={24} sm={8}>
                  <Statistic
                    title={t.processes.stopped}
                    value={proc.stopped_instances}
                    prefix={<Square size={14} strokeWidth={1.7} />}
                    valueStyle={{ color: '#ff4d4f' }}
                  />
                </Col>
              </Row>

              <div style={{ marginBottom: 16 }}>
                <strong>{t.nav.nodes}:</strong>{' '}
                {proc.instances.map((inst, idx) => (
                  <Tag key={idx} color="default">
                    {inst.node_name}
                  </Tag>
                ))}
              </div>

              <Collapse
                onChange={(keys) => handleDetailsToggle(proc.name, keys)}
                items={[
                  {
                    key: 'details',
                    label: t.common.details,
                    children: expandedProcesses[proc.name] ? (
                      <Suspense
                        fallback={
                          <div style={{ textAlign: 'center', padding: 24 }}>
                            <Spin />
                          </div>
                        }
                      >
                        <ProcessInstanceList
                          instances={proc.instances}
                          processName={proc.name}
                          onRefresh={loadProcesses}
                        />
                      </Suspense>
                    ) : null,
                  },
                ]}
              />
            </Card>
          ))}
        </Space>
      )}
    </div>
  );
};

export default ProcessesPage;
