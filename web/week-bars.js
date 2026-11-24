// Each bit is one weekday. Disjoint occurrences can share a lane, while
// separate dates of the same event stay aligned even across a gap.
export function layoutWeek(columns) {
    const groups = new Map();
    columns.forEach((items, day) => {
        for (const item of items) {
            const key = JSON.stringify([item.id, item.title]);
            let group = groups.get(key);
            if (!group) {
                group = { id: item.id, title: item.title, days: 0, count: 0, segments: [] };
                groups.set(key, group);
            }
            group.days |= 1 << day;
            group.count++;
            const last = group.segments.at(-1);
            if (last && last.end === day - 1 && last.done === item.done)
                last.end = day;
            else
                group.segments.push({ start: day, end: day, done: item.done });
        }
    });
    const lanes = [];
    const bars = [];
    for (const group of [...groups.values()].sort((a, b) => b.count - a.count)) {
        let lane = lanes.findIndex((used) => (used & group.days) === 0);
        if (lane === -1)
            lane = lanes.length;
        lanes[lane] = (lanes[lane] ?? 0) | group.days;
        // Label the longest run once; leave short or interrupted repeats marked
        // only on the dates they actually occur.
        const center = group.segments[0].start + group.segments.at(-1).end;
        const label = group.segments.reduce((a, b) => {
            const longer = b.end - b.start - (a.end - a.start);
            if (longer !== 0)
                return longer > 0 ? b : a;
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
    return { bars, laneCount: lanes.length };
}
const colors = [
    "#c7fa72",
    "#76e7ef",
    "#ecabff",
    "#ffb77f",
    "#9cbcff",
    "#ff9fbe",
];
export function eventColor(id) {
    let hash = 0;
    for (const ch of id)
        hash = (Math.imul(hash, 31) + ch.charCodeAt(0)) >>> 0;
    return colors[hash % colors.length];
}
function luminance(rgb) {
    const [r, g, b] = rgb.map((channel) => {
        const s = channel / 255;
        return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
    });
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}
export function bandTextColor(color) {
    const rgb = [1, 3, 5].map((i) => parseInt(color.slice(i, i + 2), 16));
    // Matches .event-band's 20% event color mixed into #1b211d.
    const background = [27, 33, 29].map((base, i) => 0.8 * base + 0.2 * rgb[i]);
    return (luminance(rgb) + 0.05) / (luminance(background) + 0.05) >= 4.5
        ? color : "var(--ink)";
}
