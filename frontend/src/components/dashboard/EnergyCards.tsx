import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card';
import { InfoTerm } from '../ui/info-popover';
import { useRegisterStore } from '../../lib/stores/useRegisterStore';
import { SkeletonCard } from '../ui/skeleton';
import { EnergyValue } from './EnergyValue';
import type { GroupConfig } from '../../types';

interface EnergyCardProps {
  group: GroupConfig;
  className?: string;
}

// Energy card of one period (Today / Month / Year / Total), used by the phone carousel:
// label left, value right-aligned so the figures line up, one divider per row.
export function EnergyCard({ group, className = '' }: EnergyCardProps): React.ReactElement {
  const registerMetadata = useRegisterStore(state => state.registerMetadata);

  const registers = group.dataIds.flatMap(id => {
    const reg = registerMetadata.get(id);
    return reg ? [reg] : [];
  });

  if (registers.length === 0) {
    return <SkeletonCard className={`w-full ${className}`} />;
  }

  return (
    <Card className={`w-full min-w-0 flex flex-col ${className}`}>
      <CardHeader className="flex-row items-center gap-2">
        <CardTitle>{group.title}</CardTitle>
      </CardHeader>
      <CardContent className="flex-1">
        <dl className="text-sm tabular-nums">
          {registers.map(reg => (
            <div key={reg.id} className="flex items-center justify-between gap-3 border-t border-border/70 py-2">
              <dt className="flex min-w-0 items-center gap-1 text-muted-foreground">
                {reg.description ? (
                  <InfoTerm label={reg.name} info={<p>{reg.description}</p>} className="truncate">
                    {reg.name}
                  </InfoTerm>
                ) : (
                  <span className="truncate">{reg.name}</span>
                )}
              </dt>
              <dd className="font-semibold">
                <EnergyValue id={reg.id} showUnit />
              </dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  );
}
