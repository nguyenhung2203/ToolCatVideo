<script setup lang="ts">
/**
 * BaseCard — surface container with optional header/body/footer.
 * Phase 3 — form & layout shells.
 *
 * defaults theo phần II:
 *  - radius: lg (12px)
 *  - shadow: md
 *  - padded: true
 */
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  /** tiêu đề header (text). nếu cần rich → dùng slot #header */
  title?: string
  /** mô tả nhỏ dưới title */
  subtitle?: string
  /** padding body. mặc định true */
  padded?: boolean
  /** mức shadow */
  shadow?: 'none' | 'sm' | 'md' | 'lg' | 'xl'
  /** radius */
  radius?: 'md' | 'lg' | 'xl' | '2xl'
  /** hover lift effect (translateY -2px + shadow +1) — deprecated, dùng hoverEffect thay */
  hoverable?: boolean
  /** kiểu hover effect:
   *  - 'none'        : không hover effect
   *  - 'glow'        : vùng xanh gradient fade vào trong từ rìa
   *  - 'lift'        : nhấc lên + shadow sâu
   *  - 'shimmer'     : scale nhẹ + ánh sáng brand sweep qua
   *  - 'glow-lift'   : kết hợp glow + lift (::before + transform/shadow — không conflict)
   *  - 'glow-shimmer': kết hợp glow + shimmer (::before + ::after — không conflict)
   *  Lưu ý: 'lift' và 'shimmer' không combine được với nhau vì cùng dùng `transform`.
   */
  hoverEffect?: 'none' | 'glow' | 'lift' | 'shimmer' | 'glow-lift' | 'glow-shimmer'
  /** màu accent riêng (CSS color) — thêm viền trái 3px + nhuộm màu cho glow/lift hover thay vì brand-primary mặc định */
  accentColor?: string
  /** hiện viền — mặc định true */
  bordered?: boolean
  /** trạng thái selected (highlight) */
  selected?: boolean
  /** loading skeleton overlay */
  loading?: boolean
  /** disable interaction */
  disabled?: boolean
  /** click toàn card → emit click + cursor pointer */
  clickable?: boolean
}>(), {
  padded: true,
  shadow: 'md',
  radius: 'lg',
  hoverable: false,
  hoverEffect: 'none',
  bordered: true,
  selected: false,
  loading: false,
  disabled: false,
  clickable: false,
})

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

const hasHeader = computed(() => Boolean(props.title || props.subtitle))

function onClick(e: MouseEvent) {
  if (props.disabled || props.loading) return
  if (props.clickable) emit('click', e)
}
</script>

<template>
  <div
    class="wx-card"
    :data-radius="radius"
    :data-shadow="shadow"
    :data-state="loading ? 'loading' : disabled ? 'disabled' : selected ? 'selected' : 'default'"
    :style="accentColor ? { '--wx-card-accent': accentColor } : undefined"
    :class="{
      'wx-card--padded': padded,
      'wx-card--hoverable': hoverable && !disabled,
      'wx-card--hover-glow': (hoverEffect === 'glow' || hoverEffect === 'glow-lift' || hoverEffect === 'glow-shimmer') && !disabled,
      'wx-card--hover-lift': (hoverEffect === 'lift' || hoverEffect === 'glow-lift') && !disabled,
      'wx-card--hover-shimmer': (hoverEffect === 'shimmer' || hoverEffect === 'glow-shimmer') && !disabled,
      'wx-card--accent': !!accentColor,
      'wx-card--bordered': bordered,
      'wx-card--clickable': clickable && !disabled,
    }"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable && !disabled ? 0 : undefined"
    @click="onClick"
    @keydown.enter.space="clickable && !disabled ? $emit('click', $event as unknown as MouseEvent) : undefined"
  >
    <header v-if="hasHeader || $slots.header" class="wx-card__header" data-part="header">
      <slot name="header">
        <div class="wx-card__header-text">
          <h3 v-if="title" class="wx-card__title">{{ title }}</h3>
          <p v-if="subtitle" class="wx-card__subtitle">{{ subtitle }}</p>
        </div>
      </slot>
      <div v-if="$slots.actions" class="wx-card__actions" data-part="actions">
        <slot name="actions" />
      </div>
    </header>

    <div class="wx-card__body" data-part="body">
      <slot name="body">
        <slot />
      </slot>
    </div>

    <footer v-if="$slots.footer" class="wx-card__footer" data-part="footer">
      <slot name="footer" />
    </footer>

    <div v-if="loading" class="wx-card__loading" aria-hidden="true">
      <span class="wx-card__loading-bar" />
      <span class="wx-card__loading-bar wx-card__loading-bar--short" />
    </div>
  </div>
</template>

<style scoped>
.wx-card {
  position: relative;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  background: var(--wx-surface-elevated);
  color: var(--wx-content-primary);
  font-family: var(--wx-font-primary);
  transition:
    transform var(--wx-d-fast) var(--wx-ease-standard),
    box-shadow var(--wx-d-fast) var(--wx-ease-standard),
    border-color var(--wx-d-fast) var(--wx-ease-standard);
}

/* radius */
.wx-card[data-radius="md"]  { border-radius: var(--wx-radius-md); }
.wx-card[data-radius="lg"]  { border-radius: var(--wx-radius-lg); }
.wx-card[data-radius="xl"]  { border-radius: var(--wx-radius-xl); }
.wx-card[data-radius="2xl"] { border-radius: var(--wx-radius-2xl); }

/* shadow */
.wx-card[data-shadow="none"] { box-shadow: none; }
.wx-card[data-shadow="sm"]   { box-shadow: var(--wx-shadow-sm); }
.wx-card[data-shadow="md"]   { box-shadow: var(--wx-shadow-md); }
.wx-card[data-shadow="lg"]   { box-shadow: var(--wx-shadow-lg); }
.wx-card[data-shadow="xl"]   { box-shadow: var(--wx-shadow-xl); }

.wx-card--bordered {
  border: 1px solid color-mix(in srgb, var(--wx-border-default) 60%, transparent);
}
.wx-card--accent {
  border-left: 3px solid var(--wx-card-accent);
}

.wx-card--padded > .wx-card__body { padding: var(--wx-space-5); }
.wx-card--padded > .wx-card__header { padding: var(--wx-space-4) var(--wx-space-5); }
.wx-card--padded > .wx-card__footer { padding: var(--wx-space-4) var(--wx-space-5); }

.wx-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--wx-space-3);
  border-bottom: 1px solid var(--wx-border-subtle);
}
.wx-card__header-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.wx-card__title {
  margin: 0;
  font-size: var(--wx-fs-16);
  font-weight: var(--wx-fw-semibold);
  color: var(--wx-content-primary);
  letter-spacing: var(--wx-tracking-normal);
  line-height: var(--wx-lh-snug);
}
.wx-card__subtitle {
  margin: 0;
  font-size: var(--wx-fs-13);
  color: var(--wx-content-muted);
  line-height: var(--wx-lh-normal);
}
.wx-card__actions {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  flex-shrink: 0;
}

.wx-card__body {
  flex: 1;
  min-width: 0;
}

.wx-card__footer {
  border-top: 1px solid var(--wx-border-subtle);
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--wx-space-2);
  background: var(--wx-surface-sunken);
}

/* states */
.wx-card[data-state="selected"] {
  border-color: var(--wx-brand-primary);
  box-shadow: 0 0 0 1px var(--wx-brand-primary), var(--wx-shadow-md);
}
.wx-card[data-state="disabled"] {
  opacity: 0.55;
  pointer-events: none;
}

/* default hover — blue glow ring for all non-disabled cards */
.wx-card:not([data-state="disabled"]):not([data-state="loading"]):hover {
  border-color: color-mix(in srgb, var(--wx-brand-primary) 50%, transparent);
  box-shadow:
    0 0 0 3px color-mix(in srgb, var(--wx-brand-primary) 12%, transparent),
    var(--wx-shadow-md);
}

/* hoverable adds lift on top of the blue glow */
.wx-card--hoverable:not([data-state="disabled"]):hover {
  transform: translateY(-2px);
  box-shadow:
    0 0 0 3px color-mix(in srgb, var(--wx-brand-primary) 12%, transparent),
    var(--wx-shadow-lg);
}

/* ══════════════════════════════════════════════════════════════
   Hover Effect: glow — vùng xanh gradient fade vào trong
   ══════════════════════════════════════════════════════════════ */
.wx-card--hover-glow {
  transition:
    box-shadow var(--wx-d-normal) var(--wx-ease-standard),
    border-color var(--wx-d-normal) var(--wx-ease-standard);
}
.wx-card--hover-glow::before {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: radial-gradient(
    ellipse at 50% 50%,
    transparent 40%,
    color-mix(in srgb, var(--wx-card-accent, var(--wx-brand-primary)) 8%, transparent) 70%,
    color-mix(in srgb, var(--wx-card-accent, var(--wx-brand-primary)) 14%, transparent) 100%
  );
  opacity: 0;
  transition: opacity var(--wx-d-normal) var(--wx-ease-standard);
  pointer-events: none;
  z-index: 0;
}
.wx-card--hover-glow:not([data-state="disabled"]):not([data-state="loading"]):hover {
  border-color: color-mix(in srgb, var(--wx-card-accent, var(--wx-brand-primary)) 20%, transparent);
}
.wx-card--hover-glow:not([data-state="disabled"]):not([data-state="loading"]):hover::before {
  opacity: 1;
}

/* ══════════════════════════════════════════════════════════════
   Hover Effect: lift — nhấc lên + shadow sâu
   ══════════════════════════════════════════════════════════════ */
.wx-card--hover-lift {
  transition:
    transform var(--wx-d-normal) var(--wx-ease-bounce),
    box-shadow var(--wx-d-normal) var(--wx-ease-standard),
    border-color var(--wx-d-fast) var(--wx-ease-standard);
}
.wx-card--hover-lift:not([data-state="disabled"]):not([data-state="loading"]):hover {
  transform: translateY(-4px);
  border-color: var(--wx-card-accent, var(--wx-border-subtle));
  box-shadow:
    0 12px 32px -8px color-mix(in srgb, var(--wx-card-accent, var(--wx-brand-primary)) 18%, transparent),
    0 4px 12px color-mix(in srgb, var(--wx-card-accent, var(--wx-brand-primary)) 10%, transparent);
}

/* ══════════════════════════════════════════════════════════════
   Hover Effect: shimmer — scale + ánh sáng brand sweep chậm
   ══════════════════════════════════════════════════════════════ */
.wx-card--hover-shimmer {
  transition:
    transform var(--wx-d-normal) var(--wx-ease-standard),
    box-shadow var(--wx-d-normal) var(--wx-ease-standard),
    border-color var(--wx-d-normal) var(--wx-ease-standard);
}
.wx-card--hover-shimmer::after {
  content: '';
  position: absolute;
  inset: 0;
  border-radius: inherit;
  background: linear-gradient(
    105deg,
    transparent 35%,
    color-mix(in srgb, var(--wx-brand-accent) 15%, transparent) 45%,
    color-mix(in srgb, var(--wx-brand-primary) 12%, transparent) 55%,
    transparent 65%
  );
  transform: translateX(-110%);
  pointer-events: none;
  z-index: 1;
}
.wx-card--hover-shimmer:not([data-state="disabled"]):not([data-state="loading"]):hover {
  transform: scale(1.015);
  border-color: color-mix(in srgb, var(--wx-brand-primary) 35%, var(--wx-border-default));
  box-shadow: var(--wx-shadow-lg);
}
.wx-card--hover-shimmer:not([data-state="disabled"]):not([data-state="loading"]):hover::after {
  animation: wx-card-shimmer-sweep 0.9s var(--wx-ease-standard) forwards;
}

.wx-card--clickable {
  cursor: pointer;
}
.wx-card--clickable:focus-visible {
  outline: 2px solid var(--wx-border-focus);
  outline-offset: 2px;
}
.wx-card--clickable:active:not([data-state="disabled"]) {
  transform: scale(0.995);
}

/* loading skeleton overlay (replaces body content visually) */
.wx-card__loading {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: var(--wx-space-2);
  padding: var(--wx-space-5);
  background: var(--wx-surface-elevated);
  border-radius: inherit;
}
.wx-card__loading-bar {
  height: 12px;
  border-radius: var(--wx-radius-md);
  background: linear-gradient(
    90deg,
    var(--wx-surface-sunken) 0%,
    var(--wx-border-subtle) 50%,
    var(--wx-surface-sunken) 100%
  );
  background-size: 200% 100%;
  animation: wx-card-shimmer 1.4s infinite var(--wx-ease-standard);
}
.wx-card__loading-bar--short { width: 60%; }

@keyframes wx-card-shimmer-sweep {
  to { transform: translateX(110%); }
}

@keyframes wx-card-shimmer {
  0%   { background-position: 100% 0; }
  100% { background-position: -100% 0; }
}

@media (prefers-reduced-motion: reduce) {
  .wx-card { transition: none; }
  .wx-card--hover-glow,
  .wx-card--hover-lift,
  .wx-card--hover-shimmer { transition: none; }
  .wx-card--hover-shimmer:hover::after { animation: none; }
  .wx-card__loading-bar { animation: none; }
}
</style>
