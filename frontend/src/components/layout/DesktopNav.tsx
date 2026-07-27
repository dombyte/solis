import { Link, useLocation } from 'react-router-dom';
import { useState } from 'react';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { cn } from '../../lib/utils';
import { LineAwesomeIcon } from '../ui/LineAwesomeIcon';
import { Button } from '../ui/button';
import { ThemeToggle } from './ThemeToggle';

export function DesktopNav() {
  const location = useLocation();
  const isLoading = useRegisterStore(state => state.isLoading);
  const [isCollapsed, setIsCollapsed] = useState(false);

  const isActive = (path: string) => {
    if (location.pathname === path) return true;
    if (path === '/dashboard' && (location.pathname === '/' || location.pathname.startsWith('/dashboard'))) return true;
    if (location.pathname.startsWith(path)) return true;
    return false;
  };

  const navItems = [
    { path: '/dashboard', label: 'Dashboard', icon: 'la-home' },
    { path: '/history', label: 'History', icon: 'la-chart-line' },
    { path: '/status', label: 'Status', icon: 'la-info-circle' },
    { path: '/info', label: 'Info', icon: 'la-file-alt' },
  ];

  if (isLoading) {
    return (
      <aside className="hidden md:flex w-16 bg-muted/70 backdrop-blur-2xl border-r border-border/50 z-40 flex-col flex-shrink-0">
        <div className="p-3 w-full">
          <span className="text-xs text-muted-foreground">Loading...</span>
        </div>
      </aside>
    );
  }

  return (
    <>
      {/* Modern Sidebar Navigation - Glassy transparent like mobile menu */}
      <aside 
        className={cn(
          "hidden md:flex flex-col z-40 transition-width duration-300 ease-in-out flex-shrink-0",
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
              {navItems.map((item) => (
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
                  <Link to={item.path} className="flex items-center justify-center w-full overflow-hidden">
                    <LineAwesomeIcon icon={item.icon} size="lg" />
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
              onClick={() => setIsCollapsed(!isCollapsed)}
            >
              <LineAwesomeIcon 
                icon={isCollapsed ? 'la-angle-double-right' : 'la-angle-double-left'} 
                size="lg" 
              />
            </Button>
          </div>
        </div>
      </aside>
    </>
  );
}
