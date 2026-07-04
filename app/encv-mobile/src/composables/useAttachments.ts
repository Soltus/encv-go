/**
 * useAttachments - Composer 附件管理复合式
 *
 * Task 12：Composer 底栏 `+` 按钮触发文件选择器，区分 image（缩略图行）
 * 和 file（卡片行），均显示在 textarea 上方。发送时 attachments 编入
 * message content（OpenAI multimodal 数组格式）。
 *
 * 设计约束：
 * - attachment 仅前端处理（不传后端）—— 用 base64 data URL inline 到
 *   message content。
 * - image ≤ 5MB，file ≤ 20MB（超限抛错，调用方弹 toast）。
 * - 移动端需要支持多选（调用方传 FileList）。
 *
 * OpenAI multimodal content 格式参考：
 *   [
 *     { type: 'text', text: '...' },
 *     { type: 'image_url', image_url: { url: 'data:image/png;base64,...' } },
 *     { type: 'file', file: { filename, file_data: 'data:...;base64,...' } }
 *   ]
 *
 * 注：当前 encv-go 后端只读取 message.content 字符串（见 useAgent.ts
 * 构造的 apiMessages），不会解析 multimodal 数组。本任务只是把附件数据
 * inline 到本地 message + 通过 serialize() 提供给调用方；后端是否会
 * 解析留作后续 story。本任务严格遵循「严禁修改后端 API」约束。
 */
import { type Ref, ref } from "vue";

// =============================================================================
// 类型定义
// =============================================================================

export interface Attachment {
  id: string;
  name: string;
  mimeType: string;
  sizeBytes: number;
  /** base64 data URL（含前缀 data:<mime>;base64,...） */
  dataUrl: string;
  kind: "image" | "file";
}

/**
 * OpenAI multimodal content 元素的简化版。Task 12 把 Attachment
 * 编码成这个数组塞进 Message.content 字段。
 *
 * - text 元素保留用户输入文本（与纯文本消息兼容）
 * - image_url 用于图片预览（OpenAI 标准字段）
 * - file 元素带 file_data，承载非图片附件（OpenAI 新版字段）
 */
export type MessageContentPart =
  | { type: "text"; text: string }
  | { type: "image_url"; image_url: { url: string } }
  | { type: "file"; file: { filename: string; file_data: string } };

export interface AddFilesResult {
  added: Attachment[];
  rejected: { name: string; reason: string }[];
}

// =============================================================================
// 常量
// =============================================================================

/** 单张图片上限 5MB */
export const MAX_IMAGE_SIZE = 5 * 1024 * 1024;
/** 单个文件上限 20MB */
export const MAX_FILE_SIZE = 20 * 1024 * 1024;
/** 单次选择最多处理的文件数（防止一次性选择 1000+ 文件卡死） */
export const MAX_FILES_PER_BATCH = 50;

// =============================================================================
// 工具函数
// =============================================================================

/**
 * 根据 MIME 判断是否图片。优先用 file.type，缺失时看扩展名兜底。
 */
function isImageMime(file: File): boolean {
  if (file.type && file.type.startsWith("image/")) return true;
  // 某些文件选择器在 Android/iOS 上 type 可能为空，用扩展名兜底
  const name = (file.name || "").toLowerCase();
  return /\.(png|jpe?g|gif|webp|bmp|svg|heic|heif|avif)$/.test(name);
}

function generateAttachmentId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return `att-${crypto.randomUUID()}`;
  }
  return `att-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

/**
 * 把 File 读成 base64 data URL。jsdom 与浏览器都支持 FileReader。
 */
function readFileAsDataUrl(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const result = reader.result;
      if (typeof result === "string") {
        resolve(result);
      } else {
        reject(new Error("FileReader returned non-string result"));
      }
    };
    reader.onerror = () => {
      reject(reader.error || new Error("Failed to read file"));
    };
    reader.readAsDataURL(file);
  });
}

// =============================================================================
// 复合式主体
// =============================================================================

export interface UseAttachmentsOptions {
  /** 自定义上限；不传则用模块常量 */
  maxImageSize?: number;
  maxFileSize?: number;
  /** 单批最多处理文件数；超限部分静默丢弃（防卡死） */
  maxFilesPerBatch?: number;
  /** 错误回调（如超限）—— 默认 console.warn，调用方可注入弹 toast */
  onError?: (message: string) => void;
}

export function useAttachments(options: UseAttachmentsOptions = {}) {
  const maxImageSize = options.maxImageSize ?? MAX_IMAGE_SIZE;
  const maxFileSize = options.maxFileSize ?? MAX_FILE_SIZE;
  const maxFilesPerBatch = options.maxFilesPerBatch ?? MAX_FILES_PER_BATCH;
  const onError = options.onError ?? (msg => console.warn("[useAttachments]", msg));

  const attachments: Ref<Attachment[]> = ref([]);

  /**
   * 把 FileList / File[] 加进 attachments。超限的文件不会中断整体流程，
   * 而是被收集到 rejected 列表里由调用方决定如何提示。
   */
  async function addFiles(files: FileList | File[] | null | undefined): Promise<AddFilesResult> {
    if (!files) return { added: [], rejected: [] };
    const list = Array.from(files);
    const accepted: Attachment[] = [];
    const rejected: { name: string; reason: string }[] = [];

    // 截断到 maxFilesPerBatch
    const truncated = list.length > maxFilesPerBatch;
    if (truncated) {
      onError(`一次最多选择 ${maxFilesPerBatch} 个文件，多余部分已忽略`);
    }
    const processList = list.slice(0, maxFilesPerBatch);

    for (const file of processList) {
      const kind: "image" | "file" = isImageMime(file) ? "image" : "file";
      const limit = kind === "image" ? maxImageSize : maxFileSize;
      if (file.size > limit) {
        const limitMb = (limit / (1024 * 1024)).toFixed(0);
        rejected.push({
          name: file.name || "(未命名)",
          reason: `超过 ${limitMb}MB 限制`,
        });
        continue;
      }
      try {
        const dataUrl = await readFileAsDataUrl(file);
        accepted.push({
          id: generateAttachmentId(),
          name: file.name || "(未命名)",
          mimeType: file.type || (kind === "image" ? "image/*" : "application/octet-stream"),
          sizeBytes: file.size,
          dataUrl,
          kind,
        });
      } catch (e: any) {
        rejected.push({
          name: file.name || "(未命名)",
          reason: e?.message || "读取失败",
        });
      }
    }

    if (accepted.length > 0) {
      attachments.value.push(...accepted);
    }
    return { added: accepted, rejected };
  }

  function removeAttachment(id: string): void {
    const idx = attachments.value.findIndex(a => a.id === id);
    if (idx >= 0) {
      attachments.value.splice(idx, 1);
    }
  }

  function clearAttachments(): void {
    attachments.value.splice(0, attachments.value.length);
  }

  /**
   * 把当前 attachments 编码为 OpenAI multimodal content 数组。
   * 调用方把整个数组塞进 message.content 字段（替代原先的纯字符串）。
   *
   * - text 始终放在最前，便于无图模型 fallback
   * - image → image_url 元素
   * - file → file 元素（filename + file_data）
   */
  function serialize(text: string): MessageContentPart[] {
    return serializeAttachments(text, attachments.value);
  }

  return {
    attachments,
    addFiles,
    removeAttachment,
    clearAttachments,
    serialize,
  };
}

/**
 * 纯函数版 serialize：不依赖任何 reactive state，方便 useAgent
 * 等非组件层代码直接调用。
 *
 * 规则：
 *  - text 非空时，text 元素放在最前（OpenAI multimodal 规范）
 *  - image → { type: 'image_url', image_url: { url: dataUrl } }
 *  - file  → { type: 'file', file: { filename, file_data: dataUrl } }
 */
export function serializeAttachments(text: string, attachments: Attachment[]): MessageContentPart[] {
  const parts: MessageContentPart[] = [];
  if (text) {
    parts.push({ type: "text", text });
  }
  for (const att of attachments) {
    if (att.kind === "image") {
      parts.push({
        type: "image_url",
        image_url: { url: att.dataUrl },
      });
    } else {
      parts.push({
        type: "file",
        file: {
          filename: att.name,
          file_data: att.dataUrl,
        },
      });
    }
  }
  return parts;
}
