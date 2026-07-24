<script setup lang="ts">
/**
 * BaseAvatar — circle avatar with image / initial / icon fallback,
 * status dot, optional ring, 5 sizes.
 *
 * Defaults: size='md' (32px), shape='circle', status=null.
 */
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  src?: string | null
  name?: string
  size?: 'xs' | 'sm' | 'md' | 'lg' | 'xl'
  shape?: 'circle' | 'square'
  status?: 'online' | 'offline' | 'busy' | 'away' | null
  ring?: boolean
  alt?: string
}>(), {
  src: null,
  name: '',
  size: 'md',
  shape: 'circle',
  status: null,
  ring: false,
  alt: '',
})

const initials = computed(() => {
  if (!props.name) return ''
  const parts = props.name.trim().split(/\s+/)
  if (parts.length === 1) return parts[0].charAt(0).toUpperCase()
  return (parts[0].charAt(0) + parts[parts.length - 1].charAt(0)).toUpperCase()
})

const showImg = computed(() => !!props.src)
</script>

<template>
  <span
    class="wx-avatar"
    :data-size="size"
    :data-shape="shape"
    :data-ring="ring || undefined"
    role="img"
    :aria-label="alt || name || 'avatar'"
  >
    <img v-if="showImg" :src="src!" :alt="alt || name" class="wx-avatar__img" />
    <span v-else-if="initials" class="wx-avatar__initial">{{ initials }}</span>
    <svg v-else class="wx-avatar__icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6">
      <circle cx="12" cy="8" r="4" />
      <path d="M4 21v-1a8 8 0 0116 0v1" />
    </svg>
    <span v-if="status" class="wx-avatar__status" :data-status="status" />
  </span>
</template>

<style scoped src="./BaseAvatar.scoped.css"></style>
