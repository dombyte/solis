package solis

import (
	"errors"
	"fmt"
	"sort"

	"github.com/dombyte/solis/internal/period"
)

// Read-planning limits.
const (
	// MaxBlockLen is the Modbus limit of registers per read.
	MaxBlockLen = 125
	// MaxBlockGap is the largest run of unused registers bridged inside one block;
	// reading a few unused words is cheaper than an extra round-trip.
	MaxBlockGap = 32
)

// ErrInvalidRegistry wraps every register table validation failure.
var ErrInvalidRegistry = errors.New("invalid register table")

// RegistryError reports the register (or map edge target) that failed validation.
type RegistryError struct {
	// Key is the offending register key.
	Key string
	// Reason explains the failure.
	Reason string
}

func (e *RegistryError) Error() string {
	return fmt.Sprintf("%v: %s", ErrInvalidRegistry, e.Reason)
}

// Unwrap returns ErrInvalidRegistry.
func (e *RegistryError) Unwrap() error { return ErrInvalidRegistry }

// Registry is the immutable, validated register table with its lookup indexes,
// computed-value definitions and read plan.
type Registry struct {
	regs   []Register
	byKey  map[string]Register
	byAddr map[uint16]Register
	edges  []Edge
	nets   []NetPair
	blocks []Block
}

// NewRegistry builds and validates the v3 register table. An error here is a
// programming error and must abort startup.
func NewRegistry() (*Registry, error) {
	energy, edges := energyRegisters()
	net, pairs := netRegisters()
	regs := append(append(append(energy, net...), statusRegisters()...), liveRegisters()...)
	return newRegistry(regs, edges, pairs)
}

func newRegistry(regs []Register, edges []Edge, nets []NetPair) (*Registry, error) {
	r := &Registry{
		regs:   regs,
		byKey:  make(map[string]Register, len(regs)),
		byAddr: make(map[uint16]Register),
		edges:  edges,
		nets:   nets,
	}
	if err := r.index(); err != nil {
		return nil, err
	}
	if err := r.validateComputed(); err != nil {
		return nil, err
	}
	if err := r.validateTotalSources(); err != nil {
		return nil, err
	}
	r.blocks = PlanBlocks(r.Polled(), MaxBlockGap, MaxBlockLen)
	return r, nil
}

func invalid(key, format string, args ...any) error {
	return &RegistryError{Key: key, Reason: fmt.Sprintf(format, args...)}
}

// index fills the lookup maps and checks per-register invariants.
func (r *Registry) index() error {
	for _, reg := range r.regs {
		if err := validateRegister(reg); err != nil {
			return err
		}
		if _, dup := r.byKey[reg.Key]; dup {
			return invalid(reg.Key, "duplicate key %q", reg.Key)
		}
		r.byKey[reg.Key] = reg
		if reg.Polled() {
			if _, dup := r.byAddr[reg.Address]; dup {
				return invalid(reg.Key, "duplicate address %d (%s)", reg.Address, reg.Key)
			}
			r.byAddr[reg.Address] = reg
		}
	}
	return r.validateOverlaps()
}

func validateRegister(reg Register) error {
	if reg.Key == "" || reg.Scale == 0 {
		return invalid(reg.Key, "register %q needs a key and a non-zero scale", reg.Key)
	}
	if reg.Store < StoreNone || reg.Store > StoreStatus {
		return invalid(reg.Key, "register %q has invalid store %d", reg.Key, reg.Store)
	}
	if int(reg.Address)+int(reg.Count()) > addressSpace {
		return invalid(reg.Key, "register %q runs past address 65535", reg.Key)
	}
	return validateAddressing(reg)
}

// validateAddressing checks that net/computed registers have no address and that
// daily and status registers are polled.
func validateAddressing(reg Register) error {
	if !netAddressingOK(reg) {
		return invalid(reg.Key, "net register %q must be a computed energy register", reg.Key)
	}
	if reg.Computed() && reg.Polled() {
		return invalid(reg.Key, "computed register %q must not have an address", reg.Key)
	}
	if reg.needsAddress() && !reg.Polled() {
		return invalid(reg.Key, "register %q (%s) must be polled", reg.Key, reg.Store)
	}
	return nil
}

// netAddressingOK reports whether a net register is an unaddressed energy register.
func netAddressingOK(reg Register) bool {
	return !reg.Net || (!reg.Polled() && reg.Store.energy())
}

// validateOverlaps rejects registers whose address ranges overlap.
func (r *Registry) validateOverlaps() error {
	polled := r.Polled()
	for i := 1; i < len(polled); i++ {
		prev := polled[i-1]
		if int(prev.Address)+int(prev.Count()) > int(polled[i].Address) { // no uint16 wrap
			return invalid(prev.Key, "registers %q and %q overlap", prev.Key, polled[i].Key)
		}
	}
	return nil
}

// addressSpace is the number of Modbus register addresses (0-65535).
const addressSpace = 1 << 16

// validateTotalSources requires every daily source of a total to also feed a yearly
// value: the baseline fold at year close adds the year's sums per total edge, and a
// source without a yearly edge would silently fold 0 (review AGG-L2).
func (r *Registry) validateTotalSources() error {
	yearly := make(map[string]bool)
	for _, e := range r.Edges(period.Yearly) {
		yearly[e.Source] = true
	}
	for _, e := range r.Edges(period.Total) {
		if !yearly[e.Source] {
			return invalid(e.Target, "total %q sums %q, which has no yearly edge", e.Target,
				e.Source)
		}
	}
	return nil
}

// validateComputed checks edges, net pairs and that every computed register has exactly
// one definition.
func (r *Registry) validateComputed() error {
	defined, err := r.countDefinitions()
	if err != nil {
		return err
	}
	for _, reg := range r.regs {
		if reg.Computed() && defined[reg.Key] != 1 {
			return invalid(reg.Key, "computed register %q needs exactly one definition, has %d",
				reg.Key, defined[reg.Key])
		}
	}
	return nil
}

// countDefinitions validates every edge and net pair and counts definitions per target.
func (r *Registry) countDefinitions() (map[string]int, error) {
	defined := make(map[string]int)
	for _, e := range r.edges {
		if err := r.validateEdge(e); err != nil {
			return nil, err
		}
		defined[e.Target]++
	}
	for _, n := range r.nets {
		if err := r.validateNet(n); err != nil {
			return nil, err
		}
		defined[n.Target]++
	}
	return defined, nil
}

func (r *Registry) validateEdge(e Edge) error {
	if src, ok := r.byKey[e.Source]; !ok || !src.polledDaily() {
		return invalid(e.Source, "edge source %q must be a polled daily register", e.Source)
	}
	dst, ok := r.byKey[e.Target]
	if !ok || dst.Net || dst.Store != StoreForLevel(e.Level) || e.Level == period.Daily {
		return invalid(e.Target, "edge target %q must be a %s register", e.Target, e.Level)
	}
	return nil
}

func (r *Registry) validateNet(n NetPair) error {
	want := StoreForLevel(n.Level)
	for _, k := range []string{n.Export, n.Import} {
		if !r.isStore(k, want, false) {
			return invalid(k, "net source %q must be a non-net %s register", k, n.Level)
		}
	}
	if !r.isStore(n.Target, want, true) {
		return invalid(n.Target, "net target %q must be a net %s register", n.Target, n.Level)
	}
	return nil
}

// isStore reports whether key exists with the given store and net flag.
func (r *Registry) isStore(key string, store Store, net bool) bool {
	reg, ok := r.byKey[key]
	return ok && reg.Store == store && reg.Net == net
}

// ByKey returns the register with the given key.
func (r *Registry) ByKey(key string) (Register, bool) {
	reg, ok := r.byKey[key]
	return reg, ok
}

// ByAddress returns the polled register starting at addr.
func (r *Registry) ByAddress(addr uint16) (Register, bool) {
	reg, ok := r.byAddr[addr]
	return reg, ok
}

// All returns every register in table order.
func (r *Registry) All() []Register {
	return append([]Register(nil), r.regs...)
}

// Keys returns every register key sorted.
func (r *Registry) Keys() []string {
	keys := make([]string, 0, len(r.regs))
	for _, reg := range r.regs {
		keys = append(keys, reg.Key)
	}
	sort.Strings(keys)
	return keys
}

// Polled returns the addressed registers sorted by address.
func (r *Registry) Polled() []Register {
	var out []Register
	for _, reg := range r.regs {
		if reg.Polled() {
			out = append(out, reg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out
}

// ByStore returns the registers with the given store (net included).
func (r *Registry) ByStore(s Store) []Register {
	var out []Register
	for _, reg := range r.regs {
		if reg.Store == s {
			out = append(out, reg)
		}
	}
	return out
}

// DailyKeys returns the polled (non-net) daily keys.
func (r *Registry) DailyKeys() []string {
	var out []string
	for _, reg := range r.regs {
		if reg.Store == StoreDaily && !reg.Net {
			out = append(out, reg.Key)
		}
	}
	return out
}

// Edges returns the daily → level sum definitions.
func (r *Registry) Edges(l period.Level) []Edge {
	var out []Edge
	for _, e := range r.edges {
		if e.Level == l {
			out = append(out, e)
		}
	}
	return out
}

// NetPairs returns the net definitions of a level.
func (r *Registry) NetPairs(l period.Level) []NetPair {
	var out []NetPair
	for _, n := range r.nets {
		if n.Level == l {
			out = append(out, n)
		}
	}
	return out
}

// Blocks returns the planned read blocks.
func (r *Registry) Blocks() []Block {
	return append([]Block(nil), r.blocks...)
}

// PlanBlocks coalesces addressed registers (sorted by address) into read blocks: a
// register joins the current block while the unused gap is at most maxGap and the block
// stays within maxLen registers.
func PlanBlocks(regs []Register, maxGap, maxLen uint16) []Block {
	var blocks []Block
	for _, reg := range regs {
		end := reg.Address + reg.Count() // exclusive
		if n := len(blocks); n > 0 {
			cur := &blocks[n-1]
			curEnd := cur.Start + cur.Count
			if reg.Address >= curEnd && reg.Address-curEnd <= maxGap &&
				end-cur.Start <= maxLen {
				cur.Count = end - cur.Start
				continue
			}
		}
		blocks = append(blocks, Block{Start: reg.Address, Count: reg.Count()})
	}
	return blocks
}
