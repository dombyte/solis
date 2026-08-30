# Solis Monitor

Monitoring solution for Solis inverters using Modbus TCP. Polls register data, stores it in SQLite, and exposes it via a web API and frontend dashboard.

## Desktop
<img width="2554" height="1299" alt="solis02" src="https://github.com/user-attachments/assets/df3be693-1289-4a87-8aa1-fad199e92220" />

<img width="2548" height="1299" alt="solis03" src="https://github.com/user-attachments/assets/de4beb8f-04eb-406b-ae7f-a67bacd320c1" />

## Mobile

<img width="397" height="873" alt="solis04" src="https://github.com/user-attachments/assets/cc011509-130c-4bb1-93ca-b9e0eee2162e" />


## API

After starting, the API Docs are available at `/docs`.

## Health Check
A health check endpoint is available at `/health`.

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

storage:
  path: ./data/solis.db
  daily_retention: 87600h  # 10 years
  monthly_retention: 87600h
  yearly_retention: 87600h
  error_retention: 720h   # 30 days
  wal_mode: true
  synchronous: NORMAL
  temp_store: MEMORY

# Aggregator settings for background computation of derived values
aggregator:
  interval: 60s

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
- HTTP API and WebSocket all function normally
- Historical data (daily, monthly, yearly, total energy) is served from SQLite
- Current register values (energy values, status, etc.) are served from cache
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
| `type` | string | tcp | Connection type: tcp |
| `host` | string | 192.168.1.100 | Modbus server IP/hostname |
| `port` | int | 502 | Modbus server port (1-65535) |
| `timeout` | duration | 5s | Connection/read timeout |
| `unit_id` | byte | 1 | Modbus unit/slave ID (1-247) |

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

### Aggregator Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `interval` | duration | 60s | Interval for recomputing monthly/yearly/net values |

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

## Available Registers

In v2, the register set has been streamlined to focus on **energy values and status registers**. Many voltage, current, power, temperature, and meter registers have been removed to reduce Modbus polling load and improve performance.

All energy and status registers are polled by default. The register set is now fixed and cannot be disabled via configuration (the `registers.disabled_keys` setting has been removed).

### Energy Registers

These registers track energy production, consumption, and flow in various time periods.

| Key | Name | Address | Type | Scale | Unit | Aggregation |
|-----|------|---------|------|-------|------|-------------|
| `pv_energy_daily` | PV Energy Daily | 33035 | Uint16 | 0.1 | kWh | daily |
| `pv_energy_monthly` | PV Energy Monthly | 33031 | Uint32 | 1 | kWh | monthly |
| `pv_energy_yearly` | PV Energy Yearly | 33037 | Uint32 | 1 | kWh | yearly |
| `pv_energy_total` | PV Energy Total | 33029 | Uint32 | 1 | kWh | total |

### Battery Energy Registers

| Key | Name | Address | Type | Scale | Unit | Aggregation |
|-----|------|---------|------|-------|------|-------------|
| `battery_charge_daily` | Battery Charge Daily | 33163 | Uint16 | 0.1 | kWh | daily |
| `battery_discharge_daily` | Battery Discharge Daily | 33167 | Uint16 | 0.1 | kWh | daily |
| `battery_discharge_total` | Battery Discharge Total | 33165 | Uint32 | 1 | kWh | total |
| `battery_charge_total` | Battery Charge Total | 33161 | Uint32 | 1 | kWh | total |

### Grid Energy Registers

| Key | Name | Address | Type | Scale | Unit | Aggregation |
|-----|------|---------|------|-------|------|-------------|
| `grid_import_daily` | Grid Import Daily | 33171 | Uint16 | 0.1 | kWh | daily |
| `grid_import_total` | Grid Import Total | 33169 | Uint32 | 1 | kWh | total |
| `grid_export_daily` | Grid Export Daily | 33175 | Uint16 | 0.1 | kWh | daily |
| `grid_export_total` | Grid Export Total | 33173 | Uint32 | 1 | kWh | total |

### Consumption Registers

| Key | Name | Address | Type | Scale | Unit | Aggregation |
|-----|------|---------|------|-------|------|-------------|
| `energy_consumption_daily` | Energy Consumption Daily | 33179 | Uint16 | 0.1 | kWh | daily |
| `energy_consumption_total` | Energy Consumption Total | 33177 | Uint32 | 1 | kWh | total |

### Household Energy Registers

| Key | Name | Address | Type | Scale | Unit | Aggregation |
|-----|------|---------|------|-------|------|-------------|
| `household_energy_daily` | Household Energy Daily | 33586 | Uint16 | 0.1 | kWh | daily |
| `household_energy_monthly` | Household Energy Monthly | 33584 | Uint32 | 1 | kWh | monthly |
| `household_energy_yearly` | Household Energy Yearly | 33582 | Uint32 | 1 | kWh | yearly |
| `household_energy_total` | Household Energy Total | 33580 | Uint32 | 1 | kWh | total |

### Backup Energy Registers

| Key | Name | Address | Type | Scale | Unit | Aggregation |
|-----|------|---------|------|-------|------|-------------|
| `backup_energy_daily` | Backup Energy Daily | 33596 | Uint16 | 0.1 | kWh | daily |
| `backup_energy_total` | Backup Energy Total | 33590 | Uint32 | 1 | kWh | total |
| `backup_energy_yearly` | Backup Energy Yearly | 33592 | Uint32 | 1 | kWh | yearly |
| `backup_energy_monthly` | Backup Energy Monthly | 33594 | Uint32 | 1 | kWh | monthly |

### Status Registers

Status and fault registers provide operational state and error information.

| Key | Name | Address | Type | Scale | Unit | Stability |
|-----|------|---------|------|-------|------|-----------|
| `solis_status` | Solis Status | 33095 | Uint16 | 1.0 | | dynamic |
| `grid_fault_1` | Grid Fault 1 (Bitmask) | 33116 | Uint16 | 1.0 | | dynamic |
| `backup_fault_2` | Backup Fault 2 (Bitmask) | 33117 | Uint16 | 1.0 | | dynamic |
| `battery_fault_3` | Battery Fault 3 (Bitmask) | 33118 | Uint16 | 1.0 | | dynamic |
| `device_fault_4` | Device Fault 4 (Bitmask) | 33119 | Uint16 | 1.0 | | dynamic |
| `device_fault_5` | Device Fault 5 (Bitmask) | 33120 | Uint16 | 1.0 | | dynamic |
| `operating_status` | Solis Operating Status (Bitmask) | 33121 | Uint16 | 1.0 | | dynamic |
| `battery_fault_1_bms` | Battery Fault 1 (BMS) | 33145 | Uint16 | 1.0 | | dynamic |
| `battery_fault_2_bms` | Battery Fault 2 (BMS) | 33146 | Uint16 | 1.0 | | dynamic |

### Computed Registers

These are **virtual/computed** registers (Address = 0) that are calculated from other registers. They are computed by the aggregator on a regular interval.

| Key | Name | Source | Scale | Unit | Aggregation |
|-----|------|--------|-------|------|-------------|
| `grid_energy_total` | Grid Energy Total (Net) | grid_export_total - grid_import_total | 1 | kWh | total |
| `grid_energy_daily` | Grid Energy Daily (Net) | grid_export_daily - grid_import_daily | 1 | kWh | daily |
| `energy_consumption_monthly` | Energy Consumption Monthly (Computed) | Sum of energy_consumption_daily daily values | 1 | kWh | monthly |
| `grid_export_monthly` | Grid Export Monthly (Computed) | Sum of grid_export_daily daily values | 1 | kWh | monthly |
| `grid_import_monthly` | Grid Import Monthly (Computed) | Sum of grid_import_daily daily values | 1 | kWh | monthly |
| `battery_discharge_monthly` | Battery Discharge Monthly (Computed) | Sum of battery_discharge_daily daily values | 1 | kWh | monthly |
| `battery_charge_monthly` | Battery Charge Monthly (Computed) | Sum of battery_charge_daily daily values | 1 | kWh | monthly |
| `grid_energy_monthly` | Grid Energy Monthly (Net, Computed) | grid_export_monthly - grid_import_monthly | 1 | kWh | monthly |
| `energy_consumption_yearly` | Energy Consumption Yearly (Computed) | Sum of energy_consumption_daily daily values | 1 | kWh | yearly |
| `grid_export_yearly` | Grid Export Yearly (Computed) | Sum of grid_export_daily daily values | 1 | kWh | yearly |
| `grid_import_yearly` | Grid Import Yearly (Computed) | Sum of grid_import_daily daily values | 1 | kWh | yearly |
| `battery_discharge_yearly` | Battery Discharge Yearly (Computed) | Sum of battery_discharge_daily daily values | 1 | kWh | yearly |
| `battery_charge_yearly` | Battery Charge Yearly (Computed) | Sum of battery_charge_daily daily values | 1 | kWh | yearly |
| `grid_energy_yearly` | Grid Energy Yearly (Net, Computed) | grid_export_yearly - grid_import_yearly | 1 | kWh | yearly |

### Register Aggregation Types

| Type | Description | Query Support |
|------|-------------|---------------|
| **daily** | Resets at midnight, stores per-day values | Supports `?start=` and `?end=` for date range queries |
| **monthly** | Aggregated from daily values at month end | Supports `?start=` and `?end=` for month range queries (YYYY-MM format) |
| **yearly** | Aggregated from daily values at year end | Supports `?start=` and `?end=` for year range queries (YYYY format) |
| **total** | Lifetime accumulation, never resets | Returns single lifetime value |

---

## Virtual/Computed Registers - Detailed Lifecycle

The Solis Monitor provides **virtual registers** that are computed from real Modbus registers. These derived metrics enable powerful energy analysis. This section explains exactly **when**, **where**, and **how** each type of computed register is calculated, stored, and retrieved.

### When Are Virtual Registers Computed?

#### 1. Aggregator Cycle (Every `aggregator.interval`, default: 60 seconds)

On **every aggregator cycle**, the system:
- Computes daily sums from storage for monthly registers
- Computes daily sums from storage for yearly registers  
- Computes net values (grid_energy_*) from component values
- Updates cache with all computed values
- Stores computed values in database tables (monthly_values, yearly_values, total_values)

**Key point:** All virtual registers are recomputed on every aggregator cycle, **regardless of API requests**. The aggregator runs independently.

#### 2. On-Demand Historical Queries (When API is Called)

When you request historical data via the API:
- **Past periods** use cached/stored values (fast response)
- **Current period** is always fresh (accurate) - recalculated from source daily registers
- **Missing data** gets backfilled automatically

---

### Where Are Virtual Registers Stored?

| Register | In Memory (Cache) | Database Table | Persistence |
|----------|------------------|----------------|-------------|
| `grid_energy_total` | Yes | `total_values` | Permanent |
| `grid_energy_daily` | Yes | `daily_values` | Permanent |
| `energy_consumption_monthly` | Yes | `monthly_values` | Permanent |
| `grid_export_monthly` | Yes | `monthly_values` | Permanent |
| `grid_import_monthly` | Yes | `monthly_values` | Permanent |
| `battery_discharge_monthly` | Yes | `monthly_values` | Permanent |
| `battery_charge_monthly` | Yes | `monthly_values` | Permanent |
| `grid_energy_monthly` | Yes | `monthly_values` | Permanent |
| `energy_consumption_yearly` | Yes | `yearly_values` | Permanent |
| `grid_export_yearly` | Yes | `yearly_values` | Permanent |
| `grid_import_yearly` | Yes | `yearly_values` | Permanent |
| `battery_discharge_yearly` | Yes | `yearly_values` | Permanent |
| `battery_charge_yearly` | Yes | `yearly_values` | Permanent |
| `grid_energy_yearly` | Yes | `yearly_values` | Permanent |

---

### How Are Virtual Registers Retrieved?

#### Method 1: HTTP API - Current Values

```bash
# Get all current register keys with metadata
GET /api/keys

# Get specific current value (real or virtual)
GET /api/data/grid_energy_total
GET /api/data/grid_energy_monthly
```

**Response:** Current value from cache (computed during last aggregator cycle)

```json
{
  "key": "grid_energy_total",
  "name": "Grid Energy Total (Net)",
  "value": 15000.5,
  "unit": "kWh",
  "timestamp": "2024-01-15T10:30:00+01:00"
}
```

#### Method 2: HTTP API - Historical Values

```bash
# Get daily history
GET /api/data/energy_consumption_daily?start=2024-01-01&end=2024-01-31

# Get monthly history (computed monthly aggregations)
GET /api/data/energy_consumption_monthly?start=2024-01&end=2024-12
GET /api/data/grid_energy_monthly?start=2024-01&end=2024-12

# Get yearly history
GET /api/data/grid_energy_yearly?start=2023&end=2024

# Get total (lifetime) value
GET /api/data/grid_energy_total
```


---

### Computation Logic Examples

#### Example 1: Net Grid Energy (Simple Subtraction)

```
# At time T (aggregator cycle):
grid_export_total    = 15,000 kWh  (from Modbus register 33173)
grid_import_total = 5,000 kWh  (from Modbus register 33169)

# Computation in aggregator:
grid_energy_total = 15,000 - 5,000 = 10,000 kWh

# Stored in:
- Cache: available immediately for API/WebSocket
- total_values table: permanent storage
```

#### Example 2: Monthly Energy Aggregation

```
# In database (daily_values table):
Date          | energy_consumption_daily |
-------------|--------------------------|
2024-01-01   | 25.5 kWh                 |
2024-01-02   | 30.2 kWh                 |
2024-01-03   | 28.7 kWh                 |
...           | ...                      |
2024-01-31   | 22.1 kWh                 |

# At aggregator cycle on 2024-01-31:
# Computation in aggregator:
energy_consumption_monthly = SUM(25.5 + 30.2 + 28.7 + ... + 22.1) = 850.0 kWh

# Stored in:
- Cache: available immediately
- monthly_values table: {month: "2024-01", value: 850.0}
```

#### Example 3: Net Monthly Grid Energy (Nested Computation)

```
# From monthly_values table (or computed on demand):
grid_export_monthly      = 600 kWh  (Jan 2024)
grid_import_monthly = 250 kWh  (Jan 2024)

# Computation in aggregator:
grid_energy_monthly = 600 - 250 = 350 kWh

# Stored in:
- Cache: available immediately
- monthly_values table: {month: "2024-01", value: 350.0}
```

---

### Timing Summary

| Scenario | When Computed | Where Stored | How Retrieved |
|----------|---------------|--------------|---------------|
| Current real-time value | Every poll cycle (30-60s) | Cache + DB | API `/api/data/{key}`, WebSocket |
| Historical daily value | On first request, then cached | daily_values | API `/api/data/{key}` with `?start=` and `?end=` |
| Historical monthly value (past) | On first request, then cached | monthly_values | API `/api/data/{key}` with `?start=` and `?end=` |
| Historical monthly value (current) | Every aggregator cycle | Cache + monthly_values | API `/api/data/{key}` with `?start=` and `?end=` |
| Historical yearly value (past) | On first request, then cached | yearly_values | API `/api/data/{key}` with `?start=` and `?end=` |
| Historical yearly value (current) | Every aggregator cycle | Cache + yearly_values | API `/api/data/{key}` with `?start=` and `?end=` |
| Total/lifetime value | Every aggregator cycle | Cache + total_values | API `/api/data/{key}` |

---

### Configuration Impact

The `aggregator.interval` setting controls how often virtual registers are recomputed:

```yaml
aggregator:
  interval: 60s  # Virtual registers update every 60 seconds
```

**Note:** Virtual registers update at the same frequency as the aggregator interval. More frequent computation = more up-to-date virtual values, but higher computation load.

---

### Important Notes

1. **Virtual registers have Address=0** in the register definition, which identifies them as computed
2. **Scale=1** for all virtual registers (values are already in correct units)
3. **Aggregation types** (daily, monthly, yearly, total) determine query support
4. **Database backfill**: When historical computed data is requested, it's calculated and stored for future efficiency
5. **Current period always fresh**: For monthly/yearly queries, the current period is always recalculated, never used from cache

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
## Migration from v1

Version 2 introduced significant changes to the register set and architecture:

### Removed Features
- **Register filtering**: The `registers.disabled_keys` configuration has been removed. All energy and status registers are always enabled.
- **Old API endpoints**: The `/api/registers` endpoint has been removed.

### Removed Registers
The following categories of registers have been removed to reduce Modbus load:
- Information registers (model, serial number, versions)
- PV voltage and current registers (pv_voltage_1-4, pv_current_1-4)
- Grid voltage, current, power, and frequency registers
- Temperature registers
- Battery voltage, current, SOC, SOH registers
- Power registers (household_power, backup_power, battery_power, ac_grid_power)
- Backup AC registers
- Meter registers

See `removed_registers_v2.md` for a complete list of removed registers.

### New Features
- **Aggregator service**: Background computation of derived values (monthly, yearly, net) on a configurable interval
- **Simplified API**: Cleaner endpoints at `/api/keys` and `/api/data/{key}`
- **Improved performance**: Reduced Modbus polling load by focusing on energy-only registers
- **Better historical queries**: Support for daily, monthly, and yearly historical data directly from the API


## License 
This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
## Dependency Licenses

This project uses various open source dependencies. All license information is automatically tracked and maintained through GitHub's dependency graph.

https://github.com/dombyte/solis/network/dependencies
