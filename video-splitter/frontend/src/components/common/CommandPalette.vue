<script setup lang="ts">
/**
 * CommandPalette — cmd+k modal with search + grouped results.
 *
 * Defaults: shortcut='mod+k' (cmd on mac, ctrl on win/linux),
 *           placeholder='tìm lệnh hoặc trang…', maxResults=8.
 *
 * Items (shape):
 *   { id, label, group?, icon?, shortcut?, action: () => void }
 *
 * Phím: ↑↓ điều hướng, enter chọn, esc đóng, type-ahead fuzzy (substring).
 */
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'

export interface PaletteItem {
  id: string
  label: string
  group?: string
  icon?: string
  shortcut?: string
  description?: string
  action: () => void | Promise<void>
}

const props = withDefaults(defineProps<{
  show?: boolean
  items: PaletteItem[]
  placeholder?: string
  maxResults?: number
  /** auto bind cmd+k to open. set false nếu muốn quản lý ngoài. */
  autoShortcut?: boolean
}>(), {
  show: false,
  placeholder: 'tìm lệnh hoặc trang…',
  maxResults: 8,
  autoShortcut: true,
})

const emit = defineEmits<{
  'update:show': [v: boolean]
  select: [item: PaletteItem]
}>()

const open = ref(props.show)
const query = ref('')
const cursor = ref(0)
const inputRef = ref<HTMLInputElement>()

watch(() => props.show, (v) => { open.value = v })
watch(open, (v) => emit('update:show', v))

function fuzzy(label: string, q: string): boolean {
  if (!q) return true
  const lc = label.toLowerCase()
  const qc = q.toLowerCase()
  // substring OR all letters in order
  if (lc.includes(qc)) return true
  let qi = 0
  for (const ch of lc) {
    if (ch === qc[qi]) qi++
    if (qi === qc.length) return true
  }
  return false
}

const filtered = computed(() => {
  const q = query.value.trim()
  return props.items.filter((it) => fuzzy(it.label, q)).slice(0, props.maxResults)
})

const grouped = computed(() => {
  const map = new Map<string, PaletteItem[]>()
  for (const it of filtered.value) {
    const g = it.group ?? 'tổng quát'
    if (!map.has(g)) map.set(g, [])
    map.get(g)!.push(it)
  }
  return Array.from(map.entries())
})

const flat = computed(() => filtered.value)

watch(query, () => { cursor.value = 0 })
watch(filtered, () => { cursor.value = Math.min(cursor.value, Math.max(0, flat.value.length - 1)) })

function show() { open.value = true; nextTick(() => inputRef.value?.focus()) }
function hide() { open.value = false; query.value = ''; cursor.value = 0 }
function pick(it: PaletteItem) { emit('select', it); it.action(); hide() }

function onKey(e: KeyboardEvent) {
  // toggle bằng cmd/ctrl + k
  if (props.autoShortcut && (e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    open.value ? hide() : show()
    return
  }
  if (!open.value) return
  if (e.key === 'Escape') { e.stopPropagation(); hide(); return }
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    cursor.value = (cursor.value + 1) % Math.max(1, flat.value.length)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    cursor.value = (cursor.value - 1 + flat.value.length) % Math.max(1, flat.value.length)
  } else if (e.key === 'Enter') {
    e.preventDefault()
    const it = flat.value[cursor.value]
    if (it) pick(it)
  }
}

onMounted(() => document.addEventListener('keydown', onKey))
onBeforeUnmount(() => document.removeEventListener('keydown', onKey))

defineExpose({ show, hide })
</script>

<template>
  <teleport to="body">
    <transition name="wx-cmdk-bd">
      <div
        v-if="open"
        class="wx-cmdk-backdrop"
        @click="hide"
        aria-hidden="true"
      />
    </transition>
    <transition name="wx-cmdk">
      <div
        v-if="open"
        class="wx-cmdk"
        role="dialog"
        aria-modal="true"
        aria-label="bảng lệnh"
      >
        <div class="wx-cmdk__head">
          <svg class="wx-cmdk__search" width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <circle cx="7" cy="7" r="5" stroke="currentColor" stroke-width="1.6" />
            <path d="M11 11 L14 14" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
          </svg>
          <input
            ref="inputRef"
            v-model="query"
            type="text"
            class="wx-cmdk__input"
            :placeholder="placeholder"
            aria-label="tìm kiếm"
            autocomplete="off"
            spellcheck="false"
          />
          <kbd class="wx-cmdk__kbd">esc</kbd>
        </div>

        <div class="wx-cmdk__list" role="listbox">
          <template v-if="flat.length === 0">
            <div class="wx-cmdk__empty">không có kết quả phù hợp</div>
          </template>
          <template v-else>
            <div v-for="[group, items] in grouped" :key="group" class="wx-cmdk__group">
              <div class="wx-cmdk__group-title">{{ group }}</div>
              <button
                v-for="it in items"
                :key="it.id"
                type="button"
                class="wx-cmdk__item"
                :data-active="flat[cursor]?.id === it.id || undefined"
                role="option"
                :aria-selected="flat[cursor]?.id === it.id"
                @click="pick(it)"
                @mouseenter="cursor = flat.findIndex((x) => x.id === it.id)"
              >
                <span v-if="it.icon" class="wx-cmdk__icon" v-html="it.icon" />
                <span class="wx-cmdk__label">
                  <span>{{ it.label }}</span>
                  <span v-if="it.description" class="wx-cmdk__desc">{{ it.description }}</span>
                </span>
                <kbd v-if="it.shortcut" class="wx-cmdk__kbd">{{ it.shortcut }}</kbd>
              </button>
            </div>
          </template>
        </div>

        <div class="wx-cmdk__foot">
          <span><kbd class="wx-cmdk__kbd">↑</kbd><kbd class="wx-cmdk__kbd">↓</kbd> chọn</span>
          <span><kbd class="wx-cmdk__kbd">↵</kbd> mở</span>
          <span><kbd class="wx-cmdk__kbd">esc</kbd> đóng</span>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<style scoped src="./CommandPalette.scoped.css"></style>
