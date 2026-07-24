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

<style scoped src="./TagList.scoped.css"></style>
