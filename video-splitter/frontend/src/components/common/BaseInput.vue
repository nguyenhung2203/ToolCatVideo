<script lang="ts">
let _idCounter = 0
</script>

<script setup lang="ts">
import { ref, computed } from 'vue'

defineOptions({ inheritAttrs: false })

const props = defineProps<{
  modelValue?: string | number
  type?: 'text' | 'number' | 'password' | 'email' | 'search' | 'tel' | 'url'
  label?: string
  placeholder?: string
  error?: string
  /** error styling without message text — complementary to FormField :error */
  invalid?: boolean
  /** success state with check icon */
  success?: boolean
  disabled?: boolean
  readonly?: boolean
  size?: 'sm' | 'md'
  /** text alignment inside the input */
  align?: 'left' | 'center' | 'right'
}>()

const inputId = `base-input-${++_idCounter}`
const errorId = `${inputId}-error`

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
  'blur': [event: FocusEvent]
  'focus': [event: FocusEvent]
}>()

const showPassword = ref(false)

const inputType = computed(() => {
  if (props.type === 'password') return showPassword.value ? 'text' : 'password'
  return props.type ?? 'text'
})

const hasError = computed(() => Boolean(props.error) || props.invalid)
</script>

<template>
  <div class="base-input" :class="[`base-input--${size ?? 'md'}`]">
    <label v-if="label" :for="inputId" class="base-input__label">{{ label }}</label>
    <div
      class="base-input__wrapper"
      :class="{
        'base-input__wrapper--error': hasError,
        'base-input__wrapper--success': success && !hasError,
        'base-input__wrapper--disabled': disabled,
      }"
    >
      <input
        v-bind="$attrs"
        :id="inputId"
        class="base-input__field"
        :class="{
          'base-input__field--has-toggle': type === 'password',
          'base-input__field--has-icon': success && !hasError,
          [`base-input__field--align-${align ?? (type === 'number' ? 'center' : 'left')}`]: true,
        }"
        :type="inputType"
        :value="modelValue"
        :placeholder="placeholder"
        :disabled="disabled"
        :readonly="readonly"
        :aria-invalid="hasError ? 'true' : undefined"
        :aria-describedby="error ? errorId : undefined"
        @input="emit('update:modelValue', ($event.target as HTMLInputElement).value)"
        @blur="emit('blur', $event)"
        @focus="emit('focus', $event)"
      />

      <!-- success check -->
      <span v-if="success && !hasError" class="base-input__icon base-input__icon--success" aria-hidden="true">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="20 6 9 17 4 12"/>
        </svg>
      </span>

      <!-- password toggle -->
      <button
        v-if="type === 'password'"
        type="button"
        class="base-input__eye"
        tabindex="-1"
        :aria-label="showPassword ? 'Ẩn mật khẩu' : 'Hiện mật khẩu'"
        @click="showPassword = !showPassword"
      >
        <!-- eye open -->
        <svg v-if="showPassword" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/>
          <circle cx="12" cy="12" r="3"/>
        </svg>
        <!-- eye off -->
        <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/>
          <line x1="1" y1="1" x2="23" y2="23"/>
        </svg>
      </button>
    </div>
    <span v-if="error" :id="errorId" role="alert" class="base-input__error">{{ error }}</span>
  </div>
</template>

<style scoped src="./BaseInput.scoped.css"></style>
