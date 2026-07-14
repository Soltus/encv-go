/**
 * useDisclosure 单测（纯逻辑，无 DOM 依赖）
 */
import { describe, expect, it } from "vitest";
import { useDisclosure } from "../useDisclosure";

describe("useDisclosure", () => {
  it("默认关闭", () => {
    const { isOpen } = useDisclosure();
    expect(isOpen.value).toBe(false);
  });

  it("可初始打开", () => {
    const { isOpen } = useDisclosure(true);
    expect(isOpen.value).toBe(true);
  });

  it("open / close / toggle 行为正确", () => {
    const d = useDisclosure();
    d.open();
    expect(d.isOpen.value).toBe(true);
    d.toggle();
    expect(d.isOpen.value).toBe(false);
    d.toggle();
    expect(d.isOpen.value).toBe(true);
    d.close();
    expect(d.isOpen.value).toBe(false);
  });
});
