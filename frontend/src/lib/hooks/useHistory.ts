import { useState, useCallback } from 'react';
import { api } from '../api/client';
import { getSourceById, apiDataObjects } from '../config/data';
import type { Period, HistoryDataPoint, ChartData, ChartDataset } from '../../types';

/**
 * Get timestamp label based on period
 */
function getTimestampLabel(ts: string, period: Period): string {
  const date = new Date(ts);
  
  switch (period) {
    case 'daily':
      return date.toLocaleDateString();
    case 'monthly':
      return date.toLocaleString('default', { month: 'short', year: 'numeric' });
    case 'yearly':
      return ts; // For yearly, ts should be just the year
    default:
      return ts;
  }
}

export interface UseHistoryOptions {
  startDate?: string;
  endDate?: string;
}

export interface UseHistoryResult {
  data: ChartData | null;
  isLoading: boolean;
  error: string | null;
  loadHistory: (registerIds: string[], startDate?: string, endDate?: string) => Promise<void>;
}

export function useHistory(): UseHistoryResult {
  const [data, setData] = useState<ChartData | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadHistory = useCallback(async (registerIds: string[], startDate?: string, endDate?: string) => {
    setIsLoading(true);
    setError(null);
    
    try {
      // Map internal IDs to source paths using the helper function
      const sources = registerIds.map(id => getSourceById(id)).filter(Boolean) as string[];

      if (sources.length === 0) {
        setData(null);
        return;
      }

      // Collect all timestamps and build datasets
      const allTimestamps: Set<string> = new Set();
      const timestampToLabel: Map<string, string> = new Map();
      const datasetDataMap: Map<string, Map<string, number>> = new Map();
      const datasets: ChartDataset[] = [];

      // Fetch data for each source
      for (const source of sources) {
        const dataObj = apiDataObjects.find(o => o.source === source);
        let historyData: HistoryDataPoint[] = [];
        
        try {
          // The source already contains the full path including period
          const params: Record<string, string> = {};
          if (startDate) params.start = startDate;
          if (endDate) params.end = endDate;
          
          historyData = await api.get(source, params) as HistoryDataPoint[];
        } catch (err) {
          console.error(`Failed to fetch history for ${source}:`, err);
          continue;
        }

        if (dataObj && historyData?.length) {
          datasetDataMap.set(dataObj.key, new Map());
          
          for (const d of historyData) {
            const ts = d.date || d.month || d.year || '';
            if (!ts) continue;
            
            // Determine period from source path
            const period = source.includes('/daily/') ? 'daily' : 
                          source.includes('/monthly/') ? 'monthly' : 
                          source.includes('/yearly/') ? 'yearly' : 'daily';
            const label = getTimestampLabel(ts, period);
            allTimestamps.add(ts);
            timestampToLabel.set(ts, label);
            datasetDataMap.get(dataObj.key)!.set(ts, d.value);
          }
          
          datasets.push({
            label: dataObj.name,
            key: dataObj.key,
            borderColor: '',
            backgroundColor: '',
            unit: dataObj.unit || '',
            data: [],
          });
        }
      }

      // Build final chart data
      if (datasets.length > 0) {
        const sortedTimestamps = Array.from(allTimestamps).sort();
        setData({
          labels: sortedTimestamps.map(ts => timestampToLabel.get(ts)!),
          datasets: datasets.map(ds => ({
            ...ds,
            data: sortedTimestamps.map(ts => {
              const val = ds.key ? datasetDataMap.get(ds.key)?.get(ts) : undefined;
              return val !== undefined ? val : null;
            }),
          })),
        });
      } else {
        setData(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load history');
      setData(null);
    } finally {
      setIsLoading(false);
    }
  }, []);

  return { data, isLoading, error, loadHistory };
}
