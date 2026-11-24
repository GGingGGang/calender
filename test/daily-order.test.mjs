import assert from "node:assert/strict";
import test from "node:test";
import { moveChanges } from "../web/daily-order.js";

const day = "2026-11-25";
const orderKey = "order:" + day;

test("moving a card one place sends only the two changed records", () => {
  const column = Array.from({ length: 100 }, (_, i) => String(i));
  const items = Object.fromEntries(column.map((id, i) => [
    id, Object.freeze({ kind: "once", title: id, [orderKey]: String(i + 1) }),
  ]));
  const changes = moveChanges(items, day, "50", "Todo", column, "49");
  assert.deepEqual(changes.map(c => [c.ID, c.After[orderKey]]), [["50", "50"], ["49", "51"]]);
  assert.equal(items["50"][orderKey], "51");
  assert.deepEqual(moveChanges(items, day, "50", "Todo", column, "51"), []);
});

test("moving a repeating card to Done changes only the selected day's status", () => {
  const item = { kind: "repeat", title: "회의", "status:2026-11-24": "Doing", [orderKey]: "1" };
  const changes = moveChanges({ a: item }, day, "a", "Done", []);
  assert.deepEqual(changes, [{ ID: "a", Before: item, After: { ...item, ["status:" + day]: "Done" } }]);
});
