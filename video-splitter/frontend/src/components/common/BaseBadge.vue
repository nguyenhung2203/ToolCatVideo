<script setup lang="ts">
defineProps<{
  text?: string | number
  variant?: 'primary' | 'success' | 'warning' | 'danger' | 'info' | 'ghost' | 'neutral'
  size?: 'sm' | 'md' | 'lg'
  dot?: boolean
  pulsing?: boolean
  /** Solid fill (notification count style) — bg đậm + text trắng + viền trắng */
  solid?: boolean
}>()
</script>

<template>
  <span
    class="base-badge"
    :class="[
      `base-badge--${variant ?? 'primary'}`,
      `base-badge--${size ?? 'md'}`,
      {
        'base-badge--dot-only': dot && !text && !$slots.default,
        'base-badge--with-dot': dot && (text || $slots.default),
        'base-badge--pulsing': pulsing,
        'base-badge--solid': solid,
      }
    ]"
  >
    <span v-if="dot && (text || $slots.default)" class="base-badge__dot" />
    <slot>{{ text }}</slot>
  </span>
</template>

<style scoped src="./BaseBadge.scoped.css"></style>
