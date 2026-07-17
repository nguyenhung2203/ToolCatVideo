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

<style scoped>
.wx-avatar-group {
  display: inline-flex;
  align-items: center;
}
.wx-avatar-group > :deep(.wx-avatar) { margin-left: -8px; }
.wx-avatar-group > :deep(.wx-avatar:first-child) { margin-left: 0; }

.wx-avatar-group[data-size="xs"] > :deep(.wx-avatar) { margin-left: -6px; }
.wx-avatar-group[data-size="sm"] > :deep(.wx-avatar) { margin-left: -6px; }
.wx-avatar-group[data-size="lg"] > :deep(.wx-avatar) { margin-left: -10px; }
.wx-avatar-group[data-size="xl"] > :deep(.wx-avatar) { margin-left: -14px; }

.wx-avatar-group__more {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  margin-left: -8px;
  background: var(--wx-bg-sunken);
  color: var(--wx-content-secondary);
  font-family: var(--wx-font-primary);
  font-weight: var(--wx-fw-semibold);
  border-radius: var(--wx-radius-full);
  box-shadow: 0 0 0 2px var(--wx-bg-base);
  user-select: none;
}
.wx-avatar-group__more[data-size="xs"] { width: 20px; height: 20px; font-size: 10px; margin-left: -6px; }
.wx-avatar-group__more[data-size="sm"] { width: 24px; height: 24px; font-size: 10px; margin-left: -6px; }
.wx-avatar-group__more[data-size="md"] { width: 32px; height: 32px; font-size: var(--wx-fs-12); }
.wx-avatar-group__more[data-size="lg"] { width: 40px; height: 40px; font-size: var(--wx-fs-13); margin-left: -10px; }
.wx-avatar-group__more[data-size="xl"] { width: 56px; height: 56px; font-size: var(--wx-fs-16); margin-left: -14px; }
</style>
