# Agent Documentation


## Project Overview

Solis Monitor polls a Solis hybrid inverter over Modbus (TCP or RTU), stores daily energy counters and
status/fault changes in SQLite, computes monthly/yearly/total values itself, and serves a React
dashboard (REST + WebSocket) from the same Go binary.

- **v3 is in progress.** The authoritative design is `ref/solis-monitor-v3-refactor-specification(3).md`;
  the implementation plan, spec decisions (D1–D15) and phase order are in `Plan.md`.
- The dashboard power-flow reference is `ref/solis-v3-dashboard-power-flow-diagram-prototype.tsx`
  (layout/geometry/colors/animation; emoji icons are placeholders for lucide-react).
- Where this file and the spec disagree, the spec + `Plan.md` decisions win; update this file.
- Cross-project Go standard: `ref/Arch_Plan.md` (DI, factories, zero global state, error
  handling, testing, tooling). All new/changed Go code must comply; `Plan.md` §7 maps each rule
  to the v3 design and lists the open confirmations (A1–A7).

## Tools
go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -ignore "(?:.*_test\.go|.*test.*\.go|frontend)" .
go run github.com/securego/gosec/v2/cmd/gosec@v2.23.0 ./...
go run github.com/gordonklaus/ineffassign@latest ./...
go run golang.org/x/tools/cmd/deadcode@latest -test ./...   # -test: test helpers (clocktest) count as used
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
golangci-lint fmt --config .golangci.yml   # gofumpt + goimports (local prefix github.com/dombyte/solis)
golangci-lint run --config .golangci.yml
go run github.com/vektra/mockery/v2@latest   # regenerate mocks from .mockery.yaml
go run github.com/client9/misspell/cmd/misspell@latest -w . -j 200
go fmt ./...
go vet ./...

---

## Branch Management

- **Never commit or merge directly into `master` or `main` branches.**
- For new features, fixes, or changes, create a new branch following the schema:
  - `{feat,fix,doc,refactor,chore}/{name}`
  - Example: `feat/add-dark-mode`, `fix/cache-leak`, `doc/update-readme`, `feat/all-registers`
- Current working branch: `refactor/v3` (per-phase sub-branches like `refactor/v3-event-bus` are fine)

---

## Project Structure

v3 target layout (packages marked *new* are created during the v3 phases, see `Plan.md`):

```
cmd/                     main.go (subcommand dispatch + restart loop), serve.go, backfill.go
internal/
  app/           *new*   composition root: Create* factories, health adapters, logger wiring
  config/                YAML/env config, structural Validate(); domain rules injected by app
  period/        *new*   Period (day/month/year keys from one captured instant), rollover window
  eventbus/      *new*   event bus (ValuesUpdated, PeriodClosed), non-blocking, per-subscriber policy
  health/        *new*   supervisor, component contract, restart budget, ErrHealthFatal, snapshot
  solis/                 register table (Store enum), computed defs, block planning, decode, derive
  modbus/                Modbus TCP/RTU client, reconnect loop, Source slot for the poller
  poller/                poll loop, DayAttributor (day attribution + rollover), cold-start seeding
  aggregation/   *new*   PURE period math (monthly/yearly/total/net) shared by aggregator + CLI
  aggregator/            event-driven runner (debounce/heartbeat/catch-up) using aggregation
  cache/                 latest values; publishes change events; ReplaceDomain/Merge
  storage/               sole SQLite owner; PollerStore/AggregatorStore/ReadStore; closed periods
  history/       *new*   history read-model rows (daily/monthly/yearly/total/status data points)
  database/              manager: migrations (incl. V3 meta table), backups, retention cleanup
  websocket/             subscription protocol, per-client diff, same-origin upgrader
  service/               ReadService for HTTP (cache + ReadStore + health snapshot)
  http/{handlers,routes,server}/
  maintenance/   *new*   CLI jobs (backfill): flock, backup, recompute, report
  logging/, utils/       zerolog constructor (no global logger); math, Clock, Slot[T]
  <pkg>/mocks/           mockery-generated mocks (never hand-edit)
frontend/                React 19 + Vite + Tailwind 4 + zustand SPA
docs/                    Swagger UI + openapi.yaml
ref/                     v3 spec + dashboard prototype (reference only, not built)
```

---

## Package Responsibilities

### Config Package (`internal/config/`)
- Load configuration from YAML files
- Validate configuration on startup: structural checks in `Validate()`; rules owned by other
  packages (Modbus address, strict `rollover.time` HH:MM, poll timeout vs health grace) are
  `config.Rule`s passed in by `app.ConfigRules()`
- Provide typed config structs (AppConfig, ModbusSettings, …); config imports no domain
  package, and `app` maps sections onto each package's own `Settings`

### HTTP Package (`internal/http/`)

#### Handler Package (`internal/http/handler/`)
- Implement HTTP handlers as `func(http.Handler) http.Handler`
- Handle request/response cycle
- Call service layer for business logic
- Return consistent error responses
- **Do NOT** contain business logic

#### Routes Package (`internal/http/routes/`)
- Define all HTTP routes using Chi router
- Group related routes together
- Centralize route definitions
- Mount sub-routers for API versions

#### Server Package (`internal/http/server/`)
- Initialize and configure HTTP server
- Handle graceful shutdown
- Manage server lifecycle
- Configure middleware chain

### Modbus Package (`internal/modbus/`)
- Modbus client implementation (single TCP or RTU connection, used **only by the poller**);
  transport is selected by the `modbus.address` URL scheme (`tcp://host:port` or
  `rtu://<device path>`)
- Reconnection loop with exponential backoff; beats while waiting so it never looks stale
- Raw Modbus read operations and error classification
- Construction never fails on an unreachable device — it starts `recovering` and reconnects
- Publishes the current client through a `Source` slot so a restart swaps it without touching the poller

### Service Package (`internal/service/`)
- Read-side business logic for HTTP: cache for current values, `ReadStore` for history
- Health snapshot passthrough for `/health`
- **Do NOT** directly handle HTTP requests
- **Do NOT** touch Modbus, the poller or the aggregator (no direct reads, no `PollNow`)

### Solis Package (`internal/solis/`)
- Solis inverter-specific register definitions (single table, `Store` enum, `Net` flag)
- Computed register definitions (daily→monthly/yearly/total maps, net export/import pairs)
- Read-block planning from addressed registers (`PlanBlocks`)
- Decoding (`DecodeRegister`, status/fault maps) and derived values (`DeriveValues`, e.g. `battery_power_signed`)
- everything not Solis specific which is more generic should be in utils
- **Do NOT** contain Modbus client logic (belongs in modbus package)

### Poller Package (`internal/poller/`)
- Non-overlapping poll loop (next poll = interval after previous start)
- Sole writer of daily rows and error/status rows (`PollerStore`)
- Owns day attribution and rollover detection (`DayAttributor`) and emits `PeriodClosed`
- Replaces its own key domain in the cache after each successful poll

### Aggregation Package (`internal/aggregation/`)
- Pure functions only: sums + baseline in → monthly/yearly/total/net values out
- No I/O, no clock, no logging — called identically by the live aggregator and the CLI backfill

### Aggregator Package (`internal/aggregator/`)
- Event-driven: debounce 4× poll interval, heartbeat 5× poll interval, no config section
- Sole writer of computed values (`AggregatorStore`); merges computed keys into the cache
- Idempotent catch-up of missed period closes on every run

### Cache Package (`internal/cache/`)
- Latest values, `lastPollTime`; never recreated at runtime
- Publishes change events on the injected bus on every write
- **Do NOT** import websocket or any consumer — consumers subscribe to the bus

### App Package (`internal/app/`)
- Composition root: the only package that names concrete types and wires them together
- `Create*` factory functions, `health.Component` adapters (e.g. around the Modbus client)
- Builds the root logger and hands each component a logger scoped with `component`

### Eventbus Package (`internal/eventbus/`)
- Created in `internal/app`, injected as `Publisher`/`Subscriber` interfaces; never recreated
- `Publish` never blocks; each subscriber has a buffered channel with an overflow policy

### Health Package (`internal/health/`)
- Supervisor: calls factories, registers, starts, watches and restarts restartable components
- Owns the root context (`context.WithCancelCause`); fatal escalation cancels it with `ErrHealthFatal`
- Publishes the snapshot served by `/health` (503 fail-closed)

### Storage Package (`internal/storage/`)
- The only package that touches SQLite (single connection, WAL)
- Narrow interfaces per consumer: `PollerStore`, `AggregatorStore`, `ReadStore`
- Enforces immutability of closed periods (`ErrPeriodClosed`) and the net-key write guard
- Period sums run as SQL with an explicit bound, never by fetching rows into Go

### WebSocket Package (`internal/websocket/`)
- Subscription protocol (subscribe/unsubscribe → snapshot/update/error), per-client diff
- Same-origin upgrader; ping/pong, stale-client cleanup, full-buffer disconnect

### Maintenance Package (`internal/maintenance/`)
- Out-of-band CLI jobs (`solis backfill --years N`); never starts server, poller or hub
- Exclusive flock next to the DB, verified backup before any write, per-period report

### Utils Package (`internal/utils/`)
- Common utility functions (e.g., error handling, logging helpers)
- Data transformation utilities
- Data type handling (Uint16, Int16, Uint32, Int32, Float32, String, Bool)
- `Clock` interface (inject everywhere a loop waits — tests use a fake clock) and `Slot[T]`
---

## Structure Best Practices

### 1. Single Responsibility Principle
Each package has one clear purpose. Each file has one clear responsibility.

### 2. Dependency Direction
Dependencies flow downward:
```
routes → handlers → services → models
```
HTTP layer depends on service layer, not vice versa.

Background components never reference each other; they interact only through the cache,
the event bus and the health supervisor:
```
poller     → solis, period, eventbus, health(Reporter) + own interfaces (Store, Cache, Reader)
aggregator → aggregation, solis, period, eventbus, health(Reporter) + own Store, Cache
websocket  → eventbus, solis, health(Reporter) + own Snapshotter
cache      → eventbus, solis
storage    → solis, period, history
history    → utils only (shared by storage, service, handlers)
config     → utils only
modbus     → stdlib + simonvetter only (external layer: no config/health/logging-global imports)
service    → own interfaces (ReadStore, CacheReader, HealthSnapshotter)
app        → everything (composition root)
```
Forbidden: poller ↔ aggregator, cache → websocket, handlers → storage/modbus, anything → cmd/app.

Interfaces are declared by the consumer (except the storage interfaces in
`storage/interfaces.go`, as the spec asks) and every interface has a mockery mock.

### 3. No Circular Dependencies
Avoid circular imports between packages. Use interfaces for decoupling.

### 4. Clear API Boundaries
Each package exposes a clean, minimal public API. Internal details stay unexported.

### 5. Avoid Global State
Use dependency injection instead of global variables. Zero tolerance (Arch_Plan): no
package-level loggers, caches, upgraders or lookup maps in new/changed code — build them in a
constructor (e.g. `solis.NewRegistry()`, `solis.NewDecoder()`) and inject them.

### 6. Testable Components
Design packages to be easily testable in isolation. Use interfaces for external dependencies.

### 7. Logging
Loggers are injected (`zerolog.Logger`) and scoped with `Str("component", …)` by `internal/app`.
Log errors with component context and the wrapped error; never log secrets.

---

## What NOT to Do

- ❌ **Don't mix middleware and handlers** in the same package
- ❌ **Don't put business logic** in handler packages (belongs in service layer)
- ❌ **Don't create utility functions** in domain packages (belongs in utils)
- ❌ **Don't duplicate data structures** across packages
- ❌ **Don't use global variables** for configuration, dependencies, loggers or lookup tables
- ❌ **Don't panic** - return errors explicitly
- ❌ **Don't ignore errors** - always handle or return them
- ❌ **Don't read Modbus from the HTTP path** - no `?direct=true`, no `PollNow`; only the poller talks to the inverter
- ❌ **Don't let a component restart another component** - only the health supervisor restarts things
- ❌ **Don't call `time.Now()` mid-run** in poller/aggregator logic - capture once, derive a `period.Period`, inject `utils.Clock`
- ❌ **Don't write outside your write domain** (see below)
- ❌ **Don't add config knobs for health/aggregation constants** (restart budget 3, debounce 4×, heartbeat 5×, reset threshold 10 %) - they are constants until real outage data says otherwise

---

## Component Supervision Contract (v3)

Every restartable component (modbus, poller, aggregator, WebSocket hub) implements:

```go
Start(ctx context.Context) error // start goroutines; return an error only for unrecoverable conditions
Stop() error                      // idempotent graceful shutdown
State() health.State              // Healthy | Recovering | Failed
LastBeat() time.Time              // atomic timestamp, updated by the component's own loop
```

- Constructed only through a `health.Factory` called by the supervisor; the component receives its
  `health.Reporter` in the constructor and pushes state transitions (rare, discrete) through it.
- Beat from the **loop**, not only after work: an idle aggregator or hub must still beat, a
  reconnecting Modbus client beats on every attempt and while waiting in backoff.
- Ordinary failures (poll timeout, read error, connection loss) are handled internally and reported
  as `Recovering`; they never touch the restart counter.
- No `Restart()` method. Restart = supervisor Stop → factory → Start.
- Non-restartable parts (storage, cache, event bus, HTTP server) are `Watch`ed; if they fail the
  supervisor cancels the root context with `ErrHealthFatal` and `main()` restarts the whole app.

## Write Domains (v3)

| Writer | May write | Via |
|---|---|---|
| poller | `daily_values` (non-net keys, max of the open day), `error_data` (on change), poller key domain in cache | `storage.PollerStore`, `cache.ReplaceDomain` |
| aggregator | `monthly_values`, `yearly_values`, `total_values`, net daily rows, baselines/freeze marks in `meta`; computed keys in cache | `storage.AggregatorStore`, `cache.Merge` |
| maintenance (CLI) | computed tables + baselines, only with the app stopped (flock) | `storage` + `aggregation` |
| everyone else | nothing | read-only interfaces |

Closed periods are immutable; storage rejects such writes with `ErrPeriodClosed`. Values are stored
at full precision; rounding to 2 decimals happens only in JSON serialization.

---

## Naming Conventions

### Packages
- Lowercase, singular or compound (Arch_Plan), e.g. `eventbus`, `httphandler`
- Existing plural packages (`handlers/`, `routes/`, `utils/`) keep their names until a rename is
  confirmed (Plan.md A3); do not create new plural package names

### Files
- Lowercase, underscores for multi-word names
- `_test.go` suffix for test files

### Types
- PascalCase for exported types
- camelCase for unexported types

### Functions
- PascalCase for exported functions
- camelCase for unexported functions
- VerbNoun naming (e.g., `ReadRegister`, `DecodeValue`)

### Variables
- camelCase for variables
- PascalCase for exported constants
- `err` prefix for error variables

---

## Build and Test

### Build
```bash
# Build the application
go build -o server ./cmd

# Run the application (server mode)
go run ./cmd

# Maintenance mode (v3): runs a job against the DB and exits 0/1; app must be stopped
go run ./cmd backfill --years 0

# Build with race detector
go build -race -o server ./cmd
```

### Test
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with race detector
go test -race ./...

# Run specific test
go test ./internal/solis -run TestDecoder

# Run with verbose output
go test -v ./...
```

### Quality Checks
```bash
# Format code
go fmt ./...

# Check for formatting issues
gofmt -l .

# Run vet for suspicious constructs
go vet ./...

# Check for dependency vulnerabilities
go vuln ./...
```

---

## Code Style

- `go fmt ./...` before commits
- Follow Go conventions (camelCase, short functions)
- Comments for all public functions and types (Godoc style)
- Functions ideally < 15 lines, max < 40 lines
- Error handling: return errors explicitly, don't panic; wrap with `%w`; sentinel errors +
  custom error types (with `Unwrap`/`Is`) per package; HTTP status via the central error mapper
- Receivers: single letter (`s`, `p`, `c`, `h`); acronyms stdlib-style (`ID`, `URL`, `HTTP`)
- Line length < 100, cyclomatic complexity < 8, no magic numbers (named constants)
- Commits: short imperative subject, **no AI signatures / `Co-Authored-By` trailers**
- Imports grouped: standard library, third-party, project

---

## HTTP Handler Guidelines

### Return Type
Handler functions **must** return `http.Handler` (not `http.HandlerFunc`) for middleware compatibility.

```go
// GOOD
func GetHealthHandler() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // handler logic
    })
}

// ALSO GOOD (direct return)
func GetHealthHandler() http.Handler {
    return http.HandlerFunc(healthHandler)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    // handler logic
}
```

### Dependency Injection
Use `HandlerDeps` struct to pass all dependencies to handlers.

```go
type HandlerDeps struct {
    Config  *config.AppConfig
    Service *service.Service
    Logger  *slog.Logger
}

func NewRegistersHandler(deps HandlerDeps) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // use deps.Service, deps.Config, deps.Logger
    })
}
```

### Middleware Pattern
Implement middleware as functions that return `func(http.Handler) http.Handler`

```go
func LoggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // before
            next.ServeHTTP(w, r)
            // after
        })
    }
}
```

### Route Management
Centralize route definitions in `routes.go`

```go
func SetupRoutes(deps HandlerDeps) *chi.Mux {
    r := chi.NewRouter()
    
    r.Use(middleware.Recoverer)
    r.Use(middleware.Logger)
    
    r.Get("/api/health", handler.GetHealthHandler())
    r.Route("/api/v1", func(r chi.Router) {
        r.Get("/registers", handler.GetRegistersHandler(deps))
    })
    
    return r
}
```

### Error Responses
Use consistent error response format:

```go
type ErrorResponse struct {
    Error   string `json:"error"`
    Message string `json:"message"`
    Code    int    `json:"code"`
}

func WriteError(w http.ResponseWriter, msg string, code int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    json.NewEncoder(w).Encode(ErrorResponse{
        Error:   http.StatusText(code),
        Message: msg,
        Code:    code,
    })
}
```

### Health Endpoint (v3, fail-closed)
- `GET /health` reads the supervisor snapshot only; it never blocks on a component.
- 200 with `ok` or `degraded` (+ details); **503** with the failed component and reason when any
  component is failed/budget-exhausted or a non-restartable part failed. Never report `ok` when a
  subsystem is dead.

### WebSocket Protocol (v3)
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
- Only changed, subscribed keys are pushed (diff by value/status per client); pushes are coalesced
  (~75 ms) so poller + aggregator events close together become one frame.
- Unknown keys never drop the connection. `ping` from the client is accepted and ignored.
- Upgrader is same-origin only. No history over WebSocket — history is REST only.
- New UI-driven registers need no new endpoint/message: define the register in `solis`, clients subscribe.

---

## Service Layer Guidelines

### Service Initialization
Use constructor pattern with dependency injection:

```go
type ReadService struct {
    store  ReadStore         // read-only view; history
    cache  CacheReader       // current values
    health HealthSnapshotter // /health
    log    zerolog.Logger
}

func NewReadService(
    store ReadStore, ca CacheReader, h HealthSnapshotter, log zerolog.Logger,
) *ReadService {
    return &ReadService{store: store, cache: ca, health: h, log: log}
}
```
The service never receives a Modbus client, poller or aggregator (v3).

### Business Logic
Keep handlers thin, put logic in service layer:

```go
// In handler
func GetDataHandler(svc *service.Service) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        data, err := svc.GetAllRegisters(r.Context())
        if err != nil {
            WriteError(w, err.Error(), http.StatusInternalServerError)
            return
        }
        json.NewEncoder(w).Encode(data)
    })
}

// In service
func (s *Service) GetAllRegisters(ctx context.Context) (*solis.SolisData, error) {
    // Business logic here
    // Call modbus client, apply caching, transform data
}
```

---

## Configuration Guidelines

Use Viper for YAML configuration with environment variable overrides:

Env overrides use the `SOLIS_` prefix (e.g. `SOLIS_MODBUS_ADDRESS`). v3 shape (abridged):

```yaml
# config.yaml
app:
  debug: INFO          # DEBUG, INFO, WARN, ERROR, FATAL
  port: 8080
  timeout: 30s

poller:
  interval: 5s         # also drives aggregator debounce (4x), heartbeat (5x), health graces
  block_attempts: 2
  block_retry_delay: 1s
  block_interval: 0s
  poll_timeout: 5s

modbus:
  # Address selects the transport via URL scheme: tcp://host:port or rtu://<device path>
  address: "tcp://192.168.1.100:502"   # or "rtu:///dev/ttyUSB0"
  timeout: 2s
  slave_id: 1
  # speed/data_bits/parity/stop_bits apply to rtu only (defaults: 19200 8N2)
  speed: 9600
  data_bits: 8
  parity: "N"
  stop_bits: 1

rollover:
  time: "23:59"        # strict 24h HH:MM; anything else fails startup

storage:
  path: ./data/solis.db
  daily_retention: 1y  # monthly/yearly rows follow it; only frozen years are ever deleted
  error_retention: 1y  # durations accept d (24h), w (7d), y (365d) on top of s/m/h
  # WAL, backup and cleanup settings unchanged from v2
```

- Removed in v3: `app.serve_only`, the whole `aggregator` section (incl.
  `backfill_current_year_monthly` → use the `backfill` CLI), `storage.monthly_retention`,
  `storage.yearly_retention` (follow `daily_retention`) and `storage.enable_migrations`
  (migrations always run).
- Cross-field rule: `poller.poll_timeout` < 3 × `poller.interval` (health healthy grace).
- Timezone comes from the `TZ` env var (`time.Local`), never from config. Pin `TZ` in
  docker-compose; the image ships zoneinfo.

```go
// In config/models.go
type AppConfig struct {
    Debug bool   `mapstructure:"debug"`
}

type ModbusConfig struct {
    Host    string `mapstructure:"host"`
    Port    int    `mapstructure:"port"`
    Timeout string `mapstructure:"timeout"`
    UnitID  byte   `mapstructure:"unit_id"`
}

// In config/config.go
func LoadConfig(path string) (*AppConfig, error) {
    viper.SetConfigFile(path)
    viper.AutomaticEnv()
    viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
    
    if err := viper.ReadInConfig(); err != nil {
        return nil, fmt.Errorf("failed to read config: %w", err)
    }
    
    var config AppConfig
    if err := viper.Unmarshal(&config); err != nil {
        return nil, fmt.Errorf("failed to unmarshal config: %w", err)
    }
    
    return &config, nil
}
```



---

## Testing

### Unit Tests
- Framework: stdlib `testing` + testify (`require`/`assert`) + mockery mocks
- Create `_test.go` files for each package
- Aim for >90% coverage (Arch_Plan minimums by gocyclo: <5 → 60 %, 5–9 → 70 %, ≥10 → 80 %)
- Test both happy paths and error cases
- Test edge cases (empty inputs, invalid data, etc.)

### Test Isolation
- Avoid global state in tests
- Create fresh instances for each test
- Use `t.Run()` for sub-tests

### Time-Dependent Logic (v3)
- Never sleep on wall-clock time in tests. Inject `utils.Clock` and drive a fake clock for
  debounce/heartbeat, supervisor graces, rollover windows and backoff.
- Rollover/attribution and `period` tests are table-driven and include midnight, DST
  spring-forward and fall-back nights (`time.LoadLocation("Europe/Berlin")`).
- Storage tests must cover closed-period rejection and write-domain guards.

### HTTP Tests
```go
func TestHealthHandler(t *testing.T) {
    handler := handler.GetHealthHandler()
    
    req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
    w := httptest.NewRecorder()
    
    handler.ServeHTTP(w, req)
    
    if w.Code != http.StatusOK {
        t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
    }
}
```

### Mocking Dependencies
Use mockery-generated mocks (`.mockery.yaml`, output in `internal/<pkg>/mocks/`) with testify for
all interfaces. The hand-written example below shows the shape only:
```go
type MockModbusClient struct {
    readRegisterFunc func(address uint16, count uint16) ([]byte, error)
}

func (m *MockModbusClient) ReadRegisters(address, count uint16) ([]byte, error) {
    return m.readRegisterFunc(address, count)
}

func TestService_ReadRegister(t *testing.T) {
    mockClient := &MockModbusClient{
        readRegisterFunc: func(address, count uint16) ([]byte, error) {
            return []byte{0x00, 0x01}, nil
        },
    }
    
    svc := service.NewService(mockClient, nil, nil, nil)
    // test service methods
}
```

---

## Security

### Input Validation
- Validate all inputs from users, config files, network
- Use appropriate types (uint16 for Modbus addresses, etc.)
- Check bounds on all numeric inputs

### Error Handling
- Never expose internal errors to users
- Log errors with context
- Return appropriate HTTP status codes
- Use consistent error messages

### Dependencies
- Keep dependencies updated (`go mod tidy`, `go get -u`)
- Regularly check for vulnerabilities (`go vuln ./...`)
- Prefer well-maintained, popular libraries


---

## Register Development Guidelines

### Adding New Registers
All registers live in one table in `internal/solis/registers.go` (v3 model, spec §4.1):

```go
{
    Key:      "grid_power",
    Name:     "Grid Power",
    Address:  33130,          // 0 = computed/derived, never part of the poll plan
    DataType: solis.Int32,    // register count is derived from the type
    Scale:    1,
    Unit:     "W",
    Store:    solis.StoreNone, // none | daily | monthly | yearly | total | status
},
```

- `Store` decides the destination: `none` = cache only (live values), `status` = `error_data`
  on change, `daily/monthly/yearly/total` = the matching table. `Net: true` marks export−import
  registers (latest-value rule); net registers are always computed.
- Computed registers (monthly/yearly/total/net) have **no address**; add their edge to the
  daily→monthly/yearly/total maps or the net pair list instead.
- Derived live values (e.g. `battery_power_signed`) have no address and are produced in
  `solis.DeriveValues` after a full poll is decoded.
- `solis.Validate()` must pass (unique keys/addresses, valid `Store`, map edges consistent);
  the block-plan golden test must be updated when addresses change.
- Do not add `Stability` or new `IsXxxRegister` lists — switch on `reg.Store`.

### Read Planning
`solis.PlanBlocks` builds read blocks from addressed registers, merging nearby addresses up to
the Modbus limit of 125 registers per read. Prefer an alternate address that falls inside an
existing block over adding a new round-trip (e.g. `grid_power` at 33130 instead of 33263).

### Decoding
```go
value := solis.DecodeRegister(reg, raw) // raw []uint16 for exactly this register
values := solis.DecodeRange(startAddr, blockRaw)
solis.DeriveValues(values, pollStart)
```
Decoded values are full precision; rounding to 2 decimals happens only in JSON serialization
(v3 change: the v2 `RoundTo2DecimalPlaces` call inside `DecodeRegister` is removed).

---

## CLI Maintenance Mode (v3)

- `solis <subcommand>` runs a job against the database and exits 0/1; without a subcommand the
  binary runs the server. Subcommands never start the HTTP server, poller or WebSocket hub.
- `solis backfill --years N` (default 0): recompute monthly + yearly (always both) for the current
  year plus N closed years; refresh the total baseline when a closed year is touched.
- Hard-refuse when the app is running: exclusive `flock` on a lock file next to the DB (the server
  holds a shared lock). Always take a backup first, verified with `PRAGMA integrity_check`
  (`database.CreateBackup`); no backup → no write → exit 1.
- Refuses to recompute periods whose daily rows retention already deleted (meta
  `purged_before`); with any purge, `--years N > 0` fails instead of writing partial sums.
- Uses the same `aggregation` functions as the live aggregator. Output format: spec §12.1.
- Dangerous behavior belongs in CLI jobs, never in config toggles.

---

## Frontend Guidelines (v3)

- Stack: React 19, Vite, Tailwind 4, zustand, lucide-react, chart.js. Checks:
  `npm run typecheck && npm run lint && npm run knip` in `frontend/`.
- Live data comes only via WebSocket subscriptions: components call `useSubscription(keys)`;
  the client ref-counts keys and re-subscribes on reconnect. History stays on REST.
- Register metadata lives in `src/lib/config/data.ts` (single source of truth for the UI).
- Mobile vs desktop: `useMobile()` (coarse pointer). The power-flow diagram uses the mobile
  variant only for coarse pointer **and** width < 768 px; tablets get the desktop variant.
- Power-flow diagram (`src/components/dashboard/flow/`): follow the prototype geometry; icons
  from lucide-react; colors from CSS tokens in `src/index.css` (light + dark), never hardcoded hex;
  edge animation via CSS keyframes (binary on/off, fixed speed, `prefers-reduced-motion` respected),
  not `requestAnimationFrame` + state updates.
- Direction rules: PV → inverter; inverter → household/backup; grid export out / import in;
  battery `battery_power_signed > 0` = charging (into battery). Zero = shown as 0, edge stops;
  missing data = node (or whole diagram) grayed out.

---

## Migration Guidelines

When refactoring existing code:

1. **Create new packages** before moving code
2. **Update imports** in dependent packages
3. **Test thoroughly** after each move
4. **Keep old files** until new structure is verified
5. **Remove old files** only after confirmation
6. **Update documentation** (Plan.md, AGENTS.md)

---

## Performance Pitfalls and Lessons Learned

### 1. Shared Modbus Connection Blocking HTTP Requests

**Problem:**
After integrating SQLite storage and background polling, direct HTTP reads (`?direct=true`) became ~2x slower (600-700ms → 1.5s). The poller and HTTP handlers shared the same Modbus client connection. Since the Solis inverter is sequential-only, when the poller was reading registers, HTTP requests had to wait for the poller to finish before their Modbus reads could execute.

**Root Cause:**
The simonvetter/modbus library serializes requests at the TCP level. With a single shared connection, all requests (poller + HTTP) are queued through the same pipe. If the poller is reading 33 registers (grouped into ranges) taking ~800ms, an HTTP request arriving during that time must wait.

**Solution:**
Create **separate Modbus clients** for the poller and HTTP service in `cmd/main.go`. This allows both to queue requests independently at the device level:
AND NEVER IMPLEMENT DIRECT AGAIN
```go
// Create separate modbus client for the poller
pollerClient, err := createClient(cfg)
if err != nil {
    log.Fatalf("Failed to create poller modbus client: %v", err)
}
defer pollerClient.Close()

// Create poller with its own client
pl = poller.NewPoller(pollerClient, st, scheduler, nil)

// HTTP service uses the original client
readService := service.NewReadService(cfg, client, st, pl)
```

**Lesson:** When dealing with sequential-only devices, separate connections prevent one subsystem from blocking another, even though the device itself processes requests sequentially.

**v3 note:** The HTTP layer no longer has a Modbus client at all (no direct reads, no `PollNow`);
the poller is the only Modbus user, so there is exactly one connection and nothing to block on.
Lessons #1 and #4 are kept as history — the "never implement direct again" rule still applies.

---

### 2. Unnecessary Mutex Overhead in Modbus Wrapper

**Problem:**
The modbus wrapper (`internal/modbus/tcp.go`) used `sync.RWMutex` with `RLock/RUnlock` on every read operation, adding ~1-2μs of lock overhead per register read.

**Root Cause:**
Over-cautious synchronization. The simonvetter/modbus library already handles its own concurrency control at the TCP level (as documented in old commit messages: "simonvetter/modbus handles connection serialization at the TCP level").

**Solution:**
Remove the mutex for normal read operations. Keep mutex only for reconnection logic which modifies handler state (`c.handler`, `c.isConnected`):

```go
// Read path - no mutex needed
rawBytes, err := c.client.ReadInputRegisters(ctx, addr, count)

// Reconnect path - mutex needed
if err != nil {
    c.mu.Lock()
    if reconnectErr := c.reconnect(ctx); reconnectErr != nil {
        c.mu.Unlock()
        return nil, fmt.Errorf("modbus read failed: %w", err)
    }
    c.mu.Unlock()
    // Retry outside mutex
    rawBytes, err = c.client.ReadInputRegisters(ctx, addr, count)
}
```

**Lesson:** Don't add synchronization on top of libraries that already handle it. Profile before adding locks.

---

### 3. Redundant Context Creation

**Problem:**
Both the modbus wrapper and solis reader created new contexts with timeouts for every register read, adding ~50-100ns of allocation overhead per call.

**Root Cause:**
The grid-x handler already has a configured `Timeout` (set in `NewTCP`), so creating a new context per-call was redundant. The handler-level timeout applies to all operations on that handler.

**Solution:**
Use `context.Background()` instead of `context.WithTimeout` for normal reads. The handler's timeout still applies:

```go
// In modbus wrapper
ctx := context.Background()
rawBytes, err := c.client.ReadInputRegisters(ctx, addr, count)

// In solis reader
ctx := context.Background()
rawBytes, err := client.ReadInputRegisters(ctx, reg.Address, reg.Count)
```

**Lesson:** When a library already has timeout configuration, don't duplicate it at the caller level unless you need per-call override capability.

---

### 4. `?direct=true` Flag Not Respected for All Registers

**Problem:**
`GET /api/registers?direct=true` (without `?keys=`) was ignoring the `direct=true` flag and always reading from storage with fallback to individual direct reads instead of batched reads.

**Root Cause:**
`GetAllValues()` hardcoded `forceDirect=false`:
```go
func (s *ReadService) GetAllValues() (map[string]*solis.Value, error) {
    return s.GetValues(s.GetKeys(), false)  // <-- hardcoded false
}
```

**Solution:**
Pass the `forceDirect` parameter through:
```go
// In service.go
func (s *ReadService) GetAllValues(forceDirect bool) (map[string]*solis.Value, error) {
    return s.GetValues(s.GetKeys(), forceDirect)
}

// In handler.go
values, err := h.readService.GetAllValues(forceDirect)
```

**Lesson:** When adding new parameters to handle special cases (like `forceDirect`), ensure all code paths that call the function are updated to pass the parameter through.

---

### 5. Byte-to-Uint16 Conversion Overhead

**Problem:**
Switching from `github.com/grid-x/modbus` to `github.com/simonvetter/modbus` introduced byte-to-uint16 conversion overhead. The old library returned `[]uint16` directly; the new one returns `[]byte`.

**Root Cause:**
Library API change. The simonvetter library was chosen for better maintenance, but its API returns raw bytes requiring manual conversion:
```go
results := make([]uint16, len(rawBytes)/2)
for i := 0; i < len(results); i++ {
    results[i] = uint16(rawBytes[i*2])<<8 | uint16(rawBytes[i*2+1])
}
```

**Solution:**
This overhead is unavoidable (~10-20ns per read) but minimal compared to network latency. Accept as a tradeoff for using a maintained library.

**Lesson:** Library migrations may introduce small performance regressions. Ensure the benefits (maintenance, features, bug fixes) outweigh the costs.

---

### 6. Inefficient GetValues Fetching All Registers

**Problem:**
API endpoint `/api/v1/data/{key}` (without query params) was ~14x slower than `?interval=raw` for the same key. Both should return similar data (latest value vs all history), but no-params was much slower.

**Root Cause:**
`Service.GetValues([]string{key})` → `getAllFromStorage()` → `Storage.GetLatestDynamicValues()` which ran:
```sql
SELECT register_key, raw_value, decoded_value, unit, timestamp 
FROM raw_data 
WHERE register_key IN (all_dynamic_registers)
ORDER BY timestamp DESC
```
This fetched **ALL historical rows** for **ALL dynamic registers** (millions of rows), then Go filtered to keep only the latest per register. For a single key request, this was wasteful.

**Solution:**
Added targeted storage methods:
- `Storage.GetLatestValues(keys []string)` - fetches only latest row per requested key using subquery
- `Storage.GetStableValues(keys []string)` - fetches only requested stable registers

Updated `Service.GetValues()` to:
1. Separate requested keys by stability (stable vs dynamic)
2. Call the appropriate storage method for each group
3. Merge results

New query for dynamic registers:
```sql
SELECT rd.register_key, rd.raw_value, rd.decoded_value, rd.unit, rd.timestamp
FROM raw_data rd
INNER JOIN (
    SELECT register_key, MAX(timestamp) as max_timestamp
    FROM raw_data
    WHERE register_key IN (requested_keys)
    GROUP BY register_key
) latest ON rd.register_key = latest.register_key AND rd.timestamp = latest.max_timestamp
ORDER BY rd.register_key
```

**Performance Impact:**
- Before: Fetched ALL rows for ALL N dynamic registers
- After: Fetched only latest row for requested keys
- Improvement: ~N× faster (14× with 14 dynamic registers)

**Lesson:** Always fetch only the data you need. Filtering in Go after a broad database query is a common anti-pattern that causes performance issues at scale.

---


### Baseline Performance Expectations

| Operation | Expected Latency | Notes |
|-----------|-----------------|-------|
| Direct single register read | 600-700ms | Device-dependent, no blocking |
| Batched register read (n registers) | 600-700ms + (n-1)*~50ms | Grouped by contiguous addresses |
| Storage read (cached) | <1ms | SQLite query |
| HTTP overhead | ~100-200μs | JSON encoding, routing |

---
