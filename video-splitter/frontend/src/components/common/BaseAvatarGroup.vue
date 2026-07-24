<script setup lang="ts">
/**
 * BaseAvatarGroup — stack avatars with overlap, +N overflow indicator.
 * Defaults: size='md', max=4.
 */
import { computed } from 'vue'
import BaseAvatar from './BaseAvatar.vue'

interface AvatarItem {
  src?: string | null
  name?: string
}

const props = withDefaults(defineProps<{
  items?: AvatarItem[]
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  max?: number
}>(), {
  items: () => [],
  size: 'md',
  max: 4,
})

const visible = computed(() => props.items.slice(0, props.max))
const overflow = computed(() => Math.max(0, props.items.length - props.max))
</script>

<template>
  <span class="wx-avatar-group" :data-size="size">
    <template v-if="items.length">
      <BaseAvatar
        v-for="(it, i) in visible"
        :key="i"
        :src="it.src"
        :name="it.name"
        :size="size"
        ring
      />
      <span v-if="overflow > 0" class="wx-avatar-group__more" :data-size="size">
        +{{ overflow }}
      </span>
    </template>
    <slot v-else />
  </span>
</template>

<style scoped src="./BaseAvatarGroup.scoped.css"></style>
