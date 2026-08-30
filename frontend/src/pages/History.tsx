import React, { useState, useEffect, useCallback } from 'react';
import { HistoryChart } from '../components/history/HistoryChart';
import { PeriodSelector } from '../components/history/PeriodSelector';
import { RegisterSelector } from '../components/history/RegisterSelector';
import { DatePickerWithRange } from '../components/ui/date-picker-range';
import { Button } from '../components/ui/button';
import { Sheet, SheetTrigger, SheetContent, SheetHeader, SheetTitle } from '../components/ui/sheet';
import { useHistory } from '../lib/hooks/useHistory';
import { useMobile } from '../hooks/useMobile';
import { getDateRangeForPeriod } from '../lib/utils/date';
import { historyDataGroups } from '../lib/config/groups';
import { Menu, RefreshCw } from 'lucide-react';
import type { Period } from '../types';
import type { DateRange } from 'react-day-picker';

export function History(): React.ReactElement {
  const isMobile = useMobile();

  // Initialize with proper date range for daily period
  const initialRange = getDateRangeForPeriod('daily');
  const [period, setPeriod] = useState<Period>('daily');
  const [selectedIds, setSelectedIds] = useState<string[]>(['pv_energy_today']);
  const [startDate, setStartDate] = useState(initialRange.start);
  const [endDate, setEndDate] = useState(initialRange.end);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  
  const { data, isLoading, error, loadHistory } = useHistory();

  // Format date string for Date objects (handles yyyy, yyyy-MM, yyyy-MM-dd)
  const formatForDate = useCallback((dateStr: string, targetPeriod: Period = period): string => {
    if (targetPeriod === 'yearly' && dateStr.length === 4) return `${dateStr}-01-01`;
    if (targetPeriod === 'monthly' && dateStr.length === 7) return `${dateStr}-01`;
    if (dateStr.length === 4) return `${dateStr}-01-01`;
    if (dateStr.length === 7) return `${dateStr}-01`;
    return dateStr;
  }, [period]);

  // Derive dateRange from startDate, endDate, and period - no state needed
  const dateRange: DateRange | undefined = React.useMemo(() => {
    if (startDate && endDate) {
      return {
        from: new Date(formatForDate(startDate, period)),
        to: new Date(formatForDate(endDate, period))
      };
    }
    return undefined;
  }, [startDate, endDate, period, formatForDate]);

  // Track if we've loaded initial data to avoid duplicate loads
  const loadedRef = React.useRef(false);

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

  // Load initial history data on mount
  useEffect(() => {
    if (loadedRef.current) return;
    loadedRef.current = true;
    if (startDate && endDate && selectedIds.length > 0) {
      loadHistory(selectedIds, startDate, endDate);
    }
  }, [loadHistory, selectedIds, startDate, endDate]);

  // Load history when state changes (period, dates, or selected IDs)
  useEffect(() => {
    // Only load if we have valid data
    if (startDate && endDate && selectedIds.length > 0) {
      // Check if all selected IDs are valid for the current period
      const validIdsForPeriod = historyDataGroups[period] || [];
      const allValid = selectedIds.every(id => validIdsForPeriod.includes(id));
      
      if (allValid && loadedRef.current) {
        loadHistory(selectedIds, startDate, endDate);
      }
    }
  }, [startDate, endDate, selectedIds, period, loadHistory]);

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
        setStartDate(range.from.toISOString().split('T')[0]);
        setEndDate(range.to.toISOString().split('T')[0]);
      }
    }
  }, [period]);

  return (
    <div className="p-2 sm:p-4 md:p-6 lg:p-8 w-full">
      <div className="w-full overflow-x-hidden">
        <div className="flex flex-col lg:flex-row gap-3 sm:gap-4 mb-4 sm:mb-6 px-2 overflow-x-hidden">
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
            {!isMobile ? <h1 className="text-xl sm:text-2xl font-bold">History</h1> : null}
            {/* Desktop refresh button - top right, only on lg+ screens */}
            <div className="hidden lg:flex">
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
            </div>
          </div>
          {/* Mobile header actions */}
          <div className="lg:hidden flex items-center gap-2">
              <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
                <SheetTrigger asChild>
                  <Button variant="outline" size="sm" className="gap-2">
                    <Menu className="h-4 w-4" />
                    <span>Filters</span>
                  </Button>
                </SheetTrigger>
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
                <SheetContent side="left" className="p-0 w-full max-w-xs max-w-[100vw] h-full">
                  <SheetHeader className="p-4 pb-2 border-b border-border">
                    <SheetTitle>History Filters</SheetTitle>
                  </SheetHeader>
                  <div className="flex flex-col h-full bg-background">
                    {/* Filter sections with dividers */}
                    <div className="p-4 space-y-1 border-b border-border">
                      <h3 className="text-sm font-medium text-muted-foreground mb-2">Time Period</h3>
                      <PeriodSelector period={period} onPeriodChange={handlePeriodChange} />
                    </div>
                    
                    <div className="p-4 space-y-1 border-b border-border">
                      <h3 className="text-sm font-medium text-muted-foreground mb-2">Date Range</h3>
                      <DatePickerWithRange 
                        date={dateRange}
                        setDate={handleDateRangeChange}
                        label="Date Range"
                        className="w-full"
                        period={period}
                        hideLabel={true}
                      />
                    </div>
                    
                    <div className="flex-1 p-4 overflow-y-auto">
                      <h3 className="text-sm font-medium text-muted-foreground mb-2">Data Series</h3>
                      <RegisterSelector 
                        selectedIds={selectedIds} 
                        onToggle={handleToggle} 
                        period={period} 
                      />
                    </div>
                    
                    {/* Sticky bottom actions */}
                    <div className="p-4 pt-2 border-t border-border bg-background">
                        <Button 
                          onClick={() => {
                            handleRefresh();
                            setMobileSidebarOpen(false);
                          }}
                          disabled={isLoading || selectedIds.length === 0}
                          className="w-full gap-2"
                        >
                          <RefreshCw className="h-4 w-4" />
                          {isLoading ? 'Loading...' : 'Apply Filters'}
                        </Button>
                    </div>
                  </div>
                </SheetContent>
              </Sheet>
          </div>
        </div>
        
        <div className="flex flex-col lg:flex-row gap-3 sm:gap-4 px-2 min-w-0 overflow-x-hidden">
          {/* Desktop sidebar - hidden on mobile */}
          <div className="hidden lg:flex lg:flex-col gap-3 sm:gap-4 w-full max-w-[16rem] min-w-0 flex-shrink-0">
            <div className="space-y-1">
              <h3 className="text-sm font-medium text-muted-foreground mb-1">Time Period</h3>
              <PeriodSelector period={period} onPeriodChange={handlePeriodChange} />
            </div>
            <div className="space-y-1">
              <h3 className="text-sm font-medium text-muted-foreground mb-1">Date Range</h3>
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
              <h3 className="text-sm font-medium text-muted-foreground mb-1">Data Series</h3>
              <RegisterSelector 
                selectedIds={selectedIds} 
                onToggle={handleToggle} 
                period={period} 
              />
            </div>
          </div>
          
          {/* Chart area */}
          <div className="flex-1 min-w-0 overflow-x-hidden">
            {error && (
              <div className="p-3 sm:p-4 bg-destructive/10 text-destructive rounded mb-3 sm:mb-4">
                {error}
              </div>
            )}
            <div className="bg-card rounded-lg p-3 sm:p-4 shadow-sm overflow-hidden mb-6 sm:mb-8 lg:mb-10">
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
      </div>
    </div>
  );
}
