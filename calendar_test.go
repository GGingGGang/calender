package main

import (
	"maps"
	"slices"
	"strconv"
	"testing"
)

func TestRestoreOneOccurrenceWithRemoteNotes(t *testing.T) {
	base := Record{
		"title": "수업", "kind": "repeat", "start": "2026-12-01", "interval": "1",
		"skip:2026-12-02": "yes", "skip:2026-12-03": "yes",
		"status:2026-12-02": "Done", "note:2026-12-02": "기존 메모",
	}
	local, remote := maps.Clone(base), maps.Clone(base)
	delete(local, "skip:2026-12-02")
	remote["note:2026-12-02"] = "다른 기기의 메모"
	result, conflicts := merge(Change{"series", base, local}, remote)
	if len(conflicts) != 0 || result["note:2026-12-02"] != remote["note:2026-12-02"] ||
		result["status:2026-12-02"] != "Done" {
		t.Fatal(result, conflicts)
	}
	days := (&App{}).Days(map[string]Record{"series": result}, "2026-12-02", 2)
	if !slices.Equal(days["2026-12-02"], []string{"series"}) || len(days["2026-12-03"]) != 0 {
		t.Fatal(days)
	}
}

func TestRepeatEndDate(t *testing.T) {
	app := App{}
	r := Record{"kind": "repeat", "title": "수업", "start": "2024-02-27", "interval": "2"}
	for _, tc := range []struct {
		until string
		dates []string
	}{
		{"2024-02-27", []string{"2024-02-27"}},
		{"2024-02-29", []string{"2024-02-27", "2024-02-29"}},
		{"2024-03-01", []string{"2024-02-27", "2024-02-29"}},
		{"", []string{"2024-02-27", "2024-02-29", "2024-03-02"}},
	} {
		r["until"] = tc.until
		if err := validate(r); err != nil {
			t.Fatal(err)
		}
		var got []string
		for key := range app.Days(map[string]Record{"i": r}, "2024-02-26", 7) {
			got = append(got, key)
		}
		slices.Sort(got)
		if !slices.Equal(got, tc.dates) {
			t.Fatalf("until %q: got %v, want %v", tc.until, got, tc.dates)
		}
	}
	for _, bad := range []string{"2024-02-26", "2024-02-30"} {
		r["until"] = bad
		if validate(r) == nil {
			t.Fatalf("accepted invalid end date %q", bad)
		}
	}
}

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
