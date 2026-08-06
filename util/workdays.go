package util

import "time"

// WorkingDaysInMonth counts Monday-Friday days in the given month. There is
// no holiday calendar in this system, so "working day" here means
// "weekday" — a deliberate simplification, not a claim every weekday is
// actually worked.
func WorkingDaysInMonth(year, month int) int {
	first := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	days := 0
	for d := first; d.Month() == first.Month(); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			days++
		}
	}
	return days
}
