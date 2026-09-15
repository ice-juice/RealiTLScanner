<script setup lang="ts">
import {
  NButton,
  NCheckbox,
  NCollapse,
  NCollapseItem,
  NInput,
  NInputNumber,
  NTooltip,
} from "naive-ui";
import { storeToRefs } from "pinia";
import Icon from "./Icon.vue";
import { useScanStore } from "../stores/scan";
import type { TargetScope } from "../types/scan";

const store = useScanStore();
const { request, running } = storeToRefs(store);

const scopes: { key: TargetScope; label: string; desc: string }[] = [
  { key: "prefix", label: "BGP 前缀", desc: "扫描种子 IP 所属的 BGP 路由前缀段 (推荐)" },
  { key: "asn", label: "ASN", desc: "扫描该 IP 所在自治系统 (ASN) 宣告的所有前缀" },
  { key: "nearby", label: "邻近 IP", desc: "以种子 IP 为中心向两侧相邻数值连续扩展" },
];
</script>

<template>
  <div class="options-panel">
    <div class="panel-header">
      <span class="section-title">扫描设置</span>
    </div>

    <!-- 2x2 参数网格 -->
    <div class="params-grid">
      <div class="form-item">
        <label class="item-label">
          <span>端口</span>
          <span class="param-tag">-port</span>
        </label>
        <n-input-number
          v-model:value="request.port"
          :disabled="running"
          :min="1"
          :max="65535"
          size="small"
        />
      </div>

      <div class="form-item">
        <label class="item-label">
          <span>握手超时 (秒)</span>
          <span class="param-tag">-timeout</span>
        </label>
        <n-input-number
          v-model:value="request.timeoutSec"
          :disabled="running"
          :min="1"
          :max="120"
          size="small"
        />
      </div>

      <div class="form-item">
        <label class="item-label">
          <span>扫描线程</span>
          <span class="param-tag">-thread</span>
        </label>
        <n-input-number
          v-model:value="request.scanThreads"
          :disabled="running"
          :min="1"
          :max="1000"
          size="small"
        />
      </div>

      <div class="form-item">
        <label class="item-label">
          <span>预探测线程</span>
          <span class="param-tag">probe</span>
        </label>
        <n-input-number
          v-model:value="request.probeThreads"
          :disabled="running"
          :min="1"
          :max="1000"
          size="small"
        />
      </div>
    </div>

    <div v-if="request.scanThreads > 500" class="warn-banner">
      ⚠ 线程数大于 500 可能触发系统最大打开连接数限制
    </div>

    <!-- 目标上限 -->
    <div class="form-item">
      <div class="item-label-row">
        <label class="item-label">
          <span>目标生成上限</span>
          <span class="param-tag">-limit</span>
        </label>
        <span class="label-tip">{{ request.limit === 0 ? '已设为无限连续扫描' : '设为 0 即不限' }}</span>
      </div>
      <n-input-number
        v-model:value="request.limit"
        :disabled="running"
        :min="0"
        size="small"
        placeholder="默认 4096，0 为持续运行"
      />
    </div>

    <!-- 发现范围卡片组 -->
    <div class="form-item">
      <label class="item-label">
        <span>发现范围策略</span>
        <span class="param-tag">-scope</span>
      </label>
      <div class="scope-picker">
        <div
          v-for="s in scopes"
          :key="s.key"
          class="scope-chip"
          :class="{ active: request.scope === s.key }"
          @click="!running && (request.scope = s.key)"
        >
          <div class="scope-chip-title">{{ s.label }}</div>
          <div class="scope-chip-desc">{{ s.desc }}</div>
        </div>
      </div>
    </div>

    <!-- 输出文件 -->
    <div class="form-item">
      <label class="item-label">
        <span>输出文件路径</span>
        <span class="param-tag">-out</span>
      </label>
      <div class="file-picker-row">
        <n-input
          v-model:value="request.outputPath"
          :disabled="running"
          size="small"
          placeholder="默认为文档目录下的 CSV 文件"
        />
        <n-button :disabled="running" size="small" secondary @click="store.pickOutput">
          <Icon name="folder" :size="13" />
        </n-button>
      </div>
    </div>

    <!-- 高级可折叠选项 -->
    <n-collapse arrow-placement="right" class="adv-collapse">
      <n-collapse-item name="advanced">
        <template #header>
          <div class="adv-header">
            <span>高级扫描配置</span>
            <span class="adv-badge">SNI / 探测 / IPv6</span>
          </div>
        </template>

        <div class="adv-body">
          <div class="check-grid">
            <n-checkbox v-model:checked="request.verifySni" :disabled="running">
              <span>SNI 反查复验</span>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <span class="info-bubble">?</span>
                </template>
                提取出证书域名后，将其作为 SNI 发送给目标以核验是否能正常握手
              </n-tooltip>
            </n-checkbox>

            <n-checkbox v-model:checked="request.disablePortProbe" :disabled="running">
              <span>关闭端口预探测</span>
            </n-checkbox>

            <n-checkbox v-model:checked="request.enableIPv6" :disabled="running">
              <span>启用 IPv6 目标</span>
            </n-checkbox>

            <n-checkbox v-model:checked="request.verbose" :disabled="running">
              <span>详细调试日志</span>
            </n-checkbox>
          </div>

          <div class="adv-inputs-grid">
            <div class="form-item">
              <label class="item-label">
                <span>ASN 前缀上限</span>
                <span class="param-tag">-asn-prefixes</span>
              </label>
              <n-input-number
                v-model:value="request.asnMaxPrefixes"
                :disabled="running"
                :min="1"
                size="small"
              />
            </div>

            <div class="form-item">
              <label class="item-label">
                <span>探测超时 (秒)</span>
                <span class="param-tag">-probe-timeout</span>
              </label>
              <n-input-number
                v-model:value="request.probeTimeoutSec"
                :disabled="running"
                :min="1"
                size="small"
              />
            </div>
          </div>
        </div>
      </n-collapse-item>
    </n-collapse>
  </div>
</template>

<style scoped>
.options-panel {
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

/* 2x2 参数网格 */
.params-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px;
}

.form-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.item-label {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  font-weight: 500;
  color: var(--text-secondary);
}

.item-label-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.label-tip {
  font-size: 10px;
  color: var(--accent-color);
}

.param-tag {
  font-size: 10px;
  font-family: Consolas, monospace;
  color: var(--text-muted);
  background: rgba(255, 255, 255, 0.04);
  padding: 0 4px;
  border-radius: 3px;
}

.warn-banner {
  font-size: 11px;
  color: #f59e0b;
  background: rgba(245, 158, 11, 0.1);
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid rgba(245, 158, 11, 0.2);
}

/* 范围选择卡片 */
.scope-picker {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.scope-chip {
  padding: 6px 10px;
  border-radius: 6px;
  background: rgba(0, 0, 0, 0.15);
  border: 1px solid var(--card-border);
  cursor: pointer;
  transition: all 0.15s ease;
}

.scope-chip:hover {
  border-color: rgba(16, 185, 129, 0.3);
  background: rgba(255, 255, 255, 0.03);
}

.scope-chip.active {
  background: var(--accent-soft);
  border-color: var(--accent-color);
}

.scope-chip-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-primary);
}

.scope-chip.active .scope-chip-title {
  color: var(--accent-color);
}

.scope-chip-desc {
  font-size: 10px;
  color: var(--text-muted);
  margin-top: 1px;
  line-height: 1.3;
}

/* 文件选择器 */
.file-picker-row {
  display: flex;
  gap: 6px;
}

/* 折叠高级设置 */
.adv-collapse {
  background: rgba(0, 0, 0, 0.1);
  border-radius: 8px;
  padding: 4px 8px;
  border: 1px solid var(--card-border);
}

.adv-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
}

.adv-badge {
  font-size: 10px;
  color: var(--text-muted);
  background: rgba(255, 255, 255, 0.05);
  padding: 1px 5px;
  border-radius: 3px;
}

.adv-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 4px 0 6px;
}

.check-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  font-size: 12px;
}

.info-bubble {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 13px;
  height: 13px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.1);
  color: var(--text-muted);
  font-size: 9px;
  margin-left: 3px;
  cursor: help;
}

.adv-inputs-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin-top: 4px;
}
</style>
