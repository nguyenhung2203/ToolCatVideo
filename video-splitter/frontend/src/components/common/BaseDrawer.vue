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

<style scoped>
.wx-drawer-backdrop {
  position: fixed;
  inset: 0;
  background: var(--wx-bg-overlay);
  z-index: var(--wx-z-overlay);
  backdrop-filter: blur(2px);
}

.wx-drawer {
  position: fixed;
  background: var(--wx-bg-base);
  color: var(--wx-content-primary);
  border: 1px solid rgba(59, 130, 246, 0.25);
  box-shadow:
    0 20px 50px rgba(37, 99, 235, 0.25),
    0 8px 24px rgba(37, 99, 235, 0.15);
  z-index: calc(var(--wx-z-overlay) + 1);
  display: flex;
  flex-direction: column;
  outline: none;
  font-family: var(--wx-font-primary);
}

/* placement + size */
.wx-drawer[data-placement="right"], .wx-drawer[data-placement="left"] {
  top: 0; bottom: 0; height: 100vh;
  border-radius: 0;
}
.wx-drawer[data-placement="right"] { right: 0; border-top-left-radius: var(--wx-radius-xl); border-bottom-left-radius: var(--wx-radius-xl); }
.wx-drawer[data-placement="left"]  { left: 0;  border-top-right-radius: var(--wx-radius-xl); border-bottom-right-radius: var(--wx-radius-xl); }

.wx-drawer[data-placement="top"], .wx-drawer[data-placement="bottom"] {
  left: 0; right: 0; width: 100vw;
}
.wx-drawer[data-placement="top"]    { top: 0; border-bottom-left-radius: var(--wx-radius-xl); border-bottom-right-radius: var(--wx-radius-xl); }
.wx-drawer[data-placement="bottom"] { bottom: 0; border-top-left-radius: var(--wx-radius-xl); border-top-right-radius: var(--wx-radius-xl); }

/* size — applies to "main axis" depending on placement */
.wx-drawer[data-placement="right"][data-size="sm"], .wx-drawer[data-placement="left"][data-size="sm"]  { width: 320px; }
.wx-drawer[data-placement="right"][data-size="md"], .wx-drawer[data-placement="left"][data-size="md"]  { width: 400px; }
.wx-drawer[data-placement="right"][data-size="lg"], .wx-drawer[data-placement="left"][data-size="lg"]  { width: 560px; }
.wx-drawer[data-placement="right"][data-size="xl"], .wx-drawer[data-placement="left"][data-size="xl"]  { width: 720px; }
.wx-drawer[data-placement="right"][data-size="full"], .wx-drawer[data-placement="left"][data-size="full"]{ width: 100vw; }

.wx-drawer[data-placement="top"][data-size="sm"], .wx-drawer[data-placement="bottom"][data-size="sm"]  { height: 240px; }
.wx-drawer[data-placement="top"][data-size="md"], .wx-drawer[data-placement="bottom"][data-size="md"]  { height: 360px; }
.wx-drawer[data-placement="top"][data-size="lg"], .wx-drawer[data-placement="bottom"][data-size="lg"]  { height: 480px; }
.wx-drawer[data-placement="top"][data-size="xl"], .wx-drawer[data-placement="bottom"][data-size="xl"]  { height: 600px; }
.wx-drawer[data-placement="top"][data-size="full"], .wx-drawer[data-placement="bottom"][data-size="full"]{ height: 100vh; }

@media (max-width: 640px) {
  .wx-drawer[data-placement="right"], .wx-drawer[data-placement="left"] {
    width: min(90vw, 400px) !important;
  }
}

/* parts */
.wx-drawer__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--wx-space-3);
  padding: var(--wx-space-4) var(--wx-space-5);
  border-bottom: 1px solid var(--wx-border-default);
  flex-shrink: 0;
}
.wx-drawer__title {
  margin: 0;
  font-size: var(--wx-fs-18);
  font-weight: var(--wx-fw-semibold);
  letter-spacing: var(--wx-tracking-tight);
}
.wx-drawer__close {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: none;
  border-radius: var(--wx-radius-md);
  background: transparent;
  color: var(--wx-content-secondary);
  cursor: pointer;
  transition: background var(--wx-d-micro) var(--wx-ease-standard);
}
.wx-drawer__close:hover { background: var(--wx-hover-bg); color: var(--wx-content-primary); }

.wx-drawer__body {
  flex: 1;
  overflow-y: auto;
  padding: var(--wx-space-5);
}
.wx-drawer__footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--wx-space-2);
  padding: var(--wx-space-4) var(--wx-space-5);
  border-top: 1px solid var(--wx-border-default);
  flex-shrink: 0;
}

/* transitions */
.wx-drawer-bd-enter-active, .wx-drawer-bd-leave-active { transition: opacity var(--wx-d-fast) var(--wx-ease-standard); }
.wx-drawer-bd-enter-from, .wx-drawer-bd-leave-to { opacity: 0; }

.wx-drawer-right-enter-active, .wx-drawer-right-leave-active,
.wx-drawer-left-enter-active,  .wx-drawer-left-leave-active,
.wx-drawer-top-enter-active,   .wx-drawer-top-leave-active,
.wx-drawer-bottom-enter-active,.wx-drawer-bottom-leave-active {
  transition: transform var(--wx-d-normal) var(--wx-ease-decelerate);
}
.wx-drawer-right-enter-from,  .wx-drawer-right-leave-to  { transform: translateX(100%); }
.wx-drawer-left-enter-from,   .wx-drawer-left-leave-to   { transform: translateX(-100%); }
.wx-drawer-top-enter-from,    .wx-drawer-top-leave-to    { transform: translateY(-100%); }
.wx-drawer-bottom-enter-from, .wx-drawer-bottom-leave-to { transform: translateY(100%); }
</style>
