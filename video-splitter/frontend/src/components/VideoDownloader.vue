<script setup lang="ts">
import { onMounted, toRef, ref, reactive } from 'vue'
import { useDownloader } from './composables/useDownloader'
import {
  Globe, Link2, Search, Download, Square, FolderOpen,
  Check, X, Eye, Heart, Clock, Calendar, Copy, Loader2, Trash2, Film, SlidersHorizontal
} from 'lucide-vue-next'
import BaseButton from './common/BaseButton.vue'

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

const showFilters = ref(false)

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
        <BaseButton variant="primary" size="md" :loading="isProbing" :disabled="isProbing || !downloadUrl.trim()" @click="probeUrl">
          <Link2 v-if="!isProbing && downloadMode === 'link'" :size="14" />
          <Search v-else-if="!isProbing" :size="14" />
          {{ isProbing ? (downloadMode === 'link' ? 'Đang dò...' : 'Đang tìm...') : (downloadMode === 'link' ? 'Tìm Video' : 'Tìm kiếm') }}
        </BaseButton>
        <button type="button" @click="showFilters = !showFilters" class="btn dl-filter-toggle-btn" :class="{ active: showFilters }">
          <SlidersHorizontal :size="14" />
          <span>Bộ lọc</span>
        </button>
      </div>
    </div>

    <!-- ── Advanced settings collapsible ────────────────── -->
    <div v-show="showFilters" class="dl-advanced-settings">
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

    <!-- ── Results area ────────────────────────────── -->
    <div class="dl-results-area" v-if="probeResult && !isProbing">
      <!-- Results header -->
      <div class="dl-results-header" v-if="filteredDlEntries.length > 0">
        <span class="dl-results-count">
          <strong>{{ filteredDlEntries.length }}</strong> video tìm thấy
          <span v-if="probeResult && probeResult.platform" class="dl-source-tag">{{ probeResult.platform }}</span>
        </span>
        <button type="button" @click="dlToggleAll" class="dl-toggle-all-btn">
          {{ dlSelectedCount === filteredDlEntries.length ? 'Bỏ tất cả' : 'Chọn tất cả' }}
          <span class="dl-count-badge">({{ dlSelectedCount }}/{{ filteredDlEntries.length }})</span>
        </button>
      </div>

      <!-- ── Entry list ──────────────────────────────── -->
      <div class="dl-entries-list"
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
            draggable="false"
            @dragstart.prevent
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
    </div> <!-- end dl-results-area -->

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
        <BaseButton variant="secondary" size="md" title="Chọn thư mục" @click="pickDownloadDir">
          <FolderOpen :size="14" />
        </BaseButton>
      </div>
      <div class="dl-footer-actions">
        <BaseButton v-if="isDownloading" variant="danger" size="md" @click="cancelDl">
          <Square :size="12" /> Hủy tải
        </BaseButton>
        <template v-else>
          <BaseButton
            v-if="dlSelectedCount > 0"
            variant="danger"
            size="md"
            :title="`Xóa ${dlSelectedCount} video đã chọn khỏi danh sách`"
            @click="removeSelectedEntries"
          >
            <Trash2 :size="13" /> Xóa {{ dlSelectedCount }} video đã chọn
          </BaseButton>
          <BaseButton variant="primary" size="md" :disabled="dlSelectedCount === 0" @click="startDownload">
            <Download :size="12" />
            <span v-if="dlSelectedCount === 0">Chọn video để tải</span>
            <span v-else>Tải {{ dlSelectedCount }} video đã chọn</span>
          </BaseButton>
        </template>
      </div>
    </div>

  </div>
</template>

<style scoped src="./VideoDownloader.scoped.css"></style>

