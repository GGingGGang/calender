export function skippedDatesField(item) {
    const dates = Object.keys(item)
        .filter((key) => key.startsWith("skip:") && item[key] === "yes")
        .map((key) => key.slice(5))
        .sort();
    if (!dates.length)
        return null;
    const field = document.createElement("details");
    field.className = "skipped-dates";
    const summary = document.createElement("summary");
    summary.textContent = `건너뛴 회차 (${dates.length})`;
    field.append(summary);
    field.addEventListener("toggle", () => {
        if (!field.open || field.childElementCount > 1)
            return;
        const hint = document.createElement("p");
        hint.textContent = "복원할 날짜를 선택하고 저장하세요.";
        field.append(hint);
        for (const day of dates) {
            const label = document.createElement("label");
            const input = document.createElement("input");
            input.type = "checkbox";
            input.value = day;
            label.append(input, day);
            field.append(label);
        }
    });
    return field;
}
