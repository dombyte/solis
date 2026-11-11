import type { ReportFigure } from '../api/report';

const FIGURE_LABELS: Record<string, string> = {
  pv: 'PV yield',
  consumption: 'Consumption',
  self_sufficiency: 'Self-sufficiency',
  self_consumption: 'Self-consumption',
  grid_import: 'Grid import',
  grid_export: 'Grid export',
  battery_charge: 'Battery charged',
  battery_discharge: 'Battery discharged',
};

export const figureLabel = (id: string): string => FIGURE_LABELS[id] ?? id;

/** One decimal in the browser's locale ("4,512.3" / "4.512,3"). */
export function formatNumber(v: number): string {
  return v.toLocaleString(undefined, { minimumFractionDigits: 1, maximumFractionDigits: 1 });
}

/** A figure's value without unit; "–" without data. */
export function figureValue(value: number | null): string {
  if (value === null) return '–';
  return formatNumber(value);
}

export const figureUnit = (kind: ReportFigure['kind']): string => (kind === 'ratio' ? '%' : 'kWh');

export type Tone = 'better' | 'worse' | 'neutral';

export interface Delta {
  text: string;
  tone: Tone;
  /** Accessible wording ("up 8.8 %"), the arrow alone is not read out well. */
  label: string;
}

const round1 = (v: number): number => Math.round(v * 10) / 10;

/**
 * The change of a figure between its two compared values (`compared` and `base`):
 * percentage points for ratios (the plain difference of the two percentages), percent for
 * energy (kWh when the base is 0). Point and kWh changes are the difference of the
 * displayed values, so 98.0 % -> 84.3 % reads 13.7 points, never 13.6.
 */
export function figureDelta(f: ReportFigure): Delta {
  if (f.change === null || f.compared === null || f.base === null) {
    return { text: 'no data', tone: 'neutral', label: 'no comparison data' };
  }
  let value = round1(f.compared) - round1(f.base);
  let unit = ' kWh';
  let spoken = ' kWh';
  if (f.kind === 'ratio') {
    unit = ' points';
    spoken = ' percentage points';
  } else if (f.change_pct !== null) {
    value = f.change_pct;
    unit = spoken = ' %';
  }
  const magnitude = formatNumber(Math.abs(value));
  if (magnitude === formatNumber(0)) return { text: `±${magnitude}${unit}`, tone: 'neutral', label: 'unchanged' };
  const up = value > 0;
  let tone: Tone = 'neutral';
  if (f.better !== 'neutral') tone = (f.better === 'higher') === up ? 'better' : 'worse';
  return {
    text: `${up ? '↑' : '↓'} ${magnitude}${unit}`,
    tone,
    label: `${up ? 'up' : 'down'} ${magnitude}${spoken}`,
  };
}
