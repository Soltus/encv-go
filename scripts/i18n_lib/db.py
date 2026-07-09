"""数据库模块 - SQLite 存储与索引"""
from __future__ import annotations

import sqlite3
from collections import defaultdict

from .config import DB_PATH, get_app_config
from .loader import load_all_dicts
from .scanner import extract_used_keys
from .perf import perf_tracker


def get_db_conn() -> sqlite3.Connection:
    conn = sqlite3.connect(str(DB_PATH))
    conn.execute("PRAGMA journal_mode=WAL")
    conn.execute("PRAGMA synchronous=NORMAL")
    conn.execute("PRAGMA cache_size=10000")
    return conn


def init_db(app_name: str | None = None) -> sqlite3.Connection:
    perf_tracker.start("数据库初始化")
    conn = get_db_conn()

    conn.executescript("""
        CREATE TABLE IF NOT EXISTS translations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            key TEXT NOT NULL,
            locale TEXT NOT NULL,
            value TEXT NOT NULL,
            source_file TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(key, locale)
        );

        CREATE INDEX IF NOT EXISTS idx_translations_key ON translations(key);
        CREATE INDEX IF NOT EXISTS idx_translations_locale ON translations(locale);

        CREATE TABLE IF NOT EXISTS key_usage (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            key TEXT NOT NULL,
            file_path TEXT NOT NULL,
            line_number INTEGER,
            app_name TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        );

        CREATE INDEX IF NOT EXISTS idx_usage_key ON key_usage(key);
        CREATE INDEX IF NOT EXISTS idx_usage_file ON key_usage(file_path);

        CREATE VIRTUAL TABLE IF NOT EXISTS translation_fts USING fts5(
            key, value, locale,
            tokenize='unicode61'
        );
    """)

    dicts = load_all_dicts(app_name)
    file_source = dicts.get("_file_source", {})
    translations = []

    for locale in ["zh-CN", "en"]:
        locale_dict = dicts.get(locale, {})
        for key, value in locale_dict.items():
            translations.append((
                key, locale, value,
                file_source.get(key, ""),
            ))

    conn.executemany(
        "INSERT OR REPLACE INTO translations (key, locale, value, source_file) VALUES (?, ?, ?, ?)",
        translations,
    )

    used_keys = extract_used_keys(app_name)
    usage_records = []
    for key, files in used_keys.items():
        for f in files:
            parts = f.rsplit(":", 1)
            file_path = parts[0]
            line_num = int(parts[1]) if len(parts) > 1 and parts[1].isdigit() else 0
            usage_records.append((key, file_path, line_num, app_name or ""))

    conn.execute("DELETE FROM key_usage WHERE app_name = ?", (app_name or "",))
    conn.executemany(
        "INSERT INTO key_usage (key, file_path, line_number, app_name) VALUES (?, ?, ?, ?)",
        usage_records,
    )

    conn.execute("DELETE FROM translation_fts")
    conn.execute(
        "INSERT INTO translation_fts (key, value, locale) SELECT key, value, locale FROM translations",
    )

    conn.commit()

    total_trans = conn.execute("SELECT COUNT(*) FROM translations").fetchone()[0]
    total_keys = conn.execute("SELECT COUNT(DISTINCT key) FROM translations").fetchone()[0]
    total_usage = conn.execute("SELECT COUNT(*) FROM key_usage").fetchone()[0]
    total_fts = conn.execute("SELECT COUNT(*) FROM translation_fts").fetchone()[0]

    print(f"✅ 数据库初始化完成: {DB_PATH}")
    print(f"   翻译条目: {total_trans}")
    print(f"   唯一 key: {total_keys}")
    print(f"   使用记录: {total_usage}")
    print(f"   搜索索引: {total_fts} 条")

    perf_tracker.end("数据库初始化", total_trans)
    return conn


def db_query(sql: str, params: tuple = ()) -> list[dict]:
    conn = get_db_conn()
    conn.row_factory = sqlite3.Row
    cursor = conn.execute(sql, params)
    rows = [dict(row) for row in cursor.fetchall()]
    conn.close()
    return rows


def search_db(query: str, locale: str | None = None, limit: int = 50) -> list[dict]:
    conn = get_db_conn()
    conn.row_factory = sqlite3.Row

    if locale:
        rows = conn.execute(
            """SELECT t.* FROM translation_fts f
               JOIN translations t ON f.key = t.key AND f.locale = t.locale
               WHERE translation_fts MATCH ? AND t.locale = ?
               ORDER BY rank LIMIT ?""",
            (query, locale, limit),
        ).fetchall()
    else:
        rows = conn.execute(
            """SELECT t.* FROM translation_fts f
               JOIN translations t ON f.key = t.key AND f.locale = t.locale
               WHERE translation_fts MATCH ?
               ORDER BY rank LIMIT ?""",
            (query, limit),
        ).fetchall()

    conn.close()
    return [dict(row) for row in rows]
