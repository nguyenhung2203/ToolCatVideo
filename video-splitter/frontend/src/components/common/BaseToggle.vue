<script setup lang="ts">
defineProps<{
  modelValue?: boolean
  label?: string
  disabled?: boolean
}>()

defineEmits<{
  'update:modelValue': [value: boolean]
}>()
</script>

<template>
  <label class="base-toggle" :class="{ 'base-toggle--disabled': disabled }">
    <span class="toggle">
      <input
        type="checkbox"
        :checked="modelValue"
        :disabled="disabled"
        @change="$emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
      />
      <span class="toggle-slider" />
    </span>
    <span v-if="label" class="base-toggle__label">{{ label }}</span>
  </label>
</template>

<style scoped>
.base-toggle {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
}

.base-toggle--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.base-toggle__label {
  font-size: 13px;
  color: var(--wx-text-secondary);
  user-select: none;
}
</style>
