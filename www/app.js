// The theme switch is the matchbox: dark mode is a lit match, light mode is a
// match put back in its box. Loaded in <head> so the stored theme applies
// before the first paint (the CSP allows no inline script).
// onReady runs fn once the DOM is parsed, also when the script loads after
// that (an embedding page can add it late).
function onReady(fn) {
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", fn, { once: true });
  else fn();
}

(() => {
  const root = document.documentElement;
  const KEY = "matchblox-theme";

  const stored = () => {
    try { return localStorage.getItem(KEY); } catch { return null; }
  };
  const store = (t) => {
    try { localStorage.setItem(KEY, t); } catch { /* private window: the choice lasts this visit */ }
  };
  const system = () => (matchMedia("(prefers-color-scheme: light)").matches ? "light" : "dark");
  const current = () => root.dataset.theme || stored() || system();

  root.dataset.theme = current();

  onReady(() => {
    const btn = document.getElementById("match");
    if (!btn) return;
    const svg = btn.querySelector("svg");
    const drawer = svg.querySelector(".drawer");
    const match = svg.querySelector(".m");
    const flame = svg.querySelector(".flame");
    const spark = svg.querySelector(".spark");
    const smoke = svg.querySelector(".smoke");
    const still = () => matchMedia("(prefers-reduced-motion: reduce)").matches;

    // Match poses, as SVG transforms of the match group (tail at the origin, head at +x).
    const POSE = {
      boxed: "translate(54px, 46px) rotate(0deg)",
      strikeFrom: "translate(-4px, 41px) rotate(-15deg)",
      strikeTo: "translate(27px, 41px) rotate(-15deg)",
      upright: "translate(84px, 60px) rotate(-90deg)",
    };
    const OPEN = "translateX(30px)";
    const SHUT = "translateX(0px)";

    const label = () => {
      const dark = root.dataset.theme === "dark";
      btn.setAttribute("aria-label", dark ? "Put the match out (light mode)" : "Light the match (dark mode)");
      btn.setAttribute("aria-pressed", String(dark));
    };

    const rest = (theme) => {
      svg.classList.toggle("lit", theme === "dark");
      drawer.style.transform = SHUT;
      match.style.transform = theme === "dark" ? POSE.upright : POSE.boxed;
      match.style.opacity = theme === "dark" ? "1" : "0";
      flame.style.transform = theme === "dark" ? "scale(1)" : "scale(0)";
    };

    const set = (theme) => {
      root.dataset.theme = theme;
      store(theme);
      label();
    };

    rest(current());
    label();

    let busy = false;
    const step = (el, frames, ms, easing = "cubic-bezier(.2,.7,.2,1)") =>
      el.animate(frames, { duration: ms, easing, fill: "forwards" }).finished;

    const light = async () => {
      await step(drawer, [{ transform: SHUT }, { transform: OPEN }], 260);
      match.style.opacity = "1";
      await step(match, [{ transform: POSE.boxed, opacity: 0 }, { transform: POSE.boxed, opacity: 1, offset: 0.2 }, { transform: POSE.strikeFrom, opacity: 1 }], 280);
      await step(match, [{ transform: POSE.strikeFrom }, { transform: POSE.strikeTo }], 150, "cubic-bezier(.55,0,1,.45)");
      spark.animate([{ opacity: 0, transform: "scale(.4)" }, { opacity: 1, transform: "scale(1.2)", offset: 0.3 }, { opacity: 0, transform: "scale(1.6)" }], { duration: 260 });
      set("dark");
      svg.classList.add("lit");
      step(flame, [{ transform: "scale(0)" }, { transform: "scale(1.25)", offset: 0.6 }, { transform: "scale(1)" }], 240);
      await Promise.all([
        step(match, [{ transform: POSE.strikeTo }, { transform: POSE.upright }], 320),
        step(drawer, [{ transform: OPEN }, { transform: SHUT }], 260),
      ]);
    };

    const douse = async () => {
      await step(flame, [{ transform: "scale(1)" }, { transform: "scale(0)" }], 180, "ease-in");
      svg.classList.remove("lit");
      set("light");
      smoke.animate([{ opacity: 0, transform: "translateY(0)" }, { opacity: 0.7, offset: 0.3 }, { opacity: 0, transform: "translateY(-14px)" }], { duration: 520, easing: "ease-out" });
      await step(drawer, [{ transform: SHUT }, { transform: OPEN }], 240);
      await step(match, [{ transform: POSE.upright, opacity: 1 }, { transform: POSE.boxed, opacity: 1, offset: 0.8 }, { transform: POSE.boxed, opacity: 0 }], 320);
      match.style.opacity = "0";
      await step(drawer, [{ transform: OPEN }, { transform: SHUT }], 240);
    };

    btn.addEventListener("click", async () => {
      if (busy) return;
      const next = current() === "dark" ? "light" : "dark";
      if (still()) { set(next); rest(next); return; }
      busy = true;
      try { await (next === "dark" ? light() : douse()); }
      finally {
        for (const el of [drawer, match, flame]) el.getAnimations().forEach((a) => { a.commitStyles(); a.cancel(); });
        rest(next);
        busy = false;
      }
    });

    // Follow the system while the visitor has not chosen.
    matchMedia("(prefers-color-scheme: light)").addEventListener("change", () => {
      if (stored()) return;
      const t = system();
      root.dataset.theme = t;
      rest(t);
      label();
    });
  });
})();

// The replay: frames of the real console (www/demo/frames.json, recorded
// by TestReplay), drawn as text in the page's theme. With reduced motion,
// or without script, the still frame in the page stays.
onReady(() => {
  const pre = document.querySelector(".stage .replay");
  if (!pre || matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  const HOLD = 3000;
  const FLAGS = ["", "b", "u", "b u"];

  const line = (spans) => {
    const out = document.createDocumentFragment();
    for (const [text, fg, bg, flags] of spans || []) {
      const cls = [fg && "fg-" + fg, bg && "bg-" + bg, FLAGS[flags]].filter(Boolean).join(" ");
      if (!cls) { out.append(text); continue; }
      const s = document.createElement("span");
      s.className = cls;
      s.textContent = text;
      out.append(s);
    }
    return out;
  };

  fetch("demo/frames.json").then((r) => (r.ok ? r.json() : Promise.reject(r.status))).then((doc) => {
    const rows = [];
    pre.textContent = "";
    for (let i = 0; i < doc.rows; i++) {
      const el = document.createElement("span");
      rows.push(el);
      pre.append(el, i + 1 < doc.rows ? "\n" : "");
    }
    let timer = 0;
    const show = (k) => {
      const f = doc.frames[k];
      f.lines.forEach((spans, i) => { if (spans !== null && rows[i]) rows[i].replaceChildren(line(spans)); });
      const next = k + 1 < doc.frames.length ? doc.frames[k + 1].at - f.at : HOLD;
      timer = setTimeout(() => show((k + 1) % doc.frames.length), next);
    };
    // Frame 0 has every line; a loop starts there again.
    show(0);
    document.addEventListener("visibilitychange", () => {
      clearTimeout(timer);
      if (!document.hidden) show(0);
    });
  }).catch(() => { /* the still frame stays */ });
});

// Install: a tab picks the method, Copy takes the command. If the browser
// refuses the clipboard, the command is selected to copy by hand.
onReady(() => {
  const tabs = [...document.querySelectorAll('.install [role="tab"]')];
  const cmd = document.getElementById("cmdtext");
  const note = document.getElementById("note");
  const read = document.getElementById("readit");
  const copy = document.getElementById("copy");
  const status = document.querySelector(".copied");
  if (!cmd || !copy) return;
  const pick = (tab, focus) => {
    tabs.forEach((t) => {
      const on = t === tab;
      t.setAttribute("aria-selected", String(on));
      t.tabIndex = on ? 0 : -1;
    });
    cmd.textContent = tab.dataset.cmd;
    note.textContent = tab.dataset.note;
    read.hidden = !tab.dataset.read;
    document.getElementById("cmdline").setAttribute("aria-labelledby", tab.id);
    if (focus) tab.focus();
  };
  tabs.forEach((t, i) => {
    t.addEventListener("click", () => pick(t));
    t.addEventListener("keydown", (e) => {
      const n = { ArrowRight: 1, ArrowLeft: -1 }[e.key];
      if (n) { e.preventDefault(); pick(tabs[(i + n + tabs.length) % tabs.length], true); }
    });
  });
  copy.addEventListener("click", async () => {
    const text = cmd.textContent.trim();
    try {
      await navigator.clipboard.writeText(text);
      copy.textContent = "Copied";
      copy.dataset.done = "";
      status.textContent = "Copied: " + text;
      setTimeout(() => { copy.textContent = "Copy"; delete copy.dataset.done; }, 1600);
    } catch {
      getSelection().selectAllChildren(cmd);
      status.textContent = "Selected. Press Ctrl+C or ⌘C to copy.";
    }
  });
});

// The strike: blocks fall into a matchbox, the drawer opens, a match strikes
// on its side and lights. Touch the head and the flame jumps onto the cursor
// for a moment, and the match smokes. In light mode it is daylight: the
// match lights only while you hover it. Reduced motion shows the last frame.
onReady(() => {
  const cv = document.getElementById("scene");
  if (!cv) return;
  const root = document.documentElement;
  const C30 = Math.cos(Math.PI / 6), S30 = 0.5;
  const css = (n) => getComputedStyle(root).getPropertyValue(n).trim();
  const still = () => matchMedia("(prefers-reduced-motion: reduce)").matches;
  const pal = () => ({
    top: css("--box-top"), left: css("--box-left"), right: css("--box-right"), edge: css("--box-edge"),
    tray: css("--tray"), striker: css("--striker"), stick: css("--stick"), head: css("--primary"),
    dark: root.dataset.theme !== "light",
  });

  const view = (ctx, ox, oy, s) => {
    const p = (x, y, z) => [ox + (x - y) * C30 * s, oy + (x + y) * S30 * s - z * s];
    const poly = (pts, fill, stroke) => {
      ctx.beginPath();
      pts.forEach(([x, y, z], i) => { const [a, b] = p(x, y, z); i ? ctx.lineTo(a, b) : ctx.moveTo(a, b); });
      ctx.closePath();
      if (fill) { ctx.fillStyle = fill; ctx.fill(); }
      if (stroke) { ctx.strokeStyle = stroke; ctx.lineWidth = Math.max(1, s * 0.03); ctx.stroke(); }
    };
    const box = (x, y, z, w, d, h, c, top) => {
      poly([[x, y + d, z], [x + w, y + d, z], [x + w, y + d, z + h], [x, y + d, z + h]], c.left, c.edge);
      poly([[x + w, y, z], [x + w, y + d, z], [x + w, y + d, z + h], [x + w, y, z + h]], c.right, c.edge);
      poly([[x, y, z + h], [x + w, y, z + h], [x + w, y + d, z + h], [x, y + d, z + h]], top || c.top, c.edge);
    };
    return { p, poly, box };
  };

  const flame = (ctx, x, y, size, t) => {
    const f = 1 + 0.06 * Math.sin(t * 9) + 0.04 * Math.sin(t * 23);
    const g = ctx.createRadialGradient(x, y - size * 0.6, 0, x, y - size * 0.6, size * 2.4);
    g.addColorStop(0, "rgba(232,130,46,.38)"); g.addColorStop(1, "rgba(232,130,46,0)");
    ctx.fillStyle = g; ctx.beginPath(); ctx.arc(x, y - size * 0.6, size * 2.4, 0, Math.PI * 2); ctx.fill();
    ctx.save(); ctx.translate(x, y); ctx.scale(1 / f, f);
    ctx.fillStyle = "#E8822E"; ctx.beginPath();
    ctx.moveTo(0, 0); ctx.bezierCurveTo(size * 0.7, -size * 0.5, size * 0.45, -size * 1.3, 0, -size * 1.9);
    ctx.bezierCurveTo(-size * 0.45, -size * 1.3, -size * 0.7, -size * 0.5, 0, 0); ctx.fill();
    ctx.fillStyle = "#FFD27A"; ctx.beginPath();
    ctx.moveTo(0, -size * 0.15); ctx.bezierCurveTo(size * 0.35, -size * 0.45, size * 0.22, -size * 0.95, 0, -size * 1.2);
    ctx.bezierCurveTo(-size * 0.22, -size * 0.95, -size * 0.35, -size * 0.45, 0, -size * 0.15); ctx.fill();
    ctx.restore();
  };

  const stick = (ctx, x0, y0, x1, y1, c, w, r) => {
    ctx.strokeStyle = c.stick; ctx.lineWidth = w; ctx.lineCap = "round";
    ctx.beginPath(); ctx.moveTo(x0, y0); ctx.lineTo(x1, y1); ctx.stroke();
    ctx.fillStyle = c.head; ctx.beginPath(); ctx.arc(x1, y1, r, 0, Math.PI * 2); ctx.fill();
  };

  const drawBox = (ctx, v, s, c, out, withMatch) => {
    v.box(0, 0, 0, 3, 2, 1, c);
    v.poly([[0.15, 2, 0.2], [2.85, 2, 0.2], [2.85, 2, 0.8], [0.15, 2, 0.8]], c.striker, null);
    if (out > 0.01) {
      v.box(3, 0.12, 0.08, out, 1.76, 0.8, c, c.tray);
      if (withMatch) {
        const [ax, ay] = v.p(2.8, 1, 0.9), [bx, by] = v.p(3 + out - 0.25, 1, 0.9);
        stick(ctx, ax, ay, bx, by, c, Math.max(1.2, s * 0.12), Math.max(1.4, s * 0.17));
      }
    }
  };

  const fit = (cssW, cssH) => {
    const dpr = Math.max(1, Math.min(3, devicePixelRatio || 1));
    cv.width = Math.round(cssW * dpr); cv.height = Math.round(cssH * dpr);
    const ctx = cv.getContext("2d"); ctx.setTransform(dpr, 0, 0, dpr, 0, 0); ctx.clearRect(0, 0, cssW, cssH);
    return ctx;
  };

  const ease = (x) => 1 - Math.pow(1 - Math.min(1, Math.max(0, x)), 3);
  const fall = (x) => { x = Math.min(1, Math.max(0, x)); return x < 0.62 ? (x / 0.62) ** 2 : 1; };
  const ptr = { x: 0, y: 0, in: false };
  const NEAR = 22, BORROW = 1300;
  let t0 = 0, raf = 0, head = null, fireUntil = 0, daylightUntil = 0, embers = [];

  function frame(now) {
    const t = still() ? 9 : (now - t0) / 1000;
    const W = cv.clientWidth, H = cv.clientHeight;
    const ctx = fit(W, H);
    const c = pal();
    const s = Math.min(44, W / 11, H / 7);
    const ox = W * 0.36, oy = H * 0.38;
    const v = view(ctx, ox, oy, s);
    const nowMs = performance.now();
    head = null;
    if (t < 1.1) {
      [[0, 0], [1, 0], [0, 1], [2, 0], [1, 1], [2, 1]].forEach(([i, j], k) => {
        const start = k * 0.12;
        if (t < start) return;
        v.box(i, j, (1 - fall((t - start) / 0.52)) * 6, 1, 1, 1, c);
      });
      if (t > 0.9) { ctx.globalAlpha = ease((t - 0.9) / 0.2); drawBox(ctx, v, s, c, 0, false); ctx.globalAlpha = 1; }
    } else {
      const strikeStart = 1.45, strikeEnd = 1.75, lightAt = 1.8;
      const inBox = t < 1.4;
      const out = t < 2.2 ? 1.4 * ease((t - 1.1) / 0.3) : 1.4 * (1 - ease((t - 2.2) / 0.35));
      drawBox(ctx, v, s, c, out, inBox);
      if (!inBox) {
        const [sx0, sy0] = v.p(0.3, 2, 0.5), [sx1, sy1] = v.p(2.8, 2, 0.5), [ux, uy] = v.p(5.2, 1.2, 0);
        const L = s * 2.6;
        let hx, hy, tx, ty;
        if (t < strikeStart) { hx = sx0; hy = sy0; tx = hx - L * 0.9; ty = hy + L * 0.35 * ease((t - 1.4) / 0.05); }
        else if (t < strikeEnd) {
          const k = Math.pow((t - strikeStart) / (strikeEnd - strikeStart), 1.6);
          hx = sx0 + (sx1 - sx0) * k; hy = sy0 + (sy1 - sy0) * k; tx = hx - L * 0.9; ty = hy + L * 0.35;
        } else {
          const k = ease((t - strikeEnd) / 0.45);
          hx = sx1 + (ux - sx1) * k; hy = sy1 + (uy - L - sy1) * k;
          tx = sx1 - L * 0.9 + (ux - (sx1 - L * 0.9)) * k; ty = sy1 + L * 0.35 + (uy - (sy1 + L * 0.35)) * k;
        }
        stick(ctx, tx, ty, hx, hy, c, s * 0.12, s * 0.17);
        if (t > strikeEnd - 0.05 && t < strikeEnd + 0.25) {
          const a = 1 - (t - strikeEnd + 0.05) / 0.3;
          ctx.strokeStyle = `rgba(255,210,122,${a})`; ctx.lineWidth = 1.6; ctx.beginPath();
          for (let i = 0; i < 6; i++) { const g = i * Math.PI / 3; ctx.moveTo(sx1, sy1); ctx.lineTo(sx1 + Math.cos(g) * s * 0.5 * (1.4 - a), sy1 + Math.sin(g) * s * 0.5 * (1.4 - a)); }
          ctx.stroke();
        }
        if (t > strikeEnd + 0.45) head = [hx, hy];
        const near = head && ptr.in && Math.hypot(ptr.x - hx, ptr.y - hy + s * 0.5) < NEAR;
        if (near && c.dark && nowMs > fireUntil) fireUntil = nowMs + BORROW;
        if (near && !c.dark) daylightUntil = nowMs + 900;
        const borrowed = nowMs < fireUntil;
        if (t > lightAt && c.dark && !borrowed) {
          const back = fireUntil && nowMs - fireUntil < 260 ? ease((nowMs - fireUntil) / 260) : 1;
          const g = ease((t - lightAt) / 0.24);
          flame(ctx, hx, hy - s * 0.1, s * 0.42 * (g < 1 ? g * 1.15 : 1) * back, t);
        }
        if (!c.dark && nowMs < daylightUntil) flame(ctx, hx, hy - s * 0.1, s * 0.34, t);
        if (borrowed) {
          ctx.strokeStyle = "rgba(160,150,135,.5)"; ctx.lineWidth = 1.2; ctx.beginPath();
          const k = (nowMs % 900) / 900;
          ctx.moveTo(hx, hy - 4); ctx.bezierCurveTo(hx - 6, hy - 12 - 10 * k, hx + 6, hy - 18 - 10 * k, hx, hy - 28 - 12 * k); ctx.stroke();
        }
      }
    }
    if (nowMs < fireUntil && ptr.in) {
      const left = (fireUntil - nowMs) / BORROW;
      embers.push({ x: ptr.x, y: ptr.y, vx: (Math.random() - 0.5) * 0.6, vy: -0.4 - Math.random() * 0.8, life: 1 });
      flame(ctx, ptr.x, ptr.y + 2, s * 0.36 * Math.min(1, left * 3), t);
    }
    embers = embers.filter((e) => (e.life -= 0.035) > 0);
    for (const e of embers) {
      e.x += e.vx; e.y += e.vy;
      ctx.fillStyle = `rgba(255,${(150 + 60 * e.life) | 0},90,${e.life})`;
      ctx.beginPath(); ctx.arc(e.x, e.y, 1.6 * e.life + 0.4, 0, Math.PI * 2); ctx.fill();
    }
    cv.style.cursor = nowMs < fireUntil ? "none" : head && ptr.in && Math.hypot(ptr.x - head[0], ptr.y - head[1]) < NEAR + 10 ? "pointer" : "default";
    // Draw on while it builds, while the flame burns, or while the pointer is here.
    if (!still() && (t < 2.7 || c.dark || ptr.in || embers.length)) raf = requestAnimationFrame(frame);
  }

  const play = () => { cancelAnimationFrame(raf); t0 = performance.now(); raf = requestAnimationFrame(frame); };
  const kick = () => { cancelAnimationFrame(raf); raf = requestAnimationFrame(frame); };
  cv.addEventListener("pointermove", (e) => {
    const r = cv.getBoundingClientRect(); ptr.x = e.clientX - r.left; ptr.y = e.clientY - r.top;
    if (!ptr.in) { ptr.in = true; kick(); }
  });
  cv.addEventListener("pointerleave", () => { ptr.in = false; });
  // The theme switch changes the light: strike again in the new one.
  new MutationObserver(play).observe(root, { attributes: true, attributeFilter: ["data-theme"] });
  let rt = 0;
  addEventListener("resize", () => { clearTimeout(rt); rt = setTimeout(kick, 120); });
  // While it is off screen, it does not draw.
  new IntersectionObserver(([e]) => { if (e.isIntersecting) kick(); else cancelAnimationFrame(raf); }).observe(cv);
  play();
});
