import { useLocation } from 'react-router-dom';
import { MobileThemeToggle } from './MobileThemeToggle';
import { pageTitle } from './nav';

export function MobileHeader() {
  const location = useLocation();

  const title = pageTitle(location.pathname);

  return (
    <div className="flex items-center justify-between px-4 py-3 w-full">
      <h1 className="text-xl font-bold">{title}</h1>
      <MobileThemeToggle />
    </div>
  );
}
