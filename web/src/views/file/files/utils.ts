import { formatBytes } from "@/utils/format";
import type { Component } from "vue";
import { getSignedUrl, type FileItem, type FileKind } from "@/api/file";
import FileImage from "~icons/ri/file-image-line";
import FilePdf from "~icons/ri/file-pdf-2-line";
import FileWord from "~icons/ri/file-word-2-line";
import FileExcel from "~icons/ri/file-excel-2-line";
import FilePpt from "~icons/ri/file-ppt-2-line";
import FileZip from "~icons/ri/file-zip-line";
import FileText from "~icons/ri/file-text-line";
import FileMusic from "~icons/ri/file-music-line";
import FileVideo from "~icons/ri/file-video-line";
import FileMarkdown from "~icons/ri/markdown-line";
import FileGeneric from "~icons/ri/file-3-line";

export const KIND_OPTIONS: ReadonlyArray<{ label: string; value: FileKind }> = [
  { label: "图片", value: "image" },
  { label: "文档", value: "document" },
  { label: "音视频", value: "media" },
  { label: "压缩包", value: "archive" },
  { label: "其他", value: "other" }
];

export const KIND_LABELS: Record<FileKind, string> = Object.fromEntries(
  KIND_OPTIONS.map(o => [o.value, o.label])
) as Record<FileKind, string>;

/** 类别配色：图表与标签共用，保证同一类别在各处颜色一致 */
export const KIND_COLORS: Record<FileKind, string> = {
  image: "#3b82f6",
  document: "#10b981",
  media: "#f59e0b",
  archive: "#8b5cf6",
  other: "#94a3b8"
};

export const KIND_TAG_TYPES: Record<
  FileKind,
  "primary" | "success" | "warning" | "danger" | "info"
> = {
  image: "primary",
  document: "success",
  media: "warning",
  archive: "danger",
  other: "info"
};

type ExtVisual = { icon: Component; color: string };

const EXT_VISUALS: Record<string, ExtVisual> = {
  pdf: { icon: FilePdf, color: "#ef4444" },
  doc: { icon: FileWord, color: "#2563eb" },
  docx: { icon: FileWord, color: "#2563eb" },
  odt: { icon: FileWord, color: "#2563eb" },
  xls: { icon: FileExcel, color: "#16a34a" },
  xlsx: { icon: FileExcel, color: "#16a34a" },
  ods: { icon: FileExcel, color: "#16a34a" },
  csv: { icon: FileExcel, color: "#16a34a" },
  ppt: { icon: FilePpt, color: "#ea580c" },
  pptx: { icon: FilePpt, color: "#ea580c" },
  odp: { icon: FilePpt, color: "#ea580c" },
  zip: { icon: FileZip, color: "#8b5cf6" },
  gz: { icon: FileZip, color: "#8b5cf6" },
  rar: { icon: FileZip, color: "#8b5cf6" },
  txt: { icon: FileText, color: "#64748b" },
  log: { icon: FileText, color: "#64748b" },
  json: { icon: FileText, color: "#64748b" },
  md: { icon: FileMarkdown, color: "#334155" },
  mp3: { icon: FileMusic, color: "#f59e0b" },
  wav: { icon: FileMusic, color: "#f59e0b" },
  mp4: { icon: FileVideo, color: "#db2777" },
  webm: { icon: FileVideo, color: "#db2777" }
};

/** 按扩展名选择文件图标与颜色 */
export function extVisual(ext: string, kind?: FileKind): ExtVisual {
  const known = EXT_VISUALS[ext.toLowerCase()];
  if (known) return known;
  if (kind === "image") return { icon: FileImage, color: KIND_COLORS.image };
  return { icon: FileGeneric, color: KIND_COLORS.other };
}

/** 字节数转可读大小，如 1.5 MB */
export const formatSize = formatBytes;

/** 文件名拆分为主名与扩展名（扩展名不含点） */
export function splitName(name: string): { base: string; ext: string } {
  const i = name.lastIndexOf(".");
  if (i <= 0) return { base: name, ext: "" };
  return { base: name.slice(0, i), ext: name.slice(i + 1) };
}

export function isPreviewable(file: Pick<FileItem, "mimeType">): boolean {
  return (
    file.mimeType.startsWith("image/") || file.mimeType === "application/pdf"
  );
}

/** 签名链接提前这么久视为过期，避免拿到即将失效的链接 */
const SIGNED_URL_MARGIN_MS = 30_000;

type CachedUrl = { url: string; expiresAt: number };

const signedCache = new Map<string, CachedUrl>();
const inflight = new Map<string, Promise<CachedUrl>>();

/**
 * 获取文件的签名链接（站内相对地址），在有效期内复用并合并并发请求。
 * 缩略图、预览、复制链接、下载共用同一份缓存。
 */
export async function signedUrlFor(id: string): Promise<CachedUrl> {
  const cached = signedCache.get(id);
  if (cached && cached.expiresAt - SIGNED_URL_MARGIN_MS > Date.now()) {
    return cached;
  }
  const pending = inflight.get(id);
  if (pending) return pending;
  const request = getSignedUrl(id)
    .then(res => {
      const entry = { url: res.url, expiresAt: Date.parse(res.expiresAt) };
      signedCache.set(id, entry);
      return entry;
    })
    .finally(() => inflight.delete(id));
  inflight.set(id, request);
  return request;
}

/** 文件删除后清除缓存的链接 */
export function forgetSignedUrl(id: string): void {
  signedCache.delete(id);
}

export function absoluteUrl(url: string): string {
  return new URL(url, window.location.origin).href;
}

/** 通过签名链接触发浏览器原生下载（大文件不经过内存） */
export async function downloadFile(file: FileItem): Promise<void> {
  const { url } = await signedUrlFor(file.id);
  const link = document.createElement("a");
  link.href = `${url}?download=1`;
  link.download = file.originalName;
  link.rel = "noopener";
  link.style.display = "none";
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
}

/** 复制文本到剪贴板，非安全上下文时回退到 execCommand */
export async function copyText(text: string): Promise<void> {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.opacity = "0";
  document.body.appendChild(area);
  area.select();
  const ok = document.execCommand("copy");
  document.body.removeChild(area);
  if (!ok) throw new Error("copy failed");
}
