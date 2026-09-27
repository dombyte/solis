import React from 'react';
import { DataCard } from '../components/dashboard/DataCard';
import { PowerFlow } from '../components/dashboard/flow';
import { dashboardGroups } from '../lib/config/groups';
import { useSubscription } from '../lib/hooks/useSubscription';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { apiDataObjects } from '../lib/config/data';
import { useMobile } from '../hooks/useMobile';
import { SkeletonCard } from '../components/ui/skeleton';

// Dashboard shows every register, so it subscribes to the full key set.
const ALL_KEYS = apiDataObjects.map(obj => obj.key);

export function Dashboard(): React.ReactElement {
  useSubscription(ALL_KEYS);
  const isLoading = useRegisterStore(state => state.isLoading);
  const isMobile = useMobile();

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
      <div className="w-full overflow-x-hidden">
        {!isMobile ? <h1 className="text-xl sm:text-2xl font-bold mb-4 sm:mb-6 px-2">Dashboard</h1> : null}
        
        {/* Power Flow Diagram - shown first */}
        <div className="px-2 mb-3 sm:mb-4 md:mb-5 lg:mb-6">
          <PowerFlow />
        </div>
        
        {isLoading ? (
          <div className="flex flex-col gap-3 sm:gap-4 md:gap-5 lg:gap-6 px-2 pb-6 sm:pb-8 lg:pb-10">
            {energyGroups.length > 0 && (
              <div className="grid grid-cols-4-custom gap-3 sm:gap-4 md:gap-5 lg:gap-6 w-full">
                {energyGroups.map(group => (
                  <SkeletonCard key={group.id} className="w-full" />
                ))}
              </div>
            )}
            <div className="grid grid-cols-4-custom gap-3 sm:gap-4 md:gap-5 lg:gap-6">
              {otherGroups.map(group => (
                <SkeletonCard key={group.id} className="w-full" />
              ))}
            </div>
          </div>
        ) : (
          <div className="flex flex-col gap-3 sm:gap-4 md:gap-5 lg:gap-6 px-2 pb-6 sm:pb-8 lg:pb-10">
            {energyGroups.length > 0 && (
              <div className="grid grid-cols-4-custom gap-3 sm:gap-4 md:gap-5 lg:gap-6 w-full">
                {energyGroups.map(group => (
                  <DataCard key={group.id} group={group} />
                ))}
              </div>
            )}
            <div className="grid grid-cols-4-custom gap-3 sm:gap-4 md:gap-5 lg:gap-6">
              {otherGroups.map(group => (
                <DataCard key={group.id} group={group} />
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
