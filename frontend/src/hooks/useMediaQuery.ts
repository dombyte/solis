import { useState, useEffect } from 'react';

function matches(query: string): boolean {
  return typeof window !== 'undefined' && window.matchMedia(query).matches;
}

/**
 * Returns whether the media query matches and follows its changes. The first render
 * already uses the current answer, so layouts do not flash the wrong variant on load.
 */
export function useMediaQuery(query: string): boolean {
  const [state, setState] = useState(() => matches(query));

  useEffect(() => {
    if (typeof window === 'undefined') return;

    const mediaQuery = window.matchMedia(query);
    const handleChange = () => setState(mediaQuery.matches);

    handleChange();
    mediaQuery.addEventListener('change', handleChange);
    return () => mediaQuery.removeEventListener('change', handleChange);
  }, [query]);

  return state;
}
