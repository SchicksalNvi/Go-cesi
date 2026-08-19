// LogViewer v2.1 - Fixed timestamp formatting
import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  Modal,
  Button,
  Switch,
  Space,
  Tag,
  Spin,
  message,
  Input,
  Select,
} from 'antd';
import { PlayCircle, PauseCircle, Eraser, Download, History, Search as SearchIcon } from 'lucide-react';
import { nodesApi } from '@/api/nodes';
import { useWebSocket } from '@/hooks/useWebSocket';
import { LogEntry, LogStreamMessage } from '@/types';
import { useStore } from '@/store';

const { Search } = Input;
const { Option } = Select;

interface LogViewerProps {
  visible: boolean;
  onClose: () => void;
  nodeName: string;
  processName: string;
}

const LogViewer: React.FC<LogViewerProps> = ({
  visible,
  onClose,
  nodeName,
  processName,
}) => {
  const { userPreferences, t } = useStore();
  const [logEntries, setLogEntries] = useState<LogEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [realTimeEnabled, setRealTimeEnabled] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [levelFilter, setLevelFilter] = useState<string>('');
  const [autoScroll, setAutoScroll] = useState(true);
  
  const logContainerRef = useRef<HTMLDivElement>(null);
  const isSubscribedRef = useRef(false);
  const [paging, setPaging] = useState(false);

  // LOGVIEWER_TIMESTAMP_V3 - 根据用户时区设置格式化时间戳
  const formatTimestamp = useCallback((timestamp: string) => {
    // 优先使用用户设置的时区，默认使用 Asia/Shanghai
    const timezone = userPreferences?.timezone || 'Asia/Shanghai';
    try {
      const date = new Date(timestamp);
      if (isNaN(date.getTime())) {
        return timestamp || '----/--/-- --:--:--';
      }
      const formatter = new Intl.DateTimeFormat('en-CA', {
        timeZone: timezone,
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
      });
      const parts = formatter.formatToParts(date);
      const get = (type: string) => parts.find(p => p.type === type)?.value || '00';
      return `${get('year')}-${get('month')}-${get('day')} ${get('hour')}:${get('minute')}:${get('second')}`;
    } catch {
      return timestamp || '----/--/-- --:--:--';
    }
  }, [userPreferences?.timezone]);

  // WebSocket for real-time logs
  const { send, isConnected } = useWebSocket({
    onMessage: (message) => {
      if (message.type === 'log_stream' && message.data) {
        const logData = message.data as LogStreamMessage;
        if (logData.node_name === nodeName && logData.process_name === processName) {
          handleNewLogEntries(logData.entries);
        }
      }
    },
  });

  const handleNewLogEntries = useCallback((newEntries: LogEntry[]) => {
    if (newEntries.length > 0) {
      setLogEntries(prev => {
        // 去重：基于 timestamp + message 组合
        const existingKeys = new Set(
          prev.map(e => `${e.timestamp}|${e.message}`)
        );
        const uniqueNewEntries = newEntries.filter(
          e => !existingKeys.has(`${e.timestamp}|${e.message}`)
        );
        
        if (uniqueNewEntries.length === 0) {
          return prev; // 没有新条目，不更新
        }
        
        const combined = [...prev, ...uniqueNewEntries];
        // Keep only last 1000 entries to prevent memory issues
        return combined.slice(-1000);
      });
      
      // Auto scroll to bottom if enabled
      if (autoScroll && logContainerRef.current) {
        setTimeout(() => {
          logContainerRef.current?.scrollTo({
            top: logContainerRef.current.scrollHeight,
            behavior: 'smooth'
          });
        }, 100);
      }
    }
  }, [autoScroll]);

  const loadInitialLogs = async () => {
    setLoading(true);
    try {
      // 不传 offset，让后端从文件末尾读取最新日志
      const response = await nodesApi.getProcessLogStream(nodeName, processName, undefined, 100);
      // 后端返回 { status, data: LogStream }
      setLogEntries(response.data?.entries || []);
    } catch (error) {
      console.error('Failed to load logs:', error);
      message.error('Failed to load logs');
    } finally {
      setLoading(false);
    }
  };

  // 加载更早的历史日志(分页)- 调用 supervisor.readProcessStdoutLog/StderrLog
  const loadOlderLogs = async () => {
    if (logEntries.length === 0) return;
    setPaging(true);
    try {
      const stdout = await nodesApi.readProcessLogs(nodeName, processName, 'stdout', 0, 20000);
      if (!stdout.data) {
        message.info(t.logViewer.noMoreLogs || 'No more logs');
        return;
      }
      const rawLines = stdout.data.split('\n').map((l: string) => l.trim()).filter((l: string) => l.length > 0);
      const existingKeys = new Set((logEntries as any[]).map(e => `${e.timestamp}|${e.message}`));
      const before: any[] = [];
      for (const line of rawLines) {
        const key = `-1|${line}`;
        if (!existingKeys.has(key)) {
          before.push({
            timestamp: new Date().toISOString(),
            level: 'INFO',
            message: line,
            source: 'stdout',
            process_name: processName,
            node_name: nodeName,
          });
          existingKeys.add(key);
        }
      }
      if (before.length === 0) {
        message.info(t.logViewer.noMoreLogs || 'No more logs');
      } else {
        setLogEntries(prev => [...before, ...prev]);
      }
    } catch (error: any) {
      console.error('Failed to load older logs:', error);
      message.error(error.response?.data?.message || 'Failed to load older logs');
    } finally {
      setPaging(false);
    }
  };

  const subscribeToLogs = () => {
    if (isConnected && !isSubscribedRef.current) {
      send({
        type: 'subscribe_logs',
        data: {
          node_name: nodeName,
          process_name: processName,
        },
      });
      isSubscribedRef.current = true;
    }
  };

  const unsubscribeFromLogs = () => {
    if (isConnected && isSubscribedRef.current) {
      send({
        type: 'unsubscribe_logs',
        data: {
          node_name: nodeName,
          process_name: processName,
        },
      });
      isSubscribedRef.current = false;
    }
  };

  const toggleRealTime = (enabled: boolean) => {
    setRealTimeEnabled(enabled);
    if (enabled) {
      subscribeToLogs();
    } else {
      unsubscribeFromLogs();
    }
  };

  const clearServerLogs = async () => {
    try {
      await nodesApi.clearProcessLogs(nodeName, processName);
      setLogEntries([]);
      message.success(t.logViewer.clearSuccess || 'Process logs cleared');
    } catch (error: any) {
      message.error(error.response?.data?.message || 'Failed to clear logs');
    }
  };

  const exportLogs = () => {
    const filteredEntries = getFilteredEntries();
    const logText = filteredEntries
      .map(entry => `[${entry.timestamp}] [${entry.level}] ${entry.message}`)
      .join('\n');
    
    const blob = new Blob([logText], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${nodeName}-${processName}-logs.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  };

  const getLogLevelColor = (level: string) => {
    switch (level.toUpperCase()) {
      case 'ERROR':
        return 'red';
      case 'WARN':
      case 'WARNING':
        return 'orange';
      case 'INFO':
        return 'blue';
      case 'DEBUG':
        return 'green';
      case 'TRACE':
        return 'purple';
      default:
        return 'default';
    }
  };

  const getFilteredEntries = () => {
    return logEntries.filter(entry => {
      const matchesSearch = !searchText || 
        entry.message.toLowerCase().includes(searchText.toLowerCase());
      const matchesLevel = !levelFilter || 
        entry.level.toLowerCase() === levelFilter.toLowerCase();
      return matchesSearch && matchesLevel;
    });
  };

  useEffect(() => {
    if (visible) {
      loadInitialLogs();
    } else {
      // Clean up when modal closes
      unsubscribeFromLogs();
      setRealTimeEnabled(false);
      setLogEntries([]);
    }
  }, [visible]);

  useEffect(() => {
    return () => {
      unsubscribeFromLogs();
    };
  }, []);

  const filteredEntries = getFilteredEntries();

  return (
    <Modal
      title={
        <Space>
          <span>{t.logViewer.title}: {processName} on {nodeName}</span>
          <Tag color={realTimeEnabled ? 'green' : 'default'}>
            {realTimeEnabled ? t.logViewer.live : t.logViewer.static}
          </Tag>
        </Space>
      }
      open={visible}
      onCancel={onClose}
      width={1000}
      footer={[
        <Button
          key="load-older"
          onClick={loadOlderLogs}
          loading={paging}
          icon={<History size={14} strokeWidth={1.7} />}
        >
          {t.logViewer.loadOlder !== undefined ? t.logViewer.loadOlder : 'Load older'}
        </Button>,
        <Button key="close" onClick={onClose}>
          {t.common.close}
        </Button>,
      ]}
    >
      <div style={{ marginBottom: 16 }}>
        <Space wrap>
          <Space>
            <span>{t.logViewer.realTime}:</span>
            <Switch
              checked={realTimeEnabled}
              onChange={toggleRealTime}
              disabled={!isConnected}
              checkedChildren={<PlayCircle size={14} strokeWidth={1.7} />}
              unCheckedChildren={<PauseCircle size={14} strokeWidth={1.7} />}
            />
          </Space>
          
          <Space>
            <span>{t.logViewer.autoScroll}:</span>
            <Switch
              checked={autoScroll}
              onChange={setAutoScroll}
              size="small"
            />
          </Space>

          <Search
            placeholder={t.logViewer.searchLogs}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 200 }}
            prefix={<SearchIcon size={14} strokeWidth={1.7} />}
            allowClear
          />

          <Select
            placeholder={t.logViewer.filterByLevel}
            value={levelFilter}
            onChange={setLevelFilter}
            style={{ width: 120 }}
            allowClear
          >
            <Option value="error">{t.logs.error}</Option>
            <Option value="warn">{t.logs.warn}</Option>
            <Option value="info">{t.logs.info}</Option>
            <Option value="debug">{t.logs.debug}</Option>
            <Option value="trace">Trace</Option>
          </Select>

          <Button
            icon={<Eraser size={14} strokeWidth={1.7} />}
            onClick={() => {
              Modal.confirm({
                title: t.logViewer.clearConfirmTitle || 'Clear process logs',
                content: t.logViewer.clearConfirmContent || 'This will permanently delete the stdout/stderr log files of this process on the node. Continue?',
                okText: t.common.confirm,
                cancelText: t.common.cancel,
                onOk: clearServerLogs,
              });
            }}
            size="small"
          >
            {t.logViewer.clear}
          </Button>

          <Button
            icon={<Download size={14} strokeWidth={1.7} />}
            onClick={exportLogs}
            size="small"
          >
            {t.logViewer.export}
          </Button>
        </Space>
      </div>

      <div
        ref={logContainerRef}
        style={{
          height: 500,
          overflow: 'auto',
          border: '1px solid var(--hairline-strong)',
          borderRadius: 12,
          padding: 12,
          backgroundColor: '#080a12',
          color: '#d6def5',
          fontFamily: 'var(--font-mono)',
          fontSize: 12,
        }}
      >
        {loading ? (
          <div style={{ textAlign: 'center', padding: 50 }}>
            <Spin size="large" />
          </div>
        ) : filteredEntries.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 50, color: 'var(--text-low)' }}>
            {t.logViewer.noLogEntries}
          </div>
        ) : (
          <div style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>
            {filteredEntries.map((entry, index) => (
              <div 
                key={index} 
                style={{ 
                  display: 'flex', 
                  gap: 8, 
                  padding: '4px 0',
                  alignItems: 'flex-start',
                  borderBottom: index < filteredEntries.length - 1 ? '1px solid rgba(255,255,255,0.04)' : 'none',
                  borderRadius: 6,
                }}
                onMouseEnter={(e) => (e.currentTarget.style.background = 'rgba(129,140,248,0.07)')}
                onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
              >
                <span 
                  style={{ 
                    color: '#5d6886',
                    width: 152,
                    minWidth: 152,
                    flexShrink: 0,
                    fontSize: 11,
                  }}
                >
                  {formatTimestamp(entry.timestamp)}
                </span>
                <Tag
                  color={getLogLevelColor(entry.level)}
                  style={{ width: 60, minWidth: 60, textAlign: 'center', flexShrink: 0, margin: 0 }}
                >
                  {entry.level.toUpperCase()}
                </Tag>
                <span style={{ flex: 1, wordBreak: 'break-word', color: '#d6def5' }}>
                  {entry.message}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      <div style={{ marginTop: 8, fontSize: '12px', color: '#666' }}>
        {t.logViewer.showingEntries.replace('{filtered}', String(filteredEntries.length)).replace('{total}', String(logEntries.length))}
        {realTimeEnabled && isConnected && (
          <span style={{ marginLeft: 16, color: '#52c41a' }}>
            ● {t.logViewer.connectedReceiving}
          </span>
        )}
      </div>
    </Modal>
  );
};

export default LogViewer;