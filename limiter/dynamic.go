package limiter

import (
	"strconv"
	"strings"
	"time"
)

// determineSpeedLimit returns the minimum non-zero rate
func determineSpeedLimit(limit1, limit2 int) (limit int) {
	if limit1 == 0 || limit2 == 0 {
		if limit1 > limit2 {
			return limit1
		} else if limit1 < limit2 {
			return limit2
		} else {
			return 0
		}
	} else {
		if limit1 > limit2 {
			return limit2
		} else if limit1 < limit2 {
			return limit1
		} else {
			return limit1
		}
	}
}

type timeRange struct {
	startMin int // minutes from 00:00
	endMin   int
}

func parseTimeRanges(s string) []timeRange {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var ranges []timeRange
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		sides := strings.SplitN(part, "-", 2)
		if len(sides) != 2 {
			continue
		}
		start := parseHHMM(sides[0])
		end := parseHHMM(sides[1])
		if start < 0 || end < 0 {
			continue
		}
		ranges = append(ranges, timeRange{startMin: start, endMin: end})
	}
	return ranges
}

func parseHHMM(s string) int {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return -1
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil || h < 0 || h > 24 {
		return -1
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil || m < 0 || m > 59 {
		return -1
	}
	return h*60 + m
}

func inTimeRanges(ranges []timeRange, now time.Time) bool {
	if len(ranges) == 0 {
		return true // empty = all day
	}
	utc8 := now.In(time.FixedZone("UTC+8", 8*3600))
	cur := utc8.Hour()*60 + utc8.Minute()
	for _, r := range ranges {
		if r.startMin <= r.endMin {
			if cur >= r.startMin && cur < r.endMin {
				return true
			}
		} else {
			if cur >= r.startMin || cur < r.endMin {
				return true
			}
		}
	}
	return false
}

func parseWhitelist(s string) map[int]struct{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	m := make(map[int]struct{})
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if id, err := strconv.Atoi(part); err == nil {
			m[id] = struct{}{}
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
