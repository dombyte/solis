import React from 'react';
import { DataCard } from '../components/dashboard/DataCard';
import { dashboardGroups } from '../lib/config/groups';
import { useWebSocket } from '../lib/hooks/useWebSocket';
import { useRegisterStore } from '../lib/stores/useRegisterStore';
import { useMobile } from '../hooks/useMobile';
import { SkeletonCard } from '../components/ui/skeleton';

export function Dashboard(): React.ReactElement {
  // Use WebSocket connection (initialized at app level)
  // requestInitialData: true to fetch fresh data when page mounts
  useWebSocket({ autoConnect: false, requestInitialData: true });
  const isLoading = useRegisterStore(state => state.isLoading);
  const isMobile = useMobile();

  // Sort groups by order
  const sortedGroups = [...dashboardGroups].sort((a, b) => (a.order ?? 0) - (b.order ?? 0));

  // Separate System Status from other groups and filter its dataIds
  const systemStatusGroup = sortedGroups.find(g => g.id === 'system_status');
  const filteredSystemStatusGroup = systemStatusGroup ? {
    ...systemStatusGroup,
    dataIds: systemStatusGroup.dataIds.filter(id => {
      // Always include these core status items
      if (['inverter_temp', 'solis_status', 'operating_status', 'grid_fault_1'].includes(id)) {
        return true;
      }

    }),
    layout: 'grid' as const,
  } : null;
  
  // Separate energy groups (Today, Month, Year, Total) for full-width display
  const energyGroups = sortedGroups.filter(g => 
    ['energy_daily', 'energy_monthly', 'energy_yearly', 'energy_total'].includes(g.id)
  );
  const otherGroups = sortedGroups.filter(g => 
    g.id !== 'system_status' && 
    !['energy_daily', 'energy_monthly', 'energy_yearly', 'energy_total'].includes(g.id)
  );

  return (
    <div className="p-2 sm:p-4 md:p-6 lg:p-8 w-full overflow-x-hidden">
      <div className="w-full overflow-x-hidden">
        {!isMobile ? <h1 className="text-xl sm:text-2xl font-bold mb-4 sm:mb-6 px-2">Dashboard</h1> : null}
        {isLoading ? (
          <div className="flex flex-col gap-3 sm:gap-4 md:gap-5 lg:gap-6 px-2 pb-6 sm:pb-8 lg:pb-10">
            {filteredSystemStatusGroup && (
              <SkeletonCard key={filteredSystemStatusGroup.id} className="w-full" />
            )}
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
            {filteredSystemStatusGroup && (
              <div className="w-full">
                <DataCard key={filteredSystemStatusGroup.id} group={filteredSystemStatusGroup} />
              </div>
            )}
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
