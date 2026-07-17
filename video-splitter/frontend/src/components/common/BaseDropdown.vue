<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'

defineProps<{
  placement?: 'bottom-start' | 'bottom-end' | 'top-start' | 'top-end'
}>()

const isOpen = ref(false)
const triggerRef = ref<HTMLElement>()
const dropRef = ref<HTMLElement>()

function toggle() {
  isOpen.value = !isOpen.value
  if (isOpen.value) nextTick(adjustPosition)
}

function close() {
  isOpen.value = false
}

function adjustPosition() {
  if (!dropRef.value || !triggerRef.value) return
  const drop = dropRef.value
  const rect = drop.getBoundingClientRect()
  if (rect.bottom > window.innerHeight) {
    drop.style.top = 'auto'
    drop.style.bottom = '100%'
    drop.style.marginBottom = '4px'
    drop.style.marginTop = '0'
  }
  if (rect.right > window.innerWidth) {
    drop.style.left = 'auto'
    drop.style.right = '0'
  }
}

function onClickOutside(e: MouseEvent) {
  if (!triggerRef.value?.contains(e.target as Node) && !dropRef.value?.contains(e.target as Node)) {
    close()
  }
}

onMounted(() => document.addEventListener('click', onClickOutside))
onBeforeUnmount(() => document.removeEventListener('click', onClickOutside))
</script>

<template>
  <div class="base-dropdown" :class="[`base-dropdown--${placement ?? 'bottom-start'}`]">
    <div ref="triggerRef" class="base-dropdown__trigger" @click.stop="toggle">
      <slot name="trigger" />
    </div>
    <transition name="dropdown">
      <div v-if="isOpen" ref="dropRef" class="base-dropdown__content" @click.stop>
        <slot :close="close" />
      </div>
    </transition>
  </div>
</template>

<style scoped>
.base-dropdown {
  position: relative;
  display: inline-block;
}

.base-dropdown__trigger {
  cursor: pointer;
}

.base-dropdown__content {
  position: absolute;
  z-index: 100;
  margin-top: 6px;
  background: var(--wx-surface-base);
  border: 1px solid var(--wx-border-subtle);
  border-radius: var(--wx-radius-xl);
  box-shadow:
    0 4px 6px -1px rgba(0,0,0,0.08),
    0 10px 24px -4px rgba(0,0,0,0.12);
  min-width: 160px;
  overflow: hidden;
}

.base-dropdown--bottom-start .base-dropdown__content { left: 0; top: 100%; }
.base-dropdown--bottom-end .base-dropdown__content { right: 0; top: 100%; }
.base-dropdown--top-start .base-dropdown__content { left: 0; bottom: 100%; margin-top: 0; margin-bottom: 4px; }
.base-dropdown--top-end .base-dropdown__content { right: 0; bottom: 100%; margin-top: 0; margin-bottom: 4px; }

/* Transition */
.dropdown-enter-active { transition: all 0.15s ease; }
.dropdown-leave-active { transition: all 0.1s ease; }
.dropdown-enter-from, .dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
