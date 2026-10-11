# Agent Documentation

The complete, self-contained rule set for solis. **MUST** = blocker, **SHOULD** = fix
unless there is a written reason, **MAY** = allowed. A MUST deviation is valid only if
listed in section 12. When code and this file disagree, fix one of them in the same change.

## 1. Overview

Polls one Solis hybrid inverter over Modbus (TCP or RTU), stores daily energy counters and
status/fault changes in SQLite, computes monthly/yearly/total values itself, and serves a
React dashboard (REST + WebSocket) plus Swagger UI from the same binary (`go:embed` of
`frontend/dist`, `docs/dist`). No subcommand = server; `solis backfill` = maintenance job.
Shipped as linux/darwin binaries and a multi-arch `scratch` image on ghcr.io.

## 2. Tools

```bash
make check                                  # ./scripts/pre-commit.sh, check-only (no mockery)
golangci-lint fmt --config .golangci.yml    # gofumpt + goimports (prefix github.com/dombyte/solis)
golangci-lint run --config .golangci.yml
go run golang.org/x/tools/cmd/deadcode@v0.52.0 -test ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
go tool mockery                             # regenerate mocks (.mockery.yaml)
go test -race ./...                         # as in CI
make assets | build | all | docker          # dists; ./solis with ldflags; both; dev image
go run ./cmd [-config <path>]               # server (default ./config.yaml)
go run ./cmd backfill --years 0             # maintenance job (app stopped), exits 0/1
go run ./cmd version                        # build info (also -version)
cd frontend && npm run typecheck && npm run lint && npm run knip
```

- `make check` runs the same checks as CI (`checks.yml`: plus tidy check, mock drift,
  frontend checks); run it by hand before every merge. `.golangci.yml` is the source of
  the lint and size rules — don't relax it without a deviation in section 12.
- Tool versions are pinned and bumped deliberately, same locally and in CI.
- Renovate is the only dependency updater. `renovate.json` MUST track every pinned version
  (gomod, Dockerfiles, compose, GitHub Actions, npm in `frontend/` + `docs/`, regex managers
  for pinned `go run` tools and the golangci-lint `version:`); a change that adds a pin
  extends it.
- Release: SemVer tag `vX.Y.Z` on `main` → `release.yml` reruns the checks, goreleaser
  (`make assets` in its before hooks) builds archives and images (`latest` only for stable).

## 3. Structure

```
cmd/                 main.go (dispatch, signal ctx, exit code, ldflags), serve.go, backfill.go
internal/
  app/               composition root: Create* factories, health adapters, logger, config mapping
  buildinfo/         build info passed down from cmd
  config/            YAML/env config, structural Validate(); domain rules injected by app
  period/            Period (day/month/year keys from one captured instant), rollover window
  eventbus/          ValuesUpdated, PeriodClosed; Publish never blocks; per-subscriber overflow policy
  health/            supervisor, component contract, restart budget, ErrHealthFatal, snapshot
  solis/             register table, computed defs, PlanBlocks, Decoder, status/fault maps
  modbus/            TCP/RTU client, reconnect loop, Source slot for the poller
  poller/            poll loop, DayAttributor (day attribution + rollover), cold-start seeding
  aggregation/       PURE period math (no I/O, clock, logging), shared by aggregator + CLI
  aggregator/        event-driven runner (debounce/heartbeat/catch-up)
  cache/             latest values; publishes change events; ReplaceDomain/Merge
  storage/           sole SQLite owner; PollerStore/AggregatorStore/ReadStore; closed periods
  history/           history read-model rows
  database/          migrations (incl. V3 meta table), backups, retention cleanup
  websocket/         subscription protocol, per-client diff, same-origin upgrader
  service/           ReadService for HTTP (cache + ReadStore + health snapshot)
  http/{httphandler,middleware,router,server}/
  maintenance/       CLI jobs: flock, backup, recompute, report
  logging/           zerolog constructor (no global logger)
  util/              math, data types, Clock, Slot[T], RequireAll; util/clocktest = fake Clock
  <pkg>/mocks/       mockery-generated (never hand-edit)
frontend/            React 19 + Vite + Tailwind 4 + zustand + lucide-react + chart.js
docs/                Swagger UI + openapi.yaml
example/             docker-compose (TCP, RTU) and config.yaml templates
```

- All Go code lives in `internal/`, at most 3 levels deep. No `pkg/` without the user's
  confirmation.

Package rules:

- **config** imports no domain package. `Validate()` reports every problem with its field
  path (`poller.interval: must be at least 1s, got 0s; …`). Rules owned elsewhere (Modbus
  address, strict `rollover.time` HH:MM, poll timeout vs health grace) are `config.Rule`s
  from `app.ConfigRules()` returning `*config.ValidationError`. `app` maps sections onto each
  package's own `Settings` (`internal/app/config_mapping.go`).
- **modbus**: transport chosen by `modbus.address` scheme (`tcp://host:port`,
  `rtu://<device>`); exponential-backoff reconnect; construction never fails on an
  unreachable device (starts `recovering`); raw reads + error classification only. Published
  via `util.Slot` so the poller always reads the current client after a restart.
- **solis**: everything Solis-specific; generic code goes to `util`; no Modbus client logic.
- **poller**: non-overlapping loop (next poll = interval after previous start); owns day
  attribution and rollover; emits `PeriodClosed`.
- **storage**: single connection, WAL; enforces closed-period immutability
  (`ErrPeriodClosed`) and write domains; period sums run in SQL with explicit bounds.
- **http**: handlers in `httphandler` (no business logic; they call `service`), middleware
  in `middleware` (never mixed), all routes in `router/router.go`, lifecycle in `server`.

## 4. Architecture

- **DI (MUST):** every dependency comes in through the constructor as a small interface
  declared by the consumer (except the shared ones below); nothing is created inside
  methods. `internal/app` is the only composition root (`Create*` factories) and, with
  `cmd`, the only importer of `config`.
- **Required deps:** constructors check them with `util.RequireAll` (→
  `util.DependencyError`); a nil dependency fails startup. No optional components behind
  `if x != nil` — a switchable feature is a separate component or subcommand. Missing
  *data* (empty cache) returns an explicit typed answer. Functional options MAY carry 3+
  optional settings, never dependencies.
- **No global state:** no package-level loggers, singletons, caches or lookup maps. Only
  `Err…` sentinels and the linker-set build info in `cmd/main.go` are allowed.

### Dependency direction

`router → httphandler → service → storage (ReadStore) / cache`. Dependencies flow down
only. Background components never reference each other; they meet only through cache,
event bus and health supervisor.

| Package | May import |
|---|---|
| poller | solis, period, eventbus, health (Reporter), storage types, util |
| aggregator | aggregation, solis, period, eventbus, health (Reporter), storage types, util |
| websocket | eventbus, solis, health (Reporter), util |
| cache | eventbus, solis |
| storage | solis, period, history, database/migrations, util |
| history, config | util (config only imported by cmd and app) |
| modbus | stdlib, simonvetter, util (Clock), injected zerolog.Logger — no config/health |
| service | storage (ReadStore only), health, history, period, solis |
| httphandler | service, health, period, solis, buildinfo, util |
| buildinfo | stdlib |
| app | everything, incl. frontend/docs embed packages |

- "storage types" = interfaces, DTOs (`PollWrite`, `DailyRow`, …) and sentinels in
  `storage/interfaces.go`; nobody imports `*storage.Storage`.
- Forbidden: poller ↔ aggregator, cache → websocket, httphandler → storage/modbus,
  anything → cmd/app.
- Shared exported interfaces (same contract for several consumers):
  `storage.{PollerStore,AggregatorStore,ReadStore,KeyLookup}`,
  `eventbus.{Publisher,Subscriber}`, `health.{Component,Reporter}`, `util.{Clock,Timer,Ticker}`.
  Every interface has a mockery mock.

## 5. Write Domains

| Writer | May write | Via |
|---|---|---|
| poller | `daily_values` (non-net keys, max of the open day), `error_data` (on change), its own cache keys | `PollerStore`, `cache.ReplaceDomain` |
| aggregator | monthly/yearly/total values, net daily rows, baselines/freeze marks in `meta`, computed cache keys | `AggregatorStore`, `cache.Merge` |
| maintenance CLI | computed tables + baselines, only with the app stopped (flock) | `storage` + `aggregation` |
| everyone else | nothing | read-only interfaces |

- Storage rejects net keys from the poller and non-net keys from the aggregator, and any
  write to a closed period (`ErrPeriodClosed`).
- `ReplaceDomain` and `Merge` touch disjoint keys; neither wipes the other.
- Values are stored at full precision; rounding to 2 decimals only in JSON.

## 6. Lifecycle and Concurrency

Every goroutine has an owner that starts, stops and waits for it. Waiting loops use the
injected `util.Clock`; channels document their owner (sender closes) and full behaviour.

### Supervision

Single supervisor: `internal/health`. Restartable components (modbus, poller, aggregator,
WebSocket hub) implement:

```go
Start(ctx context.Context) error // error only for unrecoverable conditions
Stop() error                      // idempotent, graceful, bounded
State() health.State              // Healthy | Recovering | Failed
LastBeat() time.Time              // atomic, updated by the component's own loop
```

- Built only by a `health.Factory` called by the supervisor; receives its `health.Reporter`
  in the constructor and pushes (rare) state transitions through it. No `Restart()`; no
  component restarts another. Restart = Stop → factory → Start.
- Beat from the **loop**, not only after work: idle aggregator/hub still beat; the Modbus
  reconnect loop beats on every attempt and while in backoff. Initial grace after
  (re)start = 5× poll interval.
- Ordinary failures (poll timeout, read error, lost connection) are handled internally
  (log, back off, retry) as `Recovering` and never count as restarts.
- Restarts are at least one sweep apart. The failure counter resets only once a restarted
  instance **kept** beating for the healthy grace (latest beat ≥ grace after its start).
- `ErrHealthFatal` (cancels root ctx via `context.WithCancelCause`) when: restart budget (3)
  exhausted; a restart's `Stop()` exceeded `StopTimeout` (never start a successor next to a
  hung instance); a `Watch`ed non-restartable part (storage, cache, event bus, HTTP server)
  failed.
- Constants, not config: restart budget 3, aggregator debounce 4× / heartbeat 5× poll
  interval, reset threshold 10 %.

### Shutdown and exit codes

- `main` builds a signal context (SIGINT/SIGTERM). Single-phase shutdown: producers first,
  consumers drain, each `Stop()` bounded, one hard deadline `app.ShutdownTimeout` (30 s);
  no second-signal force mode. Backup/cleanup jobs are joined before storage closes.
- Exit 1 on `ErrHealthFatal`, startup error, panic or `app.ErrShutdownTimeout`; exit 0 on
  signal. The app never restarts itself; the container runtime does
  (`restart: unless-stopped`). `log.Fatal`/`os.Exit` only in `main`.

## 7. Code Rules

- **Errors:** wrap with `%w` and a package prefix (`storage: save day %s: %w`); sentinels
  for branching, `…Error` types when callers need data. Never swallow (`_ = f()` needs a
  `//nolint` with reason). Log once, where the error is handled. **No panic.**
- **Logging:** injected `zerolog.Logger` scoped with `component`; context fields, not
  formatted strings; `debug` per cycle, `info` lifecycle, `warn` recovered, `error` needs
  attention. Never log secrets.
- **Style:** gofumpt + goimports; lines ≤ 100, functions ≤ 40 lines/statements, gocyclo
  < 8, ≤ 5 params, no magic numbers (all lint-enforced). Stdlib naming (`ID`, `URL`,
  one-letter receivers, verb + noun functions). Doc comments on packages and exported
  types (MUST) and functions (SHOULD); inline comments explain *why*.

### Project rules

- ❌ **No Modbus reads from the HTTP path** (no `?direct=true`, no `PollNow`). In v2 this
  made HTTP wait behind the sequential-only inverter (0.6 → 1.5 s) and the flag leaked
  through some paths. Only the poller talks to the inverter.
- ❌ **No `time.Now()` mid-run** in poller/aggregator: capture once, derive a
  `period.Period`, use the injected `util.Clock`.
- ❌ **No writes outside your write domain** (section 5).
- ❌ **No locks on top of the Modbus library** — simonvetter serializes requests; lock only
  state you own (e.g. reconnect).
- Fetch only what you need: query per key with SQL bounds (v2 filtered all rows in Go,
  ~14× slower).
- I/O functions take the caller's `ctx` (no `context.Background()` below `cmd`/`app`); SQL
  uses `…Context` variants.
- Dangerous behaviour belongs in CLI jobs, never in config toggles.

## 8. Behaviour Reference

### HTTP

- Routes (`router.SetupRoutes(Deps)`): `/health`, `/api/keys`, `/api/data/{key}`,
  `/api/version`, `/ws`, `/docs/`, SPA. No CORS middleware.
- Handlers return `http.Handler` and take `httphandler.HandlerDeps` (`Service` =
  handler-declared `ReadService`, `Errors` = central `*ErrorMapper`, `Clock`, `Timeout` =
  `app.timeout`, `Build`). They parse, call `Service.Data`/`Keys`/`Health`, map errors,
  write JSON. Input is validated at this boundary or in `service`.
- Middleware (`Recover(log)`, `Logger(log)`, `SecurityHeaders`) is
  `func(http.Handler) http.Handler`.
- One error shape (`WriteError(w, status, msg)`, documented in `docs/src/openapi.yaml`):
  `{ "error": "Bad Request", "message": "…", "code": 400 }`.
- `service.NewReadService(service.Deps{Store, Cache, Health, Registry, Decoder, Log})`, all
  required; never gets a Modbus client, poller or aggregator.
- Static files come from `fs.FS` in `router.Deps` (`Frontend`, `Docs`): `app` passes
  `frontend.Dist()`/`docs.Dist()`, tests an `fstest.MapFS`. A tree without `index.html` is
  not mounted (`app` warns). `go:embed all:dist*` also matches the committed `dist.md`, so
  Go builds/tests/lint work without npm; release builds always run `make assets`.

### Version

One version for binary, UI and docs: ldflags (`Version`, `Commit`, `BuildDate`,
`GoVersion`) → `buildinfo.Info` → `solis version`, startup log and `GET /api/version`;
the frontend and Swagger UI get `VITE_APP_VERSION` from the same source (make,
goreleaser, Dockerfile). No `version.json`. The frontend polls `/api/version`; a different version shows the update
banner. `dev` on either side never reports an update.

### Health endpoint (fail-closed)

`GET /health` reads only the supervisor snapshot, never blocks on a component.

- **200** `ok` or `degraded` (+ details). `degraded` = a restartable component's first
  failure, or a restarted instance not yet proven healthy.
- **503** (+ failed component, reason) when a restarted instance failed or went silent again
  (until one kept beating for the healthy grace), the budget is exhausted, a non-restartable
  part failed, or the snapshot is older than 3 sweeps. Never `ok` with a dead subsystem.

### WebSocket protocol (public interface; changes are breaking, `!`)

```json
→ { "type": "subscribe",   "keys": ["pv_total_power", "solis_status"] }
← { "type": "snapshot",    "values": { "pv_total_power": { "value": 5230, "timestamp": "…", "unit": "W" } } }
← { "type": "update",      "ts": "…", "values": { "pv_total_power": { "value": 5102.5 } } }
← { "type": "update",      "ts": "…", "values": {}, "removed": ["battery_power_signed"] }
→ { "type": "unsubscribe", "keys": ["solis_status"] }
← { "type": "error",       "code": "unknown_keys", "keys": ["foo"] }
→ { "type": "ping" }       ← { "type": "pong" }
```

- Only changed, subscribed keys are pushed, diffed per client by `value` + `status_decoded`
  (timestamps change every poll). Pushes are coalesced (~75 ms) into one frame with one
  `ts`. `removed` = subscribed keys gone from the cache (`ReplaceDomain` events carry them).
- Unknown keys never drop the connection. Frontend pings every 20 s and on tab focus;
  reconnects if no frame arrives within 5 s.
- Same-origin upgrader (Origin host == Host header, scheme ignored). No history over WS.
- Limits: 256 clients (257th closed with 1013), 256 keys per frame.
- New UI registers need no new endpoint: define the register in `solis`, clients subscribe.

### Configuration

Viper YAML (`./config.yaml`, gitignored; path via `-config`) with `SOLIS_` env overrides
(`SOLIS_MODBUS_ADDRESS`). Order: defaults → file → env → validate once at startup.
`example/config.yaml` documents every key; README lists only the common ones with defaults.

- Every key has a default in `setDefaults` (zero values too) — viper only applies env
  overrides to known keys; `TestSetDefaults_CoversEveryKey` enforces it. An invalid env
  override is an error, never ignored.
- Durations need a unit (`30s`, `1y`; `d`, `w`, `y` added); bare numbers are rejected.
  `poller.interval`, `app.timeout` ≥ 1 s; `modbus.slave_id` 1–247; block delays ≥ 0.
- Cross-field: `poller.poll_timeout + modbus.timeout` < 3 × `poller.interval` (healthy
  grace; a read in flight can overrun `poll_timeout` by `modbus.timeout`).
- Timezone only from `TZ` (`time.Local`), never config; pin it in docker-compose.
- Secrets (if ever needed) come from env vars or secret files, never the committed config.
- Removed in v3 (ignored with a warning): `app.serve_only`, the `aggregator` section (use
  `backfill`), `storage.monthly_retention`, `storage.yearly_retention`,
  `storage.enable_migrations` (migrations always run).

### Registers (`internal/solis/table.go`)

```go
solis.Register{
    Key: "grid_power", Name: "Grid Power",
    Address:  33130,           // 0 = computed/derived, never polled
    DataType: solis.Int32,     // register count derived from the type
    Scale: 1, Unit: "W",
    Store:    solis.StoreNone, // none | daily | monthly | yearly | total | status
}
```

- `Store`: `none` = cache only, `status` = `error_data` on change, others = matching table.
  Switch on `reg.Store`; never add `Stability` or `IsXxxRegister` lists.
- `energyKinds()` lists the eight energy series once; each gets a polled daily register
  plus computed monthly/yearly/total registers and daily→level edges.
- Computed registers (monthly/yearly/total/net, `Net: true` = export − import, latest-value
  rule) have no address — add an edge or net pair. Derived live values (e.g.
  `battery_power_signed`, needs two registers) come from `Decoder.Derive` after a full poll.
- `solis.NewRegistry()` validates the table (unique keys/addresses, valid `Store`,
  consistent edges) and fails startup otherwise.
- `PlanBlocks` merges nearby addresses up to 125 registers per read; the plan is 3 reads,
  pinned by `TestBlockPlanGolden` (update it when addresses change). Prefer an alternate
  address inside an existing block over a new round-trip (`grid_power` at 33130, not 33263).
- Decode: `Decoder.Decode(reg, raw, pollStart)` / `DecodeBlock(block, blockRaw, pollStart)`,
  then `Derive(values, pollStart)`; full precision.
- Grid power sign (positive = export) and battery direction are verified on the device; if
  firmware changes them, flip in `Decoder.Derive`, not in the UI.

### CLI maintenance (`solis backfill --years N`)

- Runs against the DB and exits 0/1; never starts HTTP, poller or hub.
- Recomputes monthly + yearly for the current year plus N closed years (default 0) with the
  same `aggregation` functions as the live aggregator; refreshes the total baseline when a
  closed year is touched.
- Refuses while the app runs: exclusive `flock` next to the DB (server holds a shared one).
  Always backs up first, verified with `PRAGMA integrity_check` (`database.CreateBackup`);
  no backup → no write → exit 1.
- Refuses periods whose daily rows retention deleted (meta `purged_before`); with any
  purge, `--years N > 0` fails rather than write partial sums.
- Periods starting before the oldest daily row are skipped (stored value kept, reported).
- Baseline refresh sums daily rows from the cutover year's 1 January through the baseline
  year (as the live aggregator folds); nothing is refreshed while the baseline year precedes
  the cutover.
- `--force` recomputes periods without complete daily history (partial sums) and rebuilds
  the baseline from all daily rows; the `purged_before` refusal still applies.
- Output: backup path; one line per row (`monthly  pv_energy_monthly        2026-08
  412.30 kWh -> 409.87 kWh`, `n/a` if no row); one `skipped` line per incomplete period;
  a summary (recomputed / unchanged / lower / skipped).

### Frontend

- Live data only via WebSocket: components call `useSubscription(keys)`; the client
  ref-counts keys and re-subscribes on reconnect. History via REST.
- Register metadata: `src/lib/config/data.ts` (single UI source of truth).
- `useMobile()` = coarse pointer. The power-flow diagram uses the mobile variant only for
  coarse pointer **and** width < 768 px (tablets get desktop).
- Power-flow (`src/components/dashboard/flow/`): `model.ts`, `FlowDesktop.tsx`,
  `FlowMobile.tsx` are the layout reference; lucide-react icons; colors only from CSS
  tokens in `src/index.css` (light + dark), never hex; edge animation via CSS keyframes
  (on/off, fixed speed, respects `prefers-reduced-motion`), not rAF + state.
- Directions: PV → inverter; inverter → household/backup; grid export out / import in;
  `battery_power_signed > 0` = charging. Zero = shown as 0, edge stops; missing data =
  node (or whole diagram) grayed out.

### Security and platforms

- linux and darwin only: the server needs `flock`; elsewhere `maintenance.AcquireShared`
  fails and the server does not start.
- LAN app without auth: the WS origin check does not stop DNS rebinding and REST has no
  CORS. Never expose it to the internet without an authenticating reverse proxy (which also
  terminates TLS and rate-limits).
- Every outgoing call (Modbus) has a timeout.

## 9. Testing

- testify (`require`/`assert`) + mockery mocks in `<pkg>/mocks` (never hand-edited; CI
  fails on drift). Table-driven, `t.Parallel()` where possible, always `-race`.
- No network, real clock or sleeps: `util/clocktest` drives debounce, heartbeat, graces,
  rollover and backoff. Real-device tests go behind `//go:build integration`.
- New code comes with tests, bug fixes with a regression test. Coverage > 90 % per
  package; `cmd`/`app` are covered by a startup test.
- Rollover/attribution and `period` tests cover midnight and DST (`Europe/Berlin`);
  storage tests cover closed-period rejection and write-domain guards; HTTP tests use
  `httptest` + the mockery `ReadService` mock.

## 10. Design Decisions

- **`meta` table** (only schema addition): cutover date, closed-period watermarks
  (`closed:daily:<key>`, `closed:netdaily`, `closed:monthly`, `closed:yearly`), total
  baselines (`baseline:<key>`), `baseline_year`, retention watermark `purged_before`. At
  cutover: daily/net-daily closed = today − 2; monthly = month before that day's month;
  yearly = year before that day's year (a cutover on the 1st keeps the previous month open
  while its last day is still writable).
- **Cutover month/year keep inverter values:** they stay open, and the difference between
  each stored monthly/yearly value and its daily sum is stored as
  `offset:<level>:<period>:<key>`, added on top of the growing daily sum. `backfill` writes
  the pure daily sum and deletes the offset.
- **Totals are app-lifetime:** daily rows plus a baseline folded in at each year close;
  pre-cutover inverter totals are not carried over.
- **Retention:** monthly/yearly rows follow `storage.daily_retention` (only frozen years are
  deleted). Intentional; keep a longer retention for more history.
- **Catch-up:** every aggregator run finalises and freezes any closed, unfrozen month/year;
  `PeriodClosed` (once per rollover window, on the first closing key or a forced close)
  only makes it happen sooner.
- **Net values** are computed from the same run's export/import values, never read back
  from the cache.
- **Day attribution:** after a counter reset the "new day" is the day whose local midnight
  is nearest the rollover time. Per-key bases live in memory, seeded from the DB at start.
  The inverter resets all daily counters together: a key that cannot show its own reset
  (no base or base 0) follows a reset another key confirmed in the same window, and is held
  (not written) from the opening day's midnight until then, so post-reset energy is never
  counted twice. After a confirmed reset any decrease on the closing day is the reset.
  Seeding returns day closes missed while down; closes are written even while Modbus is
  disconnected.
- **Embedded web assets:** archives and image ship only the binary; dists are build inputs.
- **simonvetter returns `[]byte`:** words are converted by hand (~10–20 ns/read),
  negligible next to network latency.

## 11. Git

- Branch `type/description` + PR (CI green before merge); the maintainer MAY commit to
  `main` directly after `make check`. Regular merges; squash/rebase only on request.
  Several small changes MAY share one batch PR: local `--no-ff` branches merged into one
  pushed batch branch.
- Commits (MUST): `<type>(<scope>)!: <imperative lowercase subject>` with types `feat`,
  `fix`, `perf`, `refactor`, `docs`, `chore`, `build`, `ci`, `test`, `style`
  (`<type>(deps)` for bumps). The changelog is built from subjects.
- Scopes: `ui`, `api`, `ws`, `health`, `poller`, `aggregator`, `storage`, `modbus`,
  `solis`, `config`, `backfill`, `docker`, `release`, `ci`, `deps`.
- `!` = breaks upgrading users (REST/WS protocol, removed config or migration paths,
  changed runtime requirements); a footer alone is not enough.

```
feat(ui): show the energy cards as a swipe carousel on phones
refactor(storage)!: remove V1/V2 migrations and refuse pre-V2 databases
```

## 12. Deviations and Backlog

| Rule | Deviation | Reason |
|---|---|---|
| Go tools pinned in `go run` commands | mockery is a `tool` directive in `go.mod` (`go tool mockery`) | a pinned `go run` checks ~20 modules against sum.golang.org on every CI run; a checksum-DB hiccup failed the mock drift job. Bump: `go get -tool github.com/vektra/mockery/v3@vX.Y.Z` |

Backlog (gaps between code and these rules): none open.
