<template>
  <div class="browser-ai-page">
    <!-- Main Workspace (Grid layout: Left for config, Right for progress / preview) -->
    <div class="workspace-grid">
      
      <!-- Left Column: Config Form -->
      <div class="config-panel" :class="{ 'panel-disabled': isGenerating }">
        <button
          type="button"
          @click="configCollapsed = !configCollapsed"
          style="display: flex; align-items: center; justify-content: space-between; width: 100%; background: none; border: none; cursor: pointer; padding: 0; margin-bottom: 8px;"
          :title="configCollapsed ? 'Mở rộng cấu hình' : 'Thu gọn cấu hình để tập trung vào prompt'"
        >
          <h3 class="panel-section-title" style="margin: 0;">Cấu hình yêu cầu</h3>
          <ChevronDown v-if="configCollapsed" :size="18" style="color: var(--wx-brand-accent);" />
          <ChevronUp v-else :size="18" style="color: var(--wx-brand-accent);" />
        </button>

        <div v-show="!configCollapsed">
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

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: var(--wx-space-2); align-items: start;">
            <div class="form-row" style="margin-bottom: 0;">
              <div style="height: 22px; display: flex; align-items: center; margin-bottom: 6px;">
                <label class="img-label" style="margin-bottom: 0;">Thư mục lưu:</label>
              </div>
              <div style="display: flex; gap: var(--wx-space-1); height: 40px;">
                <input type="text" v-model="outputDir" class="img-text-input read-only-input" readonly style="flex: 1; height: 40px;" />
                <BaseButton variant="secondary" size="icon" @click="pickOutputDir" title="Chọn thư mục lưu">
                  <FolderOpen :size="14" />
                </BaseButton>
              </div>
            </div>

            <div class="form-row" style="margin-bottom: 0;">
              <div style="height: 22px; display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px; gap: 8px;">
                <label class="img-label" style="margin-bottom: 0; white-space: nowrap; flex: none;">Tên file ảnh:</label>
                <label style="font-size: 11px; display: flex; align-items: center; gap: 5px; cursor: pointer; user-select: none; color: var(--wx-text-muted); white-space: nowrap; margin-left: auto;" title="Bật: Tự động tạo tên file theo nội dung Prompt hoặc ngày giờ. Tắt: Sử dụng tên file nhập thủ công.">
                  <input type="checkbox" v-model="autoNaming" style="width: 13px; height: 13px; accent-color: var(--wx-brand-primary);" />
                  Tự động đặt tên
                </label>
              </div>
              <input
                type="text"
                v-model="fileName"
                :disabled="autoNaming"
                :placeholder="autoNaming ? 'Tự động theo Prompt (VD: Co_gai_xinh_1...)' : 'Tên file...'"
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
              <label style="font-size: 11px; display: flex; align-items: center; gap: 3px; cursor: pointer; user-select: none; color: var(--wx-text-muted); white-space: nowrap;" title="Bật: tự động tải ảnh sinh ra.">
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
              placeholder="Mô tả ảnh bạn muốn tạo... (Gõ nhiều prompt bằng cách xuống dòng 2 lần. Nhấn Ctrl+V để dán ảnh)"
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
          <h4 class="state-title">Sẵn sàng tạo ảnh AI</h4>
          <p class="state-description">
            Điền mô tả bên trái sau đó nhấn nút <strong>"Bắt đầu tạo"</strong> bên dưới để bắt đầu luồng tự động hóa trình duyệt.
          </p>
          <BaseButton variant="primary" size="md" :disabled="effectiveTasks.length === 0" @click="startGeneration">
            <Play :size="14" /> Bắt đầu tạo{{ effectiveTasks.length > 1 ? ` (${effectiveTasks.length} prompt song song)` : '' }}
          </BaseButton>
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
            <div style="display: flex; align-items: center; gap: 8px;">
              <button
                v-if="!queueAutoDownload && !queueStatus.isRunning && queueResultPaths.length > 0"
                type="button"
                @click="toggleSelectAllResults"
                class="img-source-pill active"
                style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px;"
              >
                {{ selectedResultIndexes.size === queueResultPaths.length ? 'Bỏ tất cả' : 'Chọn tất cả' }}
                <span style="margin-left: 2px;">({{ selectedResultIndexes.size }}/{{ queueResultPaths.length }})</span>
              </button>
              <button
                v-if="!queueAutoDownload && !queueStatus.isRunning && selectedResultIndexes.size > 0"
                type="button"
                @click="deleteSelectedResults"
                class="img-source-pill"
                style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px; color: var(--wx-danger-solid);"
              >
                <Trash2 :size="12" style="margin-right: 3px;" /> Xóa {{ selectedResultIndexes.size }} ảnh
              </button>
              <button v-if="queueStatus.isRunning" @click="cancelQueue" class="img-source-pill" style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px; color: var(--wx-danger-solid);">
                <StopCircle :size="12" style="margin-right: 3px;" /> Dừng
              </button>
              <button v-else @click="newJobKeepInput" class="img-source-pill active" style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px;" title="Tạo lại với cùng prompt & ảnh (không phải nhập lại)">
                <RefreshCw :size="12" style="margin-right: 3px;" /> Tạo lại
              </button>
              <button v-if="!queueStatus.isRunning" @click="resetForm" class="img-source-pill" style="font-size: 11px; height: 28px; flex: none; width: auto; padding: 0 12px;" title="Xóa hết prompt & ảnh, làm mới hoàn toàn">
                <Trash2 :size="12" style="margin-right: 3px;" /> Xóa hết
              </button>
            </div>
          </div>

          <div style="padding: 6px 0 10px; width: 100%;">
            <div style="position: relative; height: 18px; border-radius: 9px; background: var(--wx-glass-light-bg); overflow: hidden;">
              <div :style="{ width: queuePercent + '%', height: '100%', background: 'var(--wx-brand-accent)', transition: 'width 0.3s' }"></div>
              <span style="position: absolute; top: 0; left: 0; right: 0; bottom: 0; display: flex; align-items: center; justify-content: center; font-size: 11px; font-weight: 700; color: var(--wx-text-primary, #f8fafc); text-shadow: 0 1px 2px rgba(0,0,0,0.5);">
                {{ queuePercent }}%
              </span>
            </div>
            <div style="font-size: 11.5px; color: var(--l-text-muted); margin-top: 6px;">
              Thành công: {{ queueStatus.completed }} · Lỗi: {{ queueStatus.failed }} · Tổng: {{ queueStatus.total }}
            </div>
          </div>

          <div class="custom-scroll-grid" style="flex: 1; overflow-y: auto; min-height: 0; padding: 4px 0; width: 100%;">
            <div class="img-grid" style="display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 12px; width: 100%; justify-items: center;">
              <div
                v-for="(rPath, rIdx) in queueResultPaths"
                :key="rIdx"
                class="img-card"
                :class="{ selected: !queueAutoDownload && selectedResultIndexes.has(rIdx) }"
                @click="(!queueAutoDownload && !queueStatus.isRunning) ? toggleResultImage(rIdx) : null"
                @dblclick.stop="openLightbox(queueResultPaths, rIdx)"
                :style="{ display: 'flex', flexDirection: 'column', alignItems: 'center', width: '100%', maxWidth: '160px', background: 'var(--wx-glass-light-bg)', border: '1.5px solid var(--wx-border-default)', borderRadius: 'var(--wx-radius-md)', padding: '6px', boxShadow: 'var(--wx-shadow-md)', position: 'relative', cursor: (!queueAutoDownload && !queueStatus.isRunning) ? 'pointer' : 'default' }"
              >
                <img :src="rPath" style="width: 100%; height: 160px; object-fit: cover; border-radius: var(--wx-radius-sm);" draggable="false" />
                <div v-if="!queueAutoDownload && selectedResultIndexes.has(rIdx)" class="img-card-check" style="position: absolute; top: 10px; right: 10px; background: var(--wx-brand-primary); color: var(--wx-text-inverse); border-radius: var(--wx-radius-full); width: 22px; height: 22px; display: flex; align-items: center; justify-content: center; box-shadow: var(--wx-shadow-sm); border: 1px solid var(--wx-text-inverse); z-index: 10;">
                  <Check :size="12" />
                </div>
                <!-- Zoom icon hover -->
                <div class="img-zoom-btn" @click.stop="openLightbox(queueResultPaths, rIdx)" title="Xem ảnh to">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg>
                </div>
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
          <div class="custom-scroll-grid" style="flex: 1; overflow-y: auto; min-height: 0; padding: 10px 0; width: 100%;">
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
                @click="onCardClick(pIdx, $event)"
                style="display: flex; flex-direction: column; align-items: center; width: 100%; max-width: 160px; background: var(--wx-glass-light-bg); backdrop-filter: blur(var(--wx-glass-light-blur)); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); padding: 6px; box-shadow: var(--wx-shadow-md); cursor: pointer; transition: all var(--wx-d-fast) var(--wx-ease-standard); position: relative;"
              >
                <img :src="pUrl" style="width: 100%; height: 160px; object-fit: cover; border-radius: var(--wx-radius-sm);" draggable="false" />
                
                <div
                  @click.stop.prevent="onBadgeClick(pIdx)"
                  class="img-card-check"
                  style="position: absolute; top: 10px; right: 10px; border-radius: var(--wx-radius-full); width: 24px; height: 24px; display: flex; align-items: center; justify-content: center; box-shadow: var(--wx-shadow-sm); z-index: 10; cursor: pointer; transition: all 0.2s ease;"
                  :style="selectedPreviewIndexes.has(pIdx) 
                    ? 'background: var(--wx-brand-primary); color: var(--wx-text-inverse); border: 1.5px solid #fff;' 
                    : 'background: rgba(0,0,0,0.6); color: rgba(255,255,255,0.7); border: 1.5px solid rgba(255,255,255,0.6);'"
                >
                  <Check v-if="selectedPreviewIndexes.has(pIdx)" :size="12" />
                </div>
              </div>
            </div>
          </div>

          <!-- Bottom Action Row -->
          <div class="completed-actions" style="flex-shrink: 0; margin-top: 10px; display: flex; gap: 8px; align-items: center; width: 100%;">
            <BaseButton variant="secondary" size="md" @click="cancel">
              <StopCircle :size="12" /> Hủy bỏ
            </BaseButton>
            <span style="font-size: var(--wx-fs-12); color: var(--wx-text-muted); flex: 1; text-align: left;">
              Lưu vào: <code>{{ outputDir }}</code>
            </span>
            <BaseButton
              variant="primary"
              size="md"
              :disabled="selectedPreviewIndexes.size === 0"
              @click="confirmDownload"
            >
              <Download :size="14" />
              Tải {{ selectedPreviewIndexes.size }} ảnh đã chọn
            </BaseButton>
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

          <BaseButton variant="danger" size="md" @click="cancel">
            <StopCircle :size="14" /> Hủy bỏ tác vụ
          </BaseButton>
        </div>

        <!-- Login Required panel -->
        <div v-else-if="state === 'login_required'" class="login-required-panel">
          <AlertCircle :size="40" class="login-alert-icon" />
          <h3 class="login-title">Yêu cầu Đăng nhập Google</h3>
          <p class="login-desc">
            Vui lòng hoàn tất quá trình đăng nhập tài khoản Google của bạn trên cửa sổ Chrome vừa được mở.
          </p>
          <div style="display: flex; gap: var(--wx-space-2); justify-content: center;">
            <BaseButton variant="secondary" size="md" @click="openBrowser(provider, props.showChrome)">
              <Chrome :size="14" style="margin-right: 4px;" /> Mở lại Chrome
            </BaseButton>
            <BaseButton variant="primary" size="md" @click="checkLogin(provider)">
              Xác nhận đã đăng nhập
            </BaseButton>
          </div>
        </div>

        <!-- Failed State -->
        <div v-else-if="state === 'failed'" class="failed-panel">
          <AlertCircle :size="40" class="failed-icon" />
          <h3 class="failed-title">Tạo thất bại</h3>
          <p class="failed-desc">{{ error }}</p>
          <BaseButton variant="secondary" size="md" @click="newJobKeepInput" title="Thử lại với cùng prompt & ảnh (không phải nhập lại)">
            <RefreshCw :size="12" style="margin-right: 4px;" /> Thử lại
          </BaseButton>
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
          <div class="custom-scroll-grid" style="flex: 1; overflow-y: auto; min-height: 0; padding: 10px 4px; width: 100%;">
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
                @dblclick.stop="openLightbox(previewURLs.length > 0 ? previewURLs : (previewURL ? [previewURL] : []), pIdx)"
                style="display: flex; flex-direction: column; align-items: center; width: 100%; max-width: 160px; background: var(--wx-glass-light-bg); backdrop-filter: blur(var(--wx-glass-light-blur)); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); padding: 6px; box-shadow: var(--wx-shadow-md); cursor: pointer; transition: all var(--wx-d-fast) var(--wx-ease-standard); position: relative;"
              >
                <img :src="pUrl" style="width: 100%; height: 160px; object-fit: cover; border-radius: var(--wx-radius-sm);" draggable="false" />

                <!-- Checkmark Overlay Badge -->
                <div v-if="selectedCompletedIndexes.has(pIdx)" class="img-card-check" style="position: absolute; top: 10px; right: 10px; background: var(--wx-brand-primary); color: var(--wx-text-inverse); border-radius: var(--wx-radius-full); width: 22px; height: 22px; display: flex; align-items: center; justify-content: center; box-shadow: var(--wx-shadow-sm); border: 1px solid var(--wx-text-inverse); z-index: 10;">
                  <Check :size="12" />
                </div>
                <!-- Zoom icon hover -->
                <div class="img-zoom-btn" @click.stop="openLightbox(previewURLs.length > 0 ? previewURLs : (previewURL ? [previewURL] : []), pIdx)" title="Xem ảnh to">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/><line x1="11" y1="8" x2="11" y2="14"/><line x1="8" y1="11" x2="14" y2="11"/></svg>
                </div>
              </div>
            </div>
          </div>

          <!-- Single Row Bottom Action & Info Bar -->
          <div class="completed-actions" style="flex-shrink: 0; margin-top: 8px; display: flex; gap: 8px; align-items: center; width: 100%;">
            <BaseButton variant="secondary" size="md" @click="newJobKeepInput" title="Tạo lại với cùng prompt & ảnh (không phải nhập lại)">
              <RefreshCw :size="12" /> Tạo lại
            </BaseButton>

            <span style="font-size: var(--wx-fs-12); color: var(--wx-text-muted); flex: 1; text-align: left; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
              <template v-if="(previewURLs.length > 0 ? previewURLs : [previewURL]).length <= 1">
                Đường dẫn: <code style="color: var(--wx-brand-accent);">{{ result.filePath }}</code> ({{ formatSize(result.fileSize) }})
              </template>
              <template v-else>
                Lưu vào: <code style="color: var(--wx-brand-accent);">{{ outputDir }}</code> (Đã chọn {{ selectedCompletedIndexes.size }}/{{ (previewURLs.length > 0 ? previewURLs : [previewURL]).length }} ảnh - {{ formatSize(result.fileSize) }})
              </template>
            </span>

            <BaseButton variant="secondary" size="md" @click="openFolder">
              <FolderOpen :size="12" /> Mở thư mục
            </BaseButton>
          </div>
        </div>

        <!-- Card log tiến trình (ĐỘC LẬP): hiện bất kể trạng thái trang, gộp log CẢ 2
             nguồn (cắt video + tạo ảnh AI), mỗi dòng có nhãn nguồn để phân biệt. Nhờ
             tách khỏi block queueMode nên khi cắt video chạy mà trang AI đang ở màn
             chờ, log vẫn hiện. -->
        <div v-if="queueLogs.length > 0" style="width: 100%; flex-shrink: 0; margin-top: 8px; border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-md); background: var(--wx-surface-sunken, #0e1626); overflow: hidden;">
          <div style="display: flex; align-items: center; justify-content: space-between; width: 100%; padding: 6px 10px; background: var(--wx-surface-base); border-bottom: 1px solid var(--wx-border-default);">
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
          <div v-show="!queueLogCollapsed" ref="queueLogBox" style="max-height: 300px; overflow-y: auto; padding: 4px 10px 8px; font-size: 11px; font-family: 'Consolas', monospace; line-height: 1.55;">
            <div
              v-for="line in queueLogs"
              :key="line.id"
              :style="{ color: line.level === 'error' ? '#fca5a5' : (line.level === 'success' ? '#86efac' : 'var(--l-text-muted)'), whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }"
              :title="`[${line.source === 'video-cut' ? 'Xuất video' : 'Tạo ảnh'} · W${line.worker} ${line.name}] ${line.step}`"
            >
              <span style="opacity: 0.6;">{{ line.time }}</span>
              <span
                :style="{ fontWeight: 700, marginLeft: '4px', padding: '0 5px', borderRadius: '4px', fontSize: '10px', color: line.source === 'video-cut' ? '#fcd34d' : '#93c5fd', background: line.source === 'video-cut' ? 'rgba(252,211,77,0.12)' : 'rgba(147,197,253,0.12)' }"
              >{{ line.source === 'video-cut' ? 'XUẤT VIDEO' : 'TẠO ẢNH' }}</span>
              <span style="opacity: 0.85; font-weight: 600;"> W{{ line.worker }}</span>
              <span> · {{ line.step }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- ===== LIGHTBOX OVERLAY ===== -->
  <Teleport to="body">
    <div
      v-if="lightbox.open"
      class="lightbox-overlay"
      @click.self="closeLightbox"
      @keydown.esc="closeLightbox"
      tabindex="0"
      ref="lightboxEl"
    >
      <!-- Nav prev -->
      <button v-if="lightbox.list.length > 1" class="lightbox-nav lightbox-prev" @click="lightboxStep(-1)" title="Ảnh trước (←)">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="15 18 9 12 15 6"/></svg>
      </button>

      <!-- Image -->
      <div class="lightbox-content" @click.self="closeLightbox">
        <img
          :src="lightbox.list[lightbox.index]"
          class="lightbox-img"
          draggable="false"
          :style="{ transform: `scale(${lightbox.scale})`, transition: 'transform 0.2s ease' }"
          @wheel.prevent="onLightboxWheel"
        />
        <!-- Info bar -->
        <div class="lightbox-bar">
          <span style="font-size: 12px; opacity: 0.75;">{{ lightbox.index + 1 }} / {{ lightbox.list.length }}</span>
          <span style="font-size: 11px; opacity: 0.5; margin-left: 10px;">(Lăn chuột để phóng to · Nhấn ESC để đóng)</span>
        </div>
      </div>

      <!-- Nav next -->
      <button v-if="lightbox.list.length > 1" class="lightbox-nav lightbox-next" @click="lightboxStep(1)" title="Ảnh sau (→)">
        <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="9 18 15 12 9 6"/></svg>
      </button>

      <!-- Close button -->
      <button class="lightbox-close" @click="closeLightbox" title="Đóng (ESC)">
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
      </button>
    </div>
  </Teleport>

</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useBrowserAI } from './composables/useBrowserAI'
import BaseButton from './common/BaseButton.vue'
import { Sparkles, AlertCircle, FolderOpen, Chrome, Play, StopCircle, Trash2, CheckCircle, RefreshCw, ImageIcon, Check, Download, ChevronDown, ChevronUp } from 'lucide-vue-next'
// @ts-ignore
import { SelectFolder, GetStreamURL, GetGlobalSettings, SaveGlobalSettings } from '../../wailsjs/go/main/App'
// @ts-ignore
import { OpenOutputFolder, EnqueueThumbnailTasks, CancelQueueSource, DeleteResultFiles } from '../../wailsjs/go/browserai/Service'
import { EventsOn } from '../../wailsjs/runtime/runtime'

const mediaType = 'image'

// Hàm hủy listener RIÊNG của trang này (EventsOn trả về closure hủy chính nó).
let offQueueProgress: (() => void) | null = null
let offQueueLog: (() => void) | null = null

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

const ratios = ['16:9', '4:3', '1:1', '3:4', '9:16']
const aspectRatio = ref('1:1')
const outputDir = ref('')
const fileName = ref('')
const previewURL = ref('')
const previewURLs = ref<string[]>([])

// Tự động tải: BẬT → tự tải mọi ảnh sinh ra (không cần chọn tay).
// TẮT → chế độ cũ: hiện preview cho người dùng chọn rồi mới tải (chỉ khi TỔNG = 1 task).
const autoDownload = ref(true)

// Tự động đặt tên: BẬT → tự động tạo tên file sạch theo nội dung Prompt hoặc ngày giờ.
// TẮT → sử dụng tên file do người dùng tự nhập.
const autoNaming = ref(true)

const generateAutoFileName = (promptText: string, index: number, total: number) => {
  let clean = (promptText || '')
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/đ/g, 'd').replace(/Đ/g, 'D')
    .replace(/[^a-zA-Z0-9\s]/g, '')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 4)
    .join('_')
    .substring(0, 30)
    .replace(/_$/, '')

  if (!clean) {
    const now = new Date()
    const timeStr = `${now.getFullYear()}${String(now.getMonth()+1).padStart(2,'0')}${String(now.getDate()).padStart(2,'0')}_${String(now.getHours()).padStart(2,'0')}${String(now.getMinutes()).padStart(2,'0')}${String(now.getSeconds()).padStart(2,'0')}`
    clean = `Anh_AI_${timeStr}`
  }

  return total > 1 ? `${clean}_${index + 1}` : clean
}

// Tách prompt: BẬT → mỗi đoạn cách nhau bằng 1 dòng trống (2 lần Enter) tính là 1
// prompt RIÊNG (chạy thành nhiều task song song). TẮT → toàn bộ nội dung trong ô
// tính là 1 prompt DUY NHẤT dù có dòng trống (giữ nguyên bố cục nhiều dòng).
const splitPrompts = ref(true)

// Thu gọn khối "Cấu hình yêu cầu" để tập trung vào phần nhập prompt.
const configCollapsed = ref(false)

interface InputImage {
  id: string
  path: string
  base64: string
  preview: string
}

// Mỗi card = 1 prompt + tối đa 3 ảnh riêng của prompt đó. Mặc định 1 card trống.
interface PromptCard {
  id: string
  text: string
  images: InputImage[]
}
const promptCards = ref<PromptCard[]>([
  { id: `card_${Date.now()}`, text: '', images: [] }
])

// Tự động co giãn chiều cao ô prompt theo nội dung (Min 44px, Max 180px + scroll)
const autoResizeTextarea = (el: HTMLElement | null) => {
  if (!el || !(el instanceof HTMLTextAreaElement)) return
  el.style.height = 'auto'
  const minH = 44
  const maxH = 180
  const computedH = Math.min(Math.max(el.scrollHeight, minH), maxH)
  el.style.height = `${computedH}px`
  el.style.overflowY = el.scrollHeight > maxH ? 'auto' : 'hidden'
}

const onPromptInput = (e: Event) => {
  autoResizeTextarea(e.target as HTMLElement)
}

const prompt = ref('')
interface InputImage {
  id: string
  path: string
  base64: string
  preview: string
}
const inputImages = ref<InputImage[]>([])

interface PromptTask {
  prompt: string
  images: InputImage[]
}
const effectiveTasks = computed<PromptTask[]>(() => {
  const tasks: PromptTask[] = []
  let promptsList: string[]
  if (splitPrompts.value) {
    promptsList = prompt.value
      .split(/\n\s*\n/)
      .map(b => b.split('\n').map(l => l.trim()).filter(l => l !== '').join('\n').trim())
      .filter(b => b !== '')
  } else {
    const whole = prompt.value.trim()
    promptsList = whole !== '' ? [whole] : []
  }

  if (inputImages.value.length > 0) {
    for (const img of inputImages.value) {
      let chosenPrompt = ''
      if (promptsList.length > 0) {
        const randomIndex = Math.floor(Math.random() * promptsList.length)
        chosenPrompt = promptsList[randomIndex]
      }
      tasks.push({
        prompt: chosenPrompt,
        images: [img]
      })
    }
  } else {
    if (promptsList.length > 0) {
      for (const pText of promptsList) {
        tasks.push({
          prompt: pText,
          images: []
        })
      }
    }
  }
  return tasks
})

// Trạng thái Hàng Đợi AI khi chạy nhiều prompt song song trên trang này.
const queueMode = ref(false)
const queueStatus = reactive({ total: 0, completed: 0, failed: 0, isRunning: false })
const queueResultPaths = ref<string[]>([])
// Đường dẫn file thật (local path) tương ứng từng ảnh trong queueResultPaths — để
// gọi xóa file khỏi thư mục. Song song 1-1 với queueResultPaths theo chỉ số.
const queueResultLocalPaths = ref<string[]>([])
// Ghi lại chế độ "tự động tải" tại lúc bấm tạo. TẮT → lưới kết quả hiện công cụ
// tick chọn + nút xóa để người dùng dọn ảnh thừa khỏi thư mục sau khi tải hết.
const queueAutoDownload = ref(true)
// Các ảnh kết quả đang được tick chọn (chỉ dùng khi queueAutoDownload=false).
const selectedResultIndexes = ref<Set<number>>(new Set())

// Phần trăm hoàn tất hàng đợi (đã xong / tổng), làm tròn để hiển thị trên thanh tiến độ.
const queuePercent = computed(() =>
  queueStatus.total > 0 ? Math.round((queueStatus.completed + queueStatus.failed) / queueStatus.total * 100) : 0
)

// Card log tiến trình Hàng Đợi AI: mỗi dòng 1 bước ngắn gọn (info/success/error).
interface QueueLogLine {
  id: number
  time: string
  worker: number
  name: string
  level: string
  step: string
  // Nguồn tạo log: "video-cut" (thumbnail từ cắt video) hoặc "ai-image" (tạo ảnh
  // AI riêng). Dùng để gắn nhãn phân biệt khi hiện chung 2 nguồn trên cùng card log.
  source: string
}
const queueLogs = ref<QueueLogLine[]>([])
const queueLogCollapsed = ref(false)
const queueLogBox = ref<HTMLElement | null>(null)
let queueLogSeq = 0

const clearQueueLogs = () => {
  queueLogs.value = []
}

// ===== LIGHTBOX =====
const lightboxEl = ref<HTMLElement | null>(null)
const lightbox = reactive({
  open: false,
  list: [] as string[],
  index: 0,
  scale: 1,
})

const openLightbox = (list: string[], index: number) => {
  lightbox.list = list
  lightbox.index = index
  lightbox.scale = 1
  lightbox.open = true
  nextTick(() => lightboxEl.value?.focus())
}

const closeLightbox = () => {
  lightbox.open = false
  lightbox.scale = 1
}

const lightboxStep = (dir: number) => {
  lightbox.scale = 1
  lightbox.index = (lightbox.index + dir + lightbox.list.length) % lightbox.list.length
}

const onLightboxWheel = (e: WheelEvent) => {
  const delta = e.deltaY < 0 ? 0.15 : -0.15
  lightbox.scale = Math.min(5, Math.max(0.5, lightbox.scale + delta))
}

const onLightboxKey = (e: KeyboardEvent) => {
  if (!lightbox.open) return
  if (e.key === 'Escape') closeLightbox()
  if (e.key === 'ArrowLeft' || e.key === 'ArrowUp') lightboxStep(-1)
  if (e.key === 'ArrowRight' || e.key === 'ArrowDown') lightboxStep(1)
}

onMounted(() => { window.addEventListener('keydown', onLightboxKey) })
onUnmounted(() => { window.removeEventListener('keydown', onLightboxKey) })


// Tự cuộn xuống dòng log mới nhất khi có bước mới (chỉ khi đang mở).
// Watch theo id dòng CUỐI (luôn tăng) thay vì .length — vì khi log đạt trần 200
// dòng (push rồi splice), length giữ nguyên 200 nên watch .length sẽ không kích
// hoạt và mất auto-scroll. id dòng cuối luôn thay đổi nên cuộn không bao giờ chết.
watch(() => queueLogs.value.length > 0 ? queueLogs.value[queueLogs.value.length - 1].id : -1, () => {
  if (queueLogCollapsed.value) return
  nextTick(() => {
    if (queueLogBox.value) queueLogBox.value.scrollTop = queueLogBox.value.scrollHeight
  })
})

// Khi mở lại card từ trạng thái thu nhỏ → cuộn ngay xuống dòng mới nhất.
watch(queueLogCollapsed, (collapsed) => {
  if (collapsed) return
  nextTick(() => {
    if (queueLogBox.value) queueLogBox.value.scrollTop = queueLogBox.value.scrollHeight
  })
})

watch(() => result.value, () => {
  const list = (previewURLs.value && previewURLs.value.length > 0) ? previewURLs.value : (previewURL.value ? [previewURL.value] : [])
  selectedCompletedIndexes.value = new Set(list.map((_, i) => i))
}, { immediate: true })

import { SelectImageFiles } from '../../wailsjs/go/main/App'

const handlePasteImage = async (e: ClipboardEvent) => {
  const items = e.clipboardData?.items as any
  if (!items) return
  for (let i = 0; i < items.length; i++) {
    const item = items[i]
    if (item.type.indexOf('image') !== -1) {
      const file = item.getAsFile()
      if (file) {
        const reader = new FileReader()
        reader.onload = (event) => {
          const base64 = event.target?.result as string
          inputImages.value.push({
            id: `paste_${Date.now()}_${Math.random()}`,
            path: '',
            base64,
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
      let added = 0
      for (const path of paths) {
        if (inputImages.value.some(img => img.path === path)) continue
        const url = await GetStreamURL(path)
        inputImages.value.push({
          id: `file_${Date.now()}_${Math.random()}`,
          path,
          base64: '',
          preview: url
        })
        added++
      }
      if (added > 0) showToast(`Đã thêm ${added} ảnh!`, "success")
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
    gSettings[`browserAIConfigCollapsed_${suffix}`] = configCollapsed.value
    gSettings[`browserAISplitPrompts_${suffix}`] = splitPrompts.value

    await SaveGlobalSettings(JSON.stringify(gSettings))
  } catch (err) {
    console.error("Lỗi tự động lưu cấu hình BrowserAI:", err)
  }
}

let saveSettingsTimeout: any = null
watch(
  [selectedModel, aspectRatio, selectedBatchSize, selectedResolution, confirmBeforeCreate, configCollapsed, splitPrompts],
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
      if (gSettings[`browserAIConfigCollapsed_${suffix}`] !== undefined) configCollapsed.value = gSettings[`browserAIConfigCollapsed_${suffix}`]
      if (gSettings[`browserAISplitPrompts_${suffix}`] !== undefined) splitPrompts.value = gSettings[`browserAISplitPrompts_${suffix}`]
    }
  } catch (err) {
    console.error("Lỗi tải cấu hình BrowserAI từ settings.json:", err)
  }
  
  isSettingsLoaded.value = true
  updateBrowserStatus()

  // Theo dõi Hàng Đợi AI khi chạy nhiều prompt song song trên trang này.
  // Lấy resultPath trực tiếp từ status.tasks (không nghe clip_ai_thumb_completed
  // riêng để tránh xung đột EventsOff với listener cùng tên ở component cha).
  offQueueProgress = EventsOn('browser-ai:queue-progress', (status: any) => {
    // Trang Tạo Ảnh AI CHỈ quan tâm task nguồn "ai-image" — lọc bỏ task "video-cut"
    // (thumbnail từ luồng cắt video) để 2 nguồn chạy chung hàng đợi không đếm lẫn nhau.
    const mine = Array.isArray(status.tasks)
      ? status.tasks.filter((t: any) => t && (t.source === 'ai-image' || !t.source))
      : []

    queueStatus.total = mine.length
    queueStatus.completed = mine.filter((t: any) => t.state === 'completed').length
    queueStatus.failed = mine.filter((t: any) => t.state === 'failed').length
    // Đang chạy nếu còn task ai-image chưa kết thúc (pending/processing).
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
      // Giữ đường dẫn cục bộ (để xóa file sau) song song với stream URL (để <img> hiển thị).
      queueResultLocalPaths.value = allPaths
      Promise.all(allPaths.map((p: string) => GetStreamURL(p).catch(() => '')))
        .then((urls) => { queueResultPaths.value = urls })
    }

    if (queueMode.value && !queueStatus.isRunning && queueStatus.total > 0) {
      const ok = queueStatus.completed
      const fail = queueStatus.failed
      showToast(`Hoàn tất Hàng Đợi AI: ${ok} thành công, ${fail} lỗi.`, fail > 0 ? 'warning' : 'success')
    }
  })

  // Card log: nhận log của CẢ 2 nguồn (video-cut + ai-image) để hiện chung, mỗi
  // dòng gắn nhãn nguồn để phân biệt log từ trang nào.
  offQueueLog = EventsOn('browser-ai:queue-log', (e: any) => {
    if (!e) return
    if (e.source && e.source !== 'ai-image' && e.source !== 'video-cut') return
    const d = new Date()
    queueLogs.value.push({
      id: queueLogSeq++,
      time: `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`,
      worker: e.worker || 0,
      name: e.name || '',
      level: e.level || 'info',
      step: e.step || '',
      source: e.source || 'ai-image'
    })
    // Giới hạn 200 dòng gần nhất để tránh phình bộ nhớ khi chạy nhiều task.
    if (queueLogs.value.length > 200) {
      queueLogs.value.splice(0, queueLogs.value.length - 200)
    }
  })
})

onUnmounted(() => {
  // Chỉ hủy ĐÚNG listener của trang này. EventsOff xóa TẤT CẢ listener cùng tên
  // trên toàn app, nên trước đây rời trang này là tắt luôn listener queue-progress
  // của VideoSplitter.vue (tab Cắt Video) → thumbnail AI mất tiến trình.
  offQueueProgress?.()
  offQueueLog?.()
  offQueueProgress = null
  offQueueLog = null
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
  const tasksList = effectiveTasks.value
  if (tasksList.length === 0) return

  const tasks = tasksList.map((t, i) => {
    let taskFileName = ''
    if (autoNaming.value) {
      taskFileName = generateAutoFileName(t.prompt, i, tasksList.length)
    } else {
      const userCustom = fileName.value.trim()
      if (userCustom) {
        taskFileName = tasksList.length === 1 ? userCustom : `${userCustom}_${i + 1}`
      } else {
        taskFileName = generateAutoFileName(t.prompt, i, tasksList.length)
      }
    }

    return {
      id: `img_prompt_${Date.now()}_${i}`,
      clipName: `Prompt #${i + 1}`,
      clipPath: '',
      outputDir: outputDir.value,
      fileName: taskFileName,
      prompt: t.prompt,
      inputImagePath: '',
      inputImagePaths: t.images.map(im => im.path).filter(p => p !== ''),
      inputImageBase64s: t.images.map(im => im.base64).filter(b => b !== ''),
      provider: provider.value,
      model: selectedModel.value,
      aspectRatio: aspectRatio.value,
      batchSize: selectedBatchSize.value,
      resolution: selectedResolution.value,
      state: '',
      errorMessage: '',
      resultPath: '',
      source: 'ai-image'
    }
  })

  // DỌN task ai-image CŨ trước khi nạp job mới (chỉ nguồn của trang này — KHÔNG
  // đụng task video-cut đang chạy). Backend Enqueue APPEND, nên nếu không dọn thì
  // task ai-image lỗi/hoàn tất của lần trước bị đếm lẫn vào job mới.
  try { await CancelQueueSource('ai-image') } catch (_) {}

  queueResultPaths.value = []
  queueResultLocalPaths.value = []
  selectedResultIndexes.value = new Set()
  queueLogs.value = []
  queueStatus.total = tasks.length
  queueStatus.completed = 0
  queueStatus.failed = 0
  queueStatus.isRunning = true
  queueMode.value = true
  // Nhớ chế độ tải để lưới kết quả biết có cần hiện công cụ chọn/xóa hay không.
  queueAutoDownload.value = autoDownload.value

  try {
    await EnqueueThumbnailTasks(tasks as any)
    showToast(`Đã nạp ${tasks.length} prompt vào Hàng Đợi AI. Đang tạo song song...`, 'info')
  } catch (err: any) {
    queueMode.value = false
    queueStatus.isRunning = false
    showToast('Lỗi nạp hàng đợi: ' + String(err), 'error')
  }
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

// Dọn sạch trạng thái hàng đợi + kết quả hiển thị (KHÔNG đụng prompt/ảnh). Dùng
// chung cho cả "làm mới giữ input" lẫn "xóa hết".
const clearQueueDisplay = () => {
  reset()
  previewURL.value = ''
  queueMode.value = false
  queueStatus.total = 0
  queueStatus.completed = 0
  queueStatus.failed = 0
  queueStatus.isRunning = false
  queueResultPaths.value = []
  queueResultLocalPaths.value = []
  selectedResultIndexes.value = new Set()
}

// Làm mới để tạo JOB MỚI nhưng GIỮ NGUYÊN prompt + ảnh đã nhập (tránh phải gõ lại
// khi tạo lỗi hoặc muốn tạo thêm cùng nội dung). AWAIT hủy job ai-image cũ TRƯỚC để
// không còn lệnh hủy lơ lửng giết nhầm job mới (lỗi "context canceled").
const newJobKeepInput = async () => {
  try { await CancelQueueSource('ai-image') } catch (_) {}
  clearQueueDisplay()
}

// Xóa HẾT: làm mới hoàn toàn kể cả prompt + ảnh (về form trống). AWAIT hủy job cũ.
const resetForm = async () => {
  try { await CancelQueueSource('ai-image') } catch (_) {}
  prompt.value = ''
  inputImages.value = []
  fileName.value = ''
  selectedResolution.value = '1K'
}

const onCardClick = (idx: number, e: MouseEvent) => {
  toggleResultImage(idx)
}

const onBadgeClick = (idx: number) => {
  toggleResultImage(idx)
}

// Tick chọn / bỏ chọn 1 ảnh kết quả (chỉ hiện khi tắt tự động tải).
const toggleResultImage = (idx: number) => {
  const newSet = new Set(selectedResultIndexes.value)
  if (newSet.has(idx)) {
    newSet.delete(idx)
  } else {
    newSet.add(idx)
  }
  selectedResultIndexes.value = newSet
}

// Chọn tất cả / bỏ chọn tất cả ảnh kết quả.
const toggleSelectAllResults = () => {
  if (selectedResultIndexes.value.size === queueResultPaths.value.length) {
    selectedResultIndexes.value = new Set()
  } else {
    selectedResultIndexes.value = new Set(queueResultPaths.value.map((_, i) => i))
  }
}

// Xóa các ảnh ĐANG TICK CHỌN khỏi thư mục lưu (không đảo ngược được). Sau khi xóa,
// bỏ chúng khỏi lưới kết quả và reset lựa chọn.
const deleteSelectedResults = async () => {
  const idxs = Array.from(selectedResultIndexes.value).sort((a, b) => a - b)
  if (idxs.length === 0) return
  const pathsToDelete = idxs
    .map(i => queueResultLocalPaths.value[i])
    .filter((p): p is string => !!p)
  if (pathsToDelete.length === 0) return
  try {
    await DeleteResultFiles(pathsToDelete)
    // Loại các chỉ số đã xóa khỏi cả 2 mảng song song.
    const drop = new Set(idxs)
    queueResultPaths.value = queueResultPaths.value.filter((_, i) => !drop.has(i))
    queueResultLocalPaths.value = queueResultLocalPaths.value.filter((_, i) => !drop.has(i))
    selectedResultIndexes.value = new Set()
    showToast(`Đã xóa ${pathsToDelete.length} ảnh khỏi thư mục.`, 'success')
  } catch (err: any) {
    showToast('Lỗi xóa ảnh: ' + String(err), 'error')
  }
}

const cancelQueue = () => {
  // Chỉ dừng task nguồn ai-image của trang này, KHÔNG đụng thumbnail đang chạy
  // từ luồng cắt video (nguồn video-cut) — 2 nguồn dùng chung hàng đợi.
  try { CancelQueueSource('ai-image') } catch (_) {}
  queueStatus.isRunning = false
  showToast('Đã dừng tạo ảnh AI.', 'warning')
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
