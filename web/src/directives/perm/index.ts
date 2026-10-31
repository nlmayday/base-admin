import {
  shallowRef,
  watchEffect,
  type Directive,
  type DirectiveBinding,
  type ShallowRef,
  type WatchStopHandle
} from "vue";
import { hasPerm, type PermValue } from "@/utils/permission";

type PermState = {
  value: ShallowRef<PermValue>;
  stop: WatchStopHandle;
  display: string;
};

const states = new WeakMap<HTMLElement, PermState>();

function apply(el: HTMLElement, allowed: boolean, display: string): void {
  el.style.display = allowed ? display : "none";
  if (allowed) el.removeAttribute("data-perm-denied");
  else el.setAttribute("data-perm-denied", "");
}

function assertValue(binding: DirectiveBinding<PermValue>): void {
  if (!binding.value || binding.value.length === 0) {
    throw new Error(
      "[Directive: perm]: need a permission code, like v-perm=\"'iam:user:create'\""
    );
  }
}

/**
 * `v-perm="'iam:user:create'"`：没有权限时隐藏元素。
 * 响应式跟随 Pinia 中的权限集合（重新拉取 /me 后自动更新）。
 * 采用隐藏而不是从 DOM 移除，避免破坏 Vue 对节点的后续 patch。
 */
export const perm: Directive<HTMLElement, PermValue> = {
  mounted(el, binding) {
    assertValue(binding);
    const value = shallowRef<PermValue>(binding.value);
    const display = el.style.display;
    const stop = watchEffect(() => apply(el, hasPerm(value.value), display));
    states.set(el, { value, stop, display });
  },
  updated(el, binding) {
    const state = states.get(el);
    if (!state) return;
    assertValue(binding);
    if (binding.value !== state.value.value) state.value.value = binding.value;
    // 组件重新渲染可能覆盖 style，这里重新应用一次
    apply(el, hasPerm(binding.value), state.display);
  },
  unmounted(el) {
    states.get(el)?.stop();
    states.delete(el);
  }
};
