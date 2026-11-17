import React from 'react';
import { Card, CardContent, CardHeader, CardTitle } from '../ui/card';
import { InfoPopover } from '../ui/info-popover';
import { ENERGY_ROWS } from '../../lib/config/data';
import { EnergyValue } from './EnergyValue';
import type { GroupConfig } from '../../types';

interface EnergyTableProps {
  /** The energy groups (Today, This Month, This Year, Total), one column each. */
  groups: GroupConfig[];
  className?: string;
}

/**
 * All energy figures in one table: a row per series, a column per period. Replaces four
 * cards that repeated the same nine labels; each label now appears once and the values
 * line up on the decimal point. Phones keep the swipe cards (four columns do not fit).
 */
export function EnergyTable({ groups, className = '' }: EnergyTableProps): React.ReactElement {
  return (
    <Card className={`@container w-full min-w-0 flex flex-col ${className}`}>
      <CardHeader className="flex-row items-baseline justify-between">
        <CardTitle>Energy</CardTitle>
        <span className="text-xs text-muted-foreground">kWh</span>
      </CardHeader>
      {/* h-full on the table: a card stretched by its row (next to the flow diagram)
          spreads its rows instead of leaving the lower part empty. */}
      <CardContent className="flex-1 overflow-x-auto">
        <table className="w-full h-full text-sm @[36rem]:text-base tabular-nums">
          <thead>
            <tr className="text-xs @[36rem]:text-sm text-muted-foreground">
              <th scope="col" className="pb-2 text-left font-medium"><span className="sr-only">Series</span></th>
              {groups.map(g => (
                <th key={g.id} scope="col" title={g.description} className="pb-2 pl-3 text-right font-medium whitespace-nowrap">
                  {g.title}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {ENERGY_ROWS.map(row => (
              <tr key={row.name} className="border-t border-border/70">
                <th scope="row" className="py-1.5 pr-2 text-left font-normal text-muted-foreground">
                  <span className="inline-flex items-center gap-1">
                    <span className="whitespace-nowrap">{row.name}</span>
                    <InfoPopover label={row.name} align="start">
                      <p>{row.description}</p>
                    </InfoPopover>
                  </span>
                </th>
                {row.ids.map(id => (
                  <td key={id} className="py-1.5 pl-3 text-right font-semibold">
                    <EnergyValue id={id} />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </CardContent>
    </Card>
  );
}
