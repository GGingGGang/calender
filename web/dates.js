const fmt = (d) => [
    d.getFullYear(),
    String(d.getMonth() + 1).padStart(2, "0"),
    String(d.getDate()).padStart(2, "0"),
].join("-");
const date = (s) => new Date(s + "T12:00:00");
function shift(d, n) {
    const t = new Date(d);
    t.setDate(t.getDate() + n);
    return t;
}
function monday(d) {
    return shift(d, -((d.getDay() + 6) % 7));
}
const holidays = {
    "2026": "01-01 02-16 02-17 02-18 03-01 03-02 05-01 05-05 05-24 05-25 06-03 06-06 07-17 08-15 08-17 09-24 09-25 09-26 10-03 10-05 10-09 12-25",
    "2027": "01-01 02-06 02-07 02-08 02-09 03-01 05-01 05-03 05-05 05-13 06-06 07-17 07-19 08-15 08-16 09-14 09-15 09-16 10-03 10-04 10-09 10-11 12-25 12-27",
};
function rest(d) {
    return ([0, 6].includes(d.getDay()) ||
        holidays[d.getFullYear()]?.split(" ").includes(fmt(d).slice(5)));
}
export { fmt, date, shift, monday, rest };
