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

<style scoped>
.wx-accordion {
  font-family: var(--wx-font-primary);
}
.wx-accordion[data-variant="bordered"] {
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-lg);
  overflow: hidden;
  background: var(--wx-bg-base);
}
.wx-accordion[data-variant="bordered"] .wx-accordion__item + .wx-accordion__item {
  border-top: 1px solid var(--wx-border-default);
}
.wx-accordion[data-variant="flush"] .wx-accordion__item + .wx-accordion__item {
  border-top: 1px solid var(--wx-border-subtle);
}

.wx-accordion__trigger {
  width: 100%;
  display: flex;
  align-items: center;
  gap: var(--wx-space-3);
  padding: var(--wx-space-3) var(--wx-space-4);
  background: transparent;
  border: none;
  text-align: left;
  cursor: pointer;
  color: var(--wx-content-primary);
  font-family: inherit;
  font-size: var(--wx-fs-14);
  transition: background var(--wx-d-micro) var(--wx-ease-standard);
}
.wx-accordion__trigger:hover { background: var(--wx-hover-bg); }
.wx-accordion__trigger:disabled { opacity: 0.5; cursor: not-allowed; }

.wx-accordion__icon {
  display: inline-flex;
  align-items: center;
  color: var(--wx-content-secondary);
  flex-shrink: 0;
}
.wx-accordion__head {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.wx-accordion__title {
  font-weight: var(--wx-fw-medium);
}
.wx-accordion__desc {
  font-size: var(--wx-fs-12);
  color: var(--wx-content-muted);
}
.wx-accordion__chevron {
  color: var(--wx-content-secondary);
  transition: transform var(--wx-d-fast) var(--wx-ease-standard);
  flex-shrink: 0;
}
.wx-accordion__item[data-open] .wx-accordion__chevron {
  transform: rotate(180deg);
}

.wx-accordion__panel {
  border-top: 1px solid var(--wx-border-subtle);
  background: var(--wx-bg-sunken);
}
.wx-accordion__body {
  padding: var(--wx-space-3) var(--wx-space-4);
  color: var(--wx-content-primary);
  font-size: var(--wx-fs-14);
  line-height: var(--wx-lh-normal);
}
</style>
