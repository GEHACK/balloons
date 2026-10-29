import type { Client } from "@connectrpc/connect";
import type { BalloonService } from "./gen/balloons/v1/balloons_pb.js";
import type { Balloon, PrintJob } from "./gen/balloons/v1/balloons_pb.js";
import {
  makeBalloonCircle,
  makePrintIcon,
  printJobCode,
  printJobTitle,
} from "./render.js";

const UNDO_MS = 4000;

export interface ScanDeps {
  state: Map<string, Balloon>;
  printJobs: Map<string, PrintJob>;
  client: Client<typeof BalloonService>;
  scanForm: HTMLFormElement;
  scanInput: HTMLInputElement;
  scanHint: HTMLElement;
  toastStack: HTMLElement;
}

export function initScan(deps: ScanDeps) {
  const { state, printJobs, client, scanForm, scanInput, scanHint, toastStack } =
    deps;
  const pendingDeliveries = new Set<string>();

  type Scanned = { kind: "balloon" | "print"; id: string };

  // Balloon tickets encode the bare balloon id (or, on the phone QR, a
  // /scan?id= URL); cuproxy cover sheets encode "P<print job id>".
  function parseScan(raw: string): Scanned | null {
    const s = raw.trim();
    if (!s) return null;
    if (/^\d+$/.test(s)) return { kind: "balloon", id: s };
    const print = /^p(\d+)$/i.exec(s);
    if (print) return { kind: "print", id: print[1] };
    try {
      const id = new URL(s, "http://x").searchParams.get("id");
      if (id && /^\d+$/.test(id)) return { kind: "balloon", id };
    } catch {}
    return null;
  }

  // Deliverable is what the undo toast needs from a scanned item, whether
  // it's a balloon ticket or a print cover sheet.
  interface Deliverable {
    key: string; // unique across kinds
    icon: () => HTMLElement;
    title: string;
    sub: string;
    // Checked again when the undo window closes: the stream may have flipped
    // it to done meanwhile (another runner, another tab).
    isDone: () => boolean;
    commit: () => Promise<unknown>;
  }

  function balloonDeliverable(b: Balloon): Deliverable {
    const idStr = b.id.toString();
    return {
      key: `b${idStr}`,
      icon: () => makeBalloonCircle(b, "sm"),
      title: b.teamName,
      sub: `Problem ${b.problemLabel}`,
      isDone: () => !!state.get(idStr)?.done,
      commit: () => client.markDone({ balloonId: b.id }),
    };
  }

  function printDeliverable(j: PrintJob): Deliverable {
    const idStr = j.id.toString();
    return {
      key: `p${idStr}`,
      icon: () => makePrintIcon("sm"),
      title: printJobTitle(j),
      sub: `Print ${printJobCode(j)}`,
      isDone: () => !!printJobs.get(idStr)?.done,
      commit: () => client.markPrintJobDone({ printJobId: j.id }),
    };
  }

  function flashHint(msg: string, kind: "ok" | "err") {
    scanHint.textContent = msg;
    scanHint.classList.remove("hidden", "text-emerald-400", "text-red-400");
    scanHint.classList.add(kind === "ok" ? "text-emerald-400" : "text-red-400");
    window.setTimeout(() => scanHint.classList.add("hidden"), 2500);
  }

  function showDeliverToast(d: Deliverable) {
    if (pendingDeliveries.has(d.key)) return;
    pendingDeliveries.add(d.key);

    const toast = document.createElement("div");
    toast.className =
      "flex items-start gap-3 rounded-lg border border-emerald-700/50 bg-emerald-950/90 px-4 py-3 text-sm text-emerald-100 shadow-lg backdrop-blur light:border-emerald-300 light:bg-emerald-50 light:text-emerald-900";
    toast.appendChild(d.icon());

    const body = document.createElement("div");
    body.className = "min-w-0 flex-1";
    const title = document.createElement("div");
    title.className = "truncate font-semibold";
    title.textContent = `Delivering: ${d.title}`;
    body.appendChild(title);
    const sub = document.createElement("div");
    sub.className = "text-xs text-emerald-300/80 light:text-emerald-700";
    sub.textContent = d.sub;
    body.appendChild(sub);
    toast.appendChild(body);

    const cancelBtn = document.createElement("button");
    cancelBtn.type = "button";
    cancelBtn.className =
      "shrink-0 rounded-md border border-emerald-700 bg-emerald-900/40 px-2 py-1 text-xs font-medium hover:bg-emerald-900/70 light:border-emerald-400 light:bg-white light:text-emerald-800 light:hover:bg-emerald-100";
    toast.appendChild(cancelBtn);
    toastStack.appendChild(toast);

    const startedAt = Date.now();
    const updateLabel = () => {
      const remaining = Math.max(0, UNDO_MS - (Date.now() - startedAt));
      cancelBtn.textContent = `Cancel (${Math.ceil(remaining / 1000)}s)`;
    };
    updateLabel();
    const tick = window.setInterval(updateLabel, 200);

    const finish = () => {
      window.clearInterval(tick);
      window.clearTimeout(commit);
      pendingDeliveries.delete(d.key);
      toast.remove();
    };

    cancelBtn.onclick = () => {
      finish();
      flashHint(`Cancelled ${d.title}`, "ok");
    };

    const commit = window.setTimeout(async () => {
      window.clearInterval(tick);
      // Don't fire a redundant mark if it went done while we waited.
      if (d.isDone()) {
        finish();
        return;
      }
      cancelBtn.disabled = true;
      cancelBtn.textContent = "delivering…";
      try {
        await d.commit();
        finish();
      } catch (err) {
        console.error("deliver:", err);
        pendingDeliveries.delete(d.key);
        cancelBtn.disabled = false;
        cancelBtn.textContent = "retry";
        cancelBtn.onclick = () => {
          toast.remove();
          showDeliverToast(d);
        };
      }
    }, UNDO_MS);
  }

  function handleScan(raw: string) {
    const scanned = parseScan(raw);
    if (!scanned) {
      flashHint("Unrecognized scan", "err");
      return;
    }
    let d: Deliverable;
    if (scanned.kind === "print") {
      const j = printJobs.get(scanned.id);
      if (!j) {
        flashHint(`Unknown print P${scanned.id}`, "err");
        return;
      }
      d = printDeliverable(j);
    } else {
      const b = state.get(scanned.id);
      if (!b) {
        flashHint(`Unknown balloon #${scanned.id}`, "err");
        return;
      }
      d = balloonDeliverable(b);
    }
    if (d.isDone()) {
      flashHint(`Already delivered: ${d.title}`, "err");
      return;
    }
    showDeliverToast(d);
  }

  scanForm.addEventListener("submit", (e) => {
    e.preventDefault();
    const v = scanInput.value;
    scanInput.value = "";
    handleScan(v);
  });

  // Hand scanners act as keyboards, so the scan input has to stay focused —
  // but don't steal focus from interactive elements (buttons, other inputs),
  // otherwise tabbing and clicks land somewhere the user didn't intend.
  function focusScan() {
    const a = document.activeElement;
    if (a === scanInput) return;
    if (a && a !== document.body) return;
    scanInput.focus();
  }
  focusScan();
  scanInput.addEventListener("blur", () => {
    window.setTimeout(focusScan, 0);
  });
  window.addEventListener("focus", focusScan);
  document.addEventListener("click", () => {
    window.setTimeout(focusScan, 0);
  });

  // Last-resort catch: focus can still sit on a button (clicked Reprint, tabbed
  // there) when a scan arrives, and focusScan deliberately won't steal it. So
  // redirect the keystrokes instead — a printable character typed outside a
  // text field means someone/something is scanning, so it belongs in the scan
  // box. Enter/Tab/arrows are left alone so buttons stay usable, and the scan
  // suffix then lands on the input we just focused.
  function isTextEntry(el: Element | null): boolean {
    if (!el) return false;
    if (el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement)
      return true;
    if (el instanceof HTMLInputElement) return true;
    return el instanceof HTMLElement && el.isContentEditable;
  }

  document.addEventListener("keydown", (e) => {
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    if (e.key.length !== 1 || e.key === " ") return;
    if (isTextEntry(e.target as Element | null)) return;
    e.preventDefault();
    scanInput.focus();
    scanInput.value += e.key;
  });
}
