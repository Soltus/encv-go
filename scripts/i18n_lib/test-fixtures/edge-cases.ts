/**
 * i18n 边界测试字典 - 覆盖各种边界和恶意场景
 *
 * 测试场景：
 * 1. 空字符串 value
 * 2. 超长 value（10KB+）
 * 3. 特殊字符（转义引号、换行、反斜杠）
 * 4. 单引号、双引号、模板字符串混用
 * 5. key 含特殊字符（点、连字符、数字开头）
 * 6. 深层嵌套（虽然是扁平字典，但测试解析器鲁棒性）
 * 7. 注释干扰
 * 8. 大量相似 key（近重复检测压力测试）
 * 9. Unicode / emoji
 * 10. 模板字符串变量
 */
export default {
  "zh-CN": {
    "edge.emptyString": "",
    "edge.singleChar": "a",
    "edge.onlySpaces": "   ",
    "edge.withTab": "hello\tworld",
    "edge.withNewline": "line1\nline2",
    "edge.withCarriageReturn": "line1\rline2",

    "quote.singleQuoteValue": 'It\'s a test',
    "quote.doubleQuoteValue": "He said \"hello\" to me",
    "quote.mixedQuotes": 'He said "it\'s mine"',
    "quote.backtickValue": `template string`,
    "quote.escapedBackslash": "path\\to\\file",
    "quote.escapedEverything": "a\"b\\c'd",

    "unicode.chinese": "你好世界",
    "unicode.emoji": "🎉 欢迎 🚀",
    "unicode.mixed": "Hello 你好 🌟 World 世界",
    "unicode.rtl": "مرحبا بالعالم",

    "key.with-dashes": "dashed-key",
    "key.with_underscores": "underscore_key",
    "key.with.dots.multiple": "multiple.dots.in.key",
    "key.startsWithNumber": "123start",
    "key.specialChars@#": "special chars in key",

    "long.superLongValue": `${"这是一个超长的value。".repeat(500)}`,

    "comment.notAComment": "// this is not a comment",
    "comment.hasHash": "# not a comment",
    "comment.inValue": "key: value // still value",

    "var.simple": "Hello {name}",
    "var.multiple": "{count} items found in {folder}",
    "var.withFormatter": "Date: {date:YYYY-MM-DD}",
    "var.nestedBraces": "a {{b}} c",
    "var.escapedBrace": "use \\{key\\} for literal",

    "whitespace.leading": "   indented",
    "whitespace.trailing": "trailing   ",
    "whitespace.both": "   both sides   ",
  },
  en: {
    "edge.emptyString": "",
    "edge.singleChar": "a",
    "edge.onlySpaces": "   ",
    "edge.withTab": "hello\tworld",
    "edge.withNewline": "line1\nline2",
    "edge.withCarriageReturn": "line1\rline2",

    "quote.singleQuoteValue": "It's a test",
    "quote.doubleQuoteValue": 'He said "hello" to me',
    "quote.mixedQuotes": 'He said "it\'s mine"',
    "quote.backtickValue": `template string`,
    "quote.escapedBackslash": "path\\to\\file",
    "quote.escapedEverything": "a\"b\\c'd",

    "unicode.chinese": "Hello World",
    "unicode.emoji": "🎉 Welcome 🚀",
    "unicode.mixed": "Hello 你好 🌟 World 世界",
    "unicode.rtl": "Hello World",

    "key.with-dashes": "dashed-key",
    "key.with_underscores": "underscore_key",
    "key.with.dots.multiple": "multiple.dots.in.key",
    "key.startsWithNumber": "123start",
    "key.specialChars@#": "special chars in key",

    "long.superLongValue": `${"This is a super long value. ".repeat(500)}`,

    "comment.notAComment": "// this is not a comment",
    "comment.hasHash": "# not a comment",
    "comment.inValue": "key: value // still value",

    "var.simple": "Hello {name}",
    "var.multiple": "{count} items found in {folder}",
    "var.withFormatter": "Date: {date:YYYY-MM-DD}",
    "var.nestedBraces": "a {{b}} c",
    "var.escapedBrace": "use \\{key\\} for literal",

    "whitespace.leading": "   indented",
    "whitespace.trailing": "trailing   ",
    "whitespace.both": "   both sides   ",
  },
}
