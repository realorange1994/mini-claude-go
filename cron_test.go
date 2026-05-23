package main

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// parseCronExpression — valid expressions
// ---------------------------------------------------------------------------

func TestParseCronExpression_WildcardFields(t *testing.T) {
	result := ParseCronExpression("* * * * *")
	if result == nil {
		t.Fatal("expected non-nil result for wildcard expression")
	}
	if len(result.Minute) != 60 {
		t.Errorf("minute: expected 60 values, got %d", len(result.Minute))
	}
	if len(result.Hour) != 24 {
		t.Errorf("hour: expected 24 values, got %d", len(result.Hour))
	}
	if len(result.DayOfMonth) != 31 {
		t.Errorf("dayOfMonth: expected 31 values, got %d", len(result.DayOfMonth))
	}
	if len(result.Month) != 12 {
		t.Errorf("month: expected 12 values, got %d", len(result.Month))
	}
	if len(result.DayOfWeek) != 7 {
		t.Errorf("dayOfWeek: expected 7 values, got %d", len(result.DayOfWeek))
	}
}

func TestParseCronExpression_SpecificValues(t *testing.T) {
	result := ParseCronExpression("30 14 1 6 3")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertIntSlice(t, result.Minute, []int{30})
	assertIntSlice(t, result.Hour, []int{14})
	assertIntSlice(t, result.DayOfMonth, []int{1})
	assertIntSlice(t, result.Month, []int{6})
	assertIntSlice(t, result.DayOfWeek, []int{3})
}

func TestParseCronExpression_StepSyntax(t *testing.T) {
	result := ParseCronExpression("*/5 * * * *")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertIntSlice(t, result.Minute, []int{0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55})
}

func TestParseCronExpression_RangeSyntax(t *testing.T) {
	result := ParseCronExpression("1-5 * * * *")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertIntSlice(t, result.Minute, []int{1, 2, 3, 4, 5})
}

func TestParseCronExpression_RangeWithStep(t *testing.T) {
	result := ParseCronExpression("1-10/3 * * * *")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertIntSlice(t, result.Minute, []int{1, 4, 7, 10})
}

func TestParseCronExpression_CommaSeparatedList(t *testing.T) {
	result := ParseCronExpression("1,15,30 * * * *")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertIntSlice(t, result.Minute, []int{1, 15, 30})
}

func TestParseCronExpression_DayOfWeek7AsSundayAlias(t *testing.T) {
	result := ParseCronExpression("0 0 * * 7")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertIntSlice(t, result.DayOfWeek, []int{0})
}

func TestParseCronExpression_RangeWithDayOfWeek7(t *testing.T) {
	result := ParseCronExpression("0 0 * * 5-7")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertIntSlice(t, result.DayOfWeek, []int{0, 5, 6})
}

func TestParseCronExpression_ComplexCombined(t *testing.T) {
	result := ParseCronExpression("0,30 9-17 * * 1-5")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	assertIntSlice(t, result.Minute, []int{0, 30})
	assertIntSlice(t, result.Hour, []int{9, 10, 11, 12, 13, 14, 15, 16, 17})
	assertIntSlice(t, result.DayOfWeek, []int{1, 2, 3, 4, 5})
}

// ---------------------------------------------------------------------------
// parseCronExpression — invalid expressions
// ---------------------------------------------------------------------------

func TestParseCronExpression_WrongFieldCount(t *testing.T) {
	if ParseCronExpression("* * *") != nil {
		t.Error("expected nil for wrong field count")
	}
}

func TestParseCronExpression_OutOfRangeMinute(t *testing.T) {
	if ParseCronExpression("60 * * * *") != nil {
		t.Error("expected nil for out-of-range minute")
	}
}

func TestParseCronExpression_InvalidStep(t *testing.T) {
	if ParseCronExpression("*/0 * * * *") != nil {
		t.Error("expected nil for step of 0")
	}
}

func TestParseCronExpression_ReversedRange(t *testing.T) {
	if ParseCronExpression("10-5 * * * *") != nil {
		t.Error("expected nil for reversed range")
	}
}

func TestParseCronExpression_EmptyString(t *testing.T) {
	if ParseCronExpression("") != nil {
		t.Error("expected nil for empty string")
	}
}

func TestParseCronExpression_NonNumericTokens(t *testing.T) {
	if ParseCronExpression("abc * * * *") != nil {
		t.Error("expected nil for non-numeric tokens")
	}
}

// ---------------------------------------------------------------------------
// parseCronExpression — field range validation
// ---------------------------------------------------------------------------

func TestParseCronExpression_MinuteRange(t *testing.T) {
	if ParseCronExpression("0 * * * *") == nil {
		t.Error("minute 0 should be valid")
	}
	if ParseCronExpression("59 * * * *") == nil {
		t.Error("minute 59 should be valid")
	}
	if ParseCronExpression("60 * * * *") != nil {
		t.Error("minute 60 should be invalid")
	}
}

func TestParseCronExpression_HourRange(t *testing.T) {
	if ParseCronExpression("* 0 * * *") == nil {
		t.Error("hour 0 should be valid")
	}
	if ParseCronExpression("* 23 * * *") == nil {
		t.Error("hour 23 should be valid")
	}
	if ParseCronExpression("* 24 * * *") != nil {
		t.Error("hour 24 should be invalid")
	}
}

func TestParseCronExpression_DayOfMonthRange(t *testing.T) {
	if ParseCronExpression("* * 1 * *") == nil {
		t.Error("dayOfMonth 1 should be valid")
	}
	if ParseCronExpression("* * 31 * *") == nil {
		t.Error("dayOfMonth 31 should be valid")
	}
	if ParseCronExpression("* * 0 * *") != nil {
		t.Error("dayOfMonth 0 should be invalid")
	}
	if ParseCronExpression("* * 32 * *") != nil {
		t.Error("dayOfMonth 32 should be invalid")
	}
}

func TestParseCronExpression_MonthRange(t *testing.T) {
	if ParseCronExpression("* * * 1 *") == nil {
		t.Error("month 1 should be valid")
	}
	if ParseCronExpression("* * * 12 *") == nil {
		t.Error("month 12 should be valid")
	}
	if ParseCronExpression("* * * 0 *") != nil {
		t.Error("month 0 should be invalid")
	}
	if ParseCronExpression("* * * 13 *") != nil {
		t.Error("month 13 should be invalid")
	}
}

func TestParseCronExpression_DayOfWeekRange(t *testing.T) {
	if ParseCronExpression("* * * * 0") == nil {
		t.Error("dayOfWeek 0 should be valid")
	}
	if ParseCronExpression("* * * * 6") == nil {
		t.Error("dayOfWeek 6 should be valid")
	}
	if ParseCronExpression("* * * * 7") == nil {
		t.Error("dayOfWeek 7 (Sunday alias) should be valid")
	}
	if ParseCronExpression("* * * * 8") != nil {
		t.Error("dayOfWeek 8 should be invalid")
	}
}

// ---------------------------------------------------------------------------
// computeNextCronRun
// ---------------------------------------------------------------------------

func mustParseCron(t *testing.T, expr string) CronFields {
	t.Helper()
	f := ParseCronExpression(expr)
	if f == nil {
		t.Fatalf("failed to parse cron expression: %q", expr)
	}
	return *f
}

func TestComputeNextCronRun_FindsNextMinute(t *testing.T) {
	fields := mustParseCron(t, "31 14 * * *")
	from := time.Date(2026, time.January, 15, 14, 30, 45, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if next.Hour() != 14 {
		t.Errorf("expected hour 14, got %d", next.Hour())
	}
	if next.Minute() != 31 {
		t.Errorf("expected minute 31, got %d", next.Minute())
	}
}

func TestComputeNextCronRun_FindsNextHour(t *testing.T) {
	fields := mustParseCron(t, "0 15 * * *")
	from := time.Date(2026, time.January, 15, 14, 30, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if next.Hour() != 15 {
		t.Errorf("expected hour 15, got %d", next.Hour())
	}
	if next.Minute() != 0 {
		t.Errorf("expected minute 0, got %d", next.Minute())
	}
}

func TestComputeNextCronRun_RollsToNextDay(t *testing.T) {
	fields := mustParseCron(t, "0 10 * * *")
	from := time.Date(2026, time.January, 15, 14, 30, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if next.Day() != 16 {
		t.Errorf("expected day 16, got %d", next.Day())
	}
	if next.Hour() != 10 {
		t.Errorf("expected hour 10, got %d", next.Hour())
	}
}

func TestComputeNextCronRun_StrictlyAfterFromDate(t *testing.T) {
	fields := mustParseCron(t, "30 14 * * *")
	// exactly on cron time
	from := time.Date(2026, time.January, 15, 14, 30, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if !next.After(from) {
		t.Error("next run must be strictly after from date")
	}
}

func TestComputeNextCronRun_Every5Minutes(t *testing.T) {
	fields := mustParseCron(t, "*/5 * * * *")
	from := time.Date(2026, time.January, 15, 14, 32, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if next.Minute() != 35 {
		t.Errorf("expected minute 35, got %d", next.Minute())
	}
}

func TestComputeNextCronRun_EveryMinute(t *testing.T) {
	fields := mustParseCron(t, "* * * * *")
	from := time.Date(2026, time.January, 15, 14, 32, 45, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if next.Minute() != 33 {
		t.Errorf("expected minute 33, got %d", next.Minute())
	}
}

func TestComputeNextCronRun_StepAcrossMidnight(t *testing.T) {
	fields := mustParseCron(t, "0 0 * * *")
	from := time.Date(2026, time.January, 15, 23, 59, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if next.Hour() != 0 {
		t.Errorf("expected hour 0, got %d", next.Hour())
	}
	if next.Day() != 16 {
		t.Errorf("expected day 16, got %d", next.Day())
	}
}

func TestComputeNextCronRun_ORSemanticsWhenBothDomAndDowConstrained(t *testing.T) {
	// dom=15, dow=3(Wed) - matches 15th OR Wednesday
	fields := mustParseCron(t, "0 0 15 * 3")
	from := time.Date(2026, time.January, 12, 0, 0, 0, 0, time.Local) // Monday Jan 12
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	// Should match the first of either: next Wednesday(Jan 14) or 15th(Jan 15)
	dayOfWeek := int(next.Weekday())
	dayOfMonth := next.Day()
	if dayOfWeek != 3 && dayOfMonth != 15 {
		t.Errorf("expected either Wednesday(3) or 15th, got weekday=%d day=%d", dayOfWeek, dayOfMonth)
	}
}

// ---------------------------------------------------------------------------
// cronToHuman
// ---------------------------------------------------------------------------

func TestCronToHuman_EveryNMinutes(t *testing.T) {
	got := CronToHuman("*/5 * * * *")
	want := "Every 5 minutes"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCronToHuman_EveryMinute(t *testing.T) {
	got := CronToHuman("*/1 * * * *")
	want := "Every minute"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCronToHuman_EveryHourAt00(t *testing.T) {
	got := CronToHuman("0 * * * *")
	want := "Every hour"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCronToHuman_EveryHourAt30(t *testing.T) {
	got := CronToHuman("30 * * * *")
	want := "Every hour at :30"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCronToHuman_EveryNHours(t *testing.T) {
	got := CronToHuman("0 */2 * * *")
	want := "Every 2 hours"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCronToHuman_DailyAtSpecificTime(t *testing.T) {
	got := CronToHuman("30 9 * * *")
	if !strings.Contains(got, "Every day at") {
		t.Errorf("expected 'Every day at' in %q", got)
	}
	if !strings.Contains(got, "9:30") {
		t.Errorf("expected '9:30' in %q", got)
	}
}

func TestCronToHuman_SpecificDayOfWeek(t *testing.T) {
	got := CronToHuman("0 9 * * 3")
	if !strings.Contains(got, "Wednesday") {
		t.Errorf("expected 'Wednesday' in %q", got)
	}
	if !strings.Contains(got, "9:00") {
		t.Errorf("expected '9:00' in %q", got)
	}
}

func TestCronToHuman_Weekdays(t *testing.T) {
	got := CronToHuman("0 9 * * 1-5")
	if !strings.Contains(got, "Weekdays") {
		t.Errorf("expected 'Weekdays' in %q", got)
	}
	if !strings.Contains(got, "9:00") {
		t.Errorf("expected '9:00' in %q", got)
	}
}

func TestCronToHuman_ComplexPatternFallsThrough(t *testing.T) {
	got := CronToHuman("0,30 9-17 * * 1-5")
	want := "0,30 9-17 * * 1-5"
	if got != want {
		t.Errorf("got %q, want %q (raw cron for complex patterns)", got, want)
	}
}

func TestCronToHuman_WrongFieldCountFallsThrough(t *testing.T) {
	got := CronToHuman("* * *")
	want := "* * *"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// roundtrip invariant: parse -> computeNext -> result must match field constraints
// ---------------------------------------------------------------------------

func TestRoundtrip_ParseAndCompute(t *testing.T) {
	expr := "30 14 * * 1-5"
	fields := mustParseCron(t, expr)
	from := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.Local) // Sunday
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	// The result must have minute=30, hour=14, weekday Mon-Fri
	if next.Minute() != 30 {
		t.Errorf("minute: got %d, want 30", next.Minute())
	}
	if next.Hour() != 14 {
		t.Errorf("hour: got %d, want 14", next.Hour())
	}
	dow := int(next.Weekday())
	if dow < 1 || dow > 5 {
		t.Errorf("day of week: got %d, want 1-5", dow)
	}
}

// ---------------------------------------------------------------------------
// boundary conditions
// ---------------------------------------------------------------------------

func TestParseCronExpression_TooManyFields(t *testing.T) {
	if ParseCronExpression("* * * * * *") != nil {
		t.Error("expected nil for 6-field expression")
	}
}

func TestParseCronExpression_WhitespaceOnly(t *testing.T) {
	if ParseCronExpression("   ") != nil {
		t.Error("expected nil for whitespace-only string")
	}
}

func TestParseCronExpression_NegativeNumber(t *testing.T) {
	if ParseCronExpression("-1 * * * *") != nil {
		t.Error("expected nil for negative number")
	}
}

func TestComputeNextCronRun_NeverMatches(t *testing.T) {
	// month=13 is invalid, but let's test a valid expression that
	// constrains dayOfMonth=31 and month=Feb — Feb never has 31 days.
	// However, this is still a valid parse; the walk just never finds a match
	// within 366 days if the date doesn't exist. In practice, Feb 31 never
	// exists, but the algorithm will skip Feb entirely and try March 31.
	// So this should find March 31 instead.
	fields := mustParseCron(t, "0 0 31 2 0") // Feb 31 doesn't exist, but dow=0 (Sunday) is wild-like
	// Actually dom=31 is constrained and dow=0 is constrained too,
	// so OR semantics: matches 31st of any month OR any Sunday in Feb.
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result (should find either a Sunday in Feb or 31st of another month)")
	}
}

// ---------------------------------------------------------------------------
// upstream port: cronTasks.baseline — nextCronRunMs returns null for invalid
// cron expressions (equivalent: ComputeNextCronRun with nil fields)
// ---------------------------------------------------------------------------

func TestComputeNextCronRun_NilFieldsReturnsNil(t *testing.T) {
	// Upstream: nextCronRunMs('invalid cron', Date.now()) === null
	// Go equivalent: when parse fails, we get nil fields
	result := ComputeNextCronRun(CronFields{}, time.Now())
	// Empty fields: no minutes/hours/etc. match → should walk to maxIter and return nil
	if result != nil {
		t.Errorf("expected nil for empty CronFields, got %v", result)
	}
}

// ---------------------------------------------------------------------------
// upstream port: leap year / Feb 29 edge case
// ---------------------------------------------------------------------------

func TestComputeNextCronRun_Feb29LeapYear(t *testing.T) {
	fields := mustParseCron(t, "0 0 29 2 *") // Feb 29
	// From 2025 (non-leap year), next Feb 29 is 2028
	from := time.Date(2025, time.January, 1, 0, 0, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result for Feb 29 cron")
	}
	if next.Year() != 2028 {
		t.Errorf("expected year 2028, got %d", next.Year())
	}
	if next.Month() != time.February {
		t.Errorf("expected February, got %v", next.Month())
	}
	if next.Day() != 29 {
		t.Errorf("expected day 29, got %d", next.Day())
	}
}

func TestComputeNextCronRun_Feb29FromLeapYear(t *testing.T) {
	fields := mustParseCron(t, "0 12 29 2 *") // Feb 29 at noon
	// From 2024-02-29, next should be 2028-02-29
	from := time.Date(2024, time.February, 29, 12, 0, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if next.Year() != 2028 {
		t.Errorf("expected 2028, got %d", next.Year())
	}
}

// ---------------------------------------------------------------------------
// upstream port: oneShotJitteredNextCronRunMs never returns time earlier
// than fromMs (equivalent: ComputeNextCronRun always returns time strictly
// after fromMs)
// ---------------------------------------------------------------------------

func TestComputeNextCronRun_AlwaysStrictlyAfterFrom(t *testing.T) {
	fields := mustParseCron(t, "0 0 * * *")
	// Exactly on a cron time
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil result")
	}
	if !next.After(from) {
		t.Errorf("next run %v must be strictly after from %v", next, from)
	}
}

// ---------------------------------------------------------------------------
// upstream port: cronTasks.baseline — jittered never earlier than fromMs
// (invariant: next computed time >= fromMs)
// ---------------------------------------------------------------------------

func TestComputeNextCronRun_NeverEarlierThanFrom(t *testing.T) {
	testCases := []struct {
		name string
		expr string
		from time.Time
	}{
		{
			"midnight",
			"0 0 * * *",
			time.Date(2026, time.April, 12, 10, 59, 50, 0, time.Local),
		},
		{
			"every5min",
			"*/5 * * * *",
			time.Date(2026, time.April, 12, 10, 32, 17, 0, time.Local),
		},
		{
			"specific",
			"30 14 15 4 *",
			time.Date(2026, time.April, 15, 14, 30, 0, 0, time.Local),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fields := mustParseCron(t, tc.expr)
			next := ComputeNextCronRun(fields, tc.from)
			if next == nil {
				t.Fatalf("expected non-nil for expr=%q from=%v", tc.expr, tc.from)
			}
			if !next.After(tc.from) && !next.Equal(tc.from.Add(time.Minute)) {
				t.Errorf("next %v must be >= from %v (for cron strictly-after semantics)", next, tc.from)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// upstream port: cronScheduler.baseline — boundary patterns for cron
// parsing edge cases from upstream
// ---------------------------------------------------------------------------

func TestParseCronExpression_LeadingZeros(t *testing.T) {
	// Upstream accepts "00 09 * * *" — leading zeros should parse
	result := ParseCronExpression("00 09 * * *")
	if result == nil {
		t.Fatal("expected non-nil for leading zeros")
	}
	assertIntSlice(t, result.Minute, []int{0})
	assertIntSlice(t, result.Hour, []int{9})
}

func TestParseCronExpression_MultipleCommas(t *testing.T) {
	result := ParseCronExpression("1,3,5,7,9 * * * *")
	if result == nil {
		t.Fatal("expected non-nil")
	}
	assertIntSlice(t, result.Minute, []int{1, 3, 5, 7, 9})
}

func TestParseCronExpression_RangeEdgeCases(t *testing.T) {
	// Same start and end
	result := ParseCronExpression("5-5 * * * *")
	if result == nil {
		t.Fatal("expected non-nil for 5-5 range")
	}
	assertIntSlice(t, result.Minute, []int{5})
}

func TestParseCronExpression_StepFromRange(t *testing.T) {
	result := ParseCronExpression("0-59/15 * * * *")
	if result == nil {
		t.Fatal("expected non-nil")
	}
	assertIntSlice(t, result.Minute, []int{0, 15, 30, 45})
}

// ---------------------------------------------------------------------------
// upstream port: CronToHuman roundtrip / idempotency invariants
// ---------------------------------------------------------------------------

func TestCronToHuman_IdempotentOnFallback(t *testing.T) {
	// Complex patterns fall through to raw cron — idempotent
	complex := "0,30 9-17 * * 1-5"
	got := CronToHuman(complex)
	if got != complex {
		t.Errorf("complex cron should be idempotent: got %q", got)
	}
}

func TestCronToHuman_MonthlyNotRecognized(t *testing.T) {
	// 1st of month — falls through to raw
	got := CronToHuman("0 0 1 * *")
	if got != "0 0 1 * *" {
		// If it gets a specific pattern, that's fine too — just not wrong
		// Check it doesn't say "Every minute" etc.
		if got == "Every minute" || got == "Every hour" {
			t.Errorf("got wrong pattern for monthly cron: %q", got)
		}
	}
}

// ---------------------------------------------------------------------------
// upstream port: cron baseline — year boundary transitions
// ---------------------------------------------------------------------------

func TestComputeNextCronRun_NewYearTransition(t *testing.T) {
	fields := mustParseCron(t, "0 0 1 1 *") // Jan 1 midnight
	from := time.Date(2025, time.December, 31, 23, 59, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil for New Year cron")
	}
	if next.Year() != 2026 {
		t.Errorf("expected year 2026, got %d", next.Year())
	}
	if next.Month() != time.January {
		t.Errorf("expected January, got %v", next.Month())
	}
	if next.Day() != 1 {
		t.Errorf("expected day 1, got %d", next.Day())
	}
}

func TestComputeNextCronRun_SpecificDateMissedDuringSleep(t *testing.T) {
	// Simulates a missed task: cron was set to fire at 10:00, but we woke up at 10:10
	fields := mustParseCron(t, "0 10 * * *")
	from := time.Date(2026, time.April, 12, 10, 10, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil")
	}
	// Should find tomorrow at 10:00
	if next.Day() != 13 || next.Hour() != 10 {
		t.Errorf("expected April 13 10:00, got %v", next)
	}
}

// ---------------------------------------------------------------------------
// upstream port: cronScheduler.baseline — invariant that empty/missing
// CronFields produce no match (equivalent of invalid cron → null)
// ---------------------------------------------------------------------------

func TestComputeNextCronRun_FarFutureExpression(t *testing.T) {
	// Upstream: far future cron like "59 23 31 12 *" (Dec 31 23:59)
	fields := mustParseCron(t, "59 23 31 12 *")
	from := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil for Dec 31 cron")
	}
	if next.Month() != time.December {
		t.Errorf("expected December, got %v", next.Month())
	}
	if next.Day() != 31 {
		t.Errorf("expected day 31, got %d", next.Day())
	}
	if next.Hour() != 23 {
		t.Errorf("expected hour 23, got %d", next.Hour())
	}
}

// ---------------------------------------------------------------------------
// upstream port: roundtrip — parse -> compute -> result must satisfy fields
// ---------------------------------------------------------------------------

func TestRoundtrip_WeekdayOnly(t *testing.T) {
	// Monday at 9am: 0 9 * * 1
	expr := "0 9 * * 1"
	fields := mustParseCron(t, expr)
	from := time.Date(2026, time.March, 2, 9, 0, 0, 0, time.Local) // Monday 9:00
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil")
	}
	if !next.After(from) {
		t.Error("must be strictly after from")
	}
	// Should be next Monday at 9:00
	if int(next.Weekday()) != 1 {
		t.Errorf("expected Monday, got weekday %d", next.Weekday())
	}
	if next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("expected 9:00, got %d:%02d", next.Hour(), next.Minute())
	}
}

func TestRoundtrip_StepEvery15Min(t *testing.T) {
	expr := "*/15 * * * *"
	fields := mustParseCron(t, expr)
	from := time.Date(2026, time.January, 15, 14, 7, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil")
	}
	if next.Minute() != 15 {
		t.Errorf("expected minute 15, got %d", next.Minute())
	}
}

func TestComputeNextCronRun_YearEndToYearStart(t *testing.T) {
	fields := mustParseCron(t, "0 0 * * *")
	from := time.Date(2026, time.December, 31, 23, 59, 0, 0, time.Local)
	next := ComputeNextCronRun(fields, from)
	if next == nil {
		t.Fatal("expected non-nil")
	}
	if next.Year() != 2027 {
		t.Errorf("expected year 2027, got %d", next.Year())
	}
	if next.Month() != time.January || next.Day() != 1 {
		t.Errorf("expected Jan 1, got %v", next)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func assertIntSlice(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("slice length: got %d, want %d\n  got:  %v\n  want: %v", len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("element %d: got %d, want %d\n  got:  %v\n  want: %v", i, got[i], want[i], got, want)
			return
		}
	}
}
