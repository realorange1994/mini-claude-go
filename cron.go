package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CronFields holds the expanded numeric values for a 5-field cron expression.
type CronFields struct {
	Minute     []int
	Hour       []int
	DayOfMonth []int
	Month      []int
	DayOfWeek  []int
}

type fieldRange struct {
	min int
	max int
}

var fieldRanges = []fieldRange{
	{0, 59}, // minute
	{0, 23}, // hour
	{1, 31}, // dayOfMonth
	{1, 12}, // month
	{0, 6},  // dayOfWeek (0=Sunday; 7 accepted as Sunday alias)
}

// expandField parses a single cron field into a sorted slice of values.
// Supports: *, */N, N, N-M, N-M/S, N,M,...
func expandField(field string, r fieldRange) []int {
	out := make(map[int]bool)
	parts := strings.Split(field, ",")

	for _, part := range parts {
		// wildcard or */N
		if part == "*" {
			for i := r.min; i <= r.max; i++ {
				out[i] = true
			}
			continue
		}
		if strings.HasPrefix(part, "*/") {
			stepStr := part[2:]
			step, err := strconv.Atoi(stepStr)
			if err != nil || step < 1 {
				return nil
			}
			for i := r.min; i <= r.max; i += step {
				out[i] = true
			}
			continue
		}

		// N-M or N-M/S
		rangeParts := strings.Split(part, "/")
		rangePart := rangeParts[0]
		step := 1
		if len(rangeParts) == 2 {
			s, err := strconv.Atoi(rangeParts[1])
			if err != nil || s < 1 {
				return nil
			}
			step = s
		} else if len(rangeParts) > 2 {
			return nil
		}

		if strings.Contains(rangePart, "-") {
			bounds := strings.SplitN(rangePart, "-", 2)
			if len(bounds) != 2 {
				return nil
			}
			lo, err1 := strconv.Atoi(bounds[0])
			hi, err2 := strconv.Atoi(bounds[1])
			if err1 != nil || err2 != nil {
				return nil
			}
			// dayOfWeek: accept 7 as Sunday alias in ranges
			effMax := r.max
			isDow := r.min == 0 && r.max == 6
			if isDow {
				effMax = 7
			}
			if lo > hi || lo < r.min || hi > effMax {
				return nil
			}
			for i := lo; i <= hi; i += step {
				val := i
				if isDow && val == 7 {
					val = 0
				}
				out[val] = true
			}
			continue
		}

		// plain N
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		isDow := r.min == 0 && r.max == 6
		if isDow && n == 7 {
			n = 0
		}
		if n < r.min || n > r.max {
			return nil
		}
		out[n] = true
	}

	if len(out) == 0 {
		return nil
	}
	result := make([]int, 0, len(out))
	for k := range out {
		result = append(result, k)
	}
	slices.Sort(result)
	return result
}

// ParseCronExpression parses a 5-field cron expression.
// Returns nil if invalid.
func ParseCronExpression(expr string) *CronFields {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil
	}

	expanded := make([][]int, 5)
	for i := 0; i < 5; i++ {
		result := expandField(parts[i], fieldRanges[i])
		if result == nil {
			return nil
		}
		expanded[i] = result
	}

	return &CronFields{
		Minute:     expanded[0],
		Hour:       expanded[1],
		DayOfMonth: expanded[2],
		Month:      expanded[3],
		DayOfWeek:  expanded[4],
	}
}

// ComputeNextCronRun computes the next Date strictly after `from` matching
// the cron fields, using the process's local timezone.
func ComputeNextCronRun(fields CronFields, from time.Time) *time.Time {
	minuteSet := make(map[int]bool)
	for _, v := range fields.Minute {
		minuteSet[v] = true
	}
	hourSet := make(map[int]bool)
	for _, v := range fields.Hour {
		hourSet[v] = true
	}
	domSet := make(map[int]bool)
	for _, v := range fields.DayOfMonth {
		domSet[v] = true
	}
	monthSet := make(map[int]bool)
	for _, v := range fields.Month {
		monthSet[v] = true
	}
	dowSet := make(map[int]bool)
	for _, v := range fields.DayOfWeek {
		dowSet[v] = true
	}

	domWild := len(fields.DayOfMonth) == 31
	dowWild := len(fields.DayOfWeek) == 7

	// Round up to the next whole minute (strictly after `from`)
	t := from.Truncate(time.Minute).Add(time.Minute)

	const maxIter = 366 * 24 * 60
	for i := 0; i < maxIter; i++ {
		// Go months are 0-indexed, so getMonth()+1
		month := int(t.Month())
		if !monthSet[month] {
			// Jump to start of next month
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		dom := t.Day()
		dow := int(t.Weekday())
		dayMatches := false
		if domWild && dowWild {
			dayMatches = true
		} else if domWild {
			dayMatches = dowSet[dow]
		} else if dowWild {
			dayMatches = domSet[dom]
		} else {
			dayMatches = domSet[dom] || dowSet[dow]
		}

		if !dayMatches {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		if !hourSet[t.Hour()] {
			t = t.Add(time.Hour)
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
			continue
		}

		if !minuteSet[t.Minute()] {
			t = t.Add(time.Minute)
			continue
		}

		return &t
	}

	return nil
}

// --- cronToHuman ------------------------------------------------------------

var dayNames = []string{
	"Sunday",
	"Monday",
	"Tuesday",
	"Wednesday",
	"Thursday",
	"Friday",
	"Saturday",
}

func formatLocalTime(minute, hour int) string {
	d := time.Date(2000, time.January, 1, hour, minute, 0, 0, time.UTC)
	return d.Format("3:04 PM")
}

// CronToHuman converts a cron expression string to a human-readable form.
func CronToHuman(cron string) string {
	parts := strings.Fields(cron)
	if len(parts) != 5 {
		return cron
	}

	minute, hour, dayOfMonth, month, dayOfWeek := parts[0], parts[1], parts[2], parts[3], parts[4]

	// Every N minutes: */N * * * *
	if dayOfMonth == "*" && month == "*" && dayOfWeek == "*" && hour == "*" {
		if strings.HasPrefix(minute, "*/") {
			nStr := minute[2:]
			if n, err := strconv.Atoi(nStr); err == nil && n > 0 {
				if n == 1 {
					return "Every minute"
				}
				return fmt.Sprintf("Every %d minutes", n)
			}
		}
	}

	// Every hour at :M: M * * * *
	if hour == "*" && dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		if n, err := strconv.Atoi(minute); err == nil {
			if n == 0 {
				return "Every hour"
			}
			return fmt.Sprintf("Every hour at :%02d", n)
		}
	}

	// Every N hours: 0 */N * * *
	if dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		if strings.HasPrefix(hour, "*/") {
			m, errM := strconv.Atoi(minute)
			n, errN := strconv.Atoi(hour[2:])
			if errM == nil && errN == nil && n > 0 {
				suffix := ""
				if m != 0 {
					suffix = fmt.Sprintf(" at :%02d", m)
				}
				if n == 1 {
					return fmt.Sprintf("Every hour%s", suffix)
				}
				return fmt.Sprintf("Every %d hours%s", n, suffix)
			}
		}
	}

	// Remaining cases need single-digit minute and hour
	m, errM := strconv.Atoi(minute)
	h, errH := strconv.Atoi(hour)
	if errM != nil || errH != nil {
		return cron
	}

	// Daily at specific time: M H * * *
	if dayOfMonth == "*" && month == "*" && dayOfWeek == "*" {
		return fmt.Sprintf("Every day at %s", formatLocalTime(m, h))
	}

	// Specific day of week: M H * * D
	if dayOfMonth == "*" && month == "*" && len(dayOfWeek) == 1 {
		if d, err := strconv.Atoi(dayOfWeek); err == nil {
			dayIndex := d % 7
			return fmt.Sprintf("Every %s at %s", dayNames[dayIndex], formatLocalTime(m, h))
		}
	}

	// Weekdays: M H * * 1-5
	if dayOfMonth == "*" && month == "*" && dayOfWeek == "1-5" {
		return fmt.Sprintf("Weekdays at %s", formatLocalTime(m, h))
	}

	return cron
}
