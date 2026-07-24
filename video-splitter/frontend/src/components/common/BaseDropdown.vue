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

<style scoped src="./BaseDropdown.scoped.css"></style>
