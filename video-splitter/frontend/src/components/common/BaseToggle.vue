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

<style scoped src="./BaseToggle.scoped.css"></style>
