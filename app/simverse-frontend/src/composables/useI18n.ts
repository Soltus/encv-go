import { ref, computed } from "vue";

const currentLocale = ref("zh-CN");

const zhCN: Record<string, string> = {
  "tabs.home": "首页",
  "tabs.world": "世界",
  "tabs.settings": "设置",
  "tabs.devlogs": "日志",
  "common.back": "返回",
  "simverse.home.title": "SimVerse",
  "simverse.home.subtitle": "一个可交互的物理沙盒世界",
  "simverse.home.enterWorld": "进入世界",
  "simverse.home.chronicle": "事件编年史",
  "simverse.settings.title": "设置",
  "simverse.settings.physics": "物理",
  "simverse.settings.gravity": "重力",
  "simverse.settings.fps": "帧率",
  "simverse.settings.graphics": "图形",
  "simverse.settings.debugMode": "调试模式",
  "simverse.settings.showFps": "显示 FPS",
  "simverse.devlogs.title": "开发日志",
  "simverse.devlogs.search": "搜索日志...",
  "simverse.devlogs.frontend": "前端日志",
  "simverse.devlogs.backend": "后端日志",
  "simverse.devlogs.noLogs": "暂无日志",
};

const translations: Record<string, Record<string, string>> = {
  "zh-CN": zhCN,
};

export function useI18n() {
  const locale = computed(() => currentLocale.value);

  function t(key: string): string {
    const dict = translations[currentLocale.value] || zhCN;
    return dict[key] || key;
  }

  function setLocale(l: string) {
    currentLocale.value = l;
  }

  return { t, locale, setLocale, currentLocale };
}
