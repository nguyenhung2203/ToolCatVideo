<script setup lang="ts">
// Ảnh xem trước của thẻ clip, chỉ tải khi thẻ lọt vào tầm nhìn.
//
// Thay cho <video preload="auto"> trỏ vào file gốc: với video 1 tiếng và 40 thẻ,
// cách cũ dựng 40 bộ giải mã cùng lúc trong WebView2 (~5 GB RAM → renderer chết).
// Ở đây mỗi thẻ chỉ xin 1 ảnh JPG qua hàng đợi có chặn trần, và chỉ xin khi
// thật sự hiện ra trên màn hình — cuộn tới đâu tải tới đó.
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { Film, Loader2 } from 'lucide-vue-next'
import { requestThumb } from '../../utils/thumbQueue'

const props = defineProps<{
  /** Ảnh đã có sẵn (thumbnail/coverPath của clip). Có thì dùng luôn, không trích lại. */
  src?: string
  /** Video nguồn để trích khung hình khi chưa có ảnh sẵn. */
  videoPath?: string
  /** Mốc giây cần trích trong video nguồn. */
  timeSec?: number
  /** Đổi đường dẫn ảnh trên đĩa thành URL phát được trong WebView (dùng GetStreamURL). */
  toUrl: (p: string) => string
}>()

const emit = defineEmits<{ (e: 'error', path: string): void }>()

const rootRef = ref<HTMLElement | null>(null)
const resolvedPath = ref('')
const loading = ref(false)
const visible = ref(false)
let observer: IntersectionObserver | null = null

const load = async () => {
  if (props.src) {
    resolvedPath.value = props.src
    return
  }
  if (!props.videoPath || resolvedPath.value || loading.value) return
  loading.value = true
  try {
    resolvedPath.value = await requestThumb(props.videoPath, props.timeSec ?? 0.1)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (!rootRef.value) return
  // rootMargin 300px: tải sẵn khoảng 1 hàng thẻ trước khi người dùng cuộn tới,
  // để ảnh kịp hiện mà vẫn không dựng đồng loạt cả trăm thẻ ngoài tầm nhìn.
  observer = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue
        visible.value = true
        load()
        // Đã tải rồi thì thôi theo dõi nữa — ảnh giữ trong cache, không cần tải lại.
        observer?.disconnect()
        observer = null
      }
    },
    { rootMargin: '300px 0px' }
  )
  observer.observe(rootRef.value)
})

onUnmounted(() => {
  observer?.disconnect()
  observer = null
})

// Người dùng sửa mốc bắt đầu / clip được xuất xong → xin lại ảnh ở mốc mới.
watch(
  () => [props.src, props.videoPath, props.timeSec],
  () => {
    resolvedPath.value = ''
    if (visible.value) load()
  }
)

const onImgError = () => {
  emit('error', resolvedPath.value)
  resolvedPath.value = ''
}
</script>

<template>
  <div ref="rootRef" class="lazy-thumb-root">
    <img
      v-if="resolvedPath && toUrl(resolvedPath)"
      :src="toUrl(resolvedPath)"
      alt=""
      decoding="async"
      @error="onImgError"
    />
    <div v-else-if="loading" class="lazy-thumb-state">
      <Loader2 :size="18" class="lazy-thumb-spin" />
    </div>
    <div v-else class="lazy-thumb-state">
      <Film :size="24" />
    </div>
  </div>
</template>

<style scoped>
.lazy-thumb-root {
  width: 100%;
  height: 100%;
}

.lazy-thumb-root img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.lazy-thumb-state {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  background: rgba(0, 0, 0, 0.3);
  color: var(--l-text-muted, #94a3b8);
  opacity: 0.5;
}

.lazy-thumb-spin {
  animation: lazy-thumb-rotate 0.9s linear infinite;
}

@keyframes lazy-thumb-rotate {
  to {
    transform: rotate(360deg);
  }
}
</style>
