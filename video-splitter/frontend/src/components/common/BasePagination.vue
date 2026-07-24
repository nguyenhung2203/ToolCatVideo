<script setup lang="ts">
/**
 * BasePagination — page navigation with siblings/ellipsis.
 *
 * Defaults: pageSize=20, siblings=1, variant='full', size='md'.
 *
 * variants:
 *  - full     : page numbers + prev/next + ellipsis
 *  - compact  : "x/y" + prev/next
 *  - load-more: 1 nút "tải thêm" cho infinite scroll
 */
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  modelValue: number
  total: number
  pageSize?: number
  siblings?: number
  variant?: 'full' | 'compact' | 'load-more'
  size?: 'sm' | 'md'
  loading?: boolean
}>(), {
  pageSize: 20,
  siblings: 1,
  variant: 'full',
  size: 'md',
  loading: false,
})

const emit = defineEmits<{
  'update:modelValue': [page: number]
  'load-more': []
}>()

const totalPages = computed(() =>
  Math.max(1, Math.ceil(props.total / props.pageSize)),
)

/** mảng các "trang" hiển thị; số = page, '…' = ellipsis */
const pageList = computed<(number | '…')[]>(() => {
  const total = totalPages.value
  const cur = props.modelValue
  const sib = props.siblings
  const pages = new Set<number>([1, total])
  for (let i = cur - sib; i <= cur + sib; i++) {
    if (i >= 1 && i <= total) pages.add(i)
  }
  const sorted = Array.from(pages).sort((a, b) => a - b)
  const out: (number | '…')[] = []
  for (let i = 0; i < sorted.length; i++) {
    if (i > 0 && sorted[i] - sorted[i - 1] > 1) out.push('…')
    out.push(sorted[i])
  }
  return out
})

function go(p: number) {
  if (p < 1 || p > totalPages.value || p === props.modelValue) return
  emit('update:modelValue', p)
}
function prev() { go(props.modelValue - 1) }
function next() { go(props.modelValue + 1) }
</script>

<template>
  <nav class="wx-pagination" :data-variant="variant" :data-size="size" aria-label="phân trang">
    <!-- ── load-more ── -->
    <button
      v-if="variant === 'load-more'"
      type="button"
      class="wx-pagination__loadmore"
      :disabled="loading || modelValue >= totalPages"
      @click="emit('load-more')"
    >
      <span v-if="loading">đang tải…</span>
      <span v-else-if="modelValue >= totalPages">đã hết</span>
      <span v-else>tải thêm</span>
    </button>

    <!-- ── compact ── -->
    <template v-else-if="variant === 'compact'">
      <button class="wx-pagination__btn" :disabled="modelValue <= 1" aria-label="trước" @click="prev">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
          <path d="M9 3 L5 7 L9 11" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
      <span class="wx-pagination__count">{{ modelValue }} / {{ totalPages }}</span>
      <button class="wx-pagination__btn" :disabled="modelValue >= totalPages" aria-label="sau" @click="next">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
          <path d="M5 3 L9 7 L5 11" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
    </template>

    <!-- ── full ── -->
    <template v-else>
      <button class="wx-pagination__btn" :disabled="modelValue <= 1" aria-label="trước" @click="prev">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
          <path d="M9 3 L5 7 L9 11" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
      <template v-for="(p, idx) in pageList" :key="idx">
        <span v-if="p === '…'" class="wx-pagination__ellipsis">…</span>
        <button
          v-else
          class="wx-pagination__btn"
          :data-active="p === modelValue || undefined"
          :aria-current="p === modelValue ? 'page' : undefined"
          :aria-label="`trang ${p}`"
          @click="go(p)"
        >{{ p }}</button>
      </template>
      <button class="wx-pagination__btn" :disabled="modelValue >= totalPages" aria-label="sau" @click="next">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none">
          <path d="M5 3 L9 7 L5 11" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
    </template>
  </nav>
</template>

<style scoped src="./BasePagination.scoped.css"></style>
