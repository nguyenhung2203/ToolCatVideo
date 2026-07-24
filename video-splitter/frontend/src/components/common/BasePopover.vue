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

<style scoped src="./BasePopover.scoped.css"></style>
