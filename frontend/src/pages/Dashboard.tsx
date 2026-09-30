import React from 'react';
import { DataCard } from '../components/dashboard/DataCard';
import { EnergyCard } from '../components/dashboard/EnergyCards';
import { EnergyCarousel } from '../components/dashboard/EnergyCarousel';
import { PowerFlow } from '../components/dashboard/flow';
import { dashboardGroups } from '../lib/config/groups';
import { useSubscription } from '../lib/hooks/useSubscription';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { apiDataObjects } from '../lib/config/data';
import { useMobile } from '../hooks/useMobile';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { SkeletonCard } from '../components/ui/skeleton';

// Dashboard shows every register, so it subscribes to the full key set.
const ALL_KEYS = apiDataObjects.map(obj => obj.key);

export function Dashboard(): React.ReactElement {
  useSubscription(ALL_KEYS);
  const isLoading = useRegisterStore(state => state.isLoading);
  const isMobile = useMobile();
  // Same phone rule as the power-flow diagram: coarse pointer and narrow viewport.
  // Tablets keep the grid.
  const isNarrow = useMediaQuery('(max-width: 767px)');
  const isPhone = isMobile && isNarrow;

  // Sort groups by order
  const sortedGroups = [...dashboardGroups].sort((a, b) => (a.order ?? 0) - (b.order ?? 0));

  // Separate energy groups (Today, Month, Year, Total) for full-width display
  const energyGroups = sortedGroups.filter(g =>
    ['energy_daily', 'energy_monthly', 'energy_yearly', 'energy_total'].includes(g.id)
  );

  // Other groups except power_flow (which is shown separately) and system_status (shown in inverter box)
  const otherGroups = sortedGroups.filter(g => 
    g.id !== 'system_status' && 
    g.id !== 'power_flow' &&
    !['energy_daily', 'energy_monthly', 'energy_yearly', 'energy_total'].includes(g.id)
  );

  return (
    <div className="p-2 sm:p-4 md:p-6 lg:p-8 w-full overflow-x-hidden">
      {/* Caps the whole page (hero row + card grids) together on ultra-wide monitors, so
          they stay aligned and the flow-chart hero doesn't turn into a shrinking island
          against an ever-widening grid as the viewport grows past typical desktop sizes. */}
      <div className="w-full max-w-[1800px] mx-auto overflow-x-hidden">
        {!isMobile ? <h1 className="text-xl sm:text-2xl font-bold mb-4 sm:mb-6 px-2">Dashboard</h1> : null}
        
        {/* Power flow chart + energy cards share one grid, capped at 3 columns (unlike
            the 4-column grid below) so the layout always settles into chart+Today on
            row one and Month/Year/Total on row two, instead of a 4th column pulling
            Month up next to the chart on very wide screens. The columns follow the
            space this grid actually gets (container queries), not the viewport: the
            desktop sidebar takes 256px, so a 1024px window only leaves tablet width,
            where three columns squeezed the chart and wrapped the energy values. Three
            columns start at a 56rem container, two at 36rem. The flow card spans 2
            columns under the same condition as the 2-column grid - with a span-2 in
            the single-column tier the browser creates an implicit 2nd column and
            auto-places the next card into it, breaking the stack. Default (stretch)
            row alignment plus h-full/centering in PowerFlow keeps every card in a row
            the same height with no dead gap next to the shorter ones. */}
        <div className="@container px-2 mb-3 sm:mb-4 md:mb-5 lg:mb-6">
          <div className="grid grid-cols-1 @xl:grid-cols-2 @4xl:grid-cols-3 gap-3 sm:gap-4 md:gap-5 lg:gap-6">
            <div className="@xl:col-span-2 min-w-0">
              <PowerFlow />
            </div>
            {isPhone ? (
              <EnergyCarousel groups={energyGroups} isLoading={isLoading} className="@xl:col-span-2" />
            ) : (
              energyGroups.map(group =>
                isLoading ? (
                  <SkeletonCard key={group.id} className="w-full" />
                ) : (
                  <EnergyCard key={group.id} group={group} />
                )
              )
            )}
          </div>
        </div>

        <div className="flex flex-col gap-3 sm:gap-4 md:gap-5 lg:gap-6 px-2 pb-6 sm:pb-8 lg:pb-10">
          <div className="grid grid-cols-4-custom gap-3 sm:gap-4 md:gap-5 lg:gap-6">
            {otherGroups.map(group =>
              isLoading ? (
                <SkeletonCard key={group.id} className="w-full" />
              ) : (
                <DataCard key={group.id} group={group} />
              )
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
