<script setup lang="ts">
/**
 * BaseDrawer — slide-in panel from any edge.
 *
 * Defaults: placement='right', size='md' (400px), closable=true,
 *           closeOnBackdrop=true, closeOnEsc=true, radius='xl' (16px).
 */
import { watch, onBeforeUnmount, ref, nextTick } from 'vue'

const props = withDefaults(defineProps<{
  show: boolean
  title?: string
  placement?: 'left' | 'right' | 'top' | 'bottom'
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'full'
  closable?: boolean
  closeOnBackdrop?: boolean
  closeOnEsc?: boolean
}>(), {
  placement: 'right',
  size: 'md',
  closable: true,
  closeOnBackdrop: true,
  closeOnEsc: true,
})

const emit = defineEmits<{
  'update:show': [value: boolean]
  close: []
}>()

const panelRef = ref<HTMLElement>()
let lastFocused: HTMLElement | null = null

function close() {
  emit('update:show', false)
  emit('close')
}
function onBackdrop() { if (props.closeOnBackdrop) close() }

function onKey(e: KeyboardEvent) {
  if (!props.show) return
  if (e.key === 'Escape' && props.closeOnEsc) {
    e.stopPropagation()
    close()
  }
  // simple focus trap
  if (e.key === 'Tab' && panelRef.value) {
    const focusables = panelRef.value.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    )
    if (focusables.length === 0) return
    const first = focusables[0]
    const last = focusables[focusables.length - 1]
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault(); last.focus()
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault(); first.focus()
    }
  }
}

watch(() => props.show, async (v) => {
  if (v) {
    lastFocused = document.activeElement as HTMLElement
    document.addEventListener('keydown', onKey)
    document.body.style.overflow = 'hidden'
    await nextTick()
    panelRef.value?.focus()
  } else {
    document.removeEventListener('keydown', onKey)
    document.body.style.overflow = ''
    lastFocused?.focus?.()
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onKey)
  document.body.style.overflow = ''
})
</script>

<template>
  <teleport to="body">
    <transition name="wx-drawer-bd">
      <div
        v-if="show"
        class="wx-drawer-backdrop"
        @click="onBackdrop"
        aria-hidden="true"
      />
    </transition>
    <transition :name="`wx-drawer-${placement}`">
      <aside
        v-if="show"
        ref="panelRef"
        class="wx-drawer"
        :data-placement="placement"
        :data-size="size"
        role="dialog"
        aria-modal="true"
        :aria-label="title"
        tabindex="-1"
      >
        <header v-if="title || closable" class="wx-drawer__header">
          <h2 v-if="title" class="wx-drawer__title">{{ title }}</h2>
          <button
            v-if="closable"
            class="wx-drawer__close"
            type="button"
            aria-label="đóng"
            @click="close"
          >
            <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
              <path d="M3 3 L13 13 M13 3 L3 13" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" />
            </svg>
          </button>
        </header>

        <div class="wx-drawer__body">
          <slot />
        </div>

        <footer v-if="$slots.footer" class="wx-drawer__footer">
          <slot name="footer" />
        </footer>
      </aside>
    </transition>
  </teleport>
</template>

<style scoped src="./BaseDrawer.scoped.css"></style>
