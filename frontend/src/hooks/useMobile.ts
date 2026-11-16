import { useMediaQuery } from './useMediaQuery';

/**
 * Detects if the device has a coarse pointer (touch/finger input)
 * Returns true for phones, tablets, and other touch devices
 * Returns false for desktop/laptop with mouse input
 */
export function useMobile(): boolean {
  return useMediaQuery('(pointer: coarse)');
}

/**
 * Whether the app shell uses the compact layout (top header + bottom navigation instead
 * of the sidebar): on touch devices, and on any window narrower than 768 px, where the
 * 256 px sidebar would leave too little room for the page.
 */
export function useCompactLayout(): boolean {
  return useMediaQuery('(pointer: coarse), (max-width: 767px)');
}
