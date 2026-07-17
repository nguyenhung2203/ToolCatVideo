<script setup lang="ts">
/**
 * ConfirmDialog — accessible confirmation dialog với icon + description.
 * Dùng cho: xoá, archive, reset, revoke, disable.
 *
 * Usage:
 *   <ConfirmDialog
 *     v-model="show"
 *     title="Xoá người dùng?"
 *     description="Hành động này không thể hoàn tác."
 *     intent="danger"
 *     confirm-label="Xoá"
 *     :loading="deleting"
 *     @confirm="handleDelete"
 *   />
 */
import BaseModal from './BaseModal.vue'
import BaseButton from './BaseButton.vue'

type Intent = 'danger' | 'warning' | 'info'

withDefaults(defineProps<{
  modelValue: boolean
  title: string
  description?: string
  intent?: Intent
  confirmLabel?: string
  cancelLabel?: string
  loading?: boolean
}>(), {
  intent: 'danger',
  confirmLabel: 'Xác nhận',
  cancelLabel: 'Huỷ',
  loading: false,
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  confirm: []
}>()

const iconMap: Record<Intent, string> = {
  danger: `<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>`,
  warning: `<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="12"/><line x1="12" y1="16" x2="12.01" y2="16"/></svg>`,
  info:    `<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>`,
}

const confirmVariantMap: Record<Intent, 'danger' | 'warning' | 'primary'> = {
  danger:  'danger',
  warning: 'warning',
  info:    'primary',
}

function close() {
  emit('update:modelValue', false)
}
</script>

<template>
  <BaseModal
    :show="modelValue"
    :title="title"
    size="sm"
    :loading="loading"
    :intent="intent === 'info' ? 'default' : intent"
    @close="close"
  >
    <div class="cd-body">
      <div class="cd-icon" :class="`cd-icon--${intent}`" v-html="iconMap[intent]" aria-hidden="true" />
      <p v-if="description" class="cd-desc">{{ description }}</p>
    </div>

    <template #footer>
      <div class="cd-footer">
        <BaseButton variant="ghost" :disabled="loading" @click="close">
          {{ cancelLabel }}
        </BaseButton>
        <BaseButton
          :variant="confirmVariantMap[intent]"
          :loading="loading"
          @click="emit('confirm')"
        >
          {{ confirmLabel }}
        </BaseButton>
      </div>
    </template>
  </BaseModal>
</template>

<style scoped>
.cd-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
  padding: var(--wx-space-3) 0 var(--wx-space-2);
  gap: var(--wx-space-3);
}

.cd-icon {
  width: 52px;
  height: 52px;
  border-radius: var(--wx-radius-full);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.cd-icon--danger  { background: var(--wx-danger-bg);  color: var(--wx-danger-text); }
.cd-icon--warning { background: var(--wx-warning-bg); color: var(--wx-warning-text); }
.cd-icon--info    { background: var(--wx-info-bg);    color: var(--wx-info-text); }

.cd-desc {
  margin: 0;
  font-size: var(--wx-fs-14);
  color: var(--wx-text-secondary);
  line-height: var(--wx-lh-normal);
  max-width: 320px;
}

.cd-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--wx-space-2);
  width: 100%;
}
</style>
