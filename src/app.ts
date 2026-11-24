import { api, type Item, type State, type Change } from "./bridge.js";
import { fmt, date, shift, startOfWeek, weekdayColor, dateColor } from "./dates.js";
import { eventColor, layoutWeek } from "./week-bars.js";
const el = <T extends HTMLElement = HTMLElement>(selector: string) =>
  document.querySelector<T>(selector)!;
const toolbar = el("#toolbar"),
  content = el("#content"),
  nav = el("nav"),
  modal = el<HTMLDialogElement>("dialog"),
  form = el<HTMLFormElement>("#editor");
const phases = ["Todo", "Doing", "Done"];
const views = ["Daily", "Weekly", "Monthly"] as const;
const groupColors = {
  "#c7fa72": "라임",
  "#76e7ef": "민트",
  "#ecabff": "라일락",
  "#ffb77f": "살구",
  "#9cbcff": "하늘",
  "#ff9fbe": "로즈",
};
let groupFilter = "*";
let settingsQuery = "";
function groups() {
  return Object.entries(state.Items || {})
    .filter(([, r]) => r.kind === "group")
    .sort((a, b) => a[1].title.localeCompare(b[1].title, "ko"));
}
function groupFor(item: Item) {
  const group = state.Items?.[item.group];
  return group?.kind === "group" ? group : undefined;
}
function inGroup(item: Item) {
  return (
    groupFilter === "*" || (groupFor(item) ? item.group : "") === groupFilter
  );
}
function groupSelect(value: string, all: boolean) {
  const entries = groups();
  return select(
    [...(all ? ["*"] : []), "", ...entries.map(([id]) => id)],
    [
      ...(all ? ["전체 그룹"] : []),
      "미분류",
      ...entries.map(([, r]) => r.title),
    ],
    value,
  );
}
let selected = new Date(),
  page: (typeof views)[number] = "Daily",
  tabThroughControls = false,
  settings = false,
  repositoryDraft = "",
  busy = false,
  drag = "",
  revision = 0;
let state: State = {
  URL: "",
  Last: "",
  Message: "",
  Pending: false,
  PendingCount: 0,
  Conflicts: null,
  Items: {},
};
function node(tag: string, text = "", cls = "") {
  const n = document.createElement(tag);
  n.textContent = text;
  n.className = cls;
  return n;
}
function button(text: string, fn: () => unknown, cls = "", label = "") {
  const b = document.createElement("button");
  b.type = "button";
  b.textContent = text;
  b.className = cls;
  if (label) {
    b.setAttribute("aria-label", label);
    b.title = label;
  }
  b.onclick = () => {
    Promise.resolve().then(fn).catch(report);
  };
  return b;
}
function report(e: unknown) {
  el("#status").textContent = String(e);
  if (modal.open) el("#error").textContent = String(e);
}
async function transact(fn: () => Promise<State>) {
  if (busy) throw Error("동기화 중입니다. 잠시 후 다시 시도하세요.");
  busy = true;
  try {
    state = await fn();
    await render();
    return !state.Message && !state.Pending;
  } finally {
    busy = false;
  }
}
async function refresh() {
  const refreshButton = el<HTMLButtonElement>("#refresh");
  if (busy) return;
  refreshButton.disabled = true;
  refreshButton.textContent = "동기화 중…";
  try {
    return await transact(() => api.State(true));
  } finally {
    refreshButton.disabled = false;
    refreshButton.textContent = "동기화";
  }
}
async function gate() {
  state = await api.State(false);
  if (!state.Last || state.Message || !state.URL)
    throw Error(state.Message || "저장소 연결을 확인하세요.");
}
const idate = () => fmt(selected);
function statusKey(r: Item, d: string) {
  return r.kind === "repeat" ? "status:" + d : "status";
}
function phase(r: Item, d: string) {
  return r[statusKey(r, d)] || "Todo";
}
function title(r: Item, d: string) {
  return r["title:" + d] || r.title;
}
function idsFor(ids: string[], d: string) {
  return ids.sort((a, b) => {
    const x = state.Items![a],
      y = state.Items![b];
    return (
      Number(x["order:" + d] || 0) - Number(y["order:" + d] || 0) ||
      a.localeCompare(b)
    );
  });
}
function movePointer(n: number) {
  if (page === "Monthly") {
    selected = new Date(selected.getFullYear(), selected.getMonth() + n, 1);
  } else selected = shift(selected, n * (page === "Weekly" ? 7 : 1));
  return render();
}
function navigateWithKeyboard(e: KeyboardEvent) {
  if (
    settings || modal.open || e.defaultPrevented || e.isComposing ||
    e.altKey || e.ctrlKey || e.metaKey
  ) return;
  if (e.key === "F6" && !e.shiftKey) {
    e.preventDefault();
    if (e.repeat) return;
    tabThroughControls = !tabThroughControls;
    el(tabThroughControls ? "#previous-period" : "#view-" + page).focus();
    return;
  }
  if (e.target instanceof HTMLElement &&
      (e.target.isContentEditable || e.target.closest("input, textarea, select"))) return;
  if (e.key === "Tab" && !tabThroughControls) {
    e.preventDefault();
    const next = views.indexOf(page) + (e.shiftKey ? -1 : 1);
    page = views[(next + views.length) % views.length];
    void render(true).catch(report);
    return;
  }
  if (e.shiftKey || (e.key !== "ArrowLeft" && e.key !== "ArrowRight")) return;
  e.preventDefault();
  void movePointer(e.key === "ArrowLeft" ? -1 : 1).catch(report);
}
function dateInput(type: string, value: string) {
  const i = document.createElement("input");
  i.type = type;
  i.value = value;
  return i;
}
async function render(focusView = false) {
  const token = ++revision;
  const previousFocus = document.activeElement?.id;
  const restoreFocus = () => {
    if (
      previousFocus &&
      document.activeElement === document.body &&
      !modal.open
    )
      document.getElementById(previousFocus)?.focus();
  };
  nav.replaceChildren();
  toolbar.replaceChildren();
  content.replaceChildren();
  el("#settings").hidden = settings;
  el("#quit").hidden = settings;
  el("body").classList.toggle("settings", settings);
  el("#status").textContent =
    (state.Pending
      ? `로컬 저장됨 · 전송 대기 ${state.PendingCount}건 · `
      : "") +
    (state.Message ||
      (state.Last
        ? "동기화 " + new Date(state.Last).toLocaleTimeString("ko-KR")
        : "저장소 연결 필요"));
  if (state.Conflicts?.length)
    toolbar.append(button("충돌 해결", resolveForm, "danger"));
  if (settings) {
    renderSettings();
    restoreFocus();
    return;
  }
  for (const p of views) {
    const view = button(
      p,
      () => {
        page = p;
        return render();
      },
      p === page ? "active" : "",
    );
    view.id = "view-" + p;
    if (p === page) view.setAttribute("aria-current", "page");
    nav.append(view);
  }
  if (focusView) el("#view-" + page).focus();
  const d = idate(),
    today = fmt(new Date());
  const weekStart = startOfWeek(selected);
  const label =
    page === "Monthly"
      ? selected.toLocaleDateString("ko-KR", { year: "numeric", month: "long" })
      : page === "Weekly"
        ? fmt(weekStart) +
          " — " +
          fmt(shift(weekStart, 6)).slice(5)
        : selected.toLocaleDateString("ko-KR", {
            month: "long",
            day: "numeric",
            weekday: "long",
          });
  const picker = dateInput("date", d);
  picker.className = "picker";
  picker.tabIndex = -1;
  picker.setAttribute("aria-label", "날짜 선택");
  picker.onchange = () => {
    if (picker.value) {
      selected = date(picker.value);
      void render();
    }
  };
  const period = { Daily: "날짜", Weekly: "주", Monthly: "달" }[page] || "날짜";
  const previous = button("‹", () => movePointer(-1), "", "이전 " + period);
  previous.id = "previous-period";
  previous.setAttribute("aria-keyshortcuts", "ArrowLeft");
  previous.title += " (←)";
  const next = button("›", () => movePointer(1), "", "다음 " + period);
  next.id = "next-period";
  next.setAttribute("aria-keyshortcuts", "ArrowRight");
  next.title += " (→)";
  toolbar.append(
    previous,
    button(
      label,
      () => picker.showPicker(),
      page === "Daily" ? ("date " + dateColor(selected)).trimEnd() : "date",
    ),
    picker,
    next,
    button(
      "오늘",
      () => {
        selected = new Date();
        return render();
      },
      "today",
    ),
  );
  if (groupFilter && groupFilter !== "*" && !state.Items?.[groupFilter])
    groupFilter = "*";
  if (groups().length) {
    const filter = groupSelect(groupFilter, true);
    filter.id = "group-filter";
    filter.setAttribute("aria-label", "표시할 그룹");
    filter.onchange = () => {
      groupFilter = filter.value;
      void render().catch(report);
    };
    toolbar.append(filter);
  }
  const monthStart = new Date(selected.getFullYear(), selected.getMonth(), 1);
  const start =
    page === "Daily"
      ? selected
      : page === "Weekly"
        ? weekStart
        : startOfWeek(monthStart);
  const monthEnd = new Date(selected.getFullYear(), selected.getMonth() + 1, 0);
  const n =
    page === "Daily"
      ? 1
      : page === "Weekly"
        ? 7
        : 7 * Math.ceil((monthStart.getDay() + monthEnd.getDate()) / 7);
  const days = await api.Days(state.Items || {}, fmt(start), n);
  if (token !== revision) return;
  if (page === "Daily") {
    const board = node("div", "", "board");
    content.append(board);
    for (const p of phases) {
      const col = node("section", "", "column " + p);
      const allIds = idsFor(days[d] || [], d).filter(
        (id) => phase(state.Items![id], d) === p,
      );
      const list = allIds.filter((id) => inGroup(state.Items![id]));
      const h = node("h3", p);
      h.append(node("small", String(list.length)));
      col.append(h);
      if (p === "Todo") {
        const add = button("＋", () => edit(), "add", "Todo 일정 추가");
        add.id = "add-event";
        h.append(add);
      }
      for (const id of list) {
        const r = state.Items![id],
          card = button("", () => edit(id), "card");
        card.append(node("span", title(r, d), "card-title"));
        const group = groupFor(r);
        if (group) {
          const tag = node("small", group.title, "group-tag");
          tag.style.setProperty("--group-color", group.color);
          card.append(tag);
        }
        const notePreview = [r.note, r.kind === "repeat" ? r["note:" + d] : ""]
          .filter(Boolean)
          .join(" · ");
        if (notePreview) card.append(node("span", notePreview, "card-note"));
        if (r.kind !== "once")
          card.append(
            node(
              "small",
              r.kind === "repeat"
                ? r.interval + "일마다"
                : r.end + (r.deadline ? " · " + r.deadline : ""),
            ),
          );
        card.draggable = true;
        card.dataset.id = id;
        card.id = "event-" + id;
        card.title = "Alt+↑ 또는 Alt+↓로 같은 열 안에서 순서 변경";
        card.setAttribute("aria-keyshortcuts", "Alt+ArrowUp Alt+ArrowDown");
        card.onkeydown = (e) => {
          if (
            !e.altKey ||
            e.ctrlKey ||
            e.shiftKey ||
            e.metaKey ||
            (e.key !== "ArrowUp" && e.key !== "ArrowDown")
          )
            return;
          e.preventDefault();
          const index = list.indexOf(id);
          const down = e.key === "ArrowDown";
          if (
            busy ||
            e.repeat ||
            (down ? index === list.length - 1 : index === 0)
          )
            return;
          void move(id, p, allIds, list[down ? index + 2 : index - 1]).catch(
            report,
          );
        };
        card.ondragstart = (e) => {
          drag = id;
          e.dataTransfer?.setData("text/plain", id);
        };
        card.ondragend = () => {
          drag = "";
        };
        col.append(card);
      }
      col.ondragover = (e) => e.preventDefault();
      col.ondrop = (e) => {
        e.preventDefault();
        const id = drag;
        const target = (e.target as HTMLElement).closest<HTMLElement>(
          "[data-id]",
        )?.dataset.id;
        if (id && id !== target) void move(id, p, allIds, target).catch(report);
      };
      board.append(col);
    }
  } else {
    const grid = node("div", "", "calendar " + page);
    content.append(grid);
    for (const [day, w] of ["일", "월", "화", "수", "목", "금", "토"].entries()) {
      const color = page === "Weekly" ? dateColor(shift(start, day)) : weekdayColor(day);
      grid.append(node("small", w, ("weekday " + color).trimEnd()));
    }
    const currentWeekStart = fmt(startOfWeek(new Date()));
    for (let i = 0; i < n; i += 7) {
      const week = node("div", "", "calendar-week");
      const dates = Array.from({ length: 7 }, (_, day) =>
        shift(start, i + day),
      );
      const columns = dates.map((t) => {
        const key = fmt(t);
        return idsFor(
          (days[key] || []).filter((id) => {
            const item = state.Items![id];
            return (
              inGroup(item) &&
              (item.kind !== "repeat" || item["hide:" + page] !== "yes")
            );
          }),
          key,
        ).map((id) => {
          const item = state.Items![id];
          return { id, title: title(item, key), done: phase(item, key) === "Done" };
        });
      });
      const { bars, laneCount } = layoutWeek(columns);
      week.style.gridTemplateRows = [
        "28px",
        ...Array(laneCount).fill("22px"),
        "minmax(4px, 1fr)",
      ].join(" ");
      dates.forEach((t, day) => {
        const key = fmt(t);
        const cell = button(
          "",
          () => {
            selected = t;
            page = "Daily";
            return render();
          },
          "cell" +
            (fmt(dates[0]) === currentWeekStart ? " currentweek" : "") +
            (key === today ? " current" : "") +
            (t.getMonth() !== selected.getMonth() && page === "Monthly"
              ? " muted"
              : ""),
        );
        cell.append(node("b", String(t.getDate()), dateColor(t)));
        cell.style.gridColumn = String(day + 1);
        cell.style.gridRow = "1 / -1";
        const labels = columns[day].map((item) => item.done ? item.title + " (완료)" : item.title);
        cell.title = labels.join("\n");
        cell.setAttribute(
          "aria-label",
          [key + " Daily 열기", ...labels].join(", "),
        );
        week.append(cell);
      });
      for (const bar of bars) {
        const band = node("span", bar.labeled ? bar.title : "", bar.done ? "event-band done" : "event-band");
        band.style.gridColumn = bar.start + 1 + " / " + (bar.end + 2);
        band.style.gridRow = String(bar.lane + 2);
        band.style.setProperty(
          "--event-color",
          bar.done ? "var(--muted)" : groupFor(state.Items![bar.id])?.color || eventColor(bar.id),
        );
        band.setAttribute("aria-hidden", "true");
        week.append(band);
      }
      grid.append(week);
    }
  }
  restoreFocus();
}
async function move(id: string, p: string, old: string[], target?: string) {
  const d = idate(),
    before = structuredClone(state.Items || {});
  await gate();
  const list = old.filter((x) => x !== id);
  list.splice(
    target && list.includes(target) ? list.indexOf(target) : list.length,
    0,
    id,
  );
  const changes: Change[] = list.map((key, i) => {
    const r = before[key],
      after = { ...r, ["order:" + d]: String(i + 1) };
    if (key === id) after[statusKey(r, d)] = p;
    return { ID: key, Before: r, After: after };
  });
  await transact(() => api.Save(changes));
}
function renderSettings() {
  nav.append(node("b", "설정"));
  const add = button("＋ 일정 추가", () => edit(), "primary");
  add.id = "add-event";
  toolbar.append(node("h2", "일정 관리"), add);
  const conn = node("section", "", "connection");
  conn.append(node("small", "GITHUB 저장소"));
  if (state.URL) conn.append(node("p", state.URL));
  else {
    const url = dateInput("url", repositoryDraft);
    url.setAttribute("aria-label", "GitHub 저장소 HTTPS 주소");
    url.oninput = () => {
      repositoryDraft = url.value;
    };
    url.placeholder = "https://github.com/owner/repository";
    conn.append(
      url,
      button("연결", async () => {
        await api.Connect(url.value.trim());
        await refresh();
      }),
    );
  }
  content.append(conn);
  const management = node("section", "", "group-management");
  const groupHeader = node("div", "", "group-header");
  groupHeader.append(
    node("h3", "그룹"),
    button("＋ 그룹 추가", () => editGroup()),
  );
  management.append(groupHeader);
  for (const [id, group] of groups()) {
    const chip = button(group.title, () => editGroup(id), "group-chip");
    chip.id = id;
    chip.style.setProperty("--group-color", group.color);
    management.append(chip);
  }
  content.append(management);
  const day = dateInput("date", idate());
  day.onchange = () => {
    if (day.value) {
      selected = date(day.value);
      void render();
    }
  };
  const l = node("label", "회차 편집 기준일 ");
  l.append(day);
  content.append(l);
  const search = dateInput("search", settingsQuery);
  search.id = "event-search";
  search.placeholder = "제목, 메모, 그룹으로 검색";
  search.setAttribute("aria-label", "일정 검색");
  const list = node("div");
  const entries = Object.entries(state.Items || {})
    .filter(([, item]) => item.kind !== "group")
    .sort((a, b) => a[1].start.localeCompare(b[1].start));
  const renderList = () => {
    const query = settingsQuery.trim().toLowerCase();
    const rows = document.createDocumentFragment();
    for (const [id, r] of entries) {
      const group = groupFor(r)?.title || "";
      for (const occurrence of matchingDates(r, group, query)) {
        const row = button("", () => edit(id, occurrence), "row");
        row.id = occurrence ? `event-${id}:${occurrence}` : "event-" + id;
        row.append(
          node("b", occurrence ? title(r, occurrence) : r.title),
          node("small", [
            occurrence || r.start,
            occurrence ? "이 날짜만" : { once: "단일", repeat: "반복", span: "기간" }[r.kind],
            group,
          ].filter(Boolean).join(" · ")),
        );
        rows.append(row);
      }
    }
    if (query && !rows.childElementCount) rows.append(node("p", "검색 결과가 없습니다."));
    list.replaceChildren(rows);
  };
  search.oninput = () => {
    settingsQuery = search.value;
    renderList();
  };
  content.append(search, list);
  renderList();
}
function matchingDates(item: Item, group: string, query: string): (string | undefined)[] {
  if (!query) return [undefined];
  const matches = (text: string) => text.toLowerCase().includes(query);
  const series = [item.title, item.note || "", group].some(matches);
  const dates = new Set<string>();
  if (item.kind === "repeat") {
    for (const [key, value] of Object.entries(item)) {
      if (!(key.startsWith("title:") || key.startsWith("note:")) || !matches(value)) continue;
      const day = key.slice(key.indexOf(":") + 1);
      if (item["skip:" + day] !== "yes") dates.add(day);
    }
  }
  const sorted = [...dates].sort();
  return series ? [undefined, ...sorted] : sorted;
}
function field(label: string, input: HTMLElement) {
  const l = node("label", label);
  l.append(input);
  form.append(l);
  return l;
}
function select(opts: string[], labels: string[], v: string) {
  const s = document.createElement("select");
  opts.forEach((o, i) => s.add(new Option(labels[i], o)));
  s.value = v;
  return s;
}
function begin(name: string) {
  const heading = node("h2", name);
  heading.id = "dialog-title";
  modal.setAttribute("aria-labelledby", heading.id);
  form.replaceChildren(heading);
  form.onsubmit = (e) => e.preventDefault();
  if (!modal.open) modal.showModal();
}
function actions(...buttons: HTMLElement[]) {
  const e = node("p", "", "red");
  e.id = "error";
  e.setAttribute("role", "alert");
  const bar = node("div", "", "actions");
  bar.append(
    button("닫기", () => modal.close()),
    ...buttons,
  );
  form.append(e, bar);
}
async function editGroup(id?: string) {
  await gate();
  const before = id ? state.Items?.[id] || null : null;
  if (id && !before) throw Error("이미 삭제된 그룹입니다.");
  begin(id ? "그룹 편집" : "그룹 추가");
  const name = dateInput("text", before?.title || "");
  name.required = true;
  const color = select(
    Object.keys(groupColors),
    Object.values(groupColors),
    before?.color || Object.keys(groupColors)[0],
  );
  field("그룹 이름", name);
  field("그룹 색상", color);
  const save = async (remove: boolean) => {
    if (busy) return;
    const key = id || "group:" + crypto.randomUUID();
    const title = name.value.trim();
    if (
      !remove &&
      (!title ||
        groups().some(([other, g]) => other !== key && g.title === title))
    )
      throw Error("비어 있지 않은 고유한 그룹 이름을 입력하세요.");
    if (
      remove &&
      !confirm("이 그룹을 삭제할까요? 소속 일정은 미분류로 보관합니다.")
    )
      return;
    const changes: Change[] = [
      {
        ID: key,
        Before: before,
        After: remove ? null : { kind: "group", title, color: color.value },
      },
    ];
    if (remove)
      for (const [eventID, item] of Object.entries(state.Items || {})) {
        if (item.group === key) {
          const after = { ...item };
          delete after.group;
          changes.push({ ID: eventID, Before: item, After: after });
        }
      }
    form.inert = true;
    modal.oncancel = (e) => e.preventDefault();
    try {
      await transact(() => api.Save(changes));
      modal.close();
      document.getElementById(key)?.focus();
    } finally {
      form.inert = false;
      modal.oncancel = null;
    }
  };
  const submit = button("저장", () => {}, "primary");
  submit.type = "submit";
  form.onsubmit = (e) => {
    e.preventDefault();
    void save(false).catch(report);
  };
  actions(
    ...(id ? [button("그룹 삭제", () => save(true), "danger")] : []),
    submit,
  );
  name.focus();
}
async function edit(id?: string, occurrence?: string) {
  await gate();
  const editBase = id ? structuredClone(state.Items?.[id] || null) : null;
  if (id && !editBase) throw Error("이미 삭제된 일정입니다.");
  const r = editBase || { kind: "once", start: idate(), title: "" },
    d = occurrence || idate();
  const canOne =
    r.kind === "repeat" && id && (await api.Days({ [id]: r }, d, 1))[d]?.length;
  if (occurrence && !canOne) throw Error("현재 반복 규칙에 포함되지 않는 날짜입니다.");
  begin(occurrence ? d + " 일정 편집" : id ? "일정 편집" : "일정 추가");
  const scope = select(
    ["all", "one"],
    ["반복 전체", "이 날짜만"],
    settings && !occurrence ? "all" : "one",
  );
  if (canOne) field("편집 범위", scope);
  const kind = select(
      ["once", "repeat", "span"],
      ["단일 일정", "반복 일정", "기간 일정"],
      r.kind,
    ),
    name = dateInput("text", r.title),
    start = dateInput("date", r.start),
    end = dateInput("date", r.end || d),
    interval = dateInput("number", r.interval || "1"),
    deadline = dateInput("time", r.deadline || "");
  interval.min = "1";
  name.required = true;
  field("제목", name);
  const commonNote = node("div", "", "common-note");
  commonNote.append(node("small", "반복 전체 메모"), node("p", r.note || ""));
  form.append(commonNote);
  const note = document.createElement("textarea");
  note.rows = 3;
  const noteCaption = node("span", "메모");
  const noteField = node("label", "", "note-field");
  noteField.append(noteCaption, note);
  form.append(noteField);
  field("종류", kind);
  const group = groupSelect(
    id ? (groupFor(r) ? r.group : "") : groupFilter === "*" ? "" : groupFilter,
    false,
  );
  const groupField = field("그룹", group);
  field("시작일", start);
  field("반복 간격 (일)", interval);
  field("종료일", end);
  field("마감 시각 (선택)", deadline);
  const visibility = document.createElement("fieldset");
  visibility.className = "visibility";
  visibility.append(node("legend", "반복 일정 표시"));
  const views = ["Weekly", "Monthly"].map((view) => {
    const input = document.createElement("input");
    input.type = "checkbox";
    input.checked = r["hide:" + view] !== "yes";
    const label = node("label");
    label.append(input, view + "에 표시");
    visibility.append(label);
    return { view, input };
  });
  form.append(visibility);
  const update = () => {
    const one = !!canOne && scope.value === "one";
    groupField.hidden = one || !groups().length;
    kind.disabled = !!id;
    start.disabled = one;
    interval.disabled = one || kind.value !== "repeat";
    end.disabled = deadline.disabled = kind.value !== "span";
    visibility.hidden = kind.value !== "repeat" || one;
    interval.parentElement!.hidden = kind.value !== "repeat";
    end.parentElement!.hidden = deadline.parentElement!.hidden =
      kind.value !== "span";
  };
  update();
  const titleDrafts: Record<string, string> = {
    one: title(r, d),
    all: r.title,
  };
  const noteDrafts: Record<string, string> = {
    one: r["note:" + d] || "",
    all: r.note || "",
  };
  let editScope = canOne ? scope.value : "all";
  name.value = titleDrafts[editScope];
  const showNote = () => {
    note.value = noteDrafts[editScope];
    noteCaption.textContent =
      editScope === "one"
        ? "이 날짜의 메모"
        : kind.value === "repeat"
          ? "반복 전체 메모"
          : "메모";
    commonNote.hidden = editScope !== "one" || !r.note;
  };
  showNote();
  const progress = select(phases, phases, phase(r, d));
  const progressLabel = field(
    r.kind === "span" ? "진행 상태 (기간 전체)" : "진행 상태",
    progress,
  );
  const showProgress = () => {
    progressLabel.hidden =
      !id || settings || (r.kind === "repeat" && editScope !== "one");
  };
  showProgress();
  scope.onchange = () => {
    titleDrafts[editScope] = name.value;
    noteDrafts[editScope] = note.value;
    editScope = scope.value;
    name.value = titleDrafts[editScope];
    update();
    showProgress();
    showNote();
  };
  kind.onchange = () => {
    update();
    noteDrafts[editScope] = note.value;
    showNote();
  };
  const save = async (del: boolean) => {
    const one = canOne && scope.value === "one";
    let after: Item | null = { ...r };
    if (del) {
      if (one) after["skip:" + d] = "yes";
      else after = null;
    } else if (one) {
      if (!name.value.trim()) throw Error("제목을 입력하세요.");
      if (name.value.trim() !== title(r, d))
        after["title:" + d] = name.value.trim();
    } else {
      Object.assign(after, {
        title: name.value.trim(),
        kind: kind.value,
        start: start.value,
      });
      if (group.value) after.group = group.value;
      else delete after.group;
      if (kind.value === "repeat") {
        after.interval = interval.value;
        for (const { view, input } of views) {
          if (input.checked) delete after["hide:" + view];
          else after["hide:" + view] = "yes";
        }
      }
      if (kind.value === "span") {
        after.end = end.value;
        after.deadline = deadline.value;
      }
    }
    if (after && !progressLabel.hidden && progress.value !== phase(r, d)) {
      after[statusKey(r, d)] = progress.value;
    }
    if (after && !del) {
      const memoKey = one ? "note:" + d : "note";
      if (note.value.trim()) after[memoKey] = note.value;
      else delete after[memoKey];
    }
    if (busy) return;
    submit.textContent = del ? "삭제 중…" : "저장 중…";
    form.inert = true;
    modal.oncancel = (e) => e.preventDefault();
    try {
      const eventID = id || crypto.randomUUID();
      const ok = await transact(() =>
        api.Save([{ ID: eventID, Before: editBase, After: after }]),
      );
      if (ok || state.Pending) {
        modal.close();
        (
          (occurrence && document.getElementById(`event-${eventID}:${occurrence}`)) ||
          document.getElementById("event-" + eventID) ||
          (settingsQuery && document.getElementById("event-search")) ||
          document.getElementById("add-event")
        )?.focus();
      } else report(state.Message);
    } finally {
      form.inert = false;
      modal.oncancel = null;
      submit.textContent = "저장";
    }
  };
  const submit = button("저장", () => {}, "primary");
  submit.type = "submit";
  form.onsubmit = (e) => {
    e.preventDefault();
    void save(false).catch(report);
  };
  const buttons = [submit];
  if (id)
    buttons.unshift(
      button(
        "삭제",
        () => {
          if (confirm("선택한 범위의 일정을 삭제할까요?")) return save(true);
        },
        "danger",
      ),
    );
  actions(...buttons);
  name.focus();
}
function resolveForm() {
  const fields = state.Conflicts || [];
  begin("변경 충돌");
  for (const f of fields) {
    form.append(
      node("p", (state.Items?.[f.ID]?.title || f.ID) + " · " + f.Field),
      node("p", "이 기기: " + f.Local),
      node("p", "저장소: " + f.Remote),
    );
  }
  const choose = async (local: boolean) => {
    await transact(() => api.Resolve(fields, local));
    modal.close();
  };
  actions(
    button("저장소 값 사용", () => choose(false)),
    button("이 기기 값 사용", () => choose(true)),
  );
}
async function boot() {
  let lastDay = fmt(new Date());
  settings = await api.Mode();
  el("#quit").onclick = () => api.Close();
  el("#settings").onclick = () => {
    void api.Settings().catch(report);
  };
  el("#refresh").onclick = () => {
    void refresh().catch(report);
  };
  state = await api.State(false);
  document.addEventListener("keydown", navigateWithKeyboard);
  await render();
  let lastVersion: string | undefined;
  setInterval(() => {
    if (busy || modal.open) return;
    const now = fmt(new Date());
    if (fmt(selected) === lastDay && lastDay !== now) {
      selected = new Date();
      void render().catch(report);
    }
    lastDay = now;
  }, 60000);
  setInterval(async () => {
    if (busy || modal.open || drag) return;
    try {
      const version = await api.Version();
      if (version === lastVersion) return;
      const next = await api.State(false);
      if (!busy && !modal.open && !drag) {
        lastVersion = version;
        state = next;
        await render();
      }
    } catch (e) {
      report(e);
    }
  }, 1500);
}
void boot().catch(report);

export {};
