# Solis Monitor

Monitoring solution for Solis inverters using Modbus TCP/RTU. Polls register data, stores it in SQLite, and exposes it via a web API, frontend dashboard and Prometheus metrics.

## Desktop
<img width="2554" height="1299" alt="solis02" src="https://github.com/user-attachments/assets/df3be693-1289-4a87-8aa1-fad199e92220" />

<img width="2548" height="1299" alt="solis03" src="https://github.com/user-attachments/assets/de4beb8f-04eb-406b-ae7f-a67bacd320c1" />

## Mobile

<img width="397" height="873" alt="solis04" src="https://github.com/user-attachments/assets/cc011509-130c-4bb1-93ca-b9e0eee2162e" />


## API

After starting, the API Docs are available at `/docs`.

## Health Check
A health check endpoint is available at `/health`.

## Metrics
If enabled, Prometheus metrics are exposed at `/metrics`.

## Configuration

Copy `config.yaml` and adjust settings. All options can be overridden via environment variables using the `SOLIS_` prefix (e.g., `SOLIS_MODBUS_HOST=192.168.1.200`).

### Example Configuration

```yaml
# Solis-Monitor Configuration
app:
  debug: INFO      # DEBUG, INFO, WARN, ERROR, FATAL
  port: 8080
  timeout: 30s
  serve_only: false  # Set to true to disable Modbus polling (serve-only mode)

poller:
  interval: 30s
  block_attempts: 3
  block_retry_delay: 1s
  block_interval: 0s
  poll_timeout: 30s

modbus:
  type: tcp
  host: 192.168.2.151
  port: 502
  timeout: 5s
  unit_id: 1
  # For RTU:
  # type: rtu
  # serial_port: /dev/ttyUSB0
  # baud_rate: 9600
  # data_bits: 8
  # stop_bits: 1
  # parity: none

storage:
  path: ./data/solis.db
  daily_retention: 87600h  # 10 years
  monthly_retention: 87600h
  yearly_retention: 87600h
  error_retention: 720h   # 30 days
  wal_mode: true
  synchronous: NORMAL
  temp_store: MEMORY

metrics:
  enabled: true

registers:
  disabled_keys: []
  # Example: disable specific registers
  # disabled_keys:
  #   - meter_total_active_power
```

### App Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `debug` | string | INFO | Log level: DEBUG, INFO, WARN, ERROR, FATAL |
| `port` | int | 8080 | HTTP server port (1-65535) |
| `timeout` | duration | 30s | Request timeout |
| `serve_only` | bool | false | Run in serve-only mode (disables Modbus polling, uses cached/stored data only) |

### Serve-Only Mode

Serve-only mode allows running the Solis Monitor **without** connecting to the inverter. This is useful for:

- Running a read-only reporting/analytics instance
- Deploying a backup instance that only serves stored data
- Running the API when the inverter is temporarily unavailable
- Containerized deployments where Modbus is not available

**How it works:**
- HTTP API, WebSocket, and Prometheus metrics all function normally
- Historical data (daily, monthly, yearly, total energy) is served from SQLite
- Current register values (PV voltage, battery SOC, power, etc.) are served from cache
- Modbus connection and background polling are completely disabled

**Important limitation:** Current register values are stored in memory (cache) only. In serve-only mode, the cache starts empty. To have current values available:
1. First run in normal mode to poll and cache values from the inverter
2. Then switch to serve-only mode (cache persists in memory until restart)
3. Or ensure another instance is polling and updating the database

**Configuration:**
```yaml
app:
  serve_only: true
```

Or via environment variable:
```bash
SOLIS_APP_SERVE_ONLY=true ./solis
```

**Health check in serve-only mode:**
```json
{
  "modbus_connected": "disabled",
  "poller_running": "disabled",
  "storage": "ok",
  "status": "ok"
}
```

### Poller Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `interval` | duration | 30s | Base interval between poll cycles |
| `block_attempts` | int | 3 | Retry attempts per block if read fails |
| `block_retry_delay` | duration | 1s | Delay between retry attempts for same block |
| `block_interval` | duration | 0s | Delay between successive block reads (0 = immediate) |
| `poll_timeout` | duration | 30s | Maximum duration for full poll cycle before aborting |

### Modbus Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `type` | string | tcp | Connection type: tcp, rtu, or rtu_over_tcp |
| `host` | string | 192.168.1.100 | Modbus server IP/hostname (TCP only) |
| `port` | int | 502 | Modbus server port (1-65535) |
| `timeout` | duration | 5s | Connection/read timeout |
| `unit_id` | byte | 1 | Modbus unit/slave ID (1-247) |
| `serial_port` | string | | Serial port for RTU (e.g., /dev/ttyUSB0) |
| `baud_rate` | int | | Baud rate for RTU |
| `data_bits` | int | | Data bits for RTU (5, 6, 7, 8) |
| `stop_bits` | int | | Stop bits for RTU (1, 2) |
| `parity` | string | | Parity for RTU: none, even, odd |

### Storage Settings (SQLite)

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `path` | string | ./data/solis.db | Path to SQLite database file |
| `daily_retention` | duration | 8760h | Retention for daily energy values |
| `monthly_retention` | duration | 8760h | Retention for monthly energy values |
| `yearly_retention` | duration | 8760h | Retention for yearly energy values |
| `error_retention` | duration | 720h | Retention for error/fault data (30 days) |
| `wal_mode` | bool | true | Enable Write-Ahead Logging |
| `synchronous` | string | NORMAL | Sync mode: OFF, NORMAL, FULL, EXTRA |
| `temp_store` | string | MEMORY | Temp storage: DEFAULT, FILE, MEMORY |
| `enable_migrations` | bool | true | Enable automatic schema migrations on startup |
| `enable_backup` | bool | true | Enable database backup functionality |
| `max_backups` | int | 3 | Maximum number of backup files to keep (0 = unlimited) |
| `backup_interval` | duration | 24h | Interval for periodic backups |

### Database Maintenance

The application includes a comprehensive database lifecycle management system that automatically handles migrations, backups, and cleanup on startup.

#### Schema Migrations

- **Automatic Version Detection**: The application tracks the current database schema version in a `schema_version` table
- **Forward-Only Migrations**: Each version increment adds new schema changes while preserving existing data
- **Legacy Database Support**: Pre-migration databases are automatically detected and marked as version 1
- **Migration Safety**: Database is backed up before any migration is applied

#### Backup System

**Automatic Backups**: All backups use the same simplified format:
- Filename format: `{database}.{timestamp}.backup`
- Example: `solis.db.20260627_143022.backup`
- Created at startup if database file exists (before any migration)
- Created periodically at the interval specified by `backup_interval`
- Runs in background without affecting application performance

**Backup Management**:
- Automatic cleanup keeps only the most recent `max_backups` files
- Old backups are removed oldest first
- Set `max_backups: 0` to disable automatic cleanup (keep all backups)
- Backup files are stored in a `backups` subdirectory of the database directory

**Backup Verification**:
- Each backup is verified to have the same size as the source database
- Empty backups are rejected and deleted
- All backup operations are logged with timestamps and file sizes

#### Configuration Examples

**Daily backups with 7-day retention**:
```yaml
storage:
  enable_backup: true
  max_backups: 7
  backup_interval: 24h
```

**Frequent backups with unlimited retention**:
```yaml
storage:
  enable_backup: true
  max_backups: 0  # Keep all backups
  backup_interval: 6h
```

**Disable backup functionality**:
```yaml
storage:
  enable_backup: false
```

**Disable migrations (not recommended)**:
```yaml
storage:
  enable_migrations: false
```

#### Manual Backup Management

Backup files can be manually managed:
- **Copy**: Backup files can be copied to other locations for offsite storage
- **Restore**: Copy a backup file over the database file and restart the application
- **Delete**: Old backups can be safely deleted (application will recreate as needed)

#### Migration Process

1. **Startup**: Application checks database schema version
2. **Backup**: If database file exists, a backup is created (regardless of migration need)
3. **Migration**: If needed, pending migrations are applied in order
4. **Verification**: Schema version is updated and verified
5. **Cleanup**: Old backups beyond `max_backups` are removed
6. **Initialization**: Storage is initialized with the updated schema

#### Troubleshooting

**Application fails to start with migration error**:
- Check logs for specific error message
- Restore from the latest backup file: `cp backups/solis.db.20260627_143022.backup solis.db`
- Ensure database file has proper permissions

**Backup files accumulating**:
- Increase `max_backups` to keep more backups
- Set `max_backups: 0` to keep all backups
- Manually delete old backups if needed

**Migrations taking too long**:
- Migration only runs when necessary (schema version mismatch)
- Complex migrations may take time; this is normal for large databases
- Check logs for migration progress

### Metrics Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `enabled` | bool | false | Enable Prometheus metrics endpoint |

### Registers Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `disabled_keys` | []string | [] | Register keys to disable (exclude from polling, storage, API) |

## Available Registers

All registers are polled by default. Disable specific ones via `registers.disabled_keys` in config.

### Information Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| solis_model_no | Solis Model No | 33000 | Uint16 | 1.0 | | stable |
| solis_dsp_version | Solis DSP Version | 33001 | Uint16 | 1.0 | | stable |
| solis_hmi_version | Solis HMI Version | 33002 | Uint16 | 1.0 | | stable |
| solis_protocol_version | Solis Protocol Version | 33003 | Uint16 | 1.0 | | stable |
| solis_serial_number | Solis Serial Number | 33004 | String | 1.0 | | stable |

### Energy Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| pv_today_energy | Solis PV Today Energy Generation | 33035 | Uint16 | 0.1 | kWh | dynamic |
| pv_month_energy | Solis PV Current Month Energy Generation | 33031 | Uint32 | 1 | kWh | dynamic |
| pv_year_energy | Solis PV This Year Energy Generation | 33037 | Uint32 | 1 | kWh | dynamic |
| pv_total_energy | Solis PV Total Energy Generation | 33029 | Uint32 | 1 | kWh | dynamic |

### PV Voltage/Current Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| pv_voltage_1 | Solis PV Voltage 1 | 33049 | Uint16 | 0.1 | V | dynamic |
| pv_current_1 | Solis PV Current 1 | 33050 | Uint16 | 0.1 | A | dynamic |
| pv_voltage_2 | Solis PV Voltage 2 | 33051 | Uint16 | 0.1 | V | dynamic |
| pv_current_2 | Solis PV Current 2 | 33052 | Uint16 | 0.1 | A | dynamic |
| pv_voltage_3 | Solis PV Voltage 3 | 33053 | Uint16 | 0.1 | V | dynamic |
| pv_current_3 | Solis PV Current 3 | 33054 | Uint16 | 0.1 | A | dynamic |
| pv_voltage_4 | Solis PV Voltage 4 | 33055 | Uint16 | 0.1 | V | dynamic |
| pv_current_4 | Solis PV Current 4 | 33056 | Uint16 | 0.1 | A | dynamic |

### PV Power and Bus Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| total_pv_power | Solis Total PV Power | 33057 | Uint32 | 1 | W | dynamic |

### Grid Voltage/Current Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| a_phase_voltage | Solis A Phase Voltage | 33073 | Uint16 | 0.1 | V | dynamic |
| b_phase_voltage | Solis B Phase Voltage | 33074 | Uint16 | 0.1 | V | dynamic |
| c_phase_voltage | Solis C Phase Voltage | 33075 | Uint16 | 0.1 | V | dynamic |
| a_phase_current | Solis A Phase Current | 33076 | Uint16 | 0.1 | A | dynamic |
| b_phase_current | Solis B Phase Current | 33077 | Uint16 | 0.1 | A | dynamic |
| c_phase_current | Solis C Phase Current | 33078 | Uint16 | 0.1 | A | dynamic |
| active_power | Solis Active Power | 33079 | Uint32 | 1 | W | dynamic |
| reactive_power | Solis Reactive Power | 33081 | Int32 | 0.1 | VAR | dynamic |
| apparent_power | Solis Apparent Power | 33083 | Uint32 | 0.1 | VA | dynamic |
| grid_frequency | Solis Grid Frequency | 33094 | Uint16 | 0.01 | HZ | dynamic |

### Temperature and Status Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| temperature | Solis Temperature | 33093 | Int16 | 0.1 | C | dynamic |
| solis_status | Solis Status | 33095 | Uint16 | 1.0 | | dynamic |

### Fault Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| grid_fault_status_01 | Solis Grid Fault Status 01 (Bitmask) | 33116 | Uint16 | 1.0 | | dynamic |
| backup_load_fault_status_02 | Solis Backup Load Fault Status 02 (Bitmask) | 33117 | Uint16 | 1.0 | | dynamic |
| battery_fault_status_03 | Solis Battery Fault Status 03 (Bitmask) | 33118 | Uint16 | 1.0 | | dynamic |
| device_fault_status_04 | Solis Device Fault Status 04 (Bitmask) | 33119 | Uint16 | 1.0 | | dynamic |
| device_fault_status_05 | Solis Device Fault Status 05 (Bitmask) | 33120 | Uint16 | 1.0 | | dynamic |
| operating_status | Solis Operating Status (Bitmask) | 33121 | Uint16 | 1.0 | | dynamic |

### Battery Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| battery_voltage | Solis Battery Voltage | 33133 | Uint16 | 0.1 | V | dynamic |
| battery_current | Solis Battery Current | 33134 | Int16 | 0.1 | A | dynamic |
| battery_current_direction | Solis Battery Current Direction | 33135 | Uint16 | 1.0 | | dynamic |
| battery_soc | Solis Battery SOC | 33139 | Uint16 | 1.0 | % | dynamic |
| battery_soh | Solis Battery SOH | 33140 | Uint16 | 1.0 | % | dynamic |
| battery_voltage_bms | Solis Battery Voltage (BMS) | 33141 | Uint16 | 0.01 | V | dynamic |
| battery_fault_status_1_bms | Solis Battery Fault Status 1 (BMS) | 33145 | Uint16 | 1.0 | | dynamic |
| battery_fault_status_2_bms | Solis Battery Fault Status 2 (BMS) | 33146 | Uint16 | 1.0 | | dynamic |
| battery_power | Solis Battery Power | 33149 | Int32 | 1 | W | dynamic |

### Backup Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| backup_ac_voltage_phase_a | Solis Backup AC Voltage Phase A | 33137 | Uint16 | 0.1 | V | dynamic |
| backup_ac_current_phase_a | Solis Backup AC Current Phase A | 33138 | Uint16 | 0.1 | A | dynamic |
| household_load_power | Solis Household Load Power | 33147 | Uint16 | 1 | W | dynamic |
| backup_load_power | Solis Backup Load Power | 33148 | Uint16 | 1 | W | dynamic |
| ac_grid_port_power | Solis AC Grid Port Power | 33151 | Int32 | 0.1 | W | dynamic |

### Battery Energy Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| today_battery_charge_energy | Solis Today Battery Charge Energy | 33163 | Uint16 | 0.1 | kWh | dynamic |
| today_battery_discharge_energy | Solis Today Battery Discharge Energy | 33167 | Uint16 | 0.1 | kWh | dynamic |
| total_battery_discharge_energy | Solis Total Battery Discharge Energy | 33165 | Uint32 | 1 | kWh | dynamic |
| total_battery_charge_energy | Solis Total Battery Charge Energy | 33161 | Uint32 | 1 | kWh | dynamic |

### Grid Energy Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| today_energy_imported_from_grid | Solis Today Energy Imported From Grid | 33171 | Uint16 | 0.1 | kWh | dynamic |
| total_energy_imported_from_grid | Solis Total Energy Imported From Grid | 33169 | Uint32 | 0.1 | kWh | dynamic |
| total_energy_fed_into_grid | Solis Total Energy Fed Into Grid | 33173 | Uint32 | 0.1 | kWh | dynamic |
| today_energy_fed_into_grid | Solis Today Energy Fed Into Grid | 33175 | Uint16 | 0.1 | kWh | dynamic |
| today_energy_consumption | Solis Today Energy Consumption | 33179 | Uint16 | 0.1 | kWh | dynamic |
| total_energy_consumption | Solis Total Energy Consumption | 33177 | Uint32 | 1 | kWh | dynamic |

### Household Load Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| household_load_today_energy | Solis Household Load Today Energy | 33586 | Uint16 | 0.1 | kWh | dynamic |
| household_load_year_energy | Solis Household Load Year Energy | 33582 | Uint32 | 1 | kWh | dynamic |
| household_load_month_energy | Solis Household Load Month Energy | 33584 | Uint32 | 1 | kWh | dynamic |
| household_load_total_energy | Solis Household Load Total Energy | 33580 | Uint32 | 1 | kWh | dynamic |

### Backup Load Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| backup_load_today_energy | Solis Backup Load Today Energy | 33596 | Uint16 | 0.1 | kWh | dynamic |
| backup_load_total_energy | Solis Backup Load Total Energy | 33590 | Uint32 | 1 | kWh | dynamic |
| backup_load_year_energy | Solis Backup Load Year Energy | 33592 | Uint32 | 1 | kWh | dynamic |
| backup_load_month_energy | Solis Backup Load Month Energy | 33594 | Uint32 | 1 | kWh | dynamic |

### Meter Registers

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| meter_ac_voltage_a | Solis Meter AC Voltage A | 33251 | Uint16 | 0.1 | V | dynamic |
| meter_ac_current_a | Solis Meter AC Current A | 33252 | Uint16 | 0.1 | A | dynamic |
| meter_ac_voltage_b | Solis Meter AC Voltage B | 33253 | Uint16 | 0.1 | V | dynamic |
| meter_ac_current_b | Solis Meter AC Current B | 33254 | Uint16 | 0.1 | A | dynamic |
| meter_ac_voltage_c | Solis Meter AC Voltage C | 33255 | Uint16 | 0.1 | V | dynamic |
| meter_ac_current_c | Solis Meter AC Current C | 33256 | Uint16 | 0.1 | A | dynamic |
| meter_active_power_a | Solis Meter Active Power A | 33257 | Int32 | 0.1 | W | dynamic |
| meter_active_power_b | Solis Meter Active Power B | 33259 | Int32 | 0.1 | W | dynamic |
| meter_active_power_c | Solis Meter Active Power C | 33261 | Int32 | 0.1 | W | dynamic |
| meter_total_active_power | Solis Meter Total Active Power | 33263 | Int32 | 1.0 | W | dynamic |

### Computed Registers

**Note:** These registers are **computed/virtual** (Address = 0) and do not exist in the inverter's Modbus interface. They are calculated from other registers. Values may be inaccurate if source registers are incorrect or if the calculation logic has changed.

| Key | Name | Source | Scale | Unit | Stability |
|-----|------|--------|-------|------|-----------|
| today_grid_energy | Today Grid Energy (Net) | today_energy_fed_into_grid - today_energy_imported_from_grid | 1 | kWh | dynamic |
| total_grid_energy | Total Grid Energy (Net) | total_energy_fed_into_grid - total_energy_imported_from_grid | 1 | kWh | dynamic |
| energy_consumption_month_energy | Energy Consumption Month Energy (Computed) | Sum of today_energy_consumption daily values | 1 | kWh | dynamic |
| energy_fed_into_grid_month_energy | Energy Fed Into Grid Month Energy (Computed) | Sum of today_energy_fed_into_grid daily values | 1 | kWh | dynamic |
| energy_imported_from_grid_month_energy | Energy Imported From Grid Month Energy (Computed) | Sum of today_energy_imported_from_grid daily values | 1 | kWh | dynamic |
| battery_discharge_month_energy | Battery Discharge Month Energy (Computed) | Sum of today_battery_discharge_energy daily values | 1 | kWh | dynamic |
| battery_charge_month_energy | Battery Charge Month Energy (Computed) | Sum of today_battery_charge_energy daily values | 1 | kWh | dynamic |
| month_grid_energy | Month Grid Energy (Net, Computed) | energy_fed_into_grid_month_energy - energy_imported_from_grid_month_energy | 1 | kWh | dynamic |
| energy_consumption_year_energy | Energy Consumption Year Energy (Computed) | Sum of today_energy_consumption daily values | 1 | kWh | dynamic |
| energy_fed_into_grid_year_energy | Energy Fed Into Grid Year Energy (Computed) | Sum of today_energy_fed_into_grid daily values | 1 | kWh | dynamic |
| energy_imported_from_grid_year_energy | Energy Imported From Grid Year Energy (Computed) | Sum of today_energy_imported_from_grid daily values | 1 | kWh | dynamic |
| battery_discharge_year_energy | Battery Discharge Year Energy (Computed) | Sum of today_battery_discharge_energy daily values | 1 | kWh | dynamic |
| battery_charge_year_energy | Battery Charge Year Energy (Computed) | Sum of today_battery_charge_energy daily values | 1 | kWh | dynamic |
| year_grid_energy | Year Grid Energy (Net, Computed) | energy_fed_into_grid_year_energy - energy_imported_from_grid_year_energy | 1 | kWh | dynamic |

---

## Virtual/Computed Registers - Detailed Lifecycle

The Solis Monitor provides **virtual registers** that are computed from real Modbus registers. These derived metrics enable powerful energy analysis. This section explains exactly **when**, **where**, and **how** each type of computed register is calculated, stored, and retrieved.

### Register Categories

Virtual registers fall into **4 categories** based on their computation method and storage pattern:

| Category | Computation | Storage | Examples |
|----------|------------|---------|----------|
| **Real-time Net** | Simple subtraction in poller | Cache + total_values/daily_values tables | `total_grid_energy`, `today_grid_energy` |
| **Aggregated Monthly** | Sum of daily values | Cache + monthly_values table | `energy_consumption_month_energy`, `battery_discharge_month_energy` |
| **Aggregated Yearly** | Sum of daily values | Cache + yearly_values table | `energy_consumption_year_energy`, `battery_discharge_year_energy` |
| **Net Monthly/Yearly** | Subtraction of aggregated values | Cache + monthly_values/yearly_values tables | `month_grid_energy`, `year_grid_energy` |

---

### When Are Virtual Registers Computed?

#### 1. Poller Cycle (Every `poller.interval`, e.g., 30-60 seconds)

On **every poll cycle**, the poller:

```
Timer triggers (e.g., every 60 seconds)
  ↓
Read real registers from Modbus (4 contiguous blocks)
  ↓
Compute virtual registers:
  ├─ total_grid_energy = total_energy_fed_into_grid - total_energy_imported_from_grid
  ├─ today_grid_energy = today_energy_fed_into_grid - today_energy_imported_from_grid
  ├─ energy_consumption_month_energy = SUM(today_energy_consumption for current month)
  ├─ energy_fed_into_grid_month_energy = SUM(today_energy_fed_into_grid for current month)
  ├─ energy_imported_from_grid_month_energy = SUM(today_energy_imported_from_grid for current month)
  ├─ battery_discharge_month_energy = SUM(today_battery_discharge_energy for current month)
  ├─ battery_charge_month_energy = SUM(today_battery_charge_energy for current month)
  ├─ month_grid_energy = energy_fed_into_grid_month_energy - energy_imported_from_grid_month_energy
  ├─ energy_consumption_year_energy = SUM(today_energy_consumption for current year)
  ├─ energy_fed_into_grid_year_energy = SUM(today_energy_fed_into_grid for current year)
  ├─ energy_imported_from_grid_year_energy = SUM(today_energy_imported_from_grid for current year)
  ├─ battery_discharge_year_energy = SUM(today_battery_discharge_energy for current year)
  ├─ battery_charge_year_energy = SUM(today_battery_charge_energy for current year)
  └─ year_grid_energy = energy_fed_into_grid_year_energy - energy_imported_from_grid_year_energy
  ↓
Update cache with ALL values (real + virtual)
  ↓
Store in database:
  ├─ daily_values table ← today_* registers + today_grid_energy
  ├─ total_values table ← total_* registers + total_grid_energy
  ├─ monthly_values table ← month_* registers (computed)
  └─ yearly_values table ← year_* registers (computed)
  ↓
IF WebSocket clients connected:
  ↓
Broadcast cache update to all clients
```

**Key point:** All virtual registers are recomputed on every poll cycle, **regardless of WebSocket connections**. The poller's timer runs independently.

#### 2. On-Demand Historical Queries (When API is Called)

When you request historical data via the API (e.g., `GET /api/history/monthly/energy_consumption_month_energy?start=2024-01&end=2024-12`):

```
API Request received
  ↓
Check if key is a computed register
  ↓
Retrieve stored data from database (for past periods)
  ↓
For current period (current month/year):
  ↓
  Recalculate from source daily registers
  ↓
  Store computed values back to database (backfill)
  ↓
Return complete dataset
```

This ensures:
- **Past periods** use cached/stored values (fast response)
- **Current period** is always fresh (accurate)
- **Missing data** gets backfilled automatically

---

### Where Are Virtual Registers Stored?

| Register | In Memory (Cache) | Database Table | Persistence |
|----------|------------------|----------------|-------------|
| `today_grid_energy` | ✅ Yes | `daily_values` | ✅ Permanent |
| `total_grid_energy` | ✅ Yes | `total_values` | ✅ Permanent |
| `energy_consumption_month_energy` | ✅ Yes | `monthly_values` | ✅ Permanent |
| `energy_fed_into_grid_month_energy` | ✅ Yes | `monthly_values` | ✅ Permanent |
| `energy_imported_from_grid_month_energy` | ✅ Yes | `monthly_values` | ✅ Permanent |
| `battery_discharge_month_energy` | ✅ Yes | `monthly_values` | ✅ Permanent |
| `battery_charge_month_energy` | ✅ Yes | `monthly_values` | ✅ Permanent |
| `month_grid_energy` | ✅ Yes | `monthly_values` | ✅ Permanent |
| `energy_consumption_year_energy` | ✅ Yes | `yearly_values` | ✅ Permanent |
| `energy_fed_into_grid_year_energy` | ✅ Yes | `yearly_values` | ✅ Permanent |
| `energy_imported_from_grid_year_energy` | ✅ Yes | `yearly_values` | ✅ Permanent |
| `battery_discharge_year_energy` | ✅ Yes | `yearly_values` | ✅ Permanent |
| `battery_charge_year_energy` | ✅ Yes | `yearly_values` | ✅ Permanent |
| `year_grid_energy` | ✅ Yes | `yearly_values` | ✅ Permanent |

---

### How Are Virtual Registers Retrieved?

#### Method 1: HTTP API - Current Values

```bash
# Get all current register values (includes virtual registers)
GET /api/keys

# Get specific current value (real or virtual)
GET /api/data/total_grid_energy
GET /api/data/month_grid_energy
```

**Response:** Current value from cache (computed during last poll)

```json
{
  "key": "total_grid_energy",
  "name": "Total Grid Energy (Net)",
  "value": 15000.5,
  "unit": "kWh",
  "timestamp": "2024-01-15T10:30:00+01:00"
}
```

#### Method 2: HTTP API - Historical Values

```bash
# Get daily history (computed daily net grid energy)
GET /api/history/daily/today_grid_energy?start=2024-01-01&end=2024-01-31

# Get monthly history (computed monthly aggregations)
GET /api/history/monthly/energy_consumption_month_energy?start=2024-01&end=2024-12
GET /api/history/monthly/month_grid_energy?start=2024-01&end=2024-12

# Get yearly history
GET /api/history/yearly/year_grid_energy?start=2023&end=2024

# Get total (lifetime) value
GET /api/history/total/total_grid_energy
```


---

### Computation Logic Examples

#### Example 1: Net Grid Energy (Simple Subtraction)

```
# At time T (poll cycle):
total_energy_fed_into_grid    = 15,000 kWh  (from Modbus register 33173)
total_energy_imported_from_grid = 5,000 kWh  (from Modbus register 33169)

# Computation in poller:
total_grid_energy = 15,000 - 5,000 = 10,000 kWh

# Stored in:
- Cache: available immediately for API/WebSocket
- total_values table: permanent storage
```

#### Example 2: Monthly Energy Aggregation

```
# In database (daily_values table):
Date          | today_energy_consumption |
-------------|--------------------------|
2024-01-01   | 25.5 kWh                 |
2024-01-02   | 30.2 kWh                 |
2024-01-03   | 28.7 kWh                 |
...           | ...                      |
2024-01-31   | 22.1 kWh                 |

# At poll cycle on 2024-01-31:
# Computation in poller:
energy_consumption_month_energy = SUM(25.5 + 30.2 + 28.7 + ... + 22.1) = 850.0 kWh

# Stored in:
- Cache: available immediately
- monthly_values table: {month: "2024-01", value: 850.0}
```

#### Example 3: Net Monthly Grid Energy (Nested Computation)

```
# From monthly_values table (or computed on demand):
energy_fed_into_grid_month_energy      = 600 kWh  (Jan 2024)
energy_imported_from_grid_month_energy = 250 kWh  (Jan 2024)

# Computation in poller/service:
month_grid_energy = 600 - 250 = 350 kWh

# Stored in:
- Cache: available immediately
- monthly_values table: {month: "2024-01", value: 350.0}
```

---

### Timing Summary

| Scenario | When Computed | Where Stored | How Retrieved |
|----------|---------------|--------------|---------------|
| Current real-time value | Every poll cycle (30-60s) | Cache + DB | API `/api/data/{key}`, WebSocket |
| Historical daily value | On first request, then cached | daily_values | API `/api/history/daily/{key}` |
| Historical monthly value (past) | On first request, then cached | monthly_values | API `/api/history/monthly/{key}` |
| Historical monthly value (current) | Every poll cycle | Cache + monthly_values | API `/api/history/monthly/{key}` |
| Historical yearly value (past) | On first request, then cached | yearly_values | API `/api/history/yearly/{key}` |
| Historical yearly value (current) | Every poll cycle | Cache + yearly_values | API `/api/history/yearly/{key}` |
| Total/lifetime value | Every poll cycle | Cache + total_values | API `/api/history/total/{key}` |

---

### Configuration Impact

The `poller.interval` setting controls how often virtual registers are recomputed:

```yaml
poller:
  interval: 30s  # Virtual registers update every 30 seconds
```

**Note:** Virtual registers update at the same frequency as the poller interval. More frequent polling = more up-to-date virtual values, but higher Modbus load.

---

### Important Notes

1. **Virtual registers have Address=0** in the register definition, which identifies them as computed
2. **Scale=1** for all virtual registers (values are already in correct units)
3. **Stability=dynamic** for all virtual registers (they change over time)
4. **Database backfill**: When historical computed data is requested, it's calculated and stored for future efficiency
5. **Current period always fresh**: For monthly/yearly queries, the current period is always recalculated, never used from cache
6. **WebSocket independence**: Virtual registers are computed on every poll cycle regardless of whether WebSocket clients are connected

## Running

### Docker (recommended)

```bash
# Production
docker-compose up -d

# Development (builds from source)
docker-compose -f docker-compose.dev.yaml up --build
```

Both mount `./data` for persistence and `./config.yaml` for configuration.


```bash
---
services:
  app:
    image: ghcr.io/dombyte/solis:latest
    volumes:
      - ./data:/app/data
      - ./config.yaml:/app/config.yaml:ro
    ports:
      - "8080:8080"
    restart: unless-stopped
    environment:
      - TZ=Europe/Berlin
    healthcheck:
      test: ["CMD", "httpcheck", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3
```
### Local

```bash
go run ./cmd
# or build first
go build -o solis ./cmd && ./solis
```

## Development

```bash
# Build
go build -o solis ./cmd

# Run tests
go test ./...

# Format code
go fmt ./...

# Update dependencies
go mod tidy
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

## License 
This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
## Dependency Licenses

This project uses various open source dependencies. All license information is automatically tracked and maintained through GitHub's dependency graph.

https://github.com/dombyte/solis/network/dependencies
