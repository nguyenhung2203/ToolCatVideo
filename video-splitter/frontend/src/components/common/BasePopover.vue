<script setup lang="ts">
/**
 * BasePopover — rich-content floating panel with arrow.
 *
 * Defaults: placement='bottom', trigger='click', radius='lg' (12px).
 *
 * Khác BaseTooltip: tooltip chỉ chứa text ngắn, popover chứa rich content
 *  (form, list, link…). Khác BaseDropdown: dropdown là menu list, popover
 *  free-form via slot.
 */
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'

const props = withDefaults(defineProps<{
  placement?: 'top' | 'bottom' | 'left' | 'right'
  trigger?: 'click' | 'hover'
  showArrow?: boolean
  width?: string
}>(), {
  placement: 'bottom',
  trigger: 'click',
  showArrow: true,
  width: '280px',
})

const isOpen = ref(false)
const triggerRef = ref<HTMLElement>()
const popRef = ref<HTMLElement>()

function open()  { isOpen.value = true }
function close() { isOpen.value = false }
function toggle() { isOpen.value ? close() : open() }

function onClickOutside(e: MouseEvent) {
  if (!isOpen.value) return
  const t = e.target as Node
  if (popRef.value?.contains(t) || triggerRef.value?.contains(t)) return
  close()
}
function onEsc(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) { e.stopPropagation(); close() }
}

onMounted(() => {
  document.addEventListener('mousedown', onClickOutside)
  document.addEventListener('keydown', onEsc)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onClickOutside)
  document.removeEventListener('keydown', onEsc)
})

defineExpose({ open, close, toggle })
</script>

<template>
  <span class="wx-popover">
    <span
      ref="triggerRef"
      class="wx-popover__trigger"
      @click="trigger === 'click' && toggle()"
      @mouseenter="trigger === 'hover' && open()"
      @mouseleave="trigger === 'hover' && close()"
    >
      <slot name="trigger" :open="isOpen" />
    </span>

    <transition name="wx-popover">
      <div
        v-if="isOpen"
        ref="popRef"
        class="wx-popover__panel"
        :data-placement="placement"
        :data-arrow="showArrow || undefined"
        :style="{ width }"
        role="dialog"
      >
        <span v-if="showArrow" class="wx-popover__arrow" aria-hidden="true" />
        <div class="wx-popover__body">
          <slot :close="close" />
        </div>
      </div>
    </transition>
  </span>
</template>

<style scoped>
.wx-popover {
  position: relative;
  display: inline-flex;
}
.wx-popover__trigger { display: inline-flex; }

.wx-popover__panel {
  position: absolute;
  background: var(--wx-bg-base);
  color: var(--wx-content-primary);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-lg);
  box-shadow: var(--wx-shadow-lg);
  z-index: var(--wx-z-popover);
  font-family: var(--wx-font-primary);
  font-size: var(--wx-fs-14);
}

.wx-popover__panel[data-placement="bottom"] {
  top: calc(100% + 8px);
  left: 50%;
  transform: translateX(-50%);
}
.wx-popover__panel[data-placement="top"] {
  bottom: calc(100% + 8px);
  left: 50%;
  transform: translateX(-50%);
}
.wx-popover__panel[data-placement="right"] {
  left: calc(100% + 8px);
  top: 50%;
  transform: translateY(-50%);
}
.wx-popover__panel[data-placement="left"] {
  right: calc(100% + 8px);
  top: 50%;
  transform: translateY(-50%);
}

.wx-popover__body {
  padding: var(--wx-space-3);
}

/* arrow */
.wx-popover__arrow {
  position: absolute;
  width: 10px;
  height: 10px;
  background: var(--wx-bg-base);
  border: 1px solid var(--wx-border-default);
  transform: rotate(45deg);
}
.wx-popover__panel[data-placement="bottom"] .wx-popover__arrow {
  top: -6px; left: 50%; margin-left: -5px;
  border-right: none; border-bottom: none;
}
.wx-popover__panel[data-placement="top"] .wx-popover__arrow {
  bottom: -6px; left: 50%; margin-left: -5px;
  border-left: none; border-top: none;
}
.wx-popover__panel[data-placement="right"] .wx-popover__arrow {
  left: -6px; top: 50%; margin-top: -5px;
  border-right: none; border-top: none;
}
.wx-popover__panel[data-placement="left"] .wx-popover__arrow {
  right: -6px; top: 50%; margin-top: -5px;
  border-left: none; border-bottom: none;
}

/* transition */
.wx-popover-enter-active, .wx-popover-leave-active {
  transition: opacity var(--wx-d-fast) var(--wx-ease-standard),
              transform var(--wx-d-fast) var(--wx-ease-decelerate);
}
.wx-popover-enter-from, .wx-popover-leave-to {
  opacity: 0;
}
.wx-popover__panel[data-placement="bottom"].wx-popover-enter-from,
.wx-popover__panel[data-placement="bottom"].wx-popover-leave-to {
  transform: translateX(-50%) translateY(-4px);
}
.wx-popover__panel[data-placement="top"].wx-popover-enter-from,
.wx-popover__panel[data-placement="top"].wx-popover-leave-to {
  transform: translateX(-50%) translateY(4px);
}
</style>
