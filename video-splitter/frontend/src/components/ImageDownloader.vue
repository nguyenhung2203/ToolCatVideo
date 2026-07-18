<script setup lang="ts">
import { onMounted } from 'vue'
import { useImageDownloader, IMAGE_SOURCES } from './composables/useImageDownloader'

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
} = useImageDownloader(props.showToast)

onMounted(async () => {
  await openImagePanel()
  initImageEvents()
})
</script>

<template>
  <div class="inline-page-card" style="background: var(--bg-panel, #182235); border: 1px solid var(--border-color, rgba(255,255,255,0.08)); border-radius: 12px; padding: 20px; display: flex; flex-direction: column; gap: 16px; box-shadow: 0 4px 12px rgba(0,0,0,0.15); flex: 1; overflow-y: auto;">
    <div style="display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid rgba(255,255,255,0.06); padding-bottom: 12px; margin-bottom: 4px;">
      <h2 style="display:flex; align-items:center; gap:8px; margin: 0; font-size: 18px; color: var(--l-text);">
        <svg viewBox="0 0 24 24" width="20" height="20" style="color:#06b6d4;flex-shrink:0;"><path fill="currentColor" d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/></svg>
        Tải Ảnh Theo Chủ Đề (Làm Ảnh Bìa / Hình Nền AI)
      </h2>
      <button class="btn select-btn" @click="emit('back')" style="padding: 5px 12px; font-size: 12px; font-weight: 600; border-radius: 6px;">
        ← Quay lại Cắt Video
      </button>
    </div>

    <div class="modal-body" style="padding: 0; overflow-y: visible;">
      <!-- Search section -->
      <div style="display:flex;flex-direction:column;gap:4px;margin-bottom:12px;">
        <label style="font-size:11px;color:var(--l-text-muted);font-weight:600;text-transform:uppercase;letter-spacing:.06em;">Nhập chủ đề / từ khóa:</label>
        <div style="display:flex;gap:8px;">
          <input v-model="imageQuery" type="text" class="dl-url-input" style="flex:1;"
            placeholder="VD: mèo cute, phong cảnh Việt Nam, ẩm thực đường phố, nature..."
            @keyup.enter="searchImages" :disabled="isImageSearching || isImageDownloading" />
          <button @click="searchImages" class="btn dl-probe-btn"
            :disabled="isImageSearching || isImageDownloading || !imageQuery.trim()"
            style="display:inline-flex;align-items:center;gap:5px;white-space:nowrap;height:38px;background:linear-gradient(135deg,#0891b2,#06b6d4)!important;">
            <template v-if="isImageSearching">
              <span style="display:inline-block;width:13px;height:13px;border:2px solid rgba(255,255,255,.3);border-top-color:#fff;border-radius:50%;animation:spin .9s linear infinite;"></span>
              Đang tìm...
            </template>
            <template v-else>
              <svg viewBox="0 0 24 24" width="14" height="14"><path fill="currentColor" d="M15.5 14h-.79l-.28-.27C15.41 12.59 16 11.11 16 9.5 16 5.91 13.09 3 9.5 3S3 5.91 3 9.5 5.91 16 9.5 16c1.61 0 3.09-.59 4.23-1.57l.27.28v.79l5 4.99L20.49 19l-4.99-5zm-6 0C7.01 14 5 11.99 5 9.5S7.01 5 9.5 5 14 7.01 14 9.5 11.99 14 9.5 14z"/></svg>
              Tìm Ảnh
            </template>
          </button>
        </div>
      </div>
      <div style="display:flex;gap:16px;align-items:flex-start;flex-wrap:wrap;margin-bottom:12px;">
        <div style="flex:1;min-width:220px;">
          <label style="font-size:11px;color:var(--l-text-muted);font-weight:600;display:block;margin-bottom:6px;text-transform:uppercase;letter-spacing:.06em;">Nguồn ảnh:</label>
          <div style="display:flex;gap:5px;flex-wrap:wrap;">
            <button v-for="src in IMAGE_SOURCES" :key="src.value" type="button" :title="src.hint"
              @click="imageSource = src.value"
              style="font-size:12px;padding:5px 12px;border-radius:7px;cursor:pointer;transition:all .2s;font-weight:600;outline:none;display:inline-flex;align-items:center;gap:4px;white-space:nowrap;"
              :style="{
                background: imageSource===src.value ? 'rgba(6,182,212,.15)' : 'rgba(255,255,255,.03)',
                border: imageSource===src.value ? '1px solid #06b6d4' : '1px solid rgba(255,255,255,.08)',
                color: imageSource===src.value ? '#22d3ee' : 'var(--l-text-muted)'
              }">
              {{ src.icon }} {{ src.label }}
              <span v-if="src.needsKey" style="font-size:9px;padding:1px 5px;background:rgba(245,158,11,.2);color:#fbbf24;border-radius:4px;border:1px solid rgba(245,158,11,.35);">Key</span>
            </button>
          </div>
          <p v-if="imageSelectedSource" style="font-size:11px;color:var(--l-text-muted);margin:5px 0 0;font-style:italic;line-height:1.4;">{{ imageSelectedSource.hint }}</p>
        </div>
        <div style="flex-shrink:0;">
          <label style="font-size:11px;color:var(--l-text-muted);font-weight:600;display:block;margin-bottom:6px;text-transform:uppercase;letter-spacing:.06em;">Số lượng:</label>
          <select v-model="imageMaxCount" class="dl-select" style="padding:7px 12px;font-size:13px;">
            <option :value="20">20 ảnh</option>
            <option :value="50">50 ảnh</option>
            <option :value="100">100 ảnh</option>
            <option :value="200">200 ảnh</option>
            <option :value="500">500 ảnh</option>
            <option :value="1000">1000 ảnh</option>
          </select>
        </div>
      </div>
      <div v-if="sourceNeedsKey" style="display:flex;align-items:center;gap:10px;margin-top:10px;margin-bottom:12px;padding:10px 14px;background:rgba(245,158,11,.06);border:1px solid rgba(245,158,11,.2);border-radius:8px;flex-wrap:wrap;">
        <svg viewBox="0 0 24 24" width="14" height="14" style="color:#fbbf24;flex-shrink:0;"><path fill="currentColor" d="M12.65 10C11.83 7.67 9.61 6 7 6c-3.31 0-6 2.69-6 6s2.69 6 6 6c2.61 0 4.83-1.67 5.65-4H17v4h4v-4h2v-4H12.65zM7 14c-1.1 0-2-.9-2-2s.9-2 2-2 2 .9 2 2-.9 2-2 2z"/></svg>
        <label style="font-size:12px;color:var(--l-text-muted);font-weight:600;white-space:nowrap;flex-shrink:0;">{{ imageSelectedSource?.label }} API Key:</label>
        <div style="flex:1;min-width:160px;display:flex;gap:6px;align-items:center;position:relative;">
          <input v-model="imageApiKey" type="password" class="dl-url-input" style="flex:1;height:34px;font-size:12.5px;" :placeholder="`Nhập ${imageSelectedSource?.label} API Key...`" />
          <!-- Badge: key từ .env -->
          <span v-if="imageHasEnvKey" style="position:absolute;right:8px;top:50%;transform:translateY(-50%);font-size:10.5px;font-weight:700;padding:2px 7px;border-radius:5px;background:rgba(16,185,129,.15);color:#10b981;border:1px solid rgba(16,185,129,.3);pointer-events:none;">✅ .env</span>
        </div>
        <a :href="imageSource==='pixabay'?'https://pixabay.com/api/docs/':imageSource==='unsplash'?'https://unsplash.com/developers':'https://www.pexels.com/api/'" target="_blank" style="font-size:12px;font-weight:600;color:#06b6d4;text-decoration:none;white-space:nowrap;flex-shrink:0;">Lấy key miễn phí →</a>
      </div>

      <!-- Results -->
      <div style="margin-top: 14px;">
        <div v-if="!imageSearchResult && !isImageSearching" style="display:flex;flex-direction:column;align-items:center;justify-content:center;gap:12px;height:200px;color:var(--l-text-muted);text-align:center;">
          <svg viewBox="0 0 24 24" width="56" height="56" style="opacity:.2;"><path fill="currentColor" d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/></svg>
          <p style="font-size:14.5px;font-weight:600;margin:0;">Nhập chủ đề → nhấn <span style="color:#06b6d4;">Tìm Ảnh</span> để bắt đầu</p>
          <p style="font-size:12px;opacity:.55;margin:0;">DuckDuckGo miễn phí không cần key • Hỗ trợ tiếng Việt & tiếng Anh</p>
        </div>
        <div v-if="isImageSearching" style="display:flex;flex-direction:column;align-items:center;justify-content:center;gap:14px;height:200px;color:var(--l-text-muted);">
          <div style="width:40px;height:40px;border:3px solid rgba(255,255,255,.1);border-top-color:#06b6d4;border-radius:50%;animation:spin .9s linear infinite;"></div>
          <p style="font-size:14px;font-weight:600;margin:0;">Đang tìm kiếm từ <span style="color:#22d3ee;">{{ imageSelectedSource?.label }}</span>...</p>
        </div>
        <template v-if="imageSearchResult && !isImageSearching">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;">
            <span style="font-size:13px;color:var(--l-text-muted);display:flex;align-items:center;gap:8px;">
              <strong style="color:var(--l-text);font-size:17px;">{{ imageAllEntries.length }}</strong> ảnh tìm thấy
              <span style="font-size:10.5px;font-weight:700;padding:2px 9px;border-radius:12px;background:rgba(6,182,212,.12);color:#22d3ee;text-transform:capitalize;">{{ imageSearchResult.source }}</span>
            </span>
            <button type="button" @click="imageToggleAll"
              style="font-size:12px;padding:5px 14px;border-radius:7px;cursor:pointer;transition:all .18s;font-weight:600;outline:none;background:rgba(255,255,255,.04);border:1px solid rgba(255,255,255,.1);color:var(--l-text-muted);display:inline-flex;align-items:center;gap:5px;">
              {{ imageSelectedCount === imageAllEntries.length ? "Bỏ tất cả" : "Chọn tất cả" }}
              <span style="font-weight:700;color:var(--l-text);">({{ imageSelectedCount }}/{{ imageAllEntries.length }})</span>
            </button>
          </div>
          <div style="display:grid;grid-template-columns:repeat(auto-fill,minmax(130px,1fr));gap:9px;max-height: 400px; overflow-y: auto; padding: 4px;">
            <div v-for="entry in imageAllEntries" :key="entry.id" @click="toggleImage(entry.id)"
              style="border-radius:9px;overflow:hidden;cursor:pointer;transition:all .18s;position:relative;"
              :style="{
                border: selectedImageIds.has(entry.id) ? '2px solid #06b6d4' : '2px solid rgba(255,255,255,.06)',
                background: 'rgba(255,255,255,.03)',
                transform: selectedImageIds.has(entry.id) ? 'translateY(-2px)' : 'translateY(0)',
                boxShadow: selectedImageIds.has(entry.id) ? '0 6px 20px rgba(6,182,212,.2)' : 'none'
              }">
              <div style="position:relative;aspect-ratio:4/3;overflow:hidden;background:rgba(0,0,0,.3);">
                <img :src="entry.thumbUrl || entry.url" :alt="entry.title || 'anh'" loading="lazy"
                  style="width:100%;height:100%;object-fit:cover;display:block;transition:transform .25s;"
                  @error="($event.target as HTMLImageElement).style.display='none'"
                  @mouseover="($event.target as HTMLImageElement).style.transform='scale(1.07)'"
                  @mouseleave="($event.target as HTMLImageElement).style.transform='scale(1)'" />
                <div v-if="selectedImageIds.has(entry.id)"
                  style="position:absolute;top:6px;right:6px;width:22px;height:22px;border-radius:50%;background:#06b6d4;display:flex;align-items:center;justify-content:center;font-size:13px;font-weight:800;color:white;box-shadow:0 2px 8px rgba(0,0,0,.5);">
                  <svg viewBox="0 0 24 24" width="13" height="13"><path fill="currentColor" d="M9 16.17L4.83 12l-1.42 1.41L9 19 21 7l-1.41-1.41z"/></svg>
                </div>
              </div>
              <div v-if="entry.author" style="padding:5px 7px;">
                <p style="font-size:10px;color:var(--l-text-muted);margin:0;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;">by {{ entry.author }}</p>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>

    <!-- Log strip -->
    <div v-if="imageSearchLog.length > 0" style="padding:7px 0;border-top:1px solid rgba(255,255,255,0.05);font-size:11px;line-height:1.4;color:var(--l-text-muted);font-family:monospace;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;flex-shrink:0;">
      {{ imageSearchLog[0] }}
    </div>

    <!-- Footer -->
    <div class="modal-footer" style="flex-direction:column;gap:10px;padding: 16px 0 0 0; border-top: 1px solid rgba(255,255,255,0.06); background: transparent;">
      <div style="display:flex;align-items:center;gap:8px; width: 100%;">
        <svg viewBox="0 0 24 24" width="14" height="14" style="color:var(--l-text-muted);flex-shrink:0;"><path fill="currentColor" d="M10 4H4c-1.1 0-1.99.9-1.99 2L2 18c0 1.1.9 2 2 2h16c1.1 0 2-.9 2-2V8c0-1.1-.9-2-2-2h-8l-2-2z"/></svg>
        <span style="font-size:12px;color:var(--l-text-muted);font-weight:600;white-space:nowrap;flex-shrink:0;">Lưu vào:</span>
        <input v-model="imageDir" class="dl-url-input" style="flex:1;height:34px;font-size:12px;cursor:default;" readonly placeholder="Thư mục lưu ảnh..." />
        <button @click="pickImageDir" style="padding:6px 12px;font-size:15px;background:rgba(255,255,255,.05);border:1px solid rgba(255,255,255,.1);border-radius:7px;color:var(--l-text);cursor:pointer;flex-shrink:0;">&#128193;</button>
      </div>
      <div v-if="isImageDownloading" style="display:flex;align-items:center;gap:10px; width: 100%;">
        <div style="flex:1;height:5px;background:rgba(255,255,255,.08);border-radius:3px;overflow:hidden;">
          <div :style="{width: imageDownloadProgress+'%'}" style="height:100%;background:linear-gradient(90deg,#0891b2,#06b6d4);border-radius:3px;transition:width .4s ease;"></div>
        </div>
        <span style="font-size:12px;color:var(--l-text-muted);font-weight:700;white-space:nowrap;">{{ downloadDoneCount }}/{{ downloadTotalCount }} ảnh</span>
      </div>
      <div style="display:flex;justify-content:flex-end;gap:10px; width: 100%;">
        <button v-if="isImageDownloading" @click="cancelImageDl"
          style="padding:8px 18px;border-radius:8px;font-size:13px;font-weight:600;cursor:pointer;transition:all .2s;background:rgba(255,255,255,.05);border:1px solid rgba(239,68,68,.3);color:#f87171;">
          &#9209; Dừng tải</button>
        <button @click="startImageDownload"
          :disabled="isImageDownloading || isImageSearching || imageSelectedCount === 0"
          style="padding:8px 24px;border-radius:8px;font-size:13.5px;font-weight:700;cursor:pointer;transition:all .2s;border:none;display:inline-flex;align-items:center;gap:6px;"
          :style="{
            background: (isImageDownloading||isImageSearching||imageSelectedCount===0)?'rgba(255,255,255,.07)':'linear-gradient(135deg,#0891b2,#06b6d4)',
            color: (isImageDownloading||isImageSearching||imageSelectedCount===0)?'rgba(255,255,255,.25)':'white',
            boxShadow: (isImageDownloading||isImageSearching||imageSelectedCount===0)?'none':'0 4px 14px rgba(6,182,212,.4)',
            cursor: (isImageDownloading||isImageSearching||imageSelectedCount===0)?'not-allowed':'pointer'
          }">
          <svg v-if="!isImageDownloading" viewBox="0 0 24 24" width="15" height="15"><path fill="currentColor" d="M5 20h14v-2H5v2zM12 2L4 10h5v6h6v-6h5L12 2z"/></svg>
          <span v-if="isImageDownloading">Đang tải {{ downloadDoneCount }}/{{ downloadTotalCount }}...</span>
          <span v-else-if="imageSelectedCount===0">Chọn ảnh để tải</span>
          <span v-else>Tải {{ imageSelectedCount }} ảnh đã chọn</span>
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped src="./VideoSplitter.scoped.css"></style>
<style src="./VideoSplitter.global.css"></style>
