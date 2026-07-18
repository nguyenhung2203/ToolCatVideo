<script setup lang="ts">
import { onMounted, toRef, ref, reactive } from 'vue'
import { useDownloader } from './composables/useDownloader'
import {
  Globe, Link2, Search, Download, Square, FolderOpen,
  Check, X, Eye, Heart, Clock, Calendar, Copy, Loader2, Trash2, Film
} from 'lucide-vue-next'

const props = defineProps<{
  videoPaths: string[]
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void
}>()

const emit = defineEmits<{
  (e: 'back'): void
}>()

const videoPathsRef = toRef(props, 'videoPaths')

const {
  downloadUrl, downloadMode, isProbing, isDownloading, probeResult,
  downloadDir, cookieBrowser, dlSelectedIds, dlSortBy, dlKeyword,
  dlMinViews, dlProgressMap, dlLinkType, dlMaxCount, dlFetchOrder,
  searchSource, filteredDlEntries, dlSelectedCount, detectedLinkType,
  effectiveLinkType, isProfileOrPlaylist, formatViewCount, formatDlDate,
  dlToggleAll, dlToggle, openDownloadPanel, probeUrl, startDownload,
  cancelDl, pickDownloadDir, initDownloadEvents, copyToClipboard,
  removeSelectedEntries
} = useDownloader(props.showToast, videoPathsRef)

onMounted(async () => {
  await openDownloadPanel()
  initDownloadEvents((msg) => {
    console.log(msg)
  })
})

const formatTime = (seconds: number) => {
  if (seconds === undefined || seconds === null || seconds < 0) return '00:00'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  const pad = (num: number) => String(num).padStart(2, '0')
  if (h > 0) return `${pad(h)}:${pad(m)}:${pad(s)}`
  return `${pad(m)}:${pad(s)}`
}

// ── Drag-to-select ─────────────────────────────────────────────
const listRef = ref<HTMLElement | null>(null)
const dragBox = reactive({ active: false, x1: 0, y1: 0, x2: 0, y2: 0 })
const dragBoxStyle = ref<Record<string, string>>({})
let dragStartSelectedIds = new Set<string>()
let dragMode: 'select' | 'deselect' | null = null

function onListMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  const target = e.target as HTMLElement
  if (target.closest('button, input, a, select')) return
  const list = listRef.value
  if (!list) return

  dragMode = null
  dragStartSelectedIds = new Set(dlSelectedIds.value)

  const rect = list.getBoundingClientRect()
  dragBox.x1 = e.clientX - rect.left + list.scrollLeft
  dragBox.y1 = e.clientY - rect.top + list.scrollTop
  dragBox.x2 = dragBox.x1
  dragBox.y2 = dragBox.y1
  dragBox.active = false

  const onMove = (me: MouseEvent) => {
    dragBox.x2 = me.clientX - rect.left + list.scrollLeft
    dragBox.y2 = me.clientY - rect.top + list.scrollTop
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
  const list = listRef.value
  if (!list) return
  const selX1 = Math.min(dragBox.x1, dragBox.x2)
  const selY1 = Math.min(dragBox.y1, dragBox.y2)
  const selX2 = Math.max(dragBox.x1, dragBox.x2)
  const selY2 = Math.max(dragBox.y1, dragBox.y2)
  if (selX2 - selX1 < 4 && selY2 - selY1 < 4) return

  const listRect = list.getBoundingClientRect()
  const cards = list.querySelectorAll<HTMLElement>('[data-entry-id]')
  cards.forEach(card => {
    const cr = card.getBoundingClientRect()
    const cardX1 = cr.left - listRect.left + list.scrollLeft
    const cardY1 = cr.top - listRect.top + list.scrollTop
    const cardX2 = cardX1 + cr.width
    const cardY2 = cardY1 + cr.height
    const overlaps = cardX1 < selX2 && cardX2 > selX1 && cardY1 < selY2 && cardY2 > selY1
    const id = card.dataset.entryId!
    if (overlaps) {
      if (dragMode === null) dragMode = dragStartSelectedIds.has(id) ? 'deselect' : 'select'
      if (dragMode === 'select' && !dlSelectedIds.value.has(id)) dlToggle(id)
      if (dragMode === 'deselect' && dlSelectedIds.value.has(id)) dlToggle(id)
    } else {
      const wasSelected = dragStartSelectedIds.has(id)
      const isSelected = dlSelectedIds.value.has(id)
      if (wasSelected && !isSelected) dlToggle(id)
      if (!wasSelected && isSelected) dlToggle(id)
    }
  })
}
const failedThumbs = ref<Set<string>>(new Set())
</script>

<template>
  <div class="dl-page">

    <!-- ── Header ──────────────────────────────────── -->
    <div class="dl-page-header">
      <h2 class="dl-page-title">
        <Globe :size="20" class="dl-title-icon" />
        Tải Video Online
        <span class="dl-title-sub">YouTube · TikTok · Facebook · Douyin</span>
      </h2>
    </div>

    <!-- ── Scrollable body ──────────────────────────────── -->
    <div class="dl-body">

    <!-- ── URL / Keyword input (Merged Tab) ──────────────── -->
    <div class="dl-input-section">
      <label class="dl-input-label">
        {{ downloadMode === 'link'
          ? 'NHẬP ĐƯỜNG DẪN LINK NGUỒN CỦA VIDEO / DANH SÁCH / KÊNH:'
          : 'NHẬP TỪ KHÓA / CHỦ ĐỀ TÌM KIẾM ĐA NGUỒN:' }}
      </label>
      <div class="dl-input-row">
        <input
          v-model="downloadUrl"
          type="text"
          class="dl-url-input"
          placeholder="Dán link video (YouTube, TikTok, Facebook...) hoặc nhập từ khóa chủ đề (ví dụ: mèo cute)..."
          @keyup.enter="probeUrl"
          :disabled="isProbing"
        />
        <button @click="probeUrl" class="btn dl-probe-btn" :disabled="isProbing">
          <Loader2 v-if="isProbing" :size="14" class="spin-hourglass" />
          <Link2 v-else-if="downloadMode === 'link'" :size="14" />
          <Search v-else :size="14" />
          {{ isProbing ? (downloadMode === 'link' ? 'Đang dò...' : 'Đang tìm...') : (downloadMode === 'link' ? 'Dò link' : 'Tìm kiếm') }}
        </button>
      </div>
    </div>

    <!-- ── Options row ────────────────────────────── -->
    <div class="dl-options-row">
      <div class="dl-opt-group">
        <label>Loại link:</label>
        <select v-model="dlLinkType" class="dl-select">
          <option value="auto">Tự nhận diện {{ detectedLinkType !== 'auto' ? '(' + (detectedLinkType === 'video' ? 'Video đơn' : detectedLinkType === 'profile' ? 'Profile' : 'Playlist') + ')' : '' }}</option>
          <option value="video">Video đơn</option>
          <option value="profile">Profile / Kênh</option>
          <option value="playlist">Playlist</option>
        </select>
      </div>

      <div class="dl-opt-group dl-search-sources" v-if="downloadMode === 'search'">
        <label>Nguồn:</label>
        <div class="dl-source-pills">
          <button class="dl-source-pill" :class="{ active: searchSource === 'youtube' }" @click="searchSource = 'youtube'">YouTube</button>
          <button class="dl-source-pill" :class="{ active: searchSource === 'tiktok' }" @click="searchSource = 'tiktok'">TikTok</button>
          <button class="dl-source-pill" :class="{ active: searchSource === 'facebook' }" @click="searchSource = 'facebook'">Facebook</button>
          <button class="dl-source-pill" :class="{ active: searchSource === 'all' }" @click="searchSource = 'all'">Hỗn hợp</button>
        </div>
      </div>

      <div class="dl-opt-group">
        <label>Cookie:</label>
        <select v-model="cookieBrowser" class="dl-select">
          <option value="">Không dùng</option>
          <option value="chrome">Chrome</option>
          <option value="edge">Edge</option>
          <option value="firefox">Firefox</option>
        </select>
      </div>

      <div class="dl-opt-group" v-if="probeResult">
        <span class="dl-platform-badge" :class="probeResult.platform">
          {{ probeResult.platform === 'youtube' ? 'YouTube' : probeResult.platform === 'tiktok' ? 'TikTok' : probeResult.platform === 'facebook' ? 'Facebook' : probeResult.platform === 'mixed' ? 'Hỗn hợp' : 'Khác' }}
        </span>
        <span class="dl-type-badge">{{ probeResult.type === 'playlist' ? 'Playlist/Profile' : 'Video đơn' }}</span>
        <span class="dl-count-badge">{{ probeResult.entries.length }} video</span>
      </div>
    </div>

    <!-- ── Profile/Playlist config ────────────────── -->
    <div class="dl-profile-config" v-if="isProfileOrPlaylist && !probeResult">
      <div class="dl-opt-group">
        <label>Số video tối đa:</label>
        <select v-model.number="dlMaxCount" class="dl-select">
          <option :value="10">10 video</option>
          <option :value="20">20 video</option>
          <option :value="30">30 video</option>
          <option :value="50">50 video</option>
          <option :value="100">100 video</option>
          <option :value="200">200 video</option>
          <option :value="0">Tất cả (chậm)</option>
        </select>
      </div>
      <div class="dl-opt-group">
        <label>Thứ tự lấy:</label>
        <select v-model="dlFetchOrder" class="dl-select">
          <option value="newest">Mới nhất trước</option>
          <option value="oldest">Cũ nhất trước</option>
        </select>
      </div>
      <p class="dl-profile-hint">
        Dò {{ dlMaxCount > 0 ? dlMaxCount : 'tất cả' }} video {{ dlFetchOrder === 'newest' ? 'mới nhất' : 'cũ nhất' }}. Sau khi dò xong, bạn có thể lọc/sắp xếp thêm.
      </p>
    </div>

    <!-- ── Filters ─────────────────────────────────── -->
    <div class="dl-filters-row" v-if="probeResult && probeResult.entries.length > 1">
      <div class="dl-filter-item">
        <label>Sắp xếp:</label>
        <select v-model="dlSortBy" class="dl-select">
          <option value="views">Nhiều view nhất</option>
          <option value="likes">Nhiều like nhất</option>
          <option value="date">Mới nhất</option>
          <option value="duration">Dài nhất</option>
        </select>
      </div>
      <div class="dl-filter-item">
        <label>Từ khóa:</label>
        <input v-model="dlKeyword" type="text" class="dl-filter-input" placeholder="Lọc tiêu đề..." />
      </div>
      <div class="dl-filter-item">
        <label>Min views:</label>
        <input v-model.number="dlMinViews" type="number" class="dl-filter-input" min="0" step="100" />
      </div>
    </div>

    <!-- ── Select all bar ──────────────────────────── -->
    <div class="dl-select-bar" v-if="probeResult && filteredDlEntries.length > 0">
      <label class="dl-select-all-label" @click="dlToggleAll">
        <input type="checkbox" :checked="dlSelectedCount === filteredDlEntries.length && filteredDlEntries.length > 0" @click.stop="dlToggleAll" />
        Chọn tất cả
      </label>
      <span class="dl-selected-count">Đã chọn: {{ dlSelectedCount }}/{{ filteredDlEntries.length }}</span>
    </div>

    <!-- ── Empty state ── -->
    <div v-if="!probeResult && !isProbing" class="dl-empty-state">
      <Film :size="56" class="dl-empty-icon" />
      <p class="dl-empty-title">Nhập từ khóa / đường dẫn → nhấn <span class="dl-accent">Dò link</span> hoặc <span class="dl-accent">Tìm kiếm</span> để bắt đầu</p>
      <p class="dl-empty-sub">Hỗ trợ tải từ YouTube, TikTok, Facebook, Douyin và nhiều nền tảng khác</p>
    </div>

    <!-- ── Loading state ── -->
    <div v-if="isProbing" class="dl-loading-state">
      <Loader2 :size="40" class="spin-hourglass dl-loading-icon" style="color: var(--wx-brand-primary);" />
      <p class="dl-loading-text">Đang tìm kiếm / dò tìm nguồn video...</p>
    </div>

    <!-- ── Entry list ──────────────────────────────── -->
    <div class="dl-entries-list" v-if="probeResult && !isProbing"
      ref="listRef"
      style="position: relative;"
      @mousedown="onListMouseDown"
    >
      <!-- Drag selection box -->
      <div v-if="dragBox.active" class="dl-drag-box" :style="dragBoxStyle"></div>
      <div
        v-for="entry in filteredDlEntries"
        :key="entry.id"
        class="dl-entry-card"
        :class="{ selected: dlSelectedIds.has(entry.id) }"
        :data-entry-id="entry.id"
        @click="dlToggle(entry.id)"
      >
        <input type="checkbox" :checked="dlSelectedIds.has(entry.id)" @click.stop="dlToggle(entry.id)" class="dl-entry-check" />
        <div class="dl-entry-thumb">
          <img v-if="entry.thumbnail && (entry.thumbnail.startsWith('data:') || !failedThumbs.has(entry.id))" 
            :src="entry.thumbnail" 
            alt="" 
            referrerpolicy="no-referrer"
            @error="failedThumbs.add(entry.id)" />
          <Film v-else :size="20" style="opacity: 0.5;" />
        </div>
        <div class="dl-entry-info">
          <div class="dl-entry-title">{{ entry.title || 'Không rõ tên' }}</div>
          <div class="dl-entry-meta">
            <span v-if="entry.viewCount" class="dl-meta-chip" title="Lượt xem">
              <Eye :size="11" /> {{ formatViewCount(entry.viewCount) }}
            </span>
            <span v-if="entry.likeCount" class="dl-meta-chip" title="Lượt thích">
              <Heart :size="11" /> {{ formatViewCount(entry.likeCount) }}
            </span>
            <span v-if="entry.duration" class="dl-meta-chip" title="Thời lượng">
              <Clock :size="11" /> {{ formatTime(entry.duration) }}
            </span>
            <span v-if="entry.uploadDate" class="dl-meta-chip" title="Ngày đăng">
              <Calendar :size="11" /> {{ formatDlDate(entry.uploadDate) }}
            </span>
            
            <div class="dl-meta-actions">
              <a v-if="entry.url" :href="entry.url" target="_blank" @click.stop class="dl-meta-link" title="Xem video gốc" style="display: inline-flex; align-items: center; gap: 3px;">
                <Link2 :size="11" /> Xem
              </a>
              <button v-if="entry.url" type="button" @click.stop="copyToClipboard(entry.url)" class="dl-meta-link" title="Sao chép link">
                <Copy :size="11" /> Copy
              </button>
            </div>
          </div>
          <div v-if="entry.url" class="dl-entry-url" @click.stop>{{ entry.url }}</div>
        </div>
        <!-- Per-entry progress -->
        <div class="dl-entry-progress" v-if="dlProgressMap.get(entry.id)">
          <template v-if="dlProgressMap.get(entry.id)!.status === 'done'">
            <Check :size="16" class="dl-prog-icon-done" />
          </template>
          <template v-else-if="dlProgressMap.get(entry.id)!.status === 'error'">
            <X :size="16" class="dl-prog-icon-error" />
          </template>
          <template v-else>
            <div class="dl-mini-progress-bar">
              <div class="dl-mini-progress-fill" :style="{ width: dlProgressMap.get(entry.id)!.percent + '%' }"></div>
            </div>
            <span class="dl-mini-pct">{{ Math.round(dlProgressMap.get(entry.id)!.percent) }}%</span>
          </template>
        </div>
      </div>
      <div v-if="filteredDlEntries.length === 0" class="dl-empty">
        Không tìm thấy video phù hợp với bộ lọc.
      </div>
    </div>

    <!-- ── Download progress ───────────────────────── -->
    <div class="dl-progress-section" v-if="dlProgressMap.size > 0">
      <h4 class="dl-progress-title">
        <Download :size="14" />
        Tiến trình tải
      </h4>
      <div v-for="[id, p] of dlProgressMap" :key="id" class="dl-progress-row">
        <Check v-if="p.status === 'done'" :size="14" class="dl-prog-icon-done" />
        <X v-else-if="p.status === 'error'" :size="14" class="dl-prog-icon-error" />
        <Loader2 v-else :size="14" class="spin-hourglass dl-prog-icon-spin" />
        <span class="dl-prog-title">{{ p.title }}</span>
        <div class="dl-prog-bar" v-if="p.status === 'downloading'">
          <div class="dl-prog-fill" :style="{ width: p.percent + '%' }"></div>
        </div>
        <span class="dl-prog-pct" v-if="p.status === 'downloading'">{{ Math.round(p.percent) }}% · {{ p.speed }} · ETA {{ p.eta }}</span>
        <span class="dl-prog-pct dl-prog-done" v-else-if="p.status === 'done'">Hoàn tất</span>
        <span class="dl-prog-pct dl-prog-err" v-else>{{ p.error || 'Lỗi' }}</span>
      </div>
    </div>

    </div> <!-- end dl-body -->

    <!-- ── Footer ──────────────────────────────────── -->
    <div class="dl-footer">
      <div class="dl-dir-row">
        <label class="dl-dir-label">Lưu vào:</label>
        <input v-model="downloadDir" type="text" class="dl-dir-input" readonly />
        <button @click="pickDownloadDir" class="btn dl-dir-btn" title="Chọn thư mục">
          <FolderOpen :size="14" />
        </button>
      </div>
      <div class="dl-footer-actions">
        <button v-if="isDownloading" @click="cancelDl" class="btn stop-analyze-btn dl-action-btn">
          <Square :size="12" /> Hủy tải
        </button>
        <template v-else>
          <button
            v-if="dlSelectedCount > 0"
            @click="removeSelectedEntries"
            class="btn dl-remove-btn dl-action-btn"
            :title="`Xóa ${dlSelectedCount} video đã chọn khỏi danh sách`"
          >
            <Trash2 :size="13" /> Xóa {{ dlSelectedCount }} video
          </button>
          <button @click="startDownload" :disabled="dlSelectedCount === 0" class="btn btn-analyze dl-action-btn">
            <Download :size="12" /> Tải {{ dlSelectedCount }} video
          </button>
        </template>
      </div>
    </div>

  </div>
</template>

<style scoped>
/* ── Page shell ───────────────────────────────────────────── */
.dl-page {
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
.dl-page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid var(--wx-border-default);
  padding-bottom: var(--wx-space-3);
  flex-shrink: 0;
}

/* ── Scrollable body ───────────────────────────────────────── */
.dl-body {
  flex: 1;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-4);
  min-height: 0;
  padding-right: 2px;
}

.dl-body::-webkit-scrollbar { width: 5px; }
.dl-body::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--wx-text-primary) 10%, transparent);
  border-radius: var(--wx-radius-full);
}


.dl-page-title {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  margin: 0;
  font-size: var(--wx-fs-18);
  font-weight: var(--wx-fw-bold);
  color: var(--wx-text-primary);
}

.dl-title-icon { color: var(--wx-brand-primary); }

.dl-title-sub {
  font-size: var(--wx-fs-12);
  font-weight: var(--wx-fw-regular);
  color: var(--wx-text-muted);
  margin-left: var(--wx-space-2);
}

.dl-back-btn {
  padding: var(--wx-space-1) var(--wx-space-3);
  font-size: var(--wx-fs-12);
  border-radius: var(--wx-radius-md);
}

/* ── Mode tabs ─────────────────────────────────────────────── */
.dl-mode-tabs {
  display: flex;
  gap: var(--wx-space-2);
  border-bottom: 1px solid var(--wx-border-default);
  padding-bottom: var(--wx-space-3);
}

.dl-mode-tab {
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
  transition: all var(--wx-d-fast, 150ms) ease;
}

.dl-mode-tab:hover { color: var(--wx-text-primary); }

.dl-mode-tab.active {
  background: color-mix(in srgb, var(--wx-brand-primary) 12%, transparent);
  border-color: color-mix(in srgb, var(--wx-brand-primary) 40%, transparent);
  color: var(--wx-brand-primary);
}

/* ── URL Input section ─────────────────────────────────────── */
.dl-input-section {
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-2);
}

.dl-input-label {
  font-size: var(--wx-fs-12);
  font-weight: var(--wx-fw-bold);
  color: var(--wx-text-muted);
  letter-spacing: 0.04em;
}

.dl-input-row {
  display: flex;
  gap: var(--wx-space-2);
  align-items: stretch;
}

.dl-url-input {
  flex: 1;
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
}

.dl-url-input::placeholder { color: var(--wx-text-muted); opacity: 0.6; }

.dl-url-input:focus {
  border-color: var(--wx-brand-primary);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--wx-brand-primary) 15%, transparent);
}

.dl-url-input:disabled { opacity: 0.5; cursor: not-allowed; }

.dl-probe-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
  white-space: nowrap;
  height: 38px;
  box-sizing: border-box;
  padding: 0 var(--wx-space-4);
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-bold);
}

/* ── Options row ────────────────────────────────────────────── */
.dl-options-row {
  display: flex;
  gap: var(--wx-space-3);
  align-items: center;
  flex-wrap: wrap;
  padding: var(--wx-space-2) var(--wx-space-3);
  background: var(--wx-surface-sunken);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
}

.dl-opt-group {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
}

.dl-opt-group label {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
  white-space: nowrap;
}

.dl-select {
  background: var(--wx-surface-base);
  border: 1px solid var(--wx-border-default);
  color: var(--wx-text-primary);
  padding: var(--wx-space-1) var(--wx-space-2);
  border-radius: var(--wx-radius-sm);
  font-size: var(--wx-fs-12);
  outline: none;
  cursor: pointer;
  transition: border-color 150ms ease;
}

.dl-select:focus { border-color: var(--wx-brand-primary); }

/* Source pills */
.dl-source-pills {
  display: flex;
  gap: var(--wx-space-1);
}

.dl-source-pill {
  font-size: var(--wx-fs-12);
  padding: 3px var(--wx-space-2);
  border-radius: var(--wx-radius-sm);
  border: 1px solid var(--wx-border-default);
  background: transparent;
  color: var(--wx-text-muted);
  font-weight: var(--wx-fw-semibold);
  cursor: pointer;
  transition: all 150ms ease;
}

.dl-source-pill:hover { color: var(--wx-text-primary); }

.dl-source-pill.active {
  background: color-mix(in srgb, var(--wx-brand-primary) 12%, transparent);
  border-color: var(--wx-brand-primary);
  color: var(--wx-brand-primary);
}

/* Platform badges */
.dl-platform-badge {
  padding: 2px var(--wx-space-2);
  border-radius: var(--wx-radius-full);
  font-size: var(--wx-fs-12);
  font-weight: var(--wx-fw-bold);
}
.dl-platform-badge.youtube { background: color-mix(in srgb, #ff4444 15%, transparent); color: #ff4444; }
.dl-platform-badge.tiktok  { background: color-mix(in srgb, #69c9d0 15%, transparent); color: #69c9d0; }
.dl-platform-badge.facebook{ background: color-mix(in srgb, #1877f2 15%, transparent); color: #1877f2; }
.dl-platform-badge.generic,
.dl-platform-badge.mixed   { background: color-mix(in srgb, var(--wx-brand-primary) 12%, transparent); color: var(--wx-brand-primary); }

.dl-type-badge, .dl-count-badge {
  padding: 2px var(--wx-space-2);
  border-radius: var(--wx-radius-full);
  font-size: var(--wx-fs-12);
  font-weight: var(--wx-fw-semibold);
  background: color-mix(in srgb, var(--wx-brand-primary) 8%, transparent);
  color: var(--wx-text-muted);
}

/* ── Profile config ─────────────────────────────────────────── */
.dl-profile-config {
  display: flex;
  gap: var(--wx-space-3);
  align-items: center;
  flex-wrap: wrap;
  padding: var(--wx-space-2) var(--wx-space-3);
  background: color-mix(in srgb, var(--wx-brand-primary) 5%, transparent);
  border: 1px solid color-mix(in srgb, var(--wx-brand-primary) 20%, transparent);
  border-radius: var(--wx-radius-md);
}

.dl-profile-hint {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
  line-height: 1.4;
  flex-basis: 100%;
  margin: 0;
}

/* ── Filters ────────────────────────────────────────────────── */
.dl-filters-row {
  display: flex;
  gap: var(--wx-space-3);
  align-items: flex-end;
  flex-wrap: wrap;
  padding: var(--wx-space-2) var(--wx-space-3);
  background: var(--wx-surface-sunken);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
}

.dl-filter-item {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.dl-filter-item label {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
}

.dl-filter-input {
  background: var(--wx-surface-base);
  border: 1.5px solid var(--wx-border-default);
  color: var(--wx-text-primary);
  padding: var(--wx-space-1) var(--wx-space-2);
  border-radius: var(--wx-radius-sm);
  font-size: var(--wx-fs-12);
  outline: none;
  width: 130px;
  transition: border-color 150ms ease;
}

.dl-filter-input:focus { border-color: var(--wx-brand-primary); }

/* ── Select bar ─────────────────────────────────────────────── */
.dl-select-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--wx-space-1) var(--wx-space-3);
  background: var(--wx-surface-sunken);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
}

.dl-select-all-label {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  font-size: var(--wx-fs-13);
  color: var(--wx-text-primary);
  cursor: pointer;
  user-select: none;
}

.dl-selected-count {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
  font-weight: var(--wx-fw-semibold);
}

/* ── Entry cards ────────────────────────────────────────────── */
.dl-entries-list {
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-2);
  overflow-y: auto;
  max-height: 380px;
  user-select: none;
}

.dl-entry-card {
  display: flex;
  align-items: center;
  gap: var(--wx-space-3);
  padding: var(--wx-space-2) var(--wx-space-3);
  background: var(--wx-surface-elevated);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
  cursor: pointer;
  transition: all 150ms ease;
}

.dl-entry-card:hover {
  border-color: var(--wx-border-subtle);
  background: var(--wx-hover-bg);
}

.dl-entry-card.selected {
  border-color: var(--wx-brand-primary);
  background: color-mix(in srgb, var(--wx-brand-primary) 6%, var(--wx-surface-elevated));
}

.dl-entry-check { accent-color: var(--wx-brand-primary); flex-shrink: 0; }

.dl-entry-thumb {
  width: 80px;
  height: 45px;
  border-radius: var(--wx-radius-sm);
  overflow: hidden;
  background: #000;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  border: 1px solid var(--wx-border-default);
}

.dl-entry-thumb img { width: 100%; height: 100%; object-fit: cover; }

.dl-entry-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.dl-entry-title {
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-semibold);
  color: var(--wx-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.dl-entry-meta {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  flex-wrap: wrap;
}

.dl-meta-chip {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
}

.dl-meta-link {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: var(--wx-fs-12);
  color: var(--wx-brand-primary);
  text-decoration: none;
  font-weight: var(--wx-fw-semibold);
  cursor: pointer;
  background: none;
  border: none;
  padding: 0;
}

.dl-meta-actions {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 12px;
}

.dl-entry-url {
  font-size: 10px;
  color: var(--wx-text-muted);
  opacity: 0.5;
  word-break: break-all;
  user-select: text;
  margin-top: 2px;
}

/* Per-entry progress */
.dl-entry-progress {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  min-width: 80px;
  flex-shrink: 0;
}

.dl-prog-icon-done { color: var(--wx-success-solid); }
.dl-prog-icon-error { color: var(--wx-danger-solid); }
.dl-prog-icon-spin { color: var(--wx-brand-primary); }

.dl-mini-progress-bar {
  flex: 1;
  height: 4px;
  background: color-mix(in srgb, var(--wx-text-primary) 10%, transparent);
  border-radius: var(--wx-radius-full);
  overflow: hidden;
}

.dl-mini-progress-fill {
  height: 100%;
  background: var(--wx-brand-primary);
  border-radius: var(--wx-radius-full);
  transition: width 200ms ease;
}

.dl-mini-pct {
  font-size: var(--wx-fs-12);
  color: var(--wx-brand-primary);
  font-weight: var(--wx-fw-bold);
}

.dl-empty {
  text-align: center;
  padding: var(--wx-space-5);
  color: var(--wx-text-muted);
  font-size: var(--wx-fs-13);
}

/* Empty state */
.dl-empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wx-space-3);
  height: 250px;
  color: var(--wx-text-muted);
  text-align: center;
}

.dl-empty-icon { opacity: 0.2; }

.dl-empty-title {
  font-size: var(--wx-fs-14);
  font-weight: var(--wx-fw-semibold);
  margin: 0;
  color: var(--wx-text-primary);
}

.dl-empty-sub {
  font-size: var(--wx-fs-12);
  opacity: 0.55;
  margin: 0;
}

.dl-accent { color: var(--wx-brand-primary); }

/* Loading state */
.dl-loading-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--wx-space-4);
  height: 250px;
  color: var(--wx-text-muted);
}

.dl-loading-text {
  font-size: var(--wx-fs-14);
  font-weight: var(--wx-fw-semibold);
  margin: 0;
}

/* ── Download progress section ───────────────────────────────── */
.dl-progress-section {
  background: var(--wx-surface-elevated);
  border: 1px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
  padding: var(--wx-space-3);
  display: flex;
  flex-direction: column;
  gap: var(--wx-space-2);
}

.dl-progress-title {
  margin: 0;
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-bold);
  color: var(--wx-brand-primary);
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
}

.dl-progress-row {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  font-size: var(--wx-fs-12);
}

.dl-prog-title {
  flex: 1;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--wx-text-primary);
}

.dl-prog-bar {
  width: 80px;
  height: 4px;
  background: color-mix(in srgb, var(--wx-text-primary) 10%, transparent);
  border-radius: var(--wx-radius-full);
  overflow: hidden;
}

.dl-prog-fill {
  height: 100%;
  background: var(--wx-brand-primary);
  border-radius: var(--wx-radius-full);
  transition: width 200ms ease;
}

.dl-prog-pct { color: var(--wx-text-muted); font-size: var(--wx-fs-12); }
.dl-prog-done { color: var(--wx-success-solid) !important; }
.dl-prog-err  { color: var(--wx-danger-solid) !important; }

/* ── Footer ─────────────────────────────────────────────────── */
.dl-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-top: var(--wx-space-3);
  border-top: 1px solid var(--wx-border-default);
  gap: var(--wx-space-3);
  flex-wrap: wrap;
  flex-shrink: 0; /* pin to bottom */
}

.dl-dir-row {
  display: flex;
  align-items: center;
  gap: var(--wx-space-2);
  flex: 1;
}

.dl-dir-label {
  font-size: var(--wx-fs-12);
  color: var(--wx-text-muted);
  white-space: nowrap;
}

.dl-dir-input {
  flex: 1;
  background: var(--wx-surface-sunken);
  border: 1.5px solid var(--wx-border-default);
  color: var(--wx-text-primary);
  padding: var(--wx-space-1) var(--wx-space-2);
  border-radius: var(--wx-radius-md);
  font-size: var(--wx-fs-12);
  outline: none;
  cursor: default;
  min-width: 0;
}

.dl-dir-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: var(--wx-space-1) var(--wx-space-2);
  height: 32px;
  flex-shrink: 0;
}

.dl-footer-actions {
  display: flex;
  gap: var(--wx-space-2);
}

.dl-action-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--wx-space-1);
  justify-content: center;
  height: 36px;
  padding: 0 var(--wx-space-4);
  font-size: var(--wx-fs-13);
  font-weight: var(--wx-fw-bold);
}

.dl-remove-btn {
  background: var(--wx-danger-solid);
  color: var(--wx-text-inverse);
  border: none;
}

.dl-remove-btn:hover {
  filter: brightness(1.1);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px color-mix(in srgb, var(--wx-danger-solid) 35%, transparent);
}

/* Drag selection box */
.dl-drag-box {
  position: absolute;
  pointer-events: none;
  border: 1.5px solid var(--wx-brand-primary);
  background: color-mix(in srgb, var(--wx-brand-primary) 12%, transparent);
  border-radius: 4px;
  z-index: 100;
}

.spin-hourglass {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
