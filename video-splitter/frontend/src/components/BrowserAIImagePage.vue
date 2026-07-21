<template>
  <div class="browser-ai-page">
    <!-- Header -->
    <div class="dl-page-header">
      <h2 class="dl-page-title">
        <Sparkles class="dl-title-icon" :size="20" />
        <span>Tạo Ảnh bằng Google AI</span>
        <span class="dl-title-sub">Thử nghiệm — phụ thuộc giao diện Google</span>
      </h2>
    </div>

    <!-- Main Workspace (Grid layout: Left for config, Right for progress / preview) -->
    <div class="workspace-grid">
      
      <!-- Left Column: Config Form -->
      <div class="config-panel" :class="{ 'panel-disabled': isGenerating }">
        <h3 class="panel-section-title">Cấu hình yêu cầu</h3>

        <div class="form-row" v-if="provider === 'flow'">
          <label class="img-label">Model Google AI:</label>
          <div class="img-source-pills">
            <button v-for="m in modelsList" :key="m" :class="{ active: selectedModel === m }" @click="selectedModel = m" class="img-source-pill">
              {{ m }}
            </button>
          </div>
        </div>

        <div class="form-row">
          <label class="img-label">Tỷ lệ khung hình:</label>
          <div class="img-source-pills">
            <button v-for="r in ratios" :key="r" :class="{ active: aspectRatio === r }" @click="aspectRatio = r" class="img-source-pill">
              {{ r }}
            </button>
          </div>
        </div>

        <div class="form-row" v-if="provider === 'flow'">
          <label class="img-label">Số lượng sinh mỗi lần:</label>
          <div class="img-source-pills">
            <button v-for="b in ['1x', '2x', '3x', '4x']" :key="b" :class="{ active: selectedBatchSize === b }" @click="selectedBatchSize = b" class="img-source-pill">
              {{ b }}
            </button>
          </div>
        </div>

        <div class="form-row" v-if="provider === 'flow'">
          <label class="img-label">Độ phân giải tải về:</label>
          <div class="img-source-pills">
            <button v-for="res in ['1K', '2K', '4K']" :key="res" :class="{ active: selectedResolution === res }" @click="selectedResolution = res" class="img-source-pill">
              {{ res }}
            </button>
          </div>
        </div>

        <div style="display: grid; grid-template-columns: 1.2fr 0.8fr; gap: var(--wx-space-2);">
          <div class="form-row">
            <label class="img-label">Thư mục lưu:</label>
            <div style="display: flex; gap: var(--wx-space-1);">
              <input type="text" v-model="outputDir" class="img-text-input read-only-input" readonly style="flex: 1;" />
              <button @click="pickOutputDir" class="img-dir-btn" style="height: 38px; width: 38px; padding: 0;" title="Chọn thư mục lưu">
                <FolderOpen :size="14" />
              </button>
            </div>
          </div>

          <div class="form-row">
            <label class="img-label">Tên file lưu trữ:</label>
            <input type="text" v-model="fileName" placeholder="Tên file..." class="img-text-input" />
          </div>
        </div>

        <div class="form-row prompt-row">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 2px;">
            <label class="img-label" style="margin-bottom: 0;">Mô tả chi tiết nội dung (Prompt):</label>
            <div style="display: flex; align-items: center; gap: 8px;">
              <button type="button" @click="chooseImageFile" class="img-source-pill" style="padding: 2px 6px; font-size: 11px; height: 22px;" :title="'Thêm ảnh làm đầu vào (tối đa 3 ảnh, hiện có ' + inputImages.length + '/3)'">
                <ImageIcon :size="11" style="margin-right: 3px;" /> Thêm ảnh ({{ inputImages.length }}/3)
              </button>
              <span class="char-counter" :class="{ limit: prompt.length > 2000 }">{{ prompt.length }}/2000</span>
            </div>
          </div>
          <!-- Bật/tắt tự động tải + gợi ý mỗi dòng 1 prompt -->
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; gap: 8px;">
            <!-- <span style="font-size: 11px; color: var(--l-text-muted);">
              Mỗi prompt cách nhau 1 dòng trống ({{ promptLines.length }} prompt) — chạy song song.
            </span> -->
            <label class="toggle-row inline" style="font-size: 11.5px; font-weight: 600; display: flex; align-items: center; gap: 6px; cursor: pointer; user-select: none; margin-bottom: 0; white-space: nowrap;" title="Bật: tự động tải mọi ảnh sinh ra. Tắt (chỉ khi 1 prompt): hiện ảnh để bạn chọn rồi mới tải.">
              <input type="checkbox" v-model="autoDownload" style="width:15px; height:15px;" />
              Tự động tải ảnh
            </label>
          </div>
          <div style="position: relative; flex: 1; display: flex; flex-direction: column;">
            <textarea
              v-model="prompt"
              @paste="handlePasteImage"
              placeholder="Mỗi prompt cách nhau bằng 1 DÒNG TRỐNG — Google AI tạo song song mỗi prompt thành 1 ảnh. Một prompt có thể dài nhiều dòng.&#10;Ví dụ:&#10;một chú mèo phi hành gia&#10;trôi giữa dải ngân hà&#10;&#10;thành phố cyberpunk về đêm&#10;mưa neon phản chiếu&#10;&#10;(Mẹo: nhấn Ctrl+V để dán ảnh làm đầu vào)"
              class="img-text-input prompt-textarea"
              maxlength="2000"
              style="padding-bottom: 56px;"
            ></textarea>

            <!-- Previews inside textarea -->
            <div v-if="inputImages.length > 0" style="position: absolute; bottom: 8px; left: 8px; display: flex; gap: 8px; pointer-events: auto; z-index: 10;">
              <div v-for="img in inputImages" :key="img.id" style="position: relative; width: 40px; height: 40px; border-radius: 4px; border: 1px solid var(--wx-border-default); background: #000; box-shadow: 0 2px 6px rgba(0,0,0,0.4);">
                <img :src="img.preview" style="width: 100%; height: 100%; object-fit: cover; border-radius: 3px;" />
                <button type="button" @click="removeSelectedImage(img.id)" style="position: absolute; top: -5px; right: -5px; width: 13px; height: 13px; border-radius: 50%; background: var(--wx-danger-solid); color: #fff; border: none; display: flex; align-items: center; justify-content: center; cursor: pointer; padding: 0; box-shadow: 0 1px 2px rgba(0,0,0,0.3); font-size: 8px; font-weight: bold;" title="Xóa ảnh">
                  ×
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Right Column: Status / Progress / Preview -->
      <div class="preview-panel">
        
        <!-- Idle State (Wait for generation) -->
        <div v-if="!queueMode && (state === 'idle' || state === 'cancelled')" class="state-placeholder">
          <Sparkles :size="48" class="placeholder-decor" />
          <h4 class="state-title">Sẵn sàng tạo ảnh AI</h4>
          <p class="state-description">
            Điền mô tả bên trái sau đó nhấn nút <strong>"Bắt đầu tạo"</strong> bên dưới để bắt đầu luồng tự động hóa trình duyệt.
          </p>
          <button @click="startGeneration" :disabled="promptLines.length === 0" class="img-action-btn start-generate-btn">
            <Play :size="14" /> Bắt đầu tạo{{ promptLines.length > 1 ? ` (${promptLines.length} prompt song song)` : '' }}
          </button>
        </div>

        <!-- Queue Mode State (Chạy nhiều prompt song song qua Hàng Đợi AI) -->
        <div v-if="queueMode" class="completed-panel" style="display: flex; flex-direction: column; height: 100%; overflow: hidden; min-height: 0; width: 100%;">
          <div class="preview-header" style="display: flex; justify-content: space-between; align-items: center; width: 100%;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <Sparkles :size="20" style="color: var(--wx-brand-accent);" />
              <span class="success-title">
                {{ queueStatus.isRunning ? 'Đang tạo ảnh song song...' : 'Hoàn tất Hàng Đợi AI' }}
                ({{ queueStatus.completed + queueStatus.failed }}/{{ queueStatus.total }})
              </span>
            </div>
            <button v-if="queueStatus.isRunning" @click="cancelQueue" class="img-source-pill" style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px; color: var(--wx-danger-solid);">
              <StopCircle :size="12" style="margin-right: 3px;" /> Dừng
            </button>
            <button v-else @click="resetForm" class="img-source-pill active" style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px;">
              <RefreshCw :size="12" style="margin-right: 3px;" /> Tạo mới
            </button>
          </div>

          <div style="padding: 6px 0 10px; width: 100%;">
            <div style="height: 8px; border-radius: 4px; background: var(--wx-glass-light-bg); overflow: hidden;">
              <div :style="{ width: queueStatus.total > 0 ? ((queueStatus.completed + queueStatus.failed) / queueStatus.total * 100) + '%' : '0%', height: '100%', background: 'var(--wx-brand-accent)', transition: 'width 0.3s' }"></div>
            </div>
            <div style="font-size: 11.5px; color: var(--l-text-muted); margin-top: 6px;">
              Thành công: {{ queueStatus.completed }} · Lỗi: {{ queueStatus.failed }} · Tổng: {{ queueStatus.total }}
            </div>
          </div>

          <div style="flex: 1; overflow-y: auto; min-height: 0; padding: 4px 0; width: 100%;">
            <div class="img-grid" style="display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; width: 100%; justify-items: center;">
              <div
                v-for="(rPath, rIdx) in queueResultPaths"
                :key="rIdx"
                class="img-card"
                style="display: flex; flex-direction: column; align-items: center; width: 100%; max-width: 160px; background: var(--wx-glass-light-bg); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); padding: 6px; box-shadow: var(--wx-shadow-md);"
              >
                <img :src="rPath" style="width: 100%; height: 160px; object-fit: cover; border-radius: var(--wx-radius-sm);" draggable="false" />
              </div>
            </div>
          </div>
        </div>

        <!-- Selection Required State (Choose which images to download) -->
        <div v-if="state === 'selection_required'" class="completed-panel" style="display: flex; flex-direction: column; height: 100%; overflow: hidden; min-height: 0; width: 100%;">
          <div class="preview-header" style="display: flex; justify-content: space-between; align-items: center; width: 100%;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <Sparkles :size="20" style="color: var(--wx-brand-accent);" />
              <span class="success-title">
                Đã sinh xong! Chọn ảnh muốn tải về máy
              </span>
            </div>
            <button type="button" @click="toggleSelectAllPreviews" class="img-source-pill active" style="margin-left: auto; font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px;">
              {{ selectedPreviewIndexes.size === selectionPreviews.length ? 'Bỏ tất cả' : 'Chọn tất cả' }}
              <span style="margin-left: 2px;">({{ selectedPreviewIndexes.size }}/{{ selectionPreviews.length }})</span>
            </button>
          </div>

          <!-- Previews Area with Drag-select support -->
          <div style="flex: 1; overflow-y: auto; min-height: 0; padding: 10px 0; width: 100%;">
            <div
              ref="gridRef"
              class="img-grid"
              @mousedown.left="onGridMouseDown"
              style="position: relative; user-select: none; display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; width: 100%; justify-items: center;"
            >
              <!-- Drag-select overlay box -->
              <div v-if="dragBox.active" class="img-drag-box" :style="dragBoxStyle"></div>

              <div
                v-for="(pUrl, pIdx) in selectionPreviews"
                :key="pIdx"
                class="img-card"
                :class="{ selected: selectedPreviewIndexes.has(pIdx) }"
                :data-preview-idx="pIdx"
                @click="togglePreviewImage(pIdx)"
                style="display: flex; flex-direction: column; align-items: center; width: 100%; max-width: 160px; background: var(--wx-glass-light-bg); backdrop-filter: blur(var(--wx-glass-light-blur)); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); padding: 6px; box-shadow: var(--wx-shadow-md); cursor: pointer; transition: all var(--wx-d-fast) var(--wx-ease-standard); position: relative;"
              >
                <img :src="pUrl" style="width: 100%; height: 160px; object-fit: cover; border-radius: var(--wx-radius-sm);" draggable="false" />
                
                <div v-if="selectedPreviewIndexes.has(pIdx)" class="img-card-check" style="position: absolute; top: 12px; right: 12px; background: var(--wx-brand-primary); color: var(--wx-text-inverse); border-radius: var(--wx-radius-full); width: 22px; height: 22px; display: flex; align-items: center; justify-content: center; box-shadow: var(--wx-shadow-sm); border: 1px solid var(--wx-text-inverse); z-index: 10;">
                  <Check :size="12" />
                </div>
              </div>
            </div>
          </div>

          <!-- Bottom Action Row -->
          <div class="completed-actions" style="flex-shrink: 0; margin-top: 10px; display: flex; gap: 8px; align-items: center; width: 100%;">
            <button @click="cancel" class="img-dir-btn" style="height: 38px;">
              <StopCircle :size="12" /> Hủy bỏ
            </button>
            <span style="font-size: var(--wx-fs-12); color: var(--wx-text-muted); flex: 1; text-align: left;">
              Lưu vào: <code>{{ outputDir }}</code>
            </span>
            <button
              @click="confirmDownload"
              :disabled="selectedPreviewIndexes.size === 0"
              class="img-action-btn apply-btn"
              style="height: 38px; font-weight: bold; background: var(--wx-brand-primary); color: var(--wx-text-inverse);"
            >
              <Download :size="14" style="margin-right: 4px;" />
              Tải {{ selectedPreviewIndexes.size }} ảnh đã chọn
            </button>
          </div>
        </div>

        <!-- Generating / Progress State -->
        <div v-else-if="isGenerating" class="progress-panel">
          <div class="spinner-container">
            <div class="dual-ring-spinner"></div>
            <div class="elapsed-timer">Thời gian chạy: {{ formattedElapsed }}</div>
          </div>

          <div class="stage-info">
            <span class="stage-msg">{{ message }}</span>
            <span class="stage-percent">{{ Math.floor(smoothProgress) }}%</span>
          </div>

          <div class="img-progress-track">
            <div class="img-progress-fill" :style="{ width: smoothProgress + '%' }"></div>
          </div>

          <button @click="cancel" class="img-action-btn cancel-btn">
            <StopCircle :size="14" /> Hủy bỏ tác vụ
          </button>
        </div>

        <!-- Login Required panel -->
        <div v-else-if="state === 'login_required'" class="login-required-panel">
          <AlertCircle :size="40" class="login-alert-icon" />
          <h3 class="login-title">Yêu cầu Đăng nhập Google</h3>
          <p class="login-desc">
            Vui lòng hoàn tất quá trình đăng nhập tài khoản Google của bạn trên cửa sổ Chrome vừa được mở.
          </p>
          <div style="display: flex; gap: var(--wx-space-2); justify-content: center;">
            <button @click="openBrowser(provider, props.showChrome)" class="img-dir-btn">
              <Chrome :size="14" style="margin-right: 4px;" /> Mở lại Chrome
            </button>
            <button @click="checkLogin(provider)" class="img-action-btn check-login-btn">
              Xác nhận đã đăng nhập
            </button>
          </div>
        </div>

        <!-- Failed State -->
        <div v-else-if="state === 'failed'" class="failed-panel">
          <AlertCircle :size="40" class="failed-icon" />
          <h3 class="failed-title">Tạo thất bại</h3>
          <p class="failed-desc">{{ error }}</p>
          <button @click="resetForm" class="img-dir-btn">
            <RefreshCw :size="12" style="margin-right: 4px;" /> Thử lại
          </button>
        </div>

        <!-- Success Preview panel -->
        <div v-else-if="state === 'completed' && result" class="completed-panel" style="display: flex; flex-direction: column; height: 100%; overflow: hidden; min-height: 0;">
          <div class="preview-header" style="display: flex; justify-content: space-between; align-items: center; width: 100%; padding-bottom: 4px; flex-shrink: 0;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <CheckCircle :size="18" class="success-check-icon" />
              <span class="success-title">
                {{ (previewURLs.length > 0 ? previewURLs : (previewURL ? [previewURL] : [])).length > 1 ? `Đã tạo thành công ${(previewURLs.length > 0 ? previewURLs : [previewURL]).length} file!` : `Đã tạo thành công file: ${result.fileName}` }}
              </span>
            </div>
            <button 
              v-if="(previewURLs.length > 0 ? previewURLs : (previewURL ? [previewURL] : [])).length > 1" 
              type="button" 
              @click="toggleSelectAllCompleted" 
              class="img-source-pill active" 
              style="margin-left: auto; font-size: 11px; height: 26px; flex: none; width: auto; padding: 0 10px;"
            >
              {{ selectedCompletedIndexes.size === (previewURLs.length > 0 ? previewURLs : [previewURL]).length ? 'Bỏ tất cả' : 'Chọn tất cả' }}
              <span style="margin-left: 2px;">({{ selectedCompletedIndexes.size }}/{{ (previewURLs.length > 0 ? previewURLs : [previewURL]).length }})</span>
            </button>
          </div>

          <!-- Previews Compact Grid Area with Drag-select & Card Selection -->
          <div style="flex: 1; overflow-y: auto; min-height: 0; padding: 10px 4px; width: 100%;">
            <div
              ref="gridRef"
              class="img-grid"
              @mousedown.left="onGridMouseDown"
              style="position: relative; user-select: none; display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; width: 100%; justify-items: center;"
            >
              <!-- Drag-select overlay box -->
              <div v-if="dragBox.active" class="img-drag-box" :style="dragBoxStyle"></div>

              <div 
                v-for="(pUrl, pIdx) in (previewURLs.length > 0 ? previewURLs : (previewURL ? [previewURL] : []))" 
                :key="pIdx" 
                class="img-card" 
                :class="{ selected: selectedCompletedIndexes.has(pIdx) }"
                :data-preview-idx="pIdx"
                @click="toggleCompletedImage(pIdx)"
                style="display: flex; flex-direction: column; align-items: center; width: 100%; max-width: 160px; background: var(--wx-glass-light-bg); backdrop-filter: blur(var(--wx-glass-light-blur)); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); padding: 6px; box-shadow: var(--wx-shadow-md); cursor: pointer; transition: all var(--wx-d-fast) var(--wx-ease-standard); position: relative;"
              >
                <img :src="pUrl" style="width: 100%; height: 160px; object-fit: cover; border-radius: var(--wx-radius-sm);" draggable="false" />

                <!-- Checkmark Overlay Badge -->
                <div v-if="selectedCompletedIndexes.has(pIdx)" class="img-card-check" style="position: absolute; top: 10px; right: 10px; background: var(--wx-brand-primary); color: var(--wx-text-inverse); border-radius: var(--wx-radius-full); width: 22px; height: 22px; display: flex; align-items: center; justify-content: center; box-shadow: var(--wx-shadow-sm); border: 1px solid var(--wx-text-inverse); z-index: 10;">
                  <Check :size="12" />
                </div>
              </div>
            </div>
          </div>

          <!-- Single Row Bottom Action & Info Bar -->
          <div class="completed-actions" style="flex-shrink: 0; margin-top: 8px; display: flex; gap: 8px; align-items: center; width: 100%;">
            <button @click="resetForm" class="img-dir-btn" style="height: 38px;">
              <RefreshCw :size="12" style="margin-right: 4px;" /> Tạo lại
            </button>

            <span style="font-size: var(--wx-fs-12); color: var(--wx-text-muted); flex: 1; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
              <template v-if="(previewURLs.length > 0 ? previewURLs : [previewURL]).length <= 1">
                Đường dẫn: <code style="color: var(--wx-brand-accent);">{{ result.filePath }}</code> ({{ formatSize(result.fileSize) }})
              </template>
              <template v-else>
                Lưu vào: <code style="color: var(--wx-brand-accent);">{{ outputDir }}</code> (Đã chọn {{ selectedCompletedIndexes.size }}/{{ (previewURLs.length > 0 ? previewURLs : [previewURL]).length }} ảnh - {{ formatSize(result.fileSize) }})
              </template>
            </span>

            <button @click="openFolder" class="img-dir-btn" style="height: 38px;">
              <FolderOpen :size="12" style="margin-right: 4px;" /> Mở thư mục
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted, onUnmounted, computed } from 'vue'
import { useBrowserAI } from './composables/useBrowserAI'
import { Sparkles, AlertCircle, FolderOpen, Chrome, Play, StopCircle, Trash2, CheckCircle, RefreshCw, ImageIcon, Check, Download } from 'lucide-vue-next'
// @ts-ignore
import { SelectFolder, GetStreamURL, GetGlobalSettings, SaveGlobalSettings } from '../../wailsjs/go/main/App'
// @ts-ignore
import { OpenOutputFolder, EnqueueThumbnailTasks, ClearQueue, CancelQueue } from '../../wailsjs/go/browserai/Service'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const mediaType = 'image'

const props = defineProps<{
  defaultOutputDir: string
  showChrome: boolean
}>()

const emit = defineEmits<{
  (e: 'back'): void
  (e: 'apply-image', path: string): void
  (e: 'show-toast', msg: string, type: 'success' | 'error' | 'warning' | 'info'): void
}>()

const showToast = (msg: string, type: 'success' | 'error' | 'warning' | 'info') => {
  emit('show-toast', msg, type)
}

const {
  taskId,
  state,
  progress,
  message,
  error,
  result,
  selectionPreviews,
  browserOpen,
  browserProvider,
  isGenerating,
  openBrowser,
  checkLogin,
  generate,
  submitSelection,
  cancel,
  clearBrowserProfile,
  closeBrowser,
  reset,
  updateBrowserStatus
} = useBrowserAI(showToast)

const selectedPreviewIndexes = ref<Set<number>>(new Set())
const selectedCompletedIndexes = ref<Set<number>>(new Set([0]))

function toggleCompletedImage(idx: number) {
  if (selectedCompletedIndexes.value.has(idx)) {
    selectedCompletedIndexes.value.delete(idx)
  } else {
    selectedCompletedIndexes.value.add(idx)
  }
}

function toggleSelectAllCompleted() {
  const list = (previewURLs.value && previewURLs.value.length > 0) ? previewURLs.value : (previewURL.value ? [previewURL.value] : [])
  if (selectedCompletedIndexes.value.size === list.length) {
    selectedCompletedIndexes.value.clear()
  } else {
    selectedCompletedIndexes.value = new Set(list.map((_, i) => i))
  }
}

// Drag-to-select logic
const gridRef = ref<HTMLElement | null>(null)
const dragBox = reactive({ active: false, x1: 0, y1: 0, x2: 0, y2: 0 })
const dragBoxStyle = ref<Record<string, string>>({})
let dragStartSelectedIndices = new Set<number>()
let dragMode: 'select' | 'deselect' | null = null

function togglePreviewImage(idx: number) {
  if (selectedPreviewIndexes.value.has(idx)) {
    selectedPreviewIndexes.value.delete(idx)
  } else {
    selectedPreviewIndexes.value.add(idx)
  }
}

function toggleSelectAllPreviews() {
  if (selectedPreviewIndexes.value.size === selectionPreviews.value.length) {
    selectedPreviewIndexes.value.clear()
  } else {
    selectedPreviewIndexes.value = new Set(selectionPreviews.value.map((_, i) => i))
  }
}

// Auto select all newly generated previews
watch(selectionPreviews, (newPreviews) => {
  if (newPreviews && newPreviews.length > 0) {
    selectedPreviewIndexes.value = new Set(newPreviews.map((_, i) => i))
  } else {
    selectedPreviewIndexes.value.clear()
  }
}, { immediate: true })

const smoothProgress = ref(0)
let smoothInterval: any = null

watch(() => progress.value, (newVal) => {
  if (smoothInterval) {
    clearInterval(smoothInterval)
    smoothInterval = null
  }
  
  smoothProgress.value = newVal
  
  if (newVal === 50) {
    smoothInterval = setInterval(() => {
      if (smoothProgress.value < 83) {
        smoothProgress.value += 0.5
      }
    }, 1000)
  } else if (newVal === 10) {
    smoothInterval = setInterval(() => {
      if (smoothProgress.value < 34) {
        smoothProgress.value += 1
      }
    }, 800)
  }
}, { immediate: true })

function onGridMouseDown(e: MouseEvent) {
  if (e.button !== 0) return
  const target = e.target as HTMLElement
  if (target.closest('button, input, a, select')) return

  const grid = gridRef.value
  if (!grid) return

  dragMode = null
  dragStartSelectedIndices = new Set(state.value === 'completed' ? selectedCompletedIndexes.value : selectedPreviewIndexes.value)

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
  const currentSet = state.value === 'completed' ? selectedCompletedIndexes.value : selectedPreviewIndexes.value

  const cards = grid.querySelectorAll<HTMLElement>('[data-preview-idx]')
  cards.forEach(card => {
    const cr = card.getBoundingClientRect()
    const cardX1 = cr.left - gridRect.left + grid.scrollLeft
    const cardY1 = cr.top - gridRect.top + grid.scrollTop
    const cardX2 = cardX1 + cr.width
    const cardY2 = cardY1 + cr.height

    const overlaps = cardX1 < selX2 && cardX2 > selX1 && cardY1 < selY2 && cardY2 > selY1
    const idx = parseInt(card.dataset.previewIdx!)

    if (overlaps) {
      if (dragMode === null) {
        dragMode = dragStartSelectedIndices.has(idx) ? 'deselect' : 'select'
      }

      if (dragMode === 'select') currentSet.add(idx)
      if (dragMode === 'deselect') currentSet.delete(idx)
    } else {
      const wasSelected = dragStartSelectedIndices.has(idx)
      if (wasSelected) {
        currentSet.add(idx)
      } else {
        currentSet.delete(idx)
      }
    }
  })
}

const confirmDownload = () => {
  submitSelection(Array.from(selectedPreviewIndexes.value))
}

const provider = computed<'gemini' | 'flow'>(() => {
  return 'flow'
})

const prompt = ref('')
const ratios = ['16:9', '4:3', '1:1', '3:4', '9:16']
const aspectRatio = ref('1:1')
const outputDir = ref('')
const fileName = ref('')
const previewURL = ref('')
const previewURLs = ref<string[]>([])

// Tự động tải: BẬT → tự tải mọi ảnh sinh ra (không cần chọn tay).
// TẮT → chế độ cũ: hiện preview cho người dùng chọn rồi mới tải (chỉ áp dụng khi 1 prompt).
const autoDownload = ref(true)

// Mỗi prompt cách nhau bằng 1 DÒNG TRỐNG (mỗi prompt có thể dài nhiều dòng).
// Tách theo cụm ≥1 dòng trống → 1 task riêng; gộp các dòng trong cùng cụm lại,
// bỏ khoảng trắng thừa và cụm rỗng.
const promptLines = computed(() =>
  prompt.value
    .split(/\n\s*\n/)
    .map(block => block.split('\n').map(l => l.trim()).filter(l => l !== '').join('\n').trim())
    .filter(block => block !== '')
)

// Trạng thái Hàng Đợi AI khi chạy nhiều prompt song song trên trang này.
const queueMode = ref(false)
const queueStatus = reactive({ total: 0, completed: 0, failed: 0, isRunning: false })
const queueResultPaths = ref<string[]>([])

watch(() => result.value, () => {
  const list = (previewURLs.value && previewURLs.value.length > 0) ? previewURLs.value : (previewURL.value ? [previewURL.value] : [])
  selectedCompletedIndexes.value = new Set(list.map((_, i) => i))
}, { immediate: true })

interface InputImage {
  id: string
  path: string
  base64: string
  preview: string
}
const inputImages = ref<InputImage[]>([])

const handlePasteImage = async (e: ClipboardEvent) => {
  const items = e.clipboardData?.items as any
  if (!items) return
  for (let i = 0; i < items.length; i++) {
    const item = items[i]
    if (item.type.indexOf('image') !== -1) {
      const file = item.getAsFile()
      if (file) {
        if (inputImages.value.length >= 3) {
          showToast("Chỉ được thêm tối đa 3 ảnh làm đầu vào!", "warning")
          e.preventDefault()
          return
        }
        const reader = new FileReader()
        reader.onload = async (event) => {
          const base64 = event.target?.result as string
          inputImages.value.push({
            id: `paste_${Date.now()}_${Math.random()}`,
            path: '',
            base64: base64,
            preview: base64
          })
          showToast("Đã dán ảnh từ clipboard!", "success")
        }
        reader.readAsDataURL(file)
        e.preventDefault()
      }
    }
  }
}

import { SelectImageFiles } from '../../wailsjs/go/main/App'

const chooseImageFile = async () => {
  if (inputImages.value.length >= 3) {
    showToast("Chỉ được thêm tối đa 3 ảnh làm đầu vào!", "warning")
    return
  }
  try {
    const paths = await SelectImageFiles()
    if (paths && paths.length > 0) {
      let addedCount = 0
      for (const path of paths) {
        if (inputImages.value.length >= 3) {
          showToast("Chỉ chọn được tối đa 3 ảnh, các ảnh thừa đã bị bỏ qua.", "warning")
          break
        }
        if (inputImages.value.some(img => img.path === path)) {
          continue
        }
        const url = await GetStreamURL(path)
        inputImages.value.push({
          id: `file_${Date.now()}_${Math.random()}`,
          path: path,
          base64: '',
          preview: url
        })
        addedCount++
      }
      if (addedCount > 0) {
        showToast(`Đã thêm ${addedCount} ảnh thành công!`, "success")
      }
    }
  } catch (err) {
    showToast("Lỗi chọn ảnh: " + err, "error")
  }
}

const removeSelectedImage = (id: string) => {
  inputImages.value = inputImages.value.filter(img => img.id !== id)
}

const getFilename = (path: string) => {
  if (!path) return ""
  const parts = path.split(/[/\\]/)
  return parts[parts.length - 1]
}

const selectedModel = ref('Nano Banana 2')
const selectedBatchSize = ref('1x')
const confirmBeforeCreate = ref('never')
const selectedResolution = ref('1K')
const modelsList = ['Nano Banana 2', 'Nano Banana Pro', 'Nano Banana 2 Lite']

// Elapsed time counter
const elapsedSeconds = ref(0)
let timerId: any = null

const formattedElapsed = computed(() => {
  const m = Math.floor(elapsedSeconds.value / 60)
  const s = elapsedSeconds.value % 60
  return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
})

watch(isGenerating, (val) => {
  if (val) {
    elapsedSeconds.value = 0
    timerId = setInterval(() => {
      elapsedSeconds.value++
    }, 1000)
  } else {
    if (timerId) {
      clearInterval(timerId)
      timerId = null
    }
  }
})

// Resolve stream URL for previews
watch(result, async (res) => {
  previewURLs.value = []
  previewURL.value = ""
  if (res && res.filePaths && res.filePaths.length > 0) {
    for (const path of res.filePaths) {
      try {
        const url = await GetStreamURL(path)
        previewURLs.value.push(url)
      } catch (_) {
         previewURLs.value.push("")
      }
    }
    if (previewURLs.value.length > 0) {
      previewURL.value = previewURLs.value[0]
    }
  } else if (res && res.filePath) {
    try {
      const url = await GetStreamURL(res.filePath)
      previewURLs.value.push(url)
      previewURL.value = url
    } catch (_) {
      previewURLs.value.push("")
    }
  }
})

watch(() => props.defaultOutputDir, (newDir) => {
  if (newDir) {
    outputDir.value = newDir
  }
})

const isSettingsLoaded = ref(false)

const saveBrowserAISettings = async () => {
  if (!isSettingsLoaded.value) return
  try {
    const settingsStr = await GetGlobalSettings()
    let gSettings: any = {}
    if (settingsStr) {
      gSettings = JSON.parse(settingsStr)
    }
    
    const suffix = 'image'
    gSettings[`browserAIModel_${suffix}`] = selectedModel.value
    gSettings[`browserAIAspectRatio_${suffix}`] = aspectRatio.value
    gSettings[`browserAIBatchSize_${suffix}`] = selectedBatchSize.value
    gSettings[`browserAIResolution_${suffix}`] = selectedResolution.value
    gSettings[`browserAIConfirmBeforeCreate_${suffix}`] = confirmBeforeCreate.value
    
    await SaveGlobalSettings(JSON.stringify(gSettings))
  } catch (err) {
    console.error("Lỗi tự động lưu cấu hình BrowserAI:", err)
  }
}

let saveSettingsTimeout: any = null
watch(
  [selectedModel, aspectRatio, selectedBatchSize, selectedResolution, confirmBeforeCreate],
  () => {
    if (!isSettingsLoaded.value) return
    if (saveSettingsTimeout) clearTimeout(saveSettingsTimeout)
    saveSettingsTimeout = setTimeout(() => {
      saveBrowserAISettings()
    }, 800)
  }
)

// Initialize directory and status on load
onMounted(async () => {
  resetForm()
  if (props.defaultOutputDir) {
    outputDir.value = props.defaultOutputDir
  }
  
  try {
    const settingsStr = await GetGlobalSettings()
    if (settingsStr) {
      const gSettings = JSON.parse(settingsStr)
      const suffix = 'image'
      if (gSettings[`browserAIModel_${suffix}`]) selectedModel.value = gSettings[`browserAIModel_${suffix}`]
      if (gSettings[`browserAIAspectRatio_${suffix}`]) aspectRatio.value = gSettings[`browserAIAspectRatio_${suffix}`]
      if (gSettings[`browserAIBatchSize_${suffix}`]) selectedBatchSize.value = gSettings[`browserAIBatchSize_${suffix}`]
      if (gSettings[`browserAIResolution_${suffix}`]) selectedResolution.value = gSettings[`browserAIResolution_${suffix}`]
      if (gSettings[`browserAIConfirmBeforeCreate_${suffix}`]) confirmBeforeCreate.value = gSettings[`browserAIConfirmBeforeCreate_${suffix}`]
    }
  } catch (err) {
    console.error("Lỗi tải cấu hình BrowserAI từ settings.json:", err)
  }
  
  isSettingsLoaded.value = true
  updateBrowserStatus()

  // Theo dõi Hàng Đợi AI khi chạy nhiều prompt song song trên trang này.
  // Lấy resultPath trực tiếp từ status.tasks (không nghe clip_ai_thumb_completed
  // riêng để tránh xung đột EventsOff với listener cùng tên ở component cha).
  EventsOn('browser-ai:queue-progress', (status: any) => {
    queueStatus.total = status.total || 0
    queueStatus.completed = status.completed || 0
    queueStatus.failed = status.failed || 0
    queueStatus.isRunning = status.isRunning || false

    if (queueMode.value && Array.isArray(status.tasks)) {
      const paths = status.tasks
        .filter((t: any) => t && t.resultPath)
        .map((t: any) => t.resultPath)
      // Chuyển đường dẫn cục bộ sang stream URL để <img> hiển thị được.
      Promise.all(paths.map((p: string) => GetStreamURL(p).catch(() => '')))
        .then((urls) => { queueResultPaths.value = urls.filter((u: string) => u !== '') })
    }

    if (queueMode.value && !status.isRunning && status.total > 0) {
      const ok = status.completed || 0
      const fail = status.failed || 0
      showToast(`Hoàn tất Hàng Đợi AI: ${ok} thành công, ${fail} lỗi.`, fail > 0 ? 'warning' : 'success')
    }
  })
})

onUnmounted(() => {
  EventsOff('browser-ai:queue-progress')
})

const pickOutputDir = async () => {
  try {
    const dir = await SelectFolder()
    if (dir) {
      outputDir.value = dir
    }
  } catch (err) {
    showToast("Lỗi chọn thư mục: " + err, "error")
  }
}

const startGeneration = async () => {
  const lines = promptLines.value
  if (lines.length === 0) return

  const baseName = fileName.value.trim() || `ai_image_${Date.now()}`
  const filePaths = inputImages.value.map(i => i.path).filter(p => p !== '')

  // Nhiều dòng prompt, HOẶC 1 dòng nhưng bật tự động tải → chạy qua Hàng Đợi AI
  // (song song, mỗi dòng 1 task, tự tải hết). 1 dòng + tắt tự tải → giữ luồng
  // xem-trước-chọn-tay cũ.
  const useQueue = lines.length > 1 || (lines.length === 1 && autoDownload.value)

  if (useQueue) {
    // Mỗi prompt 1 task. Đặt tên file _1, _2... theo thứ tự dòng để dễ tìm.
    const single = lines.length === 1
    const inputImagePath = filePaths.length > 0 ? filePaths[0] : ''
    const tasks = lines.map((line, i) => ({
      id: `img_prompt_${Date.now()}_${i}`,
      clipName: `Prompt #${i + 1}`,
      clipPath: '',
      outputDir: outputDir.value,
      fileName: single ? baseName : `${baseName}_${i + 1}`,
      prompt: line,
      inputImagePath,
      provider: provider.value,
      model: selectedModel.value,
      aspectRatio: aspectRatio.value,
      resolution: selectedResolution.value,
      state: '',
      errorMessage: '',
      resultPath: ''
    }))

    queueResultPaths.value = []
    queueStatus.total = tasks.length
    queueStatus.completed = 0
    queueStatus.failed = 0
    queueStatus.isRunning = true
    queueMode.value = true

    try {
      await EnqueueThumbnailTasks(tasks as any)
      showToast(`Đã nạp ${tasks.length} prompt vào Hàng Đợi AI. Đang tạo song song...`, 'info')
    } catch (err: any) {
      queueMode.value = false
      queueStatus.isRunning = false
      showToast('Lỗi nạp hàng đợi: ' + String(err), 'error')
    }
    return
  }

  // Luồng đơn cũ: 1 prompt, tắt tự tải → hiện preview cho người dùng chọn.
  const base64s = inputImages.value.map(i => i.base64).filter(b => b !== '')
  await generate({
    provider: provider.value,
    mediaType: 'image',
    prompt: lines[0],
    aspectRatio: aspectRatio.value,
    outputDir: outputDir.value,
    fileName: baseName,
    timeoutSecond: 300,
    showChrome: props.showChrome,
    model: selectedModel.value,
    batchSize: selectedBatchSize.value,
    confirmBeforeCreate: confirmBeforeCreate.value,
    resolution: selectedResolution.value,
    inputImagePaths: filePaths,
    inputImageBase64s: base64s
  })
}

const applyAsThemeImage = (path?: string) => {
  const targetPath = path || (result.value && result.value.filePath)
  if (targetPath) {
    emit('apply-image', targetPath)
  }
}

const openFolder = async () => {
  const dir = outputDir.value || (result.value && (result.value.filePath || (result.value.filePaths && result.value.filePaths[0])))
  if (dir) {
    try {
      await OpenOutputFolder(dir)
    } catch (err) {
      showToast("Lỗi mở thư mục: " + err, "error")
    }
  } else {
    showToast("Không tìm thấy đường dẫn thư mục", "warning")
  }
}

const resetForm = () => {
  reset()
  prompt.value = ''
  fileName.value = ''
  previewURL.value = ''
  selectedResolution.value = '1K'
  inputImages.value = []
  queueMode.value = false
  queueStatus.total = 0
  queueStatus.completed = 0
  queueStatus.failed = 0
  queueStatus.isRunning = false
  queueResultPaths.value = []
  try { ClearQueue() } catch (_) {}
}

const cancelQueue = () => {
  try { CancelQueue() } catch (_) {}
  queueStatus.isRunning = false
  showToast('Đã dừng Hàng Đợi AI.', 'warning')
}

const formatSize = (bytes: number) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}
</script>

<style scoped src="./BrowserAIPage.scoped.css"></style>
