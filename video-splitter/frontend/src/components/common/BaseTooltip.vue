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

<style scoped src="./BaseTooltip.scoped.css"></style>
