<script setup lang="ts">
import { onMounted, ref, reactive } from 'vue'
import { useImageDownloader, IMAGE_SOURCES } from './composables/useImageDownloader'
import {
  Image as ImageIcon, Search, Download, Square,
  FolderOpen, Check, Key, Loader2, Trash2, Globe, Palette, Camera
} from 'lucide-vue-next'

const props = defineProps<{
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void
}>()

const emit = defineEmits<{
  (e: 'back'): void
}>()

const {
  imageQuery, imageSource, imageApiKey, imageMaxCount,
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

    <!-- ── Header ────────────────────────────────────── -->
    <div class="img-page-header">
      <h2 class="img-page-title">
        <ImageIcon :size="20" class="img-title-icon" />
        Tải Ảnh Theo Chủ Đề
        <span class="img-title-sub">Làm Ảnh Bìa / Hình Nền AI</span>
      </h2>
    </div>

    <!-- ── Scrollable body ──────────────────────────────── -->
    <div class="img-body">

    <!-- ── Search input ───────────────────────────────── -->
    <div class="img-search-section">
      <label class="img-label">Nhập chủ đề / từ khóa:</label>
      <div class="img-input-row">
        <input
          v-model="imageQuery"
          type="text"
          class="img-text-input"
          placeholder="VD: mèo cute, phong cảnh Việt Nam, ẩm thực đường phố, nature..."
          @keyup.enter="searchImages"
          :disabled="isImageSearching || isImageDownloading"
        />
        <button
          @click="searchImages"
          class="btn img-search-btn"
          :disabled="isImageSearching || isImageDownloading || !imageQuery.trim()"
        >
          <Loader2 v-if="isImageSearching" :size="14" class="spin-hourglass" />
          <Search v-else :size="14" />
          {{ isImageSearching ? 'Đang tìm...' : 'Tìm Ảnh' }}
        </button>
      </div>
    </div>

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
            <Globe v-if="src.value === 'duckduckgo'" :size="12" />
            <Palette v-else-if="src.value === 'pixabay'" :size="12" />
            <Camera v-else-if="src.value === 'unsplash'" :size="12" />
            <ImageIcon v-else-if="src.value === 'pexels'" :size="12" />
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

    <!-- ── API Key row ─────────────────────────────────── -->
    <div class="img-apikey-row" v-if="sourceNeedsKey">
      <Key :size="14" class="img-key-icon" />
      <label class="img-apikey-label">{{ imageSelectedSource?.label }} API Key:</label>
      <div class="img-apikey-input-wrap">
        <input
          v-model="imageApiKey"
          type="password"
          class="img-text-input img-apikey-input"
          :placeholder="`Nhập ${imageSelectedSource?.label} API Key...`"
        />
        <span v-if="imageHasEnvKey" class="img-env-badge" style="display: inline-flex; align-items: center; gap: 3px;">
          <Check :size="11" />
          .env
        </span>
      </div>
      <a
        :href="imageSource==='pixabay'?'https://pixabay.com/api/docs/':imageSource==='unsplash'?'https://unsplash.com/developers':'https://www.pexels.com/api/'"
        target="_blank"
        class="img-getkey-link"
      >Lấy key miễn phí →</a>
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

    <!-- ── Log strip ──────────────────────────────────── -->
    <div v-if="imageSearchLog.length > 0" class="img-log-strip">
      {{ imageSearchLog[0] }}
    </div>

    </div> <!-- end img-body -->

    <!-- ── Footer ─────────────────────────────────────── -->
    <div class="img-footer">
      <!-- Save folder -->
      <div class="img-dir-row">
        <FolderOpen :size="14" class="img-dir-icon" />
        <span class="img-dir-label">Lưu vào:</span>
        <input v-model="imageDir" class="img-text-input img-dir-input" readonly placeholder="Thư mục lưu ảnh..." />
        <button @click="pickImageDir" class="btn img-dir-btn">
          <FolderOpen :size="14" />
        </button>
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
        <button v-if="isImageDownloading" @click="cancelImageDl" class="btn stop-analyze-btn img-action-btn">
          <Square :size="12" /> Dừng tải
        </button>
        <button
          v-if="!isImageDownloading && imageSelectedCount > 0"
          @click="removeSelectedImages"
          class="btn img-remove-btn img-action-btn"
          :title="`Xóa ${imageSelectedCount} ảnh đã chọn khỏi danh sách`"
        >
          <Trash2 :size="13" />
          Xóa {{ imageSelectedCount }} ảnh đã chọn
        </button>
        <button
          @click="startImageDownload"
          :disabled="isImageDownloading || isImageSearching || imageSelectedCount === 0"
          class="btn btn-analyze img-action-btn"
        >
          <Download :size="15" />
          <span v-if="isImageDownloading">Đang tải {{ downloadDoneCount }}/{{ downloadTotalCount }}...</span>
          <span v-else-if="imageSelectedCount === 0">Chọn ảnh để tải</span>
          <span v-else>Tải {{ imageSelectedCount }} ảnh đã chọn</span>
        </button>
      </div>
    </div>

  </div>
</template>

<style scoped>
/* ── Page shell ───────────────────────────────────────────── */
.img-page {
  background: var(--wx-surface-base);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-lg);
  padding: var(--wx-space-4);
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-4);
  box-shadow: var(--wx-shadow-md);
  flex: 1;
  overflow: hidden; /* prevent outer scroll */
  min-height: 0;
}

/* ── Header ────────────────────────────────────────────────── */
.img-page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid var(--wx-border-default);
  padding-bottom: var(--wx-space-3);
  flex-shrink: 0;
}

/* ── Scrollable body ────────────────────────────────────── */
.img-body {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-4);
  min-height: 0;
  padding-right: 2px;
}

.img-body::-webkit-scrollbar { width: 5px; }
.img-body::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--wx-text-primary) 10%, transparent);
  border-radius: var(--wx-radius-full);
}


.img-page-title {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  margin: 0;
  font-size: var(--wx-fs-18);
  font-weight: var(--wx-fw-bold);
  color: var(--wx-text-primary);
}

.img-title-icon { color: var(--wx-brand-primary); }

.img-title-sub {
  font-size: var(--wx-fs-12);
  font-weight: var(--wx-fw-regular);
  color: var(--wx-text-muted);
  margin-left: var(--wx-space-2);
}

.img-back-btn {
  padding: var(--wx-space-1) var(--wx-space-3);
  font-size: var(--wx-fs-12);
  border-radius: var(--wx-radius-md);
}

/* ── Common label ──────────────────────────────────────────── */
.img-label {
  display: block;
  font-size: var(--wx-fs-12);
  font-weight: var(--wx-fw-bold);
  color: var(--wx-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-bottom: var(--wx-space-2);
}

/* ── Search section ────────────────────────────────────────── */
.img-search-section {
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-1);
}

.img-input-row {
  display: flex;
  gap: var(--wx-space-2);
  align-items: stretch;
}

/* ── Common text input ─────────────────────────────────────── */
.img-text-input {
  background: var(--wx-surface-sunken, #0d1929);
  border: 1.5px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
  color: var(--wx-text-primary);
  padding: var(--wx-space-2) var(--wx-space-3);
  font-size: var(--wx-fs-13);
  height: 38px;
  box-sizing: border-box;
  outline: none;
  transition: border-color 150ms ease, box-shadow 150ms ease;
  flex: 1;
}

.img-text-input::placeholder {
  color: var(--wx-text-muted);
  opacity: 0.6;
}

.img-text-input:focus {
  border-color: var(--wx-brand-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--wx-brand-primary) 15%, transparent);
}

.img-text-input:disabled { opacity: 0.5; cursor: not-allowed; }

/* ── Search button ─────────────────────────────────────────── */
.img-search-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
  white-space: nowrap;
  height: 38px;
  padding: 0 var(--wx-space-4);
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-bold);
  background: var(--wx-brand-primary);
  color: var(--wx-text-inverse);
  border: none;
  border-radius: var(--wx-radius-md);
  cursor: pointer;
  transition: all 150ms ease;
}

.img-search-btn:hover:not(:disabled) {
  filter: brightness(1.1);
  transform: translateY(-1px);
}

.img-search-btn:disabled { opacity: 0.45; cursor: not-allowed; }

/* ── Config row ────────────────────────────────────────────── */
.img-config-row {
  display: flex;
  gap: var(--wx-space-4);
  align-items: flex-start;
  flex-wrap: wrap;
}

.img-sources-col { flex: 1; min-width: 220px; }
.img-count-col  { flex-shrink: 0; }

/* Source pills */
.img-source-pills {
  display: flex;
  gap: var(--wx-space-1);
  flex-wrap: wrap;
}

.img-source-pill {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
  font-size: var(--wx-fs-12);
  padding: var(--wx-space-1) var(--wx-space-3);
  border-radius: var(--wx-radius-md);
  border: 1px solid var(--wx-border-default);
  background: color-mix(in srgb, var(--wx-surface-base) 2%, transparent);
  color: var(--wx-text-muted);
  font-weight: var(--wx-fw-semibold);
  cursor: pointer;
  transition: all 150ms ease;
  white-space: nowrap;
}

.img-source-pill:hover { color: var(--wx-text-primary); }

.img-source-pill.active {
  background: color-mix(in srgb, var(--wx-brand-primary) 12%, transparent);
  border-color: var(--wx-brand-primary);
  color: var(--wx-brand-primary);
}

.img-key-badge {
  font-size: 9px;
  padding: 1px 5px;
  background: color-mix(in srgb, #f59e0b 20%, transparent);
  color: #fbbf24;
  border-radius: var(--wx-radius-sm);
  border: 1px solid color-mix(in srgb, #f59e0b 35%, transparent);
}

.img-source-hint {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
  margin: var(--wx-space-1) 0 0;
  font-style: italic;
  line-height: 1.4;
}

/* Count select */
.img-select {
  background: var(--wx-surface-base);
  border: 1.5px solid var(--wx-border-default);
  color: var(--wx-text-primary);
  padding: var(--wx-space-2) var(--wx-space-3);
  border-radius: var(--wx-radius-md);
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-semibold);
  outline: none;
  cursor: pointer;
  height: 38px;
  transition: border-color 150ms ease;
}

.img-select:focus { border-color: var(--wx-brand-primary); }

/* ── API Key row ────────────────────────────────────────────── */
.img-apikey-row {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  flex-wrap: wrap;
  padding: var(--wx-space-2) var(--wx-space-3);
  background: color-mix(in srgb, #f59e0b 5%, transparent);
  border: 1px solid color-mix(in srgb, #f59e0b 20%, transparent);
  border-radius: var(--wx-radius-md);
}

.img-key-icon { color: #fbbf24; flex-shrink: 0; }

.img-apikey-label {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
  font-weight: var(--wx-fw-bold);
  white-space: nowrap;
  flex-shrink: 0;
}

.img-apikey-input-wrap {
  flex: 1;
  min-width: 160px;
  display: flex;
  align-items: center;
  position: relative;
}

.img-apikey-input {
  width: 100%;
  height: 34px;
  font-size: var(--wx-fs-12);
}

.img-env-badge {
  position: absolute;
  right: var(--wx-space-2);
  top: 50%;
  transform: translateY(-50%);
  font-size: 10px;
  font-weight: var(--wx-fw-bold);
  padding: 2px var(--wx-space-2);
  border-radius: var(--wx-radius-sm);
  background: color-mix(in srgb, #10b981 15%, transparent);
  color: #10b981;
  border: 1px solid color-mix(in srgb, #10b981 30%, transparent);
  pointer-events: none;
}

.img-getkey-link {
  font-size: var(--wx-fs-12);
  font-weight: var(--wx-fw-bold);
  color: var(--wx-brand-primary);
  text-decoration: none;
  white-space: nowrap;
  flex-shrink: 0;
  transition: opacity 150ms ease;
}

.img-getkey-link:hover { opacity: 0.8; }

/* ── Results area ───────────────────────────────────────────── */
.img-results-area {
  flex: 1;
  min-height: 200px;
}

/* Empty state */
.img-empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wx-space-3);
  height: 200px;
  color: var(--wx-text-muted);
  text-align: center;
}

.img-empty-icon { opacity: 0.2; }

.img-empty-title {
  font-size: var(--wx-fs-14);
  font-weight: var(--wx-fw-semibold);
  margin: 0;
  color: var(--wx-text-primary);
}

.img-empty-sub {
  font-size: var(--wx-fs-12);
  opacity: 0.55;
  margin: 0;
}

.img-accent { color: var(--wx-brand-primary); }

/* Loading state */
.img-loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wx-space-4);
  height: 200px;
  color: var(--wx-text-muted);
}

.img-spinner {
  width: 40px;
  height: 40px;
  border: 3px solid color-mix(in srgb, var(--wx-text-primary) 10%, transparent);
  border-top-color: var(--wx-brand-primary);
  border-radius: 50%;
  animation: spin 0.9s linear infinite;
}

@keyframes spin { to { transform: rotate(360deg); } }

.img-loading-text {
  font-size: var(--wx-fs-14);
  font-weight: var(--wx-fw-semibold);
  margin: 0;
}

/* Results header */
.img-results-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: var(--wx-space-3);
}

.img-results-count {
  font-size: var(--wx-fs-13);
  color: var(--wx-text-muted);
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
}

.img-results-count strong {
  color: var(--wx-text-primary);
  font-size: var(--wx-fs-18);
}

.img-source-tag {
  font-size: 10px;
  font-weight: var(--wx-fw-bold);
  padding: 2px var(--wx-space-2);
  border-radius: var(--wx-radius-full);
  background: color-mix(in srgb, var(--wx-brand-primary) 12%, transparent);
  color: var(--wx-brand-primary);
  text-transform: capitalize;
}

.img-toggle-all-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
  font-size: var(--wx-fs-12);
  padding: var(--wx-space-1) var(--wx-space-3);
  border-radius: var(--wx-radius-md);
  border: 1px solid var(--wx-border-default);
  background: color-mix(in srgb, var(--wx-surface-base) 4%, transparent);
  color: var(--wx-text-muted);
  font-weight: var(--wx-fw-semibold);
  cursor: pointer;
  transition: all 150ms ease;
}

.img-toggle-all-btn:hover {
  border-color: var(--wx-brand-primary);
  color: var(--wx-brand-primary);
}

.img-count-badge {
  font-weight: var(--wx-fw-bold);
  color: var(--wx-text-primary);
}

/* Image grid */
.img-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: var(--wx-space-2);
  max-height: 400px;
  overflow-y: auto;
  padding: 4px;
}

.img-grid::-webkit-scrollbar { width: 5px; }
.img-grid::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--wx-text-primary) 10%, transparent);
  border-radius: var(--wx-radius-full);
}

/* Drag-select rubber band box */
.img-drag-box {
  position: absolute;
  background: color-mix(in srgb, var(--wx-brand-primary) 15%, transparent);
  border: 1.5px solid var(--wx-brand-primary);
  border-radius: var(--wx-radius-sm);
  pointer-events: none;
  z-index: 10;
}

.img-card {
  border-radius: var(--wx-radius-md);
  overflow: hidden;
  cursor: pointer;
  border: 2px solid var(--wx-border-default);
  background: color-mix(in srgb, var(--wx-surface-base) 3%, transparent);
  transition: all 150ms ease;
}

.img-card:hover { border-color: var(--wx-border-subtle); }

.img-card.selected {
  border-color: var(--wx-brand-primary);
  transform: translateY(-2px);
  box-shadow: 0 6px 20px color-mix(in srgb, var(--wx-brand-primary) 20%, transparent);
}

.img-card-thumb {
  position: relative;
  aspect-ratio: 4/3;
  overflow: hidden;
  background: rgba(0,0,0,.3);
}

.img-card-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  transition: transform 250ms ease;
}

.img-card:hover .img-card-img { transform: scale(1.06); }

.img-card-check {
  position: absolute;
  top: 6px;
  right: 6px;
  width: 22px;
  height: 22px;
  border-radius: var(--wx-radius-full);
  background: var(--wx-brand-primary);
  color: var(--wx-text-inverse);
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(0,0,0,.5);
}

.img-card-author {
  padding: 5px var(--wx-space-2);
  font-size: 10px;
  color: var(--wx-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ── Log strip ─────────────────────────────────────────────── */
.img-log-strip {
  padding: var(--wx-space-1) 0;
  border-top: 1px solid var(--wx-border-default);
  font-size: var(--wx-fs-12);
  line-height: 1.4;
  color: var(--wx-text-muted);
  font-family: var(--wx-font-mono, monospace);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 0;
}

/* ── Footer ─────────────────────────────────────────────────── */
.img-footer {
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  gap: var(--wx-space-3);
  flex-wrap: wrap;
  padding-top: var(--wx-space-3);
  border-top: 1px solid var(--wx-border-default);
  flex-shrink: 0; /* pin to bottom */
}


/* Directory row */
.img-dir-row {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  flex: 1;
  min-width: 0;
}

.img-dir-icon { color: var(--wx-text-muted); flex-shrink: 0; }

.img-dir-label {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
  font-weight: var(--wx-fw-semibold);
  white-space: nowrap;
  flex-shrink: 0;
}

.img-dir-input {
  flex: 1;
  height: 34px;
  font-size: var(--wx-fs-12);
  cursor: default;
}

.img-dir-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: var(--wx-space-1) var(--wx-space-2);
  height: 34px;
  flex-shrink: 0;
  background: color-mix(in srgb, var(--wx-surface-base) 5%, transparent);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
  color: var(--wx-text-primary);
  cursor: pointer;
  transition: all 150ms ease;
}

.img-dir-btn:hover {
  border-color: var(--wx-brand-primary);
  color: var(--wx-brand-primary);
}

/* Progress row */
.img-progress-row {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  flex: 1;
  min-width: 0;
}

.img-progress-track {
  flex: 1;
  height: 5px;
  background: color-mix(in srgb, var(--wx-text-primary) 8%, transparent);
  border-radius: var(--wx-radius-full);
  overflow: hidden;
}

.img-progress-fill {
  height: 100%;
  background: var(--wx-brand-primary);
  border-radius: var(--wx-radius-full);
  transition: width 400ms ease;
}

.img-progress-label {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
  font-weight: var(--wx-fw-bold);
  white-space: nowrap;
}

/* Footer actions */
.img-footer-actions {
  display: flex;
  gap: var(--wx-space-2);
  flex-shrink: 0;
}

.img-action-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
  justify-content: center;
  height: 38px;
  padding: 0 var(--wx-space-5);
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-bold);
}

.img-remove-btn {
  background: var(--wx-danger-solid);
  color: var(--wx-text-inverse);
  border: none;
}

.img-remove-btn:hover {
  filter: brightness(1.1);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px color-mix(in srgb, var(--wx-danger-solid) 35%, transparent);
}

.spin-hourglass {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
