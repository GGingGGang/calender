import assert from "node:assert/strict";
import test from "node:test";
import { date, dateColor, fmt, shift, startOfWeek } from "../web/dates.js";
import { matchingDates } from "../web/schedule-search.js";
import { layoutWeek } from "../web/week-bars.js";

test("weeks start on Sunday across December and January", () => {
  const start = startOfWeek(date("2027-01-01"));
  assert.equal(fmt(start), "2026-12-27");
  assert.equal(fmt(shift(start, 6)), "2027-01-02");
  assert.equal(fmt(startOfWeek(date("2027-01-03"))), "2027-01-03");
});

test("holiday red takes precedence over Saturday blue", () => {
  assert.equal(dateColor(date("2026-08-15")), "red");
  assert.equal(dateColor(date("2026-08-22")), "blue");
  assert.equal(dateColor(date("2026-08-23")), "red");
  assert.equal(dateColor(date("2026-08-24")), "");
});

test("completed occurrences stay in the same lane across status changes and gaps", () => {
  const active = { id: "series", title: "회의", done: false };
  const done = { ...active, done: true };
  const { bars, laneCount } = layoutWeek([[active], [active], [done], [], [done], [], []]);
  assert.equal(laneCount, 1);
  assert.deepEqual(bars.map(b => [b.start, b.end, b.lane, b.done, b.labeled]), [
    [0, 1, 0, false, true], [2, 2, 0, true, false], [4, 4, 0, true, false],
  ]);
});

test("a crowded day shows all 25 events even when their titles match", () => {
  const events = Array.from({ length: 25 }, (_, i) => ({
    id: String(i), title: "일정", done: i % 2 === 0,
  }));
  const { bars, laneCount } = layoutWeek([[], [], [], events, [], [], []]);
  assert.equal(laneCount, 25);
  assert.equal(new Set(bars.map(b => b.id)).size, 25);
  assert.ok(bars.every(b => b.start === 3 && b.end === 3 && b.labeled));
});

test("search distinguishes series results and dated matches and omits skipped dates", () => {
  const item = {
    kind: "repeat", title: "회의", note: "Agenda", start: "2026-11-01", until: "2026-11-25",
    "note:2026-11-26": "회의", "note:2026-10-31": "회의",
    "note:2026-11-21": "회의", "note:2026-11-16": "회의",
    "title:2026-11-16": "회의 변경", "note:2026-11-23": "회의", "skip:2026-11-23": "yes",
  };
  assert.deepEqual(matchingDates(item, "Work", "회의"), [undefined, "2026-11-16", "2026-11-21"]);
  assert.deepEqual(matchingDates(item, "Work", "agenda"), [undefined]);
  assert.deepEqual(matchingDates(item, "Work", "work"), [undefined]);
  assert.deepEqual(matchingDates(item, "Work", ""), [undefined]);
  assert.deepEqual(matchingDates(item, "Work", "2026"), []);
});
