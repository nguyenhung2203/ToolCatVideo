<script setup lang="ts">
defineProps<{
  variant?: 'primary' | 'secondary' | 'neutral' | 'ghost' | 'danger' | 'success' | 'warning' | 'cta' | 'link' | 'text'
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'icon'
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  loading?: boolean
  icon?: string
  iconRight?: string
  block?: boolean
}>()

defineEmits<{
  click: [event: MouseEvent]
}>()
</script>

<template>
  <button
    class="wx-btn"
    :class="[
      `wx-btn--${variant ?? 'primary'}`,
      `wx-btn--${size ?? 'md'}`,
      { 'wx-btn--loading': loading, 'wx-btn--block': block },
    ]"
    :type="type ?? 'button'"
    :disabled="disabled || loading"
    @click="$emit('click', $event)"
  >
    <span v-if="loading" class="wx-btn__spinner" aria-hidden="true" />
    <span v-else-if="icon" class="wx-btn__icon" v-html="icon" aria-hidden="true" />
    <span v-if="$slots.default" class="wx-btn__label"><slot /></span>
    <span v-if="iconRight && !loading" class="wx-btn__icon" v-html="iconRight" aria-hidden="true" />
    <span v-if="variant === 'primary' || variant === 'cta'" class="wx-btn__shine" aria-hidden="true" />
  </button>
</template>

<style scoped src="./BaseButton.scoped.css"></style>
