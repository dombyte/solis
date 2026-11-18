import { useState } from 'react';
import { Check, ChevronDown, GitCompareArrows } from 'lucide-react';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import type { CompareMode } from '../../lib/api/report';
import { compareRange, formatRange, type DayRange } from '../../lib/utils/reportRange';
import { cn } from '../../lib/utils';
import { useCompactLayout } from '../../hooks/useMobile';
import { MENU_CLASS, OPTION_CLASS, TOUCH_MENU_CLASS, TRIGGER_CLASS } from './styles';

const OPTIONS: { id: CompareMode; label: string }[] = [
  { id: 'last_year', label: 'Same period last year' },
  { id: 'previous', label: 'Previous period' },
  { id: 'none', label: 'No comparison' },
];

interface CompareSelectProps {
  mode: CompareMode;
  range: DayRange;
  onChange: (mode: CompareMode) => void;
}

/** What the report period is compared with; every option shows its actual dates. */
export function CompareSelect({ mode, range, onChange }: CompareSelectProps) {
  // Compact = touch or narrow (same rule as the app shell and the date picker).
  const isMobile = useCompactLayout();
  const [open, setOpen] = useState(false);
  const current = OPTIONS.find(o => o.id === mode) ?? OPTIONS[0];
  const currentRange = compareRange(mode, range);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          className={TRIGGER_CLASS}
          aria-label={`Compare with: ${current.label}${currentRange ? `, ${formatRange(currentRange)}` : ''}`}
        >
          <GitCompareArrows aria-hidden="true" className="size-5 shrink-0 text-muted-foreground" />
          <span className="flex flex-col min-w-0 flex-1">
            <span className="text-xs text-muted-foreground">Compare with</span>
            <span className="font-semibold truncate">{current.label}</span>
            <span className="text-muted-foreground truncate">
              {currentRange ? formatRange(currentRange) : 'Show the period on its own'}
            </span>
          </span>
          <ChevronDown aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" collisionPadding={12} className={cn(MENU_CLASS, 'p-2', isMobile ? TOUCH_MENU_CLASS : 'w-72')}>
        <div role="group" aria-label="Compare with" className="flex flex-col gap-0.5">
          {OPTIONS.map(o => {
            const r = compareRange(o.id, range);
            const active = o.id === mode;
            return (
              <button
                key={o.id}
                type="button"
                aria-pressed={active}
                onClick={() => {
                  onChange(o.id);
                  setOpen(false);
                }}
                className={OPTION_CLASS}
              >
                <Check aria-hidden="true" className={cn('size-4 shrink-0', !active && 'invisible')} />
                <span className="flex flex-col">
                  <span className={cn(active && 'font-semibold')}>{o.label}</span>
                  <span className="text-xs text-muted-foreground">{r ? formatRange(r) : 'Show the period on its own'}</span>
                </span>
              </button>
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}
