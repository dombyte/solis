import { useLocation } from 'react-router-dom';
import { MobileThemeToggle } from './MobileThemeToggle';

export function MobileHeader() {
  const location = useLocation();

  // Extract page title from path
  const getPageTitle = (): string => {
    const path = location.pathname;
    if (path === '/' || path.startsWith('/dashboard')) return 'Dashboard';
    if (path.startsWith('/history')) return 'History';
    if (path.startsWith('/status')) return 'Status';
    if (path.startsWith('/info')) return 'Info';
    return 'Solis Monitor';
  };

  const title = getPageTitle();

  return (
    <div className="md:hidden flex items-center justify-between px-4 py-3 w-full">
      <h1 className="text-xl font-bold">{title}</h1>
      <MobileThemeToggle />
    </div>
  );
}
