import { cn } from '../../lib/utils';

/** Shared look of the period and compare dropdowns. */
export const TRIGGER_CLASS = cn(
  'w-full min-h-16 flex items-center gap-3 rounded-xl border border-border bg-background/80 px-3.5 py-2 text-left text-sm',
  'hover:bg-muted/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
  'data-[state=open]:border-primary',
);

// Solid: the theme's glass popover lets the bright download button shine through.
export const MENU_CLASS = 'bg-card max-w-[calc(100vw-1.5rem)]';

export const OPTION_CLASS = cn(
  'w-full flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-sm min-h-11',
  'hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
);
