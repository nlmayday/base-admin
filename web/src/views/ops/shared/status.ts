import type { RunStatus } from "@/api/ops/job";
import type { RunTrigger } from "@/api/ops/run";

type TagType = "primary" | "success" | "warning" | "danger" | "info";

export const RUN_STATUS_META: Record<
  RunStatus | "",
  { label: string; type: TagType }
> = {
  running: { label: "运行中", type: "primary" },
  success: { label: "成功", type: "success" },
  failed: { label: "失败", type: "danger" },
  timeout: { label: "超时", type: "warning" },
  skipped: { label: "已跳过", type: "info" },
  "": { label: "未执行", type: "info" }
};

export const RUN_STATUS_OPTIONS = (
  ["running", "success", "failed", "timeout", "skipped"] as RunStatus[]
).map(value => ({ value, label: RUN_STATUS_META[value].label }));

export const TRIGGER_META: Record<
  RunTrigger,
  { label: string; type: TagType }
> = {
  cron: { label: "定时", type: "info" },
  manual: { label: "手动", type: "primary" }
};

export const TRIGGER_OPTIONS = (["cron", "manual"] as RunTrigger[]).map(
  value => ({ value, label: TRIGGER_META[value].label })
);
