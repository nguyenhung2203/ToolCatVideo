<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  value: number
  max?: number
  variant?: 'primary' | 'success' | 'warning' | 'error' | 'danger'
  size?: 'sm' | 'md' | 'lg'
  striped?: boolean
  animated?: boolean
  showLabel?: boolean
}>(), {
  max: 100,
  variant: 'primary',
  size: 'md',
  striped: false,
  animated: false,
  showLabel: false,
})

const percent = computed(() => Math.min(100, Math.max(0, (props.value / props.max) * 100)))
</script>

<template>
  <div class="base-progress" :class="`base-progress--${size}`">
    <div class="base-progress__track">
      <div
        class="base-progress__fill"
        :class="[
          `base-progress__fill--${variant}`,
          { 'base-progress__fill--striped': striped || animated },
          { 'base-progress__fill--animated': animated }
        ]"
        :style="{ width: percent + '%' }"
      />
    </div>
    <span v-if="showLabel || $slots.label" class="base-progress__label">
      <slot name="label">{{ Math.round(percent) }}%</slot>
    </span>
  </div>
</template>

<style scoped src="./BaseProgress.scoped.css"></style>
