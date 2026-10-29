import type { Client } from "@connectrpc/connect";
import type { BalloonService } from "./gen/balloons/v1/balloons_pb.js";
import type { Balloon, PrintJob } from "./gen/balloons/v1/balloons_pb.js";

export function makeBalloonCircle(b: Balloon, size: "sm" | "md"): HTMLElement {
  const sizeClass = size === "sm" ? "h-8 w-8 text-xs" : "h-10 w-10";
  const ball = document.createElement("div");
  ball.className = `flex ${sizeClass} shrink-0 items-center justify-center rounded-full font-bold text-white shadow ring-1 ring-black/20`;
  ball.style.background = b.problemRgb || "#888";
  ball.textContent = b.problemLabel || "?";
  return ball;
}

export function makePrintIcon(size: "sm" | "md"): HTMLElement {
  const sizeClass = size === "sm" ? "h-8 w-8 text-sm" : "h-10 w-10 text-lg";
  const icon = document.createElement("div");
  icon.className = `flex ${sizeClass} shrink-0 items-center justify-center rounded-md bg-zinc-200 text-zinc-900 shadow ring-1 ring-black/20`;
  icon.textContent = "🖨";
  return icon;
}

// printJobCode mirrors server.PrintJobCode: the barcode on the cuproxy cover.
export function printJobCode(j: PrintJob): string {
  return `P${j.id}`;
}

// printJobTitle falls back to the printing machine's IP when cuproxy's team
// lookup came back empty.
export function printJobTitle(j: PrintJob): string {
  return j.teamName || j.requestingIp || "Unknown team";
}

export function printRow(
  j: PrintJob,
  client: Client<typeof BalloonService>,
): HTMLElement {
  const card = document.createElement("div");
  card.className =
    "flex items-center gap-4 rounded-lg border border-zinc-800 bg-zinc-900/40 px-4 py-3 light:border-zinc-200 light:bg-white";
  if (j.done) card.classList.add("opacity-60");

  card.appendChild(makePrintIcon("md"));

  const meta = document.createElement("div");
  meta.className = "min-w-0 flex-1";
  const team = document.createElement("div");
  team.className = "truncate font-semibold";
  team.textContent = printJobTitle(j);
  meta.appendChild(team);
  const sub = document.createElement("div");
  sub.className = "text-sm text-zinc-400 light:text-zinc-500";
  sub.textContent = j.teamName
    ? `Print ${printJobCode(j)} · ${j.requestingIp}`
    : `Print ${printJobCode(j)}`;
  meta.appendChild(sub);
  card.appendChild(meta);

  if (!j.done) {
    const btn = document.createElement("button");
    btn.className =
      "shrink-0 rounded-md border border-emerald-700/50 bg-emerald-900/30 px-3 py-1.5 text-sm font-medium text-emerald-300 hover:bg-emerald-900/60 disabled:cursor-progress disabled:opacity-60 light:border-emerald-300 light:bg-emerald-100 light:text-emerald-800 light:hover:bg-emerald-200";
    btn.textContent = "Deliver";
    btn.onclick = async () => {
      btn.disabled = true;
      btn.textContent = "delivering…";
      try {
        await client.markPrintJobDone({ printJobId: j.id });
      } catch (err) {
        console.error("markPrintJobDone:", err);
        btn.disabled = false;
        btn.textContent = "Deliver";
      }
    };
    card.appendChild(btn);
  }

  return card;
}

export function row(b: Balloon, client: Client<typeof BalloonService>): HTMLElement {
  const card = document.createElement("div");
  card.className =
    "flex items-center gap-4 rounded-lg border border-zinc-800 bg-zinc-900/40 px-4 py-3 light:border-zinc-200 light:bg-white";
  if (b.done) card.classList.add("opacity-60");

  card.appendChild(makeBalloonCircle(b, "md"));

  const meta = document.createElement("div");
  meta.className = "min-w-0 flex-1";

  const team = document.createElement("div");
  team.className = "truncate font-semibold";
  team.textContent = b.teamName;
  meta.appendChild(team);

  if (b.firstSolve) {
    const sub = document.createElement("div");
    sub.className = "text-sm text-zinc-400 light:text-zinc-500";
    sub.textContent = `First team to solve problem ${b.problemLabel}!`;
    meta.appendChild(sub);
  }

  card.appendChild(meta);

  const actions = document.createElement("div");
  actions.className = "flex shrink-0 items-center gap-2";

  const reprintBtn = document.createElement("button");
  reprintBtn.className =
    "rounded-md border border-zinc-700 bg-zinc-800/60 px-3 py-1.5 text-sm font-medium text-zinc-200 hover:bg-zinc-800 disabled:cursor-progress disabled:opacity-60 light:border-zinc-300 light:bg-zinc-100 light:text-zinc-700 light:hover:bg-zinc-200";
  reprintBtn.textContent = "Reprint";
  reprintBtn.title = "Print this ticket again (use if the original got lost)";
  reprintBtn.onclick = async () => {
    reprintBtn.disabled = true;
    reprintBtn.textContent = "reprinting…";
    try {
      await client.reprint({ balloonId: b.id });
    } catch (err) {
      console.error("reprint:", err);
    } finally {
      reprintBtn.disabled = false;
      reprintBtn.textContent = "Reprint";
    }
  };
  actions.appendChild(reprintBtn);

  if (!b.done) {
    const btn = document.createElement("button");
    btn.className =
      "rounded-md border border-emerald-700/50 bg-emerald-900/30 px-3 py-1.5 text-sm font-medium text-emerald-300 hover:bg-emerald-900/60 disabled:cursor-progress disabled:opacity-60 light:border-emerald-300 light:bg-emerald-100 light:text-emerald-800 light:hover:bg-emerald-200";
    btn.textContent = "Deliver";
    btn.onclick = async () => {
      btn.disabled = true;
      btn.textContent = "delivering…";
      try {
        await client.markDone({ balloonId: b.id });
      } catch (err) {
        console.error("markDone:", err);
        btn.disabled = false;
        btn.textContent = "Deliver";
      }
    };
    actions.appendChild(btn);
  }

  card.appendChild(actions);
  return card;
}
