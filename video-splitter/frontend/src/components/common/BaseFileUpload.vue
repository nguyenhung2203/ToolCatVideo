<script setup lang="ts">
import { ref } from 'vue'

interface UploadFile {
  id: number
  file: File
}

const props = withDefaults(defineProps<{
  accept?: string
  multiple?: boolean
  maxSizeMb?: number
  disabled?: boolean
  modelValue?: File[]
}>(), {
  multiple: false,
  maxSizeMb: 10,
  disabled: false,
})

const emit = defineEmits<{
  'update:modelValue': [files: File[]]
  'error': [msg: string]
}>()

const dragOver = ref(false)
let _uid = 1

const list = ref<UploadFile[]>(
  (props.modelValue ?? []).map(f => ({ id: _uid++, file: f }))
)

function addFiles(raw: FileList) {
  const maxB = (props.maxSizeMb ?? 10) * 1024 * 1024
  const added: UploadFile[] = []
  for (let i = 0; i < raw.length; i++) {
    const f = raw[i]
    if (f.size > maxB) {
      emit('error', `"${f.name}" vượt quá ${props.maxSizeMb}MB`)
      continue
    }
    added.push({ id: _uid++, file: f })
  }
  list.value = props.multiple ? [...list.value, ...added] : added.slice(0, 1)
  emit('update:modelValue', list.value.map(u => u.file))
}

function removeFile(id: number) {
  list.value = list.value.filter(u => u.id !== id)
  emit('update:modelValue', list.value.map(u => u.file))
}

function onDragOver(e: DragEvent) {
  e.preventDefault()
  if (!props.disabled) dragOver.value = true
}
function onDragLeave() { dragOver.value = false }
function onDrop(e: DragEvent) {
  e.preventDefault()
  dragOver.value = false
  if (!props.disabled && e.dataTransfer?.files.length) addFiles(e.dataTransfer.files)
}
function onInput(e: Event) {
  const el = e.target as HTMLInputElement
  if (el.files?.length) addFiles(el.files)
  el.value = ''
}

function fmtSize(b: number) {
  if (b < 1024) return `${b} B`
  if (b < 1048576) return `${(b / 1024).toFixed(1)} KB`
  return `${(b / 1048576).toFixed(1)} MB`
}

function fileEmoji(mime: string) {
  if (mime.startsWith('image/')) return '🖼'
  if (mime === 'application/pdf') return '📄'
  if (/sheet|excel/.test(mime)) return '📊'
  if (/word|document/.test(mime)) return '📝'
  return '📎'
}
</script>

<template>
  <div class="wx-fu">
    <label
      class="wx-fu__zone"
      :class="{ 'wx-fu__zone--over': dragOver, 'wx-fu__zone--disabled': disabled }"
      @dragover.prevent="onDragOver"
      @dragleave="onDragLeave"
      @drop="onDrop"
    >
      <input
        class="wx-fu__input"
        type="file"
        :accept="accept"
        :multiple="multiple"
        :disabled="disabled"
        @change="onInput"
      />
      <slot>
        <div class="wx-fu__body">
          <svg class="wx-fu__svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/>
            <polyline points="17 8 12 3 7 8"/>
            <line x1="12" y1="3" x2="12" y2="15"/>
          </svg>
          <p class="wx-fu__hint">
            <span class="wx-fu__cta">Chọn tệp</span> hoặc kéo thả vào đây
          </p>
          <p class="wx-fu__meta">
            {{ accept ?? 'Tất cả định dạng' }} · tối đa {{ maxSizeMb }}MB
          </p>
        </div>
      </slot>
    </label>

    <TransitionGroup v-if="list.length" name="wx-fu-item" tag="ul" class="wx-fu__list">
      <li v-for="uf in list" :key="uf.id" class="wx-fu__item">
        <span class="wx-fu__item-icon" aria-hidden="true">{{ fileEmoji(uf.file.type) }}</span>
        <div class="wx-fu__item-info">
          <span class="wx-fu__item-name">{{ uf.file.name }}</span>
          <span class="wx-fu__item-size">{{ fmtSize(uf.file.size) }}</span>
        </div>
        <button
          type="button"
          class="wx-fu__item-rm"
          :disabled="disabled"
          aria-label="Xoá tệp"
          @click="removeFile(uf.id)"
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>
          </svg>
        </button>
      </li>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.wx-fu {
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-3);
}

/* ── Drop zone ── */
.wx-fu__zone {
  display: block;
  cursor: pointer;
  border: 2px dashed var(--wx-border-default);
  border-radius: var(--wx-radius-lg);
  background: var(--wx-surface-sunken);
  transition:
    border-color var(--wx-d-fast) var(--wx-ease-standard),
    background var(--wx-d-fast) var(--wx-ease-standard);
}
.wx-fu__zone:hover:not(.wx-fu__zone--disabled) {
  border-color: var(--wx-brand-primary);
  background: color-mix(in srgb, var(--wx-brand-primary) 4%, var(--wx-surface-sunken));
}
.wx-fu__zone--over {
  border-color: var(--wx-brand-primary);
  background: color-mix(in srgb, var(--wx-brand-primary) 8%, var(--wx-surface-sunken));
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--wx-brand-primary) 14%, transparent);
}
.wx-fu__zone--disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.wx-fu__input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
}

.wx-fu__body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--wx-space-2);
  padding: var(--wx-space-6) var(--wx-space-4);
  text-align: center;
}

.wx-fu__svg {
  width: 32px;
  height: 32px;
  color: var(--wx-text-muted);
  transition: color var(--wx-d-fast) var(--wx-ease-standard);
}
.wx-fu__zone--over .wx-fu__svg,
.wx-fu__zone:hover:not(.wx-fu__zone--disabled) .wx-fu__svg {
  color: var(--wx-brand-primary);
}

.wx-fu__hint {
  margin: 0;
  font-size: var(--wx-fs-14);
  color: var(--wx-text-secondary);
}
.wx-fu__cta {
  color: var(--wx-brand-primary);
  font-weight: var(--wx-fw-medium);
}

.wx-fu__meta {
  margin: 0;
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
}

/* ── File list ── */
.wx-fu__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-2);
}

.wx-fu__item {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  padding: var(--wx-space-2) var(--wx-space-3);
  background: var(--wx-surface-base);
  border: 1px solid var(--wx-border-subtle);
  border-radius: var(--wx-radius-md);
}

.wx-fu__item-icon { font-size: 18px; flex-shrink: 0; }

.wx-fu__item-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.wx-fu__item-name {
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-medium);
  color: var(--wx-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.wx-fu__item-size {
  font-size: var(--wx-fs-11);
  color: var(--wx-text-muted);
}

.wx-fu__item-rm {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  height: 24px;
  flex-shrink: 0;
  border: none;
  background: transparent;
  border-radius: var(--wx-radius-sm);
  cursor: pointer;
  color: var(--wx-text-muted);
  padding: 0;
  transition: background var(--wx-d-fast), color var(--wx-d-fast);
}
.wx-fu__item-rm:hover:not(:disabled) {
  background: var(--wx-danger-subtle);
  color: var(--wx-danger-text);
}
.wx-fu__item-rm svg { width: 14px; height: 14px; }
.wx-fu__item-rm:disabled { opacity: 0.4; cursor: not-allowed; }

/* ── List transition ── */
.wx-fu-item-enter-active,
.wx-fu-item-leave-active { transition: all var(--wx-d-fast) var(--wx-ease-standard); }
.wx-fu-item-enter-from  { opacity: 0; transform: translateY(-6px); }
.wx-fu-item-leave-to    { opacity: 0; transform: translateX(12px); }
.wx-fu-item-leave-active { position: absolute; }

@media (prefers-reduced-motion: reduce) {
  .wx-fu__zone,
  .wx-fu__svg,
  .wx-fu__item-rm,
  .wx-fu-item-enter-active,
  .wx-fu-item-leave-active { transition: none; }
}
</style>
