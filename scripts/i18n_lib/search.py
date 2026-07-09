"""搜索与近重复检测模块 - MinHash + LSH + n-gram 索引"""
from __future__ import annotations

from collections import defaultdict
from difflib import SequenceMatcher
from .tokenizer import tokenize

MINHASH_PERMUTATIONS = 64
LSH_BANDS = 8
LSH_ROWS = MINHASH_PERMUTATIONS // LSH_BANDS

_minhash_rand_a = [
    0x9e3779b1, 0x5b429ad3, 0x27d4eb2d, 0x654a8a9b, 0x8c832207, 0x47c63a29,
    0x165667b1, 0x9b11427f, 0x3c6ef35f, 0x750a1a0d, 0x5f4f7a39, 0x2c3a744d,
    0x6074a33f, 0x0e328f05, 0x4a852c07, 0x13198a2e, 0x03707344, 0xa4093822,
    0x299f31d0, 0x082efa98, 0xec4e6c89, 0x452821e6, 0x38d01377, 0xbe5466cf,
    0x34e90c6c, 0xc0ac29b7, 0xc97c50dd, 0x3f84d5b5, 0xb5470917, 0x9216d5d9,
    0x8979fb1b, 0xd1310ba6, 0x98dfb5ac, 0x2ffd72db, 0xd01adfb7, 0xb8e1afed,
    0x6a267e96, 0xba7c9045, 0xf12c7f99, 0x24a19947, 0xb3916cf7, 0x0801f2e2,
    0x858efc16, 0x636920d8, 0x71574e69, 0xa458fea3, 0xf4933d7e, 0x0d95748f,
    0x728eb658, 0x718bcd58, 0x82154aee, 0x7b54a41d, 0xc25a59b5, 0x9c30d539,
    0x2af26013, 0xc5d1b023, 0x286085f0, 0xca417918, 0xb8db38ef, 0x8e79dcb0,
]

_minhash_rand_b = [
    0x243f6a88, 0x85a308d3, 0x13198a2e, 0x03707344, 0xa4093822, 0x299f31d0,
    0x082efa98, 0xec4e6c89, 0x452821e6, 0x38d01377, 0xbe5466cf, 0x34e90c6c,
    0xc0ac29b7, 0xc97c50dd, 0x3f84d5b5, 0xb5470917, 0x9216d5d9, 0x8979fb1b,
    0xd1310ba6, 0x98dfb5ac, 0x2ffd72db, 0xd01adfb7, 0xb8e1afed, 0x6a267e96,
    0xba7c9045, 0xf12c7f99, 0x24a19947, 0xb3916cf7, 0x0801f2e2, 0x858efc16,
    0x636920d8, 0x71574e69, 0xa458fea3, 0xf4933d7e, 0x0d95748f, 0x728eb658,
    0x718bcd58, 0x82154aee, 0x7b54a41d, 0xc25a59b5, 0x9c30d539, 0x2af26013,
    0xc5d1b023, 0x286085f0, 0xca417918, 0xb8db38ef, 0x8e79dcb0, 0x6074a33f,
    0x0e328f05, 0x4a852c07, 0x9e3779b9, 0x5b429ad3, 0x27d4eb2d, 0x654a8a9b,
    0x8c832207, 0x47c63a29, 0x165667b1, 0x9b11427f, 0x3c6ef35f, 0x750a1a0d,
    0x5f4f7a39, 0x2c3a744d, 0x243f6a88, 0x85a308d3,
]

_MASK32 = 0xFFFFFFFF
_PRIME = 4294967291


def _fnv1a_32(text: str) -> int:
    h = 0x811c9dc5
    for ch in text:
        h ^= ord(ch)
        h = (h * 0x01000193) & _MASK32
    return h


def minhash_signature(tokens: list[str]) -> list[int]:
    if not tokens:
        return [0] * MINHASH_PERMUTATIONS

    hashes = [_fnv1a_32(t) for t in tokens]

    sig = []
    for i in range(MINHASH_PERMUTATIONS):
        a = _minhash_rand_a[i % len(_minhash_rand_a)]
        b = _minhash_rand_b[i % len(_minhash_rand_b)]
        min_h = _PRIME
        for h in hashes:
            hv = ((a * h + b) % _PRIME) & _MASK32
            if hv < min_h:
                min_h = hv
        sig.append(min_h)
    return sig


def lsh_buckets(sig: list[int]) -> list[int]:
    buckets = []
    for band in range(LSH_BANDS):
        start = band * LSH_ROWS
        end = start + LSH_ROWS
        h = 0x811c9dc5
        for v in sig[start:end]:
            h ^= v & 0xFF
            h = (h * 0x01000193) & _MASK32
            h ^= (v >> 8) & 0xFF
            h = (h * 0x01000193) & _MASK32
            h ^= (v >> 16) & 0xFF
            h = (h * 0x01000193) & _MASK32
            h ^= (v >> 24) & 0xFF
            h = (h * 0x01000193) & _MASK32
        buckets.append((band << 24) | (h & 0xFFFFFF))
    return buckets


def jaccard_similarity(set1: set[str], set2: set[str]) -> float:
    if not set1 or not set2:
        return 0.0
    inter = len(set1 & set2)
    union = len(set1 | set2)
    if union == 0:
        return 0.0
    return inter / union


def find_near_duplicates_lsh(
    items: list[tuple[str, str]],
    locale: str,
    threshold: float = 0.85,
) -> list[tuple[float, str, str, str, str]]:
    n = len(items)
    if n < 2:
        return []

    token_sets: list[set[str]] = []
    sigs: list[list[int]] = []

    for _, value in items:
        tokens = tokenize(value, locale)
        token_sets.append(set(tokens))
        sigs.append(minhash_signature(tokens))

    bucket_map: dict[int, list[int]] = defaultdict(list)
    for idx in range(n):
        for bucket in lsh_buckets(sigs[idx]):
            bucket_map[bucket].append(idx)

    candidate_pairs: set[tuple[int, int]] = set()
    for bucket_idxs in bucket_map.values():
        if len(bucket_idxs) < 2:
            continue
        bl = len(bucket_idxs)
        for i in range(bl):
            for j in range(i + 1, bl):
                a, b = bucket_idxs[i], bucket_idxs[j]
                if items[a][0].split(".")[0] == items[b][0].split(".")[0]:
                    continue
                if len(items[a][1]) < 10 or len(items[b][1]) < 10:
                    continue
                candidate_pairs.add((min(a, b), max(a, b)))

    results = []
    jaccard_threshold = threshold * 0.7
    for a, b in candidate_pairs:
        j = jaccard_similarity(token_sets[a], token_sets[b])
        if j < jaccard_threshold:
            continue
        k1, v1 = items[a]
        k2, v2 = items[b]
        ratio = SequenceMatcher(None, v1, v2).ratio()
        if ratio >= threshold:
            results.append((ratio, k1, v1, k2, v2))

    results.sort(key=lambda x: -x[0])
    return results


def build_ngram_index(texts: list[str], locale: str, n: int = 2) -> dict[str, list[int]]:
    index: dict[str, list[int]] = defaultdict(list)
    for idx, text in enumerate(texts):
        tokens = tokenize(text, locale)
        seen: set[str] = set()
        for i in range(len(tokens) - n + 1):
            gram = " ".join(tokens[i:i + n])
            if gram not in seen:
                seen.add(gram)
                index[gram].append(idx)
    return index


def search_ngram(
    query: str,
    texts: list[str],
    index: dict[str, list[int]],
    locale: str,
    top_k: int = 20,
) -> list[tuple[int, float]]:
    query_tokens = tokenize(query, locale)
    if not query_tokens:
        return []

    scores: dict[int, float] = defaultdict(float)
    query_ngrams: set[str] = set()

    for i in range(len(query_tokens) - 1):
        gram = " ".join(query_tokens[i:i + 2])
        query_ngrams.add(gram)
    for t in query_tokens:
        query_ngrams.add(t)

    for gram in query_ngrams:
        if gram in index:
            for idx in index[gram]:
                scores[idx] += 1.0

    if not scores:
        return []

    scored = sorted(scores.items(), key=lambda x: -x[1])
    return [(idx, score) for idx, score in scored[:top_k]]
