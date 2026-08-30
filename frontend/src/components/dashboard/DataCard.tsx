import React from 'react';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card';

import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { SkeletonCard } from '../ui/skeleton';
import { ValueDisplay } from './ValueDisplay';
import { StatusDisplay } from './StatusDisplay';
import { Popover, PopoverTrigger, PopoverContent } from '../ui/popover';
import { Info } from 'lucide-react';
import type { GroupConfig } from '../../types';

interface DataCardProps {
  group: GroupConfig;
  className?: string;
}

export function DataCard({ group, className = '' }: DataCardProps): React.ReactElement | null {
  const registerMetadata = useRegisterStore(state => state.registerMetadata);
  const showTooltips = true;
  
  // Filter out dataIds that don't have metadata
  const validDataIds = group.dataIds.filter((dataId: string) => {
    return registerMetadata.get(dataId) !== undefined;
  });

  // Show skeleton if we don't have metadata yet
  if (validDataIds.length === 0) {
    return <SkeletonCard className={`w-full ${className}`} />;
  }

  // Use auto-fit grid for dynamic column count with min width
  // For grid layout, use responsive columns: 1-2-3-4 based on screen width
  // 1 col: <640px, 2 cols: 640-999px, 3 cols: 1000-1899px, 4 cols: 1900px+ (custom CSS class)
  const gridClass = group.layout === 'grid'
    ? 'grid-cols-4-custom'
    : 'grid-cols-1';

  // For status groups, use StatusDisplay component
  const isStatusGroup = group.category === 'status';

  return (
    <Card className={`w-full min-w-0 ${className}`}>
      <CardHeader>
        <div className="flex items-center gap-2">
          <CardTitle className="text-base sm:text-lg">{group.title}</CardTitle>
          {showTooltips && group.description && (
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
        {group.description && !showTooltips && (
          <CardDescription>{group.description}</CardDescription>
        )}
      </CardHeader>
      <CardContent>
        <div className={`grid gap-4 ${gridClass}`}>
          {validDataIds.map((dataId: string) => (
              isStatusGroup ? (
                <StatusDisplay 
                  key={dataId}
                  dataId={dataId} 
                  showLabel 
                  showTooltip={showTooltips}
                />
              ) : (
                <ValueDisplay 
                  key={dataId}
                  dataId={dataId} 
                  showLabel 
                  showUnit 
                  showStatusIndicator={isStatusGroup}
                  showTooltip={showTooltips}
                />
              )
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
