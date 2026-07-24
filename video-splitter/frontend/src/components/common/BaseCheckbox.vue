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

<style scoped src="./BaseCheckbox.scoped.css"></style>
