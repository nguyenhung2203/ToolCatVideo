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

<style scoped>
.skeleton-container {
  padding: var(--card-padding-compact);
}

.skeleton-row {
  display: flex;
  gap: var(--grid-gap-compact);
  margin-bottom: var(--grid-gap-compact);
}

.skeleton-cell {
  flex: 1;
  border-radius: var(--radius-sm);
  background: linear-gradient(
    90deg,
    var(--bg-tertiary) 25%,
    var(--bg-secondary) 50%,
    var(--bg-tertiary) 75%
  );
  background-size: 400% 100%;
  animation: shimmer 1.5s ease-in-out infinite;
}

@keyframes shimmer {
  0% { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}
</style>
