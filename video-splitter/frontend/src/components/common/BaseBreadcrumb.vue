<script setup lang="ts">
/**
 * BaseBreadcrumb — navigation trail.
 *
 * Defaults: separator='chevron', size='md'.
 */
interface BreadcrumbItem {
  label: string
  to?: string
  href?: string
  icon?: string
}

withDefaults(defineProps<{
  items: BreadcrumbItem[]
  separator?: 'chevron' | 'slash' | 'dot'
  size?: 'sm' | 'md'
}>(), {
  separator: 'chevron',
  size: 'md',
})

const emit = defineEmits<{
  'item-click': [item: BreadcrumbItem, index: number, event: MouseEvent]
}>()

function onItemClick(it: BreadcrumbItem, idx: number, e: MouseEvent) {
  emit('item-click', it, idx, e)
}
</script>

<template>
  <nav class="wx-breadcrumb" :data-size="size" aria-label="đường dẫn">
    <ol class="wx-breadcrumb__list">
      <li
        v-for="(it, idx) in items"
        :key="idx"
        class="wx-breadcrumb__item"
        :data-current="idx === items.length - 1 || undefined"
      >
        <component
          :is="it.to || it.href ? 'a' : 'span'"
          class="wx-breadcrumb__link"
          :href="it.href || it.to"
          :aria-current="idx === items.length - 1 ? 'page' : undefined"
          @click="onItemClick(it, idx, $event)"
        >
          <span v-if="it.icon" class="wx-breadcrumb__icon" v-html="it.icon" />
          <span>{{ it.label }}</span>
        </component>
        <span
          v-if="idx < items.length - 1"
          class="wx-breadcrumb__sep"
          :data-sep="separator"
          aria-hidden="true"
        >
          <svg v-if="separator === 'chevron'" width="12" height="12" viewBox="0 0 12 12" fill="none">
            <path d="M4.5 3 L7.5 6 L4.5 9" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" />
          </svg>
          <span v-else-if="separator === 'slash'">/</span>
          <span v-else>·</span>
        </span>
      </li>
    </ol>
  </nav>
</template>

<style scoped>
.wx-breadcrumb {
  font-family: var(--wx-font-primary);
}
.wx-breadcrumb__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--wx-space-1);
}
.wx-breadcrumb__item {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
}

.wx-breadcrumb[data-size="sm"] { font-size: var(--wx-fs-12); }
.wx-breadcrumb[data-size="md"] { font-size: var(--wx-fs-13); }

.wx-breadcrumb__link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--wx-content-secondary);
  text-decoration: none;
  padding: 2px 6px;
  border-radius: var(--wx-radius-sm);
  transition: background var(--wx-d-micro) var(--wx-ease-standard),
              color var(--wx-d-micro) var(--wx-ease-standard);
}
a.wx-breadcrumb__link {
  cursor: pointer;
}
a.wx-breadcrumb__link:hover {
  color: var(--wx-content-link);
  background: var(--wx-hover-bg);
}

.wx-breadcrumb__item[data-current] .wx-breadcrumb__link {
  color: var(--wx-content-primary);
  font-weight: var(--wx-fw-medium);
  cursor: default;
}

.wx-breadcrumb__icon {
  display: inline-flex;
  align-items: center;
}

.wx-breadcrumb__sep {
  display: inline-flex;
  align-items: center;
  color: var(--wx-content-muted);
}
.wx-breadcrumb__sep[data-sep="slash"],
.wx-breadcrumb__sep[data-sep="dot"] {
  padding: 0 2px;
}
</style>
