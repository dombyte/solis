# Agent Documentation


## Project Overview


## Tools
go run github.com/fzipp/gocyclo/cmd/gocyclo@latest -ignore "(?:.*_test\.go|.*test.*\.go)" ./..
go run github.com/securego/gosec/v2/cmd/gosec@v2.23.0 ./...
go run github.com/gordonklaus/ineffassign@latest ./...
go run golang.org/x/tools/cmd/deadcode@latest ./...
go run github.com/client9/misspell/cmd/misspell@latest -w . -j 200
go fmt ./...
go vet ./...

---

## Branch Management

- **Never commit or merge directly into `master` or `main` branches.**
- For new features, fixes, or changes, create a new branch following the schema:
  - `{feat,fix,doc,refactor,chore}/{name}`
  - Example: `feat/add-dark-mode`, `fix/cache-leak`, `doc/update-readme`, `feat/all-registers`
- Current working branch: `fix/performance`

---

## Project Structure



---

## Package Responsibilities

### Config Package (`internal/config/`)
- Load configuration from YAML files
- Validate configuration on startup
- Provide typed config structs (AppConfig, ModbusConfig, ServerConfig)

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
- Modbus client implementation
- Connection pooling and management
- Raw Modbus read operations
- Error handling for Modbus protocol errors

### Service Package (`internal/service/`)
- Business logic orchestration
- Coordinate between Modbus client and other services
- Data transformation and aggregation
- **Do NOT** directly handle HTTP requests

### Solis Package (`internal/solis/`)
- Solis inverter-specific register definitions
- Register reading and decoding logic
- Register map management and lookup
- Solis-specific decoding patterns
- everything not Solis specific which is more generic should be in utils
- **Do NOT** contain Modbus client logic (belongs in modbus package)

### Utils Package (`internal/utils/`)
- Common utility functions (e.g., error handling, logging helpers)
- Data transformation utilities
- Data type handling (Uint16, Int16, Uint32, Int32, Float32, String, Bool)
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

### 3. No Circular Dependencies
Avoid circular imports between packages. Use interfaces for decoupling.

### 4. Clear API Boundaries
Each package exposes a clean, minimal public API. Internal details stay unexported.

### 5. Avoid Global State
Use dependency injection instead of global variables.

### 6. Testable Components
Design packages to be easily testable in isolation. Use interfaces for external dependencies.

---

## What NOT to Do

- ❌ **Don't mix middleware and handlers** in the same package
- ❌ **Don't put business logic** in handler packages (belongs in service layer)
- ❌ **Don't create utility functions** in domain packages (belongs in utils)
- ❌ **Don't duplicate data structures** across packages
- ❌ **Don't use global variables** for configuration or dependencies
- ❌ **Don't panic** - return errors explicitly
- ❌ **Don't ignore errors** - always handle or return them

---

## Naming Conventions

### Packages
- Lowercase, single word or hyphen-separated
- Plural for collections (e.g., `handlers/`, `routes/`)
- Singular for types (e.g., `config/`, `service/`)

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

# Run the application
go run ./cmd

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
- Error handling: return errors explicitly, don't panic
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

---

## Service Layer Guidelines

### Service Initialization
Use constructor pattern with dependency injection:

```go
type Service struct {
    modbusClient *modbus.TCPClient
    config       *config.AppConfig
    logger       *slog.Logger
}

func NewService(
    modbusClient *modbus.TCPClient,
    config *config.AppConfig,
    logger *slog.Logger,
) *Service {
    return &Service{
        modbusClient: modbusClient,
        config:       config,
        logger:       logger,
    }
}
```

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

```yaml
# config.yaml
app:
  debug: true

modbus:
  host: 192.168.1.100
  port: 502
  timeout: 5s
  unit_id: 1

server:
  host: 0.0.0.0
  port: 8080
  timeout: 30s
```

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
- Create `_test.go` files for each package
- Aim for >90% coverage
- Test both happy paths and error cases
- Test edge cases (empty inputs, invalid data, etc.)

### Test Isolation
- Avoid global state in tests
- Create fresh instances for each test
- Use `t.Run()` for sub-tests

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
Use the `RegisterBuilder` fluent API:

```go
var BatteryVoltage = register.NewBuilder("battery_voltage", 0x1000).
    WithName("Battery Voltage").
    WithUnit("V").
    WithDataType(register.Float32).
    WithScale(0.1).
    WithAccess(register.ReadOnly).
    Build()
```

### Register Map
All registers should be added to the `RegisterMap` for easy lookup:

```go
var RegisterMap = register.NewMap(
    BatteryVoltage,
    BatteryCurrent,
    // ... all other registers
)
```

### Decoding
Use type-safe decoding methods:

```go
value, err := decoder.DecodeRegisterWithType(raw, reg.DataType)
// or
floatVal := decoder.GetFloat64(value, reg.Scale)
```

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
The grid-x/modbus library serializes requests at the TCP level. With a single shared connection, all requests (poller + HTTP) are queued through the same pipe. If the poller is reading 33 registers (grouped into ranges) taking ~800ms, an HTTP request arriving during that time must wait.

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

---

### 2. Unnecessary Mutex Overhead in Modbus Wrapper

**Problem:**
The modbus wrapper (`internal/modbus/tcp.go`) used `sync.RWMutex` with `RLock/RUnlock` on every read operation, adding ~1-2μs of lock overhead per register read.

**Root Cause:**
Over-cautious synchronization. The grid-x/modbus library already handles its own concurrency control at the TCP level (as documented in old commit messages: "grid-x/modbus handles connection serialization at the TCP level").

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
Switching from `github.com/simonvetter/modbus` to `github.com/grid-x/modbus` introduced byte-to-uint16 conversion overhead. The old library returned `[]uint16` directly; the new one returns `[]byte`.

**Root Cause:**
Library API change. The grid-x library was chosen for better maintenance, but its API returns raw bytes requiring manual conversion:
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
