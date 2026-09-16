import { describe, it, expect, vi, beforeEach } from "vitest";
import { ref } from "vue";
import {
  useWebDavAutomationTests,
  WEBDAV_WORKFLOW_DEF,
  WEBDAV_WORKFLOW_DEF_ID,
} from "@encv/shared-components/composables/useWebDavWorkflowAdapter";
import { WEBDAV_TEST_MODULES } from "@encv/shared-components/composables/useWebDavTestModules";

// shared service.runs ref — captured so we can assert on persistence
const runsRef = ref<any[]>([]);

vi.mock("@encv/shared-components/api/mockGenerator", () => ({
  generateMockFilesViaBackend: vi.fn().mockResolvedValue({ count: 0, skipped: 0, totalSize: 0 }),
  resetMockFilesViaBackend: vi.fn().mockResolvedValue({ removed: 0 }),
}));

vi.mock("@encv/shared-components/composables/useWorkflowTaskService", () => ({
  useWorkflowTaskService: () => ({ runs: runsRef }),
}));

vi.mock("@encv/shared-components/composables/useWebDavManifest", () => ({
  useWebDavManifest: () => ({
    manifest: ref({}),
    loading: ref(false),
    error: ref(null),
    auth: ref({}),
    webdavPath: ref("/dav"),
    serverBaseUrl: ref("http://localhost"),
    activeMount: ref({
      webdav_path: "/dav",
      name: "automation",
      manifest: { virtual_tree: [], container_map: [], registered_container_exts: [] },
    }),
    activeMountName: ref("automation"),
    availableMounts: ref([]),
    isReady: ref(true),
    refresh: vi.fn().mockResolvedValue(undefined),
    setActiveMount: vi.fn(),
  }),
}));

vi.mock("@encv/shared-components/composables/useWebDavTestRunner", () => ({
  useWebDavTestRunner: () => ({
    runCase: vi.fn().mockResolvedValue({ id: "c1", name: "c1", module: "m", status: "success", durationMs: 1 }),
  }),
}));

vi.mock("@encv/shared-components/composables/useI18n", () => ({
  useI18n: () => ({ t: (k: string) => k }),
}));

describe("WEBDAV_WORKFLOW_DEF", () => {
  it("has one job per module with sequential needs", () => {
    expect(WEBDAV_WORKFLOW_DEF.jobs.length).toBe(WEBDAV_TEST_MODULES.length);
    expect(WEBDAV_WORKFLOW_DEF.jobs[0].needs).toBeUndefined();
    for (let i = 1; i < WEBDAV_WORKFLOW_DEF.jobs.length; i++) {
      expect(WEBDAV_WORKFLOW_DEF.jobs[i].needs).toEqual([WEBDAV_TEST_MODULES[i - 1].id]);
    }
  });
});

describe("useWebDavAutomationTests", () => {
  beforeEach(() => {
    runsRef.value = [];
  });

  it("returns the expected shape with 8 module states", () => {
    const api = useWebDavAutomationTests();
    expect(api.modules).toBe(WEBDAV_TEST_MODULES);
    expect(Object.keys(api.moduleStates).length).toBe(WEBDAV_TEST_MODULES.length);
    expect(typeof api.runModule).toBe("function");
    expect(typeof api.runAll).toBe("function");
    expect(typeof api.cancelModule).toBe("function");
    expect(typeof api.clearHistory).toBe("function");
    expect(typeof api.resetModule).toBe("function");
  });

  it("resetModule returns a module to idle", () => {
    const api = useWebDavAutomationTests();
    const id = WEBDAV_TEST_MODULES[0].id;
    api.moduleStates[id].value = { status: "done", results: [{ id: "x" }] as any };
    api.resetModule(id);
    expect(api.moduleStates[id].value.status).toBe("idle");
    expect(api.moduleStates[id].value.results).toEqual([]);
  });

  it("runModule executes cases and persists a WebDAV record", async () => {
    const api = useWebDavAutomationTests();
    const id = WEBDAV_TEST_MODULES[0].id;
    await api.runModule(id);
    expect(api.moduleStates[id].value.status).toBe("done");
    expect(runsRef.value.length).toBe(1);
    expect(runsRef.value[0].workflowRun?.workflowDefId).toBe(WEBDAV_WORKFLOW_DEF_ID);
  });

  it("clearHistory removes only WebDAV records", async () => {
    const api = useWebDavAutomationTests();
    const id = WEBDAV_TEST_MODULES[0].id;
    await api.runModule(id);
    expect(runsRef.value.length).toBe(1);
    api.clearHistory();
    expect(runsRef.value.length).toBe(0);
    expect(api.historyRuns.value).toEqual([]);
  });
});
