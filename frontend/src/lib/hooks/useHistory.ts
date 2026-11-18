import { useState, useCallback, useRef } from 'react';
import { format, parseISO } from 'date-fns';
import { api } from '../api/client';
import { getSourceById, apiDataObjects } from '../config/data';
import type { Period, HistoryDataPoint, ChartData, ChartDataset } from '../../types';

interface Labels {
  long: string;
  short: string;
}

/** Axis labels of a history row key in the app's date style (English, like the PDF). */
function timestampLabels(ts: string, period: Period): Labels {
  if (period === 'yearly') return { long: ts, short: ts };
  // parseISO reads "2026-09-01" / "2026-09" as local time; new Date() would read them as
  // UTC midnight and show the previous day/month west of UTC.
  const date = parseISO(ts);
  if (period === 'monthly') return { long: format(date, 'MMM yyyy'), short: format(date, 'MMM') };
  return { long: format(date, 'd MMM yyyy'), short: format(date, 'd MMM') };
}

export interface UseHistoryResult {
  data: ChartData | null;
  isLoading: boolean;
  error: string | null;
  loadHistory: (registerIds: string[], startDate?: string, endDate?: string) => Promise<void>;
  /** Aborts any request in flight and clears the chart data and error. */
  clearHistory: () => void;
}

export function useHistory(): UseHistoryResult {
  const [data, setData] = useState<ChartData | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  const loadHistory = useCallback(async (registerIds: string[], startDate?: string, endDate?: string) => {
    // Cancel any in-flight request so a slower, stale response can never overwrite a
    // newer one (e.g. rapid period/checkbox changes firing overlapping requests).
    abortControllerRef.current?.abort();
    const controller = new AbortController();
    abortControllerRef.current = controller;
    const { signal } = controller;

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
      const timestampToLabel: Map<string, Labels> = new Map();
      const datasetDataMap: Map<string, Map<string, number>> = new Map();
      const datasets: ChartDataset[] = [];
      const failed: string[] = [];

      // Fetch every source at once (one round-trip of latency instead of one per series);
      // results are processed in source order so the datasets keep their order.
      const params: Record<string, string> = {};
      if (startDate) params.start = startDate;
      if (endDate) params.end = endDate;
      const results = await Promise.allSettled(sources.map(async (source) => {
        const body = await api.get(source, params, { signal });
        if (!Array.isArray(body)) throw new Error('unexpected history response');
        return body as HistoryDataPoint[];
      }));
      if (signal.aborted) return;

      for (const [i, source] of sources.entries()) {
        const dataObj = apiDataObjects.find(o => o.source === source);
        const result = results[i];
        if (result.status === 'rejected') {
          console.error(`Failed to fetch history for ${source}:`, result.reason);
          failed.push(dataObj?.name ?? source);
          continue;
        }
        const historyData = result.value;

        if (dataObj && historyData.length) {
          datasetDataMap.set(dataObj.key, new Map());
          
          for (const d of historyData) {
            const ts = d.date || d.month || d.year || '';
            if (!ts) continue;

            // The row's own key names its period (sources are /api/data/<key>, without a
            // period segment, so the path cannot tell).
            const period: Period = d.date ? 'daily' : d.month ? 'monthly' : 'yearly';
            allTimestamps.add(ts);
            timestampToLabel.set(ts, timestampLabels(ts, period));
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

      if (signal.aborted) return;

      // A failed source must not look like "no data": all failed -> error; some failed
      // -> chart the rest and say which ones are missing.
      if (failed.length === sources.length) {
        setError(`Failed to load history (${failed.join(', ')})`);
        setData(null);
        return;
      }
      if (failed.length > 0) {
        setError(`Some series failed to load: ${failed.join(', ')}`);
      }

      // Build final chart data
      if (datasets.length > 0) {
        const sortedTimestamps = Array.from(allTimestamps).sort();
        setData({
          labels: sortedTimestamps.map(ts => timestampToLabel.get(ts)!.long),
          shortLabels: sortedTimestamps.map(ts => timestampToLabel.get(ts)!.short),
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
      if (signal.aborted) return;
      setError(err instanceof Error ? err.message : 'Failed to load history');
      setData(null);
    } finally {
      if (!signal.aborted) {
        setIsLoading(false);
      }
    }
  }, []);

  const clearHistory = useCallback(() => {
    abortControllerRef.current?.abort();
    setData(null);
    setError(null);
    setIsLoading(false);
  }, []);

  return { data, isLoading, error, loadHistory, clearHistory };
}
