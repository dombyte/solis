import React, { useState, useCallback } from 'react';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { useWebSocket } from '../lib/hooks/useWebSocket';
import { useMobile } from '../hooks/useMobile';
import { Card, CardContent, CardHeader, CardTitle } from '../components/ui/card';
import { Button } from '../components/ui/button';
import { Clock, X } from 'lucide-react';
import { SkeletonCard } from '../components/ui/skeleton';
import { dashboardGroups } from '../lib/config/groups';
import { api } from '../lib/api/client';
import { getSourceById, apiDataObjects } from '../lib/config/data';
import type { RegisterValue, SolisStatusDecoded, FaultStatusDecoded } from '../types';

export function Status(): React.ReactElement {
  const isMobile = useMobile();

  // Use WebSocket connection (initialized at app level)
  // requestInitialData: true to fetch fresh data when page mounts
  useWebSocket({ autoConnect: false, requestInitialData: true });
  const isLoading = useRegisterStore(state => state.isLoading);
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const registerValues = useRegisterStore(state => state.registerValues);
  
  // Get status register IDs from the system_status group, excluding inverter_temp
  const systemStatusGroup = dashboardGroups.find(g => g.id === 'system_status');
  // Fallback list in case dashboardGroups is not available
  const fallbackStatusIds = ['solis_status', 'operating_status', 'grid_fault_1', 'battery_1_bms_fault', 'battery_2_bms_fault', 'backup_load_fault', 'battery_fault_03', 'device_fault_04', 'device_fault_05'];
  const statusRegisterIds = systemStatusGroup?.dataIds.filter(id => id !== 'inverter_temp') || fallbackStatusIds;
  
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

  // Fetch history for selected status key
  const fetchHistory = useCallback(async (id: string) => {
    if (!id) return;

    setHistoryLoading(true);
    setHistoryError(null);

    try {
      // Get the source path for this internal id
      const source = getSourceById(id);
      if (!source) {
        throw new Error('Register not found');
      }
      
      // Use the new API client with source field
      const data = await api.get(source);
      const response = data as { history: StatusHistoryEntry[] };
      
      // Convert status history to a format similar to HistoryDataPoint
      const convertedData: StatusHistoryDataPoint[] = response.history.map(entry => ({
        date: entry.timestamp.split('T')[0], // Extract date part
        value: entry.status_decoded,
        timestamp: entry.timestamp,
      }));
      
      setHistoryData(convertedData);
    } catch (error) {
      setHistoryError(error instanceof Error ? error.message : 'Failed to load history');
      setHistoryData(null);
    } finally {
      setHistoryLoading(false);
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

  // Format status value for display
  const formatStatusValue = useCallback((value: RegisterValue | undefined): string => {
    if (!value) return '-';

    // Use statusDecoded if available
    if (value.statusDecoded !== undefined && value.statusDecoded !== null) {
      if (Array.isArray(value.statusDecoded)) {
        if (value.statusDecoded.length > 0) {
          return (value.statusDecoded as FaultStatusDecoded).join(', ');
        } else {
          return 'No faults';
        }
      } else if (typeof value.statusDecoded === 'object' && value.statusDecoded !== null) {
        const statusObj = value.statusDecoded as SolisStatusDecoded;
        return statusObj.name || JSON.stringify(value.statusDecoded);
      } else {
        return String(value.statusDecoded);
      }
    }

    // Use display value
    if (value.value !== null && value.value !== undefined) {
      return String(value.value);
    }

    // Use raw value
    if (value.rawValue !== undefined) {
      return String(value.rawValue);
    }

    return '-';
  }, []);

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
      <div className="p-2 sm:p-4 md:p-6 lg:p-8 w-full overflow-x-hidden">
        <div className="w-full overflow-x-hidden">
          {!isMobile ? <h1 className="text-xl sm:text-2xl font-bold mb-4 sm:mb-6 px-2">Status</h1> : null}
          <div className="grid grid-cols-1 gap-3 sm:gap-4 lg:gap-6 px-2 pb-6 sm:pb-8 lg:pb-10">
            {statusRegisterIds.map(id => (
              <SkeletonCard key={id} className="w-full" />
            ))}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="p-2 sm:p-4 md:p-6 lg:p-8 w-full overflow-x-hidden">
      <div className="w-full overflow-x-hidden">
        {!isMobile ? <h1 className="text-xl sm:text-2xl font-bold mb-4 sm:mb-6 px-2">Status Overview</h1> : null}
        
        {/* Status Cards Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3 sm:gap-4 lg:gap-6 px-2 pb-6 sm:pb-8 lg:pb-10">
          {statusRegisters.map(reg => {
            const value = registerValues.get(reg.id);
            const displayValue = formatStatusValue(value);
            
            return (
              <Card 
                key={reg.id} 
                className="w-full cursor-pointer hover:shadow-md transition-shadow"
                onClick={() => handleStatusClick(reg.id)}
              >
                <CardHeader className="pb-2">
                  <div className="flex items-center justify-between">
                    <CardTitle className="text-base sm:text-lg">{reg.name}</CardTitle>
                  </div>
                  {reg.description && (
                    <p className="text-xs sm:text-sm text-muted-foreground">{reg.description}</p>
                  )}
                </CardHeader>
                <CardContent className="pt-2">
                  <div className="flex items-center gap-2">
                    {value?.timestamp && (
                      <span className="text-xs text-muted-foreground">
                        <Clock className="h-3 w-3 inline mr-1" />
                        {new Date(value.timestamp).toLocaleTimeString()}
                      </span>
                    )}
                  </div>
                  <p className="text-sm sm:text-base mt-1">
                    {displayValue}
                  </p>
                </CardContent>
              </Card>
            );
          })}
        </div>
        
        {/* Status Detail Modal - Centered Box */}
        {selectedStatusKey && (
          <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4 sm:p-6 md:p-8 overflow-x-hidden" onClick={handleCloseModal}>
            <div 
              className="bg-background rounded-lg shadow-xl w-full max-w-4xl max-h-[80vh] overflow-hidden max-w-[90vw]"
              onClick={(e) => e.stopPropagation()}
            >
              {/* Header */}
              <div className="flex items-center justify-between p-4 border-b">
                <h2 className="text-lg font-semibold">
                  {getRegisterById(selectedStatusKey)?.name || selectedStatusKey}
                </h2>
                  <Button variant="ghost" size="icon" className="h-8 w-8" onClick={handleCloseModal}>
                    <X className="h-4 w-4" />
                    <span className="sr-only">Close</span>
                  </Button>
              </div>
              
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
                      {historyData.map((dataPoint, index) => {
                        const date = dataPoint.date || dataPoint.month || dataPoint.year || '';
                        const time = dataPoint.timestamp ? new Date(dataPoint.timestamp).toLocaleTimeString() : '';
                        
                        // Format the status value for display
                        let displayValue: string;
                        
                        if (dataPoint.value === null || dataPoint.value === undefined) {
                          displayValue = '-';
                        } else if (typeof dataPoint.value === 'object' && dataPoint.value !== null) {
                          // It's a status_decoded object or array
                          const valueObj = dataPoint.value as SolisStatusDecoded | FaultStatusDecoded;
                          if (Array.isArray(valueObj)) {
                            displayValue = valueObj.join(', ');
                          } else if (typeof valueObj === 'object' && valueObj !== null) {
                            const statusObj = valueObj as { name?: string; description?: string };
                            displayValue = statusObj.name || JSON.stringify(valueObj);
                          } else {
                            displayValue = String(dataPoint.value);
                          }
                        } else {
                          displayValue = String(dataPoint.value);
                        }

                        return (
                          <div key={index} className="py-1.5 px-2 rounded hover:bg-muted/50 transition-colors break-words">
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
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
