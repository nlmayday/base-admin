// 构建时把 `import { ... } from "element-plus"` 指向这里（vite.config.ts 的
// resolve.alias）：从各组件子路径导入，避免主入口把全部组件打进包里。
// 只导出项目用到的运行时值；类型仍来自 element-plus。用到新组件时在这里补一行，
// 漏了会在构建时报 “is not exported”。

export { ElAlert } from "element-plus/es/components/alert/index.mjs";
export { ElAvatar } from "element-plus/es/components/avatar/index.mjs";
export { ElBacktop } from "element-plus/es/components/backtop/index.mjs";
export { ElBadge } from "element-plus/es/components/badge/index.mjs";
export {
  ElBreadcrumb,
  ElBreadcrumbItem
} from "element-plus/es/components/breadcrumb/index.mjs";
export { ElButton } from "element-plus/es/components/button/index.mjs";
export { ElCard } from "element-plus/es/components/card/index.mjs";
export {
  ElCheckbox,
  ElCheckboxGroup
} from "element-plus/es/components/checkbox/index.mjs";
export { ElCol } from "element-plus/es/components/col/index.mjs";
export {
  ElCollapse,
  ElCollapseItem
} from "element-plus/es/components/collapse/index.mjs";
export { ElConfigProvider } from "element-plus/es/components/config-provider/index.mjs";
export { ElDatePicker } from "element-plus/es/components/date-picker/index.mjs";
export {
  ElDescriptions,
  ElDescriptionsItem
} from "element-plus/es/components/descriptions/index.mjs";
export { ElDialog } from "element-plus/es/components/dialog/index.mjs";
export { ElDivider } from "element-plus/es/components/divider/index.mjs";
export { ElDrawer } from "element-plus/es/components/drawer/index.mjs";
export {
  ElDropdown,
  ElDropdownItem,
  ElDropdownMenu
} from "element-plus/es/components/dropdown/index.mjs";
export { ElEmpty } from "element-plus/es/components/empty/index.mjs";
export { ElForm, ElFormItem } from "element-plus/es/components/form/index.mjs";
export { ElIcon } from "element-plus/es/components/icon/index.mjs";
export { ElImage } from "element-plus/es/components/image/index.mjs";
export { ElInput } from "element-plus/es/components/input/index.mjs";
export { ElInputNumber } from "element-plus/es/components/input-number/index.mjs";
export { ElInputTag } from "element-plus/es/components/input-tag/index.mjs";
export { ElLink } from "element-plus/es/components/link/index.mjs";
export { ElLoading } from "element-plus/es/components/loading/index.mjs";
export {
  ElMenu,
  ElMenuItem,
  ElSubMenu
} from "element-plus/es/components/menu/index.mjs";
export { ElMessage } from "element-plus/es/components/message/index.mjs";
export { ElMessageBox } from "element-plus/es/components/message-box/index.mjs";
export { ElPagination } from "element-plus/es/components/pagination/index.mjs";
export { ElPopconfirm } from "element-plus/es/components/popconfirm/index.mjs";
export { ElPopover } from "element-plus/es/components/popover/index.mjs";
export { ElProgress } from "element-plus/es/components/progress/index.mjs";
export {
  ElRadio,
  ElRadioButton,
  ElRadioGroup
} from "element-plus/es/components/radio/index.mjs";
export { ElRow } from "element-plus/es/components/row/index.mjs";
export { ElScrollbar } from "element-plus/es/components/scrollbar/index.mjs";
export {
  ElOption,
  ElOptionGroup,
  ElSelect
} from "element-plus/es/components/select/index.mjs";
export { ElSkeleton } from "element-plus/es/components/skeleton/index.mjs";
export { ElSpace } from "element-plus/es/components/space/index.mjs";
export { ElSwitch } from "element-plus/es/components/switch/index.mjs";
export {
  ElTable,
  ElTableColumn
} from "element-plus/es/components/table/index.mjs";
export { ElTabPane, ElTabs } from "element-plus/es/components/tabs/index.mjs";
export { ElTag } from "element-plus/es/components/tag/index.mjs";
export { ElText } from "element-plus/es/components/text/index.mjs";
export {
  ElTimeline,
  ElTimelineItem
} from "element-plus/es/components/timeline/index.mjs";
export { ElTooltip } from "element-plus/es/components/tooltip/index.mjs";
export { ElTree } from "element-plus/es/components/tree/index.mjs";
export { ElTreeSelect } from "element-plus/es/components/tree-select/index.mjs";
export { ElUpload } from "element-plus/es/components/upload/index.mjs";
