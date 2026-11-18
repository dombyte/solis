import { useLayoutEffect, type RefObject } from 'react';
import { useLocation, useNavigationType } from 'react-router-dom';

interface ScrollResetProps {
  /** Scroll containers of the shell besides the window (the desktop main column). */
  containers: RefObject<HTMLElement | null>[];
}

/**
 * Starts every newly opened page at the top. BrowserRouter keeps the scroll position, so
 * a link from the bottom of the dashboard (trend → day report) or a nav tap opened the
 * next page half scrolled. Back/forward (POP) is left alone; only the path counts, so
 * query changes on the same page (report filters) keep the position.
 */
export function ScrollReset({ containers }: ScrollResetProps): null {
  const { pathname } = useLocation();
  const navigationType = useNavigationType();

  useLayoutEffect(() => {
    if (navigationType === 'POP') return;
    window.scrollTo(0, 0);
    for (const ref of containers) ref.current?.scrollTo(0, 0);
    // Only a new path resets (navigationType belongs to that navigation; containers
    // is a stable list of refs).
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  return null;
}
