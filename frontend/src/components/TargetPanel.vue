<script setup lang="ts">
import { NButton, NInput } from "naive-ui";
import { storeToRefs } from "pinia";
import Icon from "./Icon.vue";
import { useScanStore } from "../stores/scan";
import type { TargetKind } from "../types/scan";

const store = useScanStore();
const { request, running, state } = storeToRefs(store);

const modes: { key: TargetKind; label: string; icon: 'globe' | 'file' | 'link' }[] = [
  { key: "addr", label: "单个目标", icon: "globe" },
  { key: "file", label: "目标文件", icon: "file" },
  { key: "url", label: "网页抓取", icon: "link" },
];

const hints: Record<TargetKind, { placeholder: string; note: string }> = {
  addr: {
    placeholder: "如 38.80.191.155、107.172.1.0/24 或域名",
    note: "支持单 IP、CIDR 网段或域名，将自动进行 BGP/邻近探测",
  },
  file: {
    placeholder: "C:\\targets.txt (每行一个 IP、CIDR 或域名)",
    note: "从本地文本读取待扫列表，一行一个目标",
  },
  url: {
    placeholder: "https://launchpad.net/ubuntu/+archivemirrors",
    note: "从网页内容中自动正则提取所有可用的域名",
  },
};
</script>

<template>
  <div class="target-panel">
    <div class="panel-header">
      <span class="section-title">扫描目标</span>
      <span class="section-badge">必填项</span>
    </div>

    <!-- 分段切换器 -->
    <div class="segmented-control">
      <button
        v-for="m in modes"
        :key="m.key"
        type="button"
        class="segment-btn"
        :class="{ active: request.targetKind === m.key }"
        :disabled="running"
        @click="request.targetKind = m.key"
      >
        <Icon :name="m.icon" :size="13" />
        <span>{{ m.label }}</span>
      </button>
    </div>

    <!-- 输入框 -->
    <div class="input-wrap">
      <n-input
        v-model:value="request.targetValue"
        :disabled="running"
        :placeholder="hints[request.targetKind].placeholder"
        size="medium"
        clearable
        class="target-input"
        @keydown.enter="!running && store.start()"
      >
        <template #prefix>
          <Icon :name="modes.find((m) => m.key === request.targetKind)?.icon || 'globe'" :size="15" class="input-icon" />
        </template>
        <template v-if="request.targetKind === 'file'" #suffix>
          <button type="button" class="browse-btn" :disabled="running" @click="store.pickInput">
            <Icon name="folder" :size="13" />
            <span>浏览</span>
          </button>
        </template>
      </n-input>
      <span class="input-hint">{{ hints[request.targetKind].note }}</span>
    </div>

    <!-- 开始 / 停止操作按钮 -->
    <div class="action-grid">
      <n-button
        type="primary"
        class="start-btn"
        :loading="running"
        :disabled="running || !request.targetValue.trim()"
        @click="store.start"
      >
        <template #icon>
          <Icon v-if="!running" name="play" :size="14" />
        </template>
        <span>{{ running ? (state === 'planning' ? '正在解析目标…' : '正在扫描…') : '开始扫描' }}</span>
      </n-button>

      <n-button
        type="error"
        class="stop-btn"
        :disabled="!running"
        @click="store.stop"
      >
        <template #icon>
          <Icon name="stop" :size="13" />
        </template>
        <span>停止</span>
      </n-button>
    </div>
  </div>
</template>

<style scoped>
.target-panel {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.3px;
  text-transform: uppercase;
  color: var(--text-primary);
}

.section-badge {
  font-size: 10px;
  font-weight: 600;
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(16, 185, 129, 0.15);
  color: var(--accent-color);
}

/* 分段切换按钮组 */
.segmented-control {
  display: flex;
  background: rgba(0, 0, 0, 0.15);
  padding: 3px;
  border-radius: 8px;
  border: 1px solid var(--card-border);
  gap: 3px;
}

.segment-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 6px 0;
  background: transparent;
  border: none;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.18s ease;
}

.segment-btn:hover:not(:disabled) {
  color: var(--text-primary);
  background: rgba(255, 255, 255, 0.04);
}

.segment-btn.active {
  background: var(--card-bg);
  color: var(--accent-color);
  font-weight: 600;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.2);
}

.segment-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

/* 输入框 */
.input-wrap {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.target-input {
  width: 100%;
}

.input-icon {
  color: var(--text-muted);
}

.browse-btn {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border-radius: 4px;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid var(--card-border);
  font-size: 11px;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s;
}

.browse-btn:hover:not(:disabled) {
  background: rgba(255, 255, 255, 0.15);
  color: var(--text-primary);
}

.input-hint {
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.4;
  padding: 0 2px;
}

/* 按钮组 */
.action-grid {
  display: grid;
  grid-template-columns: 1fr 88px;
  gap: 8px;
  margin-top: 2px;
}

.start-btn {
  height: 38px;
  font-size: 13px;
  font-weight: 600;
  background: linear-gradient(135deg, #10b981 0%, #059669 100%) !important;
  border: none !important;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.3);
}

.start-btn:hover:not(:disabled) {
  background: linear-gradient(135deg, #34d399 0%, #059669 100%) !important;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.45);
}

.stop-btn {
  height: 38px;
  font-size: 13px;
  font-weight: 600;
}
</style>
