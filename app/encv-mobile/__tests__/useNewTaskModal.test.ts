import { beforeEach, describe, expect, it, vi } from "vitest";
import { ref } from "vue";

vi.mock("@ionic/vue", () => ({
  modalController: {
    create: vi.fn().mockResolvedValue({
      present: vi.fn(),
      dismiss: vi.fn(),
    }),
  },
}));

vi.mock("@/api/encv", () => ({
  createTask: vi.fn().mockResolvedValue({ id: "task-1" }),
  predictPlugin: vi.fn().mockResolvedValue({ candidates: [] }),
}));

vi.mock("@/composables/useTaskForm", () => ({
  useTaskForm: () => ({
    candidates: ref([]),
    predictedPlugin: ref(null),
    selectedPluginIndex: ref(0),
    versionOptions: ref(undefined),
    extraValues: ref({}),
    visibleExtraFields: ref([]),
    predictPlugin: vi.fn(),
    reset: vi.fn(),
  }),
}));

vi.mock("@/composables/usePathResolver", () => ({
  usePathResolver: () => ({ normalize: (p: string) => p || null }),
}));

vi.mock("@/composables/useI18n", () => ({
  useI18n: () => ({ t: (key: string) => key }),
}));

vi.mock("@/composables/useToast", () => ({
  showToast: vi.fn(),
}));

vi.mock("@/composables/useEventBus", () => ({
  eventBus: { emit: vi.fn() },
}));

vi.mock("@/components/NewTaskModal.vue", () => ({
  default: { name: "NewTaskModal", template: "<div />" },
}));

import { modalController } from "@ionic/vue";
import { createTask } from "@/api/encv";
import { useNewTaskModal } from "@/composables/useNewTaskModal";

const mockedCreate = vi.mocked(createTask);
const mockedModalCreate = vi.mocked(modalController.create);

describe("useNewTaskModal", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // ─── 辅助函数：调用 openNewTask 并提取 componentProps ──────────
  async function openAndExtractProps(sourcePath?: string, taskType?: "encrypt" | "decrypt") {
    const { openNewTask } = useNewTaskModal();
    await openNewTask(sourcePath, taskType);

    expect(mockedModalCreate).toHaveBeenCalledTimes(1);
    const args = mockedModalCreate.mock.calls[0];
    return args[0].componentProps as {
      state: Record<string, unknown>;
      onSubmit: () => Promise<void>;
    };
  }

  describe("pluginName passing", () => {
    it("onSubmit passes pluginName from candidates[selectedPluginIndex].name", async () => {
      const props = await openAndExtractProps("/test.txt", "encrypt");

      props.state.sourcePath = "/test.txt";
      props.state.candidates = [
        { name: "text-plugin", matchType: "extension" as const, priority: 0, taskOptions: null },
        { name: "alist_encrypt", matchType: "general" as const, priority: 1, taskOptions: null },
      ];
      props.state.selectedPluginIndex = 0;

      await props.onSubmit();

      expect(mockedCreate).toHaveBeenCalledTimes(1);
      const callArgs = mockedCreate.mock.calls[0];
      expect(callArgs[0]).toBe("encrypt");
      expect(callArgs[1]).toBe("/test.txt");
      expect(callArgs[5]).toBe("text-plugin");
    });

    it("onSubmit uses candidates[selectedPluginIndex] when index > 0", async () => {
      const props = await openAndExtractProps("/data.bin", "encrypt");

      props.state.sourcePath = "/data.bin";
      props.state.candidates = [
        { name: "first", matchType: "mime" as const, priority: 0, taskOptions: null },
        { name: "second-plugin", matchType: "general" as const, priority: 1, taskOptions: null },
      ];
      props.state.selectedPluginIndex = 1;

      await props.onSubmit();

      expect(mockedCreate).toHaveBeenCalledTimes(1);
      expect(mockedCreate.mock.calls[0][5]).toBe("second-plugin");
    });

    it("onSubmit falls back to predictedPlugin when candidates is empty", async () => {
      const props = await openAndExtractProps("/file.mp4", "encrypt");

      props.state.sourcePath = "/file.mp4";
      props.state.candidates = [];
      props.state.predictedPlugin = "video-plugin";

      await props.onSubmit();

      expect(mockedCreate).toHaveBeenCalledTimes(1);
      expect(mockedCreate.mock.calls[0][5]).toBe("video-plugin");
    });

    it("onSubmit passes undefined for pluginName when no candidate and no prediction", async () => {
      const props = await openAndExtractProps("/unknown.xyz", "decrypt");

      props.state.sourcePath = "/unknown.xyz";
      props.state.candidates = [];
      props.state.predictedPlugin = null;

      await props.onSubmit();

      expect(mockedCreate).toHaveBeenCalledTimes(1);
      expect(mockedCreate.mock.calls[0][5]).toBeUndefined();
    });
  });

  describe("submitting lock (防重复提交)", () => {
    it("rapid double-submit only triggers createTask once", async () => {
      const props = await openAndExtractProps("/rapid.txt", "encrypt");

      props.state.sourcePath = "/rapid.txt";
      props.state.candidates = [{ name: "rapid-plugin", matchType: "extension" as const, priority: 0, taskOptions: null }];
      props.state.selectedPluginIndex = 0;

      const resolvedPromise = Promise.resolve();
      mockedCreate.mockReturnValueOnce(resolvedPromise);

      const p1 = props.onSubmit();
      const p2 = props.onSubmit();

      await Promise.all([p1, p2]);

      expect(mockedCreate).toHaveBeenCalledTimes(1);
    });

    it("submitting lock resets after first submit completes", async () => {
      const props = await openAndExtractProps("/seq.txt", "encrypt");

      props.state.sourcePath = "/seq.txt";
      props.state.candidates = [{ name: "seq-plugin", matchType: "extension" as const, priority: 0, taskOptions: null }];
      props.state.selectedPluginIndex = 0;

      await props.onSubmit();
      expect(mockedCreate).toHaveBeenCalledTimes(1);

      await props.onSubmit();
      expect(mockedCreate).toHaveBeenCalledTimes(2);
    });

    it("onSubmit is no-op when sourcePath is empty", async () => {
      const props = await openAndExtractProps();

      props.state.sourcePath = "";
      props.state.candidates = [{ name: "x", matchType: "extension" as const, priority: 0, taskOptions: null }];

      await props.onSubmit();

      expect(mockedCreate).not.toHaveBeenCalled();
    });
  });

  describe("openNewTask structure", () => {
    it("returns openNewTask function", () => {
      const { openNewTask } = useNewTaskModal();
      expect(typeof openNewTask).toBe("function");
    });

    it("calls modalController.create with NewTaskModal component", async () => {
      const { openNewTask } = useNewTaskModal();
      await openNewTask();

      expect(mockedModalCreate).toHaveBeenCalledTimes(1);
      const createOptions = mockedModalCreate.mock.calls[0][0];
      expect(createOptions.component).toBeDefined();
      expect(createOptions.componentProps).toBeDefined();
      expect(typeof createOptions.componentProps.onSubmit).toBe("function");
      expect(typeof createOptions.componentProps.onUpdateSourcePath).toBe("function");
      expect(typeof createOptions.componentProps.onSelectPlugin).toBe("function");
    });

    it("passes initial sourcePath into state", async () => {
      const { openNewTask } = useNewTaskModal();
      await openNewTask("/init/path.txt", "decrypt");

      const props = mockedModalCreate.mock.calls[0][0].componentProps;
      expect(props.state.sourcePath).toBe("/init/path.txt");
      expect(props.state.taskType).toBe("decrypt");
    });
  });
});
