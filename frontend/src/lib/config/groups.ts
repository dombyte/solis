import type { GroupConfig } from '../../types';
import { apiDataObjects } from './data';

/**
 * Register ids of a group, from data.ts (`group` + `order`) - the single source of truth
 * for group membership (review FE-L10).
 */
function idsOf(group: string): string[] {
  return apiDataObjects
    .filter(o => o.group === group)
    .sort((a, b) => (a.order ?? 0) - (b.order ?? 0))
    .map(o => o.id);
}

// Dashboard group configurations (members come from data.ts)
const groupDefs: Omit<GroupConfig, 'dataIds'>[] = [
  {
    id: 'power_flow',
    title: 'Power Flow',
    description: 'Live power flow visualization',
    category: 'power_flow',
    layout: 'grid',
    order: 1,
  },
  {
    id: 'system_status',
    title: 'System Status',
    description: 'System status and fault information',
    category: 'status',
    layout: 'list',
    order: 2,
  },
  {
    id: 'energy_daily',
    title: "Today",
    description: "Energy produced and consumed today",
    category: 'energy',
    layout: 'list',
    order: 2,
  },
  {
    id: 'energy_monthly',
    title: "This Month",
    description: "Energy produced and consumed this month",
    category: 'energy',
    layout: 'list',
    order: 3,
  },
  {
    id: 'energy_yearly',
    title: "This Year",
    description: "Energy produced and consumed this year",
    category: 'energy',
    layout: 'list',
    order: 4,
  },
  {
    id: 'energy_total',
    title: 'Total',
    description: 'Energy produced and consumed since monitoring started',
    category: 'energy',
    layout: 'list',
    order: 5,
  },
];

export const dashboardGroups: GroupConfig[] = groupDefs.map(g => ({ ...g, dataIds: idsOf(g.id) }));

// History data groups - which register IDs are available for each period
export const historyDataGroups: Record<'daily' | 'monthly' | 'yearly', string[]> = {
  daily: idsOf('energy_daily'),
  monthly: idsOf('energy_monthly'),
  yearly: idsOf('energy_yearly'),
};
