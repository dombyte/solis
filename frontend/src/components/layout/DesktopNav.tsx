import { Link, useLocation } from 'react-router-dom';
import { useCallback, useEffect, useState } from 'react';
import { cn } from '../../lib/utils';
import { ChevronsLeft, ChevronsRight } from 'lucide-react';
import { NAV_ITEMS, isNavActive } from './nav';
import { Button } from '../ui/button';
import { ThemeToggle } from './ThemeToggle';

// Per-browser convenience; storage can throw (private mode), then it is just not kept.
const COLLAPSED_KEY = 'solis.sidebar.collapsed';

function loadCollapsed(): boolean {
  try {
    return window.localStorage.getItem(COLLAPSED_KEY) === '1';
  } catch {
    return false;
  }
}

function saveCollapsed(collapsed: boolean): void {
  try {
    if (collapsed) window.localStorage.setItem(COLLAPSED_KEY, '1');
    else window.localStorage.removeItem(COLLAPSED_KEY);
  } catch {
    // not persisted; the in-memory state still applies
  }
}

// The links never depend on data, so the sidebar renders in full at once (a narrow
// "Loading..." stub that grew to full width shifted the whole page on every load).
export function DesktopNav() {
  const location = useLocation();
  const [isCollapsed, setIsCollapsed] = useState(loadCollapsed);
  useEffect(() => saveCollapsed(isCollapsed), [isCollapsed]);
  const toggleCollapsed = useCallback(() => setIsCollapsed(c => !c), []);


  const isActive = (path: string) => isNavActive(location.pathname, path);

  return (
    <>
      {/* Modern Sidebar Navigation - Glassy transparent like mobile menu */}
      <aside 
        className={cn(
          "h-full flex flex-col z-40 transition-width duration-300 ease-in-out flex-shrink-0",
          isCollapsed ? "w-16" : "w-64"
        )}
      >
        {/* Sidebar content container with glassy effect */}
        <div className="flex flex-col h-full bg-muted/70 backdrop-blur-2xl border-r border-border/50 overflow-y-auto overflow-x-hidden w-full">
          {/* Header with app name and theme toggle (visible when expanded) */}
          <div className={cn("p-4 border-b border-border/30 transition-all duration-300 overflow-hidden flex items-center justify-between", isCollapsed ? "p-2 justify-center" : "p-4")}>
            {!isCollapsed && (
              <h1 className="text-lg font-bold text-foreground truncate whitespace-nowrap">Solis Monitor</h1>
            )}
            {!isCollapsed && (
              <div className="flex-shrink-0">
                <ThemeToggle />
              </div>
            )}
          </div>

          {/* Navigation items */}
          <div className="flex-1 px-2 py-4 space-y-1 overflow-y-auto overflow-x-hidden">
              {NAV_ITEMS.map((item) => (
                <Button
                  key={item.path}
                  variant={isActive(item.path) ? 'default' : 'ghost'}
                  asChild
                  size="sm"
                  className={cn(
                    "w-full h-10 overflow-hidden",
                    isCollapsed ? "justify-center" : "justify-start gap-2"
                  )}
                >
                  <Link
                    to={item.path}
                    aria-current={isActive(item.path) ? 'page' : undefined}
                    className="flex items-center justify-center w-full overflow-hidden"
                  >
                    <item.icon className="size-5 shrink-0" aria-hidden="true" />
                    {!isCollapsed && (
                      <span className="text-sm ml-2 whitespace-nowrap">{item.label}</span>
                    )}
                  </Link>
                </Button>
              ))}
          </div>

          {/* Collapse toggle button */}
          <div className="p-2 border-t border-border/30 overflow-hidden">
            <Button
              variant="ghost"
              size="icon"
              className="w-full h-8"
              onClick={toggleCollapsed}
              aria-label={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            >
              {isCollapsed
                ? <ChevronsRight className="size-5" aria-hidden="true" />
                : <ChevronsLeft className="size-5" aria-hidden="true" />}
              <span className="sr-only">{isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}</span>
            </Button>
          </div>
        </div>
      </aside>
    </>
  );
}
