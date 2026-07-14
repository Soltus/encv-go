import { describe, expect, it } from "vitest";
import { convertBooleanQueryToVectorKeywords } from "../searchQuery";

describe("convertBooleanQueryToVectorKeywords", () => {
  it("AND 视为空格，两个词都保留", () => {
    expect(convertBooleanQueryToVectorKeywords("在线 AND 高清")).toBe("在线 高清");
  });

  it("OR 视为空格，两个词都保留", () => {
    expect(convertBooleanQueryToVectorKeywords("在线 OR 播放")).toBe("在线 播放");
  });

  it("NOT 后的词丢弃", () => {
    expect(convertBooleanQueryToVectorKeywords("在线 NOT 视频")).toBe("在线");
  });

  it("去引号，phrase 当作单个关键词", () => {
    expect(convertBooleanQueryToVectorKeywords('"exact phrase" 高清')).toBe("exact phrase 高清");
  });

  it("去 regex: 前缀和 /.../ 边界，提取词项", () => {
    expect(convertBooleanQueryToVectorKeywords("regex:^photo.*")).toBe("photo");
  });

  it("去括号，扁平化嵌套布尔", () => {
    expect(convertBooleanQueryToVectorKeywords("在线 AND (高清 OR 视频)")).toBe("在线 高清 视频");
  });

  it("无布尔语法时返回原 query", () => {
    expect(convertBooleanQueryToVectorKeywords("普通搜索词")).toBe("普通搜索词");
  });

  it("NOT 掉全部词时返回空字符串", () => {
    expect(convertBooleanQueryToVectorKeywords("NOT 视频")).toBe("");
  });
});
