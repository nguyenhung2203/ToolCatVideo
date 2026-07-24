<script setup lang="ts">
/**
 * BaseSpinner — modern conic-gradient ring loader.
 *
 * Defaults: size='md' (18px), tone='current'.
 *
 * Visual: 1 vòng tròn (annular ring) duy nhất, conic-gradient quét từ
 *  bright (currentColor) → transparent qua ~270°, có khoảng trống ~72°
 *  → cảm giác "comet tail" xoay liên tục.
 *
 *  - 1 element animate → reliable mọi browser, không phụ thuộc CSS scoping.
 *  - conic-gradient + mask radial → ring thuần CSS, scale vector hoàn hảo.
 *  - currentColor → kế thừa màu chữ container (button label, link…).
 *
 * Reduced motion: cycle 1.6s thay vì 0.8s.
 */
withDefaults(defineProps<{
  size?: 'sm' | 'md' | 'lg' | 'xl'
  tone?: 'current' | 'primary' | 'inverse' | 'muted'
  label?: string
}>(), {
  size: 'md',
  tone: 'current',
  label: 'đang tải',
})
</script>

<template>
  <span
    class="wx-spinner"
    :data-size="size"
    :data-tone="tone"
    role="status"
    :aria-label="label"
  >
    <span class="wx-spinner__ring" aria-hidden="true" />
  </span>
</template>

<style scoped src="./BaseSpinner.scoped.css"></style>
