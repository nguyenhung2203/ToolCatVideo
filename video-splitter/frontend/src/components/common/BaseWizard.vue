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

<style scoped src="./BaseWizard.scoped.css"></style>
