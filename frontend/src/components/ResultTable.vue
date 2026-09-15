<script setup lang="ts">
import {
  NButton,
  NDataTable,
  NInput,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";
import { storeToRefs } from "pinia";
import { h } from "vue";
import Icon from "./Icon.vue";
import { useScanStore } from "../stores/scan";
import type { Result } from "../types/scan";

const store = useScanStore();
const { filteredResults, filter, results } = storeToRefs(store);
const message = useMessage();

function copyRow(row: Result) {
  const text = [
    row.ip,
    row.origin,
    row.tls,
    row.alpn,
    row.curve,
    row.certLength,
    row.certSignature,
    row.certPublicKey,
    row.certDomain,
    row.certIssuer,
    row.geoCode,
  ].join("\t");
  void navigator.clipboard.writeText(text);
  message.success(`已复制目标 [${row.certDomain || row.ip}] 到剪贴板`);
}

const columns: DataTableColumns<Result> = [
  {
    title: "目标 IP",
    key: "ip",
    width: 140,
    render(row) {
      return h("span", { class: "font-mono font-bold text-ip" }, row.ip);
    },
  },
  {
    title: "证书域名 (Domain)",
    key: "certDomain",
    minWidth: 160,
    ellipsis: { tooltip: true },
    render(row) {
      return h("span", { class: "font-mono text-domain" }, row.certDomain);
    },
  },
  {
    title: "协议版本",
    key: "tls",
    width: 90,
    render(row) {
      return h(
        NTag,
        {
          size: "small",
          bordered: false,
          style: { backgroundColor: "rgba(147, 51, 234, 0.15)", color: "#c084fc", fontWeight: "600" },
        },
        { default: () => row.tls }
      );
    },
  },
  {
    title: "ALPN",
    key: "alpn",
    width: 70,
    render(row) {
      return h(
        NTag,
        {
          size: "small",
          bordered: false,
          style: { backgroundColor: "rgba(16, 185, 129, 0.15)", color: "#34d399", fontWeight: "600" },
        },
        { default: () => row.alpn }
      );
    },
  },
  {
    title: "颁发者机构",
    key: "certIssuer",
    minWidth: 140,
    ellipsis: { tooltip: true },
    render(row) {
      return h("span", { class: "text-issuer" }, row.certIssuer || "未知");
    },
  },
  {
    title: "地区",
    key: "geoCode",
    width: 65,
    render(row) {
      return h(
        NTag,
        {
          size: "small",
          bordered: false,
          style: { backgroundColor: "rgba(255, 255, 255, 0.08)", color: "#e5e7eb", fontWeight: "600" },
        },
        { default: () => row.geoCode || "N/A" }
      );
    },
  },
];
</script>

<template>
  <div class="result-table-wrap">
    <!-- 顶部操作条 -->
    <div class="table-toolbar">
      <div class="search-box">
        <n-input
          v-model:value="filter"
          placeholder="快速筛选 IP / 域名 / 颁发机构 / 地区代码…"
          clearable
          size="small"
        >
          <template #prefix>
            <Icon name="search" :size="13" class="search-icon" />
          </template>
        </n-input>
      </div>

      <div class="toolbar-actions">
        <span v-if="results.length > 0" class="stat-text">
          已显示 {{ filteredResults.length }} / 共 {{ results.length }} 条
        </span>
        <n-button
          v-if="results.length > 0"
          size="tiny"
          quaternary
          title="清空当前列表"
          @click="store.clearResults"
        >
          <template #icon>
            <Icon name="trash" :size="12" />
          </template>
          <span>清空</span>
        </n-button>
        <n-button
          v-if="results.length > 0"
          size="tiny"
          secondary
          title="打开 CSV 结果文件所在文件夹"
          @click="store.openFolder"
        >
          <template #icon>
            <Icon name="folder" :size="12" />
          </template>
          <span>定位文件</span>
        </n-button>
      </div>
    </div>

    <!-- 数据表格 -->
    <div class="table-container">
      <n-data-table
        v-if="results.length > 0"
        :columns="columns"
        :data="filteredResults"
        flex-height
        virtual-scroll
        :row-props="(row: Result) => ({
          onDblclick: () => copyRow(row),
          style: { cursor: 'pointer' },
          title: '双击复制此行数据'
        })"
        class="custom-table"
      />

      <!-- 现代科技感空状态 -->
      <div v-else class="empty-state">
        <div class="radar-box">
          <div class="radar-ring r1"></div>
          <div class="radar-ring r2"></div>
          <div class="radar-core">
            <Icon name="shield" :size="24" />
          </div>
        </div>
        <div class="empty-title">暂无命中扫描结果</div>
        <div class="empty-desc">在左侧填入目标 IP / 域名，点击「开始扫描」，适合 Reality 的节点将实时呈现在这里。</div>
        <div class="tips-pill">💡 双击结果表格中的任意行可一键复制完整信息</div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.result-table-wrap {
  display: flex;
  flex-direction: column;
  height: 100%;
  gap: 10px;
}

.table-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.search-box {
  width: 320px;
}

.search-icon {
  color: var(--text-muted);
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.stat-text {
  font-size: 11px;
  color: var(--text-muted);
}

.table-container {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  position: relative;
}

.custom-table {
  height: 100%;
}

:deep(.font-mono) {
  font-family: Consolas, "SF Mono", monospace;
  font-size: 12px;
}

:deep(.font-bold) {
  font-weight: 600;
}

:deep(.text-ip) {
  color: #38bdf8;
}

:deep(.text-domain) {
  color: #34d399;
}

:deep(.text-issuer) {
  font-size: 11px;
  color: var(--text-secondary);
}

/* 空状态动画图形 */
.empty-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 40px 20px;
}

.radar-box {
  position: relative;
  width: 90px;
  height: 90px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.radar-core {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  background: rgba(16, 185, 129, 0.15);
  color: #10b981;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 2;
  border: 1px solid rgba(16, 185, 129, 0.4);
}

.radar-ring {
  position: absolute;
  border-radius: 50%;
  border: 1px solid rgba(16, 185, 129, 0.25);
  animation: pulse-ring 2.4s cubic-bezier(0.215, 0.61, 0.355, 1) infinite;
}

.radar-ring.r1 {
  width: 70px;
  height: 70px;
}

.radar-ring.r2 {
  width: 90px;
  height: 90px;
  animation-delay: 0.8s;
}

@keyframes pulse-ring {
  0% {
    transform: scale(0.6);
    opacity: 0.8;
  }
  100% {
    transform: scale(1.2);
    opacity: 0;
  }
}

.empty-title {
  font-size: 15px;
  font-weight: 600;
  color: var(--text-primary);
}

.empty-desc {
  font-size: 12px;
  color: var(--text-muted);
  max-width: 380px;
  text-align: center;
  line-height: 1.5;
}

.tips-pill {
  margin-top: 8px;
  font-size: 11px;
  color: var(--text-secondary);
  background: rgba(255, 255, 255, 0.04);
  padding: 4px 12px;
  border-radius: 20px;
  border: 1px solid var(--card-border);
}
</style>
