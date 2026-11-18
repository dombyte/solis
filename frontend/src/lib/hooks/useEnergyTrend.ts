import { useEffect, useMemo } from 'react';
import { format, subDays } from 'date-fns';
import { useHistory } from './useHistory';
import { useRegisterStore } from '../stores/useRegisterStore';
import { formatNumber } from '../utils/format';

const PERCENT = 100;
// Live register (today) and the key of its daily history rows.
const PV = { id: 'pv_energy_today', key: 'pv_energy_daily' };
const CONSUMPTION = { id: 'energy_consumption_today', key: 'energy_consumption_daily' };
const EXPORT = { id: 'energy_fed_today', key: 'grid_export_daily' };
const IMPORT = { id: 'energy_imported_today', key: 'grid_import_daily' };
const SERIES = [PV.id, CONSUMPTION.id, EXPORT.id, IMPORT.id];

/** One day of the PV vs consumption trend; null = no data for that value. */
export interface TrendDay {
  /** yyyy-MM-dd */
  key: string;
  /** "18 Nov" */
  label: string;
  /** "Wed" */
  weekday: string;
  /** "Wed, 18 Nov 2026" */
  long: string;
  pv: number | null;
  consumption: number | null;
  exported: number | null;
  imported: number | null;
  /** (consumption − import) / consumption, as on the Reports page; null without both. */
  selfSufficiency: number | null;
  today: boolean;
}

/** Rounds v up to 1, 2, 2.5 or 5 times a power of ten (same axis rule as the reports). */
function niceMax(v: number): number {
  if (v <= 1) return 1;
  const exp = 10 ** Math.floor(Math.log10(v));
  for (const m of [1, 2, 2.5, 5, 10]) {
    if (m * exp >= v) return m * exp;
  }
  return 10 * exp;
}

/** The store's live value of a register, or null. */
function useLive(id: string): number | null {
  const v = useRegisterStore(state => state.registerValues.get(id)?.value);
  return typeof v === 'number' ? v : null;
}

function selfSufficiency(consumption: number | null, imported: number | null): number | null {
  if (consumption === null || imported === null || consumption <= 0) return null;
  return Math.min(1, Math.max(0, (consumption - imported) / consumption));
}

export const formatKwh = (v: number | null): string => (v === null ? '–' : `${formatNumber(v, 1)} kWh`);
export const formatShare = (v: number | null): string =>
  v === null ? '–' : `${formatNumber(v * PERCENT, 0)} %`;

/** The report of one finished day (the Reports page takes ?from&to). */
export const dayReportPath = (d: TrendDay): string => `/reports?from=${d.key}&to=${d.key}`;

/** Bar length of v on a scale up to top, in percent. */
export const barPercent = (v: number, top: number): number => Math.min(PERCENT, (v / top) * PERCENT);

interface EnergyTrend {
  /** Oldest first; the last one is today (so far). */
  days: TrendDay[];
  /** Common scale of all bars (nice rounded maximum of PV and consumption). */
  top: number;
  error: string | null;
}

/**
 * PV, consumption, export and import of the last `count` days, today included. Past days
 * come from the daily history (REST, loaded once per day); today follows the live
 * counters. A day without data keeps null values, never 0 kWh.
 */
export function useEnergyTrend(count: number): EnergyTrend {
  const { data, error, loadHistory } = useHistory();
  const livePV = useLive(PV.id);
  const liveConsumption = useLive(CONSUMPTION.id);
  const liveExport = useLive(EXPORT.id);
  const liveImport = useLive(IMPORT.id);

  // Recomputed on every render (live frames re-render often), so the window moves on at
  // midnight and the effect below reloads the history for the new range.
  const todayKey = format(new Date(), 'yyyy-MM-dd');
  const startKey = format(subDays(new Date(), count - 1), 'yyyy-MM-dd');

  useEffect(() => {
    loadHistory(SERIES, startKey, todayKey);
  }, [loadHistory, startKey, todayKey]);

  const days: TrendDay[] = useMemo(() => {
    // useHistory labels rows "d MMM yyyy"; match the slots by that label.
    const index = new Map((data?.labels ?? []).map((l, i) => [l, i]));
    const valueOf = (key: string, i: number | undefined): number | null => {
      if (i === undefined) return null;
      const ds = data?.datasets.find(d => d.key === key);
      return ds?.data[i] ?? null;
    };
    return Array.from({ length: count }, (_, n) => {
      const date = subDays(new Date(`${todayKey}T00:00:00`), count - 1 - n);
      const today = n === count - 1;
      const i = index.get(format(date, 'd MMM yyyy'));
      const pick = (live: number | null, key: string) => (today ? live ?? valueOf(key, i) : valueOf(key, i));
      const consumption = pick(liveConsumption, CONSUMPTION.key);
      const imported = pick(liveImport, IMPORT.key);
      return {
        key: format(date, 'yyyy-MM-dd'),
        label: format(date, 'd MMM'),
        weekday: format(date, 'EEE'),
        long: format(date, 'EEE, d MMM yyyy'),
        pv: pick(livePV, PV.key),
        consumption,
        exported: pick(liveExport, EXPORT.key),
        imported,
        selfSufficiency: selfSufficiency(consumption, imported),
        today,
      };
    });
  }, [data, livePV, liveConsumption, liveExport, liveImport, todayKey, count]);

  const top = niceMax(Math.max(0, ...days.flatMap(d => [d.pv ?? 0, d.consumption ?? 0])));
  return { days, top, error };
}
