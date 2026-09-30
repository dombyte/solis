# Agent Documentation

## Standard

This project follows the **Go Project Standard v3.0**
(local reference: `/home/dom/Dokumente/Git/go-project-standard.md`; will be replaced by a URL).
That document holds the rules for every Go project: dependency injection, composition root,
errors, logging, lifecycle, code style, naming, testing, tooling, Git workflow (branches,
Conventional Commits, no AI signatures) and review criteria.

This file holds only what is specific to solis. It wins for project-specific questions;
a deviation from a MUST rule of the standard is only valid if it is listed under
"Deviations" below with its reason. When code and this file disagree, fix one of them in
the same change.

**Current state:** v3 is implemented and follows the standard; the remaining gaps are listed
under "Migration backlog".

---

## 1. Project Overview

Solis Monitor polls a Solis hybrid inverter over Modbus (TCP or RTU), stores daily energy
counters and status/fault changes in SQLite, computes monthly/yearly/total values itself, and
serves a React dashboard (REST + WebSocket) from the same Go binary.

- **Input:** one inverter over Modbus TCP or RTU; the poller is the only Modbus user.
- **Outputs:** SQLite (daily/monthly/yearly/total values, status history), REST API,
  WebSocket live values, the SPA and Swagger UI served from disk (`frontend/dist`,
  `docs/dist`).
- **Runtime:** a single binary; without a subcommand it runs the server, `solis backfill`
  runs a maintenance job. Shipped as binaries (linux, darwin) and a multi-arch `scratch`
  image on ghcr.io.

---

## 2. Tools

```bash
make check                                  # ./scripts/pre-commit.sh, check-only (no mockery)
golangci-lint fmt --config .golangci.yml    # gofumpt + goimports (prefix github.com/dombyte/solis)
golangci-lint run --config .golangci.yml    # full linter set from the standard
go run golang.org/x/tools/cmd/deadcode@v0.50.0 -test ./...   # unused exported code
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...       # known vulnerabilities
go run github.com/vektra/mockery/v2@v2.53.7                 # regenerate mocks (.mockery.yaml)
go test -race ./...                         # all tests, as in CI
go test ./internal/solis -run TestBlockPlanGolden
make build                                  # ./solis with version info (ldflags)
make assets                                 # frontend/dist + docs/dist (make frontend / docs)
go run ./cmd                                # server (reads ./config.yaml)
go run ./cmd backfill --years 0             # maintenance job (app must be stopped), exits 0/1
cd frontend && npm run typecheck && npm run lint && npm run knip
```

- CI: `.github/workflows/checks.yml` runs the same checks plus tidy check, mock drift and
  the frontend checks; the pre-commit script is run by hand, not installed as a git hook.
- Release: push a `vX.Y.Z` tag on `main`; `release.yml` reuses the checks, then goreleaser
  (`.goreleaser.yaml`, `make assets` in its before hooks) builds archives and images.

---

## 3. Project Structure

```
cmd/                     main.go (subcommand dispatch, signal ctx, exit code), serve.go, backfill.go
internal/
  app/                   composition root: Create* factories, health adapters, logger wiring
  config/                YAML/env config, structural Validate(); domain rules injected by app
  period/                Period (day/month/year keys from one captured instant), rollover window
  eventbus/              event bus (ValuesUpdated, PeriodClosed), non-blocking, per-subscriber policy
  health/                supervisor, component contract, restart budget, ErrHealthFatal, snapshot
  solis/                 register table (Store enum), computed defs, block planning, decode, derive
  modbus/                Modbus TCP/RTU client, reconnect loop, Source slot for the poller
  poller/                poll loop, DayAttributor (day attribution + rollover), cold-start seeding
  aggregation/           PURE period math (monthly/yearly/total/net) shared by aggregator + CLI
  aggregator/            event-driven runner (debounce/heartbeat/catch-up) using aggregation
  cache/                 latest values; publishes change events; ReplaceDomain/Merge
  storage/               sole SQLite owner; PollerStore/AggregatorStore/ReadStore; closed periods
  history/               history read-model rows (daily/monthly/yearly/total/status data points)
  database/              manager: migrations (incl. V3 meta table), backups, retention cleanup
  database/migrations/   schema migrations (V3 meta table, status timestamp normalization)
  websocket/             subscription protocol, per-client diff, same-origin upgrader
  service/               ReadService for HTTP (cache + ReadStore + health snapshot)
  http/{httphandler,middleware,router,server}/
  maintenance/           CLI jobs (backfill): flock, backup, recompute, report
  logging/               zerolog constructor (no global logger)
  util/                  math, data types, Clock, Slot[T], DependencyError/RequireAll;
                         util/clocktest is the fake Clock for tests
  <pkg>/mocks/           mockery-generated mocks (never hand-edit)
frontend/                React 19 + Vite + Tailwind 4 + zustand SPA
docs/                    Swagger UI + openapi.yaml
example/                 docker-compose (TCP, RTU) and config.yaml templates for users
```

### Package notes

- **config:** structural checks in `Validate()`; rules owned by other packages (Modbus
  address, strict `rollover.time` HH:MM, poll timeout vs health grace) are `config.Rule`s
  passed in by `app.ConfigRules()`. Imports no domain package; `app` maps sections onto each
  package's own `Settings` (`internal/app/config_mapping.go`).
- **modbus:** transport selected by the `modbus.address` URL scheme (`tcp://host:port` or
  `rtu://<device path>`); reconnect with exponential backoff; construction never fails on an
  unreachable device (it starts `recovering`); raw reads and error classification only.
- **solis:** everything Solis-specific (register table, computed defs, `PlanBlocks`,
  `Decoder`, status/fault maps); anything generic goes to `util`. No Modbus client logic.
- **poller:** non-overlapping loop (next poll = interval after previous start); owns day
  attribution and rollover detection and emits `PeriodClosed`.
- **aggregation:** pure functions only — no I/O, no clock, no logging; called identically by
  the live aggregator and the CLI backfill.
- **aggregator:** event-driven; debounce 4× poll interval, heartbeat 5× poll interval, no
  config section; idempotent catch-up of missed period closes on every run.
- **cache:** never recreated at runtime; publishes change events on the injected bus on every
  write.
- **eventbus:** created in `app`, never recreated; `Publish` never blocks; each subscriber
  has a buffered channel with an overflow policy.
- **storage:** the only package that touches SQLite (single connection, WAL); enforces
  closed-period immutability (`ErrPeriodClosed`) and the net-key write guard; period sums
  run as SQL with an explicit bound, never by fetching rows into Go.
- **service:** read-side logic for HTTP (range parsing, key/level checks, history lookups);
  never touches Modbus, the poller or the aggregator.
- **http:** handlers in `httphandler`, middleware in `middleware` (never mixed), all routes
  in `router/router.go`, lifecycle in `server`.

---

## 4. Dependency Direction

```
router → httphandler → service → storage (ReadStore) / cache
```

Background components never reference each other; they interact only through the cache,
the event bus and the health supervisor:

```
poller      → solis, period, eventbus, health(Reporter), storage (store types), util
              + own interfaces (Store, Cache, Reader)
aggregator  → aggregation, solis, period, eventbus, health(Reporter), storage (store types),
              util + own Store, Cache
websocket   → eventbus, solis, health(Reporter), util + own Snapshotter
cache       → eventbus, solis
storage     → solis, period, history, database/migrations, util
history     → util only (shared by storage, service, httphandler)
config      → util only; imported only by cmd and app
modbus      → stdlib + simonvetter + util (Clock) + an injected zerolog.Logger
              (external layer: no config/health)
service     → storage (ReadStore only), health, history, period, solis + own interfaces
              (CacheReader, HealthSnapshotter)
httphandler → service, health, period, solis, util
app         → everything (composition root)
```

- "storage (store types)" means the interfaces, DTOs (`PollWrite`, `DailyRow`, …) and
  rejection sentinels in `storage/interfaces.go`; consumers never import `*storage.Storage`.
- Forbidden: poller ↔ aggregator, cache → websocket, httphandler → storage/modbus,
  anything → cmd/app.
- **Shared exported interfaces** (standard 3.1 exception, same contract for several
  consumers): `storage.{PollerStore,AggregatorStore,ReadStore,KeyLookup}`,
  `eventbus.{Publisher,Subscriber}`, `health.{Component,Reporter}`,
  `util.{Clock,Timer,Ticker}`. Every other interface is declared by its consumer. Every
  interface has a mockery mock.

---

## 5. Write Domains

| Writer | May write | Via |
|---|---|---|
| poller | `daily_values` (non-net keys, max of the open day), `error_data` (on change), poller key domain in cache | `storage.PollerStore`, `cache.ReplaceDomain` |
| aggregator | `monthly_values`, `yearly_values`, `total_values`, net daily rows, baselines/freeze marks in `meta`; computed keys in cache | `storage.AggregatorStore`, `cache.Merge` |
| maintenance (CLI) | computed tables + baselines, only with the app stopped (flock) | `storage` + `aggregation` |
| everyone else | nothing | read-only interfaces |

Closed periods are immutable; storage rejects such writes with `ErrPeriodClosed`. Values are
stored at full precision; rounding to 2 decimals happens only in JSON serialization.

---

## 6. Lifecycle

### Component supervision contract

Solis uses the single supervisor the standard allows (4.3): `internal/health`. Every
restartable component (modbus, poller, aggregator, WebSocket hub) implements:

```go
Start(ctx context.Context) error // start goroutines; return an error only for unrecoverable conditions
Stop() error                      // idempotent graceful shutdown
State() health.State              // Healthy | Recovering | Failed
LastBeat() time.Time              // atomic timestamp, updated by the component's own loop
```

- Constructed only through a `health.Factory` called by the supervisor; the component
  receives its `health.Reporter` in the constructor and pushes state transitions (rare,
  discrete) through it.
- Beat from the **loop**, not only after work: an idle aggregator or hub must still beat, a
  reconnecting Modbus client beats on every attempt and while waiting in backoff. Initial
  grace after (re)start is 5× the poll interval.
- Ordinary failures (poll timeout, read error, connection loss) are handled internally and
  reported as `Recovering`; they never touch the restart counter.
- No `Restart()` method, and no component restarts another. Restart = supervisor Stop →
  factory → Start.
- Restarts are spaced at least one sweep interval apart. The failure counter resets only
  after a restarted instance **kept** beating for the healthy grace (its latest beat lies a
  full grace after its start) — one beat followed by a hang is not a recovery.
- Escalation (`ErrHealthFatal`, cancels the root context via `context.WithCancelCause`):
  the restart budget (3) is exhausted, a restart's `Stop()` exceeded `StopTimeout` (a
  successor is never started next to a hung instance), or a non-restartable part (storage,
  cache, event bus, HTTP server; `Watch`ed) failed.
- Health/aggregation constants (restart budget 3, debounce 4×, heartbeat 5×, reset
  threshold 10 %) are constants, not config knobs, until real outage data says otherwise.

### Shutdown and exit codes

- Single-phase graceful shutdown with bounded `Stop()` and a hard overall deadline
  (`app.ShutdownTimeout`, 30 s). Periodic backup/cleanup jobs are joined before storage
  closes.
- No in-process restart of the app: on `ErrHealthFatal`, any startup error, a panic or
  `app.ErrShutdownTimeout`, `main()` exits 1 and the container runtime
  (`restart: unless-stopped`) starts a fresh process; a signal (clean shutdown) exits 0.

---

## 7. Behaviour Reference

### Project rules (in addition to the standard)

- ❌ **No Modbus reads from the HTTP path** — no `?direct=true`, no `PollNow`; only the
  poller talks to the inverter.
- ❌ **No `time.Now()` mid-run** in poller/aggregator logic — capture once, derive a
  `period.Period`, use the injected `util.Clock`.
- ❌ **No writes outside your write domain** (section 5).
- Every function that does I/O takes the caller's `ctx` (no `context.Background()` below
  `cmd`/`app`); SQL uses the `…Context` variants.
- Constructors report nil dependencies with `util.RequireAll` (→ `util.DependencyError`).
- Dangerous behaviour belongs in CLI jobs, never in config toggles.

### HTTP

- Handler functions return `http.Handler` (not `http.HandlerFunc`) and take
  `httphandler.HandlerDeps` (`Service` = the handler-declared `ReadService`, `Errors` = the
  central `*ErrorMapper`, `Clock`, `Timeout` = `app.timeout` for storage reads). Handlers
  parse, call `Service.Data`/`Keys`/`Health`, map errors, write JSON.
- Middleware (`Recover(log)`, `Logger(log)`, `SecurityHeaders`) is
  `func(http.Handler) http.Handler` with an injected logger where it logs.
- Routes (`router.SetupRoutes(Deps)`): `/health`, `/api/keys`, `/api/data/{key}`, `/ws`,
  `/docs/`, and the SPA. No CORS middleware.
- One error shape, documented in `docs/src/openapi.yaml`, written with
  `WriteError(w, status, msg)`: `{ "error": "Bad Request", "message": "…", "code": 400 }`.
- `service.NewReadService(service.Deps{Store, Cache, Health, Registry, Decoder, Log})`:
  every dependency is required; the service never receives a Modbus client, poller or
  aggregator.

### Health endpoint (fail-closed)

- `GET /health` reads the supervisor snapshot only; it never blocks on a component.
- 200 with `ok` or `degraded` (+ details). A restartable component's first failure (and a
  restarted instance that has not yet proven healthy) is `degraded`.
- **503** with the failed component and reason once a restart failed (a restarted instance
  failed or went silent again; stays 503 until an instance kept beating for the healthy
  grace), the restart budget is exhausted, a non-restartable part failed, or the snapshot is
  older than 3 sweeps (supervisor stalled). Never report `ok` when a subsystem is dead.

### WebSocket protocol

```json
→ { "type": "subscribe",   "keys": ["pv_total_power", "solis_status"] }
← { "type": "snapshot",    "values": { "pv_total_power": { "value": 5230, "timestamp": "…", "unit": "W" } } }
← { "type": "update",      "ts": "…", "values": { "pv_total_power": { "value": 5102.5 } } }
← { "type": "update",      "ts": "…", "values": {}, "removed": ["battery_power_signed"] }
→ { "type": "unsubscribe", "keys": ["solis_status"] }
← { "type": "error",       "code": "unknown_keys", "keys": ["foo"] }
```

- `removed` lists subscribed keys that disappeared from the cache (`ReplaceDomain` events
  carry removed keys too); clients drop those values.
- Only changed, subscribed keys are pushed (diff by `value` + `status_decoded` per client);
  pushes are coalesced (~75 ms) so poller + aggregator events close together become one
  frame with one frame-level `ts`.
- Unknown keys never drop the connection. `ping` from the client is accepted and ignored.
- Same-origin upgrader (Origin host must equal the Host header; the scheme is not
  compared). No history over WebSocket — history is REST only.
- Limits: at most 256 clients (the 257th is closed with 1013), at most 256 keys per frame.
- New UI-driven registers need no new endpoint/message: define the register in `solis`,
  clients subscribe.
- The protocol is a public interface; changing it is a breaking change (`!`).

### Configuration

Viper YAML config (`./config.yaml`) with `SOLIS_` env overrides (e.g.
`SOLIS_MODBUS_ADDRESS`). `example/config.yaml` is the commented reference of every key;
README has the defaults table.

- Removed in v3 (ignored with a warning): `app.serve_only`, the whole `aggregator` section
  (use the `backfill` CLI), `storage.monthly_retention`, `storage.yearly_retention` (follow
  `daily_retention`) and `storage.enable_migrations` (migrations always run).
- Cross-field rule: `poller.poll_timeout + modbus.timeout` < 3 × `poller.interval` (health
  healthy grace; a read in flight can overrun `poll_timeout` by up to `modbus.timeout`).
- Durations are strings with a unit (`30s`, `1y`; `d`, `w`, `y` added on top of Go units);
  a bare number is rejected (it would otherwise decode as nanoseconds). `poller.interval`
  and `app.timeout` are at least 1 s, `modbus.slave_id` is 1–247, block delays are ≥ 0.
- Every config key has a default in `setDefaults` (zero values included) — viper only
  applies `SOLIS_*` overrides to keys it knows; `TestSetDefaults_CoversEveryKey` enforces it.
- Timezone comes from the `TZ` env var (`time.Local`), never from config. Pin `TZ` in
  docker-compose; the image ships zoneinfo.

### Registers

All registers are built in `internal/solis/table.go`:

```go
solis.Register{
    Key:      "grid_power",
    Name:     "Grid Power",
    Address:  33130,           // 0 = computed/derived, never part of the poll plan
    DataType: solis.Int32,     // register count is derived from the type
    Scale:    1,
    Unit:     "W",
    Store:    solis.StoreNone, // none | daily | monthly | yearly | total | status
}
```

- `Store` decides the destination: `none` = cache only (live values), `status` =
  `error_data` on change, `daily/monthly/yearly/total` = the matching table. `Net: true`
  marks export−import registers (latest-value rule); net registers are always computed.
- The eight energy series are listed once in `energyKinds()`; each gets its daily (polled)
  register plus computed monthly/yearly/total registers and the daily→level edges.
- Computed registers (monthly/yearly/total/net) have **no address**; add their edge or net
  pair instead. Derived live values (e.g. `battery_power_signed`) have no address and are
  produced by `Decoder.Derive` after a full poll is decoded.
- `solis.NewRegistry()` validates the table (unique keys/addresses, valid `Store`, edges
  consistent) and fails startup otherwise; update `TestBlockPlanGolden` when addresses
  change.
- Do not add `Stability` or new `IsXxxRegister` lists — switch on `reg.Store`.
- `PlanBlocks` merges nearby addresses up to 125 registers per read. Prefer an alternate
  address inside an existing block over a new round-trip (e.g. `grid_power` at 33130
  instead of 33263).
- Decoding: `Decoder.Decode(reg, raw, pollStart)`, `DecodeBlock(block, blockRaw,
  pollStart)`, then `Derive(values, pollStart)`; decoded values are full precision.

### CLI maintenance (`solis backfill --years N`)

- Subcommands run a job against the database and exit 0/1; they never start the HTTP
  server, poller or WebSocket hub.
- `--years N` (default 0): recompute monthly + yearly (always both) for the current year
  plus N closed years; refresh the total baseline when a closed year is touched.
- Hard-refuse while the app runs: exclusive `flock` on a lock file next to the DB (the server
  holds a shared lock). Always take a backup first, verified with `PRAGMA integrity_check`
  (`database.CreateBackup`); no backup → no write → exit 1.
- Refuses to recompute periods whose daily rows retention already deleted (meta
  `purged_before`); with any purge, `--years N > 0` fails instead of writing partial sums.
- Periods that start before the oldest stored daily row are skipped and keep their stored
  value; the report lists them.
- The total baseline refresh sums the daily rows from the cutover year's 1 January through
  the baseline year, exactly what the live aggregator folds; nothing is refreshed while the
  baseline year precedes the cutover.
- `--force` recomputes periods without complete daily history (overwriting stored values
  with partial sums) and rebuilds the baseline from all daily rows. The `purged_before`
  refusal still applies.
- Uses the same `aggregation` functions as the live aggregator. Output: the backup path,
  one line per recomputed row (`monthly  pv_energy_monthly        2026-08   412.30 kWh ->
  409.87 kWh`, `n/a` when there was no row), one `skipped` line per period without complete
  daily history, and a summary line (recomputed / unchanged / lower / skipped counts).

### Frontend

- Stack: React 19, Vite, Tailwind 4, zustand, lucide-react, chart.js.
- Live data comes only via WebSocket subscriptions: components call `useSubscription(keys)`;
  the client ref-counts keys and re-subscribes on reconnect. History stays on REST.
- Register metadata lives in `src/lib/config/data.ts` (single source of truth for the UI).
- Mobile vs desktop: `useMobile()` (coarse pointer). The power-flow diagram uses the mobile
  variant only for coarse pointer **and** width < 768 px; tablets get the desktop variant.
- Power-flow diagram (`src/components/dashboard/flow/`): the current layout/geometry in
  `model.ts`, `FlowDesktop.tsx` and `FlowMobile.tsx` is the reference; icons from
  lucide-react; colors from CSS tokens in `src/index.css` (light + dark), never hardcoded
  hex; edge animation via CSS keyframes (binary on/off, fixed speed,
  `prefers-reduced-motion` respected), not `requestAnimationFrame` + state updates.
- Direction rules: PV → inverter; inverter → household/backup; grid export out / import in;
  battery `battery_power_signed > 0` = charging (into battery). Zero = shown as 0, edge
  stops; missing data = node (or whole diagram) grayed out.

### Testing (in addition to the standard)

- Coverage target: > 90 % per package (stricter than the standard's minimum).
- Time-dependent logic (debounce/heartbeat, supervisor graces, rollover windows, backoff)
  is driven by `util/clocktest`, never by wall-clock sleeps.
- Rollover/attribution and `period` tests are table-driven and include midnight, DST
  spring-forward and fall-back nights (`time.LoadLocation("Europe/Berlin")`).
- Storage tests cover closed-period rejection and write-domain guards.
- HTTP tests drive handlers with `httptest` and a mockery `ReadService` mock:

```go
svc := mocks.NewMockReadService(t)
svc.EXPECT().Health().Return(health.Snapshot{Status: health.StatusOK})
rec := httptest.NewRecorder()
GetHealthHandler(HandlerDeps{Service: svc}).
    ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
require.Equal(t, http.StatusOK, rec.Code)
```

### Security and platforms

- Releases target linux and darwin. The server needs `flock` (unix); on other platforms
  `maintenance.AcquireShared` fails and the server does not start.
- LAN app without authentication: the WebSocket same-origin check does not defend against
  DNS rebinding, and REST has no CORS. Do not expose it to the internet without a reverse
  proxy that authenticates.

---

## 8. Design Decisions

Deliberate choices, with the reason, for behaviour that is not obvious from the code.

- **`meta` table:** the only schema addition. Holds the cutover date, closed-period
  watermarks (`closed:daily:<key>`, `closed:netdaily`, `closed:monthly`, `closed:yearly`),
  the total baselines (`baseline:<key>`), `baseline_year` and the retention watermark
  `purged_before`. At cutover: daily/net-daily = today − 2 (the "closed day"); monthly = the
  month before the closed day's month, yearly = the year before the closed day's year (a
  cutover on the 1st keeps the previous month open while its last day is still writable).
- **Cutover month/year keep the inverter values:** the cutover month and year stay open, so
  at cutover the difference between each stored (inverter-reported) monthly/yearly value
  and its daily sum up to that day is stored as `offset:<level>:<period>:<key>`; the
  aggregator adds it on top of the growing daily sum. `solis backfill` overrides: it writes
  the pure daily sum and deletes the offset.
- **Retention:** monthly and yearly rows follow `storage.daily_retention` (only frozen years
  are ever deleted). This is intentional; keep a longer retention to keep more history.
- **Process restarts belong to the container runtime:** the app never restarts itself
  in-process (a hung goroutine can only be ended by a process exit); a fatal health
  escalation exits 1.
- **Cache domains:** the poller replaces only its own keys (`ReplaceDomain`); the aggregator
  merges its disjoint computed keys (`Merge`). Neither wipes the other.
- **Net values:** net daily rows live in `daily_values` and are written only by the
  aggregator; storage rejects net keys from the poller and non-net keys from the
  aggregator. Net values are computed from the same run's export/import values, never read
  back from the cache.
- **Totals are app-lifetime:** computed from daily rows plus a baseline folded in at each
  year close; pre-cutover inverter totals are not carried over.
- **Catch-up:** every aggregator run finalises and freezes any closed, not-yet-frozen
  month/year; `PeriodClosed` (published once per rollover window, on the first closing key
  or a forced close) only makes that happen sooner.
- **Day attribution:** the "new day" after a counter reset is the day whose local midnight
  is nearest the rollover time; per-key bases live in memory and are seeded from the DB at
  start. The inverter resets all daily counters together: a key that cannot show its own
  reset (no base, or a base of 0, so it never decreases) follows a reset another key
  confirmed in the same window, and is held (not written) from the opening day's midnight
  until then, so its post-reset energy is never counted on both days. After a confirmed
  reset any decrease on the closing day is the reset. Seeding returns the day closes missed
  while the app was down, and closes are written even while Modbus is disconnected.
- **WebSocket diff:** by `value` + `status_decoded` only (timestamps change every poll);
  each `update` frame carries one frame-level `ts`.
- **Liveness:** every component beats from its own loop, idle or not; the Modbus reconnect
  loop beats while waiting in backoff.
- **Modbus wiring:** construction never fails on connectivity; the client is published
  through a `util.Slot` so the poller always reads the current client after a restart.
- **Derived values:** `battery_power_signed` is produced by `Decoder.Derive` after a full
  poll (it needs two registers).
- **Read plan:** 3 Modbus reads (grid power at 33130 so it falls inside an existing block),
  pinned by the block-plan golden test.
- **Shutdown:** single-phase with a hard deadline; no second-signal force mode.
- **Grid power sign** (positive = export) and the battery direction values are as verified
  on the device; if a firmware changes them, flip in `Decoder.Derive`, not in the UI.

### Lessons learned (v2 → v3)

1. **No direct Modbus reads from HTTP — never implement `?direct=true` again.** In v2 the
   HTTP path shared (then duplicated) the Modbus connection; the sequential-only inverter
   made HTTP reads wait behind the poller (600–700 ms → 1.5 s), and `?direct=true` was not
   passed through every code path. v3: the poller is the only Modbus user (one
   connection); HTTP reads cache and SQLite only.
2. **Don't add locks on top of the Modbus library.** simonvetter/modbus serializes requests
   itself; lock only state you own (e.g. the reconnect path).
3. **Fetch only the data you need.** v2 fetched all history rows and filtered in Go (~14×
   slower for one key). Query per requested key with SQL bounds; period sums run in SQL.
4. **Library migrations cost a little.** simonvetter returns `[]byte`, so words are
   converted by hand (~10–20 ns per read) — negligible next to network latency.

---

## 9. Deviations from the Standard

None. Gaps in the code are backlog items below, not accepted deviations. Add a row here
(rule, deviation, reason) only for a choice that is meant to stay:

| Rule | Deviation | Reason |
|---|---|---|

---

## 10. Migration Backlog

Known gaps between the current code and standard v3. Each item is its own `refactor/…` (or
`fix/…`) branch; update this list when an item is done.

1. **Config path from a flag (standard 5):** `cmd/main.go` reads a fixed `./config.yaml`.
   Add a `-config` flag defaulting to `config.yaml` (image and compose files keep working),
   or move this item to "Deviations" with the reason.
2. **Report all config problems (standard 5):** `AppConfig.Validate` returns on the first
   failing section/rule; collect every problem with its field path (`errors.Join`).
3. **`t.Parallel()` (standard 9):** no test uses it yet; add it to table-driven tests
   without shared state.
4. **Build info in the dev image (standard 10):** `Dockerfile` builds without the version
   ldflags, so a locally built image logs `dev`/`unknown`; pass them as build args.

---

## 11. Commit Scopes

Conventional Commits per the standard; no AI signatures. Scopes used here:

`ui`, `api`, `ws`, `health`, `poller`, `aggregator`, `storage`, `modbus`, `solis`,
`config`, `backfill`, `docker`, `release`, `ci`, `deps`

Examples:

```
feat(ui): show the energy cards as a swipe carousel on phones
fix(storage): apply SQLite pragmas through the DSN on every connection
refactor(storage)!: remove V1/V2 migrations and refuse pre-V2 databases
docs: follow go-project-standard v3 in AGENTS.md
```

`!` marks changes that break users upgrading: REST/WebSocket protocol, removed config or
migration paths, changed runtime requirements.
