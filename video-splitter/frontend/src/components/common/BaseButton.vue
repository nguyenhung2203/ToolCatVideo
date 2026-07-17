<script setup lang="ts">
defineProps<{
  variant?: 'primary' | 'secondary' | 'neutral' | 'ghost' | 'danger' | 'success' | 'warning' | 'cta' | 'link' | 'text'
  size?: 'sm' | 'md' | 'lg' | 'xl' | 'icon'
  type?: 'button' | 'submit' | 'reset'
  disabled?: boolean
  loading?: boolean
  icon?: string
  iconRight?: string
  block?: boolean
}>()

defineEmits<{
  click: [event: MouseEvent]
}>()
</script>

<template>
  <button
    class="wx-btn"
    :class="[
      `wx-btn--${variant ?? 'primary'}`,
      `wx-btn--${size ?? 'md'}`,
      { 'wx-btn--loading': loading, 'wx-btn--block': block },
    ]"
    :type="type ?? 'button'"
    :disabled="disabled || loading"
    @click="$emit('click', $event)"
  >
    <span v-if="loading" class="wx-btn__spinner" aria-hidden="true" />
    <span v-else-if="icon" class="wx-btn__icon" v-html="icon" aria-hidden="true" />
    <span v-if="$slots.default" class="wx-btn__label"><slot /></span>
    <span v-if="iconRight && !loading" class="wx-btn__icon" v-html="iconRight" aria-hidden="true" />
    <span v-if="variant === 'primary' || variant === 'cta'" class="wx-btn__shine" aria-hidden="true" />
  </button>
</template>

<style scoped>
.wx-btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--wx-space-2, 8px);
  border: none;
  border-radius: var(--wx-radius-lg);
  font-family: var(--wx-font-primary);
  font-weight: var(--wx-fw-semibold);
  cursor: pointer;
  transition:
    background   var(--wx-d-fast) var(--wx-ease-standard),
    box-shadow   var(--wx-d-fast) var(--wx-ease-standard),
    transform    var(--wx-d-fast) var(--wx-ease-standard),
    border-color var(--wx-d-fast) var(--wx-ease-standard),
    filter       var(--wx-d-fast) var(--wx-ease-standard),
    color        var(--wx-d-fast) var(--wx-ease-standard);
  white-space: nowrap;
  line-height: 1;
  overflow: hidden;
  letter-spacing: var(--wx-tracking-normal, 0);
  -webkit-user-select: none;
  user-select: none;
}

/* Accessibility: focus ring */
.wx-btn:focus-visible {
  outline: 2px solid var(--wx-brand-focus);
  outline-offset: 2px;
}

.wx-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
  pointer-events: none;
}

.wx-btn:active:not(:disabled) {
  transform: scale(0.975);
}

.wx-btn--block { width: 100%; }

/* ── Sizes ── */
/* Padding/gap snap to the 4px design-token scale (--wx-space-1..6 → 4/8/12/16/20/24).
   Old values (13/18/22 px, gaps 5/7) drifted off-scale and showed up in the
   2026-05-25 design audit. Heights also raised to hit WCAG/iOS-HIG touch
   target floors (44px at --lg, 40px at --md). */
.wx-btn--sm   { padding: 6px 12px;  font-size: var(--wx-fs-12); gap: 4px;  min-height: 32px; }
.wx-btn--md   { padding: 8px 16px;  font-size: var(--wx-fs-14); gap: 8px;  min-height: 40px; }
.wx-btn--lg   { padding: 12px 24px; font-size: var(--wx-fs-15); gap: 8px;  min-height: 44px; }
.wx-btn--xl   { padding: 16px 28px; font-size: var(--wx-fs-16); gap: 12px; min-height: 52px; border-radius: var(--wx-radius-xl); }
.wx-btn--icon {
  padding: 0;
  /* 40px square = touch-target compliant; old 36px failed audit. */
  width: 40px; height: 40px;
  border-radius: var(--wx-radius-md);
  flex-shrink: 0;
}

/* ── Primary — gradient button token ── */
.wx-btn--primary {
  background: var(--wx-gradient-button);
  color: var(--wx-text-on-brand);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.22),
    0 1px 3px rgba(0, 0, 0, 0.12),
    0 4px 14px -2px rgba(37, 99, 235, 0.45),
    0 8px 24px -4px rgba(0, 51, 102, 0.20);
}
.wx-btn--primary:hover:not(:disabled) {
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.22),
    0 2px 4px rgba(0, 0, 0, 0.12),
    0 8px 22px -2px rgba(37, 99, 235, 0.55),
    0 12px 32px -4px rgba(0, 51, 102, 0.25);
  filter: brightness(1.06);
  transform: translateY(-1px);
}
.wx-btn--primary:active:not(:disabled) {
  filter: brightness(0.95);
  transform: scale(0.975);
}

/* ── CTA — deeper gradient ── */
.wx-btn--cta {
  background: var(--wx-gradient-cta);
  color: var(--wx-text-on-brand);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 4px 14px -2px rgba(58, 123, 213, 0.35);
}
.wx-btn--cta:hover:not(:disabled) {
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 8px 22px -2px rgba(58, 123, 213, 0.5);
  transform: translateY(-1px);
}
.wx-btn--cta:active:not(:disabled) {
  transform: translateY(0) scale(0.975);
}

/* ── Secondary — surface card ── */
.wx-btn--secondary {
  background: var(--wx-surface-elevated);
  color: var(--wx-text-primary);
  border: 1px solid var(--wx-border-default);
  box-shadow: var(--wx-shadow-sm);
}
.wx-btn--secondary:hover:not(:disabled) {
  background: var(--wx-hover-bg);
  box-shadow: var(--wx-shadow-md);
  transform: translateY(-1px);
}
.wx-btn--secondary:active:not(:disabled) {
  transform: scale(0.975);
}

/* ── Neutral — subtle filled ── */
.wx-btn--neutral {
  background: var(--wx-neutral-bg);
  color: var(--wx-neutral-text);
  border: 1px solid var(--wx-neutral-border);
}
.wx-btn--neutral:hover:not(:disabled) {
  background: var(--wx-hover-bg);
  color: var(--wx-text-primary);
  border-color: var(--wx-border-default);
}

/* ── Ghost — transparent ── */
.wx-btn--ghost {
  background: transparent;
  color: var(--wx-text-secondary);
}
.wx-btn--ghost:hover:not(:disabled) {
  background: var(--wx-hover-bg);
  color: var(--wx-text-primary);
}

/* ── Danger — gradient ── */
.wx-btn--danger {
  background: var(--wx-gradient-danger);
  color: var(--wx-text-on-brand);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 4px 12px -2px rgba(220, 38, 38, 0.25);
}
.wx-btn--danger:hover:not(:disabled) {
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 6px 20px -2px rgba(220, 38, 38, 0.4);
  filter: brightness(1.07);
  transform: translateY(-1px);
}
.wx-btn--danger:active:not(:disabled) { transform: scale(0.975); }

/* ── Success — gradient ── */
.wx-btn--success {
  background: var(--wx-gradient-success);
  color: var(--wx-text-on-brand);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 4px 12px -2px rgba(22, 163, 74, 0.25);
}
.wx-btn--success:hover:not(:disabled) {
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 6px 20px -2px rgba(22, 163, 74, 0.4);
  filter: brightness(1.07);
}

/* ── Warning — gradient ── */
.wx-btn--warning {
  background: var(--wx-gradient-warning);
  color: var(--wx-text-on-brand);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 4px 12px -2px rgba(217, 119, 6, 0.25);
}
.wx-btn--warning:hover:not(:disabled) {
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 6px 20px -2px rgba(217, 119, 6, 0.4);
  filter: brightness(1.07);
}

/* ── Link / Text — text link minimal ── */
.wx-btn--text {
  background: transparent;
  color: var(--wx-text-secondary);
  padding-left: 4px;
  padding-right: 4px;
  font-weight: var(--wx-fw-medium);
  overflow: visible;
}
.wx-btn--text:hover:not(:disabled) {
  background: var(--wx-hover-bg);
  color: var(--wx-text-primary);
}

.wx-btn--link {
  background: transparent;
  color: var(--wx-text-link);
  padding-left: 4px;
  padding-right: 4px;
  font-weight: var(--wx-fw-medium);
  overflow: visible;
}
.wx-btn--link:hover:not(:disabled) {
  text-decoration: underline;
  text-underline-offset: 2px;
}

/* ── Shine sweep — primary & cta only ── */
.wx-btn__shine {
  position: absolute;
  inset: 0;
  transform: translateX(calc(-100% - 2px));
  background: linear-gradient(90deg, transparent 0%, rgba(255, 255, 255, 0.18) 50%, transparent 100%);
  pointer-events: none;
  transition: transform var(--wx-d-decorative) var(--wx-ease-accelerate);
}
.wx-btn:hover .wx-btn__shine {
  transform: translateX(calc(100% + 2px));
}

/* ── Spinner ── */
.wx-btn__spinner {
  width: 14px;
  height: 14px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: currentColor;
  border-radius: var(--wx-radius-full);
  animation: wx-btn-spin 0.6s linear infinite;
  flex-shrink: 0;
}
@keyframes wx-btn-spin { to { transform: rotate(360deg); } }

.wx-btn__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  line-height: 1;
}

.wx-btn__label {
  position: relative;
  z-index: 1;
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

/* ── Dark mode ── */
.wx-dark .wx-btn--secondary {
  background: var(--wx-surface-elevated);
  color: var(--wx-text-primary);
  border-color: var(--wx-border-default);
}
.wx-dark .wx-btn--secondary:hover:not(:disabled) { background: var(--wx-hover-bg); }
.wx-dark .wx-btn--neutral { background: var(--wx-neutral-bg); color: var(--wx-neutral-text); }
.wx-dark .wx-btn--ghost:hover:not(:disabled) {
  background: var(--wx-hover-bg);
  color: var(--wx-text-primary);
}
</style>
