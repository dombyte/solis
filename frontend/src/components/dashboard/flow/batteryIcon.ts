import { BatteryCharging, BatteryFull, BatteryLow, BatteryMedium, type LucideIcon } from 'lucide-react';

// SOC thresholds (%) for the low / medium / full battery icons
const SOC_LOW = 35;
const SOC_FULL = 70;

/**
 * Pick the lucide battery icon for the flow diagram: charging while
 * battery_power_signed > 0, otherwise low / medium / full by state of charge.
 */
export function getBatteryIcon(soc: number | undefined, power: number | null): LucideIcon {
  if (power !== null && power > 0) return BatteryCharging;
  if (soc === undefined) return BatteryMedium;
  if (soc < SOC_LOW) return BatteryLow;
  return soc < SOC_FULL ? BatteryMedium : BatteryFull;
}
