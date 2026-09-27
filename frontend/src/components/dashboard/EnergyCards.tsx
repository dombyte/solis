import React, { useState } from 'react';
import { ChevronDown, ChevronUp, Info } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card';
import { Popover, PopoverTrigger, PopoverContent } from '../ui/popover';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { useMobile } from '../../hooks/useMobile';
import { SkeletonCard } from '../ui/skeleton';
import { ValueDisplay } from './ValueDisplay';
import type { GroupConfig } from '../../types';

interface EnergyCardProps {
  group: GroupConfig;
  className?: string;
}

// Mobile-collapsible energy card (spec §14.3): mobile shows the first dataId
// (always PV Energy, per groups.ts ordering) by default, the rest expand on tap.
// Desktop is unaffected and always shows every dataId, same as DataCard.
export function EnergyCard({ group, className = '' }: EnergyCardProps): React.ReactElement | null {
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const isMobile = useMobile();
  const [expanded, setExpanded] = useState(false);

  const validDataIds = group.dataIds.filter((dataId: string) => registerMetadata.get(dataId) !== undefined);

  if (validDataIds.length === 0) {
    return <SkeletonCard className={`w-full ${className}`} />;
  }

  const gridClass = group.layout === 'grid' ? 'grid-cols-4-custom' : 'grid-cols-1';

  const [primaryId, ...restIds] = validDataIds;
  const showAll = !isMobile || expanded;
  const visibleIds = showAll ? validDataIds : [primaryId];

  return (
    <Card className={`w-full min-w-0 ${className}`}>
      <CardHeader>
        <div className="flex items-center gap-2">
          <CardTitle className="text-base sm:text-lg">{group.title}</CardTitle>
          {group.description && (
            <Popover>
              <PopoverTrigger asChild>
                <button
                  type="button"
                  className="text-muted-foreground hover:text-foreground hover:bg-muted/50 transition-colors rounded-sm p-0.5"
                  aria-label={`Info about ${group.title}`}
                >
                  <Info className="h-4 w-4" />
                </button>
              </PopoverTrigger>
              <PopoverContent className="w-72 max-w-[300px]" align="center" sideOffset={8}>
                <p className="text-sm text-popover-foreground whitespace-normal break-words">{group.description}</p>
              </PopoverContent>
            </Popover>
          )}
        </div>
      </CardHeader>
      <CardContent>
        <div className={`grid gap-4 ${gridClass}`}>
          {visibleIds.map((dataId: string) => (
            <ValueDisplay
              key={dataId}
              dataId={dataId}
              showLabel
              showUnit
              showTooltip
            />
          ))}
        </div>
        {isMobile && restIds.length > 0 && (
          <button
            type="button"
            onClick={() => setExpanded(e => !e)}
            className="mt-3 flex w-full items-center justify-center gap-1 text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
          >
            {expanded ? (
              <>Show less <ChevronUp className="h-3.5 w-3.5" /></>
            ) : (
              <>Show {restIds.length} more <ChevronDown className="h-3.5 w-3.5" /></>
            )}
          </button>
        )}
      </CardContent>
    </Card>
  );
}
