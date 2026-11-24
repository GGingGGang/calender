package main

import (
	"slices"
	"strconv"
	"testing"
)

func TestVisibleDatesAcrossYearBoundary(t *testing.T) {
	app := App{}
	items := map[string]Record{
		"once":    {"kind": "once", "start": "2027-01-01"},
		"skipped": {"kind": "once", "start": "2027-01-01", "skip:2027-01-01": "yes"},
		"past":    {"kind": "span", "start": "2020-01-01", "end": "2026-12-30"},
		"future":  {"kind": "once", "start": "2027-01-03"},
		"span":    {"kind": "span", "start": "2026-12-20", "end": "2027-01-05", "skip:2027-01-01": "yes"},
		"repeat":  {"kind": "repeat", "start": "2026-12-29", "interval": "2"},
	}
	days := app.Days(items, "2026-12-31", 3)
	want := map[string][]string{
		"2026-12-31": {"repeat", "span"},
		"2027-01-01": {"once"},
		"2027-01-02": {"repeat", "span"},
	}
	if len(days) != len(want) {
		t.Fatal(days)
	}
	for date, ids := range want {
		slices.Sort(days[date])
		if !slices.Equal(days[date], ids) {
			t.Errorf("%s: got %v, want %v", date, days[date], ids)
		}
	}
}

func BenchmarkMonthWithHistory(b *testing.B) {
	items := make(map[string]Record, 10000)
	for i := range 10000 {
		event := Record{"kind": "once", "start": "2024-01-01"}
		if i%100 == 0 {
			event = Record{"kind": "repeat", "start": "2024-01-01", "interval": "3"}
		}
		items[strconv.Itoa(i)] = event
	}
	app := App{}
	b.ReportAllocs()
	for b.Loop() {
		app.Days(items, "2026-11-01", 42)
	}
}
