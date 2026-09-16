import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/api/encv", () => ({
  decodeAlistFilename: vi.fn().mockResolvedValue({ plain_name: "", success: false }),
  getAlistEncryptStreamUrl: vi.fn(
    (params: any) => `/api/alist-encrypt/stream?path=${encodeURIComponent(params.path)}&password=${encodeURIComponent(params.password)}`
  ),
}));

vi.mock("@/composables/useI18n", () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock("@/router", () => ({
  default: { push: vi.fn() },
}));

vi.mock("@ionic/vue", () => ({
  alertController: {
    create: vi.fn().mockReturnValue({
      // biome-ignore lint/suspicious/noThenProperty: mock promise then method
      then: (cb: Function) => cb({ present: vi.fn() }),
    }),
  },
}));

const mockOpenNewTask = vi.fn();

vi.mock("@/composables/useNewTaskModal", () => ({
  useNewTaskModal: () => ({
    openNewTask: mockOpenNewTask,
  }),
}));

const TEST_SUFFIX = ".ae";

vi.mock("@/composables/useConfig", () => ({
  getFieldValue: (_keys: string[]) => TEST_SUFFIX,
}));

import type { FileItem } from "@encv/shared-components/api/encv";
import { decodeAlistFilename } from "@encv/shared-components/api/encv";
import { getAlistActions } from "@/features/alist-encrypt/actions";
import { getAlistBadge } from "@/features/alist-encrypt/badge";
import { createAlistEncryptFeature } from "@/features/alist-encrypt/index";
import {
  clearDecodeCache,
  clearPasswordCache,
  getDecodedName,
  getSessionPassword,
  getStreamUrl,
  isAlistEncrypted,
  loadDecodedName,
  setSessionPassword,
} from "@/features/alist-encrypt/useAlistEncrypt";

const aeFile: FileItem = { name: `video${TEST_SUFFIX}`, path: `/media/video${TEST_SUFFIX}`, isDirectory: false };
const normalFile: FileItem = { name: "doc.pdf", path: "/docs/doc.pdf", isDirectory: false };
const dirFile: FileItem = { name: "folder", path: "/folder", isDirectory: true };
const encryptedFile: FileItem = { name: `secret${TEST_SUFFIX}`, path: `/secret${TEST_SUFFIX}`, isDirectory: false, isEncrypted: true };
const upperSuffixFile: FileItem = {
  name: `video${TEST_SUFFIX.toUpperCase()}`,
  path: `/video${TEST_SUFFIX.toUpperCase()}`,
  isDirectory: false,
};
const encContainerFile: FileItem = { name: "data.enc", path: "/data.enc", isDirectory: false, isEncrypted: true };

describe("isAlistEncrypted", () => {
  it("匹配配置后缀的文件 → true", () => {
    expect(isAlistEncrypted(aeFile)).toBe(true);
  });

  it("目录 → false", () => {
    expect(isAlistEncrypted(dirFile)).toBe(false);
  });

  it("isEncrypted=true 但匹配配置后缀 → true（不再排除）", () => {
    expect(isAlistEncrypted(encryptedFile)).toBe(true);
  });

  it("不匹配后缀的普通文件 → false", () => {
    expect(isAlistEncrypted(normalFile)).toBe(false);
  });

  it("大小写不匹配 → false", () => {
    expect(isAlistEncrypted(upperSuffixFile)).toBe(false);
  });
});

describe("LRU Password Cache", () => {
  beforeEach(() => {
    clearPasswordCache();
  });

  it("setSessionPassword + getSessionPassword round-trip", () => {
    setSessionPassword(`/a${TEST_SUFFIX}`, "pass123");
    expect(getSessionPassword(`/a${TEST_SUFFIX}`)).toBe("pass123");
  });

  it("unset path returns undefined", () => {
    expect(getSessionPassword("/nonexistent")).toBeUndefined();
  });

  it("clearPasswordCache removes everything", () => {
    setSessionPassword(`/a${TEST_SUFFIX}`, "p1");
    setSessionPassword(`/b${TEST_SUFFIX}`, "p2");
    clearPasswordCache();
    expect(getSessionPassword(`/a${TEST_SUFFIX}`)).toBeUndefined();
    expect(getSessionPassword(`/b${TEST_SUFFIX}`)).toBeUndefined();
  });
});

describe("Filename Decode Cache", () => {
  let fetchSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    clearDecodeCache();
    fetchSpy = vi.spyOn(globalThis, "fetch");
  });

  afterEach(() => {
    fetchSpy.mockRestore();
  });

  it("loadDecodedName calls API and caches result", async () => {
    vi.mocked(decodeAlistFilename).mockResolvedValueOnce({ plain_name: "movie.mp4", success: true });

    const result = await loadDecodedName(aeFile, "mypass");
    expect(result).toBe("movie.mp4");
    expect(getDecodedName(aeFile.path)).toBe("movie.mp4");
    expect(vi.mocked(decodeAlistFilename)).toHaveBeenCalledTimes(1);
  });

  it("cache hit skips API call", async () => {
    vi.mocked(decodeAlistFilename).mockResolvedValueOnce({ plain_name: "cached.mp4", success: true });

    await loadDecodedName(aeFile, "pass");
    fetchSpy.mockClear();

    const cached = await loadDecodedName(aeFile, "pass");
    expect(cached).toBe("cached.mp4");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("non-AE file returns null", async () => {
    const result = await loadDecodedName(normalFile, "pass");
    expect(result).toBeNull();
  });

  it("success=false returns null", async () => {
    fetchSpy.mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ plain_name: "", success: false }),
    });

    const result = await loadDecodedName(aeFile, "wrongpass");
    expect(result).toBeNull();
  });

  it("API error returns null silently", async () => {
    fetchSpy.mockRejectedValue(new Error("network error"));

    const result = await loadDecodedName(aeFile, "pass");
    expect(result).toBeNull();
  });
});

describe("getAlistBadge", () => {
  it("AE file returns danger badge", () => {
    const badge = getAlistBadge(aeFile);
    expect(badge).toEqual({ text: "AE", color: "danger" });
  });

  it("non-AE file returns null", () => {
    expect(getAlistBadge(normalFile)).toBeNull();
  });
});

describe("getAlistActions", () => {
  it("AE file returns 2 actions", () => {
    const actions = getAlistActions(aeFile);
    expect(actions).toHaveLength(2);
    expect(actions.map(a => a.id)).toEqual(["alist-stream-preview", "alist-decrypt"]);
  });

  it("non-AE file returns encrypt action (isActive expanded)", () => {
    const actions = getAlistActions(normalFile);
    expect(actions).toHaveLength(1);
    expect(actions[0].id).toBe("alist-encrypt");
  });

  it("actions have correct structure", () => {
    const actions = getAlistActions(aeFile);
    const preview = actions.find(a => a.id === "alist-stream-preview")!;
    expect(preview.color).toBe("primary");
    expect(typeof preview.text).toBe("function");
    expect(preview.text()).toBe("alistEncrypt.streamPreview");
  });
});

describe("createAlistEncryptFeature factory", () => {
  it("returns complete FileFeature shape", () => {
    const feat = createAlistEncryptFeature();
    expect(feat.id).toBe("alist-encrypt");
    expect(typeof feat.isActive).toBe("function");
    expect(typeof feat.getBadge).toBe("function");
    expect(typeof feat.getSubtitle).toBe("function");
    expect(typeof feat.getFileActions).toBe("function");
    expect(typeof feat.onActivate).toBe("function");
    expect(typeof feat.onDeactivate).toBe("function");
  });

  it("isActive scope expanded: AE file true, normal file true, dir false", () => {
    const feat = createAlistEncryptFeature();
    expect(feat.isActive(aeFile)).toBe(true);
    expect(feat.isActive(normalFile)).toBe(true);
    expect(feat.isActive(dirFile)).toBe(false);
  });

  it("onDeactivate clears caches", () => {
    setSessionPassword(`/x${TEST_SUFFIX}`, "p");
    const feat = createAlistEncryptFeature();
    feat.onDeactivate?.();
    expect(getSessionPassword(`/x${TEST_SUFFIX}`)).toBeUndefined();
  });
});

describe("getStreamUrl", () => {
  it("returns URL with encoded path and password", () => {
    const url = getStreamUrl(aeFile, "secret");
    expect(url).toContain("/api/alist-encrypt/stream");
    expect(url).toContain("path=");
    expect(url).toContain("password=secret");
  });
});

describe("isActive (expanded scope)", () => {
  it("非目录普通文件返回 true", () => {
    const feat = createAlistEncryptFeature();
    expect(feat.isActive(normalFile)).toBe(true);
  });

  it("AE 后缀加密文件返回 true", () => {
    const feat = createAlistEncryptFeature();
    expect(feat.isActive(aeFile)).toBe(true);
  });

  it("目录文件返回 false", () => {
    const feat = createAlistEncryptFeature();
    expect(feat.isActive(dirFile)).toBe(false);
  });
});

describe("getAlistActions - encrypt action for normal files", () => {
  it("普通文件返回 1 个 encrypt action", () => {
    const actions = getAlistActions(normalFile);
    expect(actions).toHaveLength(1);
    expect(actions[0].id).toBe("alist-encrypt");
  });

  it("encrypt action 有正确的属性", () => {
    const actions = getAlistActions(normalFile);
    const encrypt = actions.find(a => a.id === "alist-encrypt")!;
    expect(encrypt.color).toBe("warning");
    expect(encrypt.text()).toBe("alistEncrypt.encrypt");
  });

  it("encrypt action handler 调用 openNewTask with encrypt type", async () => {
    const actions = getAlistActions(normalFile);
    const encrypt = actions.find(a => a.id === "alist-encrypt")!;
    await encrypt.handler(normalFile);
    expect(mockOpenNewTask).toHaveBeenCalledWith(normalFile.path, "encrypt");
  });
});

describe("getAlistActions - ENCV container decrypt (branch B)", () => {
  it("isEncrypted=true 且非 alist-encrypt 后缀返回 decrypt action", () => {
    const actions = getAlistActions(encContainerFile);
    expect(actions).toHaveLength(1);
    expect(actions[0].id).toBe("alist-decrypt-container");
  });

  it("container decrypt action 调用 openNewTask with decrypt type", async () => {
    const actions = getAlistActions(encContainerFile);
    const decrypt = actions.find(a => a.id === "alist-decrypt-container")!;
    await decrypt.handler(encContainerFile);
    expect(mockOpenNewTask).toHaveBeenCalledWith(encContainerFile.path, "decrypt");
  });
});
