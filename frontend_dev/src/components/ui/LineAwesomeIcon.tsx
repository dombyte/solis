import React from 'react';

interface LineAwesomeIconProps {
  icon: string;
  className?: string;
  size?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | number;
  style?: 'solid' | 'regular' | 'light' | 'duotone' | 'thin' | 'brand';
}

const sizeClasses: Record<string, string> = {
  sm: 'text-sm',
  md: 'text-base',
  lg: 'text-lg',
  xl: 'text-xl',
  '2xl': 'text-2xl',
};

const stylePrefixes: Record<string, string> = {
  solid: 'las',
  regular: 'lar',
  light: 'lal',
  duotone: 'lad',
  thin: 'lat',
  brand: 'lab',
};

export const LineAwesomeIcon: React.FC<LineAwesomeIconProps> = ({
  icon,
  className = '',
  size = 'md',
  style = 'solid',
}) => {
  const prefix = stylePrefixes[style] || 'las';
  const sizeClass = typeof size === 'number' ? `text-[${size}px]` : (sizeClasses[size] || size);

  return (
    <i
      className={`${prefix} ${icon} ${sizeClass} ${className}`}
      aria-hidden="true"
    />
  );
};
