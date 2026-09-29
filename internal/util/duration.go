package util

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

// Calendar-free long duration units accepted by ParseDuration.
const (
	Day  = 24 * time.Hour
	Week = 7 * Day
	Year = 365 * Day
)

// ErrInvalidDuration is returned by ParseDuration for malformed input.
var ErrInvalidDuration = errors.New("invalid duration")

// ParseDuration parses a Go duration extended by the units d (24h), w (7d) and y (365d),
// e.g. "1y", "2w", "1d12h" or "720h". Units may be combined; the value must be >= 0.
func ParseDuration(s string) (time.Duration, error) {
	in := strings.TrimSpace(s)
	if in == "" {
		return 0, &DurationError{}
	}
	var total time.Duration
	for rest := in; rest != ""; {
		num, unit, tail := splitDurationToken(rest)
		if num == "" || unit == "" {
			return 0, &DurationError{Input: s}
		}
		d, err := durationToken(num, unit)
		if err != nil || total > math.MaxInt64-d {
			return 0, &DurationError{Input: s} // malformed, or the sum overflows
		}
		total += d
		rest = tail
	}
	return total, nil
}

// splitDurationToken splits the leading "<number><unit>" off s.
func splitDurationToken(s string) (num, unit, tail string) {
	i := 0
	for i < len(s) && isNumberByte(s[i]) {
		i++
	}
	j := i
	for j < len(s) && !isNumberByte(s[j]) {
		j++
	}
	return s[:i], s[i:j], s[j:]
}

func isNumberByte(b byte) bool { return b >= '0' && b <= '9' || b == '.' }

// longUnit returns the length of the extra units d, w and y.
func longUnit(unit string) (time.Duration, bool) {
	switch unit {
	case "d":
		return Day, true
	case "w":
		return Week, true
	case "y":
		return Year, true
	default:
		return 0, false
	}
}

// durationToken converts one "<number><unit>" pair.
func durationToken(num, unit string) (time.Duration, error) {
	if base, ok := longUnit(unit); ok {
		n, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, err
		}
		f := n * float64(base)
		if f >= math.MaxInt64 { // ~292 years; the conversion would wrap
			return 0, ErrInvalidDuration
		}
		return time.Duration(f), nil
	}
	return time.ParseDuration(num + unit)
}
