<script setup lang="ts">
/**
 * FormDrawer — wrapper pattern over BaseDrawer for slide-in form panels.
 * Dùng cho form dài hoặc nhiều field hơn FormModal.
 *
 * Usage:
 *   <FormDrawer v-model="show" title="Chỉnh sửa liên hệ" @submit="handleSubmit">
 *     <FormField label="Tên"><BaseInput v-model="form.name" /></FormField>
 *   </FormDrawer>
 */
import BaseDrawer from './BaseDrawer.vue'
import BaseButton from './BaseButton.vue'

withDefaults(defineProps<{
  modelValue: boolean
  title: string
  placement?: 'left' | 'right'
  size?: 'sm' | 'md' | 'lg' | 'xl'
  loading?: boolean
  submitLabel?: string
  cancelLabel?: string
  danger?: boolean
  submitDisabled?: boolean
  showSubmit?: boolean
}>(), {
  placement: 'right',
  size: 'md',
  loading: false,
  submitLabel: 'Lưu',
  cancelLabel: 'Huỷ',
  danger: false,
  submitDisabled: false,
  showSubmit: true,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  submit: []
}>()

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <BaseDrawer
    :show="modelValue"
    :title="title"
    :placement="placement"
    :size="size"
    @update:show="emit('update:modelValue', $event)"
    @close="close"
  >
    <div class="fd-body">
      <slot />
    </div>

    <template #footer>
      <BaseButton variant="ghost" :disabled="loading" @click="close">
        {{ cancelLabel }}
      </BaseButton>
      <BaseButton
        v-if="showSubmit"
        :variant="danger ? 'danger' : 'primary'"
        :loading="loading"
        :disabled="submitDisabled"
        @click="emit('submit')"
      >
        {{ submitLabel }}
      </BaseButton>
    </template>
  </BaseDrawer>
</template>

<style scoped>
.fd-body {
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-4);
}
</style>
