<script setup lang="ts">
import { ref, watch, nextTick, onMounted } from 'vue'

const props = defineProps<{
  modelValue: string
  tabs: Array<{ key: string; label: string; icon?: string; disabled?: boolean }>
  variant?: 'pill' | 'underline'
}>()

defineEmits<{
  'update:modelValue': [value: string]
}>()

const indicatorStyle = ref({ left: '0px', width: '0px' })
const tabsRef = ref<HTMLElement>()

function updateIndicator() {
  if (!tabsRef.value) return
  const active = tabsRef.value.querySelector('.base-tabs__tab--active') as HTMLElement
  if (active) {
    indicatorStyle.value = {
      left: `${active.offsetLeft}px`,
      width: `${active.offsetWidth}px`
    }
  }
}

watch(() => props.modelValue, () => nextTick(updateIndicator))
watch(() => props.tabs, () => nextTick(updateIndicator), { deep: true })
onMounted(() => nextTick(updateIndicator))
</script>

<template>
  <div class="base-tabs" :class="`base-tabs--${variant ?? 'pill'}`">
    <div ref="tabsRef" class="base-tabs__header">
      <button
        v-for="tab in tabs"
        :key="tab.key"
        class="base-tabs__tab"
        :class="{
          'base-tabs__tab--active': modelValue === tab.key,
          'base-tabs__tab--disabled': tab.disabled
        }"
        :disabled="tab.disabled"
        @click="!tab.disabled && $emit('update:modelValue', tab.key)"
      >
        <span v-if="tab.icon" class="base-tabs__icon">{{ tab.icon }}</span>
        {{ tab.label }}
      </button>
      <span v-if="(variant ?? 'pill') === 'pill'" class="base-tabs__indicator" :style="indicatorStyle" />
    </div>
    <div class="base-tabs__content">
      <template v-for="tab in tabs" :key="tab.key">
        <div v-show="modelValue === tab.key">
          <slot :name="tab.key" />
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped src="./BaseTabs.scoped.css"></style>
