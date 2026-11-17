import React, { useState, useCallback, useMemo, useRef } from 'react';
import { format } from 'date-fns';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { useSubscription } from '../lib/hooks/useSubscription';
import { Page } from '../components/layout/Page';
import { Card } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Dialog, DialogClose, DialogContent, DialogDescription, DialogTitle } from '../components/ui/dialog';
import { Clock, X } from 'lucide-react';
import { SkeletonCard } from '../components/ui/skeleton';
import { dashboardGroups } from '../lib/config/groups';
import { api } from '../lib/api/client';
import { getSourceById, apiDataObjects } from '../lib/config/data';
import { statusText } from '../lib/utils/status';
import type { RegisterValue, SolisStatusDecoded, FaultStatusDecoded } from '../types';

const GRID_CLASS = 'grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3 sm:gap-4';

export function Status(): React.ReactElement {
  const isLoading = useRegisterStore(state => state.isLoading);
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const registerValues = useRegisterStore(state => state.registerValues);

  // Status register IDs from the system_status group (members defined in data.ts)
  const statusRegisterIds = useMemo(
    () => dashboardGroups.find(g => g.id === 'system_status')?.dataIds ?? [],
    []
  );

  // Status page subscribes to all status keys.
  const statusKeys = useMemo(
    () => statusRegisterIds
      .map(id => registerMetadata.get(id)?.key)
      .filter((key): key is string => key !== undefined),
    [statusRegisterIds, registerMetadata]
  );
  useSubscription(statusKeys);

  // Get register metadata for these IDs and add order from apiDataObjects
  const statusRegisters = statusRegisterIds
    .map(id => {
      const reg = registerMetadata.get(id);
      const dataObj = apiDataObjects.find(obj => obj.id === id);
      if (!reg) return null;
      return { ...reg, order: dataObj?.order ?? 0 };
    })
    .filter(Boolean) as Array<{ id: string; name: string; description?: string; order: number; [key: string]: unknown }>;

  const [selectedStatusKey, setSelectedStatusKey] = useState<string | null>(null);
  const [historyData, setHistoryData] = useState<StatusHistoryDataPoint[] | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState<string | null>(null);

  // Status history entry from API
  interface StatusHistoryEntry {
    timestamp: string;
    status_decoded: SolisStatusDecoded | FaultStatusDecoded | null;
  }
  
  // Local history data point for status display
  interface StatusHistoryDataPoint {
    date?: string;
    month?: string;
    year?: string;
    value: SolisStatusDecoded | FaultStatusDecoded | null;
    timestamp: string;
  }

  // Fetch history for selected status key. A newer request aborts the previous one, so a
  // slow response for one register can never replace another's history (review FE-M5).
  const historyAbortRef = useRef<AbortController | null>(null);
  const fetchHistory = useCallback(async (id: string) => {
    if (!id) return;
    historyAbortRef.current?.abort();
    const controller = new AbortController();
    historyAbortRef.current = controller;
    const { signal } = controller;

    setHistoryLoading(true);
    setHistoryError(null);

    try {
      // Get the source path for this internal id
      const source = getSourceById(id);
      if (!source) {
        throw new Error('Register not found');
      }

      const data = await api.get(source, undefined, { signal });
      if (signal.aborted) return;
      const history = (data as { history?: unknown } | null)?.history;
      if (!Array.isArray(history)) {
        throw new Error('Unexpected status history response');
      }

      // Date and time both in local time (the server sends UTC "...Z" timestamps).
      const convertedData: StatusHistoryDataPoint[] = (history as StatusHistoryEntry[]).map(entry => ({
        date: format(new Date(entry.timestamp), 'yyyy-MM-dd'),
        value: entry.status_decoded,
        timestamp: entry.timestamp,
      }));

      setHistoryData(convertedData);
    } catch (error) {
      if (signal.aborted) return;
      setHistoryError(error instanceof Error ? error.message : 'Failed to load history');
      setHistoryData(null);
    } finally {
      if (!signal.aborted) setHistoryLoading(false);
    }
  }, []);

  // Handle status card click
  const handleStatusClick = useCallback((id: string) => {
    setSelectedStatusKey(id);
    fetchHistory(id);
  }, [fetchHistory]);

  // Close modal
  const handleCloseModal = useCallback(() => {
    setSelectedStatusKey(null);
    setHistoryData(null);
    setHistoryError(null);
  }, []);

  const formatStatusValue = (value: RegisterValue | undefined): string =>
    value ? statusText(value.statusDecoded, value.value) : '-';

  // Get the register metadata by id for the modal title
  const getRegisterById = useCallback((id: string) => {
    return registerMetadata.get(id);
  }, [registerMetadata]);

  // Check if we have register metadata loaded
  const hasMetadata = statusRegisterIds.length > 0 && statusRegisters.length > 0;
  
  // Check if we have any values
  const hasValues = statusRegisterIds.some(id => {
    const value = registerValues.get(id);
    return value?.value !== null && value?.value !== undefined;
  });

  // Loading state - show skeletons if no metadata or (loading and no values)
  if (!hasMetadata || (isLoading && !hasValues)) {
    return (
      <Page title="Status">
        <div className={GRID_CLASS}>
          {statusRegisterIds.map(id => (
            <SkeletonCard key={id} className="w-full" />
          ))}
        </div>
      </Page>
    );
  }

  return (
    <Page title="Status">
        {/* Status Cards Grid */}
        <div className={GRID_CLASS}>
          {statusRegisters.map(reg => {
            const value = registerValues.get(reg.id);
            const displayValue = formatStatusValue(value);
            
            return (
              <Card
                key={reg.id}
                className="w-full cursor-pointer p-4 sm:px-5 flex flex-col gap-1.5 hover:border-primary/50 transition-colors"
                onClick={() => handleStatusClick(reg.id)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    handleStatusClick(reg.id);
                  }
                }}
              >
                <div className="flex items-baseline justify-between gap-3">
                  <h3 className="text-sm font-medium text-muted-foreground">{reg.name}</h3>
                  {value?.timestamp && (
                    <span className="shrink-0 inline-flex items-center gap-1 text-xs text-muted-foreground tabular-nums">
                      <Clock className="h-3 w-3" aria-hidden="true" />
                      {format(new Date(value.timestamp), 'HH:mm:ss')}
                    </span>
                  )}
                </div>
                <p className="text-base font-semibold break-words">{displayValue}</p>
                {reg.description && (
                  <p className="text-xs text-muted-foreground">{reg.description}</p>
                )}
              </Card>
            );
          })}
        </div>
        
        {/* Status Detail Modal - Centered Box */}
        <Dialog open={selectedStatusKey !== null} onOpenChange={(open) => { if (!open) handleCloseModal(); }}>
          <DialogContent className="max-w-4xl w-[90vw] max-h-[80vh] overflow-hidden p-0">
            {/* Header */}
            <div className="flex items-center justify-between p-4 border-b">
              <DialogTitle>
                {(selectedStatusKey && getRegisterById(selectedStatusKey)?.name) || selectedStatusKey}
              </DialogTitle>
              <DialogClose asChild>
                <Button variant="ghost" size="icon" className="h-8 w-8">
                  <X className="h-4 w-4" />
                  <span className="sr-only">Close</span>
                </Button>
              </DialogClose>
            </div>
            <DialogDescription>
              History log for {(selectedStatusKey && getRegisterById(selectedStatusKey)?.name) || selectedStatusKey}
            </DialogDescription>

            {/* Content - Log-style display */}
            <div className="p-4 max-h-[calc(80vh-80px)] overflow-y-auto">
              {historyLoading ? (
                <div className="flex items-center justify-center h-64">
                  <div className="flex flex-col items-center gap-3">
                    <div className="animate-spin h-6 w-6 border-4 border-primary border-t-transparent rounded-full" />
                    <span className="text-muted-foreground">Loading history...</span>
                  </div>
                </div>
              ) : historyError ? (
                <div className="p-4 bg-destructive/10 text-destructive rounded">
                  {historyError}
                </div>
              ) : historyData && historyData.length > 0 ? (
                <div className="space-y-1">
                  <h3 className="text-sm font-semibold text-muted-foreground mb-3 pb-2 border-b">
                    History Log
                  </h3>
                  <div className="font-mono text-sm space-y-1">
                    {historyData.map((dataPoint) => {
                      const date = dataPoint.date || dataPoint.month || dataPoint.year || '';
                      const time = dataPoint.timestamp ? format(new Date(dataPoint.timestamp), 'HH:mm:ss') : '';

                      const displayValue = statusText(dataPoint.value);

                      return (
                        <div key={dataPoint.timestamp} className="py-1.5 px-2 rounded hover:bg-muted/50 transition-colors break-words">
                          <span className="text-muted-foreground">
                            [{date} {time}]
                          </span>
                          <span className="ml-2">
                            {displayValue}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                </div>
              ) : (
                <div className="flex items-center justify-center h-64 text-muted-foreground">
                  <p>No history data available</p>
                </div>
              )}
            </div>
          </DialogContent>
        </Dialog>
    </Page>
  );
}
