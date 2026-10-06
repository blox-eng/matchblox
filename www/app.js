// The theme switch is the matchbox: dark mode is a lit match, light mode is a
// match put back in its box. Loaded in <head> so the stored theme applies
// before the first paint (the CSP allows no inline script).
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

  document.addEventListener("DOMContentLoaded", () => {
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
