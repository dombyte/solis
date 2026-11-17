import type React from 'react';
import { useCompactLayout } from '../../hooks/useMobile';
import { cn } from '../../lib/utils';

interface PageProps {
  /** Page title; the compact layout shows it in the mobile header instead. */
  title: string;
  /** Optional line below the title. */
  description?: React.ReactNode;
  /** Optional controls on the right of the title (e.g. a refresh button). */
  actions?: React.ReactNode;
  className?: string;
  children: React.ReactNode;
}

/**
 * The shared page frame: one padding and one title style for every page, so switching
 * pages never moves the content or the heading. No maximum width: content starts at the
 * sidebar and uses the whole window (a centered cap left wide gaps on large screens).
 */
export function Page({ title, description, actions, className, children }: PageProps): React.ReactElement {
  const compact = useCompactLayout();
  const header = !compact || description || actions;

  return (
    <div className="px-3 py-3 sm:p-4 lg:p-6 w-full overflow-x-hidden">
      <div className={cn('w-full pb-4 flex flex-col gap-4', className)}>
        {header && (
          <header className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
            {/* No empty block (and its gap) in front of the actions when the compact
                layout shows the title in the mobile header. */}
            {(!compact || description) && (
              <div className="flex flex-col gap-1 min-w-0">
                {!compact && <h1 className="text-xl sm:text-2xl font-bold">{title}</h1>}
                {description && <p className="text-sm text-muted-foreground">{description}</p>}
              </div>
            )}
            {actions}
          </header>
        )}
        {children}
      </div>
    </div>
  );
}
