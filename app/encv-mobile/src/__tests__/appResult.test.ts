import { describe, expect, it, vi } from "vitest";
import type { AppError } from "../types/appError";
import type { AppResult } from "../types/appResult";
import { appErr, appOk, getOrElse, isFailure, isOk, mapResult, onErr, onOk, runCatching, runCatchingAsync } from "../types/appResult";

// 测试用 helper：构造 AppError
function mkAppError(type: AppError["type"], message = "x"): AppError {
  return { type, message, humanMessage: `📛 ${type}`, isRetryable: false };
}

describe("appOk / appErr", () => {
  it("appOk 构造成功结果", () => {
    const r = appOk(42);
    expect(r.ok).toBe(true);
    if (r.ok) expect(r.data).toBe(42);
  });

  it("appErr 构造失败结果", () => {
    const ae = mkAppError("Unknown");
    const r = appErr<number>(ae);
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.error).toBe(ae);
  });
});

describe("isOk / isFailure 类型守卫", () => {
  it("isOk 正确收窄", () => {
    const r: AppResult<number> = appOk(1);
    if (isOk(r)) {
      // 编译器应能识别 r.data
      expect(r.data).toBe(1);
    } else {
      throw new Error("should be ok");
    }
  });

  it("isFailure 正确收窄", () => {
    const r: AppResult<number> = appErr(mkAppError("RateLimited"));
    if (isFailure(r)) {
      expect(r.error.type).toBe("RateLimited");
    } else {
      throw new Error("should be failure");
    }
  });
});

describe("getOrElse", () => {
  it("Success 返回 data", () => {
    expect(getOrElse(appOk(7), 99)).toBe(7);
  });

  it("Failure 返回 fallback", () => {
    expect(getOrElse(appErr<number>(mkAppError("Unknown")), 99)).toBe(99);
  });
});

describe("mapResult", () => {
  it("Success 走 transform", () => {
    const r = mapResult(appOk(10), n => `n=${n}`);
    expect(isOk(r)).toBe(true);
    if (isOk(r)) expect(r.data).toBe("n=10");
  });

  it("Failure 透传", () => {
    const ae = mkAppError("NetworkTimeout");
    const r = mapResult(appErr<number>(ae), n => n * 2);
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) expect(r.error).toBe(ae);
  });
});

describe("onOk / onErr 副作用钩子", () => {
  it("onOk 在 Success 上执行", () => {
    const spy = vi.fn();
    onOk(appOk(5), spy);
    expect(spy).toHaveBeenCalledWith(5);
  });

  it("onOk 在 Failure 上跳过", () => {
    const spy = vi.fn();
    onOk(appErr<number>(mkAppError("Unknown")), spy);
    expect(spy).not.toHaveBeenCalled();
  });

  it("onErr 在 Failure 上执行", () => {
    const ae = mkAppError("RateLimited");
    const spy = vi.fn();
    onErr(appErr<number>(ae), spy);
    expect(spy).toHaveBeenCalledWith(ae);
  });

  it("onErr 在 Success 上跳过", () => {
    const spy = vi.fn();
    onErr(appOk(5), spy);
    expect(spy).not.toHaveBeenCalled();
  });
});

describe("runCatching (sync)", () => {
  it("正常返回包成 Success", () => {
    const r = runCatching(() => "hello");
    expect(isOk(r)).toBe(true);
    if (isOk(r)) expect(r.data).toBe("hello");
  });

  it("抛 AbortError → UserCancelled", () => {
    const abortErr = new Error("aborted");
    abortErr.name = "AbortError";
    const r = runCatching<string>(() => {
      throw abortErr;
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) {
      expect(r.error.type).toBe("UserCancelled");
      expect(r.error.isRetryable).toBe(false);
    }
  });

  it("抛含 timeout 的 Error → NetworkTimeout", () => {
    const r = runCatching<string>(() => {
      throw new Error("read timeout after 30s");
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) {
      expect(r.error.type).toBe("NetworkTimeout");
      expect(r.error.isRetryable).toBe(true);
    }
  });

  it("抛 connection refused → NetworkUnavailable", () => {
    const r = runCatching<string>(() => {
      throw new Error("dial tcp: connection refused");
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) expect(r.error.type).toBe("NetworkUnavailable");
  });

  it("抛含 401 → ApiKeyInvalid", () => {
    const r = runCatching<string>(() => {
      throw new Error("server returned status 401");
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) expect(r.error.type).toBe("ApiKeyInvalid");
  });

  it("抛含 429 → RateLimited", () => {
    const r = runCatching<string>(() => {
      throw new Error("HTTP 429 Too Many Requests");
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) {
      expect(r.error.type).toBe("RateLimited");
      expect(r.error.isRetryable).toBe(true);
    }
  });

  it("抛含 HTTP 5xx → ServerError", () => {
    const r = runCatching<string>(() => {
      throw new Error("HTTP 502 Bad Gateway");
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) expect(r.error.type).toBe("ServerError");
  });

  it("抛 AppError 实例 → 直接复用", () => {
    const ae = mkAppError("InsufficientBalance", "balance=0");
    const r = runCatching<string>(() => {
      throw ae;
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) {
      expect(r.error).toBe(ae);
      expect(r.error.type).toBe("InsufficientBalance");
    }
  });

  it("抛未知 Error → Unknown", () => {
    const r = runCatching<string>(() => {
      throw new Error("totally weird");
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) {
      expect(r.error.type).toBe("Unknown");
      expect(r.error.message).toBe("totally weird");
    }
  });

  it("抛非 Error 值 → Unknown", () => {
    const r = runCatching<string>(() => {
      throw "plain string"; // eslint-disable-line @typescript-eslint/no-throw-literal
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) {
      expect(r.error.type).toBe("Unknown");
      expect(r.error.message).toBe("plain string");
    }
  });

  it("自动填 humanMessage", () => {
    const r = runCatching<string>(() => {
      throw new Error("HTTP 502");
    });
    if (isFailure(r)) {
      expect(r.error.humanMessage).toBeTruthy();
    }
  });
});

describe("runCatchingAsync", () => {
  it("resolve 的 Promise 包成 Success", async () => {
    const r = await runCatchingAsync(async () => 100);
    expect(isOk(r)).toBe(true);
    if (isOk(r)) expect(r.data).toBe(100);
  });

  it("reject 的 Promise 包成 Failure", async () => {
    const r = await runCatchingAsync(async () => {
      throw new Error("boom");
    });
    expect(isFailure(r)).toBe(true);
    if (isFailure(r)) expect(r.error.type).toBe("Unknown");
  });
});
