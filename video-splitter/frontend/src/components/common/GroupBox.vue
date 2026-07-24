<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  title: string
  fill?: boolean
  collapsible?: boolean
  collapsed?: boolean
}>(), {
  fill: false,
  collapsible: false,
  collapsed: false,
})

const emit = defineEmits<{
  'update:collapsed': [value: boolean]
}>()

const isCollapsed = computed(() => props.collapsed)

function toggle() {
  if (props.collapsible) {
    emit('update:collapsed', !isCollapsed.value)
  }
}
</script>

<template>
  <div class="gbox" :class="{ 'gbox--fill': props.fill, 'gbox--collapsed': isCollapsed }">
    <div
      class="gbox-header"
      :class="{ 'gbox-header--clickable': props.collapsible }"
      @click="toggle"
    >
      <span v-if="props.collapsible" class="gbox-chevron" :class="{ 'gbox-chevron--collapsed': isCollapsed }">▸</span>
      {{ props.title }}
    </div>
    <transition name="gbox-collapse">
      <div v-show="!isCollapsed" class="gbox-body">
        <slot />
      </div>
    </transition>
  </div>
</template>

<style scoped src="./GroupBox.scoped.css"></style>
