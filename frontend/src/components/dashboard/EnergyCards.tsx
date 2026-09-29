import React from 'react';
import { Info } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card';
import { Popover, PopoverTrigger, PopoverContent } from '../ui/popover';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { SkeletonCard } from '../ui/skeleton';
import { ValueDisplay } from './ValueDisplay';
import type { GroupConfig } from '../../types';

interface EnergyCardProps {
  group: GroupConfig;
  className?: string;
}

// Energy card (Today / Month / Year / Total): always shows every dataId. On phones the
// cards sit side by side in EnergyCarousel instead of being collapsed.
export function EnergyCard({ group, className = '' }: EnergyCardProps): React.ReactElement | null {
  const registerMetadata = useRegisterStore(state => state.registerMetadata);

  const validDataIds = group.dataIds.filter((dataId: string) => registerMetadata.get(dataId) !== undefined);

  if (validDataIds.length === 0) {
    return <SkeletonCard className={`w-full ${className}`} />;
  }

  const gridClass = group.layout === 'grid' ? 'grid-cols-4-custom' : 'grid-cols-1';

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
          {validDataIds.map((dataId: string) => (
            <ValueDisplay
              key={dataId}
              dataId={dataId}
              showLabel
              showUnit
              showTooltip
            />
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
