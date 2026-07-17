<script lang="ts">
let _idCounter = 0
</script>

<script setup lang="ts">
/**
 * BaseSelect — select của WemakeUI.
 * Ô đóng giữ dáng input (border, chevron) để khớp các form field khác,
 * nhưng danh sách option là popup custom theo design system (KHÔNG dùng
 * dropdown native của trình duyệt): Teleport, auto-flip, điều hướng bàn phím,
 * nhóm option, dấu chọn, dark mode.
 */
import { ref, computed, nextTick, onMounted, onBeforeUnmount } from 'vue'

type Option = { value: string | number; label: string; group?: string }

const props = withDefaults(defineProps<{
  modelValue?: string | number
  options: Option[]
  label?: string
  placeholder?: string
  disabled?: boolean
  size?: 'sm' | 'md'
}>(), { size: 'md' })

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
}>()

const _uid = ++_idCounter
const labelId   = `bsel-label-${_uid}`
const listboxId = `bsel-listbox-${_uid}`

const isOpen     = ref(false)
const wrapperRef = ref<HTMLElement>()
const menuRef    = ref<HTMLElement>()
const triggerRef = ref<HTMLButtonElement>()
const focusedIdx = ref(-1)
const menuStyle  = ref<Record<string, string>>({})

/* ── selected label ── */
const selectedLabel = computed(() =>
  props.options.find(o => o.value === props.modelValue)?.label ?? null,
)
const hasValue = computed(() => selectedLabel.value !== null)

/* ── group helpers ── */
const groupNames = computed(() =>
  [...new Set(props.options.filter(o => o.group).map(o => o.group!))],
)
const hasGroups = computed(() => groupNames.value.length > 0)
const ungrouped = computed(() => props.options.filter(o => !o.group))
function optsByGroup(g: string) { return props.options.filter(o => o.group === g) }

/* ── position (fixed, tracked from trigger rect, flip when needed) ── */
function computePos() {
  if (!triggerRef.value) return
  const r = triggerRef.value.getBoundingClientRect()
  const spaceBelow = window.innerHeight - r.bottom - 8
  const spaceAbove = r.top - 8
  const flipUp = spaceBelow < 160 && spaceAbove > spaceBelow

  menuStyle.value = flipUp
    ? { position: 'fixed', bottom: `${window.innerHeight - r.top + 6}px`, left: `${r.left}px`, minWidth: `${r.width}px` }
    : { position: 'fixed', top: `${r.bottom + 6}px`, left: `${r.left}px`, minWidth: `${r.width}px` }
}

/* ── open / close ── */
function open() {
  if (props.disabled) return
  computePos()
  isOpen.value = true
  const sel = props.options.findIndex(o => o.value === props.modelValue)
  focusedIdx.value = sel >= 0 ? sel : 0
  nextTick(scrollFocused)
}

function close(restoreFocus = true) {
  if (!isOpen.value) return
  isOpen.value = false
  focusedIdx.value = -1
  if (restoreFocus) triggerRef.value?.focus()
}

function toggle() { isOpen.value ? close() : open() }

function select(value: string | number) {
  emit('update:modelValue', value)
  close()
}

function scrollFocused() {
  if (!menuRef.value) return
  const items = menuRef.value.querySelectorAll<HTMLElement>('.base-select__option')
  items[focusedIdx.value]?.scrollIntoView({ block: 'nearest' })
}

/* ── keyboard ── */
function onKeydown(e: KeyboardEvent) {
  const n = props.options.length
  if (!isOpen.value) {
    if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown') {
      e.preventDefault(); open()
    }
    return
  }
  switch (e.key) {
    case 'Escape':
      e.preventDefault(); close(); break
    case 'ArrowDown':
      e.preventDefault()
      focusedIdx.value = Math.min(focusedIdx.value + 1, n - 1)
      nextTick(scrollFocused); break
    case 'ArrowUp':
      e.preventDefault()
      focusedIdx.value = Math.max(focusedIdx.value - 1, 0)
      nextTick(scrollFocused); break
    case 'Enter':
    case ' ':
      e.preventDefault()
      if (focusedIdx.value >= 0 && focusedIdx.value < n)
        select(props.options[focusedIdx.value].value)
      break
    case 'Tab':
      close(false); break
  }
}

/* ── close on outside click / reposition on scroll ── */
function onOutside(e: MouseEvent) {
  if (!isOpen.value) return
  if (!wrapperRef.value?.contains(e.target as Node) &&
      !menuRef.value?.contains(e.target as Node)) {
    close(false)
  }
}
function onScroll() { if (isOpen.value) computePos() }

onMounted(() => {
  document.addEventListener('mousedown', onOutside)
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', onScroll)
})
onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onOutside)
  window.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('resize', onScroll)
})
</script>

<template>
  <div
    ref="wrapperRef"
    class="base-select"
    :class="[`base-select--${size}`, { 'base-select--open': isOpen, 'base-select--disabled': disabled }]"
  >
    <label v-if="label" :id="labelId" class="base-select__label">{{ label }}</label>

    <!-- Trigger — dáng input -->
    <button
      ref="triggerRef"
      type="button"
      class="base-select__field"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="isOpen"
      :aria-labelledby="label ? labelId : undefined"
      :aria-controls="listboxId"
      @click="toggle"
      @keydown="onKeydown"
    >
      <span class="base-select__value" :class="{ 'base-select__value--placeholder': !hasValue }">
        {{ selectedLabel ?? placeholder ?? 'Chọn...' }}
      </span>
      <svg class="base-select__chevron" width="12" height="12" viewBox="0 0 24 24" fill="none"
           stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="6 9 12 15 18 9" />
      </svg>
    </button>

    <!-- Popup — Teleport ra body để không bị cắt bởi overflow -->
    <Teleport to="body">
      <Transition name="base-select-drop">
        <div
          v-if="isOpen"
          :id="listboxId"
          ref="menuRef"
          class="base-select__menu"
          role="listbox"
          :aria-labelledby="label ? labelId : undefined"
          :style="menuStyle"
        >
          <!-- Flat list -->
          <template v-if="!hasGroups">
            <button
              v-for="(opt, idx) in options"
              :key="opt.value"
              type="button"
              class="base-select__option"
              :class="{
                'base-select__option--selected': opt.value === modelValue,
                'base-select__option--focused':  idx === focusedIdx,
              }"
              role="option"
              :aria-selected="opt.value === modelValue"
              @click="select(opt.value)"
              @mouseenter="focusedIdx = idx"
            >
              <span class="base-select__opt-label">{{ opt.label }}</span>
              <svg v-if="opt.value === modelValue" class="base-select__check" width="14" height="14"
                   viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </button>
          </template>

          <!-- Grouped list -->
          <template v-else>
            <template v-for="group in groupNames" :key="group">
              <div class="base-select__group-label">{{ group }}</div>
              <button
                v-for="opt in optsByGroup(group)"
                :key="opt.value"
                type="button"
                class="base-select__option"
                :class="{
                  'base-select__option--selected': opt.value === modelValue,
                  'base-select__option--focused':  options.indexOf(opt) === focusedIdx,
                }"
                role="option"
                :aria-selected="opt.value === modelValue"
                @click="select(opt.value)"
                @mouseenter="focusedIdx = options.indexOf(opt)"
              >
                <span class="base-select__opt-label">{{ opt.label }}</span>
                <svg v-if="opt.value === modelValue" class="base-select__check" width="14" height="14"
                     viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              </button>
            </template>

            <template v-if="ungrouped.length">
              <div class="base-select__sep" />
              <button
                v-for="opt in ungrouped"
                :key="opt.value"
                type="button"
                class="base-select__option"
                :class="{
                  'base-select__option--selected': opt.value === modelValue,
                  'base-select__option--focused':  options.indexOf(opt) === focusedIdx,
                }"
                role="option"
                :aria-selected="opt.value === modelValue"
                @click="select(opt.value)"
                @mouseenter="focusedIdx = options.indexOf(opt)"
              >
                <span class="base-select__opt-label">{{ opt.label }}</span>
                <svg v-if="opt.value === modelValue" class="base-select__check" width="14" height="14"
                     viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              </button>
            </template>
          </template>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.base-select {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.base-select__label {
  font-size: var(--wx-fs-12);
  font-weight: var(--wx-fw-semibold);
  color: var(--wx-text-secondary);
}

/* ── Trigger — giữ dáng input giống form-input cũ ── */
.base-select__field {
  display: inline-flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  cursor: pointer;
  padding: 8px 12px;
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
  background-color: var(--wx-surface-elevated);
  color: var(--wx-text-primary);
  font-family: var(--wx-font-primary);
  font-size: var(--wx-fs-13);
  text-align: left;
  line-height: 1.4;
  transition:
    border-color var(--wx-d-fast) var(--wx-ease-standard),
    box-shadow   var(--wx-d-fast) var(--wx-ease-standard);
}

.base-select__field:focus-visible {
  outline: none;
  border-color: var(--wx-brand-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--wx-brand-primary) 18%, transparent);
}

.base-select--open .base-select__field {
  border-color: var(--wx-brand-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--wx-brand-primary) 18%, transparent);
}

.base-select--disabled .base-select__field {
  opacity: 0.55;
  cursor: not-allowed;
  pointer-events: none;
}

.base-select__value {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.base-select__value--placeholder {
  color: var(--wx-text-muted);
}

.base-select__chevron {
  flex-shrink: 0;
  color: var(--wx-text-secondary);
  transition: transform var(--wx-d-fast) var(--wx-ease-standard);
}

.base-select--open .base-select__chevron { transform: rotate(180deg); }

/* ── Size sm ── */
.base-select--sm .base-select__field {
  padding: 5px 10px;
  font-size: var(--wx-fs-12);
}
.base-select--sm .base-select__label {
  font-size: var(--wx-fs-11);
}

/* ── Drop transition ── */
.base-select-drop-enter-active { transition: opacity 0.14s ease, transform 0.14s ease; }
.base-select-drop-leave-active { transition: opacity 0.1s ease,  transform 0.1s ease; }
.base-select-drop-enter-from,
.base-select-drop-leave-to { opacity: 0; transform: translateY(-4px) scale(0.98); }

@media (prefers-reduced-motion: reduce) {
  .base-select__field,
  .base-select__chevron,
  .base-select-drop-enter-active,
  .base-select-drop-leave-active { transition: none; }
}
</style>

<!-- Popup Teleport ra body — KHÔNG scoped để style áp được vào nội dung đã teleport -->
<style>
.base-select__menu {
  /* popover band — nổi trên modal (1000), dưới toast (1200) */
  z-index: var(--wx-z-popover);
  background: var(--wx-surface-base);
  border: 1px solid var(--wx-border-subtle);
  border-radius: var(--wx-radius-xl);
  box-shadow:
    0 4px 6px -1px rgba(0, 0, 0, 0.08),
    0 10px 24px -4px rgba(0, 0, 0, 0.14);
  overflow-y: auto;
  max-height: 280px;
  padding: 4px;
}

.base-select__option {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  width: 100%;
  padding: 7px 10px;
  border: none;
  background: transparent;
  border-radius: var(--wx-radius-md);
  font-family: var(--wx-font-primary);
  font-size: var(--wx-fs-13);
  color: var(--wx-text-primary);
  cursor: pointer;
  text-align: left;
  transition: background 0.1s;
}

.base-select__option--focused,
.base-select__option:hover { background: var(--wx-hover-bg, var(--wx-surface-sunken)); }

.base-select__option--selected {
  color: var(--wx-brand-primary);
  font-weight: var(--wx-fw-medium);
}

.base-select__opt-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.base-select__check { color: var(--wx-brand-primary); flex-shrink: 0; }

.base-select__group-label {
  padding: 6px 10px 3px;
  font-size: var(--wx-fs-11);
  font-weight: var(--wx-fw-semibold);
  color: var(--wx-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.base-select__sep {
  height: 1px;
  background: var(--wx-border-subtle);
  margin: 4px 0;
}
</style>
