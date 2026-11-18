import { cn } from '../../lib/utils';

interface FaultBadgeProps {
  count: number;
  /** A small dot on the icon (collapsed sidebar, bottom bar) instead of a pill with the count. */
  dot?: boolean;
  className?: string;
}

/** Number of active inverter faults on the Status nav item; nothing without faults. */
export function FaultBadge({ count, dot = false, className }: FaultBadgeProps) {
  if (count === 0) return null;
  const label = `${count} active ${count === 1 ? 'fault' : 'faults'}`;
  if (dot) {
    return (
      <span
        role="img"
        aria-label={label}
        className={cn('absolute size-2.5 rounded-full bg-destructive ring-2 ring-background', className)}
      />
    );
  }
  return (
    <span
      aria-label={label}
      className={cn('ml-auto min-w-5 rounded-full bg-destructive px-1.5 text-center text-xs font-semibold leading-5 text-destructive-foreground tabular-nums', className)}
    >
      {count}
    </span>
  );
}
