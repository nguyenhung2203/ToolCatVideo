<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  value: number
  max?: number
  variant?: 'primary' | 'success' | 'warning' | 'error' | 'danger'
  size?: 'sm' | 'md' | 'lg'
  striped?: boolean
  animated?: boolean
  showLabel?: boolean
}>(), {
  max: 100,
  variant: 'primary',
  size: 'md',
  striped: false,
  animated: false,
  showLabel: false,
})

const percent = computed(() => Math.min(100, Math.max(0, (props.value / props.max) * 100)))
</script>

<template>
  <div class="base-progress" :class="`base-progress--${size}`">
    <div class="base-progress__track">
      <div
        class="base-progress__fill"
        :class="[
          `base-progress__fill--${variant}`,
          { 'base-progress__fill--striped': striped || animated },
          { 'base-progress__fill--animated': animated }
        ]"
        :style="{ width: percent + '%' }"
      />
    </div>
    <span v-if="showLabel || $slots.label" class="base-progress__label">
      <slot name="label">{{ Math.round(percent) }}%</slot>
    </span>
  </div>
</template>

<style scoped>
.base-progress {
  display: flex;
  align-items: center;
  gap: 8px;
}

.base-progress__track {
  flex: 1;
  background: var(--border-color);
  border-radius: 100px;
  overflow: hidden;
}

/* Sizes */
.base-progress--sm .base-progress__track { height: 4px; }
.base-progress--md .base-progress__track { height: 8px; }
.base-progress--lg .base-progress__track { height: 12px; }

.base-progress__fill {
  height: 100%;
  border-radius: 100px;
  transition: width 0.3s ease;
}

/* Variants */
.base-progress__fill--primary { background: var(--brand-primary); }
.base-progress__fill--success { background: var(--success-color); }
.base-progress__fill--warning { background: var(--warning-color); }
.base-progress__fill--error,
.base-progress__fill--danger { background: var(--error-color); }

/* Striped */
.base-progress__fill--striped {
  background-image: linear-gradient(
    45deg,
    rgba(255, 255, 255, 0.2) 25%,
    transparent 25%,
    transparent 50%,
    rgba(255, 255, 255, 0.2) 50%,
    rgba(255, 255, 255, 0.2) 75%,
    transparent 75%
  );
  background-size: 16px 16px;
}

.base-progress__fill--animated {
  animation: progress-stripes 0.8s linear infinite;
}

@keyframes progress-stripes {
  from { background-position: 16px 0; }
  to { background-position: 0 0; }
}

.base-progress__label {
  font-size: 12px;
  color: var(--text-secondary);
  min-width: 36px;
  text-align: right;
  font-weight: 600;
}
</style>
