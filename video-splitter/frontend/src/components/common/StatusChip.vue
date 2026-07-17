<script setup lang="ts">
import BaseBadge from './BaseBadge.vue'

export interface StatusChipMap {
  [key: string]: {
    variant: 'success' | 'warning' | 'danger' | 'info' | 'neutral' | 'primary'
    label: string
    dot?: boolean
  }
}

const props = withDefaults(defineProps<{
  status: string
  map: StatusChipMap
  size?: 'sm' | 'md'
  dot?: boolean
}>(), {
  size: 'sm',
  dot: true,
})

const entry = () => props.map[props.status] ?? { variant: 'neutral' as const, label: props.status }
</script>

<template>
  <BaseBadge
    :variant="entry().variant"
    :text="entry().label"
    :size="size"
    :dot="dot ?? entry().dot ?? false"
  />
</template>
