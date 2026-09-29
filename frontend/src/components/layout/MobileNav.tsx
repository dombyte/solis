import { Link, useLocation } from 'react-router-dom';
import { LineAwesomeIcon } from '../ui/LineAwesomeIcon';
import { Button } from '../ui/button';

export function MobileNav() {
  const location = useLocation();

  const isActive = (path: string) => {
    if (location.pathname === path) return true;
    if (path === '/dashboard' && (location.pathname === '/' || location.pathname.startsWith('/dashboard'))) return true;
    if (location.pathname.startsWith(path)) return true;
    return false;
  };

  const navItems = [
    { path: '/dashboard', icon: 'la-home', label: 'Dashboard' },
    { path: '/history', icon: 'la-chart-line', label: 'History' },
    { path: '/status', icon: 'la-info-circle', label: 'Status' },
    { path: '/info', icon: 'la-file-alt', label: 'Info' },
  ];

  return (
    <nav className="fixed bottom-0 left-0 right-0 glassy-nav-mobile z-50 safe-area-inset-bottom w-full">
      <div className="flex justify-around items-center pt-5 pb-0 px-2 w-full">
          {navItems.map((item) => {
            const active = isActive(item.path);
            return (
              <Button
                key={item.path}
                variant="ghost"
                asChild
                size="icon"
                className="h-16 w-16 flex-col gap-1 rounded-full touch-target relative"
              >
                <Link
                  to={item.path}
                  aria-current={active ? 'page' : undefined}
                  className={active ? 'text-primary' : 'text-muted-foreground'}
                >
                  {active && (
                    <div className="absolute -top-5 left-0 w-full h-1 bg-primary rounded-full" />
                  )}
                  <LineAwesomeIcon
                    icon={item.icon}
                    size="2xl"
                    className="-mt-[20px] text-3xl leading-none"
                  />
                  <span className="text-[11px] font-medium leading-none">{item.label}</span>
                </Link>
              </Button>
            );
          })}
      </div>
    </nav>
  );
}
