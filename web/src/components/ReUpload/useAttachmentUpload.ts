import { ref } from "vue";
import {
  getUploadPolicy,
  uploadAttachment,
  type Attachment,
  type UploadPolicy
} from "@/api/file";
import { isApiError } from "@/utils/http/errors";
import { message } from "@/utils/message";

let policyRequest: Promise<UploadPolicy | null> | null = null;

/** 上传规则（全局只取一次），用于提示和上传前检查 */
function loadPolicy(): Promise<UploadPolicy | null> {
  policyRequest ??= getUploadPolicy().catch(() => {
    policyRequest = null;
    return null;
  });
  return policyRequest;
}

function sizeText(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1 << 20) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1 << 20)).toFixed(1).replace(/\.0$/, "")} MB`;
}

/** 上传组件共用：先按规则检查扩展名和大小，再上传并提示错误 */
export function useAttachmentUpload(image: boolean) {
  const uploading = ref(false);
  const percent = ref(0);
  const policy = ref<UploadPolicy | null>(null);
  loadPolicy().then(p => (policy.value = p));

  function check(file: File): boolean {
    const p = policy.value;
    if (image && !file.type.startsWith("image/")) {
      message("只能上传图片", { type: "warning" });
      return false;
    }
    if (!p) return true;
    const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
    if (!p.extensions.includes(ext)) {
      message(`不支持 .${ext} 文件，允许：${p.extensions.join("、")}`, {
        type: "warning"
      });
      return false;
    }
    if (file.size > p.maxBytes) {
      message(`文件不能超过 ${sizeText(p.maxBytes)}`, { type: "warning" });
      return false;
    }
    return true;
  }

  async function upload(file: File): Promise<Attachment | null> {
    if (!check(file)) return null;
    uploading.value = true;
    percent.value = 0;
    try {
      return await uploadAttachment(file, {
        image,
        onProgress: p => (percent.value = p)
      });
    } catch (err) {
      message(isApiError(err) ? err.message : "上传失败，请稍后重试", {
        type: "error"
      });
      return null;
    } finally {
      uploading.value = false;
    }
  }

  return { uploading, percent, policy, upload, sizeText };
}
