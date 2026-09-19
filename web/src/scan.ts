import type { Client } from "@connectrpc/connect";
import type { BalloonService } from "./gen/balloons/v1/balloons_pb.js";
import type { Balloon } from "./gen/balloons/v1/balloons_pb.js";
import { makeBalloonCircle } from "./render.js";

const UNDO_MS = 4000;

export interface ScanDeps {
  state: Map<string, Balloon>;
  client: Client<typeof BalloonService>;
  scanForm: HTMLFormElement;
  scanInput: HTMLInputElement;
  scanHint: HTMLElement;
  toastStack: HTMLElement;
}

export function initScan(deps: ScanDeps) {
  const { state, client, scanForm, scanInput, scanHint, toastStack } = deps;
  const pendingDeliveries = new Set<string>();

  function parseScan(raw: string): string | null {
    const s = raw.trim();
    if (!s) return null;
    if (/^\d+$/.test(s)) return s;
    try {
      const id = new URL(s, "http://x").searchParams.get("id");
      if (id && /^\d+$/.test(id)) return id;
    } catch {}
    return null;
  }

  function flashHint(msg: string, kind: "ok" | "err") {
    scanHint.textContent = msg;
    scanHint.classList.remove("hidden", "text-emerald-400", "text-red-400");
    scanHint.classList.add(kind === "ok" ? "text-emerald-400" : "text-red-400");
    window.setTimeout(() => scanHint.classList.add("hidden"), 2500);
  }

  function showDeliverToast(b: Balloon) {
    const idStr = b.id.toString();
    if (pendingDeliveries.has(idStr)) return;
    pendingDeliveries.add(idStr);

    const toast = document.createElement("div");
    toast.className =
      "flex items-start gap-3 rounded-lg border border-emerald-700/50 bg-emerald-950/90 px-4 py-3 text-sm text-emerald-100 shadow-lg backdrop-blur light:border-emerald-300 light:bg-emerald-50 light:text-emerald-900";
    toast.appendChild(makeBalloonCircle(b, "sm"));

    const body = document.createElement("div");
    body.className = "min-w-0 flex-1";
    const title = document.createElement("div");
    title.className = "truncate font-semibold";
    title.textContent = `Delivering: ${b.teamName}`;
    body.appendChild(title);
    const sub = document.createElement("div");
    sub.className = "text-xs text-emerald-300/80 light:text-emerald-700";
    sub.textContent = `Problem ${b.problemLabel}`;
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
      pendingDeliveries.delete(idStr);
      toast.remove();
    };

    cancelBtn.onclick = () => {
      finish();
      flashHint(`Cancelled ${b.teamName}`, "ok");
    };

    const commit = window.setTimeout(async () => {
      window.clearInterval(tick);
      // Stream may have flipped the balloon to done while we waited (another
      // runner, another tab) — don't fire a redundant MarkDone.
      if (state.get(idStr)?.done) {
        finish();
        return;
      }
      cancelBtn.disabled = true;
      cancelBtn.textContent = "delivering…";
      try {
        await client.markDone({ balloonId: b.id });
        finish();
      } catch (err) {
        console.error("markDone:", err);
        pendingDeliveries.delete(idStr);
        cancelBtn.disabled = false;
        cancelBtn.textContent = "retry";
        cancelBtn.onclick = () => {
          toast.remove();
          showDeliverToast(b);
        };
      }
    }, UNDO_MS);
  }

  function handleScan(raw: string) {
    const id = parseScan(raw);
    if (!id) {
      flashHint("Unrecognized scan", "err");
      return;
    }
    const b = state.get(id);
    if (!b) {
      flashHint(`Unknown balloon #${id}`, "err");
      return;
    }
    if (b.done) {
      flashHint(`Already delivered: ${b.teamName}`, "err");
      return;
    }
    showDeliverToast(b);
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
