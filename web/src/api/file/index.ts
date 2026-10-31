import type { AxiosProgressEvent } from "axios";
import { http, type PureHttpRequestConfig } from "@/utils/http";
import { compactParams, type Page, type PageQuery } from "@/api/types";

/** 文件类别：与后端 sys_files.kind 一致 */
export type FileKind = "image" | "document" | "media" | "archive" | "other";

export type FileItem = {
  id: string;
  originalName: string;
  ext: string;
  kind: FileKind;
  mimeType: string;
  size: number;
  sha256: string;
  storage: string;
  categoryId: string | null;
  categoryName: string;
  uploadedBy: string | null;
  uploaderName: string;
  version: number;
  createdAt: string;
  updatedAt: string;
};

/** categoryId：分类 id，"none" 表示未分类，留空表示全部 */
export type FileQuery = PageQuery & {
  keyword?: string;
  categoryId?: string;
  kind?: FileKind | "";
};

export type FileUpdatePayload = {
  originalName: string;
  categoryId: string | null;
  version: number;
};

export type FileCategory = {
  id: string;
  name: string;
  sort: number;
  fileCount: number;
  version: number;
  createdAt: string;
};

export type CategoryList = {
  list: FileCategory[];
  total: number;
  uncategorized: number;
};

export type CategoryPayload = {
  name: string;
  sort: number;
  version?: number;
};

export type SignedUrl = {
  /** 站内相对地址，如 /api/v1/files/signed/<token> */
  url: string;
  expiresAt: string;
};

export type UploadPolicy = {
  extensions: string[];
  maxBytes: number;
};

export type UsageStat = { key: string; count: number; size: number };

export type StorageStats = {
  driver: "local" | "s3" | string;
  location: {
    dir?: string;
    endpoint?: string;
    bucket?: string;
    region?: string;
    useSSL: boolean;
    usePathStyle: boolean;
  };
  maxUploadBytes: number;
  signedUrlTtlSeconds: number;
  policy: UploadPolicy | null;
  fileCount: number;
  totalSize: number;
  categoryCount: number;
  byKind: UsageStat[];
  topExtensions: UsageStat[];
  daily: UsageStat[];
};

/** 文件业务错误码（6xxxx） */
export const FileErrorCode = {
  CATEGORY_IN_USE: 60001,
  TYPE_NOT_ALLOWED: 60002,
  TOO_LARGE: 60003,
  CONTENT_MISMATCH: 60004,
  LINK_INVALID: 60005,
  LINK_EXPIRED: 60006,
  STORAGE_FAILED: 60007,
  EMPTY_FILE: 60008
} as const;

const enc = encodeURIComponent;

export const listFiles = (query: FileQuery) =>
  http.get<Page<FileItem>>("/files", compactParams(query));

export const getFile = (id: string) => http.get<FileItem>(`/files/${enc(id)}`);

export const updateFile = (id: string, data: FileUpdatePayload) =>
  http.put<FileItem>(`/files/${enc(id)}`, data);

export const deleteFile = (id: string) =>
  http.delete<null>(`/files/${enc(id)}`);

export const getSignedUrl = (id: string) =>
  http.get<SignedUrl>(`/files/${enc(id)}/signed-url`);

export const getUploadPolicy = () =>
  http.get<UploadPolicy>("/files/upload-policy");

export const getStorageStats = () => http.get<StorageStats>("/files/storage");

export const listCategories = () => http.get<CategoryList>("/files/categories");

export const createCategory = (
  data: CategoryPayload,
  config?: PureHttpRequestConfig
) => http.post<FileCategory>("/files/categories", data, config);

export const updateCategory = (
  id: string,
  data: CategoryPayload,
  config?: PureHttpRequestConfig
) => http.put<FileCategory>(`/files/categories/${enc(id)}`, data, config);

export const deleteCategory = (id: string) =>
  http.delete<null>(`/files/categories/${enc(id)}`);

export type UploadOptions = {
  categoryId?: string | null;
  onProgress?: (percent: number) => void;
  signal?: AbortSignal;
};

/**
 * 上传单个文件。走共享 axios 实例，Cookie 与 XSRF 头会自动带上；
 * 错误由调用方逐个文件展示，因此跳过全局错误提示。
 */
export function uploadFile(file: File, opts: UploadOptions = {}) {
  const form = new FormData();
  // 分类字段放在文件之前，服务端流式读取时即可拿到
  if (opts.categoryId) form.append("categoryId", opts.categoryId);
  form.append("file", file, file.name);
  return http.request<FileItem>(
    "post",
    "/files",
    { data: form },
    {
      timeout: 0,
      skipErrorHandler: true,
      signal: opts.signal,
      onUploadProgress: (e: AxiosProgressEvent) => {
        if (!opts.onProgress) return;
        const total = e.total ?? file.size;
        opts.onProgress(
          total > 0 ? Math.min(100, Math.round((e.loaded / total) * 100)) : 0
        );
      }
    }
  );
}

/** 业务附件（头像、图片、文档）；url 存入业务表，访问需要登录 */
export type Attachment = {
  id: string;
  name: string;
  /** 站内地址，如 /api/v1/files/a/2026/10/30/<随机串>.png */
  url: string;
  size: number;
  kind: FileKind;
  mimeType: string;
};

/** 业务表中保存的附件信息（jsonb 字段），由 Attachment 取 name、url、size */
export type AttachmentRef = { name: string; url: string; size: number };

/**
 * 上传业务附件：登录用户即可上传，规则同文件管理；image 为 true 时只接受图片。
 * 错误由调用方提示（跳过全局错误提示）。
 */
export function uploadAttachment(
  file: File,
  opts: { image?: boolean; onProgress?: (percent: number) => void } = {}
) {
  const form = new FormData();
  form.append("file", file, file.name);
  return http.request<Attachment>(
    "post",
    "/files/attachments",
    { data: form, params: opts.image ? { kind: "image" } : undefined },
    {
      timeout: 0,
      skipErrorHandler: true,
      onUploadProgress: (e: AxiosProgressEvent) => {
        const total = e.total ?? file.size;
        opts.onProgress?.(
          total > 0 ? Math.min(100, Math.round((e.loaded / total) * 100)) : 0
        );
      }
    }
  );
}
