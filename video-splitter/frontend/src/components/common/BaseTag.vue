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

<style scoped>
.wx-tag {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
  font-family: var(--wx-font-primary);
  font-weight: var(--wx-fw-medium);
  white-space: nowrap;
  line-height: 1;
  border: 1px solid transparent;
  transition: background var(--wx-d-micro) var(--wx-ease-standard),
              border-color var(--wx-d-micro) var(--wx-ease-standard);
}
.wx-tag[data-shape="pill"]   { border-radius: var(--wx-radius-full); }
.wx-tag[data-shape="square"] { border-radius: var(--wx-radius-sm); }

.wx-tag[data-size="sm"] { padding: 2px var(--wx-space-2); font-size: var(--wx-fs-12); }
.wx-tag[data-size="md"] { padding: 4px var(--wx-space-3); font-size: var(--wx-fs-13); }
.wx-tag[data-size="lg"] { padding: 6px var(--wx-space-4); font-size: var(--wx-fs-14); }

/* variants */
.wx-tag[data-variant="neutral"] {
  background: var(--wx-neutral-bg);
  color: var(--wx-neutral-text);
  border-color: var(--wx-neutral-border);
}
.wx-tag[data-variant="primary"] {
  background: var(--wx-info-bg);
  color: var(--wx-info-text);
  border-color: var(--wx-info-border);
}
.wx-tag[data-variant="success"] {
  background: var(--wx-success-bg);
  color: var(--wx-success-text);
  border-color: var(--wx-success-border);
}
.wx-tag[data-variant="warning"] {
  background: var(--wx-warning-bg);
  color: var(--wx-warning-text);
  border-color: var(--wx-warning-border);
}
.wx-tag[data-variant="danger"] {
  background: var(--wx-danger-bg);
  color: var(--wx-danger-text);
  border-color: var(--wx-danger-border);
}
.wx-tag[data-variant="info"] {
  background: var(--wx-info-bg);
  color: var(--wx-info-text);
  border-color: var(--wx-info-border);
}

.wx-tag[data-state="disabled"] {
  opacity: 0.5;
  cursor: not-allowed;
}

.wx-tag__dot {
  display: inline-block;
  width: 6px;
  height: 6px;
  border-radius: var(--wx-radius-full);
  background: currentColor;
  flex-shrink: 0;
}

.wx-tag__remove {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 14px;
  padding: 0;
  margin-left: 2px;
  border: none;
  background: transparent;
  color: currentColor;
  cursor: pointer;
  border-radius: var(--wx-radius-full);
  opacity: 0.6;
  transition: opacity var(--wx-d-micro) var(--wx-ease-standard),
              background var(--wx-d-micro) var(--wx-ease-standard);
}
.wx-tag__remove:hover { opacity: 1; background: rgba(0,0,0,0.08); }
.wx-tag__remove:focus-visible {
  outline: 2px solid var(--wx-border-focus);
  outline-offset: 1px;
}
</style>
