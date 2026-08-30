import { useState, useEffect } from 'react';

/**
 * Detects if the device has a coarse pointer (touch/finger input)
 * Returns true for phones, tablets, and other touch devices
 * Returns false for desktop/laptop with mouse input
 */
export function useMobile(): boolean {
  const [isMobile, setIsMobile] = useState(false);

  useEffect(() => {
    if (typeof window === 'undefined') return;

    const mediaQuery = window.matchMedia('(pointer: coarse)');
    const handleChange = () => {
      setIsMobile(mediaQuery.matches);
    };

    handleChange();
    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, []);

  return isMobile;
}
