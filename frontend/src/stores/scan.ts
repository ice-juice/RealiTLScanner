import { defineStore } from "pinia";
import { computed, ref } from "vue";
import {
  defaultRequest,
  type GeoStatus,
  type LogEntry,
  type Progress,
  type Result,
  type ScanRequest,
  type ScanState,
} from "../types/scan";
import { hasWails, wailsApp } from "../wails";

const MAX_LOGS = 2000;

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init);
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return res.json() as Promise<T>;
}

export const useScanStore = defineStore("scan", () => {
  const request = ref<ScanRequest>(defaultRequest());
  const state = ref<ScanState>("idle");
  const error = ref("");
  const progress = ref<Progress>({
    Generated: 0,
    Probed: 0,
    ProbePassed: 0,
    Scanned: 0,
    Found: 0,
    Elapsed: 0,
    Rate: 0,
    Total: 0,
  });
  const results = ref<Result[]>([]);
  const logs = ref<LogEntry[]>([]);
  const unreadLogs = ref(0);
  const geo = ref<GeoStatus>({ installed: false, path: "", modTime: "" });
  const theme = ref<"auto" | "light" | "dark">("auto");
  const filter = ref("");

  const running = computed(() =>
    ["planning", "scanning", "stopping"].includes(state.value),
  );
  const filteredResults = computed(() => {
    const q = filter.value.trim().toLowerCase();
    if (!q) return results.value;
    return results.value.filter((r) =>
      [r.ip, r.origin, r.certDomain, r.certIssuer, r.geoCode]
        .join(" ")
        .toLowerCase()
        .includes(q),
    );
  });

  function applyEvent(name: string, data: unknown) {
    if (name === "scan:state") {
      const payload = data as { state: ScanState; error?: string };
      state.value = payload.state;
      error.value = payload.error || "";
    } else if (name === "scan:progress") {
      progress.value = data as Progress;
    } else if (name === "scan:results") {
      results.value.push(...(data as Result[]));
    } else if (name === "scan:logs") {
      const batch = data as LogEntry[];
      logs.value.push(...batch);
      if (logs.value.length > MAX_LOGS) {
        logs.value.splice(0, logs.value.length - MAX_LOGS);
      }
      unreadLogs.value += batch.length;
    } else if (name === "scan:done") {
      const summary = data as { StoppedBy?: ScanState };
      if (summary.StoppedBy) state.value = summary.StoppedBy;
    }
  }

  function listen() {
    if (window.runtime?.EventsOn) {
      for (const name of [
        "scan:state",
        "scan:progress",
        "scan:results",
        "scan:logs",
        "scan:done",
        "geodb:progress",
      ]) {
        window.runtime.EventsOn(name, (data) => applyEvent(name, data));
      }
      return;
    }
    const es = new EventSource("/api/events");
    es.onmessage = (ev) => {
      const msg = JSON.parse(ev.data) as { event: string; data: unknown };
      applyEvent(msg.event, msg.data);
    };
  }

  async function loadDefaults() {
    const app = wailsApp();
    if (hasWails() && app?.GetDefaults) {
      request.value = (await app.GetDefaults()) as ScanRequest;
    } else {
      request.value = await api<ScanRequest>("/api/defaults");
    }
    await refreshGeo();
    if (app?.Theme) {
      const saved = await app.Theme();
      if (saved === "light" || saved === "dark" || saved === "auto") {
        theme.value = saved;
      }
    }
  }

  async function start() {
    error.value = "";
    results.value = [];
    logs.value = [];
    unreadLogs.value = 0;
    const app = wailsApp();
    if (hasWails() && app?.StartScan) {
      await app.StartScan(request.value);
      return;
    }
    await api("/api/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request.value),
    });
  }

  async function stop() {
    const app = wailsApp();
    if (hasWails() && app?.StopScan) {
      await app.StopScan();
      return;
    }
    await api("/api/stop", { method: "POST" });
  }

  async function refreshGeo() {
    const app = wailsApp();
    if (hasWails() && app?.GeoDBStatus) {
      geo.value = (await app.GeoDBStatus()) as GeoStatus;
      return;
    }
    geo.value = await api<GeoStatus>("/api/geodb");
  }

  async function downloadGeo() {
    const app = wailsApp();
    if (hasWails() && app?.DownloadGeoDB) {
      await app.DownloadGeoDB();
    } else {
      await api("/api/geodb/download", { method: "POST" });
    }
    await refreshGeo();
  }

  async function openFolder() {
    const path = request.value.outputPath;
    const app = wailsApp();
    if (hasWails() && app?.OpenOutputFolder) {
      await app.OpenOutputFolder(path);
      return;
    }
    await fetch("/api/open-folder?path=" + encodeURIComponent(path));
  }

  async function pickOutput() {
    const app = wailsApp();
    if (!app?.PickOutputFile) return;
    const path = await app.PickOutputFile();
    if (path) request.value.outputPath = path;
  }

  async function pickInput() {
    const app = wailsApp();
    if (!app?.PickInputFile) return;
    const path = await app.PickInputFile();
    if (path) request.value.targetValue = path;
  }

  async function persistTheme(next: "auto" | "light" | "dark") {
    theme.value = next;
    const app = wailsApp();
    if (app?.SetTheme) await app.SetTheme(next);
  }

  function clearResults() {
    results.value = [];
  }

  function clearLogs() {
    logs.value = [];
    unreadLogs.value = 0;
  }

  return {
    request,
    state,
    error,
    progress,
    results,
    logs,
    unreadLogs,
    geo,
    theme,
    filter,
    running,
    filteredResults,
    listen,
    loadDefaults,
    start,
    stop,
    refreshGeo,
    downloadGeo,
    openFolder,
    pickOutput,
    pickInput,
    persistTheme,
    clearResults,
    clearLogs,
  };
});
