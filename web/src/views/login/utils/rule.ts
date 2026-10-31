import { reactive } from "vue";
import type { FormRules } from "element-plus";

/** 密码长度（与后端一致：8–72 位） */
export const PASSWORD_MIN = 8;
export const PASSWORD_MAX = 72;

/** 登录校验：只校验必填，密码规则由后端判断 */
const loginRules = reactive<FormRules>({
  password: [{ required: true, message: "请输入密码", trigger: "blur" }]
});

export { loginRules };
