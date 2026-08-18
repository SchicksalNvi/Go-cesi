import React from 'react';
import { Space, Select, Button, Tag, InputNumber } from 'antd';
import { Eraser } from 'lucide-react';

export interface NodeFilters {
  status?: 'online' | 'offline';
  environment?: string;
  processCountRange?: [number, number];
}

interface FilterBarProps {
  filters: NodeFilters;
  onFiltersChange: (filters: NodeFilters) => void;
  environments: string[];
  totalNodes: number;
  filteredNodes: number;
  size?: 'small' | 'middle' | 'large';
}

export const FilterBar: React.FC<FilterBarProps> = ({
  filters,
  onFiltersChange,
  environments,
  totalNodes,
  filteredNodes,
  size = 'middle'
}) => {
  const updateFilter = (key: keyof NodeFilters, value: any) => {
    const newFilters = { ...filters };
    if (value === undefined || value === null || value === '') {
      delete newFilters[key];
    } else {
      newFilters[key] = value;
    }
    onFiltersChange(newFilters);
  };

  const clearAllFilters = () => {
    onFiltersChange({});
  };

  const hasActiveFilters = Object.keys(filters).length > 0;
  const isFiltered = filteredNodes !== totalNodes;

  return (
    <div 
      style={{ display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' }}
      role="region"
      aria-label="Node filters"
    >
      <Space size="middle" wrap>
        {/* Status Filter */}
        <div>
          <span style={{ marginRight: 8, fontSize: 14, color: '#666' }} id="status-filter-label">
            Status:
          </span>
          <Select
            placeholder="All"
            value={filters.status}
            onChange={(value) => updateFilter('status', value)}
            allowClear
            size={size}
            style={{ minWidth: 100 }}
            aria-labelledby="status-filter-label"
            aria-label="Filter by node status"
          >
            <Select.Option value="online">Online</Select.Option>
            <Select.Option value="offline">Offline</Select.Option>
          </Select>
        </div>

        {/* Environment Filter */}
        <div>
          <span style={{ marginRight: 8, fontSize: 14, color: '#666' }} id="environment-filter-label">
            Environment:
          </span>
          <Select
            placeholder="All"
            value={filters.environment}
            onChange={(value) => updateFilter('environment', value)}
            allowClear
            size={size}
            style={{ minWidth: 120 }}
            aria-labelledby="environment-filter-label"
            aria-label="Filter by environment"
          >
            {environments.map(env => (
              <Select.Option key={env} value={env}>{env}</Select.Option>
            ))}
          </Select>
        </div>

        {/* Process Count Range Filter */}
        <div>
          <span style={{ marginRight: 8, fontSize: 14, color: '#666' }} id="process-filter-label">
            Processes:
          </span>
          <Space.Compact>
            <InputNumber
              placeholder="Min"
              value={filters.processCountRange?.[0]}
              onChange={(value) => {
                const current = filters.processCountRange || [undefined, undefined];
                updateFilter('processCountRange', [value, current[1]]);
              }}
              size={size}
              style={{ width: 80 }}
              min={0}
              aria-label="Minimum process count"
            />
            <InputNumber
              placeholder="Max"
              value={filters.processCountRange?.[1]}
              onChange={(value) => {
                const current = filters.processCountRange || [undefined, undefined];
                updateFilter('processCountRange', [current[0], value]);
              }}
              size={size}
              style={{ width: 80 }}
              min={0}
              aria-label="Maximum process count"
            />
          </Space.Compact>
        </div>

        {/* Clear All Button */}
        {hasActiveFilters && (
          <Button
            type="text"
            icon={<Eraser size={14} strokeWidth={1.7} />}
            onClick={clearAllFilters}
            size={size}
            aria-label="Clear all filters"
          >
            Clear All
          </Button>
        )}
      </Space>

      {/* Results Count */}
      {isFiltered && (
        <div 
          style={{ marginLeft: 'auto', fontSize: 14, color: '#666' }}
          aria-live="polite"
          aria-label={`Showing ${filteredNodes} of ${totalNodes} nodes`}
        >
          Showing {filteredNodes} of {totalNodes} nodes
        </div>
      )}

      {/* Active Filter Tags */}
      {hasActiveFilters && (
        <div style={{ width: '100%', marginTop: 8 }} role="region" aria-label="Active filters">
          <Space size={4} wrap>
            {filters.status && (
              <Tag
                closable
                onClose={() => updateFilter('status', undefined)}
                color="blue"
                aria-label={`Remove status filter: ${filters.status}`}
              >
                Status: {filters.status}
              </Tag>
            )}
            {filters.environment && (
              <Tag
                closable
                onClose={() => updateFilter('environment', undefined)}
                color="green"
                aria-label={`Remove environment filter: ${filters.environment}`}
              >
                Environment: {filters.environment}
              </Tag>
            )}
            {filters.processCountRange && (
              <Tag
                closable
                onClose={() => updateFilter('processCountRange', undefined)}
                color="orange"
                aria-label={`Remove process count filter: ${filters.processCountRange[0] || 0}-${filters.processCountRange[1] || '∞'}`}
              >
                Processes: {filters.processCountRange[0] || 0}-{filters.processCountRange[1] || '∞'}
              </Tag>
            )}
          </Space>
        </div>
      )}
    </div>
  );
};

export default FilterBar;