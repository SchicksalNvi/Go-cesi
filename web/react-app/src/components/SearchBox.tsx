import React, { useState, useEffect } from 'react';
import { Input } from 'antd';
import { Search } from 'lucide-react';

interface SearchBoxProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  debounceMs?: number;
  allowClear?: boolean;
  size?: 'small' | 'middle' | 'large';
  style?: React.CSSProperties;
}

export const SearchBox: React.FC<SearchBoxProps> = ({
  value,
  onChange,
  placeholder = 'Search nodes...',
  debounceMs = 300,
  allowClear = true,
  size = 'middle',
  style
}) => {
  const [localValue, setLocalValue] = useState(value);

  // Sync with external value changes
  useEffect(() => {
    setLocalValue(value);
  }, [value]);

  // Debounced onChange
  useEffect(() => {
    const timer = setTimeout(() => {
      if (localValue !== value) {
        onChange(localValue);
      }
    }, debounceMs);

    return () => clearTimeout(timer);
  }, [localValue, onChange, value, debounceMs]);

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setLocalValue(e.target.value);
  };

  const handleClear = () => {
    setLocalValue('');
    onChange('');
  };

  return (
    <Input
      prefix={<Search size={14} strokeWidth={1.7} />}
      placeholder={placeholder}
      value={localValue}
      onChange={handleChange}
      allowClear={allowClear}
      onClear={handleClear}
      size={size}
      style={style}
      aria-label="Search nodes"
      role="searchbox"
      aria-describedby="search-help"
    />
  );
};

export default SearchBox;