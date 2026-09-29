import { useMediaQuery } from './useMediaQuery';

/**
 * Detects if the device has a coarse pointer (touch/finger input)
 * Returns true for phones, tablets, and other touch devices
 * Returns false for desktop/laptop with mouse input
 */
export function useMobile(): boolean {
  return useMediaQuery('(pointer: coarse)');
}
