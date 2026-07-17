<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import StepProgress from '@/components/async/StepProgress.vue'

type WizardStep = string | { label: string; description?: string }

const props = defineProps<{
  steps: WizardStep[]
  modelValue?: number
}>()

const emit = defineEmits<{
  'update:modelValue': [step: number]
  'finish': []
}>()

/* Uncontrolled by default — syncs when v-model is provided.
   Using :key on the component resets it automatically per item. */
const step = ref(props.modelValue ?? 0)

watch(() => props.modelValue, v => {
  if (v !== undefined) step.value = v
})

const direction = ref<'fwd' | 'bwd'>('fwd')
const tName = computed(() => `wx-wiz-${direction.value}`)

const isFirst = computed(() => step.value === 0)
const isLast  = computed(() => step.value === props.steps.length - 1)

function next() {
  if (isLast.value) return
  direction.value = 'fwd'
  step.value++
  emit('update:modelValue', step.value)
}
function back() {
  if (isFirst.value) return
  direction.value = 'bwd'
  step.value--
  emit('update:modelValue', step.value)
}
function finish() {
  emit('finish')
}
</script>

<template>
  <div class="wx-wizard">
    <StepProgress
      :steps="steps"
      :current="step"
      orientation="horizontal"
      size="sm"
      class="wx-wizard__nav"
    />

    <div class="wx-wizard__content">
      <Transition :name="tName" mode="out-in">
        <div :key="step" class="wx-wizard__panel">
          <slot
            :name="`step-${step}`"
            v-bind="{ step, isFirst, isLast, next, back, finish }"
          />
        </div>
      </Transition>
    </div>

    <div class="wx-wizard__foot">
      <slot name="actions" v-bind="{ step, isFirst, isLast, next, back, finish }">
        <button
          v-if="!isFirst"
          type="button"
          class="wx-wiz-btn wx-wiz-btn--ghost"
          @click="back"
        >← Quay lại</button>
        <span class="wx-wiz-spacer" />
        <button
          v-if="!isLast"
          type="button"
          class="wx-wiz-btn wx-wiz-btn--primary"
          @click="next"
        >Tiếp theo →</button>
        <button
          v-else
          type="button"
          class="wx-wiz-btn wx-wiz-btn--success"
          @click="finish"
        >Hoàn tất ✓</button>
      </slot>
    </div>
  </div>
</template>

<style scoped>
.wx-wizard {
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-4);
}

.wx-wizard__nav {
  width: 100%;
}

/* ── Content area with transitions ── */
.wx-wizard__content {
  position: relative;
  overflow: hidden;
  min-height: 80px;
}

.wx-wizard__panel {
  width: 100%;
}

/* ── Footer actions ── */
.wx-wizard__foot {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  padding-top: var(--wx-space-3);
  border-top: 1px solid var(--wx-border-subtle);
}

.wx-wiz-spacer { flex: 1; }

.wx-wiz-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
  padding: var(--wx-space-2) var(--wx-space-4);
  border-radius: var(--wx-radius-md);
  border: 1px solid transparent;
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-medium);
  cursor: pointer;
  transition:
    background var(--wx-d-fast) var(--wx-ease-standard),
    opacity var(--wx-d-fast);
}
.wx-wiz-btn:disabled { opacity: 0.45; cursor: not-allowed; }

.wx-wiz-btn--ghost {
  background: transparent;
  border-color: var(--wx-border-default);
  color: var(--wx-text-secondary);
}
.wx-wiz-btn--ghost:hover:not(:disabled) {
  background: var(--wx-surface-raised);
  color: var(--wx-text-primary);
}

.wx-wiz-btn--primary {
  background: var(--wx-brand-primary);
  color: var(--wx-text-inverse);
}
.wx-wiz-btn--primary:hover:not(:disabled) { opacity: 0.88; }

.wx-wiz-btn--success {
  background: var(--wx-success-solid);
  color: var(--wx-text-inverse);
}
.wx-wiz-btn--success:hover:not(:disabled) { opacity: 0.88; }

/* ── Forward slide ── */
.wx-wiz-fwd-enter-active,
.wx-wiz-fwd-leave-active { transition: all 220ms var(--wx-ease-standard); }
.wx-wiz-fwd-enter-from   { opacity: 0; transform: translateX(32px); }
.wx-wiz-fwd-leave-to     { opacity: 0; transform: translateX(-32px); }

/* ── Backward slide ── */
.wx-wiz-bwd-enter-active,
.wx-wiz-bwd-leave-active { transition: all 220ms var(--wx-ease-standard); }
.wx-wiz-bwd-enter-from   { opacity: 0; transform: translateX(-32px); }
.wx-wiz-bwd-leave-to     { opacity: 0; transform: translateX(32px); }

@media (prefers-reduced-motion: reduce) {
  .wx-wiz-fwd-enter-active,
  .wx-wiz-fwd-leave-active,
  .wx-wiz-bwd-enter-active,
  .wx-wiz-bwd-leave-active { transition: opacity 150ms; }
  .wx-wiz-fwd-enter-from,
  .wx-wiz-fwd-leave-to,
  .wx-wiz-bwd-enter-from,
  .wx-wiz-bwd-leave-to     { transform: none; }
}
</style>
