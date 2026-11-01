// The stoker takes orders: six screens of the console, drawn as the replay
// draws its frames ([text, fg, bg, flags] spans in the site's colour
// tokens). Folded they fill the phone's 44 columns; open, the Duo lays the
// sessions on the left and the step's own pane on the right. The Sessions
// step answers g, o, z, the arrows and n, as the console does. Every name
// is the demo machine's. TestStokerSVG (cmd/matchblox) loads this file in
// Node and draws the README's animation from tour().
(() => {
  const ROWS = 34, PHONE = 44, HALF = 44, DUO = 2 * HALF + 1;
  const T = "fg", M = "muted", F = "faint", H = "hair", A = "accent", W = "warn", N = "neg", NK = "night";
  const BOLD = 1, UNDER = 2;

  // A segment is [text, fg, flags, act, bg]; a line is an array of
  // segments that may carry the line's own bg (the selected row) and act
  // (a tap).
  const len = (s) => [...s].length;
  const width = (l) => l.reduce((n, s) => n + len(s[0]), 0);
  const keep = (from, to) => Object.assign(to, { bg: from.bg, act: from.act });
  const cut = (l, w) => {
    const out = [];
    let n = 0;
    for (const s of l) {
      const cs = [...s[0]];
      if (n + cs.length <= w) { out.push(s); n += cs.length; continue; }
      const k = w - n;
      if (k > 0) out.push([cs.slice(0, k - 1).join("") + "…", ...s.slice(1)]);
      n = w;
      break;
    }
    if (n < w) out.push([" ".repeat(w - n), ""]);
    return keep(l, out);
  };
  // clip keeps the first w columns as they are, for a pane under a sheet.
  const clip = (l, w) => {
    const out = [];
    let n = 0;
    for (const s of l) {
      if (n >= w) break;
      const cs = [...s[0]].slice(0, w - n);
      out.push([cs.join(""), ...s.slice(1)]);
      n += cs.length;
    }
    return keep(l, out);
  };
  const lr = (left, right, w) => {
    const r = width(right);
    if (width(left) + r + 1 > w) return keep(left, [...cut(left, Math.max(w - r - 1, 0)), [" ", ""], ...right]);
    return keep(left, [...left, [" ".repeat(w - width(left) - r), ""], ...right]);
  };
  const wrap = (text, w) => {
    const out = [];
    let cur = "";
    for (const word of text.split(" ")) {
      if (!cur) cur = word;
      else if (len(cur) + 1 + len(word) <= w) cur += " " + word;
      else { out.push(cur); cur = word; }
    }
    if (cur) out.push(cur);
    return out;
  };
  const hair = (w) => [["─".repeat(w), H]];
  const text = (t, fg, w) => cut([[t, fg]], w);
  const pad = (lines, n) => { while (lines.length < n) lines.push([]); return lines.slice(0, n); };
  const dim = (l) => keep({ act: l.act }, l.map((s) => [s[0], "dim", 0]));

  // The demo machine's sessions, as the console lists them.
  const SESSIONS = [
    { name: "kindle-web", st: "asks", mins: 4, ctx: 24, plan: 93, repo: "hearth", said: "May I run the e2e suite against the staging database?" },
    { name: "flint-auth", st: "asks", mins: 12, ctx: 16, plan: 88, repo: "hearth", said: "#4545 conflicts with main in auth/rotate.go. Rebase now, or after #4551?" },
    { name: "tinder-cli", st: "busy", mins: 0, ctx: 47, plan: 40, repo: "hearth", said: "Writing the tests for the new flags parser." },
    { name: "hearth-api", st: "busy", mins: 1, ctx: 26, plan: 88, repo: "hearth", said: "#4551 is in CI: 6 of 9 jobs green so far." },
    { name: "codex", st: "busy", mins: 2, ctx: 13, plan: 50, repo: "spark", said: "Moving the docs build to the new theme." },
    { name: "ember-billing", st: "idle", mins: 60, ctx: 10, plan: 88, repo: "hearth", why: "#4552 waits your merge", wc: A, said: "Invoice export as CSV and PDF is green. #4552 waits for your merge." },
    { name: "spark-docs", st: "idle", mins: 95, ctx: 40, plan: 100, repo: "spark", why: "#4540 merged · done", wc: A, said: "The 1.0 docs are live." },
    { name: "cinder-jobs", st: "idle", mins: 180, ctx: 23, plan: 75, repo: "hearth", why: "5h limit · back 16:00", wc: W, said: "The 5h window is full. I go on when it resets at 16:00." },
    { name: "opencode", st: "idle", mins: 300, ctx: 12, plan: 100, repo: "spark", why: "out of work", wc: F, said: "The changelog is written. Nothing left on my list." },
    { name: "blaze-search", st: "idle", mins: 360, ctx: 25, plan: 60, repo: "blaze", why: "stuck: go build fails", wc: N, said: "go build fails: index/query.go:41 undefined: Tokenize." },
    { name: "wick-mobile", st: "idle", mins: 120, ctx: 19, plan: 75, repo: "hearth", why: "", said: "The offline cache is half done." },
    { name: "coal-infra", st: "idle", mins: 1440, ctx: 20, plan: 80, repo: "coal", why: "CI red · e2e", wc: N, said: "The e2e job fails on the new runner image." },
  ];
  // What the sessions say at the later steps: the night held three, the
  // morning finds the merges live, the day pauses two on review.
  const held = (why) => ({ held: why });
  const AT = {
    night: {
      "ember-billing": { why: "#4552 merged · live", mins: 41 }, "flint-auth": { st: "idle", why: "next, after #4551", wc: F, mins: 67 },
      "kindle-web": { mins: 220 }, opencode: held("not priority"), "blaze-search": held("not priority"), "coal-infra": held("not priority"),
    },
    morning: {
      "ember-billing": { why: "#4552 live", mins: 290 }, "hearth-api": { st: "idle", why: "#4551 merged · live", wc: A, mins: 175 },
      "flint-auth": { mins: 172 }, "tinder-cli": { st: "idle", why: "#4553 fixed · pushed", wc: A, mins: 140 }, codex: { st: "idle", why: "out of work", wc: F, mins: 200 },
      "kindle-web": { mins: 470 }, opencode: held("not priority"), "blaze-search": held("not priority"), "coal-infra": held("not priority"),
    },
    day: {
      "kindle-web": { st: "busy", mins: 0 }, "flint-auth": { mins: 20 }, "spark-docs": held("on review"), codex: held("on review"),
    },
  };
  const GROUPS = ["need", "status", "repo", "none"];
  const SORTS = ["state", "newest", "context", "name"]; // the console's columns, s the next, S turns it around
  const fresh = () => ({ step: 0, group: 0, sort: 1, back: false, sel: "kindle-web", parked: new Set(["wick-mobile"]) });

  const sessions = (S, at) => SESSIONS.map((s) => Object.assign({}, s, at && AT[at][s.name]));
  const age = (m) => (!m ? "" : m < 60 ? m + "m" : m < 1440 ? Math.round(m / 60) + "h" : Math.round(m / 1440) + "d");
  const groups = (S, list) => {
    const parked = (s) => S.parked.has(s.name) || s.held;
    const key = {
      need: (s) => (parked(s) ? "HELD" : s.st === "asks" ? "WAITS FOR YOU" : s.st === "busy" ? "WORKING" : "QUIET"),
      status: (s) => (parked(s) ? "PARKED" : s.st.toUpperCase()),
      repo: (s) => s.repo.toUpperCase(),
      none: () => "ALL",
    }[GROUPS[S.group]];
    const order = S.group === 0 ? ["WAITS FOR YOU", "WORKING", "QUIET", "HELD"] : null;
    const rank = { asks: 0, busy: 1, idle: 2 };
    const by = {
      state: (a, b) => rank[a.st] - rank[b.st] || a.mins - b.mins, newest: (a, b) => a.mins - b.mins,
      context: (a, b) => b.ctx - a.ctx, name: (a, b) => a.name.localeCompare(b.name),
    }[SORTS[S.sort]];
    const cmp = S.back ? (a, b) => by(b, a) : by;
    const m = new Map();
    for (const s of [...list].sort(cmp)) {
      const k = key(s);
      if (!m.has(k)) m.set(k, []);
      m.get(k).push(s);
    }
    const keys = order ? order.filter((k) => m.has(k)) : [...m.keys()].sort();
    return keys.map((k) => [k, m.get(k)]);
  };
  const state = (S, s) => {
    if (S.parked.has(s.name)) return ["⏸", M, "parked by you", M];
    if (s.held) return ["⏸", M, "held · " + s.held, M];
    if (s.st === "asks") return ["●", A, "asks", A];
    if (s.st === "busy") return ["◐", T, "busy", M];
    return ["○", F, s.why, s.wc || F];
  };
  const row = (S, s, sel) => {
    const [g, gc, word, wc] = state(S, s);
    const name = len(s.name) > 13 ? [...s.name].slice(0, 12).join("") + "…" : s.name;
    const l = lr([[sel ? "▌" : " ", A], [g, gc], [" ", ""], [name.padEnd(14), T], [word, wc]], [[age(s.mins).padStart(3) + " ", F]], HALF);
    l.act = "sel:" + s.name;
    if (sel) l.bg = "wash";
    return l;
  };
  // The list's own lines: how it is grouped and sorted, then the rows.
  const list = (S, at, sel) => {
    const L = [cut([[" g ", A, 0, "g"], ["by " + GROUPS[S.group], T, 0, "g"], ["   s ", A, 0, "s"], [SORTS[S.sort] + (S.back ? " ↑" : ""), T, 0, "s"]], HALF)];
    for (const [k, rows] of groups(S, sessions(S, at))) {
      L.push(text(` ${k} · ${rows.length}`, F, HALF));
      for (const s of rows) L.push(row(S, s, s.name === sel));
    }
    return L;
  };
  const header = (on, w) => {
    const right = on === "night" ? [["night → 07:00 ", NK]] : on === "on" ? [["stoker ", A]] : [["● 2 wait ", A]];
    return lr([[" ▰ ", A], ["matchblox", T, BOLD]], right, w);
  };
  const tabs = (active, w) => {
    const t = (label, n, on) => (on ? [[label, A, UNDER], [n ? " " + n : "", A]] : [[label.toLowerCase(), F], [n ? " " + n : "", F]]);
    return cut([[" ", ""], ...t("SESSIONS", 12, active === "s"), ["   ", ""], ...t("SHIPS", 4, active === "p"), ["   ", ""], ...t("STOKER", "", active === "k")], w);
  };
  const keys = (list, w) => {
    const l = [[" ", ""]];
    for (const [k, label, act] of list) {
      const hint = [[(l.length > 1 ? "  " : "") + k, A, 0, act || k], [" " + label, M, 0, act || k]];
      if (width(l) + width(hint) > w) break; // a narrow key line drops the hints that do not fit
      l.push(...hint);
    }
    return cut(l, w);
  };
  const box = (title, content, footer, w) => {
    const iw = w - 4;
    const top = [["╭─ ", H], ...title, [" ", ""]];
    const out = [[...top, ["─".repeat(Math.max(w - width(top) - 1, 0)), H], ["╮", H]]];
    for (const c of content) {
      if (c === "sep") { out.push([["├" + "─".repeat(w - 2) + "┤", H]]); continue; }
      out.push([["│ ", H], ...cut(c, iw), [" │", H]]);
    }
    const bot = [["╰─ ", H], ...footer, [" ", ""]];
    out.push([...bot, ["─".repeat(Math.max(w - width(bot) - 1, 0)), H], ["╯", H]]);
    return out;
  };
  const numbered = (items, iw) => {
    const out = [];
    for (const [n, t] of items) wrap(t, iw - 3).forEach((x, i) => out.push([[i ? "   " : n + " ", A], [x, T]]));
    return out;
  };
  const typed = (words, iw) => {
    const out = wrap(words, iw - 2).map((t, i) => [[i ? "  " : "› ", A], [t, T]]);
    out[out.length - 1].push(["▏", A]);
    return out;
  };

  // The selected session's detail: what it said, its context and its plan.
  const detail = (S, at, name) => {
    const s = sessions(S, at).find((x) => x.name === name);
    if (!s) return [];
    const [g, gc, word, wc] = state(S, s);
    const bar = (label, pct, c) => {
      const n = Math.round(pct / 10);
      return cut([[" " + label.padEnd(9), F], ["▰".repeat(n), c], ["▱".repeat(10 - n), F], [" " + pct + "%", M]], HALF);
    };
    const L = [lr([[" " + s.name, T, BOLD]], [[s.repo + " ", F]], HALF), cut([[" " + g + " ", gc], [word || "quiet", wc], [s.mins ? " · " + age(s.mins) : "", F]], HALF), []];
    L.push(text(" SAID", F, HALF));
    wrap(s.said, HALF - 2).forEach((t) => L.push(text(" " + t, T, HALF)));
    L.push([], bar("context", s.ctx, s.ctx > 80 ? N : M), bar("plan", s.plan, s.plan > 90 ? W : M), []);
    L.push(cut([[" ⏎", A, 0, "Enter"], [" chat   ", M], ["a", A], [" chat   ", M], ["z", A, 0, "z"], [S.parked.has(s.name) ? " let go" : " park", M, 0, "z"]], HALF));
    return L;
  };

  const ORDER = "keep only the priority ones going, set the ready ones up in merge order and merge them on green";
  const DAY_ORDER = "pause everything waiting on review, keep tinder-cli and kindle-web going";
  // The go chips: on (✓, the run gives it) or off (○, it asks you).
  const gos = (on) => {
    const l = [["gos ", F]];
    ["merges", "releases", "deploys"].forEach((g, i) => l.push([(i ? "  " : "") + (on.includes(g) ? "✓ " : "○ ") + g, on.includes(g) ? A : M]));
    return l;
  };
  // The command that starts the stoker agent on matchblox's own tmux
  // server, as the sheet shows it while the agent comes up.
  const START = "env -u GIT_TERMINAL_PROMPT -u GIT_SSH_COMMAND -u MATCHBLOX_HOME tmux -L matchblox -f /dev/null new-session -d -s stoker -c /home/you claude ; set-option -s exit-empty on";
  const launch = () => {
    const iw = HALF - 4;
    const c = [[["How should I run tonight?", M]], [], ...typed(ORDER, iw)];
    c.push([], gos(["merges"]), [["ways ", F], ["merge-order", A, UNDER]]);
    c.push([], [["  starting the stoker…", M]], ...wrap(START, iw - 2).map((t) => [["  " + t, F]]));
    return box([["☾ Night", NK, BOLD]], c, [["⏎", A, 0, "Enter"], [" send ", M, 0, "Enter"], ["─ ", H], ["tab", A], [" chips ", M], ["─ ", H], ["esc", A, 0, "Escape"]], HALF);
  };
  const neworder = () => {
    const iw = HALF - 4;
    const c = [[["How should I run?", M]], [], ...typed("also merge ember-billing when green, then let cinder-jobs go on at 16:00", iw)];
    c.push([], gos(["merges"]));
    return box([["New order", A, BOLD]], c, [["⏎", A], [" send ", M], ["─ ", H], ["tab", A], [" chips ", M], ["─ ", H], ["esc", A, 0, "Escape"]], HALF);
  };

  const step = (g, gc, time, what, sub) => {
    const out = [];
    wrap(what, HALF - 12).forEach((t, i) => out.push(cut([[i ? "   " : " " + g + " ", gc], [i ? "      " : time.padEnd(5) + " ", F], [t, T]], HALF)));
    if (sub) wrap(sub, HALF - 12).forEach((t) => out.push(cut([["         ", ""], [t, M]], HALF)));
    return out;
  };
  // The stoker's card: your words, then every step and what came of it.
  // The stoker agent's row heads the card: what it does now, and ⏎ that
  // opens it in a popup. The phone's 44 columns cut its word, as the
  // console's do.
  const agent = (at) =>
    lr([["  ", ""], ["STOKER AGENT", F], ["  claude", T], [" · " + (at === "morning" ? "rests" : "works"), M]], [["⏎ open the stoker ", F, 0, "Enter"]], HALF);
  const card = (at) => {
    const title = at === "day" ? " STOKER · since 13:05" : " NIGHT · 23:40 → 07:00";
    const right = at === "night" ? [["sealed ", NK]] : at === "morning" ? [["ended 07:00 ", F]] : [["on ", F]];
    const L = [agent(at), [], lr([[title, T, BOLD]], right, HALF), []];
    wrap("“" + (at === "day" ? DAY_ORDER : ORDER) + "”", HALF - 3).forEach((t) => L.push(text("  " + t, M, HALF)));
    if (at !== "day") L.push(cut([["  ✓ ", A], ["merges", M]], HALF), cut([["  ! ", W], ["releases and deploys still ask you", M]], HALF));
    L.push([]);
    if (at === "night") {
      L.push(text(" PLAN 2 of 4", F, HALF),
        ...step("✓", A, "23:41", "Held 3 sessions: not priority"),
        ...step("✓", A, "01:58", "ember-billing merged #4552", "CI green 01:52"),
        ...step("◐", W, "02:31", "hearth-api: merge #4551 on green", "CI 6 of 9 jobs"),
        ...step("○", F, "", "flint-auth: rebase #4545, merge on green"),
        [], text(" YOUR GO", F, HALF),
        ...step("⏸", W, "02:40", "waits for you: kindle-web wants to run gh release create v2.1", "y gives the go, once"),
        [], text(" TRIGGERS", F, HALF),
        ...step("✓", A, "02:31", "when #4552 merged → sent hearth-api: rebase on main"),
        text(" WAITING ON", F, HALF),
        cut([["  ○ ", F], ["t2 when #4551 merged → send flint-auth", M]], HALF));
    } else if (at === "morning") {
      L.push(text(" DONE 3 of 4 · 1 WAITS FOR YOU", A, HALF),
        ...step("✓", A, "01:58", "ember-billing merged #4552", "live 02:31"),
        ...step("✓", A, "03:40", "hearth-api merged #4551", "live 04:12"),
        ...step("✕", N, "04:15", "flint-auth: #4545 still conflicts after rebase", "auth/rotate.go, both sides changed expires_at. flint-auth asks you which to keep"),
        ...step("✓", A, "23:41", "Held 3 sessions all night", "z on one lets it go"),
        [], text(" TRIGGERS", F, HALF),
        ...step("✓", A, "04:12", "when #4551 merged → sent flint-auth: rebase #4545 on main"),
        [], cut([[" ", ""], ["a", A], [" look at flint-auth   ", M], ["f", A], [" stoker for the day", M]], HALF));
    } else {
      L.push(text(" PLAN 1 of 2", F, HALF),
        ...step("✓", A, "13:05", "Paused spark-docs and codex: on review"),
        ...step("◐", W, "13:20", "kindle-web: start task 11"));
    }
    return L;
  };

  // The ways file: the starter matchblox writes (internal/setup/ways.go),
  // open in $EDITOR.
  const WAYS = `# Standing rules

- When a session opens a PR: compact it, then send it /review.
- Past 80% of the week: hold every session that is not a priority until the week resets.
- Never touch a session I parked.

# Priority

Name the sessions or the issues that come first.

# Recipe: merge-order

1. Ready = a PR that is green, or only behind main.
2. Order: fewest conflicts first, and a stacked PR after its base.
3. Tell each session to read its issue, watch the PR before it, then rebase and merge on green.
4. A failed rebase: ask me, stop that lane, and go on with the rest.`;
  const ways = (w) => {
    const L = [];
    for (const raw of WAYS.split("\n")) {
      if (raw.startsWith("#")) { L.push(cut([[" " + raw, A, BOLD]], w)); continue; }
      const lead = (raw.match(/^(- |\d+\. )/) || [""])[0];
      const lines = wrap(raw.slice(lead.length), w - 2 - lead.length);
      if (!lines.length) L.push([]);
      lines.forEach((t, i) => L.push(text(" " + (i ? " ".repeat(lead.length) : lead) + t, T, w)));
    }
    return L;
  };

  // Each step: the header's state, the active tab, the keys, and the panes.
  // Folded the phone shows `main` (with `over` drawn over its foot); open,
  // the Duo shows the sessions left and `side` right.
  const STEPS = [
    { on: "off", tab: "s", keys: [["⏎", "chat", "Enter"], ["a", "chat"], ["z", "park"], ["n", "night"], ["f", "stoker"]], list: true,
      side: (S) => detail(S, null, S.sel),
      h: "Every quiet session says why", p: "A parked session leaves the quiet pile. Try g and s, or z on a row." },
    { on: "off", tab: "s", keys: [["⏎", "chat", "Enter"], ["a", "chat"], ["z", "park"], ["n", "night"], ["f", "stoker"]], list: true, over: launch,
      side: (S) => detail(S, null, S.sel),
      h: "n asks for tonight’s orders", p: "matchblox starts the stoker agent on a tmux server of its own. Say it in your own words, or tap a recipe from your ways. Turn on the gos you give: merges, releases, deploys. ⏎ and the night starts at once." },
    { on: "night", tab: "k", keys: [["y", "give the go"], ["⏎", "command"], ["1", "sessions"], ["n", "stop night"]], at: "night", main: () => card("night"), night: true,
      h: "At 03:12 it works its own plan", p: "Each step is a message to a session. The merge you gave goes through; a release you did not waits for your y. Where it would wait, the stoker sets a trigger, and matchblox fires it." },
    { on: "off", tab: "k", keys: [["a", "look"], ["⏎", "command"], ["1", "sessions"], ["p", "ships"], ["f", "stoker"]], at: "morning", sel: "flint-auth", main: () => card("morning"),
      h: "In the morning, the card", p: "c opens it from any tab: your words, then what came of each step. What failed says why, with the one key that resolves it." },
    { on: "on", tab: "k", keys: [["⏎", "command"], ["1", "sessions"], ["p", "ships"], ["f", "stoker"]], at: "day", main: () => card("day"), over: neworder,
      h: "By day, o gives a new order", p: "One line to the stoker, with the gos you set. The run goes on, and the steps already done stay done." },
    { file: true,
      h: "Your ways, in your words", p: "One markdown file, open in your own editor. The stoker reads it at every launch: standing rules, and recipes an order can call. matchblox writes this starter once; the rest is yours." },
  ];

  // The night palette (DESIGN §2.1) for a whole screen.
  const NIGHT = { [T]: "night-fg", [M]: "night-muted", [F]: "night-faint", [H]: "night-hair" };
  const spans = (l, night) => l.map((s) => {
    const fg = night ? NIGHT[s[1]] || s[1] : s[1];
    const bg = (s[4] || l.bg) === "wash" ? (night ? "night-wash" : "wash") : night ? "night-bg" : "";
    return [s[0], fg || "", bg, s[2] || 0, s[3]];
  });
  // The screen of a step, folded or open, as lines of spans. A sheet is
  // drawn over the foot of the dimmed screen: the phone's whole width, or
  // the Duo's right pane.
  const screen = (S, open) => {
    const st = STEPS[S.step];
    const sel = st.list ? S.sel : st.sel;
    const w = open ? DUO : PHONE;
    if (st.file) { // your editor, not the console: the file and its name
      const lines = [...pad(ways(w), ROWS - 1), cut([[" ~/.config/matchblox/ways.md", F, BOLD], ["  your editor", F]], w)];
      return lines.map((l) => spans(cut(l, w), false));
    }
    const top = [header(st.on, w), tabs(st.tab, w)];
    const body = ROWS - top.length - 3; // the hairs above and below, the keys
    let lines;
    if (!open) {
      lines = [...top, hair(w), ...pad(st.list ? list(S, st.at, sel) : st.main(), body), hair(w), keys(st.keys, w)];
    } else {
      const left = pad(list(S, st.at, sel), body);
      const right = pad(st.main ? st.main() : st.side(S), body);
      lines = [...top, [["─".repeat(HALF) + "┬" + "─".repeat(HALF), H]]];
      const pane = (l) => cut(l, HALF).map((s) => [s[0], s[1], s[2], s[3], s[4] || l.bg]);
      for (let i = 0; i < body; i++) lines.push(keep({ act: left[i].act }, [...pane(left[i]), ["│", H], ...pane(right[i])]));
      lines.push([["─".repeat(HALF) + "┴" + "─".repeat(HALF), H]], keys(st.keys, w));
    }
    lines = lines.map((l) => cut(l, w));
    if (st.over) {
      const over = st.over();
      const from = ROWS - over.length;
      lines = lines.map((l, i) => (i < from ? dim(l) : open ? [...dim(clip(l, HALF + 1)), ...over[i - from]] : over[i - from]));
    }
    return lines.map((l) => spans(l, st.night));
  };

  // A key, as the console takes it. It says whether it did anything.
  const press = (S, k) => {
    if (S.step === 0) {
      const flat = groups(S, sessions(S)).flatMap((g) => g[1]);
      const i = flat.findIndex((s) => s.name === S.sel);
      if (k === "g") S.group = (S.group + 1) % GROUPS.length;
      else if (k === "s") S.sort = (S.sort + 1) % SORTS.length;
      else if (k === "S") S.back = !S.back;
      else if (k === "z") S.parked.has(S.sel) ? S.parked.delete(S.sel) : S.parked.add(S.sel);
      else if (k === "ArrowDown" || k === "j") S.sel = flat[Math.min(i + 1, flat.length - 1)].name;
      else if (k === "ArrowUp" || k === "k") S.sel = flat[Math.max(i - 1, 0)].name;
      else if (k === "n") S.step = 1;
      else return false;
    } else if (k === "Escape" && (S.step === 1 || S.step === 5)) S.step = 0;
    else if (k === "Enter" && S.step === 1) S.step = 2;
    else return false;
    return true;
  };

  // The README's animation: each step on the phone, four seconds each.
  const tour = () => {
    const S = fresh();
    return STEPS.map((_, i) => {
      S.step = i;
      return { at: i * 4000, lines: screen(S, false).map((l) => l.map((s) => s.slice(0, 4))) };
    });
  };

  if (typeof document === "undefined") {
    if (typeof module === "object") module.exports = { tour, screen, fresh };
    return;
  }

  onReady(() => {
    const stage = document.getElementById("orders");
    const fold = stage && stage.querySelector(".fold");
    const pre = fold && fold.querySelector(".replay");
    if (!pre) return;
    const glass = pre.parentElement;
    const body = fold.querySelector(".body");
    const say = stage.querySelector(".say");
    const keypad = stage.querySelector(".keypad");
    const tablist = [...stage.querySelectorAll('[role="tab"]')];
    const sides = [...stage.querySelectorAll(".foldswitch button")];
    const SWING = 620;
    const S = fresh();
    let open = false;

    const draw = () => {
      const st = STEPS[S.step];
      glass.classList.toggle("night", !!st.night);
      const out = document.createDocumentFragment();
      screen(S, open).forEach((l, i) => {
        const el = document.createElement("span");
        for (const [t, fg, bg, flags, act] of l) {
          const part = replayLine([[t, fg, bg, flags]]);
          if (!act) { el.append(part); continue; }
          const s = document.createElement("span");
          s.dataset.act = act;
          s.append(part);
          el.append(s);
        }
        const row = l.find((s) => s[4] && s[4].startsWith("sel:"));
        if (row) el.dataset.act = row[4];
        out.append(el, i + 1 < ROWS ? "\n" : "");
      });
      pre.replaceChildren(out);
      fitReplay(pre);
      tablist.forEach((t, i) => {
        t.setAttribute("aria-selected", String(i === S.step));
        t.tabIndex = i === S.step ? 0 : -1;
      });
      keypad.hidden = S.step !== 0;
      const b = document.createElement("b");
      b.textContent = st.h + ".";
      say.replaceChildren(b, " " + st.p);
    };
    const hit = (k) => { if (press(S, k)) draw(); };

    pre.addEventListener("keydown", (e) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      if (press(S, e.key)) { e.preventDefault(); draw(); }
    });
    pre.addEventListener("click", (e) => {
      const t = e.target.closest("[data-act]");
      if (!t) return;
      const act = t.dataset.act;
      if (act.startsWith("sel:")) {
        if (S.step !== 0) return;
        S.sel = act.slice(4);
        draw();
      } else hit(act);
    });
    keypad.querySelectorAll("[data-key]").forEach((b) => b.addEventListener("click", () => hit(b.dataset.key)));
    tablist.forEach((t, i) => {
      t.addEventListener("click", () => { S.step = i; draw(); });
      t.addEventListener("keydown", (e) => {
        const n = { ArrowRight: 1, ArrowLeft: -1, Home: -i, End: tablist.length - 1 - i }[e.key];
        if (n === undefined) return;
        e.preventDefault();
        S.step = (i + n + tablist.length) % tablist.length;
        draw();
        tablist[S.step].focus();
      });
    });

    // Open widens the device; the screen redraws for the new width halfway
    // through the swing, and the stage keeps its height.
    const set = (to) => {
      if (open === to) return;
      open = to;
      sides.forEach((b) => b.setAttribute("aria-pressed", String((b.dataset.fold === "duo") === to)));
      fold.style.minHeight = Math.max(fold.offsetHeight, parseFloat(fold.style.minHeight) || 0) + "px";
      fold.classList.add("swinging", "fading");
      fold.classList.toggle("open", to);
      setTimeout(() => { draw(); fold.classList.remove("fading"); }, SWING / 2);
      setTimeout(() => fold.classList.remove("swinging"), SWING + 30);
    };
    sides.forEach((b) => b.addEventListener("click", () => set(b.dataset.fold === "duo")));
    // Without script the page shows the phone's first screen as a still;
    // with it, the screen takes keys and the controls appear.
    pre.id = "orders-screen";
    pre.tabIndex = 0;
    pre.removeAttribute("aria-hidden");
    pre.setAttribute("role", "tabpanel");
    pre.setAttribute("aria-label", "The stoker on the phone. On Sessions: g groups, s sorts, S turns the sort around, z parks, the arrows move, n launches a night");
    stage.querySelector(".steps").hidden = false;
    stage.querySelector(".foldswitch").hidden = false;

    let frame = 0;
    addEventListener("resize", () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => { fold.style.minHeight = ""; fitReplay(pre); });
    });
    body.addEventListener("transitionend", (e) => { if (e.target === body && e.propertyName === "width") fitReplay(pre); });
    document.fonts.ready.then(() => fitReplay(pre));
    draw();
  });
})();
