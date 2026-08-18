import React from 'react';
import { Button, Space } from 'antd';
import { LayoutGrid, List } from 'lucide-react';

export type ViewMode = 'card' | 'list';

interface ViewToggleProps {
  value: ViewMode;
  onChange: (mode: ViewMode) => void;
  disabled?: boolean;
  size?: 'small' | 'middle' | 'large';
}

export const ViewToggle: React.FC<ViewToggleProps> = ({
  value,
  onChange,
  disabled = false,
  size = 'middle'
}) => {
  return (
    <Space.Compact size={size} role="radiogroup" aria-label="View mode selection">
      <Button
        type={value === 'card' ? 'primary' : 'default'}
        icon={<LayoutGrid size={14} strokeWidth={1.7} />}
        onClick={() => onChange('card')}
        disabled={disabled}
        role="radio"
        aria-checked={value === 'card'}
        aria-label="Card view"
      >
        Cards
      </Button>
      <Button
        type={value === 'list' ? 'primary' : 'default'}
        icon={<List size={14} strokeWidth={1.7} />}
        onClick={() => onChange('list')}
        disabled={disabled}
        role="radio"
        aria-checked={value === 'list'}
        aria-label="List view"
      >
        List
      </Button>
    </Space.Compact>
  );
};

export default ViewToggle;