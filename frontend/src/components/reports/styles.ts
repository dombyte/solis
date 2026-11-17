import { cn } from '../../lib/utils';

/** Shared look of the period and compare dropdowns. */
export const TRIGGER_CLASS = cn(
  'w-full min-h-16 flex items-center gap-3 rounded-xl border border-border bg-control px-3.5 py-2 text-left text-sm',
  'hover:bg-control-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
  'data-[state=open]:border-primary',
);

// Solid: the theme's glass popover lets the bright download button shine through.
export const MENU_CLASS = 'bg-card max-w-[calc(100vw-1.5rem)]';

/**
 * Touch devices: a menu as wide as its trigger (lined up with it), at most the screen, and
 * never reaching under the fixed bottom nav (it scrolls inside itself instead).
 */
export const TOUCH_MENU_CLASS = cn(
  'w-[min(var(--radix-popover-trigger-width),calc(100vw-1.5rem))]',
  'max-h-[calc(var(--radix-popover-content-available-height)-var(--mobile-nav-height))] overflow-y-auto',
);

export const OPTION_CLASS = cn(
  'w-full flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm min-h-11',
  'hover:bg-control-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
);
