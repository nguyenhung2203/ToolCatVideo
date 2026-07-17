<script setup lang="ts">
/**
 * ShimmerBlock — animated placeholder for loading content.
 * Use as a building block for skeletons matching real content shape.
 *
 * Defaults: width='100%', height='14px', radius='md', shape='block'.
 */
withDefaults(defineProps<{
  width?: string | number
  height?: string | number
  radius?: 'none' | 'sm' | 'md' | 'lg' | 'xl' | 'full'
  shape?: 'block' | 'text' | 'circle'
}>(), {
  width: '100%',
  height: '14px',
  radius: 'md',
  shape: 'block',
})

function px(v: string | number) {
  return typeof v === 'number' ? `${v}px` : v
}
</script>

<template>
  <span
    class="wx-shimmer-block wx-shimmer"
    :data-shape="shape"
    aria-hidden="true"
    :style="{
      width: px(width),
      height: shape === 'circle' ? px(width) : px(height),
      borderRadius:
        shape === 'circle' ? 'var(--wx-radius-full)' :
        shape === 'text'   ? 'var(--wx-radius-sm)' :
        `var(--wx-radius-${radius})`
    }"
  />
</template>

<style scoped>
.wx-shimmer-block {
  display: inline-block;
  vertical-align: middle;
}
.wx-shimmer-block[data-shape="circle"] { display: inline-block; }
</style>
