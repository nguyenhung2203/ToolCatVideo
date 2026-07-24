<script setup lang="ts">
/**
 * BaseCard — surface container with optional header/body/footer.
 * Phase 3 — form & layout shells.
 *
 * defaults theo phần II:
 *  - radius: lg (12px)
 *  - shadow: md
 *  - padded: true
 */
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  /** tiêu đề header (text). nếu cần rich → dùng slot #header */
  title?: string
  /** mô tả nhỏ dưới title */
  subtitle?: string
  /** padding body. mặc định true */
  padded?: boolean
  /** mức shadow */
  shadow?: 'none' | 'sm' | 'md' | 'lg' | 'xl'
  /** radius */
  radius?: 'md' | 'lg' | 'xl' | '2xl'
  /** hover lift effect (translateY -2px + shadow +1) — deprecated, dùng hoverEffect thay */
  hoverable?: boolean
  /** kiểu hover effect:
   *  - 'none'        : không hover effect
   *  - 'glow'        : vùng xanh gradient fade vào trong từ rìa
   *  - 'lift'        : nhấc lên + shadow sâu
   *  - 'shimmer'     : scale nhẹ + ánh sáng brand sweep qua
   *  - 'glow-lift'   : kết hợp glow + lift (::before + transform/shadow — không conflict)
   *  - 'glow-shimmer': kết hợp glow + shimmer (::before + ::after — không conflict)
   *  Lưu ý: 'lift' và 'shimmer' không combine được với nhau vì cùng dùng `transform`.
   */
  hoverEffect?: 'none' | 'glow' | 'lift' | 'shimmer' | 'glow-lift' | 'glow-shimmer'
  /** màu accent riêng (CSS color) — thêm viền trái 3px + nhuộm màu cho glow/lift hover thay vì brand-primary mặc định */
  accentColor?: string
  /** hiện viền — mặc định true */
  bordered?: boolean
  /** trạng thái selected (highlight) */
  selected?: boolean
  /** loading skeleton overlay */
  loading?: boolean
  /** disable interaction */
  disabled?: boolean
  /** click toàn card → emit click + cursor pointer */
  clickable?: boolean
}>(), {
  padded: true,
  shadow: 'md',
  radius: 'lg',
  hoverable: false,
  hoverEffect: 'none',
  bordered: true,
  selected: false,
  loading: false,
  disabled: false,
  clickable: false,
})

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()

const hasHeader = computed(() => Boolean(props.title || props.subtitle))

function onClick(e: MouseEvent) {
  if (props.disabled || props.loading) return
  if (props.clickable) emit('click', e)
}
</script>

<template>
  <div
    class="wx-card"
    :data-radius="radius"
    :data-shadow="shadow"
    :data-state="loading ? 'loading' : disabled ? 'disabled' : selected ? 'selected' : 'default'"
    :style="accentColor ? { '--wx-card-accent': accentColor } : undefined"
    :class="{
      'wx-card--padded': padded,
      'wx-card--hoverable': hoverable && !disabled,
      'wx-card--hover-glow': (hoverEffect === 'glow' || hoverEffect === 'glow-lift' || hoverEffect === 'glow-shimmer') && !disabled,
      'wx-card--hover-lift': (hoverEffect === 'lift' || hoverEffect === 'glow-lift') && !disabled,
      'wx-card--hover-shimmer': (hoverEffect === 'shimmer' || hoverEffect === 'glow-shimmer') && !disabled,
      'wx-card--accent': !!accentColor,
      'wx-card--bordered': bordered,
      'wx-card--clickable': clickable && !disabled,
    }"
    :role="clickable ? 'button' : undefined"
    :tabindex="clickable && !disabled ? 0 : undefined"
    @click="onClick"
    @keydown.enter.space="clickable && !disabled ? $emit('click', $event as unknown as MouseEvent) : undefined"
  >
    <header v-if="hasHeader || $slots.header" class="wx-card__header" data-part="header">
      <slot name="header">
        <div class="wx-card__header-text">
          <h3 v-if="title" class="wx-card__title">{{ title }}</h3>
          <p v-if="subtitle" class="wx-card__subtitle">{{ subtitle }}</p>
        </div>
      </slot>
      <div v-if="$slots.actions" class="wx-card__actions" data-part="actions">
        <slot name="actions" />
      </div>
    </header>

    <div class="wx-card__body" data-part="body">
      <slot name="body">
        <slot />
      </slot>
    </div>

    <footer v-if="$slots.footer" class="wx-card__footer" data-part="footer">
      <slot name="footer" />
    </footer>

    <div v-if="loading" class="wx-card__loading" aria-hidden="true">
      <span class="wx-card__loading-bar" />
      <span class="wx-card__loading-bar wx-card__loading-bar--short" />
    </div>
  </div>
</template>

<style scoped src="./BaseCard.scoped.css"></style>
