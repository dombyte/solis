/**
 * Number formatting for the whole UI. The UI is English and the PDF report uses the same
 * separators ("4,512.3"), so every number goes through this locale, never the browser's.
 */
export const LOCALE = 'en-US';

/** Whole watts are precise enough; anything finer is meter noise. */
const WATT_DIGITS = 0;
const DEFAULT_DIGITS = 2;
const KILO = 1000;

/** v with exactly `digits` decimals and thousands separators ("26,154.80"). */
export function formatNumber(v: number, digits: number): string {
  return v.toLocaleString(LOCALE, { minimumFractionDigits: digits, maximumFractionDigits: digits });
}

/** A formatted number and the unit it is shown in. */
export interface Measurement {
  value: string;
  unit: string;
}

/**
 * Formats a value in its register unit: power below 1 kW in whole watts, from 1 kW (and
 * energy from 1 kWh for Wh) in the kilo unit with `precision` decimals (default 2).
 */
export function formatMeasurement(v: number, unit: string, precision?: number): Measurement {
  const digits = precision ?? DEFAULT_DIGITS;
  if ((unit === 'W' || unit === 'Wh') && Math.abs(v) >= KILO) {
    return { value: formatNumber(v / KILO, digits), unit: `k${unit}` };
  }
  if (unit === 'W') return { value: formatNumber(v, WATT_DIGITS), unit };
  return { value: formatNumber(v, digits), unit };
}

/** A power in watts as text ("801 W", "4.37 kW"). */
export function formatPower(watts: number): string {
  const m = formatMeasurement(watts, 'W');
  return `${m.value} ${m.unit}`;
}
