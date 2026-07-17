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

<style scoped>
.wx-avatar {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--wx-bg-sunken);
  color: var(--wx-content-secondary);
  font-family: var(--wx-font-primary);
  font-weight: var(--wx-fw-semibold);
  overflow: hidden;
  flex-shrink: 0;
  user-select: none;
  border-radius: var(--wx-radius-full);
}
.wx-avatar[data-shape="square"] { border-radius: var(--wx-radius-md); }

.wx-avatar[data-size="xs"] { width: 20px; height: 20px; font-size: var(--wx-fs-12); }
.wx-avatar[data-size="sm"] { width: 24px; height: 24px; font-size: var(--wx-fs-12); }
.wx-avatar[data-size="md"] { width: 32px; height: 32px; font-size: var(--wx-fs-13); }
.wx-avatar[data-size="lg"] { width: 40px; height: 40px; font-size: var(--wx-fs-15); }
.wx-avatar[data-size="xl"] { width: 56px; height: 56px; font-size: var(--wx-fs-20); }

.wx-avatar[data-ring] {
  box-shadow: 0 0 0 2px var(--wx-bg-base), 0 0 0 4px var(--wx-brand-primary);
}

.wx-avatar__img { width: 100%; height: 100%; object-fit: cover; display: block; }

.wx-avatar__initial { letter-spacing: 0; }

.wx-avatar__icon {
  width: 60%;
  height: 60%;
  opacity: 0.6;
}

/* status dot — bottom-right */
.wx-avatar__status {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 30%;
  min-width: 8px;
  height: 30%;
  min-height: 8px;
  border-radius: var(--wx-radius-full);
  border: 2px solid var(--wx-bg-base);
  background: var(--wx-neutral-solid);
}
.wx-avatar__status[data-status="online"]  { background: var(--wx-success-solid); }
.wx-avatar__status[data-status="busy"]    { background: var(--wx-danger-solid); }
.wx-avatar__status[data-status="away"]    { background: var(--wx-warning-solid); }
.wx-avatar__status[data-status="offline"] { background: var(--wx-neutral-solid); }
</style>
