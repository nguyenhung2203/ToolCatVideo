<template>
  <div class="browser-ai-page">
    <!-- Main Workspace (Grid layout: Left for config, Right for progress / preview) -->
    <div class="workspace-grid">
      
      <!-- Left Column: Config Form -->
      <div class="config-panel" :class="{ 'panel-disabled': isGenerating || queueStatus.isRunning }">
        <button
          type="button"
          @click="configCollapsed = !configCollapsed"
          style="display: flex; align-items: center; justify-content: space-between; width: 100%; background: none; border: none; cursor: pointer; padding: 0; margin-bottom: 8px;"
          :title="configCollapsed ? 'Mở rộng cấu hình' : 'Thu gọn cấu hình để tập trung vào prompt'"
        >
          <h3 class="panel-section-title" style="margin: 0;">Cấu hình yêu cầu Video AI</h3>
          <ChevronDown v-if="configCollapsed" :size="18" style="color: var(--wx-brand-accent);" />
          <ChevronUp v-else :size="18" style="color: var(--wx-brand-accent);" />
        </button>

        <div v-show="!configCollapsed">
          <div class="form-row" v-if="provider === 'flow'">
            <label class="img-label">Model Google AI:</label>
            <div class="img-source-pills" style="flex-wrap: wrap; white-space: normal; gap: 6px;">
              <button
                v-for="m in modelsList"
                :key="m"
                :class="{ active: selectedModel === m }"
                @click="m === 'Veo 3.1 - Lite [Lower Priority]' ? selectedModel = m : null"
                :disabled="m !== 'Veo 3.1 - Lite [Lower Priority]'"
                class="img-source-pill"
                :style="m !== 'Veo 3.1 - Lite [Lower Priority]' ? 'opacity: 0.45; cursor: not-allowed; filter: grayscale(0.8);' : ''"
                :title="m !== 'Veo 3.1 - Lite [Lower Priority]' ? 'Model này tạm thời chưa dùng được, chỉ xem' : 'Model khả dụng mặc định'"
              >
                {{ m }}
              </button>
            </div>
          </div>

          <div class="form-row" v-if="provider === 'flow'">
            <label class="img-label">Thời lượng Video:</label>
            <div class="img-source-pills">
              <button v-for="d in ['4s', '6s', '8s']" :key="d" :class="{ active: selectedDuration === d }" @click="selectedDuration = d" class="img-source-pill">
                {{ d }}
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

          <div class="form-row">
            <label class="img-label">Tỷ lệ khung hình:</label>
            <div class="img-source-pills">
              <button v-for="r in ratios" :key="r" :class="{ active: aspectRatio === r }" @click="aspectRatio = r" class="img-source-pill">
                {{ r }}
              </button>
            </div>
          </div>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: var(--wx-space-2); align-items: start;">
            <div class="form-row" style="margin-bottom: 0;">
              <div style="height: 22px; display: flex; align-items: center; margin-bottom: 6px;">
                <label class="img-label" style="margin-bottom: 0;">Thư mục lưu:</label>
              </div>
              <div style="display: flex; gap: var(--wx-space-1); height: 38px;">
                <input type="text" v-model="outputDir" class="img-text-input read-only-input" readonly style="flex: 1; height: 38px;" />
                <button @click="pickOutputDir" class="img-dir-btn" style="height: 38px; width: 38px; padding: 0; flex: none;" title="Chọn thư mục lưu">
                  <FolderOpen :size="14" />
                </button>
              </div>
            </div>

            <div class="form-row" style="margin-bottom: 0;">
              <div style="height: 22px; display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; gap: 8px;">
                <label class="img-label" style="margin-bottom: 0; white-space: nowrap; flex: none;">Tên file video:</label>
                <label style="font-size: 11px; display: flex; align-items: center; gap: 5px; cursor: pointer; user-select: none; color: var(--wx-text-muted); white-space: nowrap; margin-left: auto;" title="Bật: Tự động tạo tên file theo nội dung Prompt hoặc ngày giờ. Tắt: Sử dụng tên file nhập thủ công.">
                  <input type="checkbox" v-model="autoNaming" style="width: 13px; height: 13px; accent-color: var(--wx-brand-primary);" />
                  Tự động đặt tên
                </label>
              </div>
              <input
                type="text"
                v-model="fileName"
                :disabled="autoNaming"
                :placeholder="autoNaming ? 'Tự động theo Prompt (VD: ai_video_1...)' : 'Tên file...'"
                class="img-text-input"
                :style="{ height: '38px', opacity: autoNaming ? '0.6' : '1', cursor: autoNaming ? 'not-allowed' : 'text' }"
              />
            </div>
          </div>
        </div>

        <div class="form-row prompt-row" style="flex: 1; min-height: 0; display: flex; flex-direction: column;">
          <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; flex-wrap: wrap; gap: 4px 8px; width: 100%;">
            <label class="img-label" style="margin-bottom: 0; flex: none;">Mô tả (Prompt):</label>
            <div style="display: flex; align-items: center; gap: 6px; flex-wrap: wrap; margin-left: auto;">
              <label style="font-size: 11px; display: flex; align-items: center; gap: 3px; cursor: pointer; user-select: none; color: var(--wx-text-muted); white-space: nowrap;" title="Bật: mỗi đoạn cách nhau 2 lần Enter là 1 prompt riêng. Tắt: tính 1 prompt.">
                <input type="checkbox" v-model="splitPrompts" style="width:12px; height:12px;" />
                Tách
              </label>
              <label style="font-size: 11px; display: flex; align-items: center; gap: 3px; cursor: pointer; user-select: none; color: var(--wx-text-muted); white-space: nowrap;" title="Bật: tự động tải video sinh ra.">
                <input type="checkbox" v-model="autoDownload" style="width:12px; height:12px;" />
                Tự tải
              </label>
              <button type="button" @click="chooseImageFile" class="img-source-pill" style="padding: 1px 6px; font-size: 11px; height: 22px; width: auto;" :title="'Thêm ảnh làm đầu vào (hiện có ' + inputImages.length + ' ảnh)'">
                <ImageIcon :size="11" style="margin-right: 2px;" /> Ảnh ({{ inputImages.length }})
              </button>
              <button type="button" v-if="inputImages.length > 0" @click="inputImages = []" class="img-source-pill" style="padding: 1px 6px; font-size: 11px; height: 22px; width: auto; color: var(--wx-danger-solid); border-color: rgba(239, 68, 68, 0.3); background: rgba(239, 68, 68, 0.08);" title="Xóa tất cả ảnh">
                <Trash2 :size="11" style="margin-right: 2px;" /> Xóa
              </button>
              <span class="char-counter" :class="{ limit: prompt.length > 2000 }" style="font-size: 10px; flex: none;">{{ prompt.length }}/2000</span>
            </div>
          </div>

          <div style="display: flex; flex-direction: column; flex: 1; min-height: 140px; background: var(--wx-surface-sunken); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); padding: 10px; box-sizing: border-box;">
            <textarea
              v-model="prompt"
              @input="onPromptInput"
              @paste="handlePasteImage"
              placeholder="Mô tả chuyển động/cảnh video bạn muốn tạo... (Gõ nhiều prompt bằng cách xuống dòng 2 lần. Nhấn Ctrl+V để dán ảnh)"
              maxlength="2000"
              style="width: 100%; height: 100%; flex: 1; min-height: 90px; border: none; background: transparent; outline: none; padding: 0; margin: 0; color: var(--wx-text-primary); font-family: inherit; font-size: 13px; line-height: 1.45; resize: none;"
            ></textarea>

            <!-- Minimal attached image badges -->
            <div v-if="inputImages.length > 0" style="display: flex; gap: 8px; align-items: center; padding-top: 8px; overflow-x: auto; flex-shrink: 0; scrollbar-width: thin;">
              <div v-for="img in inputImages" :key="img.id" style="position: relative; flex: none; width: 44px; height: 44px; border-radius: 6px; border: 1px solid var(--wx-border-default); overflow: hidden; background: #000;">
                <img :src="img.preview" style="width: 100%; height: 100%; object-fit: cover;" />
                <button type="button" @click="removeSelectedImage(img.id)" style="position: absolute; top: 2px; right: 2px; width: 16px; height: 16px; border-radius: 50%; background: rgba(0, 0, 0, 0.75); color: #fff; border: none; display: flex; align-items: center; justify-content: center; cursor: pointer; padding: 0; font-size: 11px; line-height: 1;" title="Xóa ảnh">
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
          <h4 class="state-title">Sẵn sàng tạo video AI</h4>
          <p class="state-description">
            Điền mô tả bên trái sau đó nhấn nút <strong>"Bắt đầu tạo"</strong> bên dưới để bắt đầu luồng tự động hóa trình duyệt.
          </p>
          <button @click="startGeneration" :disabled="effectiveTasks.length === 0" class="img-action-btn start-generate-btn">
            <Play :size="14" /> Bắt đầu tạo{{ effectiveTasks.length > 1 ? ` (${effectiveTasks.length} prompt song song)` : '' }}
          </button>
        </div>

        <!-- Queue Mode State (Chạy nhiều prompt song song qua Hàng Đợi AI) -->
        <div v-if="queueMode" class="completed-panel" style="display: flex; flex-direction: column; height: 100%; overflow: hidden; min-height: 0; width: 100%;">
          <div class="preview-header" style="display: flex; justify-content: space-between; align-items: center; width: 100%;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <Sparkles :size="20" style="color: var(--wx-brand-accent);" />
              <span class="success-title">
                {{ queueStatus.isRunning ? 'Đang tạo video song song...' : 'Hoàn tất Hàng Đợi Video AI' }}
                ({{ queueStatus.completed + queueStatus.failed }}/{{ queueStatus.total }})
              </span>
            </div>
            <div style="display: flex; align-items: center; gap: 8px;">
              <button
                v-if="!queueStatus.isRunning && queueResultPaths.length > 0"
                type="button"
                @click="toggleSelectAllResults"
                class="img-source-pill active"
                style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px;"
              >
                {{ selectedResultIndexes.size === queueResultPaths.length ? 'Bỏ tất cả' : 'Chọn tất cả' }}
                <span style="margin-left: 2px;">({{ selectedResultIndexes.size }}/{{ queueResultPaths.length }})</span>
              </button>
              <button
                v-if="!queueStatus.isRunning && selectedResultIndexes.size > 0"
                type="button"
                @click="deleteSelectedResults"
                class="img-source-pill"
                style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px; color: var(--wx-danger-solid);"
              >
                <Trash2 :size="12" style="margin-right: 3px;" /> Xóa {{ selectedResultIndexes.size }} video
              </button>
              <button v-if="queueStatus.isRunning" @click="cancelQueue" class="img-source-pill" style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px; color: var(--wx-danger-solid);">
                <StopCircle :size="12" style="margin-right: 3px;" /> Dừng
              </button>
              <button v-else @click="newJobKeepInput" class="img-dir-btn" style="height: 28px; font-size: 11px;">
                <RefreshCw :size="12" style="margin-right: 4px;" /> Tạo đợt mới
              </button>
            </div>
          </div>

          <!-- Lưới kết quả video đã sinh -->
          <div style="flex: 1; overflow-y: auto; min-height: 0; padding: 10px 0; width: 100%; display: flex; flex-direction: column; gap: 12px;">
            
            <!-- Tiến trình tổng quan nếu đang chạy -->
            <div v-if="queueStatus.isRunning" style="background: rgba(0,0,0,0.25); border: 1px solid var(--wx-border-default); border-radius: 8px; padding: 10px 14px;">
              <div style="display: flex; justify-content: space-between; font-size: 12px; font-weight: 600; margin-bottom: 6px;">
                <span>Đang xử lý {{ queueStatus.completed + queueStatus.failed }}/{{ queueStatus.total }} video...</span>
                <span>{{ Math.round((queueStatus.completed + queueStatus.failed) / (queueStatus.total || 1) * 100) }}%</span>
              </div>
              <div style="width: 100%; height: 6px; background: rgba(255,255,255,0.1); border-radius: 3px; overflow: hidden;">
                <div :style="{ width: Math.round((queueStatus.completed + queueStatus.failed) / (queueStatus.total || 1) * 100) + '%' }" style="height: 100%; background: linear-gradient(90deg, #6366f1, #06b6d4); transition: width 0.3s ease;"></div>
              </div>
            </div>

            <!-- Previews Grid khi hoàn tất / đang tải -->
            <div
              v-if="queueResultLocalPaths.length > 0"
              ref="gridRef"
              @pointerdown="onGridPointerDown"
              @pointermove="onGridPointerMove"
              @pointerup="onGridPointerUp"
              @pointercancel="onGridPointerUp"
              style="position: relative; display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; width: 100%; user-select: none; -webkit-user-select: none;"
            >
              <!-- Marquee Selection Box -->
              <div :style="boxStyle"></div>

              <div
                v-for="(item, rIdx) in queueResultLocalPaths"
                :key="rIdx"
                class="img-card"
                :class="{ selected: selectedResultIndexes.has(rIdx) }"
                @click="onCardClick(rIdx, $event)"
                @dblclick="openVideoLightbox(item.url)"
                style="display: flex; flex-direction: column; align-items: center; width: 100%; background: var(--wx-glass-light-bg); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); padding: 6px; position: relative; cursor: pointer; user-select: none; -webkit-user-select: none; transition: all 0.2s ease;"
                :style="selectedResultIndexes.has(rIdx) ? 'border-color: var(--wx-brand-primary); box-shadow: 0 0 0 2px var(--wx-brand-primary);' : ''"
              >
                <!-- Click vào bất kỳ đâu trên card (kể cả video) đều toggle chọn -->
                <!-- Dùng aspect-ratio thực tế của video để hiển thị đúng khung hình, không bị cắt xén -->
                <div
                  style="width: 100%; position: relative; background: #000; border-radius: 4px; overflow: hidden; pointer-events: none;"
                  :style="{ aspectRatio: videoAspectRatios[rIdx] || '16/9' }"
                >
                  <video
                    :src="item.url"
                    style="width: 100%; height: 100%; object-fit: contain; display: block; pointer-events: none;"
                    autoplay loop muted
                    @loadedmetadata="onVideoMetadata(rIdx, $event)"
                  ></video>
                </div>

                <!-- Checkbox badge (Luôn hiển thị góc trên phải để click chọn/bỏ chọn dễ dàng) -->
                <div
                  @click.stop.prevent="onBadgeClick(rIdx)"
                  style="position: absolute; top: 10px; right: 10px; border-radius: 50%; width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; z-index: 20; cursor: pointer; transition: all 0.2s ease;"
                  :style="selectedResultIndexes.has(rIdx) 
                    ? 'background: var(--wx-brand-primary); color: #fff; border: 1.5px solid #fff; box-shadow: 0 2px 6px rgba(0,0,0,0.5);' 
                    : 'background: rgba(0,0,0,0.6); color: rgba(255,255,255,0.7); border: 1.5px solid rgba(255,255,255,0.6);'"
                  :title="selectedResultIndexes.has(rIdx) ? 'Bỏ chọn video này' : 'Chọn video này'"
                >
                  <Check v-if="selectedResultIndexes.has(rIdx)" :size="14" />
                </div>

                <!-- Tiêu đề / Tên task -->
                <div style="width: 100%; margin-top: 6px; font-size: 11px; color: var(--wx-text-secondary); text-align: center; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; pointer-events: none;">
                  {{ item.name }}
                </div>
              </div>
            </div>

            <!-- Trạng thái trống -->
            <div v-else-if="!queueStatus.isRunning" style="text-align: center; padding: 40px 20px; color: var(--wx-text-muted);">
              Chưa có video nào trong danh sách kết quả.
            </div>

          </div>
        </div>

        <!-- Generating / Progress State (Single Mode) -->
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

        <!-- Card log tiến trình (ĐỘC LẬP): hiện bất kể trạng thái trang -->
        <div v-if="queueLogs.length > 0" style="width: 100%; flex-shrink: 0; margin-top: 8px; border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); background: var(--wx-surface-sunken, #0e1626); overflow: hidden;">
          <div style="display: flex; align-items: center; justify-content: space-between; width: 100%; padding: 6px 10px; background: rgba(0,0,0,0.15); border-bottom: 1px solid var(--wx-border-subtle, rgba(255,255,255,0.05));">
            <span style="font-size: 11.5px; font-weight: 700; color: var(--wx-brand-accent); display: inline-flex; align-items: center; gap: 6px;">
              Nhật ký tiến trình ({{ queueLogs.length }})
            </span>
            <div style="display: flex; align-items: center; gap: 6px;">
              <button
                type="button"
                @click="clearQueueLogs"
                title="Xóa nhật ký tiến trình"
                style="background: none; border: none; cursor: pointer; color: var(--wx-text-muted, #94a3b8); padding: 2px 4px; display: inline-flex; align-items: center; border-radius: 4px; transition: color 0.15s ease;"
                onmouseover="this.style.color='#fca5a5'"
                onmouseout="this.style.color='var(--wx-text-muted, #94a3b8)'"
              >
                <Trash2 :size="14" />
              </button>
              <button
                type="button"
                @click="queueLogCollapsed = !queueLogCollapsed"
                style="background: none; border: none; cursor: pointer; color: var(--wx-brand-accent); padding: 2px 4px; display: inline-flex; align-items: center;"
                :title="queueLogCollapsed ? 'Mở rộng log' : 'Thu nhỏ log'"
              >
                <ChevronDown v-if="queueLogCollapsed" :size="16" />
                <ChevronUp v-else :size="16" />
              </button>
            </div>
          </div>
          <div v-show="!queueLogCollapsed" ref="queueLogBox" style="max-height: 200px; overflow-y: auto; padding: 4px 10px 8px; font-size: 11px; font-family: 'Consolas', monospace; line-height: 1.55;">
            <div
              v-for="line in queueLogs"
              :key="line.id"
              :style="{ color: line.level === 'error' ? '#fca5a5' : (line.level === 'success' ? '#86efac' : 'var(--l-text-muted)'), whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }"
              :title="`[${line.source === 'ai-video' ? 'Tạo video' : (line.source === 'video-cut' ? 'Xuất video' : 'Tạo ảnh')} · W${line.worker} ${line.name}] ${line.step}`"
            >
              <span style="opacity: 0.6;">{{ line.time }}</span>
              <span
                :style="{ fontWeight: 700, marginLeft: '4px', padding: '0 5px', borderRadius: '4px', fontSize: '10px', color: line.source === 'ai-video' ? '#7dd3fc' : (line.source === 'video-cut' ? '#fcd34d' : '#93c5fd'), background: line.source === 'ai-video' ? 'rgba(14,165,233,0.12)' : (line.source === 'video-cut' ? 'rgba(252,211,77,0.12)' : 'rgba(147,197,253,0.12)') }"
              >{{ line.source === 'ai-video' ? 'TẠO VIDEO' : (line.source === 'video-cut' ? 'XUẤT VIDEO' : 'TẠO ẢNH') }}</span>
              <span style="opacity: 0.85; font-weight: 600;"> W{{ line.worker }}</span>
              <span> · {{ line.step }}</span>
            </div>
          </div>
        </div>

      </div>
    </div>

    <!-- Lightbox Modal xem video phóng to -->
    <div v-if="activeLightboxUrl" style="position: fixed; inset: 0; background: rgba(0,0,0,0.85); backdrop-filter: blur(8px); display: flex; align-items: center; justify-content: center; z-index: 99999;" @click.self="activeLightboxUrl = ''">
      <div style="position: relative; max-width: 90vw; max-height: 90vh; background: #111; border: 1px solid var(--wx-border-default); border-radius: 12px; padding: 12px; display: flex; flex-direction: column; align-items: center;">
        <button type="button" @click="activeLightboxUrl = ''" style="position: absolute; top: -12px; right: -12px; width: 28px; height: 28px; border-radius: 50%; background: #ef4444; color: #fff; border: 2px solid #fff; font-weight: bold; cursor: pointer; display: flex; align-items: center; justify-content: center; z-index: 10;">✕</button>
        <video :src="activeLightboxUrl" style="max-width: 85vw; max-height: 80vh; border-radius: 8px;" controls autoplay></video>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useBrowserAI } from './composables/useBrowserAI'
import { Sparkles, AlertCircle, FolderOpen, Chrome, Play, StopCircle, Trash2, CheckCircle, RefreshCw, ImageIcon, Check, Download, ChevronDown, ChevronUp } from 'lucide-vue-next'
// @ts-ignore
import { SelectFolder, GetStreamURL, GetGlobalSettings, SaveGlobalSettings, SelectImageFiles } from '../../wailsjs/go/main/App'
// @ts-ignore
import { OpenOutputFolder, EnqueueThumbnailTasks, CancelQueueSource, DeleteResultFiles } from '../../wailsjs/go/browserai/Service'
// @ts-ignore
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

const mediaType = 'video'

const props = defineProps<{
  defaultOutputDir: string
  showChrome: boolean
}>()

const emit = defineEmits<{
  (e: 'back'): void
  (e: 'apply-video', path: string): void
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
  cancel,
  reset,
  updateBrowserStatus
} = useBrowserAI(showToast)

const provider = computed<'gemini' | 'flow'>(() => 'flow')

const prompt = ref('')
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

const chooseImageFile = async () => {
  try {
    const paths = await SelectImageFiles()
    if (paths && paths.length > 0) {
      let addedCount = 0
      for (const path of paths) {
        if (inputImages.value.some(img => img.path === path)) continue
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

// Config Controls
const selectedModel = ref('Veo 3.1 - Lite [Lower Priority]')
const selectedDuration = ref('8s')
const selectedBatchSize = ref('1x')
const aspectRatio = ref('16:9')
const ratios = ['16:9', '9:16']
const outputDir = ref('')
const fileName = ref('')
const autoNaming = ref(true)
const splitPrompts = ref(true)
const autoDownload = ref(true)
const configCollapsed = ref(false)
const modelsList = ['Veo 3.1 - Lite [Lower Priority]', 'Veo 3.1 - Lite', 'Veo 3.1 - Fast', 'Veo 3.1 - Quality', 'Omni Flash']

const CONFIG_KEY = 'ai_video_config_v1'

// Auto resize textarea
const autoResizeTextarea = (el: HTMLElement | null) => {
  if (!el || !(el instanceof HTMLTextAreaElement)) return
  el.style.height = 'auto'
  const minH = 56
  const maxH = 180
  const computedH = Math.min(Math.max(el.scrollHeight, minH), maxH)
  el.style.height = `${computedH}px`
  el.style.overflowY = el.scrollHeight > maxH ? 'auto' : 'hidden'
}

const onPromptInput = (e: Event) => {
  autoResizeTextarea(e.target as HTMLElement)
}

// Parallel Queue State
const queueMode = ref(false)
const queueAutoDownload = ref(true)
const queueStatus = reactive({
  isRunning: false,
  total: 0,
  completed: 0,
  failed: 0,
  pending: 0
})

interface QueueLogItem {
  id: number
  time: string
  worker: number
  name: string
  level: string
  step: string
  source: string
}

let queueLogSeq = 0
const queueLogs = ref<QueueLogItem[]>([])
const queueLogCollapsed = ref(false)
const queueLogBox = ref<HTMLElement | null>(null)

const clearQueueLogs = () => {
  queueLogs.value = []
}

watch(() => queueLogs.value.length, () => {
  if (queueLogCollapsed.value) return
  nextTick(() => {
    if (queueLogBox.value) {
      queueLogBox.value.scrollTop = queueLogBox.value.scrollHeight
    }
  })
})

const queueResultPaths = ref<string[]>([])
const queueResultLocalPaths = ref<{ path: string; name: string; url: string }[]>([])
const selectedResultIndexes = ref<Set<number>>(new Set())
const activeLightboxUrl = ref('')

// Lưu aspect ratio thực tế của từng video sau khi load metadata
// Dạng '16/9', '9/16', '4/3'... dùng trực tiếp trong CSS aspect-ratio
const videoAspectRatios = ref<Record<number, string>>({})

const onVideoMetadata = (idx: number, e: Event) => {
  const video = e.target as HTMLVideoElement
  if (!video || !video.videoWidth || !video.videoHeight) return
  const w = video.videoWidth
  const h = video.videoHeight
  // Tính GCD để rút gọn tỷ lệ
  const gcd = (a: number, b: number): number => b === 0 ? a : gcd(b, a % b)
  const d = gcd(w, h)
  videoAspectRatios.value = { ...videoAspectRatios.value, [idx]: `${w / d}/${h / d}` }
}

const openVideoLightbox = (url: string) => {
  activeLightboxUrl.value = url
}

// Effective tasks breakdown
const effectiveTasks = computed(() => {
  const raw = prompt.value.trim()
  if (!raw) return []

  let prompts: string[] = []
  if (splitPrompts.value) {
    prompts = raw.split(/\n\s*\n/).map(p => p.trim()).filter(p => p.length > 0)
  } else {
    prompts = [raw]
  }

  return prompts.map((pText, idx) => {
    let name = ''
    if (!autoNaming.value && fileName.value.trim()) {
      name = `${fileName.value.trim()}_${idx + 1}`
    } else {
      const cleanPrompt = pText.replace(/[^a-zA-Z0-9_]/g, '_').substring(0, 30)
      name = cleanPrompt ? `ai_video_${cleanPrompt}` : `ai_video_${Date.now()}_${idx + 1}`
    }
    return {
      prompt: pText,
      images: inputImages.value,
      name: name
    }
  })
})

const smoothProgress = ref(0)
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

// Clean up timer on unmount
onUnmounted(() => {
  if (timerId) {
    clearInterval(timerId)
    timerId = null
  }
  EventsOff('browser-ai:queue-progress')
  EventsOff('browser-ai:queue-log')
})

// Queue event listeners
onMounted(() => {
  EventsOn('browser-ai:queue-progress', (st: any) => {
    if (!st) return
    const mine = Array.isArray(st.tasks)
      ? st.tasks.filter((t: any) => t && t.source === 'ai-video')
      : []

    queueStatus.total = mine.length
    queueStatus.completed = mine.filter((t: any) => t.state === 'completed').length
    queueStatus.failed = mine.filter((t: any) => t.state === 'failed').length
    queueStatus.pending = mine.filter((t: any) => t.state === 'pending').length
    queueStatus.isRunning = mine.some((t: any) => t.state === 'pending' || t.state === 'processing')

    if (queueMode.value) {
      const allPaths: string[] = []
      for (const t of mine) {
        if (Array.isArray(t.resultPaths) && t.resultPaths.length > 0) {
          allPaths.push(...t.resultPaths)
        } else if (t.resultPath) {
          allPaths.push(t.resultPath)
        }
      }
      queueResultPaths.value = allPaths
    }
  })

  EventsOn('browser-ai:queue-log', (e: any) => {
    if (!e) return
    if (e.source && e.source !== 'ai-video') return
    const d = new Date()
    queueLogs.value.push({
      id: queueLogSeq++,
      time: `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`,
      worker: e.worker || 0,
      name: e.name || '',
      level: e.level || 'info',
      step: e.step || '',
      source: e.source || 'ai-video'
    })
    if (queueLogs.value.length > 200) {
      queueLogs.value.splice(0, queueLogs.value.length - 200)
    }
  })
})

// Sync result URLs for parallel queue mode
watch(queueResultPaths, async (paths) => {
  const newItems: { path: string; name: string; url: string }[] = []
  if (paths && paths.length > 0) {
    for (const p of paths) {
      try {
        const streamUrl = await GetStreamURL(p)
        const parts = p.split(/[/\\]/)
        newItems.push({
          path: p,
          name: parts[parts.length - 1] || 'video.mp4',
          url: streamUrl
        })
      } catch (_) {
        newItems.push({
          path: p,
          name: 'video.mp4',
          url: ''
        })
      }
    }
  }
  queueResultLocalPaths.value = newItems
}, { immediate: true })

const gridRef = ref<HTMLElement | null>(null)
const isSelectingBox = ref(false)
const boxStart = reactive({ x: 0, y: 0 })
const boxCurrent = reactive({ x: 0, y: 0 })

const boxStyle = computed(() => {
  if (!isSelectingBox.value) return { display: 'none' }
  const left = Math.min(boxStart.x, boxCurrent.x)
  const top = Math.min(boxStart.y, boxCurrent.y)
  const width = Math.abs(boxCurrent.x - boxStart.x)
  const height = Math.abs(boxCurrent.y - boxStart.y)
  return {
    position: 'absolute' as const,
    left: left + 'px',
    top: top + 'px',
    width: width + 'px',
    height: height + 'px',
    border: '1.5px dashed #38bdf8',
    background: 'rgba(56, 189, 248, 0.2)',
    borderRadius: '4px',
    pointerEvents: 'none' as const,
    zIndex: 99
  }
})

let initialSelection = new Set<number>()
let isDragMoved = false

const onGridPointerDown = (e: PointerEvent) => {
  if (e.button !== 0 || !gridRef.value) return
  const target = e.target as HTMLElement
  if (target.closest('button, input, a')) return

  const rect = gridRef.value.getBoundingClientRect()
  boxStart.x = e.clientX - rect.left + gridRef.value.scrollLeft
  boxStart.y = e.clientY - rect.top + gridRef.value.scrollTop
  boxCurrent.x = boxStart.x
  boxCurrent.y = boxStart.y
  isSelectingBox.value = true
  isDragMoved = false

  initialSelection = new Set(selectedResultIndexes.value)

  try { gridRef.value.setPointerCapture(e.pointerId) } catch(_) {}
}

const onGridPointerMove = (e: PointerEvent) => {
  if (!isSelectingBox.value || !gridRef.value) return
  const rect = gridRef.value.getBoundingClientRect()
  boxCurrent.x = e.clientX - rect.left + gridRef.value.scrollLeft
  boxCurrent.y = e.clientY - rect.top + gridRef.value.scrollTop

  const minX = Math.min(boxStart.x, boxCurrent.x)
  const maxX = Math.max(boxStart.x, boxCurrent.x)
  const minY = Math.min(boxStart.y, boxCurrent.y)
  const maxY = Math.max(boxStart.y, boxCurrent.y)

  if (maxX - minX > 5 || maxY - minY > 5) {
    isDragMoved = true
  }

  const newSet = new Set(initialSelection)
  const cards = gridRef.value.querySelectorAll('.img-card')
  cards.forEach((card, idx) => {
    const el = card as HTMLElement
    const cardLeft = el.offsetLeft
    const cardTop = el.offsetTop
    const cardRight = cardLeft + el.offsetWidth
    const cardBottom = cardTop + el.offsetHeight

    const intersects = !(cardRight < minX || cardLeft > maxX || cardBottom < minY || cardTop > maxY)
    if (intersects) {
      if (initialSelection.has(idx)) {
        newSet.delete(idx)
      } else {
        newSet.add(idx)
      }
    }
  })
  selectedResultIndexes.value = newSet
}

const onGridPointerUp = (e: PointerEvent) => {
  if (!isSelectingBox.value) return
  isSelectingBox.value = false
  if (gridRef.value) {
    try { gridRef.value.releasePointerCapture(e.pointerId) } catch(_) {}
  }
}

const onCardClick = (idx: number, e: MouseEvent) => {
  if (isDragMoved) {
    isDragMoved = false
    return
  }
  toggleResultSelection(idx)
}

const onBadgeClick = (idx: number) => {
  toggleResultSelection(idx)
}

const toggleResultSelection = (idx: number) => {
  const newSet = new Set(selectedResultIndexes.value)
  if (newSet.has(idx)) {
    newSet.delete(idx)
  } else {
    newSet.add(idx)
  }
  selectedResultIndexes.value = newSet
}

const toggleSelectAllResults = () => {
  if (selectedResultIndexes.value.size === queueResultPaths.value.length) {
    selectedResultIndexes.value.clear()
  } else {
    selectedResultIndexes.value = new Set(queueResultPaths.value.map((_, i) => i))
  }
}

const deleteSelectedResults = async () => {
  const pathsToDelete = Array.from(selectedResultIndexes.value)
    .map(idx => queueResultPaths.value[idx])
    .filter(Boolean)

  if (pathsToDelete.length === 0) return
  try {
    await DeleteResultFiles(pathsToDelete)
    queueResultPaths.value = queueResultPaths.value.filter(p => !pathsToDelete.includes(p))
    selectedResultIndexes.value.clear()
    showToast(`Đã xóa ${pathsToDelete.length} video.`, 'info')
  } catch (err) {
    showToast(`Lỗi xóa file: ${err}`, 'error')
  }
}

// Queue cancellation
const cancelQueue = async () => {
  try {
    await CancelQueueSource('ai-video')
    queueStatus.isRunning = false
    showToast('Đã gửi yêu cầu dừng hàng đợi Video AI.', 'info')
  } catch (err) {
    showToast(`Lỗi dừng hàng đợi: ${err}`, 'error')
  }
}

const newJobKeepInput = async () => {
  await cancelQueue()
  queueMode.value = false
  queueLogs.value = []
  queueResultPaths.value = []
  queueResultLocalPaths.value = []
  selectedResultIndexes.value.clear()
}

// Start Generation flow (Queue mode)
const startGeneration = async () => {
  const tasksToRun = effectiveTasks.value
  if (tasksToRun.length === 0) {
    showToast('Vui lòng nhập nội dung prompt video!', 'warning')
    return
  }

  queueMode.value = true
  queueAutoDownload.value = autoDownload.value
  queueLogs.value = []
  queueResultPaths.value = []
  queueResultLocalPaths.value = []
  selectedResultIndexes.value.clear()

  const queueTasks = tasksToRun.map((t, idx) => ({
    id: `video_task_${Date.now()}_${idx}`,
    clipName: `Video #${idx + 1}`,
    clipPath: '',
    outputDir: outputDir.value,
    fileName: t.name,
    prompt: t.prompt,
    inputImagePath: '',
    inputImagePaths: t.images.map(i => i.path).filter(Boolean),
    inputImageBase64s: t.images.map(i => i.base64).filter(Boolean),
    provider: 'flow',
    mediaType: 'video',
    source: 'ai-video',
    model: selectedModel.value,
    aspectRatio: aspectRatio.value,
    batchSize: selectedBatchSize.value,
    duration: selectedDuration.value,
    resolution: '1K',
    state: 'pending',
    errorMessage: '',
    resultPath: '',
    resultPaths: []
  }))

  try {
    await CancelQueueSource('ai-video')
    await EnqueueThumbnailTasks(queueTasks as any)
    showToast(`Đã thêm ${queueTasks.length} prompt video vào Hàng Đợi AI song song!`, 'success')
  } catch (err) {
    showToast(`Lỗi thêm vào hàng đợi: ${err}`, 'error')
    queueMode.value = false
  }
}

// Settings persistence
const isSettingsLoaded = ref(false)

const saveBrowserAISettings = async () => {
  if (!isSettingsLoaded.value) return
  try {
    const settingsStr = await GetGlobalSettings()
    let gSettings: any = {}
    if (settingsStr) gSettings = JSON.parse(settingsStr)
    
    const suffix = 'video'
    gSettings[`browserAIModel_${suffix}`] = selectedModel.value
    gSettings[`browserAIAspectRatio_${suffix}`] = aspectRatio.value
    gSettings[`browserAIDuration_${suffix}`] = selectedDuration.value
    gSettings[`browserAIBatchSize_${suffix}`] = selectedBatchSize.value
    gSettings[`browserAIAutoNaming_${suffix}`] = autoNaming.value
    gSettings[`browserAISplitPrompts_${suffix}`] = splitPrompts.value
    gSettings[`browserAIAutoDownload_${suffix}`] = autoDownload.value
    gSettings[`browserAIConfigCollapsed_${suffix}`] = configCollapsed.value
    
    await SaveGlobalSettings(JSON.stringify(gSettings))
  } catch (err) {
    console.error("Lỗi tự động lưu cấu hình BrowserAI:", err)
  }
}

let saveSettingsTimeout: any = null
watch(
  [selectedModel, aspectRatio, selectedDuration, selectedBatchSize, autoNaming, splitPrompts, autoDownload, configCollapsed],
  () => {
    if (!isSettingsLoaded.value) return
    if (saveSettingsTimeout) clearTimeout(saveSettingsTimeout)
    saveSettingsTimeout = setTimeout(() => {
      saveBrowserAISettings()
    }, 500)
  }
)

onMounted(async () => {
  resetForm()
  if (props.defaultOutputDir) {
    outputDir.value = props.defaultOutputDir
  }
  
  try {
    const settingsStr = await GetGlobalSettings()
    if (settingsStr) {
      const gSettings = JSON.parse(settingsStr)
      const suffix = 'video'
      if (gSettings[`browserAIModel_${suffix}`]) selectedModel.value = gSettings[`browserAIModel_${suffix}`]
      if (gSettings[`browserAIAspectRatio_${suffix}`]) aspectRatio.value = gSettings[`browserAIAspectRatio_${suffix}`]
      if (gSettings[`browserAIDuration_${suffix}`]) selectedDuration.value = gSettings[`browserAIDuration_${suffix}`]
      if (gSettings[`browserAIBatchSize_${suffix}`]) selectedBatchSize.value = gSettings[`browserAIBatchSize_${suffix}`]
      if (gSettings[`browserAIAutoNaming_${suffix}`] !== undefined) autoNaming.value = gSettings[`browserAIAutoNaming_${suffix}`]
      if (gSettings[`browserAISplitPrompts_${suffix}`] !== undefined) splitPrompts.value = gSettings[`browserAISplitPrompts_${suffix}`]
      if (gSettings[`browserAIAutoDownload_${suffix}`] !== undefined) autoDownload.value = gSettings[`browserAIAutoDownload_${suffix}`]
      if (gSettings[`browserAIConfigCollapsed_${suffix}`] !== undefined) configCollapsed.value = gSettings[`browserAIConfigCollapsed_${suffix}`]
    }
  } catch (err) {
    console.error("Lỗi tải cấu hình BrowserAI từ settings.json:", err)
  }
  
  isSettingsLoaded.value = true
  updateBrowserStatus()
})

const pickOutputDir = async () => {
  try {
    const dir = await SelectFolder()
    if (dir) outputDir.value = dir
  } catch (err) {
    showToast("Lỗi chọn thư mục: " + err, "error")
  }
}

const openFolder = async () => {
  const dir = outputDir.value
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
  inputImages.value = []
  fileName.value = ''
  queueMode.value = false
  queueLogs.value = []
  queueResultPaths.value = []
  queueResultLocalPaths.value = []
  selectedResultIndexes.value.clear()
}
</script>

<style scoped src="./BrowserAIPage.scoped.css"></style>
