import type { StatusLevel } from '../../lib/utils/status';

/** Fill of a status level (strip segments, dots). */
export const LEVEL_BG: Record<StatusLevel, string> = {
  ok: 'bg-success',
  notice: 'bg-warning',
  fault: 'bg-destructive',
  unknown: 'bg-muted-foreground/30',
};

/** Text color of a status level. */
export const LEVEL_TEXT: Record<StatusLevel, string> = {
  ok: 'text-success',
  notice: 'text-warning',
  fault: 'text-destructive',
  unknown: 'text-muted-foreground',
};
