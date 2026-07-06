"""分词与文本处理模块 - 针对中英文特性深度优化"""
from __future__ import annotations

import re
from .config import EN_STOPWORDS


def tokenize_zh(text: str) -> list[str]:
    text = re.sub(r"[^\u4e00-\u9fa5a-zA-Z0-9]", " ", text)
    tokens: list[str] = []
    i = 0
    chars = list(text)
    n = len(chars)
    while i < n:
        ch = chars[i]
        if "\u4e00" <= ch <= "\u9fff":
            if i + 1 < n and "\u4e00" <= chars[i + 1] <= "\u9fff":
                tokens.append(ch + chars[i + 1])
            tokens.append(ch)
            i += 1
        elif ch.isalnum():
            j = i
            while j < n and chars[j].isalnum():
                j += 1
            word = "".join(chars[i:j]).lower()
            if len(word) > 1:
                tokens.append(word)
            i = j
        else:
            i += 1
    return tokens


def tokenize_en(text: str) -> list[str]:
    text = text.lower()
    words = re.findall(r"[a-z]+", text)
    tokens = []
    for w in words:
        if len(w) < 2:
            continue
        if w in EN_STOPWORDS:
            continue
        if w.endswith("ing") and len(w) > 5:
            tokens.append(w[:-3])
        elif w.endswith("ed") and len(w) > 4:
            tokens.append(w[:-2])
        elif w.endswith("ly") and len(w) > 4:
            tokens.append(w[:-2])
        elif w.endswith("tion") and len(w) > 5:
            tokens.append(w[:-4])
        elif w.endswith("s") and len(w) > 3 and not w.endswith("ss"):
            tokens.append(w[:-1])
        tokens.append(w)
    return tokens


def tokenize(text: str, locale: str = "zh-CN") -> list[str]:
    if not text:
        return []
    if locale.startswith("zh"):
        return tokenize_zh(text)
    else:
        return tokenize_en(text)


def file_hash(path: str) -> str:
    import hashlib
    try:
        with open(path, "rb") as f:
            return hashlib.md5(f.read()).hexdigest()
    except Exception:
        return ""
