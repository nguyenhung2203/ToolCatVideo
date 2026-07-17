<script setup lang="ts">
import { ref } from 'vue'

const props = withDefaults(defineProps<{
  content: string
  placement?: 'top' | 'right' | 'bottom' | 'left'
  delay?: number
  disabled?: boolean
}>(), {
  placement: 'top',
  delay: 300,
  disabled: false,
})

const visible = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

function show() {
  if (props.disabled) return
  timer = setTimeout(() => { visible.value = true }, props.delay)
}

function hide() {
  if (timer) { clearTimeout(timer); timer = null }
  visible.value = false
}
</script>

<template>
  <div class="base-tooltip-wrapper" @mouseenter="show" @mouseleave="hide">
    <slot />
    <transition name="tooltip">
      <div v-if="visible && content" class="base-tooltip" :class="`base-tooltip--${placement}`">
        <span class="base-tooltip__arrow" />
        {{ content }}
      </div>
    </transition>
  </div>
</template>

<style scoped>
.base-tooltip-wrapper {
  position: relative;
  display: inline-flex;
}

.base-tooltip {
  position: absolute;
  z-index: var(--wx-z-tooltip);
  padding: 5px 10px;
  font-size: 12px;
  color: #fff;
  background: #333;
  border-radius: 4px;
  white-space: nowrap;
  pointer-events: none;
  line-height: 1.4;
  font-family: var(--font-family);
}

.base-tooltip__arrow {
  position: absolute;
  width: 6px;
  height: 6px;
  background: #333;
  transform: rotate(45deg);
}

/* Placements */
.base-tooltip--top {
  bottom: calc(100% + 8px);
  left: 50%;
  transform: translateX(-50%);
}
.base-tooltip--top .base-tooltip__arrow {
  bottom: -3px;
  left: 50%;
  margin-left: -3px;
}

.base-tooltip--bottom {
  top: calc(100% + 8px);
  left: 50%;
  transform: translateX(-50%);
}
.base-tooltip--bottom .base-tooltip__arrow {
  top: -3px;
  left: 50%;
  margin-left: -3px;
}

.base-tooltip--left {
  right: calc(100% + 8px);
  top: 50%;
  transform: translateY(-50%);
}
.base-tooltip--left .base-tooltip__arrow {
  right: -3px;
  top: 50%;
  margin-top: -3px;
}

.base-tooltip--right {
  left: calc(100% + 8px);
  top: 50%;
  transform: translateY(-50%);
}
.base-tooltip--right .base-tooltip__arrow {
  left: -3px;
  top: 50%;
  margin-top: -3px;
}

/* Transition */
.tooltip-enter-active { transition: opacity 0.15s ease; }
.tooltip-leave-active { transition: opacity 0.1s ease; }
.tooltip-enter-from, .tooltip-leave-to { opacity: 0; }
</style>
