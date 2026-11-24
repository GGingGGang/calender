package main

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestResolveFieldsAfterAnotherDeviceEdits(t *testing.T) {
	seed := calendarFixture(t, 1)
	remote := filepath.Join(t.TempDir(), "origin.git")
	command(t, seed.root, "clone", "--bare", seed.path("repo"), remote)
	device := func() *App {
		s := Store{t.TempDir()}
		command(t, s.root, "clone", remote, "repo")
		return &App{store: s, wake: make(chan struct{}, 1)}
	}
	sync := func(a *App) Snapshot {
		t.Helper()
		v, err := a.State(true)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	save := func(a *App, before, after Record) {
		t.Helper()
		if _, err := a.Save([]Change{{"0", before, after}}); err != nil {
			t.Fatal(err)
		}
	}
	a, b := device(), device()
	base := sync(a).Items["0"]
	sync(b)
	left, right := maps.Clone(base), maps.Clone(base)
	left["title"], left["note"] = "원격 제목", "원격 메모"
	right["title"], right["note"] = "로컬 제목", "로컬 메모"
	save(a, base, left)
	save(b, base, right)
	sync(a)
	conflict := sync(b)
	if len(conflict.Conflicts) != 2 {
		t.Fatal(conflict)
	}
	var choices []Resolution
	for _, f := range conflict.Conflicts {
		choices = append(choices, Resolution{Conflict: f, UseLocal: f.Field == "title"})
	}
	newer := maps.Clone(left)
	newer["note"] = "새 원격 메모"
	save(a, left, newer)
	sync(a)
	result, err := b.ResolveFields(choices)
	if err != nil || !result.Pending || len(result.Conflicts) != 1 || result.Conflicts[0].Remote != newer["note"] {
		t.Fatalf("stale choice should remain unresolved: %+v, %v", result, err)
	}
	b = &App{store: b.store}
	result, err = b.ResolveFields([]Resolution{{Conflict: result.Conflicts[0], UseLocal: false}})
	if err != nil || result.Message != "" || result.Pending {
		t.Fatal(result, err)
	}
	got := sync(a).Items["0"]
	if got["title"] != right["title"] || got["note"] != newer["note"] {
		t.Fatal(got)
	}
}

func command(t testing.TB, dir string, args ...string) string {
	t.Helper()
	v, e := git(dir, "", "", args...)
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func calendarFixture(t testing.TB, count int) Store {
	t.Helper()
	s := Store{t.TempDir()}
	dir := s.path("repo")
	command(t, s.root, "init", "--quiet", dir)
	events := filepath.Join(dir, "calender", "events")
	if err := os.MkdirAll(events, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < count; i++ {
		if err := atomicJSON(filepath.Join(events, strconv.Itoa(i)+".json"), Record{"title": "회의 🎯 " + strconv.Itoa(i), "kind": "once", "start": "2026-11-22"}); err != nil {
			t.Fatal(err)
		}
	}
	command(t, dir, "add", ".")
	command(t, dir, "commit", "-qm", "fixture")
	return s
}

func BenchmarkLoad100Events(b *testing.B) {
	s := calendarFixture(b, 100)
	for b.Loop() {
		items, e := s.itemsAt("HEAD")
		if e != nil || len(items) != 100 {
			b.Fatal(len(items), e)
		}
	}
}

func TestLoadCommittedCalendar(t *testing.T) {
	s := calendarFixture(t, 3)
	dir := s.path("repo")
	file := filepath.Join(dir, "calender", "events", "1.json")
	if err := os.WriteFile(file, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	records, e := s.itemsAt("HEAD")
	if e != nil || len(records) != 3 || records["1"]["title"] != "회의 🎯 1" {
		t.Fatal(records, e)
	}
	command(t, dir, "commit", "-qam", "malformed event")
	if _, e = s.itemsAt("HEAD"); e == nil || !strings.Contains(e.Error(), "1.json") {
		t.Fatalf("expected affected file in error, got %v", e)
	}
}

func TestIdleSyncReusesOnlyTheSameCommit(t *testing.T) {
	origin := calendarFixture(t, 1)
	s := Store{t.TempDir()}
	command(t, s.root, "clone", origin.path("repo"), "repo")
	trace := filepath.Join(t.TempDir(), "trace.log")
	t.Setenv("GIT_TRACE", filepath.ToSlash(trace))
	read := func(wantTitle string, expectBlobRead bool) {
		t.Helper()
		if err := os.WriteFile(trace, nil, 0600); err != nil {
			t.Fatal(err)
		}
		v, err := s.synchronize(false, time.Now(), nil)
		if err != nil || v.Message != "" || v.Items["0"]["title"] != wantTitle {
			t.Fatal(v, err)
		}
		log, err := os.ReadFile(trace)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(log), "cat-file --batch") != expectBlobRead {
			t.Fatalf("expected blob read %v, trace:\n%s", expectBlobRead, log)
		}
	}
	read("회의 🎯 0", true)
	read("회의 🎯 0", false)
	updated := Record{"kind": "once", "start": "2026-11-23", "title": "변경된 일정"}
	if err := atomicJSON(origin.path("repo/calender/events/0.json"), updated); err != nil {
		t.Fatal(err)
	}
	command(t, origin.path("repo"), "commit", "-qam", "changed on another device")
	read(updated["title"], true)
	read(updated["title"], false)
}
func TestTwoDevices(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "origin.git")
	command(t, t.TempDir(), "init", "--bare", "--initial-branch=main", remote)
	device := func() Store { s := Store{t.TempDir()}; command(t, s.root, "clone", remote, "repo"); return s }
	a, b := device(), device()
	apply := func(s Store, c ...Change) Snapshot {
		t.Helper()
		if len(c) > 0 {
			if _, e := s.save(c); e != nil {
				t.Fatal(e)
			}
		}
		v, e := s.synchronize(false, time.Now(), nil)
		if e != nil || v.Message != "" || v.Pending {
			t.Fatalf("sync failed: %+v %v", v, e)
		}
		return v
	}
	r := Record{"title": "보고서", "kind": "span", "start": "2026-12-28", "end": "2027-01-01"}
	apply(a)
	apply(a, Change{"first", nil, r})
	base := apply(b).Items["first"]
	if !reflect.DeepEqual(r, base) {
		t.Fatal(base)
	}
	clone := func(r Record) Record {
		n := Record{}
		for k, v := range r {
			n[k] = v
		}
		return n
	}
	done := clone(r)
	done["status"] = "Done"
	apply(a, Change{"first", r, done})
	named := clone(base)
	named["title"] = "보고서 B"
	v := apply(b, Change{"first", base, named})
	if v.Items["first"]["status"] != "Done" {
		t.Fatal(v)
	}
	other := clone(done)
	other["title"] = "보고서 A"
	if _, e := a.save([]Change{{"first", done, other}}); e != nil {
		t.Fatal(e)
	}
	v, e := a.synchronize(false, time.Now(), nil)
	if e != nil || len(v.Conflicts) != 1 || !v.Pending {
		t.Fatal(v, e)
	}
	restarted := Store{a.root}
	v, e = (&App{store: restarted}).Resolve(v.Conflicts, true)
	if e != nil || v.Message != "" || v.Pending || v.Items["first"]["title"] != "보고서 A" {
		t.Fatal(v, e)
	}
	command(t, b.path("repo"), "remote", "set-url", "--push", "origin", filepath.Join(t.TempDir(), "absent.git"))
	before := apply(b).Items["first"]
	after := clone(before)
	after["title"] = "재전송"
	if _, e := b.save([]Change{{"first", before, after}}); e != nil {
		t.Fatal(e)
	}
	v, e = b.synchronize(false, time.Now(), nil)
	if e != nil || !v.Pending || v.Message == "" {
		t.Fatal(v, e)
	}
	command(t, b.path("repo"), "remote", "set-url", "--push", "origin", remote)
	apply(Store{b.root})
	if apply(a).Items["first"]["title"] != "재전송" {
		t.Fatal("pending lost")
	}
	apply(a, Change{"first", after, nil})
	if len(apply(b).Items) != 0 {
		t.Fatal("delete not synced")
	}
	command(t, b.path("repo"), "remote", "set-url", "origin", filepath.Join(t.TempDir(), "offline"))
	v, e = b.synchronize(false, time.Now(), nil)
	if e != nil || v.Message == "" || v.Pending {
		t.Fatal(v, e)
	}
	if _, e = b.save([]Change{{"new", nil, r}}); e == nil {
		t.Fatal("offline edit persisted")
	}
}
func TestOccurrences(t *testing.T) {
	app := App{}
	r := Record{"kind": "repeat", "start": "2024-02-27", "interval": "2", "skip:2024-03-02": "yes"}
	days := app.Days(map[string]Record{"repeat": r}, "2024-02-25", 10)
	for s, want := range map[string]bool{"2024-02-25": false, "2024-02-27": true, "2024-02-28": false, "2024-02-29": true, "2024-03-02": false, "2024-03-04": true} {
		if (len(days[s]) > 0) != want {
			t.Fatal(s)
		}
	}
	r = Record{"kind": "span", "start": "2026-12-31", "end": "2027-01-01"}
	days = app.Days(map[string]Record{"span": r}, "2026-12-31", 3)
	if len(days) != 2 || len(days["2026-12-31"]) != 1 || len(days["2027-01-01"]) != 1 {
		t.Fatal("span boundaries")
	}
	r["end"] = r["start"]
	days = app.Days(map[string]Record{"span": r}, "2026-12-31", 3)
	if len(days) != 1 || len(days["2026-12-31"]) != 1 {
		t.Fatal("single day")
	}
}

func TestGroupLifecycleAcrossDevices(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "groups.git")
	command(t, t.TempDir(), "init", "--bare", "--initial-branch=main", remote)
	device := func() Store { s := Store{t.TempDir()}; command(t, s.root, "clone", remote, "repo"); return s }
	a, b := device(), device()
	sync := func(s Store, changes ...Change) Snapshot {
		t.Helper()
		if len(changes) > 0 {
			if _, e := s.save(changes); e != nil {
				t.Fatal(e)
			}
		}
		v, e := s.synchronize(false, time.Now(), nil)
		if e != nil || v.Message != "" {
			t.Fatal(e, v.Message)
		}
		return v
	}
	sync(a)
	const groupID = "group:work"
	group := Record{"kind": "group", "title": "업무", "color": "#76e7ef"}
	event := Record{"kind": "repeat", "start": "2026-11-22", "interval": "1", "title": "검토", "note": "함께 보관", "group": groupID}
	sync(a, Change{groupID, nil, group}, Change{"meeting", nil, event})
	items := sync(b).Items
	if !reflect.DeepEqual(items[groupID], group) || items["meeting"]["group"] != groupID {
		t.Fatal(items)
	}
	d := (&App{}).Days(items, "2026-11-22", 1)
	if !reflect.DeepEqual(d["2026-11-22"], []string{"meeting"}) {
		t.Fatal(d)
	}
	if path := command(t, remote, "ls-tree", "-r", "--name-only", "HEAD", "calender/groups"); path != "calender/groups/work.json" {
		t.Fatal(path)
	}
	renamed := Record{"kind": "group", "title": "프로젝트", "color": "#ff9fbe"}
	sync(b, Change{groupID, group, renamed})
	if got := sync(a).Items[groupID]; !reflect.DeepEqual(got, renamed) {
		t.Fatal(got)
	}
	ungrouped := Record{}
	for k, v := range event {
		if k != "group" {
			ungrouped[k] = v
		}
	}
	sync(a, Change{groupID, renamed, nil}, Change{"meeting", event, ungrouped})
	if got := sync(b).Items; len(got) != 1 || !reflect.DeepEqual(got["meeting"], ungrouped) {
		t.Fatal(got)
	}
}

func TestSyncSeriesAndOccurrenceNotes(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "origin.git")
	command(t, t.TempDir(), "init", "--bare", "--initial-branch=main", remote)
	device := func() Store { s := Store{t.TempDir()}; command(t, s.root, "clone", remote, "repo"); return s }
	a, b := device(), device()
	sync := func(s Store) Snapshot {
		t.Helper()
		v, e := s.synchronize(false, time.Now(), nil)
		if e != nil || v.Message != "" {
			t.Fatal(v.Message, e)
		}
		return v
	}
	save := func(s Store, before, after Record) {
		t.Helper()
		if _, e := s.save([]Change{{"repeat", before, after}}); e != nil {
			t.Fatal(e)
		}
	}
	sync(a)
	event := Record{"title": "산책", "kind": "repeat", "interval": "1", "start": "2026-11-22"}
	save(a, nil, event)
	sync(a)
	sync(b)
	general := Record{}
	daily := Record{}
	for k, v := range event {
		general[k] = v
		daily[k] = v
	}
	general["note"] = "공통 메모\n둘째 줄"
	const key = "note:2026-11-22"
	daily[key] = "오늘만 기록"
	save(a, event, general)
	save(b, event, daily)
	sync(a)
	sync(b)
	combined := sync(a).Items["repeat"]
	if combined["note"] != general["note"] || combined[key] != daily[key] {
		t.Fatal(combined)
	}
	save(a, combined, general)
	sync(a)
	if got := sync(b).Items["repeat"]; !reflect.DeepEqual(got, general) {
		t.Fatal(got)
	}
}

func TestBatchThresholdsAndRestart(t *testing.T) {
	remote := filepath.Join(t.TempDir(), "origin.git")
	command(t, t.TempDir(), "init", "--bare", "--initial-branch=main", remote)
	s := Store{t.TempDir()}
	command(t, s.root, "clone", remote, "repo")
	start := time.Now()
	syncAt := func(now time.Time) Snapshot {
		t.Helper()
		v, e := s.synchronize(true, now, nil)
		if e != nil || v.Message != "" {
			t.Fatalf("sync: %+v %v", v, e)
		}
		return v
	}
	syncAt(start)
	var before Record
	save := func(i int) {
		t.Helper()
		after := Record{"title": strconv.Itoa(i), "kind": "once", "start": "2026-12-01"}
		v, e := s.save([]Change{{"one", before, after}})
		if e != nil || !v.Pending || !reflect.DeepEqual(v.Items["one"], after) {
			t.Fatal(v, e)
		}
		before = after
	}
	for i := 1; i <= 9; i++ {
		save(i)
	}
	if v := syncAt(start.Add(9 * time.Minute)); v.PendingCount != 9 {
		t.Fatal(v)
	}
	if command(t, remote, "rev-list", "--all", "--count") != "0" {
		t.Fatal("published early")
	}
	s = Store{s.root}
	save(10)
	if v := syncAt(start.Add(9 * time.Minute)); v.PendingCount != 0 || v.Items["one"]["title"] != "10" {
		t.Fatal(v)
	}
	if command(t, remote, "rev-list", "--all", "--count") != "1" {
		t.Fatal("expected a single batch commit")
	}
	save(11)
	if v := syncAt(start.Add(19*time.Minute - time.Second)); !v.Pending {
		t.Fatal(v)
	}
	if v := syncAt(start.Add(19 * time.Minute)); v.Pending {
		t.Fatal(v)
	}
	syncAt(start.Add(29 * time.Minute))
	if command(t, remote, "rev-list", "--all", "--count") != "2" {
		t.Fatal("unexpected commit count")
	}
}

func TestLegacyPendingMigration(t *testing.T) {
	s := Store{t.TempDir()}
	remote := filepath.Join(t.TempDir(), "remote.git")
	command(t, t.TempDir(), "init", "--bare", "--initial-branch=main", remote)
	command(t, s.root, "clone", remote, "repo")
	item := Record{"kind": "once", "start": "2026-12-01", "title": "previous version"}
	if e := atomicJSON(s.path("pending.json"), []Change{{"old", nil, item}}); e != nil {
		t.Fatal(e)
	}
	v, e := s.snapshot()
	if e != nil || v.PendingCount != 1 || !reflect.DeepEqual(v.Items["old"], item) {
		t.Fatal(v, e)
	}
	v, e = s.synchronize(true, time.Now(), nil)
	if e != nil || v.Pending || v.Message != "" {
		t.Fatal(v, e)
	}
	v, e = s.snapshot()
	if e != nil || v.Pending || !reflect.DeepEqual(v.Items["old"], item) {
		t.Fatal(v, e)
	}
}
