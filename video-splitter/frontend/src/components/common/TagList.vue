<script setup lang="ts">
import { computed } from 'vue'
import BaseTag from './BaseTag.vue'

const props = withDefaults(defineProps<{
  tags: string[]
  maxVisible?: number
  size?: 'sm' | 'md'
  variant?: 'neutral' | 'primary' | 'success' | 'warning' | 'danger' | 'info'
}>(), {
  maxVisible: 3,
  size: 'sm',
  variant: 'neutral',
})

const visible  = computed(() => props.tags.slice(0, props.maxVisible))
const overflow = computed(() => props.tags.slice(props.maxVisible))
const overflowLabel = computed(() => overflow.value.join(', '))
</script>

<template>
  <div class="tag-list">
    <BaseTag
      v-for="tag in visible"
      :key="tag"
      :label="tag"
      :size="size"
      :variant="variant"
    />
    <span
      v-if="overflow.length > 0"
      class="tag-list__more"
      :title="overflowLabel"
    >+{{ overflow.length }}</span>
  </div>
</template>

<style scoped>
.tag-list {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--wx-space-1);
}

.tag-list__more {
  display: inline-flex;
  align-items: center;
  padding: 2px 6px;
  font-size: var(--wx-fs-11);
  font-weight: var(--wx-fw-medium);
  color: var(--wx-text-muted);
  background: var(--wx-surface-sunken);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-full);
  cursor: default;
  white-space: nowrap;
  transition: background var(--wx-d-micro) var(--wx-ease-standard);
}

.tag-list__more:hover {
  background: var(--wx-surface-hover);
  color: var(--wx-text-secondary);
}
</style>
