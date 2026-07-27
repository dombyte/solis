import React from 'react';
import { Button } from '../ui/button';
import type { Period } from '../../types';

interface PeriodSelectorProps {
  period: Period;
  onPeriodChange: (period: Period) => void;
  className?: string;
}

const periods: { value: Period; label: string }[] = [
  { value: 'daily', label: 'Daily' },
  { value: 'monthly', label: 'Monthly' },
  { value: 'yearly', label: 'Yearly' },
];

export function PeriodSelector({ 
  period, 
  onPeriodChange, 
  className = '' 
}: PeriodSelectorProps): React.ReactElement {
  return (
    <div className={`flex gap-2 ${className}`}>
      {periods.map((p) => (
        <Button
          key={p.value}
          variant={period === p.value ? 'default' : 'ghost'}
          size="sm"
          onClick={() => onPeriodChange(p.value)}
          className="flex-1 min-h-[40px] touch-target"
        >
          {p.label}
        </Button>
      ))}
    </div>
  );
}
