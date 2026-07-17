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

<style scoped>
.base-radio__group-label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 4px;
}

.base-radio__options--horizontal {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
}

.base-radio__options--vertical {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.base-radio--disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
