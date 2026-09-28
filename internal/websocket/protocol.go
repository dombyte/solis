// Package websocket implements the v3 subscription protocol: clients subscribe to keys,
// receive a snapshot of exactly those keys and then coalesced diff updates of changed,
// subscribed keys only. The hub consumes cache events from the bus and is a restartable
// health component; the upgrader is same-origin only.
package websocket

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/dombyte/solis/internal/solis"
	"github.com/dombyte/solis/internal/util"
)

// Message types.
const (
	TypeSubscribe   = "subscribe"
	TypeUnsubscribe = "unsubscribe"
	TypePing        = "ping"
	TypeSnapshot    = "snapshot"
	TypeUpdate      = "update"
	TypeError       = "error"
)

// Error codes of error frames.
const (
	CodeUnknownKeys = "unknown_keys"
	CodeBadRequest  = "bad_request"
)

// ClientMessage is a client → server frame.
type ClientMessage struct {
	Type string   `json:"type"`
	Keys []string `json:"keys,omitempty"`
}

// ValueDTO is one key's value on the wire (rounded to two decimals).
type ValueDTO struct {
	Value         util.Float64With2Decimals `json:"value"`
	Timestamp     string                    `json:"timestamp,omitempty"`
	Unit          string                    `json:"unit,omitempty"`
	StatusDecoded any                       `json:"status_decoded,omitempty"`
}

// SnapshotMessage answers a subscribe with the current values of the new keys.
type SnapshotMessage struct {
	Type   string              `json:"type"`
	Values map[string]ValueDTO `json:"values"`
}

// UpdateMessage pushes changed subscribed keys; ts is the frame time. Removed lists
// subscribed keys that no longer have a current value (e.g. a derived value whose
// inputs became unknown); clients drop them instead of showing a stale value.
type UpdateMessage struct {
	Type    string              `json:"type"`
	TS      string              `json:"ts"`
	Values  map[string]ValueDTO `json:"values"`
	Removed []string            `json:"removed,omitempty"`
}

// ErrorMessage reports a protocol problem; the connection stays open.
type ErrorMessage struct {
	Type    string   `json:"type"`
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Keys    []string `json:"keys,omitempty"`
}

// pushed is the per-client diff state of one key (value + status only, Plan.md D7).
type pushed struct {
	value  float64
	status any
}

func (p pushed) equal(o pushed) bool {
	return p.value == o.value && reflect.DeepEqual(p.status, o.status)
}

func stateOf(v *solis.Value) pushed {
	return pushed{value: util.RoundTo2DecimalPlaces(v.DecodedValue), status: v.StatusDecoded}
}

// fullDTO is used in snapshots (timestamp + unit included).
func fullDTO(v *solis.Value) ValueDTO {
	d := updateDTO(v)
	d.Timestamp = v.Timestamp.Format(time.RFC3339)
	d.Unit = v.Unit
	return d
}

// updateDTO is used in updates (value + status only).
func updateDTO(v *solis.Value) ValueDTO {
	return ValueDTO{
		Value:         util.Float64With2Decimals(util.RoundTo2DecimalPlaces(v.DecodedValue)),
		StatusDecoded: v.StatusDecoded,
	}
}

func encode(msg any) ([]byte, error) {
	return json.Marshal(msg)
}
