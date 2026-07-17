<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  title: string
  fill?: boolean
  collapsible?: boolean
  collapsed?: boolean
}>(), {
  fill: false,
  collapsible: false,
  collapsed: false,
})

const emit = defineEmits<{
  'update:collapsed': [value: boolean]
}>()

const isCollapsed = computed(() => props.collapsed)

function toggle() {
  if (props.collapsible) {
    emit('update:collapsed', !isCollapsed.value)
  }
}
</script>

<template>
  <div class="gbox" :class="{ 'gbox--fill': props.fill, 'gbox--collapsed': isCollapsed }">
    <div
      class="gbox-header"
      :class="{ 'gbox-header--clickable': props.collapsible }"
      @click="toggle"
    >
      <span v-if="props.collapsible" class="gbox-chevron" :class="{ 'gbox-chevron--collapsed': isCollapsed }">▸</span>
      {{ props.title }}
    </div>
    <transition name="gbox-collapse">
      <div v-show="!isCollapsed" class="gbox-body">
        <slot />
      </div>
    </transition>
  </div>
</template>

<style scoped>
.gbox {
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-lg);
  overflow: hidden;
  background: var(--wx-surface-base);
}
.gbox--fill {
  flex: 1;
  display: flex;
  flex-direction: column;
}
.gbox-header {
  padding: var(--wx-space-2) var(--wx-space-4);
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-semibold);
  color: var(--wx-content-primary);
  letter-spacing: var(--wx-tracking-normal);
  background: var(--wx-surface-sunken);
  border-bottom: 1px solid var(--wx-border-subtle);
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
}
.gbox-header--clickable {
  cursor: pointer;
  user-select: none;
  transition: background var(--wx-duration-fast);
}
.gbox-header--clickable:hover {
  background: var(--wx-hover-bg);
}
.gbox--collapsed .gbox-header {
  border-bottom: none;
}
.gbox-chevron {
  display: inline-block;
  font-size: 11px;
  transition: transform 0.2s var(--wx-easing-default);
  transform: rotate(90deg);
}
.gbox-chevron--collapsed {
  transform: rotate(0deg);
}
.gbox-body {
  padding: 10px 14px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.gbox--fill .gbox-body {
  flex: 1;
  overflow-y: auto;
}

/* Collapse animation */
.gbox-collapse-enter-active,
.gbox-collapse-leave-active {
  transition: all 0.2s var(--wx-easing-default);
  overflow: hidden;
}
.gbox-collapse-enter-from,
.gbox-collapse-leave-to {
  opacity: 0;
  max-height: 0;
  padding-top: 0;
  padding-bottom: 0;
}
</style>
