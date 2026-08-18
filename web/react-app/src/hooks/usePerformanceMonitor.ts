import { useEffect, useRef, useState } from 'react';

interface PerformanceMetrics {
  renderTime: number;
  memoryUsage?: number;
  componentCount: number;
}

/**
 * Hook for monitoring component performance
 * @param componentName - Name of the component for logging
 * @param enabled - Whether monitoring is enabled
 */
export function usePerformanceMonitor(componentName: string, enabled: boolean = false) {
  const renderStartTime = useRef<number>(0);
  // Metrics data lives in a ref so updating it never triggers a re-render;
  // state is only updated when the measured values actually change.
  const metricsRef = useRef<PerformanceMetrics>({
    renderTime: 0,
    componentCount: 0,
  });
  const [metrics, setMetrics] = useState<PerformanceMetrics>({
    renderTime: 0,
    componentCount: 0,
  });

  useEffect(() => {
    if (!enabled) return;

    renderStartTime.current = performance.now();
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;

    const renderTime = performance.now() - renderStartTime.current;
    
    // Get memory usage if available
    const memoryUsage = (performance as any).memory?.usedJSHeapSize;

    const nextMetrics: PerformanceMetrics = {
      renderTime,
      memoryUsage,
      componentCount: metricsRef.current.componentCount + 1,
    };

    const prev = metricsRef.current;
    metricsRef.current = nextMetrics;

    // Only update state when the values actually change. Without this guard
    // (and without a dependency array), every setMetrics call re-renders the
    // component, which re-runs this effect, causing an infinite render loop.
    if (
      prev.renderTime !== nextMetrics.renderTime ||
      prev.memoryUsage !== nextMetrics.memoryUsage ||
      prev.componentCount !== nextMetrics.componentCount
    ) {
      setMetrics(nextMetrics);
    }

    // Log performance metrics in development
    if (process.env.NODE_ENV === 'development') {
      console.log(`[Performance] ${componentName}:`, {
        renderTime: `${renderTime.toFixed(2)}ms`,
        memoryUsage: memoryUsage ? `${(memoryUsage / 1024 / 1024).toFixed(2)}MB` : 'N/A',
        componentCount: nextMetrics.componentCount,
      });

      // Warn about slow renders
      if (renderTime > 100) {
        console.warn(`[Performance Warning] ${componentName} took ${renderTime.toFixed(2)}ms to render`);
      }
    }
  }, [enabled, componentName]);

  return metrics;
}

/**
 * Hook for monitoring list performance with large datasets
 * @param itemCount - Number of items in the list
 * @param threshold - Threshold for performance warnings
 */
export function useListPerformance(itemCount: number, threshold: number = 1000) {
  const [shouldOptimize, setShouldOptimize] = useState(false);
  const [recommendations, setRecommendations] = useState<string[]>([]);

  useEffect(() => {
    const optimize = itemCount >= threshold;
    setShouldOptimize(optimize);

    const newRecommendations: string[] = [];

    if (itemCount >= 100) {
      newRecommendations.push('Consider using virtualization for better performance');
    }

    if (itemCount >= 500) {
      newRecommendations.push('Enable pagination to reduce initial load time');
    }

    if (itemCount >= 1000) {
      newRecommendations.push('Implement server-side filtering and pagination');
    }

    setRecommendations(newRecommendations);
  }, [itemCount, threshold]);

  return {
    shouldOptimize,
    recommendations,
    itemCount,
  };
}

/**
 * Hook for debouncing expensive operations
 * @param callback - Function to debounce
 * @param delay - Delay in milliseconds
 * @param deps - Dependencies array
 */
export function useDebounceCallback<T extends (...args: any[]) => any>(
  callback: T,
  delay: number,
  deps: React.DependencyList
) {
  const timeoutRef = useRef<NodeJS.Timeout>();

  const debouncedCallback = useRef(callback);
  debouncedCallback.current = callback;

  useEffect(() => {
    return () => {
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  const debouncedFunction = (...args: Parameters<T>) => {
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
    }

    timeoutRef.current = setTimeout(() => {
      debouncedCallback.current(...args);
    }, delay);
  };

  return debouncedFunction as T;
}

export default usePerformanceMonitor;