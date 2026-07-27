import type { GroupConfig } from '../../types';

// Dashboard group configurations
export const dashboardGroups: GroupConfig[] = [
  {
    id: 'system_status',
    title: 'System Status',
    description: 'System status and fault information',
    dataIds: ['inverter_temp', 'solis_status', 'operating_status', 'grid_fault_status', 'battery_1_bms_fault', 'battery_2_bms_fault', 'backup_load_fault', 'battery_fault_03', 'device_fault_04', 'device_fault_05'],
    category: 'status',
    layout: 'list',
    order: 1,
  },
  {
    id: 'power_current',
    title: 'Power & Battery',
    description: 'Current power readings and battery status',
    dataIds: ['pv_power_total', 'active_power', 'household_load_power', 'backup_load_power', 'meter_total_active_power', 'battery_soc', 'battery_soh', 'battery_power'],
    category: 'power',
    layout: 'grid',
    columns: 2,
    order: 2,
  },
  {
    id: 'energy_daily',
    title: "Today",
    description: "Today's energy production and consumption",
    dataIds: ['pv_energy_today', 'household_energy_today', 'backup_energy_today', 'energy_consumption_today', 'energy_fed_today', 'energy_imported_today', 'battery_discharge_today', 'battery_charge_today', 'grid_energy_today'],
    category: 'energy',
    layout: 'list',
    order: 3,
  },
  {
    id: 'energy_monthly',
    title: "This Month's Energy",
    description: "Monthly energy summary",
    dataIds: ['pv_energy_month', 'household_energy_month', 'backup_energy_month', 'energy_consumption_month', 'energy_fed_month', 'energy_imported_month', 'battery_discharge_month', 'battery_charge_month', 'grid_energy_month'],
    category: 'energy',
    layout: 'list',
    order: 4,
  },
  {
    id: 'energy_yearly',
    title: "This Year's Energy",
    description: "Yearly energy summary",
    dataIds: ['pv_energy_year', 'household_energy_year', 'backup_energy_year', 'energy_consumption_year', 'energy_fed_year', 'energy_imported_year', 'battery_discharge_year', 'battery_charge_year', 'grid_energy_year'],
    category: 'energy',
    layout: 'list',
    order: 5,
  },
  {
    id: 'energy_total',
    title: 'Total Energy',
    description: 'Lifetime energy statistics',
    dataIds: ['pv_energy_total', 'household_energy_total', 'backup_energy_total', 'energy_consumption_total', 'energy_fed_total', 'energy_imported_total', 'battery_discharge_total', 'battery_charge_total', 'grid_energy_total'],
    category: 'energy',
    layout: 'list',
    order: 6,
  },
];

// History data groups - which register IDs are available for each period
export const historyDataGroups: Record<'daily' | 'monthly' | 'yearly', string[]> = {
  daily: [
    'pv_energy_today',
    'household_energy_today',
    'backup_energy_today',
    'energy_consumption_today',
    'energy_fed_today',
    'energy_imported_today',
    'battery_discharge_today',
    'battery_charge_today',
    'grid_energy_today'
  ],
  monthly: [
    'pv_energy_month',
    'household_energy_month',
    'backup_energy_month',
    'energy_consumption_month',
    'energy_fed_month',
    'energy_imported_month',
    'battery_discharge_month',
    'battery_charge_month',
    'grid_energy_month'
  ],
  yearly: [
    'pv_energy_year',
    'household_energy_year',
    'backup_energy_year',
    'energy_consumption_year',
    'energy_fed_year',
    'energy_imported_year',
    'battery_discharge_year',
    'battery_charge_year',
    'grid_energy_year'
  ],
};
