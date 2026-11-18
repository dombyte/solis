import React from 'react';
import { EnergyTable } from '../components/dashboard/EnergyTable';
import { EnergyTrend } from '../components/dashboard/EnergyTrend';
import { EnergyTrendList } from '../components/dashboard/EnergyTrendList';
import { EnergyCarousel } from '../components/dashboard/EnergyCarousel';
import { PowerFlow } from '../components/dashboard/flow';
import { dashboardGroups } from '../lib/config/groups';
import { useSubscription } from '../lib/hooks/useSubscription';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { apiDataObjects } from '../lib/config/data';
import { useCompactLayout, useMobile } from '../hooks/useMobile';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { SkeletonCard } from '../components/ui/skeleton';
import { Page } from '../components/layout/Page';
import { FaultBanner } from '../components/dashboard/FaultBanner';

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
  const compact = useCompactLayout();

  // Sort groups by order
  const sortedGroups = [...dashboardGroups].sort((a, b) => (a.order ?? 0) - (b.order ?? 0));

  // Separate energy groups (Today, Month, Year, Total) for full-width display
  const energyGroups = sortedGroups.filter(g =>
    ['energy_daily', 'energy_monthly', 'energy_yearly', 'energy_total'].includes(g.id)
  );

  return (
    <Page title="Dashboard">
        <FaultBanner />
        {/* Flow diagram and energy table side by side once the space this grid gets
            (container query, not the viewport: the desktop sidebar takes 256px) fits the
            table's four value columns next to the diagram (60rem); stacked below that. The
            diagram stops growing at ~600px (FlowDesktop), so from 110rem its column is
            fixed and the table takes the rest. The 14-day trend spans the full width
            below and takes the height the window has left.
            Default (stretch) row alignment keeps both cards equally tall; the diagram
            centers itself and the table spreads its rows over the height. Phones stack
            the diagram, the 7-day list (one row per day instead of 14 bar pairs squeezed
            into the width) and the swipe cards (one per period instead of the
            four-column table). */}
        <div className="@container">
          {isPhone ? (
            <div className="grid grid-cols-1 gap-4">
              <div className="min-w-0">
                <PowerFlow />
              </div>
              <EnergyTrendList />
              <EnergyCarousel groups={energyGroups} isLoading={isLoading} />
            </div>
          ) : (
            <div className="grid grid-cols-1 @[60rem]:grid-cols-2 @[110rem]:grid-cols-[48rem_minmax(0,1fr)] gap-4">
              <div className="min-w-0">
                <PowerFlow />
              </div>
              {isLoading ? <SkeletonCard className="w-full" /> : <EnergyTable groups={energyGroups} />}
              {/* Two weeks on desktop, one on tablets. */}
              <EnergyTrend days={compact ? 7 : 14} className="@[60rem]:col-span-2" />
            </div>
          )}
        </div>
    </Page>
  );
}
