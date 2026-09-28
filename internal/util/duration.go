package util

import (
	"errors"
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
		if err != nil {
			return 0, &DurationError{Input: s}
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

// durationToken converts one "<number><unit>" pair.
func durationToken(num, unit string) (time.Duration, error) {
	long := map[string]time.Duration{"d": Day, "w": Week, "y": Year}
	if base, ok := long[unit]; ok {
		n, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, err
		}
		return time.Duration(n * float64(base)), nil
	}
	return time.ParseDuration(num + unit)
}
