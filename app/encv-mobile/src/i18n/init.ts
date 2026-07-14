import { initSharedI18n } from "@encv/shared-components/i18n";
import { registerFieldKeyMap, registerI18nModules, registerSectionTitleMap } from "@encv/shared-components/composables/useI18n";
import agent from "@/i18n/agent";
import extensions from "@/i18n/extensions";
import files from "@/i18n/files";
import modals from "@/i18n/modals";
import player from "@/i18n/player";
import simverse from "@/i18n/simverse";
import tasks from "@/i18n/tasks";

export function initEncvI18n() {
  initSharedI18n();
  registerI18nModules([tasks, files, player, extensions, modals, agent, simverse]);

  registerFieldKeyMap({
    password: "settings.password",
    recover: "settings.recover",
    output_path: "settings.outputPath",
    plugin_settings: "settings.pluginSettings",
    server: "settings.httpServerSettings",
    admin: "settings.adminServerSettings",
    webdav: "settings.webdavServerSettings",
    proxy: "settings.proxyServerSettings",
    log: "settings.logSettings",
    port: "settings.port",
    dir: "settings.dir",
    username: "settings.username",
    root: "settings.root",
    level: "settings.level",
    file: "settings.file",
    host: "settings.host",
    description: "settings.description",
    sites: "settings.sites",
    disable_signature_verification: "settings.disableSignatureVerification",
    ext: "settings.ext",
    chunk_size_mb: "settings.chunkSizeMb",
    light_main_chunk_enabled: "settings.lightMainChunkEnabled",
    track_extensions: "settings.trackExtensions",
    keep_mkv_for_mkv_source: "settings.keepMkvForMkvSource",
    verify_after_pack: "settings.verifyAfterPack",
    plugin_cache_dir: "settings.pluginCacheDir",
    skip_merge_for_split_mkv: "settings.skipMergeForSplitMkv",
    video: "settings.video",
    audio: "settings.audio",
    image: "settings.image",
    wps: "settings.wps",
    pdf: "settings.pdf",
    text: "settings.text",
    custom_text_extensions: "settings.customTextExts",
    allow_no_reencode: "settings.allowNoReencode",
    default_stream_preset: "settings.defaultStreamPreset",
    suffix: "settings.alistEncryptSuffix",
    default_password: "settings.alistEncryptDefaultPassword",
    algorithm: "settings.alistEncryptAlgorithm",
    enabled: "settings.alistEncryptEnable",
    agent_settings: "settings.agentSettings",
    openai_api_key: "settings.openaiApiKey",
    openai_base_url: "settings.openaiBaseUrl",
    openai_model: "settings.openaiModel",
    openlist_base_url: "settings.openlistBaseUrl",
    openlist_token: "settings.openlistToken",
    default_container_version: "settings.defaultContainerVersion",
    enabled_tools: "settings.enabledTools",
    system_prompt: "settings.systemPrompt",
    max_tool_calls_per_turn: "settings.maxToolCallsPerTurn",
  });

  registerSectionTitleMap({
    全局设置: "settings.globalSettings",
    "加密/解密设置": "settings.encryptDecryptSettings",
    "内置HTTP服务器 设置": "settings.httpServerSettings",
    "管理后台服务器 设置": "settings.adminServerSettings",
    "WebDAV 服务器设置": "settings.webdavServerSettings",
    "Openlist 代理服务器设置": "settings.proxyServerSettings",
    日志设置: "settings.logSettings",
    "AI 助手设置": "settings.agentSettings",
  });
}
