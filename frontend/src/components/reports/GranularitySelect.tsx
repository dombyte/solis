import { useState } from 'react';
import { Check, ChevronDown } from 'lucide-react';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import type { Granularity } from '../../lib/api/report';
import { cn } from '../../lib/utils';
import { MENU_CLASS, OPTION_CLASS } from './styles';

const OPTIONS: { id: Granularity; label: string }[] = [
  { id: 'day', label: 'Per day' },
  { id: 'month', label: 'Per month' },
  { id: 'year', label: 'Per year' },
];

interface GranularitySelectProps {
  id?: string;
  value: Granularity;
  onChange: (value: Granularity) => void;
  className?: string;
}

/**
 * Row grouping of the CSV export. A popover instead of a native <select>, so the list
 * uses the same glass menu and option style as the period and compare dropdowns.
 */
export function GranularitySelect({ id, value, onChange, className }: GranularitySelectProps) {
  const [open, setOpen] = useState(false);
  const current = OPTIONS.find(o => o.id === value) ?? OPTIONS[0];

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          id={id}
          type="button"
          className={cn(
            'h-11 flex items-center justify-between gap-2 rounded-lg border border-border bg-control px-3 text-sm',
            'hover:bg-control-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
            'data-[state=open]:border-primary',
            className,
          )}
        >
          <span className="truncate">{current.label}</span>
          <ChevronDown aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="start"
        collisionPadding={12}
        className={cn(MENU_CLASS, 'p-2 w-[max(var(--radix-popover-trigger-width),10rem)]')}
      >
        <div role="group" aria-label="CSV rows" className="flex flex-col gap-0.5">
          {OPTIONS.map(o => {
            const active = o.id === value;
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
                <span className={cn(active && 'font-semibold')}>{o.label}</span>
              </button>
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}
