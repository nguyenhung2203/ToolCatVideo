<script setup lang="ts">
/**
 * FormField — wrapper cho input/select/textarea với label, hint, error.
 * Phase 3 — form composition.
 *
 * defaults phần II:
 *  - labelPlacement: 'top'
 *  - required: false
 *  - showError: 'on-blur' (logic showError do parent quyết định khi nào pass `error`)
 *
 * anatomy: root → label → control → hint → error
 */
import { computed, useSlots } from 'vue'

const props = withDefaults(defineProps<{
  /** label sentence case */
  label?: string
  /** mô tả thêm dưới label */
  description?: string
  /** placement của label — mặc định 'top' (label trên, control dưới) */
  labelPlacement?: 'top' | 'left'
  /** required marker (dấu *) */
  required?: boolean
  /** optional marker (chữ "(không bắt buộc)") nếu không required */
  showOptional?: boolean
  /** hint hiện ở dưới control */
  hint?: string
  /** thông báo lỗi — nếu set, ưu tiên hơn hint */
  error?: string | null
  /** id để link với control */
  fieldId?: string
  /** disabled — fade label */
  disabled?: boolean
  /** thu gọn padding cho table inline */
  dense?: boolean
  /** @deprecated dùng `labelPlacement`. legacy: 'vertical' = top, 'horizontal' = left */
  layout?: 'horizontal' | 'vertical'
  /** @deprecated dùng `hint` */
  helpText?: string
  /** @deprecated dùng `labelPlacement` ('top' = vertical) */
  vertical?: boolean
}>(), {
  required: false,
  showOptional: false,
  disabled: false,
  dense: false,
})

const slots = useSlots()

const resolvedPlacement = computed<'top' | 'left'>(() => {
  if (props.labelPlacement) return props.labelPlacement
  if (props.vertical) return 'top'
  if (props.layout === 'vertical') return 'top'
  if (props.layout === 'horizontal') return 'left'
  return 'top'
})

const resolvedHint = computed(() => props.hint ?? props.helpText)

const isHorizontal = computed(() => resolvedPlacement.value === 'left')
const hasError = computed(() => Boolean(props.error))
const hasHint = computed(() => Boolean(resolvedHint.value || slots.hint))
const fieldId = computed(() => props.fieldId)
const describedById = computed(() => fieldId.value ? `${fieldId.value}-desc` : undefined)
const errorId = computed(() => fieldId.value ? `${fieldId.value}-err` : undefined)
</script>

<template>
  <div
    class="wx-field"
    :data-layout="isHorizontal ? 'horizontal' : 'vertical'"
    :data-state="hasError ? 'invalid' : disabled ? 'disabled' : 'default'"
    :class="{ 'wx-field--dense': dense }"
  >
    <div v-if="label || $slots.label" class="wx-field__label-wrap">
      <label
        :for="fieldId"
        class="wx-field__label"
        data-part="label"
      >
        <slot name="label">{{ label }}</slot>
        <span
          v-if="required"
          class="wx-field__required"
          aria-hidden="true"
        >*</span>
        <span
          v-else-if="showOptional"
          class="wx-field__optional"
        >(không bắt buộc)</span>
      </label>
      <p v-if="description" class="wx-field__description">{{ description }}</p>
    </div>

    <div class="wx-field__control" data-part="control">
      <slot
        :field-id="fieldId"
        :described-by="hasError ? errorId : (hasHint ? describedById : undefined)"
        :invalid="hasError"
      />

      <p
        v-if="hasError"
        :id="errorId"
        class="wx-field__error"
        data-part="error"
        role="alert"
      >
        <svg
          width="14" height="14" viewBox="0 0 24 24"
          fill="none" stroke="currentColor"
          stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
          aria-hidden="true"
        >
          <circle cx="12" cy="12" r="10" />
          <line x1="12" y1="8" x2="12" y2="12" />
          <line x1="12" y1="16" x2="12.01" y2="16" />
        </svg>
        <span>{{ error }}</span>
      </p>

      <p
        v-else-if="hasHint"
        :id="describedById"
        class="wx-field__hint"
        data-part="hint"
      >
        <slot name="hint">{{ resolvedHint }}</slot>
      </p>
    </div>
  </div>
</template>

<style scoped src="./FormField.scoped.css"></style>
