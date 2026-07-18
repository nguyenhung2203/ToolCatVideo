<script setup lang="ts">
import { onMounted, toRef } from 'vue'
import { useDownloader } from './composables/useDownloader'

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
  cancelDl, pickDownloadDir, initDownloadEvents, copyToClipboard
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
</script>

<template>
  <div class="inline-page-card" style="background: var(--bg-panel, #182235); border: 1px solid var(--border-color, rgba(255,255,255,0.08)); border-radius: 12px; padding: 20px; display: flex; flex-direction: column; gap: 16px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); flex: 1; overflow-y: auto;">
    <div style="display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid rgba(255,255,255,0.06); padding-bottom: 12px; margin-bottom: 4px;">
      <h2 style="display:flex; align-items:center; gap:8px; margin: 0; font-size: 18px; color: var(--l-text);">
        <svg viewBox="0 0 24 24" width="20" height="20" style="color:var(--accent-color);"><path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-1 17.93c-3.95-.49-7-3.85-7-7.93 0-.62.08-1.21.21-1.79L9 15v1c0 1.1.9 2 2 2v1.93zm6.9-2.53c-.26-.81-1-1.4-1.9-1.4h-1v-3c0-.55-.45-1-1-1h-6v-2h2c.55 0 1-.45 1-1V7h2c1.1 0 2-.9 2-2v-.41c2.93 1.19 5 4.06 5 7.41 0 2.08-.8 3.97-2.1 5.39z"/></svg>
        Tải Video Online (YouTube / Facebook / TikTok / Douyin...)
      </h2>
      <button class="btn select-btn" @click="emit('back')" style="padding: 5px 12px; font-size: 12px; font-weight: 600; border-radius: 6px;">
        ← Quay lại Cắt Video
      </button>
    </div>

    <div class="modal-body" style="padding: 0; overflow-y: visible;">
      <!-- Mode Selector Switch: Dán Link vs Tìm Kiếm -->
      <div style="display: flex; gap: 8px; margin-bottom: 14px; border-bottom: 1px solid rgba(255,255,255,0.06); padding-bottom: 10px;">
        <button type="button" @click="downloadMode = 'link'; downloadUrl = ''; probeResult = null" 
                style="font-size: 11px; padding: 5px 12px; border-radius: 6px; cursor: pointer; transition: all 0.2s; border: 1px solid rgba(255,255,255,0.08); font-weight: 600; outline: none; display: flex; align-items: center; gap: 4px;"
                :style="{
                  background: downloadMode === 'link' ? 'rgba(99, 102, 241, 0.15)' : 'rgba(255,255,255,0.02)',
                  borderColor: downloadMode === 'link' ? 'var(--accent-color)' : 'rgba(255,255,255,0.08)',
                  color: downloadMode === 'link' ? 'var(--accent-color)' : 'var(--l-text-muted)'
                }">
          🔗 Dán link nguồn trực tiếp
        </button>
        <button type="button" @click="downloadMode = 'search'; downloadUrl = ''; probeResult = null" 
                style="font-size: 11px; padding: 5px 12px; border-radius: 6px; cursor: pointer; transition: all 0.2s; border: 1px solid rgba(255,255,255,0.08); font-weight: 600; outline: none; display: flex; align-items: center; gap: 4px;"
                :style="{
                  background: downloadMode === 'search' ? 'rgba(99, 102, 241, 0.15)' : 'rgba(255,255,255,0.02)',
                  borderColor: downloadMode === 'search' ? 'var(--accent-color)' : 'rgba(255,255,255,0.08)',
                  color: downloadMode === 'search' ? 'var(--accent-color)' : 'var(--l-text-muted)'
                }">
          🔍 Tìm kiếm bằng từ khóa
        </button>
      </div>

      <!-- Single Input Box depending on Active Mode -->
      <div style="display: flex; flex-direction: column; gap: 4px; margin-bottom: 16px;">
        <label style="font-size: 11px; color: var(--l-text-muted); font-weight: 600;">
          {{ downloadMode === 'link' ? 'NHẬP ĐƯỜNG DẪN LINK NGUỒN CỦA VIDEO / DANH SÁCH / KÊNH:' : 'NHẬP TỪ KHÓA / CHỦ ĐỀ CẦN TÌM KIẾM ĐA NGUỒN:' }}
        </label>
        <div style="display: flex; gap: 8px;">
          <input v-model="downloadUrl" type="text" class="dl-url-input" style="flex: 1;" 
                 :placeholder="downloadMode === 'link' ? 'Dán link video, kênh hoặc playlist (YouTube, TikTok, Facebook Reel/Watch hoặc website nguồn bất kỳ)...' : 'Nhập từ khóa tìm kiếm (ví dụ: xe độ, vlog, nấu ăn, hài hước)...'" 
                 @keyup.enter="probeUrl" :disabled="isProbing" />
          <button @click="probeUrl" class="btn dl-probe-btn" :disabled="isProbing" style="display:inline-flex; align-items:center; gap:4px; white-space: nowrap; height: 38px;">
            <template v-if="isProbing">
              <svg viewBox="0 0 24 24" width="14" height="14" class="spin-hourglass" style="display:inline-block; animation: spin 1.5s linear infinite;"><path fill="currentColor" d="M6 2v6h.01L6 8.01 10 12l-4 4 .01.01H6V22h12v-5.99h-.01L18 16l-4-4 4-3.99-.01-.01H18V2H6zm10 14.5V20H8v-3.5l4-4 4 4zm-4-5l-4-4V4h8v3.5l-4 4z"/></svg>
              Đang dò...
            </template>
            <template v-else>
              <svg viewBox="0 0 24 24" width="14" height="14"><path fill="currentColor" d="M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/></svg>
              {{ downloadMode === 'link' ? 'Dò link' : 'Tìm kiếm' }}
            </template>
          </button>
        </div>
      </div>

      <!-- Loại link + Cookie + Nguồn tìm kiếm -->
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
        <div class="dl-opt-group" style="flex: 1.8; min-width: 250px;" v-if="downloadMode === 'search'">
          <label>Nguồn tìm kiếm:</label>
          <div style="display: flex; gap: 5px; margin-top: 4px;">
            <button type="button" @click="searchSource = 'youtube'" 
                    style="font-size: 11px; padding: 4px 8px; border-radius: 6px; cursor: pointer; transition: all 0.2s; border: 1px solid rgba(255,255,255,0.08); font-weight: 600; outline: none;"
                    :style="{
                      background: searchSource === 'youtube' ? 'rgba(99, 102, 241, 0.15)' : 'rgba(255,255,255,0.02)',
                      borderColor: searchSource === 'youtube' ? 'var(--accent-color)' : 'rgba(255,255,255,0.08)',
                      color: searchSource === 'youtube' ? 'var(--accent-color)' : 'var(--l-text-muted)'
                    }">
              YouTube
            </button>
            <button type="button" @click="searchSource = 'tiktok'" 
                    style="font-size: 11px; padding: 4px 8px; border-radius: 6px; cursor: pointer; transition: all 0.2s; border: 1px solid rgba(255,255,255,0.08); font-weight: 600; outline: none;"
                    :style="{
                      background: searchSource === 'tiktok' ? 'rgba(99, 102, 241, 0.15)' : 'rgba(255,255,255,0.02)',
                      borderColor: searchSource === 'tiktok' ? 'var(--accent-color)' : 'rgba(255,255,255,0.08)',
                      color: searchSource === 'tiktok' ? 'var(--accent-color)' : 'var(--l-text-muted)'
                    }">
              TikTok
            </button>
            <button type="button" @click="searchSource = 'facebook'" 
                    style="font-size: 11px; padding: 4px 8px; border-radius: 6px; cursor: pointer; transition: all 0.2s; border: 1px solid rgba(255,255,255,0.08); font-weight: 600; outline: none;"
                    :style="{
                      background: searchSource === 'facebook' ? 'rgba(99, 102, 241, 0.15)' : 'rgba(255,255,255,0.02)',
                      borderColor: searchSource === 'facebook' ? 'var(--accent-color)' : 'rgba(255,255,255,0.08)',
                      color: searchSource === 'facebook' ? 'var(--accent-color)' : 'var(--l-text-muted)'
                    }">
              Facebook
            </button>
            <button type="button" @click="searchSource = 'all'" 
                    style="font-size: 11px; padding: 4px 8px; border-radius: 6px; cursor: pointer; transition: all 0.2s; border: 1px solid rgba(255,255,255,0.08); font-weight: 600; outline: none;"
                    :style="{
                      background: searchSource === 'all' ? 'rgba(99, 102, 241, 0.15)' : 'rgba(255,255,255,0.02)',
                      borderColor: searchSource === 'all' ? 'var(--accent-color)' : 'rgba(255,255,255,0.08)',
                      color: searchSource === 'all' ? 'var(--accent-color)' : 'var(--l-text-muted)'
                    }">
              Hỗn hợp
            </button>
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

      <!-- Cấu hình Profile/Playlist: số lượng + thứ tự -->
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
        <div class="dl-profile-hint">
          Dò {{ dlMaxCount > 0 ? dlMaxCount : 'tất cả' }} video {{ dlFetchOrder === 'newest' ? 'mới nhất' : 'cũ nhất' }}. Sau khi dò xong, bạn có thể lọc/sắp xếp thêm theo views, likes, thời lượng.
        </div>
      </div>

      <!-- Filters (chỉ hiển thị khi có nhiều hơn 1 video) -->
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

      <!-- Select all + count -->
      <div class="dl-select-bar" v-if="probeResult && filteredDlEntries.length > 0">
        <label class="dl-select-all-label" @click="dlToggleAll">
          <input type="checkbox" :checked="dlSelectedCount === filteredDlEntries.length && filteredDlEntries.length > 0" @click.stop="dlToggleAll" />
          Chọn tất cả
        </label>
        <span class="dl-selected-count">Đã chọn: {{ dlSelectedCount }}/{{ filteredDlEntries.length }}</span>
      </div>

      <!-- Video entries list -->
      <div class="dl-entries-list" v-if="probeResult">
        <div v-for="entry in filteredDlEntries" :key="entry.id" class="dl-entry-card" :class="{ selected: dlSelectedIds.has(entry.id) }" @click="dlToggle(entry.id)">
          <input type="checkbox" :checked="dlSelectedIds.has(entry.id)" @click.stop="dlToggle(entry.id)" class="dl-entry-check" />
          <div class="dl-entry-thumb">
            <img v-if="entry.thumbnail" :src="entry.thumbnail" alt="" referrerpolicy="no-referrer" crossorigin="anonymous" @error="($event.target as HTMLImageElement).style.display='none'; ($event.target as HTMLImageElement).parentElement!.classList.add('dl-entry-thumb-placeholder'); ($event.target as HTMLImageElement).parentElement!.textContent='🎬'" />
            <span v-else>🎬</span>
          </div>
          <div class="dl-entry-info">
            <div class="dl-entry-title">{{ entry.title || 'Không rõ tên' }}</div>
            <div class="dl-entry-meta">
              <span v-if="entry.viewCount" title="Lượt xem" style="display:inline-flex; align-items:center; gap:2px;">
                <svg viewBox="0 0 24 24" width="11" height="11" style="color:var(--text-muted);"><path fill="currentColor" d="M12 4.5C7 4.5 2.73 7.61 1 12c1.73 4.39 6 7.5 11 7.5s9.27-3.11 11-7.5c-1.73-4.39-6-7.5-11-7.5zM12 17c-2.76 0-5-2.24-5-5s2.24-5 5-5 5 2.24 5 5-2.24 5-5 5zm0-8c-1.66 0-3 1.34-3 3s1.34 3 3 3 3-1.34 3-3-1.34-3-3-3z"/></svg>
                {{ formatViewCount(entry.viewCount) }}
              </span>
              <span v-if="entry.likeCount" title="Lượt thích" style="display:inline-flex; align-items:center; gap:2px;">
                <svg viewBox="0 0 24 24" width="11" height="11" style="color:var(--text-muted);"><path fill="currentColor" d="M12 21.35l-1.45-1.32C5.4 15.36 2 12.28 2 8.5 2 5.42 4.42 3 7.5 3c1.74 0 3.41.81 4.5 2.09C13.09 3.81 14.76 3 16.5 3 19.58 3 22 5.42 22 8.5c0 3.78-3.4 6.86-8.55 11.54L12 21.35z"/></svg>
                {{ formatViewCount(entry.likeCount) }}
              </span>
              <span v-if="entry.duration" title="Thời lượng" style="display:inline-flex; align-items:center; gap:2px;">
                <svg viewBox="0 0 24 24" width="11" height="11" style="color:var(--text-muted);"><path fill="currentColor" d="M11.99 2C6.47 2 2 6.48 2 12s4.47 10 9.99 10C17.52 22 22 17.52 22 12S17.52 2 11.99 2zM12 20c-4.42 0-8-3.58-8-8s3.58-8 8-8 8 3.58 8 8-3.58 8-8 8zm.5-13H11v6l5.25 3.15.75-1.23-4.5-2.67z"/></svg>
                {{ formatTime(entry.duration) }}
              </span>
              <span v-if="entry.uploadDate" title="Ngày đăng" style="display:inline-flex; align-items:center; gap:2px;">
                <svg viewBox="0 0 24 24" width="11" height="11" style="color:var(--text-muted);"><path fill="currentColor" d="M19 3h-1V1h-2v2H8V1H6v2H5c-1.11 0-1.99.9-1.99 2L3 19c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2zm0 16H5V8h14v11z"/></svg>
                {{ formatDlDate(entry.uploadDate) }}
              </span>
              <a v-if="entry.url" :href="entry.url" target="_blank" @click.stop 
                 style="display:inline-flex; align-items:center; gap:2px; color: var(--accent-color); text-decoration: none; font-size: 11px; margin-left: auto; font-weight: 600;" 
                 title="Xem video gốc trên trình duyệt">
                🔗 Xem
              </a>
              <button v-if="entry.url" type="button" @click.stop="copyToClipboard(entry.url)"
                 style="display:inline-flex; align-items:center; gap:2px; color: var(--accent-color); background: none; border: none; font-size: 11px; margin-left: 8px; font-weight: 600; cursor: pointer; padding: 0;" 
                 title="Sao chép đường dẫn video gốc">
                📋 Copy
              </button>
            </div>
            <div v-if="entry.url" style="font-size: 10px; color: var(--l-text-muted); opacity: 0.6; margin-top: 4px; word-break: break-all; user-select: text;" @click.stop>
              {{ entry.url }}
            </div>
          </div>
          <!-- Per-entry progress -->
          <div class="dl-entry-progress" v-if="dlProgressMap.get(entry.id)">
            <template v-if="dlProgressMap.get(entry.id)!.status === 'done'">
              <svg viewBox="0 0 24 24" width="16" height="16" style="color:var(--success-color);"><path fill="currentColor" d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
            </template>
            <template v-else-if="dlProgressMap.get(entry.id)!.status === 'error'">
              <svg viewBox="0 0 24 24" width="16" height="16" style="color:var(--danger-color);"><path fill="currentColor" d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>
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

      <!-- Download progress summary -->
      <div class="dl-progress-section" v-if="dlProgressMap.size > 0">
        <h4 class="dl-progress-title" style="display:flex; align-items:center; gap:6px; margin: 12px 0 6px 0;">
          <svg viewBox="0 0 24 24" width="14" height="14" style="color:var(--accent-color);"><path fill="currentColor" d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg>
          Tiến trình tải
        </h4>
        <div v-for="[id, p] of dlProgressMap" :key="id" class="dl-progress-row">
          <span class="dl-prog-icon" v-if="p.status === 'done'">
            <svg viewBox="0 0 24 24" width="14" height="14" style="color:var(--success-color);"><path fill="currentColor" d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
          </span>
          <span class="dl-prog-icon" v-else-if="p.status === 'error'">
            <svg viewBox="0 0 24 24" width="14" height="14" style="color:var(--danger-color);"><path fill="currentColor" d="M19 6.41L17.59 5 12 10.59 6.41 5 5 6.41 10.59 12 5 17.59 6.41 19 12 13.41 17.59 19 19 17.59 13.41 12z"/></svg>
          </span>
          <span class="dl-prog-icon" v-else>
            <svg viewBox="0 0 24 24" width="14" height="14" class="spin-hourglass" style="display:inline-block;"><path fill="currentColor" d="M19 9h-4V3H9v6H5l7 7 7-7zM5 18v2h14v-2H5z"/></svg>
          </span>
          <span class="dl-prog-title">{{ p.title }}</span>
          <div class="dl-prog-bar" v-if="p.status === 'downloading'">
            <div class="dl-prog-fill" :style="{ width: p.percent + '%' }"></div>
          </div>
          <span class="dl-prog-pct" v-if="p.status === 'downloading'">{{ Math.round(p.percent) }}% · {{ p.speed }} · ETA {{ p.eta }}</span>
          <span class="dl-prog-pct" v-else-if="p.status === 'done'" style="color: var(--success-color)">Hoàn tất</span>
          <span class="dl-prog-pct" v-else style="color: var(--danger-color)">{{ p.error || 'Lỗi' }}</span>
        </div>
      </div>
    </div>

    <div class="modal-footer dl-footer" style="padding: 16px 0 0 0; border-top: 1px solid rgba(255,255,255,0.06); background: transparent;">
      <div class="dl-footer-left">
        <div class="dl-dir-row">
          <label>Lưu vào:</label>
          <input v-model="downloadDir" type="text" class="dl-dir-input" readonly />
          <button @click="pickDownloadDir" class="btn dl-dir-btn" style="display:inline-flex; align-items:center; justify-content:center; padding: 4px 8px;">
            <svg viewBox="0 0 24 24" width="14" height="14" style="color:var(--text-main);"><path fill="currentColor" d="M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/></svg>
          </button>
        </div>
      </div>
      <div class="dl-footer-right">
        <button v-if="isDownloading" @click="cancelDl" class="btn stop-analyze-btn" style="display:inline-flex; align-items:center; gap:4px; justify-content:center;">
          <svg viewBox="0 0 24 24" width="12" height="12"><path fill="currentColor" d="M6 6h12v12H6z"/></svg>
          Hủy tải
        </button>
        <button v-else @click="startDownload" :disabled="dlSelectedCount === 0" class="btn start-btn dl-start-btn" style="display:inline-flex; align-items:center; gap:4px; justify-content:center;">
          <svg viewBox="0 0 24 24" width="12" height="12"><path fill="currentColor" d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96zM17 13l-5 5-5-5h3V9h4v4h3z"/></svg>
          Tải {{ dlSelectedCount }} video
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped src="./VideoSplitter.scoped.css"></style>
<style src="./VideoSplitter.global.css"></style>
