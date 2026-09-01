# Solis Monitor

Monitoring solution for Solis inverters using Modbus TCP. Polls register data, stores it in SQLite, and exposes it via a web API and frontend dashboard.

## Desktop

<img width="2554" height="1299" alt="solis02" src="https://github.com/user-attachments/assets/df3be693-1289-4a87-8aa1-fad199e92220" />

<img width="2548" height="1299" alt="solis03" src="https://github.com/user-attachments/assets/de4beb8f-04eb-406b-ae7f-a67bacd320c1" />

## Mobile

<img width="397" height="873" alt="solis04" src="https://github.com/user-attachments/assets/cc011509-130c-4bb1-93ca-b9e0eee2162e" />

## API

After starting, the API Docs are available at `/docs`.

Health check endpoint is available at `/health`.

## Configuration

Copy `config.yaml` and adjust settings. All options can be overridden via environment variables using the `SOLIS_` prefix (e.g., `SOLIS_MODBUS_HOST=192.168.1.200`).

### Example Configuration

```yaml
app:
  debug: INFO
  port: 8080
  timeout: 30s
  serve_only: false

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
  daily_retention: 87600h
  monthly_retention: 87600h
  yearly_retention: 87600h
  error_retention: 720h
  wal_mode: true
  synchronous: NORMAL
  temp_store: MEMORY
  enable_migrations: true
  enable_backup: true
  max_backups: 3
  backup_interval: 24h

aggregator:
  interval: 60s
  backfill_current_year_monthly: false
```

### App Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `debug` | string | INFO | Log level: DEBUG, INFO, WARN, ERROR, FATAL |
| `port` | int | 8080 | HTTP server port |
| `timeout` | duration | 30s | Request timeout |
| `serve_only` | bool | false | Run in serve-only mode (disables Modbus polling, uses cached/stored data only) |

### Serve-Only Mode

Run without connecting to the inverter. Useful for read-only instances or when Modbus is unavailable.

HTTP API and WebSocket function normally. Historical data is served from SQLite, current values from cache. Modbus polling is disabled.

**Limitation:** Current register values are cache-only. Cache starts empty. To populate: run in normal mode first, then switch to serve-only.

Configuration:
```yaml
app:
  serve_only: true
```

### Poller Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `interval` | duration | 30s | Base interval between poll cycles |
| `block_attempts` | int | 3 | Retry attempts per block if read fails |
| `block_retry_delay` | duration | 1s | Delay between retry attempts |
| `block_interval` | duration | 0s | Delay between block reads |
| `poll_timeout` | duration | 30s | Max duration for full poll cycle |

### Modbus Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `type` | string | tcp | Connection type |
| `host` | string | 192.168.1.100 | Modbus server IP/hostname |
| `port` | int | 502 | Modbus server port |
| `timeout` | duration | 5s | Connection/read timeout |
| `unit_id` | byte | 1 | Modbus unit/slave ID (1-247) |

### Storage Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `path` | string | ./data/solis.db | Database file path |
| `daily_retention` | duration | 87600h | Retention for daily values |
| `monthly_retention` | duration | 87600h | Retention for monthly values |
| `yearly_retention` | duration | 87600h | Retention for yearly values |
| `error_retention` | duration | 720h | Retention for error data |
| `wal_mode` | bool | true | Enable Write-Ahead Logging |
| `synchronous` | string | NORMAL | Sync mode: OFF, NORMAL, FULL, EXTRA |
| `temp_store` | string | MEMORY | Temp storage: DEFAULT, FILE, MEMORY |
| `enable_migrations` | bool | true | Enable automatic schema migrations |
| `enable_backup` | bool | true | Enable database backup |
| `max_backups` | int | 3 | Maximum backup files to keep (0 = unlimited) |
| `backup_interval` | duration | 24h | Interval for periodic backups |

### Aggregator Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `interval` | duration | 60s | Interval for recomputing monthly/yearly/net values |
| `backfill_current_year_monthly` | boolean | false | **WARNING**: When enabled, recomputes and overwrites all monthly data for the current year from daily values on startup. Only enable temporarily if you need to rebuild monthly aggregates. |

## Available Registers

In v2, registers focus on energy values and status. Many voltage, current, power, temperature, and meter registers have been removed.

All energy and status registers are polled by default and cannot be disabled.

### Energy Registers

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

Virtual registers calculated from other registers by the aggregator.

| Key | Name | Source | Scale | Unit | Aggregation |
|-----|------|--------|-------|------|-------------|
| `grid_energy_total` | Grid Energy Total (Net) | grid_export_total - grid_import_total | 1 | kWh | total |
| `grid_energy_daily` | Grid Energy Daily (Net) | grid_export_daily - grid_import_daily | 1 | kWh | daily |
| `energy_consumption_monthly` | Energy Consumption Monthly | Sum of energy_consumption_daily values | 1 | kWh | monthly |
| `grid_export_monthly` | Grid Export Monthly | Sum of grid_export_daily values | 1 | kWh | monthly |
| `grid_import_monthly` | Grid Import Monthly | Sum of grid_import_daily values | 1 | kWh | monthly |
| `battery_discharge_monthly` | Battery Discharge Monthly | Sum of battery_discharge_daily values | 1 | kWh | monthly |
| `battery_charge_monthly` | Battery Charge Monthly | Sum of battery_charge_daily values | 1 | kWh | monthly |
| `grid_energy_monthly` | Grid Energy Monthly (Net) | grid_export_monthly - grid_import_monthly | 1 | kWh | monthly |
| `energy_consumption_yearly` | Energy Consumption Yearly | Sum of energy_consumption_daily values | 1 | kWh | yearly |
| `grid_export_yearly` | Grid Export Yearly | Sum of grid_export_daily values | 1 | kWh | yearly |
| `grid_import_yearly` | Grid Import Yearly | Sum of grid_import_daily values | 1 | kWh | yearly |
| `battery_discharge_yearly` | Battery Discharge Yearly | Sum of battery_discharge_daily values | 1 | kWh | yearly |
| `battery_charge_yearly` | Battery Charge Yearly | Sum of battery_charge_daily values | 1 | kWh | yearly |
| `grid_energy_yearly` | Grid Energy Yearly (Net) | grid_export_yearly - grid_import_yearly | 1 | kWh | yearly |

## Data Retrieval

### Current Values
```
GET /api/keys                           # List all register keys
GET /api/data/{key}                    # Current value
```

### Historical Values
```
GET /api/data/{daily_key}?start=2024-01-01&end=2024-01-31    # Daily history
GET /api/data/{monthly_key}?start=2024-01&end=2024-12     # Monthly history
GET /api/data/{yearly_key}?start=2023&end=2024           # Yearly history
GET /api/data/{total_key}                              # Lifetime value
```

Historical data is retrieved from the database. Computed monthly/yearly registers for the current period are updated by the aggregator on its interval.

## Aggregator

The aggregator computes derived values from daily data on a regular interval:

- Computed monthly registers: summed from daily values
- Computed yearly registers: summed from daily values  
- Net registers: calculated from component registers (e.g., grid_export - grid_import)

Current period computed values are updated every `aggregator.interval`. Past periods use stored values.

The `backfill_current_year_monthly` option recomputes all monthly data for the current year from daily values on startup. This overwrites directly-polled monthly values. Only enable when needed to rebuild aggregates.

## Running

### Docker

```bash
docker-compose up -d

# Development
docker-compose -f docker-compose.dev.yaml up --build
```

```yaml
services:
  app:
    image: ghcr.io/dombyte/solis:latest
    volumes:
      - ./data:/app/data
      - ./config.yaml:/app/config.yaml:ro
    ports:
      - "8080:8080"
    restart: unless-stopped
```

### Local

```bash
go run ./cmd
# or
go build -o solis ./cmd && ./solis
```

## Development

```bash
go build -o solis ./cmd
go test ./...
go fmt ./...
go mod tidy
```

### Frontend

```bash
cd frontend
npm install
npm run dev
```

## Migration from v1

### Removed Features
- Register filtering (`registers.disabled_keys`)
- `/api/registers` endpoint

### Removed Registers
- Information registers (model, serial, versions)
- PV voltage and current registers
- Grid voltage, current, power, frequency registers
- Temperature registers
- Battery voltage, current, SOC, SOH registers
- Power registers
- Backup AC registers
- Meter registers

See `removed_registers_v2.md` for complete list.

### New Features
- Aggregator service for background computation
- Simplified API at `/api/keys` and `/api/data/{key}`
- Focus on energy-only registers for improved performance
- Direct daily, monthly, yearly historical queries

## License

MIT License. See [LICENSE](LICENSE) for details.
