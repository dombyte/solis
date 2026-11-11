import { useState } from 'react';
import { format, parseISO } from 'date-fns';
import type { DateRange } from 'react-day-picker';
import { CalendarIcon, Check, ChevronDown } from 'lucide-react';
import { Button } from '../ui/button';
import { Calendar } from '../ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '../ui/popover';
import { useMobile } from '../../hooks/useMobile';
import {
  PRESET_LABEL, REPORT_PRESETS, formatRange, presetOf, presetRange, type DayRange,
} from '../../lib/utils/reportRange';
import { cn } from '../../lib/utils';
import { MENU_CLASS, OPTION_CLASS, TRIGGER_CLASS } from './styles';

interface PeriodPickerProps {
  range: DayRange;
  today: Date;
  onChange: (range: DayRange) => void;
}

/** The report period: a preset (with its dates) or a custom range from the calendar. */
export function PeriodPicker({ range, today, onChange }: PeriodPickerProps) {
  const isMobile = useMobile();
  const [open, setOpen] = useState(false);
  const [custom, setCustom] = useState<DateRange | undefined>();
  const preset = presetOf(range, today);

  const toggle = (next: boolean) => {
    if (next) setCustom({ from: parseISO(range.from), to: parseISO(range.to) });
    setOpen(next);
  };
  const choose = (r: DayRange) => {
    onChange(r);
    setOpen(false);
  };
  const customRange: DayRange | null = custom?.from && custom.to
    ? { from: format(custom.from, 'yyyy-MM-dd'), to: format(custom.to, 'yyyy-MM-dd') }
    : null;

  return (
    <Popover open={open} onOpenChange={toggle}>
      <PopoverTrigger asChild>
        <button type="button" className={TRIGGER_CLASS} aria-label={`Report period: ${PRESET_LABEL[preset]}, ${formatRange(range)}`}>
          <CalendarIcon aria-hidden="true" className="size-5 shrink-0 text-muted-foreground" />
          <span className="flex flex-col min-w-0 flex-1">
            <span className="text-xs text-muted-foreground">Period</span>
            <span className="font-semibold truncate">{PRESET_LABEL[preset]}</span>
            <span className="text-muted-foreground truncate">{formatRange(range)}</span>
          </span>
          <ChevronDown aria-hidden="true" className="size-4 shrink-0 text-muted-foreground" />
        </button>
      </PopoverTrigger>
      <PopoverContent align="start" className={cn(MENU_CLASS, 'w-auto p-0')}>
        <div className="flex flex-col md:flex-row">
          <div role="group" aria-label="Period presets" className="p-2 md:w-64 flex flex-col gap-0.5 md:border-r border-border">
            {REPORT_PRESETS.map(p => {
              const r = presetRange(p.id, today);
              const active = preset === p.id;
              return (
                <button key={p.id} type="button" aria-pressed={active} onClick={() => choose(r)} className={OPTION_CLASS}>
                  <Check aria-hidden="true" className={cn('size-4 shrink-0', !active && 'invisible')} />
                  <span className="flex flex-col">
                    <span className={cn(active && 'font-semibold')}>{p.label}</span>
                    <span className="text-xs text-muted-foreground">{formatRange(r)}</span>
                  </span>
                </button>
              );
            })}
          </div>
          <div className="p-3 border-t md:border-t-0 border-border flex flex-col gap-3">
            <p className="text-sm font-semibold">Custom range</p>
            <Calendar
              mode="range"
              selected={custom}
              onSelect={setCustom}
              defaultMonth={parseISO(range.from)}
              numberOfMonths={isMobile ? 1 : 2}
              viewMode="daily"
            />
            <Button type="button" disabled={!customRange} onClick={() => customRange && choose(customRange)}>
              <Check />
              {customRange ? `Use ${formatRange(customRange)}` : 'Pick a start and an end day'}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
