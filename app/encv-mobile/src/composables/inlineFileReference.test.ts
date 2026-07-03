import { describe, expect, it } from "vitest";
import { parseFileReferences } from "./inlineFileReference";

describe("parseFileReferences", () => {
  describe("绝对路径（Windows）", () => {
    it("匹配 C:\\foo\\bar.ts:42", () => {
      const refs = parseFileReferences("打开 C:\\foo\\bar.ts:42 看看");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("C:\\foo\\bar.ts");
      expect(refs[0]?.line).toBe(42);
      expect(refs[0]?.col).toBeUndefined();
    });

    it("匹配 C:/foo/bar.ts:42:5（行+列）", () => {
      const refs = parseFileReferences("error at C:/foo/bar.ts:42:5");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("C:/foo/bar.ts");
      expect(refs[0]?.line).toBe(42);
      expect(refs[0]?.col).toBe(5);
    });

    it("匹配无行号 C:\\foo\\bar.ts", () => {
      const refs = parseFileReferences("see C:\\foo\\bar.ts 文件");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("C:\\foo\\bar.ts");
      expect(refs[0]?.line).toBeUndefined();
    });
  });

  describe("相对路径", () => {
    it("匹配 ./src/main.go:10:5", () => {
      const refs = parseFileReferences("在 ./src/main.go:10:5 修复");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("./src/main.go");
      expect(refs[0]?.line).toBe(10);
      expect(refs[0]?.col).toBe(5);
    });

    it("匹配 src/utils/index.ts（无 ./ 前缀）", () => {
      const refs = parseFileReferences("查看 src/utils/index.ts:7");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("src/utils/index.ts");
      expect(refs[0]?.line).toBe(7);
    });

    it("匹配 ../shared/types.ts", () => {
      const refs = parseFileReferences("from ../shared/types.ts:1");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("../shared/types.ts");
      expect(refs[0]?.line).toBe(1);
    });
  });

  describe("纯文件名", () => {
    it("匹配 package.json", () => {
      const refs = parseFileReferences("请读 package.json");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("package.json");
      expect(refs[0]?.line).toBeUndefined();
    });

    it("匹配 README.md", () => {
      const refs = parseFileReferences("看 README.md:30 附近");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("README.md");
      expect(refs[0]?.line).toBe(30);
    });

    it("匹配 py 扩展名 main.py", () => {
      const refs = parseFileReferences("run main.py:42 即可");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path).toBe("main.py");
      expect(refs[0]?.line).toBe(42);
    });
  });

  describe("不匹配场景", () => {
    it("无扩展名不匹配（README 无扩展）", () => {
      const refs = parseFileReferences("see README 内容");
      expect(refs).toHaveLength(0);
    });

    it("不识别 URL 末尾的路径段", () => {
      const refs = parseFileReferences("https://example.com/file.ts:1");
      // URL 内的路径不应被识别为本地引用
      expect(refs).toHaveLength(0);
    });

    it("空文本返回空数组", () => {
      expect(parseFileReferences("")).toEqual([]);
    });

    it("普通英文句子不匹配", () => {
      const refs = parseFileReferences("Hello world, this is a normal sentence.");
      expect(refs).toHaveLength(0);
    });

    it("未知扩展名不匹配", () => {
      const refs = parseFileReferences("foo.unknownextension123");
      expect(refs).toHaveLength(0);
    });
  });

  describe("多 file ref in single text", () => {
    it("识别 3 个不同类型的引用", () => {
      const text = "fix src/main.go:10:5 and update C:\\foo\\bar.ts:42, see package.json too";
      const refs = parseFileReferences(text);
      expect(refs).toHaveLength(3);
      expect(refs[0]?.path).toBe("src/main.go");
      expect(refs[0]?.line).toBe(10);
      expect(refs[0]?.col).toBe(5);
      expect(refs[1]?.path).toBe("C:\\foo\\bar.ts");
      expect(refs[1]?.line).toBe(42);
      expect(refs[2]?.path).toBe("package.json");
    });

    it("start/end 索引可用于 segment 切分还原原文", () => {
      const text = "A package.json B src/main.go:5 C";
      const refs = parseFileReferences(text);
      expect(refs).toHaveLength(2);
      expect(refs[0]?.start).toBe(2);
      expect(refs[0]?.end).toBe(14); // 'package.json' = 12 chars
      expect(refs[1]?.start).toBe(17);
      // 验证切分
      const segments: string[] = [];
      let cursor = 0;
      for (const r of refs) {
        segments.push(text.slice(cursor, r.start));
        segments.push(text.slice(r.start, r.end));
        cursor = r.end;
      }
      segments.push(text.slice(cursor));
      expect(segments.join("")).toBe(text);
    });
  });

  describe("大小写不敏感", () => {
    it("README.MD 也能匹配", () => {
      const refs = parseFileReferences("see README.MD");
      expect(refs).toHaveLength(1);
      expect(refs[0]?.path.toLowerCase()).toBe("readme.md");
    });
  });
});
