import React, { useState, useEffect, useCallback } from 'react';
import { HistoryChart } from '../components/history/HistoryChart';
import { PeriodSelector } from '../components/history/PeriodSelector';
import { RegisterSelector } from '../components/history/RegisterSelector';
import { DatePickerWithRange } from '../components/ui/date-picker-range';
import { Button } from '../components/ui/button';
import { Sheet, SheetTrigger, SheetContent, SheetHeader, SheetTitle } from '../components/ui/sheet';
import { useHistory } from '../lib/hooks/useHistory';
import { Page } from '../components/layout/Page';
import { getDateRangeForPeriod } from '../lib/utils/date';
import { historyDataGroups } from '../lib/config/groups';
import { Menu, RefreshCw } from 'lucide-react';
import type { Period } from '../types';
import type { DateRange } from 'react-day-picker';
import { format, parseISO } from 'date-fns';
import { SURFACE } from '../components/ui/surface';
import { cn } from '../lib/utils';

export function History(): React.ReactElement {

  // Initialize with proper date range for daily period
  const initialRange = getDateRangeForPeriod('daily');
  const [period, setPeriod] = useState<Period>('daily');
  const [selectedIds, setSelectedIds] = useState<string[]>(['pv_energy_today']);
  const [startDate, setStartDate] = useState(initialRange.start);
  const [endDate, setEndDate] = useState(initialRange.end);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  
  const { data, isLoading, error, loadHistory, clearHistory } = useHistory();

  // The picker's range from the stored keys (yyyy, yyyy-MM or yyyy-MM-dd). parseISO reads
  // all three as local dates; new Date() would read them as UTC and show the previous
  // day west of UTC.
  const dateRange: DateRange | undefined = React.useMemo(
    () => (startDate && endDate ? { from: parseISO(startDate), to: parseISO(endDate) } : undefined),
    [startDate, endDate],
  );

  // Handle period change
  const handlePeriodChange = useCallback((newPeriod: Period) => {
    // Get valid IDs for the new period
    const validIdsForPeriod = historyDataGroups[newPeriod] || [];
    const firstValidId = validIdsForPeriod.length > 0 ? validIdsForPeriod[0] : '';
    
    // Get date range for the new period
    const range = getDateRangeForPeriod(newPeriod);
    
    const ids = firstValidId ? [firstValidId] : [];
    
    // Update all state at once
    setPeriod(newPeriod);
    setSelectedIds(ids);
    setStartDate(range.start);
    setEndDate(range.end);
  }, []);

  // Load history on mount and whenever period, dates or the selection change (one effect:
  // a separate mount effect fetched everything twice, review FE-L1).
  useEffect(() => {
    if (selectedIds.length === 0) {
      clearHistory(); // nothing selected: no stale chart (review FE-L2)
      return;
    }
    if (!startDate || !endDate) return;
    // Check if all selected IDs are valid for the current period
    const validIdsForPeriod = historyDataGroups[period] || [];
    if (selectedIds.every(id => validIdsForPeriod.includes(id))) {
      loadHistory(selectedIds, startDate, endDate);
    }
  }, [startDate, endDate, selectedIds, period, loadHistory, clearHistory]);

  const handleToggle = (id: string) => {
    setSelectedIds(prev => 
      prev.includes(id) 
        ? prev.filter(x => x !== id) 
        : [...prev, id]
    );
  };

  const handleRefresh = useCallback(() => {
    if (startDate && endDate && selectedIds.length > 0) {
      loadHistory(selectedIds, startDate, endDate);
    }
  }, [startDate, endDate, selectedIds, loadHistory]);

  const handleDateRangeChange = useCallback((range: DateRange | undefined) => {
    if (range?.from && range?.to) {
      if (period === 'yearly') {
        setStartDate(`${range.from.getFullYear()}`);
        setEndDate(`${range.to.getFullYear()}`);
      } else if (period === 'monthly') {
        setStartDate(`${range.from.getFullYear()}-${String(range.from.getMonth() + 1).padStart(2, '0')}`);
        setEndDate(`${range.to.getFullYear()}-${String(range.to.getMonth() + 1).padStart(2, '0')}`);
      } else {
        // Local calendar day: toISOString() would shift to the previous day east of UTC.
        setStartDate(format(range.from, 'yyyy-MM-dd'));
        setEndDate(format(range.to, 'yyyy-MM-dd'));
      }
    }
  }, [period]);

  const refreshButton = (
    <Button
      variant="ghost"
      size="icon"
      onClick={handleRefresh}
      disabled={isLoading || selectedIds.length === 0}
      className="h-9 w-9"
      aria-label="Refresh data"
    >
      <RefreshCw className="h-4 w-4" />
    </Button>
  );

  // The same three filters in the desktop sidebar and the mobile sheet.
  const filters = (headingClass: string) => (
    <>
      <div className="space-y-1">
        <h3 className={headingClass}>Time Period</h3>
        <PeriodSelector period={period} onPeriodChange={handlePeriodChange} />
      </div>
      <div className="space-y-1">
        <h3 className={headingClass}>Date Range</h3>
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
        <RegisterSelector selectedIds={selectedIds} onToggle={handleToggle} period={period} />
      </div>
    </>
  );

  return (
    <Page
      title="History"
      actions={
        <div className="flex items-center gap-2">
          <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
            <SheetTrigger asChild>
              <Button variant="outline" size="sm" className="gap-2 lg:hidden">
                <Menu className="h-4 w-4" />
                <span>Filters</span>
              </Button>
            </SheetTrigger>
            <SheetContent side="left" className="p-0 w-full max-w-xs h-full">
              <SheetHeader className="p-4 pb-2 border-b border-border">
                <SheetTitle>History Filters</SheetTitle>
              </SheetHeader>
              <div className="flex flex-col h-full bg-background">
                <div className="flex-1 p-4 space-y-4 overflow-y-auto">
                  {filters('text-sm font-medium text-muted-foreground mb-2')}
                </div>
                <div className="p-4 pt-2 border-t border-border bg-background">
                  <Button onClick={() => setMobileSidebarOpen(false)} className="w-full">
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
