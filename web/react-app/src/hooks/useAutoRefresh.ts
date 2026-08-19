import { useEffect, useRef } from 'react';
import { useStore } from '@/store';

/**
 * 检测页面上是否有打开的 antd Modal / Drawer 遮罩层。
 * antd Modal 渲染 .ant-modal-wrap(显示中)、Drawer 渲染 .ant-drawer-open。
 * 用于在弹窗(如日志查看器)打开期间暂停自动刷新。
 */
function hasOpenOverlay(): boolean {
  if (typeof document === 'undefined') return false;
  return (
    document.querySelector('.ant-modal-wrap:not([style*="display: none"])') !== null ||
    document.querySelector('.ant-modal-wrap[style*="display: block"]') !== null ||
    document.querySelector('.ant-drawer-open') !== null
  );
}

/**
 * 自动刷新 Hook
 * 从全局 store 读取 autoRefreshEnabled 和 refreshInterval
 *
 * 在检测到用户近期交互(鼠标/键盘/触摸)时暂停当次刷新,避免打断
 * 正在进行的操作(如查看日志弹窗、下拉展开、滚动列表等)。
 *
 * @param callback 要执行的回调函数
 * @param options 可选: 配置
 *   - pauseAfterInteractionMs: 距离最近一次用户交互多少毫秒内视为"活动、跳过刷新",
 *     默认 15000ms。打开 Modal 查看日志时,持续交互会自然延长抑制窗口。
 *   - interactionEvents: 视为用户交互的浏览器事件,默认捕获鼠标/键盘/触摸活动。
 */
export function useAutoRefresh(
  callback: () => void | Promise<void>,
  options?: {
    pauseAfterInteractionMs?: number;
    interactionEvents?: string[];
  }
) {
  const { autoRefreshEnabled, refreshInterval } = useStore();
  const callbackRef = useRef(callback);
  const lastInteractionRef = useRef<number>(Date.now());
  const pauseMs = options?.pauseAfterInteractionMs ?? 15000;
  const events = options?.interactionEvents ?? [
    'mousemove',
    'mousedown',
    'mouseup',
    'wheel',
    'keydown',
    'touchstart',
    'touchmove',
  ];

  useEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  useEffect(() => {
    if (!autoRefreshEnabled || refreshInterval <= 0) return;

    // 记录用户交互时刻;鼠标move会高频触发,做节流避免频繁写。
    let moveThrottle = 0;
    const onInteraction = (e: Event) => {
      if (e.type === 'mousemove' || e.type === 'touchmove') {
        const now = Date.now();
        if (now - moveThrottle < 800) return;
        moveThrottle = now;
      }
      lastInteractionRef.current = Date.now();
    };

    for (const evt of events) {
      window.addEventListener(evt, onInteraction, { passive: true });
    }

    const id = setInterval(() => {
      // 若用户在此前 pauseMs 毫秒内有交互(正在操作页面),跳过本次刷新,
      // 避免打断查看日志等操作。
      if (Date.now() - lastInteractionRef.current < pauseMs) {
        return;
      }

      // 若有 antd Modal / Drawer 打开(如查看日志弹窗、批量操作确认框、
      // 用户/环境详情抽屉),暂停自动刷新,避免打断弹窗内的阅读与操作。
      if (hasOpenOverlay()) {
        return;
      }

      void callbackRef.current();
    }, refreshInterval * 1000);

    return () => {
      clearInterval(id);
      for (const evt of events) {
        window.removeEventListener(evt, onInteraction);
      }
    };
  }, [autoRefreshEnabled, refreshInterval, events.join('|'), pauseMs]);
}
