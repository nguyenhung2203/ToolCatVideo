<script setup lang="ts">
withDefaults(defineProps<{
  rows?: number
  cols?: number
  height?: string
}>(), {
  rows: 5,
  cols: 4,
  height: '28px',
})
</script>

<template>
  <div class="skeleton-container">
    <div v-for="r in rows" :key="r" class="skeleton-row">
      <div
        v-for="c in cols"
        :key="c"
        class="skeleton-cell"
        :style="{ height }"
      />
    </div>
  </div>
</template>

<style scoped src="./BaseSkeleton.scoped.css"></style>
