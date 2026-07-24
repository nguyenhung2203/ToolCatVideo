<script setup lang="ts">
/**
 * BaseAccordion — collapsible sections, single or multi-open.
 *
 * Defaults: multi=false, defaultOpen=[], variant='bordered', radius='lg' (12px).
 */
import { ref, watch } from 'vue'

interface AccordionItem {
  key: string
  title: string
  description?: string
  icon?: string
  disabled?: boolean
}

const props = withDefaults(defineProps<{
  items: AccordionItem[]
  multi?: boolean
  defaultOpen?: string[]
  variant?: 'bordered' | 'flush'
}>(), {
  multi: false,
  defaultOpen: () => [],
  variant: 'bordered',
})

const open = ref<Set<string>>(new Set(props.defaultOpen ?? []))

watch(() => props.defaultOpen, (v) => { open.value = new Set(v ?? []) })

function isOpen(k: string) { return open.value.has(k) }

function toggle(k: string, disabled?: boolean) {
  if (disabled) return
  const next = new Set(open.value)
  if (next.has(k)) {
    next.delete(k)
  } else {
    if (!props.multi) next.clear()
    next.add(k)
  }
  open.value = next
}
</script>

<template>
  <div class="wx-accordion" :data-variant="variant">
    <div
      v-for="it in items"
      :key="it.key"
      class="wx-accordion__item"
      :data-open="isOpen(it.key) || undefined"
      :data-disabled="it.disabled || undefined"
    >
      <button
        type="button"
        class="wx-accordion__trigger"
        :aria-expanded="isOpen(it.key)"
        :aria-controls="`acc-panel-${it.key}`"
        :disabled="it.disabled"
        @click="toggle(it.key, it.disabled)"
      >
        <span v-if="it.icon" class="wx-accordion__icon" v-html="it.icon" />
        <span class="wx-accordion__head">
          <span class="wx-accordion__title">{{ it.title }}</span>
          <span v-if="it.description" class="wx-accordion__desc">{{ it.description }}</span>
        </span>
        <svg
          class="wx-accordion__chevron"
          width="16" height="16" viewBox="0 0 16 16" fill="none"
          aria-hidden="true"
        >
          <path d="M4 6 L8 10 L12 6" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
      <div
        v-show="isOpen(it.key)"
        :id="`acc-panel-${it.key}`"
        class="wx-accordion__panel"
        role="region"
      >
        <div class="wx-accordion__body">
          <slot :name="it.key" :item="it">
            <slot name="default" :item="it" />
          </slot>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped src="./BaseAccordion.scoped.css"></style>
