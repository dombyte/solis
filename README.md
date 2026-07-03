# Solis Monitor

Monitoring solution for Solis inverters using Modbus TCP/RTU. Polls register data, stores it in SQLite, and exposes it via a web API, frontend dashboard and Prometheus metrics.



<img width="1262" height="1054" alt="solism3" src="https://github.com/user-attachments/assets/a74a16b3-c779-455b-b743-2af0bc05bb2a" />
<img width="1270" height="1125" alt="solism1" src="https://github.com/user-attachments/assets/185dc03d-5e54-4578-a575-4dc484b25429" />
<img width="1259" height="1065" alt="solism2" src="https://github.com/user-attachments/assets/e9b195fd-97c1-4443-9fc1-1d6cb3ea2331" />


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

**Note:** These registers are **computed/virtual** (Address = 0) and do not exist in the inverter's Modbus interface. They are calculated from other registers and stored in the database. Values may be inaccurate if source registers are incorrect or if the calculation logic has changed.

| Key | Name | Source | Scale | Unit | Stability |
|-----|------|--------|-------|------|-----------|
| today_grid_energy | Today Grid Energy (Net) | total_energy_fed_into_grid - total_energy_imported_from_grid | 1 | kWh | dynamic |
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
