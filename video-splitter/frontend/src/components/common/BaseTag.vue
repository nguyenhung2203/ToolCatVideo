<script setup lang="ts">
/**
 * BaseTag — a small inline chip with optional icon, dot, removable.
 * Defaults: variant='neutral', size='md', removable=false, radius='full' (pill).
 *
 * Different from BaseBadge: tags are interactive (click, removable),
 * badges are status indicators.
 */
defineProps<{
  label?: string
  variant?: 'neutral' | 'primary' | 'success' | 'warning' | 'danger' | 'info'
  size?: 'sm' | 'md' | 'lg'
  removable?: boolean
  disabled?: boolean
  shape?: 'pill' | 'square'
  dot?: boolean
}>()

const emit = defineEmits<{
  remove: [event: MouseEvent]
  click: [event: MouseEvent]
}>()

function onRemove(e: MouseEvent) {
  e.stopPropagation()
  emit('remove', e)
}
</script>

<template>
  <span
    class="wx-tag"
    :data-variant="variant ?? 'neutral'"
    :data-size="size ?? 'md'"
    :data-shape="shape ?? 'pill'"
    :data-state="disabled ? 'disabled' : 'default'"
    @click="emit('click', $event)"
  >
    <span v-if="dot" class="wx-tag__dot" />
    <slot>{{ label }}</slot>
    <button
      v-if="removable && !disabled"
      type="button"
      class="wx-tag__remove"
      :aria-label="'xoá ' + (label ?? '')"
      @click="onRemove"
    >
      <svg width="10" height="10" viewBox="0 0 10 10" fill="none">
        <path d="M2 2 L8 8 M8 2 L2 8" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
      </svg>
    </button>
  </span>
</template>

<style scoped src="./BaseTag.scoped.css"></style>
