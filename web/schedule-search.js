// An undefined date opens the event or the entire series in settings.
export function matchingDates(item, group, query) {
    if (!query)
        return [undefined];
    const matches = (text) => text.toLowerCase().includes(query);
    const series = [item.title, item.note || "", group].some(matches);
    const dates = new Set();
    if (item.kind === "repeat") {
        for (const [key, value] of Object.entries(item)) {
            if (!(key.startsWith("title:") || key.startsWith("note:")) || !matches(value))
                continue;
            const day = key.slice(key.indexOf(":") + 1);
            if (day < item.start || (item.until && day > item.until))
                continue;
            if (item["skip:" + day] !== "yes")
                dates.add(day);
        }
    }
    const sorted = [...dates].sort();
    return series ? [undefined, ...sorted] : sorted;
}
