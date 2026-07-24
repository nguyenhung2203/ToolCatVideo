<script lang="ts">
let _idCounter = 0
</script>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount } from 'vue'

type Option = { value: string | number; label: string; group?: string }

const _uid = ++_idCounter
const labelId  = `bsm-label-${_uid}`
const listboxId = `bsm-listbox-${_uid}`

const props = withDefaults(defineProps<{
  modelValue?: string | number
  options: Option[]
  label?: string
  placeholder?: string
  disabled?: boolean
  size?: 'sm' | 'md'
  // When true, render a search input at the top of the popup that filters
  // options by label (case-insensitive substring match). Backward-compatible:
  // omitting this prop preserves the original "click + scroll" select UX.
  searchable?: boolean
  searchPlaceholder?: string
}>(), { size: 'md', searchable: false })

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
}>()

const isOpen     = ref(false)
const wrapperRef = ref<HTMLElement>()
const menuRef    = ref<HTMLElement>()
const triggerRef = ref<HTMLButtonElement>()
const searchInputRef = ref<HTMLInputElement>()
const focusedIdx = ref(-1)
const menuStyle  = ref<Record<string, string>>({})
const searchQuery = ref('')

/* ── selected label (always looks up against full options list) ── */
const selectedLabel = computed(() =>
  props.options.find(o => o.value === props.modelValue)?.label ?? null,
)
const hasValue = computed(() => selectedLabel.value !== null)

/* ── filtered options (search) ──
   When `searchable` is on, every list operation (rendering, keyboard
   navigation, group computation) uses the filtered set so focusedIdx and
   the displayed items stay in lockstep. */
const filteredOptions = computed(() => {
  if (!props.searchable) return props.options
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return props.options
  return props.options.filter(o =>
    o.label.toLowerCase().includes(q) ||
    String(o.value).toLowerCase().includes(q)
  )
})

/* ── group helpers ── */
const groupNames = computed(() =>
  [...new Set(filteredOptions.value.filter(o => o.group).map(o => o.group!))],
)
const hasGroups = computed(() => groupNames.value.length > 0)
const ungrouped = computed(() => filteredOptions.value.filter(o => !o.group))
function optsByGroup(g: string) { return filteredOptions.value.filter(o => o.group === g) }

/* ── position (fixed, tracked from trigger rect) ── */
function computePos() {
  if (!triggerRef.value) return
  const r = triggerRef.value.getBoundingClientRect()
  const spaceBelow = window.innerHeight - r.bottom - 8
  const spaceAbove = r.top - 8
  const flipUp = spaceBelow < 160 && spaceAbove > spaceBelow

  if (flipUp) {
    menuStyle.value = {
      position: 'fixed',
      bottom: `${window.innerHeight - r.top + 6}px`,
      left:   `${r.left}px`,
      minWidth: `${r.width}px`,
    }
  } else {
    menuStyle.value = {
      position: 'fixed',
      top:  `${r.bottom + 6}px`,
      left: `${r.left}px`,
      minWidth: `${r.width}px`,
    }
  }
}

/* ── open / close ── */
function open() {
  if (props.disabled) return
  computePos()
  isOpen.value = true
  const sel = filteredOptions.value.findIndex(o => o.value === props.modelValue)
  focusedIdx.value = sel >= 0 ? sel : 0
  nextTick(() => {
    // searchable: prioritise focusing the search input so the user can
    // immediately type to filter. Otherwise keep parity with old behaviour
    // (no autofocus inside the popup) and just scroll the focused option
    // into view.
    if (props.searchable) {
      searchInputRef.value?.focus()
    }
    scrollFocused()
  })
}

function close(restoreFocus = true) {
  if (!isOpen.value) return
  isOpen.value = false
  focusedIdx.value = -1
  searchQuery.value = ''
  if (restoreFocus) triggerRef.value?.focus()
}

function toggle() { isOpen.value ? close() : open() }

function select(value: string | number) {
  emit('update:modelValue', value)
  close()
}

function scrollFocused() {
  if (!menuRef.value) return
  const items = menuRef.value.querySelectorAll<HTMLElement>('.bsm__option')
  items[focusedIdx.value]?.scrollIntoView({ block: 'nearest' })
}

/* ── keyboard ── */
function onKeydown(e: KeyboardEvent) {
  const list = filteredOptions.value
  const n = list.length
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
      e.preventDefault()
      if (focusedIdx.value >= 0 && focusedIdx.value < n)
        select(list[focusedIdx.value].value)
      break
    case ' ':
      // In searchable mode, Space is a normal text character in the
      // search input — don't hijack it as "select focused option".
      if (!props.searchable) {
        e.preventDefault()
        if (focusedIdx.value >= 0 && focusedIdx.value < n)
          select(list[focusedIdx.value].value)
      }
      break
    case 'Tab':
      close(); break
  }
}

/* When searchQuery changes, snap focus back to the first match so Enter
   selects something predictable. */
function onSearchInput() {
  focusedIdx.value = filteredOptions.value.length > 0 ? 0 : -1
  nextTick(scrollFocused)
}

/* ── close on outside click / scroll ── */
function onOutside(e: MouseEvent) {
  if (!isOpen.value) return
  if (!wrapperRef.value?.contains(e.target as Node) &&
      !menuRef.value?.contains(e.target as Node)) {
    close(false)  // close without stealing focus back — user clicked elsewhere
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
    class="bsm"
    :class="[`bsm--${size}`, { 'bsm--open': isOpen, 'bsm--disabled': disabled }]"
  >
    <!-- Label -->
    <label v-if="label" :id="labelId" class="bsm__label">{{ label }}</label>

    <!-- Trigger -->
    <button
      ref="triggerRef"
      type="button"
      class="bsm__trigger"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="isOpen"
      :aria-labelledby="label ? labelId : undefined"
      :aria-controls="listboxId"
      @click="toggle"
      @keydown="onKeydown"
    >
      <span class="bsm__value" :class="{ 'bsm__value--placeholder': !hasValue }">
        {{ selectedLabel ?? placeholder ?? 'Chọn...' }}
      </span>
      <svg class="bsm__chevron" width="12" height="12" viewBox="0 0 24 24" fill="none"
           stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
        <polyline points="6 9 12 15 18 9" />
      </svg>
    </button>

    <!-- Menu — Teleported to body so it's never clipped -->
    <Teleport to="body">
      <Transition name="bsm-drop">
        <div
          v-if="isOpen"
          :id="listboxId"
          ref="menuRef"
          class="bsm__menu"
          role="listbox"
          :aria-labelledby="label ? labelId : undefined"
          :style="menuStyle"
        >
          <!-- Search input (searchable mode only) — sticky at top so it
               stays visible while scrolling a long list. -->
          <div v-if="searchable" class="bsm__search-wrap">
            <svg class="bsm__search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none"
                 stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8" />
              <path d="m21 21-4.3-4.3" />
            </svg>
            <input
              ref="searchInputRef"
              v-model="searchQuery"
              type="text"
              class="bsm__search-input"
              :placeholder="searchPlaceholder ?? 'Tìm kiếm…'"
              @input="onSearchInput"
              @keydown="onKeydown"
            />
          </div>

          <!-- Empty-state when nothing matches the query. -->
          <div v-if="searchable && filteredOptions.length === 0" class="bsm__empty">
            Không tìm thấy kết quả
          </div>

          <!-- Flat list -->
          <template v-if="!hasGroups">
            <button
              v-for="(opt, idx) in filteredOptions"
              :key="opt.value"
              type="button"
              class="bsm__option"
              :class="{
                'bsm__option--selected': opt.value === modelValue,
                'bsm__option--focused':  idx === focusedIdx,
              }"
              role="option"
              :aria-selected="opt.value === modelValue"
              @click="select(opt.value)"
              @mouseenter="focusedIdx = idx"
            >
              <span class="bsm__opt-label">{{ opt.label }}</span>
              <svg v-if="opt.value === modelValue" class="bsm__check" width="14" height="14"
                   viewBox="0 0 24 24" fill="none" stroke="currentColor"
                   stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </button>
          </template>

          <!-- Grouped list -->
          <template v-else>
            <template v-for="group in groupNames" :key="group">
              <div class="bsm__group-label">{{ group }}</div>
              <button
                v-for="opt in optsByGroup(group)"
                :key="opt.value"
                type="button"
                class="bsm__option"
                :class="{
                  'bsm__option--selected': opt.value === modelValue,
                  'bsm__option--focused':  filteredOptions.indexOf(opt) === focusedIdx,
                }"
                role="option"
                :aria-selected="opt.value === modelValue"
                @click="select(opt.value)"
                @mouseenter="focusedIdx = filteredOptions.indexOf(opt)"
              >
                <span class="bsm__opt-label">{{ opt.label }}</span>
                <svg v-if="opt.value === modelValue" class="bsm__check" width="14" height="14"
                     viewBox="0 0 24 24" fill="none" stroke="currentColor"
                     stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              </button>
            </template>

            <template v-if="ungrouped.length">
              <div class="bsm__sep" />
              <button
                v-for="opt in ungrouped"
                :key="opt.value"
                type="button"
                class="bsm__option"
                :class="{
                  'bsm__option--selected': opt.value === modelValue,
                  'bsm__option--focused':  filteredOptions.indexOf(opt) === focusedIdx,
                }"
                role="option"
                :aria-selected="opt.value === modelValue"
                @click="select(opt.value)"
                @mouseenter="focusedIdx = filteredOptions.indexOf(opt)"
              >
                <span class="bsm__opt-label">{{ opt.label }}</span>
                <svg v-if="opt.value === modelValue" class="bsm__check" width="14" height="14"
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

<style scoped src="./BaseSelectMenu.scoped.css"></style>

<!-- Teleported menu — global (not scoped) -->
<style src="./BaseSelectMenu.global.css"></style>
