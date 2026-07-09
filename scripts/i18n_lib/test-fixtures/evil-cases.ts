/**
 * i18n 恶意/悲观测试字典 - 构造各种可能搞崩解析器的场景
 *
 * 警告：这些是故意构造的"坏"数据，用来测试解析器的鲁棒性。
 * 真实项目中不会出现这种情况，但解析器必须能正确处理。
 */
export default {
  "zh-CN": {
    "evil.balancedBracesInString": "{{{{{{{{{{}}}}}}}}}}",
    "evil.manyQuotes": `"'""''"""''"'""''"""`,
    "evil.backslashStorm": String.raw`\\\\\\\\\\\\\\`,
    "evil.unicodeBom": "\uFEFFprefixed",
    "evil.nullBytes": "hello\x00world",
    "evil.veryLongKey": `${"x".repeat(1000)}`,
    "evil.zeroWidth": "he\u200bllo\u200cworld",
    "evil.allBraceTypes": "({[<>]})",
    "evil.colonsEverywhere": "a:b:c:d:e:f:g",
    "evil.commasInValue": "a, b, c, d, e, f",
    "evil.onlyBraceOpen": "{",
    "evil.onlyBraceClose": "}",
    "evil.emptyObject": "{}",
    "evil.justBackslash": "\\",
    "evil.manyNewlines": "\n\n\n\n\n\n\n\n\n\n",
    "evil.tabsAndSpaces": "\t\t   \t  \t\t   ",
    "evil.htmlTags": "<div class=\"test\">hello</div>",
    "evil.jsonInValue": '{"nested": "object", "arr": [1,2,3]}',
    "evil.regexInValue": "/^[a-z]+(?:\\d+)?$/i",
    "evil.templateLiteralSyntax": "${expression} ${another}",
    "evil.weirdWhitespace": `line1
    line2
      line3
        line4`,
  },
  en: {
    "evil.balancedBracesInString": "{{{{{{{{{{}}}}}}}}}}",
    "evil.manyQuotes": `"'""''"""''"'""''"""`,
    "evil.backslashStorm": String.raw`\\\\\\\\\\\\\\`,
    "evil.unicodeBom": "\uFEFFprefixed",
    "evil.nullBytes": "hello\x00world",
    "evil.veryLongKey": `${"x".repeat(1000)}`,
    "evil.zeroWidth": "he\u200bllo\u200cworld",
    "evil.allBraceTypes": "({[<>]})",
    "evil.colonsEverywhere": "a:b:c:d:e:f:g",
    "evil.commasInValue": "a, b, c, d, e, f",
    "evil.onlyBraceOpen": "{",
    "evil.onlyBraceClose": "}",
    "evil.emptyObject": "{}",
    "evil.justBackslash": "\\",
    "evil.manyNewlines": "\n\n\n\n\n\n\n\n\n\n",
    "evil.tabsAndSpaces": "\t\t   \t  \t\t   ",
    "evil.htmlTags": "<div class=\"test\">hello</div>",
    "evil.jsonInValue": '{"nested": "object", "arr": [1,2,3]}',
    "evil.regexInValue": "/^[a-z]+(?:\\d+)?$/i",
    "evil.templateLiteralSyntax": "${expression} ${another}",
    "evil.weirdWhitespace": `line1
    line2
      line3
        line4`,
  },
}
