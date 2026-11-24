import type { Change, Item } from "./bridge.js";

export function statusKey(item: Item, day: string) {
  return item.kind === "repeat" ? "status:" + day : "status";
}

export function phase(item: Item, day: string) {
  return item[statusKey(item, day)] || "Todo";
}

export function moveChanges(
  items: Record<string, Item>, day: string, id: string,
  destination: string, column: string[], beforeId?: string,
): Change[] {
  const ordered = column.filter((key) => key !== id);
  const index = beforeId ? ordered.indexOf(beforeId) : -1;
  ordered.splice(index < 0 ? ordered.length : index, 0, id);
  const changes: Change[] = [];
  const orderKey = "order:" + day;
  for (const [i, key] of ordered.entries()) {
    const item = items[key];
    if (!item) throw Error("일정 목록이 변경되었습니다. 다시 열어 주세요.");
    const order = String(i + 1);
    const movedToColumn = key === id && phase(item, day) !== destination;
    if (item[orderKey] === order && !movedToColumn) continue;
    const after = { ...item, [orderKey]: order };
    if (movedToColumn) after[statusKey(item, day)] = destination;
    changes.push({ ID: key, Before: item, After: after });
  }
  return changes;
}
