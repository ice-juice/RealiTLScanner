<script setup lang="ts">
import { NButton, NProgress } from "naive-ui";
import { storeToRefs } from "pinia";
import { computed } from "vue";
import Icon from "./Icon.vue";
import { useScanStore } from "../stores/scan";

const store = useScanStore();
const { state, progress, running, error, request } = storeToRefs(store);

const percent = computed(() => {
  if (!progress.value.Total) return running.value ? 20 : 0;
  return Math.min(100, Math.round((progress.value.Generated / progress.value.Total) * 100));
});

const elapsed = computed(() => {
  const ms = Number(progress.value.Elapsed) || 0;
  const sec = Math.floor(ms / 1e9 || ms / 1000);
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
});

const stateConfig: Record<string, { label: string; dotClass: string }> = {
  idle: { label: "就绪", dotClass: "dot-idle" },
  planning: { label: "正在解析路由前缀…", dotClass: "dot-pulse" },
  scanning: { label: "正在高速扫描…", dotClass: "dot-pulse" },
  stopping: { label: "正在安全收尾…", dotClass: "dot-warn" },
  completed: { label: "扫描已完成", dotClass: "dot-done" },
  cancelled: { label: "已由用户停止", dotClass: "dot-idle" },
  failed: { label: "扫描异常中断", dotClass: "dot-fail" },
};

const currentStatus = computed(() => stateConfig[state.value] || { label: state.value, dotClass: "dot-idle" });

const fileName = computed(() => {
  const p = request.value.outputPath;
  if (!p) return "";
  const parts = p.split(/[\\/]/);
  return parts[parts.length - 1];
});
</script>

<template>
  <footer class="app-statusbar">
    <div class="status-left">
      <span class="status-dot" :class="currentStatus.dotClass"></span>
      <span class="status-label">{{ currentStatus.label }}</span>
      <span v-if="error" class="status-error">{{ error }}</span>
    </div>

    <div class="status-metrics">
      <!-- 进度条 -->
      <div v-if="running || progress.Generated > 0" class="progress-box">
        <n-progress
          type="line"
          :percentage="percent"
          :show-indicator="false"
          :height="5"
          color="#10b981"
          rail-color="rgba(255, 255, 255, 0.08)"
          style="width: 120px"
        />
        <span class="metric-val">{{ percent }}%</span>
      </div>

      <div class="metric-item">
        <span class="metric-name">生成:</span>
        <span class="metric-val">{{ progress.Generated }} / {{ progress.Total || '∞' }}</span>
      </div>

      <div class="metric-item">
        <span class="metric-name">TCP通过:</span>
        <span class="metric-val">{{ progress.ProbePassed }}</span>
      </div>

      <div class="metric-item highlight-hit">
        <span class="metric-name">命中:</span>
        <span class="metric-val">{{ progress.Found }}</span>
      </div>

      <div class="metric-item">
        <span class="metric-name">速率:</span>
        <span class="metric-val font-mono">{{ (progress.Rate || 0).toFixed(0) }}/s</span>
      </div>

      <div class="metric-item">
        <span class="metric-name">耗时:</span>
        <span class="metric-val font-mono">{{ elapsed }}</span>
      </div>
    </div>

    <div class="status-right">
      <span v-if="fileName" class="file-hint" :title="request.outputPath">
        📄 {{ fileName }}
      </span>

      <n-button
        v-if="running"
        size="tiny"
        type="error"
        secondary
        @click="store.stop"
      >
        <template #icon><Icon name="stop" :size="10" /></template>
        <span>中止</span>
      </n-button>

      <button class="mini-btn" title="在资源管理器中定位输出目录" @click="store.openFolder">
        <Icon name="folder" :size="12" />
        <span>目录</span>
      </button>
    </div>
  </footer>
</template>

<style scoped>
.app-statusbar {
  height: 34px;
  min-height: 34px;
  background-color: var(--header-bg);
  border-top: 1px solid var(--header-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 14px;
  font-size: 11.5px;
  z-index: 10;
}

.status-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  flex-shrink: 0;
}

.dot-idle {
  background-color: #6b7280;
}

.dot-pulse {
  background-color: #10b981;
  box-shadow: 0 0 8px #10b981;
  animation: pulse-dot 1.5s infinite;
}

.dot-warn {
  background-color: #f59e0b;
}

.dot-done {
  background-color: #10b981;
}

.dot-fail {
  background-color: #ef4444;
}

@keyframes pulse-dot {
  0% {
    transform: scale(0.95);
    box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.7);
  }
  70% {
    transform: scale(1.1);
    box-shadow: 0 0 0 6px rgba(16, 185, 129, 0);
  }
  100% {
    transform: scale(0.95);
    box-shadow: 0 0 0 0 rgba(16, 185, 129, 0);
  }
}

.status-label {
  font-weight: 600;
  color: var(--text-primary);
}

.status-error {
  color: #ef4444;
  margin-left: 6px;
}

.status-metrics {
  display: flex;
  align-items: center;
  gap: 14px;
}

.progress-box {
  display: flex;
  align-items: center;
  gap: 6px;
}

.metric-item {
  display: flex;
  align-items: center;
  gap: 4px;
}

.metric-name {
  color: var(--text-muted);
}

.metric-val {
  color: var(--text-secondary);
  font-weight: 500;
}

.highlight-hit .metric-name {
  color: #10b981;
  font-weight: 600;
}

.highlight-hit .metric-val {
  color: #10b981;
  font-weight: 700;
}

.font-mono {
  font-family: Consolas, monospace;
}

.status-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.file-hint {
  color: var(--text-muted);
  max-width: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mini-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 4px;
  background: var(--nav-btn-bg);
  border: 1px solid var(--card-border);
  color: var(--text-secondary);
  font-size: 11px;
  cursor: pointer;
  transition: all 0.15s;
}

.mini-btn:hover {
  background: var(--nav-btn-hover);
  color: var(--text-primary);
}
</style>
