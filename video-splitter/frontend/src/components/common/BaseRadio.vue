<script setup lang="ts">
defineProps<{
  modelValue?: string | number
  options: Array<{ value: string | number; label: string }>
  name: string
  label?: string
  disabled?: boolean
  direction?: 'horizontal' | 'vertical'
}>()

defineEmits<{
  'update:modelValue': [value: string | number]
}>()
</script>

<template>
  <div class="base-radio">
    <span v-if="label" class="base-radio__group-label">{{ label }}</span>
    <div class="base-radio__options" :class="`base-radio__options--${direction ?? 'horizontal'}`">
      <label
        v-for="opt in options"
        :key="opt.value"
        class="radio-label"
        :class="{ 'base-radio--disabled': disabled }"
      >
        <input
          type="radio"
          :name="name"
          :value="opt.value"
          :checked="modelValue === opt.value"
          :disabled="disabled"
          @change="$emit('update:modelValue', opt.value)"
        />
        {{ opt.label }}
      </label>
    </div>
  </div>
</template>

<style scoped src="./BaseRadio.scoped.css"></style>
