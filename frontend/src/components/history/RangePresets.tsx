import React from 'react';
import { Button } from '../ui/button';
import { HISTORY_PRESETS, type HistoryPreset } from '../../lib/utils/historyRange';
import type { Period } from '../../types';

interface RangePresetsProps {
  period: Period;
  /** The active preset; undefined for a custom range (no button highlighted). */
  active: HistoryPreset | undefined;
  onSelect: (preset: HistoryPreset) => void;
}

/** Quick range buttons for the current period, above the calendar picker. */
export function RangePresets({ period, active, onSelect }: RangePresetsProps): React.ReactElement {
  return (
    <div className="flex flex-wrap gap-1.5 pb-1" role="group" aria-label="Date range presets">
      {HISTORY_PRESETS[period].map(p => (
        <Button
          key={p.id}
          variant={active === p.id ? 'default' : 'outline'}
          size="sm"
          onClick={() => onSelect(p.id)}
          aria-pressed={active === p.id}
          className="h-8 px-2.5 text-xs"
        >
          {p.label}
        </Button>
      ))}
    </div>
  );
}
