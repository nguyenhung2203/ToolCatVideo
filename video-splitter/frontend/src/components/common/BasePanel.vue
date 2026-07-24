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

<style scoped src="./BasePanel.scoped.css"></style>
