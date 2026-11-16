import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card';
import { InfoPopover } from '../ui/info-popover';
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

  return (
    <Card className={`w-full min-w-0 flex flex-col ${className}`}>
      <CardHeader>
        <div className="flex items-center gap-2">
          <CardTitle className="text-base sm:text-lg">{group.title}</CardTitle>
          {group.description && (
            <InfoPopover label={group.title} iconClassName="h-4 w-4">
              <p>{group.description}</p>
            </InfoPopover>
          )}
        </div>
      </CardHeader>
      {/* A card stretched by its row (Today next to the flow diagram) spreads its rows
          over the height instead of leaving the lower half empty. */}
      <CardContent className="flex-1">
        <div className="grid grid-cols-1 gap-4 h-full content-evenly">
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
