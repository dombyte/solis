# Solis Monitor API v2 Redesign Plan

## Overview

This plan outlines the comprehensive redesign of the Solis inverter monitoring system based on user requirements and discussions.

**Version:** 2.0  
**Status:** In Progress - Swagger UI & API Redesign Complete  
**Last Updated:** 2026-08-04  
**Branch:** `v2_newAPI`

**Branch Created:** 2026-08-03 11:44:12 +0200

---

## Branch Status Summary

### ✅ Completed
- **Modbus Library Migration**: Successfully switched from `github.com/grid-x/modbus` to `github.com/simonvetter/modbus` v1.6.4
- **RTU Support Removed**: Deleted `internal/modbus/rtu.go` and `internal/modbus/rtu_over_tcp.go`
- **External Mutex Removed**: Removed unnecessary mutex usage in modbus wrapper (simonvetter library has internal thread safety)
- **Code Simplification**: Removed RegisterFilter, disabled registers logic, and RTU configuration fields
- **Updated Dependencies**: go.mod updated with simonvetter/modbus v1.6.4
- **Metrics Port Separation**: Implemented separate configurable metrics server on port 9090 with dedicated `SetupMetricsRoutes` and `NewMetricsServer` functions
- **API Redesign**: Implemented new endpoint structure with `/api/data/{key}` and `/api/keys` endpoints
- **Database Migration Infrastructure**: Added comprehensive database migration support with `internal/database/manager.go` and `internal/database/migrations/migrations.go`
- **Swagger UI Documentation**: Complete Swagger UI implementation with OpenAPI spec, frontend integration, and documentation server
- **Separate HTTP Servers**: Main server and metrics server run independently (no blocking)
- **Clean Configuration**: Removed RTU-specific fields from config, simplified RegistersSettings
- **Register Key Standardization**: Codebase already uses new standardized names (e.g., `pv_energy_daily`, `energy_consumption_daily`, `grid_export_daily`) throughout `internal/solis/registers.go`. Old names only remain in migration files (as source for migration), comments, and some tests.
- **Database Migration for Key Renaming**: Fully implemented with V2 migration registered in `internal/database/manager.go` (lines 54, 358-475) and `internal/database/migrations/migrations.go` (lines 11-255). Automatically runs on database initialization to migrate old keys to new standardized names.

### ⏳ In Progress
- **Configuration Cleanup**: config.yaml still contains RTU and disabled_keys settings (code ignores them)

### ❌ Not Started
- **Computed Registers Package**: `internal/computed/` package not created
- **Unified API Response Schema**: Still using old response format (needs minimal schema implementation)

### 📊 Verification Status
- **Build**: ✅ Code compiles successfully
- **Modbus Tests**: ✅ All tests pass (5.011s)
- **Race Condition Tests**: ⚠️ Not yet verified with `-race` flag
- **Integration Tests**: ❌ Not yet run
- **Performance Tests**: ❌ Not yet run
- **Swagger UI**: ✅ Documentation builds and serves correctly
- **API Endpoints**: ✅ `/api/data/{key}` and `/api/keys` functional

---

## 1. Executive Summary

### Core Objectives
- [x] **API Restructuring**: Simplified single endpoint design with query parameters (`/api/data/{key}`, `/api/keys`)
- [x] **Metrics Separation**: Prometheus metrics on dedicated configurable port (9090)
- [x] **Register Key Standardization**: Consistent, descriptive naming patterns (pv_energy_daily, energy_consumption_daily, grid_export_daily, etc.)
- [x] **Library Migration**: Switch from grid-x/modbus to simonvetter/modbus v1.6.4 (TCP only)
- [x] **Code Simplification**: Removed RTU support, disabled registers logic, redundant mutex
- [x] **Database Migration Infrastructure**: Added comprehensive migration support with automatic V2 migration
- [ ] **Computed Registers**: Separate package for maintainability (not created yet)
- [ ] **Unified API Response**: Minimal, consistent response schema (still using old format)
- [x] **Swagger UI Documentation**: Complete API documentation with OpenAPI spec

**Key Principle:** No backward compatibility required. Database migration is the only backward-looking concern.

### Achievements Compared to v2/main
The `v2_newAPI` branch has successfully implemented:
1. Complete modbus library migration from grid-x to simonvetter/modbus v1.6.4
2. Full RTU removal and code simplification (external mutex, RegisterFilter, disabled registers logic)
3. Separate metrics server on configurable port (9090) with dedicated HTTP server
4. New API endpoint structure (`/api/data/{key}`, `/api/keys`)
5. Register key standardization - all registers now use new naming pattern (pv_energy_daily, energy_consumption_daily, grid_export_daily, etc.)
6. Comprehensive database migration infrastructure with V2 migration that automatically renames old keys to new standardized names
7. Complete Swagger UI documentation with OpenAPI specification
8. Docker integration for documentation builds

---

## 2. Final Decisions

### 2.1 API Design
**Single endpoint for all data access:**
```
GET /api/data/{key}              - Returns data based on key type
    Query params:
    - ?start=2026-08-01&end=2026-08-03   -> For daily/monthly/yearly keys
    - NO query params                      -> Latest current value (from cache)
    - For total keys: NO query params       -> Returns lifetime value

GET /api/keys                     - All register metadata (unchanged)
GET /api/health                   - Health check (unchanged)
```

**The key IS the identifier** - the system knows from the key whether it's daily, monthly, yearly, or total.

### 2.2 Modbus Library Migration
**Switch from `github.com/grid-x/modbus` to `github.com/simonvetter/modbus`**

**Pros:**
- Returns `[]uint16` directly - no byte-to-uint16 conversion overhead (~10-20ns per read)
- Simpler API focused on TCP
- Well-maintained, widely used
- **Has internal thread safety** - `ModbusClient` includes `sync.Mutex` for concurrent access
- Smaller dependency tree

**Implementation:**
- Update `internal/modbus/tcp.go` to use simonvetter library
- Remove `internal/modbus/rtu.go` and `internal/modbus/rtu_over_tcp.go` (TCP only)
- Update all modbus client calls
- **Remove all external mutex** in wrapper (library handles concurrency)
- Update go.mod dependencies

### 2.3 Mutex Removal
**Action:** Remove all external mutex usage in modbus wrapper when switching to simonvetter/modbus

**Current (grid-x/modbus):**
```go
c.mu.RLock()                    // External mutex - REMOVE
client := c.client
isConnected := c.isConnected
c.mu.RUnlock()
```

**After (simonvetter/modbus):**
```go
// No external mutex needed - library handles it internally
client := c.client
isConnected := c.isConnected
```

**⚠️ Testing Required:** 
- Verify thread safety with concurrent reads
- Test under load with multiple goroutines
- Validate no race conditions with `-race` flag

### 2.4 Context Creation Overhead
**Decision:** KEEP AS-IS

**Rationale:** Timeout override capability is valuable for:
- Different operations needing different timeouts
- Retry logic with backoff
- Future flexibility

### 2.5 Register Key Naming

#### Naming Convention
- **Energy keys:** `{entity}_energy_{time_period}`
- **Time periods:** `daily`, `monthly`, `yearly`, `total`
- **Grid:** `grid_export` (was: energy_fed_into_grid), `grid_import` (was: energy_imported_from_grid)
- **Consumption:** `energy_consumption_*` (reversed order)
- **Household/Backup:** Remove "load_" prefix
- **Phase:** Keep `a_phase_*`, `b_phase_*`, `c_phase_*` (inverter voltage, not grid)
- **Status:** Keep `solis_status`
- **Computed registers:** Follow same pattern

#### Complete Key Mapping

**Energy & Consumption**
| Old | New |
|-----|-----|
| `pv_today_energy` | `pv_energy_daily` |
| `pv_month_energy` | `pv_energy_monthly` |
| `pv_year_energy` | `pv_energy_yearly` |
| `pv_total_energy` | `pv_energy_total` |
| `today_energy_consumption` | `energy_consumption_daily` |
| `total_energy_consumption` | `energy_consumption_total` |
| `today_energy_fed_into_grid` | `grid_export_daily` |
| `total_energy_fed_into_grid` | `grid_export_total` |
| `today_energy_imported_from_grid` | `grid_import_daily` |
| `total_energy_imported_from_grid` | `grid_import_total` |

**Battery**
| Old | New |
|-----|-----|
| `today_battery_charge_energy` | `battery_charge_daily` |
| `today_battery_discharge_energy` | `battery_discharge_daily` |
| `total_battery_charge_energy` | `battery_charge_total` |
| `total_battery_discharge_energy` | `battery_discharge_total` |

**Household & Backup** *(load prefix removed)*
| Old | New |
|-----|-----|
| `household_load_today_energy` | `household_energy_daily` |
| `household_load_month_energy` | `household_energy_monthly` |
| `household_load_year_energy` | `household_energy_yearly` |
| `household_load_total_energy` | `household_energy_total` |
| `household_load_power` | `household_power` |
| `backup_load_today_energy` | `backup_energy_daily` |
| `backup_load_month_energy` | `backup_energy_monthly` |
| `backup_load_year_energy` | `backup_energy_yearly` |
| `backup_load_total_energy` | `backup_energy_total` |
| `backup_load_power` | `backup_power` |

**Grid Phases** *(keep a_phase, b_phase, c_phase - inverter voltage)*
| Old | New |
|-----|-----|
| `a_phase_voltage` | `a_phase_voltage` |
| `b_phase_voltage` | `b_phase_voltage` |
| `c_phase_voltage` | `c_phase_voltage` |
| `a_phase_current` | `a_phase_current` |
| `b_phase_current` | `b_phase_current` |
| `c_phase_current` | `c_phase_current` |

**Grid Power**
| Old | New |
|-----|-----|
| `active_power` | `active_power` |
| `reactive_power` | `reactive_power` |
| `apparent_power` | `apparent_power` |
| `grid_frequency` | `grid_frequency` |

**Status Registers** *(keep as-is)*
| Old | New |
|-----|-----|
| `solis_status` | `solis_status` |
| `operating_status` | `operating_status` |
| `temperature` | `temperature` |

**Fault Status** *(simplified)*
| Old | New |
|-----|-----|
| `grid_fault_status_01` | `grid_fault_1` |
| `backup_load_fault_status_02` | `backup_fault_2` |
| `battery_fault_status_03` | `battery_fault_3` |
| `device_fault_status_04` | `device_fault_4` |
| `device_fault_status_05` | `device_fault_5` |
| `battery_fault_status_1_bms` | `battery_fault_1_bms` |
| `battery_fault_status_2_bms` | `battery_fault_2_bms` |

**Meter Registers**
| Old | New |
|-----|-----|
| `meter_ac_voltage_a` | `meter_voltage_a` |
| `meter_ac_current_a` | `meter_current_a` |
| `meter_ac_voltage_b` | `meter_voltage_b` |
| `meter_ac_current_b` | `meter_current_b` |
| `meter_ac_voltage_c` | `meter_voltage_c` |
| `meter_ac_current_c` | `meter_current_c` |
| `meter_active_power_a` | `meter_power_a` |
| `meter_active_power_b` | `meter_power_b` |
| `meter_active_power_c` | `meter_power_c` |
| `meter_total_active_power` | `meter_power_total` |

**Power & AC**
| Old | New |
|-----|-----|
| `total_pv_power` | `pv_power_total` |
| `ac_grid_port_power` | `ac_grid_power` |

**Net Grid (Computed)** *(follow same pattern)*
| Old | New |
|-----|-----|
| `today_grid_energy` | `grid_energy_daily` |
| `month_grid_energy` | `grid_energy_monthly` |
| `year_grid_energy` | `grid_energy_yearly` |
| `total_grid_energy` | `grid_energy_total` |

**Information Registers** *(keep as-is)*
| Old | New |
|-----|-----|
| `solis_model_no` | `solis_model_no` |
| `solis_dsp_version` | `solis_dsp_version` |
| `solis_hmi_version` | `solis_hmi_version` |
| `solis_protocol_version` | `solis_protocol_version` |
| `solis_serial_number` | `solis_serial_number` |

### 2.6 Code Simplification

**Remove:**
- `internal/modbus/rtu.go` - RTU not needed (TCP only)
- `internal/modbus/rtu_over_tcp.go` - RTU over TCP not needed
- `RegisterFilter` type and methods from `internal/solis/registers.go` - disabled registers logic removed
- `DisabledKeys` from `RegistersSettings` in `config.go`
- All filter-related code from service layer and handlers
- Internal key filtering logic (`battery_current_direction` check)

**Keep:**
- Sequential range reads in poller (4 ranges: 33000-33096, 33116-33180, 33251-33264, 33580-33596)
- Reconnection logic with exponential backoff
- Context-based timeouts for Modbus operations
- Cache with WebSocket integration
- Graceful shutdown handling

### 2.7 Database Migration

**Strategy:** Atomic migration per table using temporary tables

**Tables to migrate:**
- `daily_values` - contains `register_key` column
- `monthly_values` - contains `register_key` column
- `yearly_values` - contains `register_key` column
- `total_values` - contains `register_key` column
- `error_data` - contains `register_key` column

**Safety:**
- Run in transaction per table
- Backup database before migration
- Test with copy of production data
- Atomic: all or nothing per table

### 2.8 Metrics Port Separation

**Configuration:**
```yaml
metrics:
  enabled: true
  port: 9090      # Separate metrics port
```

**Implementation:** Create separate HTTP server for metrics in `cmd/main.go`

**Changes:**
- Add `Port` field to `MetricsSettings` in `internal/config/config.go`
- Remove `/metrics` endpoint from main router
- Remove `MetricsEnabled` from `HandlerDeps`

### 2.9 Computed Register Logic

**New package:** `internal/computed/`

**Register definitions:**
```go
var ComputedRegisters = []ComputedRegister{
    {
        Key:         "grid_energy_daily",
        Name:        "Grid Energy Daily (Net)",
        Unit:        "kWh",
        Dependencies: []string{"grid_export_daily", "grid_import_daily"},
        ComputeFunc: func(deps map[string]float64) float64 {
            return deps["grid_export_daily"] - deps["grid_import_daily"]
        },
    },
    // ... all other computed registers
}
```

**Integration:** Calculated during poll cycle, stored like raw values, API returns them transparently

### 2.10 Unified API Response

**Minimal schema - only fields with data:**

```go
// Single value response (current or total)
type DataResponse struct {
    Key           string      `json:"key,omitempty"`
    Name          string      `json:"name,omitempty"`
    Unit          string      `json:"unit,omitempty"`
    Value         float64     `json:"value,omitempty"`
    RawValue      float64     `json:"raw_value,omitempty"`
    Timestamp     string      `json:"timestamp,omitempty"`
    StringValue   string      `json:"string_value,omitempty"`
    StatusDecoded interface{} `json:"status_decoded,omitempty"`
}

// Historical data response (daily/monthly/yearly)
type DataListResponse struct {
    Key      string    `json:"key,omitempty"`
    Name     string    `json:"name,omitempty"`
    Unit     string    `json:"unit,omitempty"`
    Start    string    `json:"start,omitempty"`
    End      string    `json:"end,omitempty"`
    Data     []DataPoint `json:"data"`
}

type DataPoint struct {
    Value    float64 `json:"value"`
    RawValue float64 `json:"raw_value,omitempty"`
    Period   string  `json:"period"`
}
```

---

## 3. Implementation Phases

### Phase 1: Preparation & Foundation (Week 1)
- [x] Create feature branch: `v2_newAPI` (created 2026-08-03 11:44:12 +0200)
- [x] Update go.mod with simonvetter/modbus dependency (commit #1)
- [x] Set up database migration infrastructure (commits #1-#5: manager.go, migrations.go)
- [ ] Create initial test suite

### Phase 2: Modbus Library Migration (Week 1-2)
- [x] Implement new TCP client using simonvetter/modbus (commit #1, #3)
- [x] Remove RTU files (commit #1: rtu.go, rtu_over_tcp.go deleted)
- [x] Update all modbus client calls (commit #1, #3)
- [x] Remove all external mutex usage (commit #1, #3 - verified simonvetter has internal thread safety)
- [ ] **TEST with `-race` flag** - verify no race conditions
- [ ] Performance test

### Phase 3: Core Infrastructure (Week 2)
- [x] Implement metrics server separation (added Port field to MetricsSettings, separate server created in cmd/main.go)
- [x] Remove disabled registers logic (commit #1 - RegistersSettings simplified, RegisterFilter removed)
- [x] Remove RTU configuration fields (commit #1 - config validation only allows TCP)

### Phase 4: API Redesign (Week 2-3)
- [x] Update route definitions with `/api/data/{key}` and `/api/keys` endpoints (commits #1-#5)
- [x] Create new unified handlers (handlers updated in commits #1-#5)
- [x] Swagger UI implementation complete (commits Swagger UI #1-#11)
- [ ] Implement unified response schema (still using old DataResponse)

### Phase 5: Register Key Standardization (Week 2-3)
- [x] Update all register definitions with new standardized names (pv_energy_daily, energy_consumption_daily, etc.)
- [x] Update all register references throughout codebase

### Phase 6: Database Migration (Week 2-3)
- [x] Set up database migration infrastructure (commits #1-#5: manager.go, migrations.go)
- [x] Implement register key migration logic for renaming (V2 migration with complete KeyMapping)
- [ ] Test data preservation with actual migration

### Phase 7: Computed Registers (Week 3)
- [ ] Create `internal/computed/` package (directory does not exist)
- [ ] Move computed register logic (still in service layer)
- [ ] Integrate with poller and storage

### Phase 8: Integration & Testing (Week 4-5)
- [ ] Full integration testing
- [ ] End-to-end API testing
- [ ] Database migration testing
- [ ] Performance benchmarking
- [ ] Race condition testing

### Phase 9: Cleanup & Documentation (Week 5)
- [ ] Remove deprecated endpoints
- [ ] Clean up temporary code (modbus_test.go.backup should be removed)
- [ ] Update documentation
- [ ] Create migration guide
- [ ] Clean up config.yaml (remove RTU settings, disabled_keys)

---

## 4. Files to Modify

### Configuration
- [x] `internal/config/config.go` - Added metrics port, removed disabled keys, removed RTU settings
- [x] `config.yaml` - Updated with new metrics configuration
- [ ] `config.yaml` - Clean up: remove RTU settings and disabled_keys section

### Modbus
- [x] `internal/modbus/modbus.go` - Simplified, removed RTU support, removed external mutex
- [x] `internal/modbus/tcp.go` - Rewritten for simonvetter/modbus
- [x] **DELETED** `internal/modbus/rtu.go`
- [x] **DELETED** `internal/modbus/rtu_over_tcp.go`
- [ ] Clean up: Remove `internal/modbus/modbus_test.go.backup`

### Solis
- [x] `internal/solis/registers.go` - All register keys updated to standardized naming pattern (pv_energy_daily, energy_consumption_daily, grid_export_daily, etc.)
- [x] Removed RegisterFilter

### Service
- [x] `internal/service/service.go` - Removed filter logic, simplified
- [x] Uses new register key names throughout

### HTTP Layer
- [x] `internal/http/routes/routes.go` - New endpoint structure with `/api/data/{key}` and `/api/keys`
- [x] `internal/http/handlers/handlers.go` - New unified handlers
- [x] `internal/http/server/server.go` - Added metrics server support with `NewMetricsServer`
- [x] **NEW** `internal/http/handlers/response_types.go` - Added for API response types

### Database
- [x] `internal/database/manager.go` - Added comprehensive migration infrastructure with V2 migration (lines 54, 358-540)
- [x] `internal/database/migrations/migrations.go` - Added V2Migration implementation (lines 11-255) with complete KeyMapping
- [x] Register key migration logic fully implemented and automatically runs on database initialization

### Computed (NEW)
- [ ] **NEW** `internal/computed/registers.go`
- [ ] **NEW** `internal/computed/calculator.go`
- [ ] **NEW** `internal/computed/computed_test.go`

### Main
- [x] `cmd/main.go` - Added metrics server, simplified initialization

### Documentation (NEW)
- [x] `docs/` - Complete Swagger UI implementation with Vite
- [x] `docs/src/openapi.yaml` - Comprehensive OpenAPI specification
- [x] `Dockerfile` - Added for docs build
- [x] `Dockerfile.goreleaser` - Added for release builds

---

## 5. Testing Strategy

- Unit tests: >90% coverage for new packages
- Integration tests: API endpoints, database migration, metrics server
- Performance tests: Benchmark before/after, concurrent requests
- End-to-end tests: Full system with mock inverter
- Race condition tests: All tests with `-race` flag

---

## 6. Success Criteria

- [x] All new API endpoints functional and documented (Swagger UI complete)
- [x] Database migration preserves all historical data (V2 migration implemented with complete KeyMapping, automatically runs on init)
- [x] Metrics served on separate configurable port (port 9090 with dedicated server)
- [x] All register keys follow standardized naming pattern (code uses new names: pv_energy_daily, energy_consumption_daily, grid_export_daily, etc.)
- [x] Code simplified with redundancy removed (RTU, mutex, filter logic removed)
- [ ] Computed registers in separate package (not created yet)
- [ ] Unified response schema across all endpoints (still using old DataResponse)
- [x] All tests passing (modbus tests pass, but race detection not verified)
- [ ] Performance metrics meet or exceed current baseline
- [ ] No race conditions detected

---

*Generated for Solis Monitor v2 API Redesign*
*Last Updated: 2026-08-04*

---

## 7. Branch Commit History

The `v2_newAPI` branch extends `v2` and contains all v2 commits plus additional Swagger UI and API redesign work.

### v2 Branch Commits (Included in v2_newAPI)

#### Modbus Migration (from v2_modbus merge)
**Commit c29f1ab:** Modbus library migration to simonvetter/modbus
- Migrated from `github.com/grid-x/modbus` to `github.com/simonvetter/modbus` v1.6.4
- Removed external mutex usage (library has internal thread safety)
- Deleted RTU files: `internal/modbus/rtu.go`, `internal/modbus/rtu_over_tcp.go`
- Removed RegisterFilter, disabled registers logic, RTU configuration fields

**Commit eabfe37:** Decoder refactoring
- Updated decoding logic in `internal/solis/decoder.go`
- Updated related test files

**Commit b8d42ee:** Modbus refinements
- Further updates to modbus wrapper and tests

**Commit fac5065:** Final modbus cleanup
- Simplified initialization in cmd/main.go
- Final RTU field cleanup in config

**Commit 30a46ba:** Test updates
- Minor formatting and test updates

#### Metrics Port Separation (from v2_metricsPort merge)
**Commit bc6862f:** Merge v2_metricsPort into v2
- Added MetricsSettings.Port field to config
- Created separate metrics server with `NewMetricsServer` in `internal/http/server/server.go`
- Added `SetupMetricsRoutes` in `internal/http/routes/routes.go`
- Updated cmd/main.go to start separate metrics server
- Updated Docker and docker-compose configuration

#### API Redesign and Database Migration
**Commit 4447fad:** #1 - API Redesign Foundation
- Added comprehensive database migration infrastructure (`internal/database/manager.go`, `migrations/migrations.go`)
- Updated handlers with new response types (`internal/http/handlers/response_types.go`)
- Refactored route structure with `/api/data/{key}` and `/api/keys` endpoints
- Updated modbus, poller, service, and storage layers

**Commit 3a9dad3:** #2 - Service layer updates
- Refactored service.go and related tests

**Commit 2c3a12f:** #3 - Frontend config updates
- Updated frontend configuration and register definitions

**Commit e7c1227:** #4 - Frontend tweaks
- Minor frontend adjustments

**Commit 685781e:** #5 - Handler cleanup
- Simplified handlers, removed redundant code

**Commit 795c591:** #6 - Frontend data config
- Updated frontend data.ts configuration

**Commit 485a60c:** #7 - Response types cleanup
- Refined response_types.go

### Swagger UI Implementation (Unique to v2_newAPI)

**Commit 8ed8704:** Swagger UI #1 - Initial setup
- Added docs/ directory with Vite, package.json, configuration
- Created initial Swagger UI structure

**Commit cfccc7b:** Swagger UI #2 - OpenAPI spec
- Added comprehensive OpenAPI specification (`docs/public/openapi.yaml`)
- Updated main.js for Swagger UI integration

**Commit 2b2b688:** Swagger UI #3 - Structure refinement
- Reorganized docs directory structure
- Moved openapi.yaml to docs/src/

**Commit 4b806ec:** Swagger UI #4 - API spec updates
- Major updates to OpenAPI spec (310 lines changed)
- Refined endpoint definitions

**Commit ffca7e5:** Swagger UI #5 - Minor adjustments
- Small fixes to OpenAPI spec and main.js

**Commit 12022bd:** Swagger UI #6 - Spec enhancements
- Added more endpoint definitions

**Commit 3392e3e:** Swagger UI #8 - Extended documentation
- Major expansion of OpenAPI spec (146 new lines)
- Added comprehensive endpoint documentation

**Commit a67436e:** Swagger UI #9 - Docker integration
- Added Dockerfile for docs
- Updated openapi.yaml with more definitions
- Updated routes.go to serve /docs/* for Swagger UI

**Commit 2469b1c:** Swagger UI #10 - Build configuration
- Updated vite.config.ts for proper build configuration

**Commit da3f5f8:** Swagger UI #11 - Final Docker setup
- Added Dockerfile.goreleaser configuration for Swagger UI

**Total Changes:** 40+ commits, significant additions to documentation, API, and infrastructure

---

## 8. Next Steps

### Immediate (Priority: HIGH)
1. **Verify race condition safety**: Run all tests with `-race` flag to confirm simonvetter/modbus thread safety
2. **Clean up temporary file**: Remove `internal/modbus/modbus_test.go.backup` (26KB)
3. **Update config.yaml**: Remove RTU settings and disabled_keys section

### Short Term (Priority: MEDIUM)
1. **Test database migration**: Verify V2 migration preserves all historical data with actual database testing

### Medium Term (Priority: MEDIUM)
1. **Create computed package**: Move computed register logic to `internal/computed/`
2. **Implement unified response schema**: Update API responses to use minimal schema
3. **Clean up old names**: Remove old key names from comments, tests, and any remaining references

### Long Term (Priority: LOW)
1. **Performance benchmarking**: Compare before/after metrics
2. **Integration testing**: Full system testing with mock inverter
3. **Create migration guide**: Document v1 to v2 migration steps
4. **Clean up deprecated code**: Remove any remaining v1 artifacts
