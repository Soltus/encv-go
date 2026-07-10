<!--
  EngineRenderer - 引擎渲染包装组件

  解决 <component :is="vnode"> 不稳定的问题：
  将 ChatEngine.renderMessages() 返回的 VNode 通过 render 函数直接渲染。

  使用纯 Options API（非 <script setup>），确保 render() 函数能正确追踪
  响应式依赖——Vue 3 的 render 追踪机制依赖 this 上访问的响应式属性，
  <script setup> + options render() 混用会导致追踪断裂。
-->
<script lang="ts">
import { defineComponent, type PropType } from "vue";
import type { ChatEngine, EngineRenderProps } from "@/composables/chatEngine";

export default defineComponent({
  name: "EngineRenderer",
  props: {
    engine: {
      type: Object as PropType<ChatEngine>,
      required: true,
    },
    renderProps: {
      type: Object as PropType<EngineRenderProps>,
      required: true,
    },
  },
  render() {
    // 通过 this 访问 props，确保 Vue 响应式追踪正常工作
    const engine = this.engine;
    const renderProps = this.renderProps;
    if (!engine || !renderProps) return null;
    return engine.renderMessages(renderProps);
  },
});
</script>
