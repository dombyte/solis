import { Link, useLocation } from 'react-router-dom';
import { NAV_ITEMS, isNavActive } from './nav';
import { Button } from '../ui/button';

export function MobileNav() {
  const location = useLocation();

  const isActive = (path: string) => isNavActive(location.pathname, path);

  return (
    <nav className="mobile-nav fixed bottom-0 left-0 right-0 glassy-nav-mobile z-50 w-full">
      <div className="flex justify-around items-start pt-2 px-2 w-full">
          {NAV_ITEMS.map((item) => {
            const active = isActive(item.path);
            return (
              <Button
                key={item.path}
                variant="ghost"
                asChild
                size="icon"
                className="h-16 w-16 flex-col justify-start gap-1.5 pt-2 rounded-xl touch-target relative"
              >
                <Link
                  to={item.path}
                  aria-current={active ? 'page' : undefined}
                  className={active ? 'text-primary' : 'text-muted-foreground'}
                >
                  {active && (
                    <div className="absolute -top-2 left-2 right-2 h-1 bg-primary rounded-full" />
                  )}
                  <item.icon className="size-7" aria-hidden="true" />
                  <span className="text-[11px] font-medium leading-none">{item.label}</span>
                </Link>
              </Button>
            );
          })}
      </div>
    </nav>
  );
}
