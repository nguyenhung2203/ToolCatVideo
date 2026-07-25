<script setup lang="ts">
import { onMounted, ref, reactive } from 'vue'
import { useImageDownloader, IMAGE_SOURCES } from './composables/useImageDownloader'
import {
  Image as ImageIcon, Search, Download, Square,
  FolderOpen, Check, Key, Loader2, Trash2, Globe, Palette, Camera, SlidersHorizontal
} from 'lucide-vue-next'
import BaseButton from './common/BaseButton.vue'

const props = defineProps<{
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void
}>()

const emit = defineEmits<{
  (e: 'back'): void
}>()

const {
  imageQuery, imageSource, imageApiKey, imageMaxCount, showFilters,
  isSearching: isImageSearching, isDownloading: isImageDownloading, searchResult: imageSearchResult,
  imageDir, selectedImageIds, progressMap: imageProgressMap, searchLog: imageSearchLog,
  downloadDoneCount, downloadTotalCount, downloadProgress: imageDownloadProgress,
  selectedCount: imageSelectedCount, allEntries: imageAllEntries,
  sourceNeedsKey, selectedSource: imageSelectedSource, hasEnvKey: imageHasEnvKey,
  toggleSelectAll: imageToggleAll, toggleImage, openImagePanel,
  searchImages, startImageDownload, cancelImageDl, pickImageDir, initImageEvents,
  removeSelectedImages,
} = useImageDownloader(props.showToast)

onMounted(async () => {
  await openImagePanel()
  initImageEvents()
})

// ── Drag-to-select ─────────────────────────────────────────────
const gridRef = ref<HTMLElement | null>(null)

const dragBox = reactive({ active: false, x1: 0, y1: 0, x2: 0, y2: 0 })

const dragBoxStyle = ref<Record<string, string>>({})

let dragStartSelectedIds = new Set<string>()
let dragMode: 'select' | 'deselect' | null = null

function onGridMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  const target = e.target as HTMLElement
  // Only block actual interactive elements (buttons, inputs, links)
  // Allow dragging to start from image cards AND inside images
  if (target.closest('button, input, a, select')) return

  const grid = gridRef.value
  if (!grid) return

  dragMode = null  // determined on first card intersection
  dragStartSelectedIds = new Set(selectedImageIds.value)

  const rect = grid.getBoundingClientRect()
  dragBox.x1 = e.clientX - rect.left + grid.scrollLeft
  dragBox.y1 = e.clientY - rect.top + grid.scrollTop
  dragBox.x2 = dragBox.x1
  dragBox.y2 = dragBox.y1
  dragBox.active = false

  const onMove = (me: MouseEvent) => {
    dragBox.x2 = me.clientX - rect.left + grid.scrollLeft
    dragBox.y2 = me.clientY - rect.top + grid.scrollTop
    dragBox.active = true
    updateDragBoxStyle()
    updateSelectionFromDrag()
    me.preventDefault()
  }

  const onUp = () => {
    dragBox.active = false
    dragBoxStyle.value = {}
    dragMode = null
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }

  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

function updateDragBoxStyle() {
  const x = Math.min(dragBox.x1, dragBox.x2)
  const y = Math.min(dragBox.y1, dragBox.y2)
  const w = Math.abs(dragBox.x2 - dragBox.x1)
  const h = Math.abs(dragBox.y2 - dragBox.y1)
  dragBoxStyle.value = {
    left: x + 'px', top: y + 'px',
    width: w + 'px', height: h + 'px',
    display: w < 4 && h < 4 ? 'none' : 'block',
  }
}

function updateSelectionFromDrag() {
  const grid = gridRef.value
  if (!grid) return

  const selX1 = Math.min(dragBox.x1, dragBox.x2)
  const selY1 = Math.min(dragBox.y1, dragBox.y2)
  const selX2 = Math.max(dragBox.x1, dragBox.x2)
  const selY2 = Math.max(dragBox.y1, dragBox.y2)

  if (selX2 - selX1 < 4 && selY2 - selY1 < 4) return

  const gridRect = grid.getBoundingClientRect()
  const ids = selectedImageIds.value

  const cards = grid.querySelectorAll<HTMLElement>('[data-img-id]')
  cards.forEach(card => {
    const cr = card.getBoundingClientRect()
    const cardX1 = cr.left - gridRect.left + grid.scrollLeft
    const cardY1 = cr.top - gridRect.top + grid.scrollTop
    const cardX2 = cardX1 + cr.width
    const cardY2 = cardY1 + cr.height

    const overlaps = cardX1 < selX2 && cardX2 > selX1 && cardY1 < selY2 && cardY2 > selY1
    const id = card.dataset.imgId!

    if (overlaps) {
      // Determine mode from the FIRST card we touch
      if (dragMode === null) {
        dragMode = dragStartSelectedIds.has(id) ? 'deselect' : 'select'
      }

      if (dragMode === 'select' && !ids.has(id)) toggleImage(id)
      if (dragMode === 'deselect' && ids.has(id)) toggleImage(id)
    } else {
      // Restore card to its pre-drag state
      const wasSelected = dragStartSelectedIds.has(id)
      const isSelected = ids.has(id)
      if (wasSelected && !isSelected) toggleImage(id)   // re-select it
      if (!wasSelected && isSelected) toggleImage(id)   // un-select it
    }
  })
}

</script>


<template>
  <div class="img-page">

    <!-- ── Scrollable body ──────────────────────────────── -->
    <div class="img-body">

    <!-- ── Search input ───────────────────────────────── -->
    <div class="img-search-section">
      <label class="img-label">NHẬP CHỦ ĐỀ / TỪ KHÓA:</label>
      <div class="img-input-row">
        <input
          v-model="imageQuery"
          type="text"
          class="img-text-input"
          placeholder="VD: mèo cute, phong cảnh Việt Nam, ẩm thực đường phố, nature..."
          @keyup.enter="searchImages"
          :disabled="isImageSearching || isImageDownloading"
        />
        <BaseButton
          variant="primary"
          size="md"
          :loading="isImageSearching"
          :disabled="isImageSearching || isImageDownloading || !imageQuery.trim()"
          @click="searchImages"
        >
          <Search v-if="!isImageSearching" :size="14" />
          {{ isImageSearching ? 'Đang tìm...' : 'Tìm Ảnh' }}
        </BaseButton>
        <button
          type="button"
          @click="showFilters = !showFilters"
          class="btn img-filter-toggle-btn"
          :class="{ active: showFilters }"
          title="Bộ lọc nâng cao"
        >
          <SlidersHorizontal :size="14" />
          <span>Bộ lọc</span>
        </button>
      </div>
    </div>

    <!-- ── Advanced settings collapsible ────────────────── -->
    <div v-show="showFilters" class="img-advanced-settings">
      <!-- ── Source & Count row ─────────────────────────── -->
      <div class="img-config-row">
        <!-- Source pills -->
        <div class="img-sources-col">
          <label class="img-label">Nguồn ảnh:</label>
          <div class="img-source-pills">
            <button
              v-for="src in IMAGE_SOURCES"
              :key="src.value"
              type="button"
              :title="src.hint"
              class="img-source-pill"
              :class="{ active: imageSource === src.value }"
              @click="imageSource = src.value"
            >
              {{ src.label }}
              <span v-if="src.needsKey" class="img-key-badge">Key</span>
            </button>
          </div>
          <p v-if="imageSelectedSource" class="img-source-hint">{{ imageSelectedSource.hint }}</p>
        </div>

        <!-- Count select -->
        <div class="img-count-col">
          <label class="img-label">Số lượng:</label>
          <select v-model="imageMaxCount" class="img-select">
            <option :value="20">20 ảnh</option>
            <option :value="50">50 ảnh</option>
            <option :value="100">100 ảnh</option>
            <option :value="200">200 ảnh</option>
            <option :value="500">500 ảnh</option>
            <option :value="1000">1000 ảnh</option>
          </select>
        </div>
      </div>

      <!-- ── API Key / Cookie row ─────────────────────────────────── -->
      <div class="img-apikey-row" v-if="sourceNeedsKey">
        <Key :size="14" class="img-key-icon" />
        <label class="img-apikey-label">
          {{ imageSource === 'pinterest' ? 'Pinterest Cookie (Tùy chọn):' : `${imageSelectedSource?.label} API Key:` }}
        </label>
        <div class="img-apikey-input-wrap">
          <input
            v-model="imageApiKey"
            :type="imageSource === 'pinterest' ? 'text' : 'password'"
            class="img-text-input img-apikey-input"
            :placeholder="imageSource === 'pinterest' ? 'Paste Cookie từ trình duyệt (csrftoken=...; _pinterest_sess=...)...' : `Nhập ${imageSelectedSource?.label} API Key...`"
          />
          <span v-if="imageHasEnvKey" class="img-env-badge" style="display: inline-flex; align-items: center; gap: 3px;">
            <Check :size="11" />
            .env
          </span>
        </div>
        <a
          v-if="imageSource !== 'pinterest'"
          :href="imageSource==='pixabay'?'https://pixabay.com/api/docs/':imageSource==='unsplash'?'https://unsplash.com/developers':'https://www.pexels.com/api/'"
          target="_blank"
          class="img-getkey-link"
        >Lấy key miễn phí →</a>
      </div>
    </div>

    <!-- ── Results area ───────────────────────────────── -->
    <div class="img-results-area">

      <!-- Empty state -->
      <div v-if="!imageSearchResult && !isImageSearching" class="img-empty-state">
        <ImageIcon :size="56" class="img-empty-icon" />
        <p class="img-empty-title">Nhập chủ đề → nhấn <span class="img-accent">Tìm Ảnh</span> để bắt đầu</p>
        <p class="img-empty-sub">DuckDuckGo miễn phí không cần key • Hỗ trợ tiếng Việt &amp; tiếng Anh</p>
      </div>

      <!-- Loading state -->
      <div v-if="isImageSearching" class="img-loading-state">
        <div class="img-spinner"></div>
        <p class="img-loading-text">Đang tìm kiếm từ <span class="img-accent">{{ imageSelectedSource?.label }}</span>...</p>
      </div>

      <!-- Results -->
      <template v-if="imageSearchResult && !isImageSearching">
        <!-- Results header -->
        <div class="img-results-header">
          <span class="img-results-count">
            <strong>{{ imageAllEntries.length }}</strong> ảnh tìm thấy
            <span class="img-source-tag">{{ imageSearchResult.source }}</span>
          </span>
          <button type="button" @click="imageToggleAll" class="img-toggle-all-btn">
            {{ imageSelectedCount === imageAllEntries.length ? 'Bỏ tất cả' : 'Chọn tất cả' }}
            <span class="img-count-badge">({{ imageSelectedCount }}/{{ imageAllEntries.length }})</span>
          </button>
        </div>

        <!-- Grid -->
        <div
          ref="gridRef"
          class="img-grid"
          @mousedown.left="onGridMouseDown"
          style="position: relative; user-select: none;"
        >
          <!-- Drag-select overlay box -->
          <div v-if="dragBox.active" class="img-drag-box" :style="dragBoxStyle"></div>

          <div
            v-for="entry in imageAllEntries"
            :key="entry.id"
            class="img-card"
            :class="{ selected: selectedImageIds.has(entry.id) }"
            :data-img-id="entry.id"
            @click="toggleImage(entry.id)"
          >
            <div class="img-card-thumb">
              <img
                :src="entry.thumbUrl || entry.url"
                :alt="entry.title || 'anh'"
                loading="lazy"
                class="img-card-img"
                draggable="false"
                @error="($event.target as HTMLImageElement).style.display='none'"
              />
              <div v-if="selectedImageIds.has(entry.id)" class="img-card-check">
                <Check :size="13" />
              </div>
            </div>
            <div v-if="entry.author" class="img-card-author">by {{ entry.author }}</div>
          </div>
        </div>

      </template>
    </div>

    </div> <!-- end img-body -->

    <!-- ── Footer ─────────────────────────────────────── -->
    <div class="img-footer">
      <!-- Save folder -->
      <div class="img-dir-row">
        <span class="img-dir-label">Lưu vào:</span>
        <input v-model="imageDir" class="img-text-input img-dir-input" readonly placeholder="Thư mục lưu ảnh..." />
        <BaseButton variant="secondary" size="icon" @click="pickImageDir">
          <FolderOpen :size="14" />
        </BaseButton>
      </div>

      <!-- Download progress bar -->
      <div v-if="isImageDownloading" class="img-progress-row">
        <div class="img-progress-track">
          <div class="img-progress-fill" :style="{ width: imageDownloadProgress + '%' }"></div>
        </div>
        <span class="img-progress-label">{{ downloadDoneCount }}/{{ downloadTotalCount }} ảnh</span>
      </div>

      <!-- Actions -->
      <div class="img-footer-actions">
        <BaseButton v-if="isImageDownloading" variant="danger" size="md" @click="cancelImageDl">
          <Square :size="12" /> Dừng tải
        </BaseButton>
        <BaseButton
          v-if="!isImageDownloading && imageSelectedCount > 0"
          variant="danger"
          size="md"
          :title="`Xóa ${imageSelectedCount} ảnh đã chọn khỏi danh sách`"
          @click="removeSelectedImages"
        >
          <Trash2 :size="13" />
          Xóa {{ imageSelectedCount }} ảnh đã chọn
        </BaseButton>
        <BaseButton
          variant="primary"
          size="md"
          :disabled="isImageDownloading || isImageSearching || imageSelectedCount === 0"
          @click="startImageDownload"
        >
          <Download :size="15" />
          <span v-if="isImageDownloading">Đang tải {{ downloadDoneCount }}/{{ downloadTotalCount }}...</span>
          <span v-else-if="imageSelectedCount === 0">Chọn ảnh để tải</span>
          <span v-else>Tải {{ imageSelectedCount }} ảnh đã chọn</span>
        </BaseButton>
      </div>
    </div>

  </div>
</template>

<style scoped src="./ImageDownloader.scoped.css"></style>

