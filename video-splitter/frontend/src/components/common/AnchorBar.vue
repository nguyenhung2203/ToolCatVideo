<script setup lang="ts">
/**
 * AnchorBar — thanh điều hướng sticky anchor trong trang.
 * Click → smooth-scroll đến section. ScrollSpy tự động highlight section đang hiện.
 *
 *   <AnchorBar :sections="sections" v-model="activeId" />
 *
 * sections: [{ id: 'intro', label: 'Giới thiệu' }, ...]
 */
import { onBeforeUnmount, onMounted, ref } from 'vue'

interface Section {
  id: string
  label: string
}

interface Props {
  sections: Section[]
  /** bật scroll-spy qua IntersectionObserver */
  scrollSpy?: boolean
  /** khoảng cách top khi sticky (px) */
  stickyTop?: number
  /** tắt sticky — dùng như inline nav */
  inline?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  scrollSpy: true,
  stickyTop: 12,
})

const model = defineModel<string>()
const navEl = ref<HTMLElement | null>(null)

function getScrollContainer(): HTMLElement | null {
  if (typeof document === 'undefined' || !navEl.value) return null
  let el: HTMLElement | null = navEl.value.parentElement
  while (el) {
    const oy = getComputedStyle(el).overflowY
    if ((oy === 'auto' || oy === 'scroll') && el.scrollHeight > el.clientHeight) return el
    el = el.parentElement
  }
  return null
}

function jump(id: string) {
  model.value = id
  const el = document.getElementById(id)
  if (!el) return

  const navHeight = props.inline ? 0 : (navEl.value?.offsetHeight ?? 0) + props.stickyTop + 8
  const container = getScrollContainer()

  if (!container) {
    // page-level scroll
    const top = el.getBoundingClientRect().top + window.scrollY - navHeight
    window.scrollTo({ top: Math.max(0, top), behavior: 'smooth' })
    return
  }

  const top =
    el.getBoundingClientRect().top -
    container.getBoundingClientRect().top +
    container.scrollTop -
    navHeight
  container.scrollTo({ top: Math.max(0, top), behavior: 'smooth' })
}

/* ── scroll-spy ── */
let observer: IntersectionObserver | null = null

onMounted(() => {
  if (!props.scrollSpy) return
  const root = props.inline ? null : getScrollContainer()
  observer = new IntersectionObserver(
    (entries) => {
      for (const e of entries) {
        if (e.isIntersecting) {
          model.value = e.target.id
          break
        }
      }
    },
    {
      root,
      rootMargin: props.inline ? '-10% 0px -80% 0px' : '-15% 0px -70% 0px',
      threshold: 0,
    },
  )
  props.sections.forEach(({ id }) => {
    const el = document.getElementById(id)
    if (el) observer!.observe(el)
  })
})

onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <nav
    ref="navEl"
    class="wx-anchor-bar"
    :class="{ 'wx-anchor-bar--inline': inline }"
    :style="!inline ? { top: `${stickyTop}px` } : undefined"
    aria-label="điều hướng trong trang"
  >
    <button
      v-for="s in sections"
      :key="s.id"
      type="button"
      class="wx-anchor-bar__item"
      :class="{ 'wx-anchor-bar__item--active': model === s.id }"
      :aria-current="model === s.id ? 'location' : undefined"
      @click="jump(s.id)"
    >
      {{ s.label }}
    </button>
  </nav>
</template>

<style scoped src="./AnchorBar.scoped.css"></style>
