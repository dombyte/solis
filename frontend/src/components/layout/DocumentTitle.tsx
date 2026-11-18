import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';
import { pageTitle } from './nav';

const APP_NAME = 'Solis Monitor';

/** Names the browser tab and history entry after the current page ("History · Solis Monitor"). */
export function DocumentTitle(): null {
  const { pathname } = useLocation();

  useEffect(() => {
    const title = pageTitle(pathname);
    document.title = title === APP_NAME ? APP_NAME : `${title} · ${APP_NAME}`;
  }, [pathname]);

  return null;
}
