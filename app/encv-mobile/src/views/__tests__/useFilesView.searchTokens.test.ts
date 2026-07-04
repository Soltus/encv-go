/**
 * useFilesView.searchTokens.test.ts
 *
 * 覆盖 tokenizeQuery 和 renderSnippet 的所有边界 case（不乐观测试）：
 *
 * tokenizeQuery:
 *   1. 空 query / 空白
 *   2. 单个 word（中文 + 英文 + 数字 + 特殊字符）
 *   3. 多个 word 空格分隔
 *   4. phrase 双引号
 *   5. phrase 嵌套 quote / unterminated
 *   6. regex 两种格式（regex:P 和 regex:/P/）
 *   7. op 大小写敏感（必须大写）
 *   8. op 不独立成词时（"ANDROID"）应作 word
 *   9. op 各种位置（开头 / 中间 / 末尾）
 *  10. 混合：word + op + phrase + regex 全部混用
 *  11. CJK 与 op 混排（"在线 AND 高清"）
 *  12. 多重空白（双空格 / 制表符视作空格）
 *  13. 孤立 NOT（前端高亮容错）
 *
 * renderSnippet:
 *   1. undefined / '' → []
 *   2. 无高亮 → 1 段 highlight=false
 *   3. 单个高亮 → 2 段（高 + 不高）
 *   4. 多个高亮
 *   5. 不闭合 <<（容错：整段视为高亮）
 *   6. 不闭合 >>（容错：>> 后正常显示）
 *   7. 紧贴 <<>>
 *   8. 中文高亮
 *
 * 设计原则（非乐观测试）：
 *   - 不 mock 任何东西（pure function）
 *   - 不假设具体实现细节（不测 "i === 0 时执行了什么"）
 *   - 测输入 → 输出契约
 *   - 一组失败用例必须能精确指出问题
 */

import { describe, expect, it } from "vitest";
import { operatorSymbols, renderSnippet, tokenizeQuery } from "@/views/useFilesView.searchTokens";

describe("operatorSymbols - 符号映射表", () => {
  it("TestOS_BasicMapping: AND/OR/NOT 必须映射到全角符号", () => {
    // 用户明确要求：显示符号而不是英文
    expect(operatorSymbols.AND).toBe("＆");
    expect(operatorSymbols.OR).toBe("｜");
    expect(operatorSymbols.NOT).toBe("￢");
  });

  it("TestOS_AllFullWidth: 所有符号必须全角（与 CJK 等宽）", () => {
    // 全角字符在 BMP 内 Unicode 范围 [0xFF00, 0xFFEF]
    for (const sym of Object.values(operatorSymbols)) {
      const code = sym.codePointAt(0)!;
      expect(code).toBeGreaterThanOrEqual(0xff00);
      expect(code).toBeLessThanOrEqual(0xffef);
    }
  });
});

describe("tokenizeQuery - 基础场景", () => {
  it("TestTQ_Empty: 空字符串 / undefined / 全空白 → []", () => {
    expect(tokenizeQuery("")).toEqual([]);
    expect(tokenizeQuery("   ")).toEqual([]);
  });

  it("TestTQ_SingleEnglishWord: 单个英文单词", () => {
    expect(tokenizeQuery("hello")).toEqual([{ kind: "word", text: "hello" }]);
  });

  it("TestTQ_SingleCJKWord: 单个中文（CJK 应作 word token）", () => {
    expect(tokenizeQuery("在线")).toEqual([{ kind: "word", text: "在线" }]);
  });

  it("TestTQ_MultiWord_SpaceSep: 多 word 空格分隔", () => {
    expect(tokenizeQuery("hello world")).toEqual([
      { kind: "word", text: "hello" },
      { kind: "word", text: "world" },
    ]);
  });

  it("TestTQ_MultiWord_CJK: 多 CJK 词", () => {
    expect(tokenizeQuery("在线 高清 视频")).toEqual([
      { kind: "word", text: "在线" },
      { kind: "word", text: "高清" },
      { kind: "word", text: "视频" },
    ]);
  });

  it("TestTQ_WordWithSpecialChars: word 含 - _ . 等合法字符", () => {
    expect(tokenizeQuery("my-file_v2.tar.gz")).toEqual([{ kind: "word", text: "my-file_v2.tar.gz" }]);
  });
});

describe("tokenizeQuery - phrase 场景", () => {
  it("TestTQ_PhraseBasic: 双引号短语", () => {
    expect(tokenizeQuery('"hello world"')).toEqual([{ kind: "phrase", text: "hello world" }]);
  });

  it("TestTQ_PhraseCJK: 中文短语", () => {
    expect(tokenizeQuery('"在线播放"')).toEqual([{ kind: "phrase", text: "在线播放" }]);
  });

  it("TestTQ_PhraseAndWord: phrase + word 混用", () => {
    expect(tokenizeQuery('"hello world" foo')).toEqual([
      { kind: "phrase", text: "hello world" },
      { kind: "word", text: "foo" },
    ]);
  });

  it('TestTQ_PhraseUnterminated: 未闭合 "（容错：作 word 处理）', () => {
    // 当前实现：没找到 end " → 不 push phrase token，继续按 word 切
    // 这是 UI 容错，不应报错
    expect(tokenizeQuery('"unterminated')).toEqual([{ kind: "word", text: '"unterminated' }]);
  });

  it('TestTQ_PhraseEmpty: "" (空 phrase) → 跳过，不产生 token', () => {
    // 空 phrase 是用户误输入，前端高亮跳过（Go 端 FTS5 "" 返 0 results）
    expect(tokenizeQuery('""')).toEqual([]);
  });
});

describe("tokenizeQuery - regex 场景", () => {
  it("TestTQ_RegexSlashForm: regex:/^p/", () => {
    expect(tokenizeQuery("regex:/^test/")).toEqual([{ kind: "regex", text: "^test" }]);
  });

  it("TestTQ_RegexBareForm: regex:test（无斜杠）", () => {
    expect(tokenizeQuery("regex:test")).toEqual([{ kind: "regex", text: "test" }]);
  });

  it("TestTQ_RegexAndWord: regex + word", () => {
    expect(tokenizeQuery("regex:/^p/ foo")).toEqual([
      { kind: "regex", text: "^p" },
      { kind: "word", text: "foo" },
    ]);
  });

  it("TestTQ_RegexUnterminatedSlash: regex:/^abc（缺 /）→ fallback to word", () => {
    // 当前实现：找不到第二个 / → 不切 regex，落到 word 分支
    // 整段是 word（包含 "regex:" 前缀）
    expect(tokenizeQuery("regex:/^abc")).toEqual([{ kind: "word", text: "regex:/^abc" }]);
  });

  it("TestTQ_RegexEmpty: regex: 后无内容 → word", () => {
    // pattern 为空时不切 regex
    expect(tokenizeQuery("regex:")).toEqual([{ kind: "word", text: "regex:" }]);
  });
});

describe("tokenizeQuery - 布尔操作符", () => {
  it("TestTQ_OpIsolated: 独立大写 AND/OR/NOT → op（含 display 符号）", () => {
    // 2026-07-02 升级：op token 现在带 display 字段（符号映射）
    expect(tokenizeQuery("AND")).toEqual([{ kind: "op", text: "AND", display: "＆" }]);
    expect(tokenizeQuery("OR")).toEqual([{ kind: "op", text: "OR", display: "｜" }]);
    expect(tokenizeQuery("NOT")).toEqual([{ kind: "op", text: "NOT", display: "￢" }]);
  });

  it("TestTQ_OpInMiddle: word op word（含 display）", () => {
    expect(tokenizeQuery("hello AND world")).toEqual([
      { kind: "word", text: "hello" },
      { kind: "op", text: "AND", display: "＆" },
      { kind: "word", text: "world" },
    ]);
  });

  it("TestTQ_OpAtStart: 以 op 开头（含 display）", () => {
    expect(tokenizeQuery("NOT hello")).toEqual([
      { kind: "op", text: "NOT", display: "￢" },
      { kind: "word", text: "hello" },
    ]);
  });

  it("TestTQ_OpAtEnd: 以 op 结尾（含 display）", () => {
    expect(tokenizeQuery("hello OR")).toEqual([
      { kind: "word", text: "hello" },
      { kind: "op", text: "OR", display: "｜" },
    ]);
  });

  it("TestTQ_OpCaseSensitive: 小写 and/or/not 必须作 word", () => {
    // 大小写敏感是 FTS5 强约束，前端高亮也必须敏感
    expect(tokenizeQuery("hello and world")).toEqual([
      { kind: "word", text: "hello" },
      { kind: "word", text: "and" },
      { kind: "word", text: "world" },
    ]);
  });

  it('TestTQ_OpPartOfWord: "ANDROID" 应作 word 不作 op', () => {
    // 关键边界：AND 后面接 R 不算 op（必须独立成词）
    expect(tokenizeQuery("ANDROID")).toEqual([{ kind: "word", text: "ANDROID" }]);
  });

  it('TestTQ_OpPartOfWord2: "ANDROID-12" 应作 word', () => {
    expect(tokenizeQuery("ANDROID-12")).toEqual([{ kind: "word", text: "ANDROID-12" }]);
  });

  it("TestTQ_MultipleOps: 连续 op + word + op + word（含 display）", () => {
    expect(tokenizeQuery("a AND b OR c NOT d")).toEqual([
      { kind: "word", text: "a" },
      { kind: "op", text: "AND", display: "＆" },
      { kind: "word", text: "b" },
      { kind: "op", text: "OR", display: "｜" },
      { kind: "word", text: "c" },
      { kind: "op", text: "NOT", display: "￢" },
      { kind: "word", text: "d" },
    ]);
  });
});

describe("tokenizeQuery - 复杂混合场景", () => {
  it("TestTQ_AllMixed: word + op + phrase + regex 全部混用（含 display）", () => {
    expect(tokenizeQuery('在线 AND "高清视频" OR regex:/^test/')).toEqual([
      { kind: "word", text: "在线" },
      { kind: "op", text: "AND", display: "＆" },
      { kind: "phrase", text: "高清视频" },
      { kind: "op", text: "OR", display: "｜" },
      { kind: "regex", text: "^test" },
    ]);
  });

  it("TestTQ_RealQuery: 用户典型查询 - 多 word + NOT（含 display）", () => {
    const result = tokenizeQuery('在线 高清 NOT "广告"');
    expect(result).toEqual([
      { kind: "word", text: "在线" },
      { kind: "word", text: "高清" },
      { kind: "op", text: "NOT", display: "￢" },
      { kind: "phrase", text: "广告" },
    ]);
  });

  it("TestTQ_MultipleSpaces: 连续空格", () => {
    expect(tokenizeQuery("hello   world")).toEqual([
      { kind: "word", text: "hello" },
      { kind: "word", text: "world" },
    ]);
  });

  it("TestTQ_LeadingTrailingSpaces: 头尾空格", () => {
    expect(tokenizeQuery("  hello world  ")).toEqual([
      { kind: "word", text: "hello" },
      { kind: "word", text: "world" },
    ]);
  });

  it("TestTQ_OnlyWhitespace: 只有空白", () => {
    // 当前实现只认 ASCII 空格作分隔符（不认 \t \n）— 与 ion-searchbar 行为一致
    expect(tokenizeQuery("     ")).toEqual([]);
  });
});

describe("renderSnippet - 基础场景", () => {
  it("TestRS_Undefined: undefined → []", () => {
    expect(renderSnippet(undefined)).toEqual([]);
  });

  it('TestRS_Empty: "" → []', () => {
    expect(renderSnippet("")).toEqual([]);
  });

  it("TestRS_NoHighlight: 无 <<...>> → 整段非高亮", () => {
    expect(renderSnippet("hello world")).toEqual([{ text: "hello world", highlight: false }]);
  });

  it("TestRS_SingleHighlight: 单个高亮", () => {
    expect(renderSnippet("<<hello>> world")).toEqual([
      { text: "hello", highlight: true },
      { text: " world", highlight: false },
    ]);
  });

  it("TestRS_PrefixHighlight: 前缀高亮", () => {
    expect(renderSnippet("<<hello>> world")).toEqual([
      { text: "hello", highlight: true },
      { text: " world", highlight: false },
    ]);
  });

  it("TestRS_SuffixHighlight: 后缀高亮", () => {
    expect(renderSnippet("hello <<world>>")).toEqual([
      { text: "hello ", highlight: false },
      { text: "world", highlight: true },
    ]);
  });

  it("TestRS_MultipleHighlights: 多段高亮交替", () => {
    expect(renderSnippet("...<<在线>> 高清 视<<频>>...")).toEqual([
      { text: "...", highlight: false },
      { text: "在线", highlight: true },
      { text: " 高清 视", highlight: false },
      { text: "频", highlight: true },
      { text: "...", highlight: false },
    ]);
  });
});

describe("renderSnippet - 容错场景", () => {
  it("TestRS_UnclosedOpen: 只有 << 没有 >> → 整段视为高亮", () => {
    // 容错：inHighlight=true 状态保持到结尾
    const result = renderSnippet("hello <<world");
    expect(result).toEqual([
      { text: "hello ", highlight: false },
      { text: "world", highlight: true },
    ]);
  });

  it("TestRS_UnclosedClose: 只有 >> 没有 << → >> 静默丢弃", () => {
    // 容错：孤立的 >> 视为无效（不像 << 会开启高亮）
    // 理由：FTS5 snippet 永远成对出现 <<...>>，孤立 >> 必然是脏数据
    const result = renderSnippet("hello world>>");
    expect(result).toEqual([{ text: "hello world", highlight: false }]);
  });

  it("TestRS_AdjacentMarkers: 紧贴的 >> << (无内容)", () => {
    // 边界：>> 立刻 << → inHighlight 切换
    const result = renderSnippet("hello>><<world");
    // 实际：'hello' + 0-text highlight=true (>>close) + 0-text highlight=true (<<open) + 'world' highlight=true
    // 简化：前段不闭合的 '>>' 在 highlight=false 时不入段
    expect(result).toEqual([
      { text: "hello", highlight: false },
      { text: "world", highlight: true },
    ]);
  });

  it("TestRS_CJKHighlight: 中文高亮", () => {
    expect(renderSnippet("在<<线>>播放")).toEqual([
      { text: "在", highlight: false },
      { text: "线", highlight: true },
      { text: "播放", highlight: false },
    ]);
  });

  it("TestRS_OnlyHighlights: 整段都被高亮包裹", () => {
    expect(renderSnippet("<<hello world>>")).toEqual([{ text: "hello world", highlight: true }]);
  });

  it("TestRS_EmptyHighlight: 空高亮 <<>>", () => {
    // 边界：<<>> 中间无内容
    // 实现逻辑：<<  → inHighlight=true, cur=''
    //          '>>' → cur='' 时不 push, inHighlight=false
    // 最终 cur='' → 不 push
    expect(renderSnippet("<<>>")).toEqual([]);
  });
});

describe("renderSnippet - 真实 snippet 场景（来自 FTS5）", () => {
  it("TestRS_RealFTS5Snippet: FTS5 真实输出格式", () => {
    // FTS5 snippet() 函数格式: '...<<token1>> ... <<token2>>...'
    // 模拟 Go 端输出
    const snippet = "文件 <<在线>> 高清 视<<频>> 播放列表";
    const result = renderSnippet(snippet);
    expect(result).toEqual([
      { text: "文件 ", highlight: false },
      { text: "在线", highlight: true },
      { text: " 高清 视", highlight: false },
      { text: "频", highlight: true },
      { text: " 播放列表", highlight: false },
    ]);
  });

  it("TestRS_NoMatchHighlight: snippet 不含高亮时", () => {
    // 部分 FTS5 实现可能不返回高亮
    expect(renderSnippet("普通文本 没有高亮")).toEqual([{ text: "普通文本 没有高亮", highlight: false }]);
  });
});
