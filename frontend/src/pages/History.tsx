import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { HistoryChart } from '../components/history/HistoryChart';
import { PeriodSelector } from '../components/history/PeriodSelector';
import { RangePresets } from '../components/history/RangePresets';
import { RegisterSelector } from '../components/history/RegisterSelector';
import { DatePickerWithRange } from '../components/ui/date-picker-range';
import { Button } from '../components/ui/button';
import { Sheet, SheetTrigger, SheetContent, SheetHeader, SheetTitle } from '../components/ui/sheet';
import { useHistory } from '../lib/hooks/useHistory';
import { useHistoryPrefs } from '../lib/hooks/useHistoryPrefs';
import { Page } from '../components/layout/Page';
import { rangeFromDates, resolveRange } from '../lib/utils/historyRange';
import { Menu, RefreshCw, RotateCcw } from 'lucide-react';
import type { DateRange } from 'react-day-picker';
import { parseISO } from 'date-fns';
import { SURFACE } from '../components/ui/surface';
import { cn } from '../lib/utils';

export function History(): React.ReactElement {
  // Period, series and range are remembered in this browser (useHistoryPrefs).
  const { prefs, isDefault, setPeriod, toggleSeries, setRange, reset } = useHistoryPrefs();
  const { period } = prefs;
  const selectedIds = prefs.series[period];
  const rangeChoice = prefs.range[period];
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);

  const { data, isLoading, error, loadHistory, clearHistory } = useHistory();

  // Presets resolve against today on every load, so "last 30 days" moves with the calendar.
  const { start: startDate, end: endDate } = useMemo(
    () => resolveRange(rangeChoice, period, new Date()),
    [rangeChoice, period],
  );
  const activePreset = 'preset' in rangeChoice ? rangeChoice.preset : undefined;

  // The picker's range from the stored keys (yyyy, yyyy-MM or yyyy-MM-dd). parseISO reads
  // all three as local dates; new Date() would read them as UTC and show the previous
  // day west of UTC.
  const dateRange: DateRange = useMemo(
    () => ({ from: parseISO(startDate), to: parseISO(endDate) }),
    [startDate, endDate],
  );

  // Load history on mount and whenever period, dates or the selection change (one effect:
  // a separate mount effect fetched everything twice, review FE-L1). The selection is kept
  // per period and validated on load, so it always belongs to the current period.
  useEffect(() => {
    if (selectedIds.length === 0) {
      clearHistory(); // nothing selected: no stale chart (review FE-L2)
      return;
    }
    loadHistory(selectedIds, startDate, endDate);
  }, [startDate, endDate, selectedIds, loadHistory, clearHistory]);

  const handleRefresh = useCallback(() => {
    if (selectedIds.length > 0) loadHistory(selectedIds, startDate, endDate);
  }, [startDate, endDate, selectedIds, loadHistory]);

  // A calendar pick is a fixed range: no preset is highlighted until one is chosen again.
  const handleDateRangeChange = useCallback((range: DateRange | undefined) => {
    if (range?.from && range?.to) setRange({ custom: rangeFromDates(range.from, range.to, period) });
  }, [period, setRange]);

  const refreshButton = (
    <Button
      variant="ghost"
      size="icon"
      onClick={handleRefresh}
      disabled={isLoading || selectedIds.length === 0}
      className="h-9 w-9 ml-auto"
      aria-label="Refresh data"
    >
      <RefreshCw className="h-4 w-4" />
    </Button>
  );

  // Part of the filters (not a header icon next to refresh, where it read as a second
  // reload button): bottom of the desktop sidebar, next to "Done" in the phone sheet.
  const resetButton = (className: string) => (
    <Button
      variant="outline"
      onClick={reset}
      disabled={isDefault}
      className={cn('gap-2', className)}
    >
      <RotateCcw className="h-4 w-4" aria-hidden="true" />
      Reset filters
    </Button>
  );

  // The same three filters in the desktop sidebar and the mobile sheet.
  const filters = (headingClass: string) => (
    <>
      <div className="space-y-1">
        <h3 className={headingClass}>Time Period</h3>
        <PeriodSelector period={period} onPeriodChange={setPeriod} />
      </div>
      <div className="space-y-1">
        <h3 className={headingClass}>Date Range</h3>
        <RangePresets
          period={period}
          active={activePreset}
          onSelect={preset => setRange({ preset })}
        />
        <DatePickerWithRange
          date={dateRange}
          setDate={handleDateRangeChange}
          label="Date Range"
          className="w-full"
          period={period}
          hideLabel={true}
        />
      </div>
      <div className="space-y-1">
        <h3 className={headingClass}>Data Series</h3>
        <RegisterSelector selectedIds={selectedIds} onToggle={toggleSeries} period={period} />
      </div>
    </>
  );

  return (
    <Page
      title="History"
      actions={
        // flex-1: the Filters button sits on the left (phones/tablets, where the title is
        // in the mobile header), refresh stays on the right.
        <div className="flex flex-1 items-center justify-between gap-2">
          <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
            <SheetTrigger asChild>
              <Button variant="outline" size="sm" className="gap-2 lg:hidden">
                <Menu className="h-4 w-4" />
                <span>Filters</span>
              </Button>
            </SheetTrigger>
            {/* Column layout: the list scrolls between header and footer. The body used to
                be h-full below the header, which pushed the Reset/Done footer off screen.
                Full width on phones ("Done" closes it, the backdrop strip only cost space);
                a side panel from `sm` up. */}
            <SheetContent side="left" className="p-0 gap-0 w-full max-w-none sm:max-w-md border-r-0 sm:border-r h-dvh flex flex-col">
              <SheetHeader className="p-4 pb-2 border-b border-border">
                <SheetTitle>History Filters</SheetTitle>
              </SheetHeader>
              <div className="flex flex-col flex-1 min-h-0 bg-background">
                <div className="flex-1 p-4 space-y-4 overflow-y-auto">
                  {filters('text-sm font-medium text-muted-foreground mb-2')}
                </div>
                <div className="p-4 pt-3 pb-[max(1rem,env(safe-area-inset-bottom))] border-t border-border bg-background flex gap-2">
                  {resetButton('flex-1')}
                  <Button onClick={() => setMobileSidebarOpen(false)} className="flex-1">
                    Done
                  </Button>
                </div>
              </div>
            </SheetContent>
          </Sheet>
          {refreshButton}
        </div>
      }
    >
      <div className="flex flex-col lg:flex-row gap-3 sm:gap-4 min-w-0">
        {/* Desktop sidebar; phones and tablets use the Filters sheet */}
        <div className="hidden lg:flex lg:flex-col gap-3 sm:gap-4 w-full max-w-[16rem] min-w-0 flex-shrink-0">
          {filters('text-sm font-medium text-muted-foreground mb-1')}
          {resetButton('w-full')}
        </div>

        <div className="flex-1 min-w-0">
          {error && (
            <div className="p-3 sm:p-4 bg-destructive/10 text-destructive rounded mb-3 sm:mb-4">
              {error}
            </div>
          )}
          <div className={cn(SURFACE, "p-3 sm:p-4 overflow-hidden")}>
            {isLoading && !data ? (
              <div className="flex items-center justify-center h-64 sm:h-80 lg:h-96">
                <div className="flex flex-col items-center gap-3 sm:gap-4">
                  <div className="animate-spin h-6 w-6 sm:h-8 sm:w-8 border-4 border-primary border-t-transparent rounded-full" />
                  <span className="text-muted-foreground">Loading history data...</span>
                </div>
              </div>
            ) : (
              <HistoryChart data={data} datasetCount={selectedIds.length} />
            )}
          </div>
        </div>
      </div>
    </Page>
  );
}
