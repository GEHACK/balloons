// Per-station display preferences: which lists show, and which comes first.
// Stored per browser, so a print desk and a balloon desk can each keep their
// own view of the same tracker.

interface View {
  prints: boolean;
  balloons: boolean;
  printsFirst: boolean;
}

type Kind = "prints" | "balloons";

const STORAGE_KEY = "view";
// Prints are the priority: a team is waiting on them to keep debugging.
const DEFAULT_VIEW: View = { prints: true, balloons: true, printsFirst: true };

function load(): View {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const v = { ...DEFAULT_VIEW, ...JSON.parse(raw) };
      // Never come back to a blank page.
      if (!v.prints && !v.balloons) v.prints = v.balloons = true;
      return v;
    }
  } catch {}
  return { ...DEFAULT_VIEW };
}

function save(v: View) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(v));
  } catch {}
}

export function initView() {
  const printsBtn = document.getElementById("view-prints")!;
  const balloonsBtn = document.getElementById("view-balloons")!;
  const orderBtn = document.getElementById("view-order") as HTMLButtonElement;
  const sections = [
    ...document.querySelectorAll<HTMLElement>("#lists > section[data-kind]"),
  ];
  let view = load();

  function apply() {
    printsBtn.setAttribute("aria-pressed", String(view.prints));
    balloonsBtn.setAttribute("aria-pressed", String(view.balloons));
    // Order only matters while both lists are on screen.
    orderBtn.disabled = !(view.prints && view.balloons);

    for (const s of sections) {
      const kind = s.dataset.kind as Kind;
      s.classList.toggle("hidden", !view[kind]);
      // Pending lists always above delivered ones; within each stage the
      // preferred kind goes first.
      const stage = s.dataset.stage === "done" ? 2 : 0;
      const first = (kind === "prints") === view.printsFirst ? 0 : 1;
      s.style.order = String(stage + first);
    }
  }

  function toggle(kind: Kind) {
    const other: Kind = kind === "prints" ? "balloons" : "prints";
    // Hiding the last visible list would leave the station blank.
    if (view[kind] && !view[other]) return;
    view = { ...view, [kind]: !view[kind] };
    save(view);
    apply();
  }

  printsBtn.onclick = () => toggle("prints");
  balloonsBtn.onclick = () => toggle("balloons");
  orderBtn.onclick = () => {
    view = { ...view, printsFirst: !view.printsFirst };
    save(view);
    apply();
  };
  apply();
}
