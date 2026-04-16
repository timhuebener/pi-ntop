package handlers

import "time"

var defaultPeriods = map[string]time.Duration{
	"1h":  time.Hour,
	"6h":  6 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
	"1y":  365 * 24 * time.Hour,
	"all": 0, // sentinel: zero means "all time"
}

const defaultPeriod = "24h"

// resolveSince returns the "since" time for the given period key.
// For "all", it returns a zero time.
func resolveSince(period string) time.Time {
	dur := defaultPeriods[period]
	if dur == 0 {
		return time.Time{}
	}
	return time.Now().UTC().Add(-dur)
}

// labelFormatForPeriod returns a Go time format string appropriate for the
// chart label granularity of the given period.
func labelFormatForPeriod(period string) string {
	switch period {
	case "1h", "6h", "24h":
		return "01/02 15:04"
	case "7d", "30d":
		return "01/02 15:04"
	case "90d":
		return "Jan 02"
	case "1y":
		return "Jan 02"
	default: // "all"
		return "Jan 2006"
	}
}
