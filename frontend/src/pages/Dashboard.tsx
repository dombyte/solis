import React from 'react';
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
import { Page } from '../components/layout/Page';

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

  return (
    <Page title="Dashboard">
        {/* Power flow chart + energy cards share one grid, capped at 3 columns so the
            layout always settles into chart+Today on
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
        <div className="@container">
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
    </Page>
  );
}
