import { describe, expect, it } from "vitest";

// ============================================================
// 防护性回归测试：Tasks.vue + useNewTaskModal 架构
// 目标：防止 P0-1 (路由损坏) 和 P0-2 (插件逻辑缺失) 回归
// ============================================================

describe("Tasks.vue 防护性回归测试", () => {
  describe("P0-1: 路由完整性保护", () => {
    it("必须使用 useNewTaskModal composable（全局 modalController.create 模式）", () => {
      const fs = require("fs");
      const source = fs.readFileSync(require("path").resolve(__dirname, "../src/views/Tasks.vue"), "utf-8");

      // 必须导入并使用 useNewTaskModal
      expect(source).toMatch(/useNewTaskModal/);
      expect(source).toMatch(/const \{ openNewTask \} = useNewTaskModal\(\)/);

      // 不应该有 inline ion-modal（已迁移到 modalController.create）
      const hasInlineIonModal = source.includes("<ion-modal") && source.includes(":is-open=");
      expect(hasInlineIonModal).toBe(false);
    });

    it("Files.vue 必须通过 FileFeature 架构委托操作（不内联插件逻辑）", () => {
      const fs = require("fs");
      const filesSource = fs.readFileSync(require("path").resolve(__dirname, "../src/views/Files.vue"), "utf-8");

      // Files.vue 必须使用 getAllActions() 从 Feature 系统获取扩展操作
      expect(filesSource).toMatch(/getAllActions/);
      expect(filesSource).toMatch(/useFileFeatures/);

      // 绝不能有内联的 handleEncryptFile/handleDecryptFile（已委托给 Feature actions）
      const hasInlineEncryptHandler = /function handleEncryptFile/.test(filesSource);
      expect(hasInlineEncryptHandler).toBe(false);

      // 绝不能通过 eventBus 中转（跨 tab 依赖会导致未挂载时丢失事件）
      const hasEventBusBridge = /handleEncryptFile.*eventBus\.emit\('open-new-task'/s.test(filesSource);
      expect(hasEventBusBridge).toBe(false);

      // Feature action 实现层（actions.ts）必须使用 useNewTaskModal
      const actionsSource = fs.readFileSync(require("path").resolve(__dirname, "../src/features/alist-encrypt/actions.ts"), "utf-8");
      expect(actionsSource).toMatch(/useNewTaskModal/);
    });

    it("processQueryAction (直链访问) 必须在 onMounted 中处理", () => {
      const fs = require("fs");
      const source = fs.readFileSync(require("path").resolve(__dirname, "../src/views/Tasks.vue"), "utf-8");

      // onMounted 中必须有 query.action === 'new' 的检查
      expect(source).toMatch(/route\.query\.action === 'new'/);

      // 必须在 query 处理后调用 openNewTask
      const mountedBlock = source.substring(source.indexOf("onMounted(() =>"), source.indexOf("onUnmounted(() =>"));
      expect(mountedBlock).toContain("openNewTask(");
    });
  });

  describe("P0-2: 插件逻辑完整性保护（委托给 useNewTaskModal）", () => {
    it("useNewTaskModal 必须封装完整的插件预测流程", () => {
      const fs = require("fs");
      const source = fs.readFileSync(require("path").resolve(__dirname, "../src/composables/useNewTaskModal.ts"), "utf-8");

      // 必须导入 useTaskForm
      expect(source).toMatch(/useTaskForm/);

      // 必须解构出 doPredict (predictPlugin)
      expect(source).toMatch(/predictPlugin:\s*doPredict/);

      // 必须导入 usePathResolver
      expect(source).toMatch(/usePathResolver/);

      // openNewTask 函数必须在有 initialSourcePath 时调用 doPredict
      expect(source).toMatch(/doPredict\(normalized/);
      expect(source).toMatch(/normalize\(/);
    });

    it("NewTaskModal 组件必须包含插件选择 UI", () => {
      const fs = require("fs");
      const source = fs.readFileSync(require("path").resolve(__dirname, "../src/components/NewTaskModal.vue"), "utf-8");

      // 必须有插件提示或选择器
      const hasPluginUI =
        (source.includes("candidates.length") || source.includes("cands.length")) &&
        (source.includes("predictedPlugin") || source.includes("pluginName")) &&
        (source.includes("plugin-hint") || source.includes("plugin-selector"));

      expect(hasPluginUI).toBe(true);
    });

    it("NewTaskModal 不应包含独立的浏览按钮（去重，源文件浏览由 EncryptBody/DecryptBody 内部 InputWithHistory 提供）", () => {
      const fs = require("fs");
      const source = fs.readFileSync(require("path").resolve(__dirname, "../src/components/NewTaskModal.vue"), "utf-8");

      // 不应有独立"browse-row"块（曾经存在导致两个源文件输入的重复 UI）
      expect(source).not.toMatch(/browse-row/);
      expect(source).not.toMatch(/browse-btn/);
      expect(source).not.toMatch(/browse-hint/);

      // 不应再直接 import FilePickerModal（浏览能力已下沉到 EncryptBody/DecryptBody）
      expect(source).not.toMatch(/import FilePickerModal/);
      expect(source).not.toMatch(/handleBrowse/);
    });

    it("EncryptBody/DecryptBody 必须提供源/目标文件的浏览按钮（InputWithHistory browsable）", () => {
      const fs = require("fs");
      const encryptSource = fs.readFileSync(require("path").resolve(__dirname, "../src/components/EncryptBody.vue"), "utf-8");
      const decryptSource = fs.readFileSync(require("path").resolve(__dirname, "../src/components/DecryptBody.vue"), "utf-8");

      // EncryptBody 应使用 InputWithHistory + browsable
      expect(encryptSource).toMatch(/InputWithHistory/);
      expect(encryptSource).toMatch(/browsable/);
      // DecryptBody 同理
      expect(decryptSource).toMatch(/InputWithHistory/);
      expect(decryptSource).toMatch(/browsable/);
    });
  });
});

describe("架构约束：全局 Modal 架构保护", () => {
  it("禁止 Tasks.vue 包含 inline <ion-modal>", () => {
    const fs = require("fs");
    const source = fs.readFileSync(require("path").resolve(__dirname, "../src/views/Tasks.vue"), "utf-8");

    // Tasks.vue 不应包含 inline ion-modal（新建任务 modal 已迁移到 useNewTaskModal）
    const hasInlineModal = /<ion-modal\s/.test(source) && /:is-open=/.test(source);
    expect(hasInlineModal).toBe(false);
  });

  it("useNewTaskModal 必须使用 modalController.create 创建 NewTaskModal", () => {
    const fs = require("fs");
    const source = fs.readFileSync(require("path").resolve(__dirname, "../src/composables/useNewTaskModal.ts"), "utf-8");

    // 必须使用 modalController.create
    expect(source).toMatch(/modalController\.create\(/);

    // component 必须是 NewTaskModal
    expect(source).toMatch(/component:\s*NewTaskModal/);

    // 必须调用 modal.present()
    expect(source).toMatch(/modal\.present\(\)/);
  });

  it("FAB 按钮必须调用 openNewTask()", () => {
    const fs = require("fs");
    const source = fs.readFileSync(require("path").resolve(__dirname, "../src/views/Tasks.vue"), "utf-8");

    // FAB 按钮的 @click 必须绑定到 openNewTask
    expect(source).toMatch(/@click="openNewTask\(\)"/);
  });
});
