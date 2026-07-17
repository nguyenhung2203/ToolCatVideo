<script setup lang="ts">
/**
 * BasePanel — flat sectioning container.
 * Khác Card: không shadow, viền nhẹ hơn, dùng cho grouping bên trong shell
 * (settings section, sidebar block, side panel).
 */
import { computed, ref, watch } from 'vue'

const props = withDefaults(defineProps<{
  /** tiêu đề sentence case */
  title?: string
  /** mô tả nhỏ */
  description?: string
  /** padding */
  padded?: boolean
  /** radius */
  radius?: 'md' | 'lg' | 'xl'
  /** tone của panel */
  tone?: 'default' | 'sunken' | 'subtle'
  /** collapsible toggle */
  collapsible?: boolean
  /** initial collapsed (uncontrolled) */
  defaultCollapsed?: boolean
  /** controlled collapsed (v-model:collapsed) */
  collapsed?: boolean
}>(), {
  padded: true,
  radius: 'lg',
  tone: 'default',
  collapsible: false,
  defaultCollapsed: false,
})

const emit = defineEmits<{
  'update:collapsed': [value: boolean]
}>()

const internalCollapsed = ref(props.defaultCollapsed)

const isCollapsed = computed<boolean>({
  get: () => props.collapsed !== undefined ? props.collapsed : internalCollapsed.value,
  set: (v) => {
    if (props.collapsed !== undefined) emit('update:collapsed', v)
    else {
      internalCollapsed.value = v
      emit('update:collapsed', v)
    }
  },
})

watch(() => props.defaultCollapsed, (v) => {
  if (props.collapsed === undefined) internalCollapsed.value = v
})

function toggle() {
  if (!props.collapsible) return
  isCollapsed.value = !isCollapsed.value
}
</script>

<template>
  <section
    class="wx-panel"
    :data-tone="tone"
    :data-radius="radius"
    :class="{ 'wx-panel--padded': padded, 'wx-panel--collapsed': isCollapsed }"
  >
    <header
      v-if="title || description || $slots.header || $slots.actions || collapsible"
      class="wx-panel__header"
      :class="{ 'wx-panel__header--clickable': collapsible }"
      data-part="header"
      @click="toggle"
    >
      <button
        v-if="collapsible"
        type="button"
        class="wx-panel__chevron"
        :aria-expanded="!isCollapsed"
        :aria-label="isCollapsed ? 'Mở rộng' : 'Thu gọn'"
        tabindex="-1"
      >
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor"
             stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="6 9 12 15 18 9" />
        </svg>
      </button>
      <div class="wx-panel__head-text">
        <slot name="header">
          <h3 v-if="title" class="wx-panel__title">{{ title }}</h3>
          <p v-if="description" class="wx-panel__desc">{{ description }}</p>
        </slot>
      </div>
      <div v-if="$slots.actions" class="wx-panel__actions" data-part="actions" @click.stop>
        <slot name="actions" />
      </div>
    </header>

    <transition name="wx-panel-collapse">
      <div v-show="!isCollapsed" class="wx-panel__body" data-part="body">
        <slot />
      </div>
    </transition>

    <footer v-if="$slots.footer" class="wx-panel__footer" data-part="footer">
      <slot name="footer" />
    </footer>
  </section>
</template>

<style scoped>
.wx-panel {
  display: flex;
  flex-direction: column;
  font-family: var(--wx-font-primary);
  color: var(--wx-content-primary);
  border: 1px solid var(--wx-border-subtle);
  background: var(--wx-surface-base);
  overflow: hidden;
}
.wx-panel[data-tone="sunken"]  { background: var(--wx-surface-sunken); }
.wx-panel[data-tone="subtle"]  { background: transparent; border: none; }

.wx-panel[data-radius="md"] { border-radius: var(--wx-radius-md); }
.wx-panel[data-radius="lg"] { border-radius: var(--wx-radius-lg); }
.wx-panel[data-radius="xl"] { border-radius: var(--wx-radius-xl); }

.wx-panel__header {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  padding: var(--wx-space-3) var(--wx-space-4);
  border-bottom: 1px solid var(--wx-border-subtle);
}
.wx-panel--collapsed > .wx-panel__header { border-bottom-color: transparent; }

.wx-panel__header--clickable {
  cursor: pointer;
  user-select: none;
  transition: background var(--wx-d-fast) var(--wx-ease-standard);
}
.wx-panel__header--clickable:hover {
  background: var(--wx-hover-bg, rgba(0, 0, 0, 0.02));
}

.wx-panel__chevron {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px; height: 20px;
  border: none;
  background: transparent;
  color: var(--wx-content-muted);
  cursor: pointer;
  border-radius: var(--wx-radius-sm);
  transition: transform var(--wx-d-fast) var(--wx-ease-standard);
}
.wx-panel--collapsed .wx-panel__chevron { transform: rotate(-90deg); }

.wx-panel__head-text {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.wx-panel__title {
  margin: 0;
  font-size: var(--wx-fs-14);
  font-weight: var(--wx-fw-semibold);
  color: var(--wx-content-primary);
}
.wx-panel__desc {
  margin: 0;
  font-size: var(--wx-fs-12);
  color: var(--wx-content-muted);
}
.wx-panel__actions {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
}

.wx-panel--padded > .wx-panel__body { padding: var(--wx-space-4); }

.wx-panel__footer {
  padding: var(--wx-space-3) var(--wx-space-4);
  border-top: 1px solid var(--wx-border-subtle);
  background: var(--wx-surface-sunken);
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wx-space-2);
}

/* collapse transition */
.wx-panel-collapse-enter-active,
.wx-panel-collapse-leave-active {
  transition: max-height var(--wx-d-normal) var(--wx-ease-standard),
              opacity var(--wx-d-fast) var(--wx-ease-standard);
  overflow: hidden;
  max-height: 800px;
}
.wx-panel-collapse-enter-from,
.wx-panel-collapse-leave-to {
  max-height: 0;
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .wx-panel-collapse-enter-active,
  .wx-panel-collapse-leave-active { transition: none; }
}
</style>
