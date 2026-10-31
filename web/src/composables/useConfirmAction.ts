import { ElMessageBox } from "element-plus";
import { message } from "@/utils/message";
import { needsReload } from "@/utils/http/errors";

type ConfirmActionOptions = {
  /** 确认框文案 */
  confirm: string;
  /** 成功提示 */
  success: string;
  action: () => Promise<unknown>;
  /** 成功或需要刷新（409/结果未知）时调用 */
  reload: () => unknown;
  type?: "warning" | "info";
};

/** 二次确认后执行操作；错误提示由 http 拦截器统一处理 */
export async function confirmAction(opts: ConfirmActionOptions): Promise<void> {
  try {
    await ElMessageBox.confirm(opts.confirm, "提示", {
      type: opts.type ?? "warning",
      confirmButtonText: "确定",
      cancelButtonText: "取消"
    });
  } catch {
    return; // 用户取消
  }
  try {
    await opts.action();
    message(opts.success, { type: "success" });
    opts.reload();
  } catch (err) {
    if (needsReload(err)) opts.reload();
  }
}
