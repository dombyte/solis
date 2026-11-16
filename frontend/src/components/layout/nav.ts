import { Activity, ChartLine, FileText, House, Info, type LucideIcon } from 'lucide-react';

/** One page of the app: the sidebar, the bottom bar and the mobile header share it. */
export interface NavItem {
  path: string;
  label: string;
  icon: LucideIcon;
}

export const NAV_ITEMS: NavItem[] = [
  { path: '/dashboard', label: 'Dashboard', icon: House },
  { path: '/history', label: 'History', icon: ChartLine },
  { path: '/reports', label: 'Reports', icon: FileText },
  { path: '/status', label: 'Status', icon: Activity },
  { path: '/info', label: 'Info', icon: Info },
];

/** Whether item path is the current page ("/" is the dashboard). */
export function isNavActive(pathname: string, path: string): boolean {
  if (path === '/dashboard' && pathname === '/') return true;
  return pathname === path || pathname.startsWith(`${path}/`);
}

/** The title of the current page. */
export function pageTitle(pathname: string): string {
  return NAV_ITEMS.find(item => isNavActive(pathname, item.path))?.label ?? 'Solis Monitor';
}
