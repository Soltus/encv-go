/**
 * useModal 单测（mock @ionic/vue 的 modalController）
 *
 * 覆盖：
 * 1. openModal 串联 create + present + onDidDismiss，返回完整 dismiss 结果
 * 2. openModal 透传其余 create 选项（breakpoints / cssClass 等）
 * 3. dismiss 透传 data / role
 */
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { useModal } from "../useModal";

const { fakeModal, mockCreate, mockDismiss } = vi.hoisted(() => {
  const fakeModal = {
    present: vi.fn(),
    onDidDismiss: vi.fn(),
    dismiss: vi.fn(),
  };
  return {
    fakeModal,
    mockCreate: vi.fn(),
    mockDismiss: vi.fn(),
  };
});

vi.mock("@ionic/vue", () => ({
  modalController: {
    create: mockCreate,
    dismiss: mockDismiss,
  },
}));

const Dummy = defineComponent({ render: () => h("div") });

beforeEach(() => {
  vi.clearAllMocks();
  fakeModal.present.mockResolvedValue(undefined);
  fakeModal.dismiss.mockResolvedValue(undefined);
  mockCreate.mockResolvedValue(fakeModal);
  mockDismiss.mockResolvedValue(undefined);
});

describe("useModal", () => {
  it("openModal：create + present + onDidDismiss 串联，返回 dismiss 结果", async () => {
    fakeModal.onDidDismiss.mockResolvedValueOnce({ data: "abc", role: "select" });
    const { openModal } = useModal();

    const res = await openModal<string>({ component: Dummy, componentProps: { foo: 1 } });

    expect(fakeModal.present).toHaveBeenCalledTimes(1);
    expect(res).toEqual({ data: "abc", role: "select" });
  });

  it("openModal：透传其余 create 选项（breakpoints）", async () => {
    fakeModal.onDidDismiss.mockResolvedValueOnce({ data: null });
    const { openModal } = useModal();

    await openModal({ component: Dummy, breakpoints: [0, 1], cssClass: "sheet" });

    expect(mockCreate).toHaveBeenCalledTimes(1);
    const arg = mockCreate.mock.calls[0][0];
    expect(arg.breakpoints).toEqual([0, 1]);
    expect(arg.cssClass).toBe("sheet");
    expect(arg.component).toBe(Dummy);
  });

  it("dismiss：透传 data / role", async () => {
    const { dismiss } = useModal();
    await dismiss("payload", "confirm");
    expect(mockDismiss).toHaveBeenCalledWith("payload", "confirm");
  });
});
