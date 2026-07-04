/**
 * messageStatus.test.ts — Stage 3 MessageStatus 8 状态机单测
 */

import { describe, expect, it } from "vitest";
import {
  isMessageActive,
  isToolCallActive,
  type MessageStatus,
  MessageStatusColor,
  MessageStatusText,
  migrateMessageStatus,
} from "../types/messageStatus";

describe("migrateMessageStatus", () => {
  it('migrates legacy "pending" to "sending"', () => {
    expect(migrateMessageStatus("pending")).toBe<MessageStatus>("sending");
  });

  it("keeps all new statuses intact", () => {
    const all: MessageStatus[] = ["sending", "sent", "thinking", "streaming", "executing", "complete", "error", "cancelled"];
    for (const s of all) {
      expect(migrateMessageStatus(s)).toBe(s);
    }
  });

  it("handles undefined / empty gracefully", () => {
    expect(migrateMessageStatus(undefined)).toBe("sending");
    expect(migrateMessageStatus("")).toBe("sending");
  });

  it('falls back to "sending" for unknown values', () => {
    expect(migrateMessageStatus("weird-status" as any)).toBe("sending");
  });
});

describe("isMessageActive", () => {
  it("returns true for in-progress statuses", () => {
    expect(isMessageActive("sending")).toBe(true);
    expect(isMessageActive("sent")).toBe(true);
    expect(isMessageActive("thinking")).toBe(true);
    expect(isMessageActive("streaming")).toBe(true);
    expect(isMessageActive("executing")).toBe(true);
  });

  it("returns false for terminal statuses", () => {
    expect(isMessageActive("complete")).toBe(false);
    expect(isMessageActive("error")).toBe(false);
    expect(isMessageActive("cancelled")).toBe(false);
  });
});

describe("isToolCallActive", () => {
  it("returns true for pending and running", () => {
    expect(isToolCallActive("pending")).toBe(true);
    expect(isToolCallActive("running")).toBe(true);
  });

  it("returns false for terminal states", () => {
    expect(isToolCallActive("completed")).toBe(false);
    expect(isToolCallActive("failed")).toBe(false);
    expect(isToolCallActive("cancelled")).toBe(false);
  });
});

describe("MessageStatusColor / Text completeness", () => {
  it("covers all 8 statuses", () => {
    const all: MessageStatus[] = ["sending", "sent", "thinking", "streaming", "executing", "complete", "error", "cancelled"];
    for (const s of all) {
      expect(MessageStatusColor[s]).toBeTruthy();
      expect(MessageStatusText[s]).toBeTruthy();
    }
  });
});
