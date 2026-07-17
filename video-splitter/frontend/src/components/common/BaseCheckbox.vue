<script setup lang="ts">
import { ref, watch } from 'vue'

const props = defineProps<{
  modelValue?: boolean
  label?: string
  disabled?: boolean
  indeterminate?: boolean
  error?: boolean
  size?: 'sm' | 'md'
}>()

defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const inputRef = ref<HTMLInputElement>()

watch(() => props.indeterminate, (val) => {
  if (inputRef.value) inputRef.value.indeterminate = !!val
}, { immediate: true })
</script>

<template>
  <label
    class="chk-label base-checkbox"
    :class="{
      'base-checkbox--disabled': disabled,
      'base-checkbox--error': error,
      'base-checkbox--sm': size === 'sm',
    }"
  >
    <span class="chk-box" :class="{ 'chk-box--checked': modelValue, 'chk-box--indeterminate': indeterminate }">
      <input
        ref="inputRef"
        class="chk-input"
        type="checkbox"
        :checked="modelValue"
        :disabled="disabled"
        @change="$emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
      />
      <!-- check SVG -->
      <svg
        v-if="modelValue && !indeterminate"
        class="chk-icon"
        width="10" height="10"
        viewBox="0 0 12 12"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        <polyline points="2 6 5 9 10 3"/>
      </svg>
      <!-- indeterminate dash -->
      <svg
        v-else-if="indeterminate"
        class="chk-icon"
        width="10" height="10"
        viewBox="0 0 12 12"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        stroke-linecap="round"
        aria-hidden="true"
      >
        <line x1="2" y1="6" x2="10" y2="6"/>
      </svg>
    </span>
    <span v-if="label" class="chk-text">{{ label }}</span>
  </label>
</template>

<style scoped>
.base-checkbox {
  display: inline-flex;
  align-items: flex-start;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

/* hidden native input */
.chk-input {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
  pointer-events: none;
}

/* custom box */
.chk-box {
  flex-shrink: 0;
  width: 16px;
  height: 16px;
  border-radius: var(--wx-radius-sm, 4px);
  border: 1.5px solid var(--wx-border-default, #e5e7eb);
  background: var(--wx-surface-sunken, #f8fafc);
  display: flex;
  align-items: center;
  justify-content: center;
  position: relative;
  margin-top: 2px;
  transition:
    border-color var(--wx-duration-fast, 150ms) var(--wx-ease-standard, cubic-bezier(.4,0,.2,1)),
    background var(--wx-duration-fast, 150ms) var(--wx-ease-standard, cubic-bezier(.4,0,.2,1)),
    box-shadow var(--wx-duration-fast, 150ms) var(--wx-ease-standard, cubic-bezier(.4,0,.2,1));
}

.chk-box--checked,
.chk-box--indeterminate {
  border-color: var(--wx-brand-primary, #2563eb);
  background: var(--wx-brand-primary, #2563eb);
}

.chk-icon {
  color: #fff;
  animation: chk-pop var(--wx-duration-fast, 150ms) var(--wx-ease-decelerate, cubic-bezier(0,0,.2,1));
}
@keyframes chk-pop {
  from { opacity: 0; transform: scale(.4); }
  to   { opacity: 1; transform: scale(1); }
}

/* focus ring via label:focus-within */
.base-checkbox:focus-within .chk-box {
  outline: none;
  box-shadow: 0 0 0 3px rgba(0,123,255,.25);
}

.chk-text {
  font-size: var(--wx-fs-13, 13px);
  font-family: var(--wx-font-primary);
  color: var(--wx-text-primary, #1e293b);
  line-height: 1.5;
}

/* error state */
.base-checkbox--error .chk-box {
  border-color: var(--wx-danger-solid, #dc2626);
}
.base-checkbox--error .chk-text {
  color: var(--wx-danger-text, #dc2626);
}

/* disabled */
.base-checkbox--disabled {
  opacity: 0.5;
  cursor: not-allowed;
  pointer-events: none;
}

/* sm */
.base-checkbox--sm .chk-box {
  width: 14px;
  height: 14px;
}
.base-checkbox--sm .chk-text { font-size: var(--wx-fs-12, 12px); }

@media (prefers-reduced-motion: reduce) {
  .chk-box { transition: none; }
  .chk-icon { animation: none; }
}
</style>
