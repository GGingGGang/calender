export type Occurrence = { id: string; title: string };
type Segment = { start: number; end: number };
type WeekEvent = Occurrence & {
  days: number;
  count: number;
  segments: Segment[];
};

// Each bit is one weekday. Disjoint occurrences can share a lane, while
// separate dates of the same event stay aligned even across a gap.
export function layoutWeek(columns: Occurrence[][], maxLanes: number) {
  const groups = new Map<string, WeekEvent>();
  columns.forEach((items, day) => {
    for (const item of items) {
      const key = JSON.stringify([item.id, item.title]);
      let group = groups.get(key);
      if (!group) {
        group = { ...item, days: 0, count: 0, segments: [] };
        groups.set(key, group);
      }
      group.days |= 1 << day;
      group.count++;
      const last = group.segments.at(-1);
      if (last && last.end === day - 1) last.end = day;
      else group.segments.push({ start: day, end: day });
    }
  });

  const lanes: number[] = [];
  const hidden = columns.map(() => 0);
  const bars = [];
  for (const group of [...groups.values()].sort((a, b) => b.count - a.count)) {
    let lane = lanes.findIndex((used) => (used & group.days) === 0);
    if (lane === -1) lane = lanes.length;
    if (lane >= maxLanes) {
      columns.forEach((_, day) => {
        if (group.days & (1 << day)) hidden[day]++;
      });
      continue;
    }
    lanes[lane] = (lanes[lane] ?? 0) | group.days;
    // Label the longest run once; leave short or interrupted repeats marked
    // only on the dates they actually occur.
    const center = group.segments[0].start + group.segments.at(-1)!.end;
    const label = group.segments.reduce((a, b) => {
      const longer = b.end - b.start - (a.end - a.start);
      if (longer !== 0) return longer > 0 ? b : a;
      return Math.abs(b.start + b.end - center) <
        Math.abs(a.start + a.end - center)
        ? b
        : a;
    });
    for (const segment of group.segments) {
      bars.push({
        ...segment,
        lane,
        id: group.id,
        title: group.title,
        labeled: segment === label,
      });
    }
  }
  return { bars, hidden, laneCount: lanes.length };
}

const colors = [
  "#c7fa72",
  "#76e7ef",
  "#ecabff",
  "#ffb77f",
  "#9cbcff",
  "#ff9fbe",
];
export function eventColor(id: string) {
  let hash = 0;
  for (const ch of id) hash = (Math.imul(hash, 31) + ch.charCodeAt(0)) >>> 0;
  return colors[hash % colors.length];
}
