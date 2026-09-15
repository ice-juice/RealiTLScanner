<script setup lang="ts">
import {
  NConfigProvider,
  NMessageProvider,
  darkTheme,
  useOsTheme,
  type GlobalThemeOverrides,
} from "naive-ui";
import { storeToRefs } from "pinia";
import { computed, onMounted, ref } from "vue";
import Icon from "./components/Icon.vue";
import LogConsole from "./components/LogConsole.vue";
import OptionsPanel from "./components/OptionsPanel.vue";
import ResultTable from "./components/ResultTable.vue";
import StatusBar from "./components/StatusBar.vue";
import TargetPanel from "./components/TargetPanel.vue";
import { useScanStore } from "./stores/scan";

const store = useScanStore();
const { theme, unreadLogs, results, geo } = storeToRefs(store);
const osTheme = useOsTheme();
const activeTab = ref<"results" | "logs">("results");

const isDark = computed(() => {
  if (theme.value === "light") return false;
  if (theme.value === "dark") return true;
  return osTheme.value === "dark";
});

const themeOverrides = computed<GlobalThemeOverrides>(() => {
  if (!isDark.value) {
    // 浅色主题覆盖
    return {
      common: {
        primaryColor: "#059669",
        primaryColorHover: "#10b981",
        primaryColorPressed: "#047857",
        primaryColorSuppl: "rgba(5, 150, 105, 0.1)",
        bodyColor: "#f3f4f6",
        cardColor: "#ffffff",
        modalColor: "#ffffff",
        popoverColor: "#ffffff",
        textColorBase: "#111827",
        textColor1: "#1f2937",
        textColor2: "#4b5563",
        textColor3: "#9ca3af",
        borderColor: "#e5e7eb",
        borderRadius: "8px",
      },
      Input: {
        color: "#ffffff",
        colorFocus: "#ffffff",
        border: "1px solid #d1d5db",
        borderHover: "1px solid #059669",
        borderFocus: "1px solid #059669",
        boxShadowFocus: "0 0 0 2px rgba(5, 150, 105, 0.15)",
        textColor: "#111827",
        placeholderColor: "#9ca3af",
        borderRadius: "8px",
      },
      InputNumber: {
        buttonColor: "#f3f4f6",
        buttonColorHover: "#e5e7eb",
      },
      DataTable: {
        thColor: "#f9fafb",
        thTextColor: "#4b5563",
        borderColor: "#f3f4f6",
        tdColorHover: "rgba(5, 150, 105, 0.05)",
      },
    };
  }

  // 高端深色主题覆盖（Emerald / Slate）
  return {
    common: {
      primaryColor: "#10b981",
      primaryColorHover: "#34d399",
      primaryColorPressed: "#059669",
      primaryColorSuppl: "rgba(16, 185, 129, 0.12)",
      bodyColor: "#0a0d11",
      cardColor: "#121720",
      modalColor: "#121720",
      popoverColor: "#18202c",
      textColorBase: "#f9fafb",
      textColor1: "#f3f4f6",
      textColor2: "#9ca3af",
      textColor3: "#6b7280",
      borderColor: "rgba(255, 255, 255, 0.08)",
      borderRadius: "8px",
    },
    Input: {
      color: "rgba(255, 255, 255, 0.04)",
      colorFocus: "rgba(255, 255, 255, 0.07)",
      border: "1px solid rgba(255, 255, 255, 0.1)",
      borderHover: "1px solid rgba(16, 185, 129, 0.6)",
      borderFocus: "1px solid #10b981",
      boxShadowFocus: "0 0 0 2px rgba(16, 185, 129, 0.2)",
      textColor: "#f3f4f6",
      placeholderColor: "#6b7280",
      borderRadius: "8px",
    },
    InputNumber: {
      buttonColor: "rgba(255, 255, 255, 0.06)",
      buttonColorHover: "rgba(255, 255, 255, 0.12)",
      buttonTextColor: "#9ca3af",
      buttonTextColorHover: "#f3f4f6",
    },
    Button: {
      borderRadiusMedium: "8px",
      borderRadiusSmall: "6px",
      fontWeight: "500",
    },
    DataTable: {
      thColor: "#0e131b",
      thTextColor: "#9ca3af",
      thFontWeight: "600",
      tdColor: "transparent",
      tdColorHover: "rgba(16, 185, 129, 0.06)",
      borderColor: "rgba(255, 255, 255, 0.06)",
      borderRadius: "8px",
    },
    Tabs: {
      tabTextColorLine: "#9ca3af",
      tabTextColorActiveLine: "#10b981",
      tabTextColorHoverLine: "#f3f4f6",
      barColor: "#10b981",
      tabFontWeight: "600",
    },
    Card: {
      color: "#121720",
      borderColor: "rgba(255, 255, 255, 0.08)",
      borderRadius: "10px",
    },
    Collapse: {
      dividerColor: "rgba(255, 255, 255, 0.06)",
    },
    Tag: {
      borderRadius: "6px",
    },
  };
});

onMounted(async () => {
  store.listen();
  await store.loadDefaults();
});
</script>

<template>
  <n-config-provider :theme="isDark ? darkTheme : null" :theme-overrides="themeOverrides">
    <n-message-provider>
      <div class="app-layout" :class="{ 'is-dark': isDark, 'is-light': !isDark }">
        <!-- 顶部导航栏 -->
        <header class="app-header">
          <div class="header-left">
            <div class="brand-badge">
              <Icon name="shield" :size="18" />
            </div>
            <div class="brand-info">
              <span class="brand-title">RealiTLScanner</span>
              <span class="brand-desc">TLS & Reality 节点特征发现套件</span>
            </div>
            <span class="version-tag">v0.2.0</span>
          </div>

          <div class="header-right">
            <!-- GeoIP 状态指示 -->
            <div
              class="status-pill"
              :class="{ 'is-installed': geo.installed, 'is-missing': !geo.installed }"
              :title="geo.installed ? geo.path : '点击下载国家代码库'"
              @click="!geo.installed && store.downloadGeo()"
            >
              <span class="pill-dot"></span>
              <span class="pill-text">{{ geo.installed ? 'GeoIP 已装载' : '未装载 GeoIP (点击下载)' }}</span>
            </div>

            <!-- 输出目录快捷打开 -->
            <button class="nav-action-btn" title="在系统资源管理器中打开输出目录" @click="store.openFolder">
              <Icon name="folder" :size="15" />
              <span>输出目录</span>
            </button>

            <!-- 深浅主题切换 -->
            <button
              class="nav-action-btn"
              :title="isDark ? '切换至明亮模式' : '切换至暗黑模式'"
              @click="store.persistTheme(isDark ? 'light' : 'dark')"
            >
              <Icon :name="isDark ? 'sun' : 'moon'" :size="15" />
              <span>{{ isDark ? '明亮' : '暗黑' }}</span>
            </button>
          </div>
        </header>

        <!-- 主体双栏区域 -->
        <main class="app-main">
          <!-- 左侧配置与操作面板 -->
          <aside class="left-sidebar">
            <div class="panel-card target-card">
              <TargetPanel />
            </div>
            <div class="panel-card options-card">
              <OptionsPanel />
            </div>
          </aside>

          <!-- 右侧视图工作区 -->
          <section class="right-workspace">
            <div class="workspace-card">
              <!-- Workspace 头部导航 Tab 与快捷工具 -->
              <div class="workspace-header">
                <div class="tabs-group">
                  <button
                    class="tab-item"
                    :class="{ active: activeTab === 'results' }"
                    @click="activeTab = 'results'"
                  >
                    <Icon name="list" :size="15" />
                    <span>命中目标</span>
                    <span v-if="results.length > 0" class="tab-badge primary-badge">{{ results.length }}</span>
                  </button>

                  <button
                    class="tab-item"
                    :class="{ active: activeTab === 'logs' }"
                    @click="activeTab = 'logs'"
                  >
                    <Icon name="terminal" :size="15" />
                    <span>实时日志</span>
                    <span v-if="unreadLogs > 0" class="tab-badge dot-badge">{{ unreadLogs > 99 ? '99+' : unreadLogs }}</span>
                  </button>
                </div>
              </div>

              <!-- Workspace 内容切换区 -->
              <div class="workspace-content">
                <ResultTable v-show="activeTab === 'results'" />
                <LogConsole v-show="activeTab === 'logs'" />
              </div>
            </div>
          </section>
        </main>

        <!-- 底部全局状态栏 -->
        <StatusBar />
      </div>
    </n-message-provider>
  </n-config-provider>
</template>

<style>
/* CSS Reset & Variables */
html,
body,
#app {
  margin: 0;
  padding: 0;
  width: 100%;
  height: 100%;
  overflow: hidden;
  user-select: none;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", "PingFang SC",
    "Microsoft YaHei UI", "Noto Sans SC", sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

* {
  box-sizing: border-box;
}

/* 滚动条美化 */
::-webkit-scrollbar {
  width: 6px;
  height: 6px;
}
::-webkit-scrollbar-track {
  background: transparent;
}
::-webkit-scrollbar-thumb {
  background: rgba(150, 150, 150, 0.2);
  border-radius: 4px;
}
::-webkit-scrollbar-thumb:hover {
  background: rgba(150, 150, 150, 0.35);
}
</style>

<style scoped>
.app-layout {
  display: flex;
  flex-direction: column;
  width: 100vw;
  height: 100vh;
  transition: background-color 0.25s ease, color 0.25s ease;
}

/* 深色模式配色 */
.app-layout.is-dark {
  background-color: #090d11;
  color: #f3f4f6;
  --header-bg: #0d1217;
  --header-border: rgba(255, 255, 255, 0.07);
  --card-bg: #121720;
  --card-border: rgba(255, 255, 255, 0.08);
  --text-primary: #f9fafb;
  --text-secondary: #9ca3af;
  --text-muted: #6b7280;
  --accent-color: #10b981;
  --accent-soft: rgba(16, 185, 129, 0.12);
  --nav-btn-bg: rgba(255, 255, 255, 0.05);
  --nav-btn-hover: rgba(255, 255, 255, 0.09);
}

/* 浅色模式配色 */
.app-layout.is-light {
  background-color: #f1f3f5;
  color: #1f2937;
  --header-bg: #ffffff;
  --header-border: rgba(0, 0, 0, 0.08);
  --card-bg: #ffffff;
  --card-border: rgba(0, 0, 0, 0.06);
  --text-primary: #111827;
  --text-secondary: #4b5563;
  --text-muted: #9ca3af;
  --accent-color: #059669;
  --accent-soft: rgba(5, 150, 105, 0.1);
  --nav-btn-bg: rgba(0, 0, 0, 0.04);
  --nav-btn-hover: rgba(0, 0, 0, 0.08);
}

/* 顶部栏 Header */
.app-header {
  height: 52px;
  min-height: 52px;
  background-color: var(--header-bg);
  border-bottom: 1px solid var(--header-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 18px;
  --wails-draggable: drag;
  z-index: 10;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.brand-badge {
  width: 32px;
  height: 32px;
  border-radius: 8px;
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: white;
  box-shadow: 0 2px 10px rgba(16, 185, 129, 0.35);
}

.brand-info {
  display: flex;
  flex-direction: column;
}

.brand-title {
  font-size: 15px;
  font-weight: 700;
  letter-spacing: -0.2px;
  color: var(--text-primary);
}

.brand-desc {
  font-size: 11px;
  color: var(--text-muted);
}

.version-tag {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
  background: var(--accent-soft);
  color: var(--accent-color);
  margin-left: 2px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
  --wails-draggable: no-drag;
}

.status-pill {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  padding: 4px 10px;
  border-radius: 20px;
  background: var(--nav-btn-bg);
  border: 1px solid var(--card-border);
  cursor: default;
  transition: all 0.2s;
}

.status-pill.is-installed .pill-dot {
  background-color: #10b981;
  box-shadow: 0 0 6px rgba(16, 185, 129, 0.6);
}

.status-pill.is-missing {
  cursor: pointer;
}
.status-pill.is-missing .pill-dot {
  background-color: #f59e0b;
  box-shadow: 0 0 6px rgba(245, 158, 11, 0.6);
}
.status-pill.is-missing:hover {
  background: rgba(245, 158, 11, 0.1);
  border-color: #f59e0b;
}

.pill-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}

.pill-text {
  color: var(--text-secondary);
  font-weight: 500;
}

.nav-action-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 500;
  padding: 6px 12px;
  border-radius: 6px;
  background: var(--nav-btn-bg);
  border: 1px solid var(--card-border);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s ease;
}

.nav-action-btn:hover {
  background: var(--nav-btn-hover);
  color: var(--text-primary);
  border-color: rgba(16, 185, 129, 0.3);
}

/* 主体区域 */
.app-main {
  flex: 1;
  display: flex;
  padding: 12px 16px;
  gap: 14px;
  min-height: 0;
  overflow: hidden;
}

/* 左侧控制栏 */
.left-sidebar {
  width: 380px;
  min-width: 380px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  overflow-y: auto;
  padding-right: 2px;
}

.panel-card {
  background-color: var(--card-bg);
  border: 1px solid var(--card-border);
  border-radius: 10px;
  padding: 14px 16px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.target-card {
  flex-shrink: 0;
}

.options-card {
  flex: 1;
}

/* 右侧工作区 */
.right-workspace {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  height: 100%;
}

.workspace-card {
  flex: 1;
  background-color: var(--card-bg);
  border: 1px solid var(--card-border);
  border-radius: 10px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
}

.workspace-header {
  height: 46px;
  min-height: 46px;
  padding: 0 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--header-border);
  background: rgba(0, 0, 0, 0.02);
}

.tabs-group {
  display: flex;
  gap: 6px;
}

.tab-item {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 6px 14px;
  border-radius: 6px;
  background: transparent;
  border: none;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s ease;
}

.tab-item:hover {
  color: var(--text-primary);
  background: var(--nav-btn-bg);
}

.tab-item.active {
  color: var(--accent-color);
  background: var(--accent-soft);
}

.tab-badge {
  font-size: 11px;
  padding: 1px 6px;
  border-radius: 10px;
  font-weight: 700;
}

.primary-badge {
  background: var(--accent-color);
  color: white;
}

.dot-badge {
  background: #ef4444;
  color: white;
}

.workspace-content {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: 12px 14px;
  overflow: hidden;
}
</style>
