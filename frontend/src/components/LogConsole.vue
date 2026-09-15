<script setup lang="ts">
import { NButton, NCheckbox, NInput, NTag } from "naive-ui";
import { storeToRefs } from "pinia";
import { computed, nextTick, ref, watch } from "vue";
import Icon from "./Icon.vue";
import { useScanStore } from "../stores/scan";

const store = useScanStore();
const { logs, unreadLogs } = storeToRefs(store);
const keyword = ref("");
const autoScroll = ref(true);
const box = ref<HTMLElement | null>(null);

const shown = computed(() => {
  const q = keyword.value.trim().toLowerCase();
  if (!q) return logs.value;
  return logs.value.filter(
    (l) => l.Message.toLowerCase().includes(q) || l.Level.toLowerCase().includes(q)
  );
});

watch(logs, async () => {
  if (!autoScroll.value) return;
  await nextTick();
  if (box.value) {
    box.value.scrollTop = box.value.scrollHeight;
  }
});

function tagType(level: string): "default" | "error" | "warning" | "info" {
  if (level === "error") return "error";
  if (level === "warn") return "warning";
  if (level === "debug") return "default";
  return "info";
}

function formatTime(iso: string) {
  if (!iso) return "";
  try {
    const d = new Date(iso);
    return d.toTimeString().split(" ")[0];
  } catch {
    return iso;
  }
}

function markRead() {
  unreadLogs.value = 0;
}
</script>

<template>
  <div class="log-console-wrap" @mouseenter="markRead">
    <!-- 日志工具栏 -->
    <div class="console-toolbar">
      <div class="search-box">
        <n-input
          v-model:value="keyword"
          placeholder="过滤日志消息或级别…"
          clearable
          size="small"
        >
          <template #prefix>
            <Icon name="search" :size="13" class="search-icon" />
          </template>
        </n-input>
      </div>

      <div class="toolbar-right">
        <n-checkbox v-model:checked="autoScroll" size="small">
          <span class="scroll-label">跟随自动滚动</span>
        </n-checkbox>

        <span class="log-count">共 {{ logs.length }} 条</span>

        <n-button
          v-if="logs.length > 0"
          size="tiny"
          quaternary
          title="清空日志控制台"
          @click="store.clearLogs"
        >
          <template #icon>
            <Icon name="trash" :size="12" />
          </template>
          <span>清空</span>
        </n-button>
      </div>
    </div>

    <!-- 终端控制台视口 -->
    <div ref="box" class="console-viewport">
      <div v-if="shown.length === 0" class="console-empty">
        <Icon name="terminal" :size="20" />
        <span>暂无日志输出</span>
      </div>

      <div v-for="(line, idx) in shown" :key="idx" class="console-line">
        <span class="line-no">{{ idx + 1 }}</span>
        <span class="line-time">{{ formatTime(line.Time) }}</span>
        <n-tag :type="tagType(line.Level)" size="tiny" :bordered="false" class="line-badge">
          {{ line.Level.toUpperCase() }}
        </n-tag>
        <span class="line-msg" :class="'msg-' + line.Level">{{ line.Message }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.log-console-wrap {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 10px;
}

.console-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.search-box {
  width: 280px;
}

.search-icon {
  color: var(--text-muted);
}

.toolbar-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.scroll-label {
  font-size: 11px;
  color: var(--text-secondary);
}

.log-count {
  font-size: 11px;
  color: var(--text-muted);
  font-family: Consolas, monospace;
}

/* 现代化暗黑终端视口 */
.console-viewport {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  background: #080b0f;
  border: 1px solid var(--card-border);
  border-radius: 8px;
  padding: 8px 10px;
  font-family: "JetBrains Mono", Consolas, "SF Mono", monospace;
  font-size: 11.5px;
  line-height: 1.6;
}

.console-empty {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--text-muted);
  font-size: 12px;
}

.console-line {
  display: flex;
  align-items: baseline;
  gap: 8px;
  padding: 1px 0;
  white-space: pre-wrap;
  word-break: break-all;
}

.console-line:hover {
  background: rgba(255, 255, 255, 0.03);
}

.line-no {
  min-width: 28px;
  color: rgba(255, 255, 255, 0.15);
  text-align: right;
  user-select: none;
  font-size: 10px;
}

.line-time {
  color: rgba(255, 255, 255, 0.35);
  font-size: 10px;
  user-select: none;
}

.line-badge {
  font-weight: 700;
  font-size: 9px;
  padding: 0 4px;
}

.line-msg {
  color: #e5e7eb;
}

.msg-error {
  color: #f87171;
  font-weight: 600;
}

.msg-warn {
  color: #fbbf24;
}

.msg-debug {
  color: #9ca3af;
}
</style>
