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

<style scoped>
.wx-spinner {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  vertical-align: middle;
  line-height: 0;
}

.wx-spinner__ring {
  display: block;
  width: 100%;
  height: 100%;
  border-radius: 9999px;
  /* conic gradient: bright head ở 0° (12 giờ), fade dần xuống 0%
     đến 75%, transparent 75-100% tạo gap → đuôi sao chổi rõ rệt. */
  background:
    conic-gradient(
      from 0deg,
      currentColor 0%,
      currentColor 8%,
      transparent 75%,
      transparent 100%
    );
  /* mask radial: cắt giữa thành donut. ring dày 33% bán kính. */
  -webkit-mask: radial-gradient(circle, transparent 60%, #000 62%);
          mask: radial-gradient(circle, transparent 60%, #000 62%);
  -webkit-mask-repeat: no-repeat;
          mask-repeat: no-repeat;
  animation: wx-spinner-rotate 0.8s linear infinite;
  transform-origin: 50% 50%;
  /* ép tăng tốc GPU cho mượt */
  will-change: transform;
}

@keyframes wx-spinner-rotate {
  from { transform: rotate(0deg); }
  to   { transform: rotate(360deg); }
}

/* size — square containers; ring fills 100% so vector scales clean */
.wx-spinner[data-size="sm"] { width: 14px; height: 14px; }
.wx-spinner[data-size="md"] { width: 18px; height: 18px; }
.wx-spinner[data-size="lg"] { width: 28px; height: 28px; }
.wx-spinner[data-size="xl"] { width: 44px; height: 44px; }

/* tone */
.wx-spinner[data-tone="current"] { color: currentColor; }
.wx-spinner[data-tone="primary"] { color: var(--wx-brand-primary); }
.wx-spinner[data-tone="inverse"] { color: var(--wx-content-inverse); }
.wx-spinner[data-tone="muted"]   { color: var(--wx-content-muted); }

@media (prefers-reduced-motion: reduce) {
  .wx-spinner__ring { animation-duration: 1.6s; }
}

</style>
