import { ref, type Ref } from "vue";
import { ErrorCode, isApiError } from "@/utils/http/errors";

export type FieldErrors = Record<string, string>;

export type UseFieldErrors = {
  /** 绑定到 el-form-item 的 :error，如 :error="fieldErrors.username" */
  fieldErrors: Ref<FieldErrors>;
  clearFieldErrors: () => void;
  /** 422 / 10002 时写入字段错误，返回是否已处理 */
  applyFieldErrors: (err: unknown) => boolean;
};

/** 把后端返回的字段错误显示在表单上 */
export function useFieldErrors(): UseFieldErrors {
  const fieldErrors = ref<FieldErrors>({});

  function clearFieldErrors(): void {
    fieldErrors.value = {};
  }

  function applyFieldErrors(err: unknown): boolean {
    if (!isApiError(err) || err.code !== ErrorCode.VALIDATION) return false;
    const fields = err.fields;
    if (fields.length === 0) return false;
    fieldErrors.value = Object.fromEntries(
      fields.map(f => [f.field, f.message])
    );
    return true;
  }

  return { fieldErrors, clearFieldErrors, applyFieldErrors };
}
