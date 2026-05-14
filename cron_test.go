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

