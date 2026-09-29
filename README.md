# Solis Monitor

Monitoring solution for Solis hybrid inverters over Modbus (TCP or RTU). Polls register data, stores
daily energy counters and status/fault changes in SQLite, computes monthly/yearly/total values
itself, and serves a React dashboard (REST + WebSocket) from the same Go binary.

## Desktop

<img width="2554" height="1299" alt="solis02" src="https://github.com/user-attachments/assets/df3be693-1289-4a87-8aa1-fad199e92220" />

<img width="2548" height="1299" alt="solis03" src="https://github.com/user-attachments/assets/de4beb8f-04eb-406b-ae7f-a67bacd320c1" />

## Mobile

<img width="397" height="873" alt="solis04" src="https://github.com/user-attachments/assets/cc011509-130c-4bb1-93ca-b9e0eee2162e" />

## API

After starting, the API docs (Swagger UI) are available at `/docs`.

The health check endpoint is `/health`: `200` with `{"status": "ok"|"degraded", ...}`, or
**503** with `{"status": "failed", "component": "...", "reason": "..."}`. A component's first
failure only degrades the status while the supervisor restarts it; `/health` turns 503 once a
restart has failed, when a non-restartable part (storage, event bus, HTTP server) failed, or
when the supervisor itself stalled. It never blocks on a component.

**A restart policy is required.** The app restarts failed components itself (up to 3 times in
a row); when that does not help it shuts down and **exits with code 1** instead of restarting
in-process, so Docker/Podman (`restart: unless-stopped`) or systemd (`Restart=on-failure`)
starts a fresh process. A clean `SIGTERM`/`SIGINT` shutdown exits 0 and is bounded to 30 s.

## Configuration

Copy [`example/config.yaml`](example/config.yaml) to `config.yaml` (next to the binary or
`docker-compose.yaml`) and adjust it. All options can be overridden via environment variables
using the `SOLIS_` prefix (e.g. `SOLIS_MODBUS_ADDRESS=tcp://192.168.1.200:502`). Invalid values
fail startup with a clear message instead of being silently misread.

The inverter's local timezone comes from the `TZ` environment variable (`time.Local`), never
from the config file.

### Example Configuration

```yaml
app:
  debug: INFO
  port: 8080
  timeout: 30s

poller:
  interval: 30s
  block_attempts: 3
  block_retry_delay: 1s
  block_interval: 0s
  poll_timeout: 30s

modbus:
  # tcp://host:port or rtu://<device path> (e.g. rtu:///dev/ttyUSB0)
  address: "tcp://192.168.2.151:502"
  timeout: 5s
  slave_id: 1
  # speed/data_bits/parity/stop_bits apply to rtu only (defaults: 19200 8N2)

rollover:
  time: "23:59"

storage:
  path: ./data/solis.db
  daily_retention: 10y
  error_retention: 10y
  wal_mode: true
  synchronous: NORMAL
  temp_store: MEMORY
  enable_backup: true
  max_backups: 3
  backup_interval: 24h
  cleanup_interval: 24h
```

### App Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `debug` | string | INFO | Log level: DEBUG, INFO, WARN, ERROR, FATAL (anything else fails startup) |
| `port` | int | 8080 | HTTP server port |
| `timeout` | duration | 30s | Request timeout and deadline of every storage call (at least 1s) |

Durations accept Go units (`s`, `m`, `h`) plus `d` (24h), `w` (7d) and `y` (365d), also
combined (`1y6w`, `1d12h`). A unit is required: a bare number such as `timeout: 30` is
rejected (it would otherwise mean 30 nanoseconds).

### Poller Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `interval` | duration | 30s | Base interval between poll cycles (at least 1s); also drives the aggregator's debounce (4×) and heartbeat (5×) |
| `block_attempts` | int | 3 | Retries per block after a failed read (3 = up to 4 reads) |
| `block_retry_delay` | duration | 1s | Delay between retry attempts for the same block (≥ 0) |
| `block_interval` | duration | 0s | Delay between successive block reads (≥ 0) |
| `poll_timeout` | duration | 30s | Max duration for a full poll cycle before aborting; `poll_timeout + modbus.timeout` must be below 3× `interval` (the health supervisor's grace — a read in flight can overrun `poll_timeout` by up to `modbus.timeout`) |

Transport failures drop the Modbus connection and reconnect. Exception replies (e.g. illegal
address, device busy) keep it open, so the block is retried up to `block_attempts` times. On
RTU, CRC errors, short frames and timeouts also keep it open, but 5 in a row (a pulled cable,
a hung comms board) reopen the serial port, and the Modbus component shows `recovering`.

### Modbus Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `address` | string | tcp://192.168.1.100:502 | Connection URL: `tcp://host:port` or `rtu://<device path>` (e.g. `rtu:///dev/ttyUSB0`) |
| `timeout` | duration | 5s | Connection/read timeout |
| `slave_id` | int | 1 | Modbus unit/slave ID (1-247; anything else fails startup) |
| `speed` | uint | 19200 (rtu default) | Serial link speed in bps (rtu only) |
| `data_bits` | uint | 8 (rtu default) | Bits per serial character (rtu only) |
| `parity` | string | N | Serial parity: `N`, `E`, or `O` (rtu only) |
| `stop_bits` | uint | 2, or 1 with parity (rtu default) | Serial stop bits (rtu only) |

The Modbus client never fails construction on an unreachable device: it starts in a
`recovering` state and reconnects with exponential backoff, reflected in `/health`.

### Rollover Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `time` | string | 23:59 | Local time of day the inverter's daily counters reset. Strict `HH:MM` (24h); any other format fails startup. |

### Storage Settings

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `path` | string | ./data/solis.db | Database file path |
| `daily_retention` | duration | 10y | Retention for daily values; monthly and yearly values follow it |
| `error_retention` | duration | 10y | Retention for error/fault data |
| `wal_mode` | bool | true | Enable Write-Ahead Logging |
| `synchronous` | string | NORMAL | Sync mode: OFF, NORMAL, FULL, EXTRA |
| `temp_store` | string | MEMORY | Temp storage: DEFAULT, FILE, MEMORY |
| `enable_backup` | bool | true | Enable periodic online backups (a backup before schema migrations is always taken) |
| `max_backups` | int | 3 | Maximum backup files to keep (0 = unlimited) |
| `backup_interval` | duration | 24h | Interval for periodic online backups |
| `cleanup_interval` | duration | 24h | Interval for retention cleanup |

Backups are written to `backups/` next to the database (e.g. `data/backups/solis.db.20260928_120000_000Z.backup`),
owner-only (`0600`), verified with `PRAGMA integrity_check`, and rotated per database. To
restore one, stop the app, copy it over `solis.db` and delete `solis.db-wal` and `solis.db-shm`.

Retention never corrupts computed values: cleanup deletes daily, monthly and yearly rows
older than `daily_retention` only up to the start of the first period that can still be
recomputed (the year after the last frozen year and the total baseline). The current year is
therefore always kept, whatever the setting. Schema migrations always run on startup;
`monthly_retention`, `yearly_retention` and `enable_migrations` were removed and are ignored
with a warning.

There is no `aggregator` section: the aggregator is event-driven off the poller and has no
config knobs of its own (debounce = 4× `poller.interval`, heartbeat = 5×). To recompute
monthly/yearly values from stored daily data, use the `backfill` CLI job (below) instead of a
config toggle.

## Available Registers

Registers focus on energy values, status/faults, and the live power values needed for the
dashboard's power-flow diagram. `GET /api/keys` lists every register with its `store`
(`none|daily|monthly|yearly|total|status`), address (only present when the register is polled),
data type and unit.

### Daily energy registers (polled)

| Key | Name | Address | Type | Scale | Unit |
|-----|------|---------|------|-------|------|
| `pv_energy_daily` | PV Energy Daily | 33035 | Uint16 | 0.1 | kWh |
| `battery_charge_daily` | Battery Charge Daily | 33163 | Uint16 | 0.1 | kWh |
| `battery_discharge_daily` | Battery Discharge Daily | 33167 | Uint16 | 0.1 | kWh |
| `grid_import_daily` | Grid Import Daily | 33171 | Uint16 | 0.1 | kWh |
| `grid_export_daily` | Grid Export Daily | 33175 | Uint16 | 0.1 | kWh |
| `energy_consumption_daily` | Energy Consumption Daily | 33179 | Uint16 | 0.1 | kWh |
| `household_energy_daily` | Household Energy Daily | 33586 | Uint16 | 0.1 | kWh |
| `backup_energy_daily` | Backup Energy Daily | 33596 | Uint16 | 0.1 | kWh |

Each of the eight series above also has computed `_monthly`, `_yearly` and `_total` keys (e.g.
`pv_energy_monthly`, `pv_energy_yearly`, `pv_energy_total`) — summed from the daily rows by the
aggregator, with **no Modbus address**.

### Net grid energy (computed, export − import)

| Key | Store | Source |
|-----|-------|--------|
| `grid_energy_daily` | daily | `grid_export_daily − grid_import_daily` |
| `grid_energy_monthly` | monthly | `grid_export_monthly − grid_import_monthly` |
| `grid_energy_yearly` | yearly | `grid_export_yearly − grid_import_yearly` |
| `grid_energy_total` | total | `grid_export_total − grid_import_total` |

### Status/fault registers (written to history on change)

| Key | Name | Address |
|-----|------|---------|
| `solis_status` | Solis Status | 33095 |
| `grid_fault_1` | Grid Fault 1 (Bitmask) | 33116 |
| `backup_fault_2` | Backup Fault 2 (Bitmask) | 33117 |
| `battery_fault_3` | Battery Fault 3 (Bitmask) | 33118 |
| `device_fault_4` | Device Fault 4 (Bitmask) | 33119 |
| `device_fault_5` | Device Fault 5 (Bitmask) | 33120 |
| `operating_status` | Solis Operating Status (Bitmask) | 33121 |
| `battery_fault_1_bms` | Battery Fault 1 (BMS) | 33145 |
| `battery_fault_2_bms` | Battery Fault 2 (BMS) | 33146 |

### Live power registers (cache-only, for the dashboard)

| Key | Name | Address | Unit |
|-----|------|---------|------|
| `pv_total_power` | PV Total Power | 33057 | W |
| `grid_power` | Grid Power | 33130 | W |
| `battery_current_direction` | Battery Current Direction | 33135 | |
| `battery_soc` | Battery SOC | 33139 | % |
| `household_load_power` | Household Load Power | 33147 | W |
| `backup_load_power` | Backup Load Power | 33148 | W |
| `battery_power` | Battery Power | 33149 | W |
| `battery_power_signed` | Battery Power (Signed) | — (derived) | W |

`battery_power_signed` is derived after each poll from `battery_power` +
`battery_current_direction` (positive = charging).

## Data Retrieval (REST)

```
GET /api/keys                                                # list all register keys + metadata
GET /api/data/{key}                                           # current value (from cache)
GET /api/data/{daily_key}?start=2024-01-01&end=2024-01-31      # daily history
GET /api/data/{monthly_key}?start=2024-01&end=2024-12          # monthly history
GET /api/data/{yearly_key}?start=2023&end=2024                 # yearly history
GET /api/data/{total_key}                                      # lifetime value (no start/end)
GET /api/data/{status_key}                                     # decoded change history
```

`start`/`end` accept `YYYY-MM-DD`, `YYYY-MM` or `YYYY`; a month or year end covers the whole
period, `start` defaults to 30 days ago and `end` to today. A malformed range, `start` after `end`,
or a range on a key without history (live, total and status keys) is answered with `400`; an
unknown key, or a key without a value yet, with `404`. Errors use one JSON shape:
`{"error": "Bad Request", "message": "…", "code": 400}`.

Values are stored at full precision; rounding to 2 decimals happens only in the JSON response.
There is no `?direct=true` mode — only the poller talks to the inverter, so all reads come from
the cache or SQLite.

## Live updates (WebSocket)

`GET /ws` upgrades same-origin connections only. Clients subscribe to the keys they need; only
changed, subscribed values are pushed (diffed per client), coalesced into one frame every
~75 ms:

```json
→ { "type": "subscribe",   "keys": ["pv_total_power", "solis_status"] }
← { "type": "snapshot",    "values": { "pv_total_power": { "value": 5230, "timestamp": "…", "unit": "W" } } }
← { "type": "update",      "ts": "…", "values": { "pv_total_power": { "value": 5102.5 } } }
← { "type": "update",      "ts": "…", "values": {}, "removed": ["battery_power_signed"] }
→ { "type": "unsubscribe", "keys": ["solis_status"] }
← { "type": "error",       "code": "unknown_keys", "keys": ["foo"] }
```

`removed` (optional) lists subscribed keys that no longer have a current value; clients drop
them instead of showing a stale value.

`ping` from the client is accepted and ignored. Unknown keys never drop the connection. History
stays on REST — there is no history over WebSocket. At most 64 clients are accepted (the next
one is closed with code 1013, "try again later") and at most 256 keys per message.

## Security

Solis Monitor is meant for a trusted LAN: there is no authentication. The REST API sends no
CORS headers (other websites cannot read it through a visitor's browser), the WebSocket only
accepts same-origin connections, and responses carry `X-Content-Type-Options: nosniff` and
`Referrer-Policy: no-referrer`. Framing is allowed so the dashboard can be embedded (e.g. a
Home Assistant iframe). Do not expose it to the internet without an authenticating reverse
proxy.

## Aggregator

The aggregator is event-driven, not timer-based: it wakes on cache changes from the poller
(debounced 4× `poller.interval`) or a heartbeat (5×), recomputes the open monthly/yearly periods
and net values from stored daily rows, and does an idempotent catch-up of any missed period
close on every run. It has no config section.

To rebuild monthly/yearly aggregates from daily data (e.g. after manually editing rows), stop
the app and run the `backfill` CLI job instead — see below.

## CLI: maintenance mode

Without a subcommand, `solis` runs the server. `solis <subcommand>` instead runs a one-shot job
against the database and exits `0`/`1`; it never starts the HTTP server, poller or WebSocket hub.

```bash
solis backfill --years 2           # recompute monthly + yearly for the current year + 2 closed years
solis backfill --years 2 --force   # same, also overwriting periods without complete daily history
solis version                      # print the build version, commit and date
```

The job refuses to run while the server holds its lock on the database, always takes a
backup first and verifies it with `PRAGMA integrity_check` (aborting with no writes on backup
failure), and refreshes the total baseline for any closed year it touches. Once retention
cleanup has deleted daily rows, closed years can no longer be recomputed: the job then
refuses `--years N > 0` instead of overwriting history with partial sums. All changes run in
one transaction: an error or Ctrl-C rolls everything back.

By default the job keeps what it cannot fully recompute: months and years that start before
the oldest daily row (e.g. inverter-reported history from before logging began) are skipped
and listed in the report, and the total baseline only covers years since the v3 cutover.
`--force` overrides both: those periods are overwritten with the sums of the rows that exist
(possibly partial or zero), and the baseline is rebuilt from all daily rows, pre-cutover
years included. Periods removed by retention are still refused.

The report lists one line per recomputed value (`old -> new`, `n/a` when there was no row),
one `skipped` line per protected period, and a summary. "Lower" values usually mean days
without daily rows; small increases usually mean the old value was a rounded inverter
figure.

## Running

### Docker

Templates live in [`example/`](example/):

| File | Use |
|---|---|
| `example/docker-compose.yaml` | Modbus TCP (data logger or RS485-to-TCP gateway) |
| `example/docker-compose.rtu.yaml` | Serial RS485 adapter (Modbus RTU, passes the device in) |
| `example/config.yaml` | Commented configuration with all settings |

```bash
cp example/docker-compose.yaml docker-compose.yaml
cp example/config.yaml config.yaml          # set modbus.address
docker compose up -d

# Development build from source
docker compose -f docker-compose.dev.yaml up --build
```

- **Restart policy required:** when self-healing fails the app exits with code 1 and relies
  on `restart: unless-stopped` (or systemd `Restart=on-failure`) to start again.

### Local

```bash
go run ./cmd
# or
make build && ./solis
```

Releases target linux and darwin (the database lock uses `flock`).

## Development

```bash
make build            # go build with version info (`./solis version`)
make check            # check-only pre-commit suite: format, lint, race tests, deadcode, govulncheck
go test -race ./...
```

See `AGENTS.md` for the full tool list (gocyclo, gosec, ineffassign, deadcode, staticcheck,
misspell, vet) and the project's architecture/testing conventions.

### Frontend

```bash
cd frontend
npm install
npm run dev                                          # proxies /api and /ws to localhost:8080
npm run typecheck && npm run lint && npm run knip    # the frontend checks CI runs
```

## License

MIT License. See [LICENSE](LICENSE) for details.
