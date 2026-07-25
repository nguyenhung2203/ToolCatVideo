<template>
  <div class="browser-ai-page">

    <!-- Workspace Grid 2 Cột chuẩn - thu gọn cột trái để tập trung bảng phải rộng như Excel -->
    <div class="workspace-grid" :style="{ gridTemplateColumns: isLeftPanelCollapsed ? '1fr' : '3.4fr 6.6fr' }">

      <!-- Cột Trái: Cấu Hình Yêu Cầu & Tham Số -->
      <div class="config-panel" style="gap: 14px;" v-if="!isLeftPanelCollapsed">
        
        <!-- BƯỚC 1: KẾT NỐI SHEET & CHỌN TAB -->
        <div class="step-card">
          <h4 class="panel-section-title" style="display: flex; align-items: center; justify-content: space-between;">
            <span style="display: flex; align-items: center; gap: 6px;">
              <span class="step-num">1</span>
              <span>KẾT NỐI SHEET & CHỌN TAB</span>
            </span>
            <span v-if="tabGid" style="font-size: 10.5px; color: #38bdf8; font-weight: normal; text-transform: none;">
              GID: {{ tabGid }}
            </span>
          </h4>
          
          <div class="form-row" style="margin-bottom: 10px;">
            <label style="font-size: 11.5px; font-weight: 600; color: var(--wx-text-secondary);">Link Google Sheet:</label>
            <div style="display: flex; gap: 6px;">
              <input
                type="text"
                v-model="sheetUrl"
                @input="onUrlInput"
                placeholder="Dán link Google Sheet của anh vào đây..."
                class="img-text-input"
                style="flex: 1; height: 40px;"
              />
              <BaseButton variant="secondary" size="md" @click="fetchSheetStructure(true)" title="Bấm để tự đọc tất cả các Tab & Cột">
                <RefreshCw :size="13" /> Kiểm Tra Link
              </BaseButton>
            </div>
          </div>

          <div class="form-row" style="margin-bottom: 10px;">
            <label style="font-size: 11.5px; font-weight: 600; color: var(--wx-text-secondary);">
              Google Apps Script Web App URL:
            </label>
            <div style="display: flex; gap: 6px;">
              <input
                type="text"
                v-model="webAppUrl"
                @input="saveConfig"
                @change="fetchSheetStructure(false)"
                placeholder="https://script.google.com/macros/s/AKfycb.../exec"
                class="img-text-input"
                style="flex: 1; height: 40px;"
              />
              <BaseButton variant="secondary" size="md" style="white-space: nowrap;" @click="openScriptModal" title="Xem hướng dẫn cài đặt Apps Script 1-Click">
                <Code :size="13" /> Lấy mã Apps Script
              </BaseButton>
            </div>
          </div>

          <!-- Danh sách Nút Chọn Tab -->
          <div class="form-row">
            <label style="font-size: 11.5px; font-weight: 600; color: var(--wx-text-secondary); display: flex; justify-content: space-between;">
              <span>Chọn Tab mục tiêu trên Sheet:</span>
              <span style="font-weight: normal; color: var(--wx-brand-accent);">({{ availableTabs.length }} Tab tìm thấy)</span>
            </label>
            <div class="tab-pills-grid">
              <button
                v-for="tab in availableTabs"
                :key="tab.gid + tab.name"
                :class="{ active: activeTabName === tab.name }"
                @click="selectTab(tab)"
                class="tab-select-pill"
              >
                <Folder :size="13" />
                <span>{{ tab.name }}</span>
              </button>
            </div>
          </div>
        </div>

        <!-- BƯỚC 2: CHỌN CỘT AI SINH & SỐ MẪU -->
        <div class="step-card">
          <h4 class="panel-section-title" style="display: flex; align-items: center; justify-content: space-between; gap: 8px;">
            <span style="display: flex; align-items: center; gap: 6px;">
              <span class="step-num">2</span>
              <span>CHỌN CỘT AI SINH NỘI DUNG</span>
            </span>
            <div style="display: flex; align-items: center; gap: 6px;">
              <span style="font-size: 11px; color: #34d399; text-transform: none; font-weight: normal;">
                ({{ selectedHeaderNames.size }}/{{ activeHeaders.length }})
              </span>
              <button
                @click="toggleSelectAllHeaders"
                class="header-text-act-btn"
                :style="{ color: isAllHeadersSelected ? '#f87171' : '#38bdf8' }"
                :title="isAllHeadersSelected ? 'Bỏ chọn tất cả các cột' : 'Chọn tất cả các cột'"
              >
                {{ isAllHeadersSelected ? 'Bỏ chọn tất cả' : 'Chọn tất cả' }}
              </button>
            </div>
          </h4>

          <div v-if="activeHeaders.length === 0" style="font-size: 11.5px; color: var(--wx-text-muted); font-style: italic; padding: 6px 0;">
            Chưa tìm thấy cột. Bấm "Kiểm Tra Link" ở trên để đọc cột từ Sheet.
          </div>

          <!-- Render danh sách Cột dạng Chip Cards dễ bấm -->
          <div class="column-chips-grid">
            <div
              v-for="headerName in activeHeaders"
              :key="headerName"
              :class="{ selected: selectedHeaderNames.has(headerName) }"
              @click="toggleHeaderSelection(headerName)"
              class="column-chip-card"
            >
              <input
                type="checkbox"
                :checked="selectedHeaderNames.has(headerName)"
                @click.stop="toggleHeaderSelection(headerName)"
                class="chip-checkbox"
              />
              <span class="chip-title">{{ headerName }}</span>
            </div>
          </div>

          <!-- Prompt riêng cho từng cột được tick -->
          <div v-if="getColumnsToGenerate().length > 0" class="col-prompts-box">
            <div class="col-prompts-title">
              <Sliders :size="13" />
              <span>YÊU CẦU NỘI DUNG THEO CỘT</span>
              <span style="font-weight: normal; color: var(--wx-text-muted); font-size: 10.5px;">(Để trống = tự do)</span>
            </div>
            <div v-for="col in getColumnsToGenerate()" :key="col" class="col-prompt-item">
              <label class="col-prompt-label">
                <Sparkles :size="12" style="color: #38bdf8;" />
                <span>Yêu cầu cho cột: <strong style="color: #38bdf8;">{{ col }}</strong></span>
              </label>
              <textarea
                :value="getColumnPrompt(col)"
                @input="setColumnPrompt(col, ($event.target as HTMLTextAreaElement).value)"
                :placeholder="`VD: Viết nội dung cho cột ${col} ngắn gọn 1-2 câu, thu hút tương tác...`"
                class="col-prompt-textarea"
                rows="2"
              ></textarea>
            </div>
          </div>

          <!-- Chọn Số Mẫu Biến Thể (mỗi chủ đề sinh bao nhiêu dòng nội dung) -->
          <div style="margin-top: 12px; padding-top: 10px; border-top: 1px dashed var(--wx-border-default);">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 8px;">
              <span style="font-size: 11.5px; font-weight: 600; color: var(--wx-text-secondary);">Số mẫu / chủ đề:</span>
              <div style="display: flex; align-items: center; gap: 6px;">
                <input
                  type="number"
                  min="1"
                  max="500"
                  :value="variantCount"
                  @input="onVariantCountInput"
                  class="img-text-input"
                  style="width: 70px; height: 26px; text-align: center; font-size: 12px; font-weight: 700;"
                />
                <span style="font-size: 11px; color: var(--wx-text-muted);">mẫu</span>
              </div>
            </div>
            <div class="img-source-pills" style="gap: 4px; flex-wrap: wrap;">
              <button v-for="v in [1, 3, 5, 10, 20, 30, 40, 50]" :key="v" :class="{ active: variantCount === v }" @click="variantCount = v" class="img-source-pill" style="height: 26px; padding: 0 10px; font-size: 11px;">
                {{ v }}
              </button>
            </div>
          </div>

          <!-- Tùy chọn Icon / Emoji -->
          <div style="margin-top: 8px; padding-top: 8px; border-top: 1px dashed var(--wx-border-default); display: flex; align-items: center; justify-content: space-between;">
            <label style="display: flex; align-items: center; gap: 6px; cursor: pointer; font-size: 11.5px; font-weight: 600; color: #38bdf8; user-select: none;">
              <input type="checkbox" v-model="enforceLeadingEmoji" style="width: 14px; height: 14px; accent-color: #38bdf8; cursor: pointer;" />
              <span>Tự động chèn Emoji / Icon phù hợp ngữ cảnh (😷, 🙈, 🔥...)</span>
            </label>
          </div>
        </div>



        <!-- Nút Hành Động Bước 1 -->
        <div class="config-footer-row" style="padding-top: 6px; display: flex; gap: 8px;">
          <BaseButton
            v-if="!isGeneratingAI"
            variant="primary"
            size="lg"
            block
            :disabled="selectedHeaderNames.size === 0"
            @click="generateAllTopicGroups"
          >
            <Sparkles :size="16" />
            <span>1. Sinh Content AI (Theo Cột Đã Chọn)</span>
          </BaseButton>
          <BaseButton
            v-else
            variant="danger"
            size="lg"
            block
            @click="cancelGenerationFlag = true"
          >
            <Loader2 :size="16" class="spin-icon" />
            <span>Dừng Sinh Content (Hủy)</span>
          </BaseButton>
        </div>

      </div>

      <!-- Cột Phải: Workspace Bảng Xem Trước, Chọn Lọc & Đẩy Sheet -->
      <div class="preview-panel" style="justify-content: flex-start; align-items: stretch; padding: 4px; width: 100%;">

        <!-- Thanh Công Cụ Điều Khiển Top Toolbar Tinh Gọn -->
        <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px; margin-bottom: 6px; flex-wrap: wrap;">
          <div style="display: flex; align-items: center; gap: 8px;">
            <BaseButton
              variant="secondary"
              size="sm"
              @click="isLeftPanelCollapsed = !isLeftPanelCollapsed"
              :title="isLeftPanelCollapsed ? 'Hiện lại bảng cấu hình bên trái' : 'Ẩn bảng cấu hình, mở rộng bảng kết quả'"
            >
              <PanelLeftOpen v-if="isLeftPanelCollapsed" :size="14" />
              <PanelLeftClose v-else :size="14" />
              <span>{{ isLeftPanelCollapsed ? 'Hiện cấu hình' : 'Mở rộng bảng Excel' }}</span>
            </BaseButton>
            
            <span style="font-size: 12.5px; font-weight: 700; color: #38bdf8; display: flex; align-items: center; gap: 6px;">
              <FileSpreadsheet :size="16" />
              <span>Bảng xem trước & đẩy Sheet</span>
            </span>
          </div>
        </div>

        <!-- Màn hình chờ (Placeholder) khi chưa có kết quả -->
        <div v-if="topicGroups.length === 0" class="state-placeholder" style="margin: auto;">
          <div class="placeholder-decor">
            <FileSpreadsheet :size="60" style="color: #38bdf8;" />
          </div>
          <template v-if="activeHeaders.length > 0">
            <h3 class="state-title">Tab đang chọn: {{ activeTabName || 'Chưa chọn' }}</h3>
            <!-- <p class="state-description">
              Đã đọc được {{ activeHeaders.length }} cột trên Tab <strong>{{ activeTabName }}</strong>. Tích chọn cột cần sinh, điền danh sách chủ đề bên trái rồi nhấn "Sinh Content AI".
            </p> -->
          </template>
          <template v-else>
            <h3 class="state-title">Chưa kết nối Google Sheet</h3>
            <p class="state-description">
              Dán Web App URL (từ Apps Script) vào ô bên trái và bấm "Kiểm Tra Link" để đọc Tab & Cột thật từ Sheet. Chưa có URL thì bấm "Lấy mã Apps Script (1-Click)" để cài đặt.
            </p>
          </template>
        </div>

        <!-- Màn hình Kết Quả Gom Nhóm & Chọn Lọc -->
        <div v-else style="display: flex; flex-direction: column; gap: 10px; height: 100%;">
          
          <!-- Danh sách các Nhóm Chủ Đề (Gộp gọn gàng thành 1 Thanh Header Duy Nhất) -->
          <div class="custom-scroll-grid" style="flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 14px; padding-right: 4px;">
            <div v-for="(group, gIdx) in topicGroups" :key="group.topicId" class="topic-group-box">
              
              <!-- Single Merged Header Bar -->
              <div class="topic-group-header" style="padding: 8px 12px;">
                <div style="display: flex; align-items: center; gap: 10px;">
                  <input
                    type="checkbox"
                    :checked="isGroupAllSelected(group)"
                    @change="toggleGroupSelection(group)"
                    style="width: 16px; height: 16px; cursor: pointer; accent-color: var(--wx-brand-primary);"
                  />
                  <span style="font-size: 13px; font-weight: 700; color: var(--wx-text-primary); display: flex; align-items: center; gap: 6px;">
                    <FileSpreadsheet :size="15" style="color: #38bdf8;" />
                    <span v-if="topicGroups.length > 1">Nhóm #{{ gIdx + 1 }}</span>
                    <span v-else>Danh sách mẫu AI sinh</span>
                    <span style="font-size: 11.5px; color: var(--wx-text-muted); font-weight: normal;">
                      ({{ group.items.filter(i => i.selected).length }}/{{ group.items.length }} mẫu được chọn)
                    </span>
                  </span>
                </div>

                <div style="display: flex; align-items: center; gap: 8px;">
                  <BaseButton
                    variant="secondary"
                    size="sm"
                    :loading="group.isGeneratingMore"
                    :disabled="group.isGeneratingMore"
                    @click="addExtraSampleToGroup(group)"
                  >
                    <Plus v-if="!group.isGeneratingMore" :size="12" />
                    <span>{{ group.isGeneratingMore ? 'Đang tạo...' : 'Sinh Thêm 1 Mẫu' }}</span>
                  </BaseButton>

                  <!-- Nút Tạo Lại Các Ô Đã Chọn -->
                  <BaseButton
                    v-if="hasSelectedCellsInGroup(group)"
                    variant="secondary"
                    size="sm"
                    :loading="group.isGeneratingMore"
                    :disabled="group.isGeneratingMore"
                    @click="regenerateSelectedCellsInGroup(group)"
                    title="Tạo lại tất cả các ô nội dung (card) đang tích chọn trong nhóm này"
                  >
                    <RefreshCw v-if="!group.isGeneratingMore" :size="12" />
                    <span>Tạo Lại Đã Chọn</span>
                  </BaseButton>

                  <BaseButton variant="danger" size="sm" @click="clearResults" title="Xóa tất cả mẫu">
                    <Trash2 :size="13" /> Xóa tất cả
                  </BaseButton>

                  <!-- Nút Đẩy Sheet -->
                  <BaseButton
                    variant="success"
                    size="sm"
                    :loading="isPushing"
                    :disabled="totalSelectedCount === 0 || isPushing"
                    @click="pushSelectedRowsToSheet"
                  >
                    <Upload v-if="!isPushing" :size="13" />
                    <span>{{ isPushing ? 'Đang Đẩy Sheet...' : `Đẩy Lên Sheet` }}</span>
                  </BaseButton>
                </div>
              </div>

              <!-- Bảng Chi Tiết Mẫu CỘT ĐỘNG (Có khung cuộn nội bộ max-height: 420px) -->
              <div v-if="!group.isCollapsed" class="topic-group-table-wrap">
                <table class="clean-data-table">
                  <thead>
                    <tr>
                      <th style="width: 42px; min-width: 42px; text-align: center;">Chọn</th>
                      <th style="width: 55px; min-width: 55px;">Mẫu</th>
                      
                      <!-- Render tiêu đề Cột Động CHỈ cho các Cột được tích chọn -->
                      <th v-for="headerName in displayHeaders" :key="headerName" style="min-width: 260px;">
                        {{ headerName }}
                      </th>

                      <th style="width: 90px; min-width: 90px;">Trạng Thái</th>
                      <th style="width: 40px; min-width: 40px; text-align: center;"></th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(item, idx) in visibleGroupItems(group)" :key="item.id" :class="{ row_active: item.selected }">
                      <td style="text-align: center; padding: 8px 4px; width: 42px; min-width: 42px;">
                        <input
                          type="checkbox"
                          :checked="isRowAllCellsSelected(item)"
                          @change="toggleRowCellsSelection(item)"
                          style="width: 15px; height: 15px; cursor: pointer; accent-color: var(--wx-brand-primary);"
                        />
                      </td>
                      <td style="font-weight: 700; color: #c084fc; padding: 8px 4px; width: 55px; min-width: 55px;">#{{ idx + 1 }}</td>

                      <!-- Render ô sửa chữ CHỈ theo các Cột được tích chọn -->
                      <td v-for="headerName in displayHeaders" :key="headerName" style="padding: 6px; position: relative; min-width: 260px;">
                        <div class="cell-textarea-wrap" style="position: relative;">
                          <!-- Checkbox chọn ô/card ở góc trên bên phải -->
                          <input
                            type="checkbox"
                            :checked="isCellSelected(item.id, headerName)"
                            @change="toggleCellSelection(item, headerName)"
                            class="cell-card-checkbox"
                          />

                          <textarea
                            v-model="item.columnData[headerName]"
                            class="clean-cell-textarea"
                            :class="{ selected: isCellSelected(item.id, headerName) }"
                            rows="2"
                            placeholder="Trống..."
                          ></textarea>

                          <!-- Hover Tooltip Bản Dịch Tiếng Việt (Tự động nảy lên TRÊN nếu ở mép dưới, nảy xuống DƯỚI nếu ở mép trên) -->
                          <div
                            v-if="getItemTranslation(item, headerName)"
                            :class="['vi-translation-tooltip', isLastRow(group, idx) ? 'position-above' : 'position-below']"
                          >
                            <div class="vi-tooltip-header">
                              <span>🇻🇳 Dịch Tiếng Việt</span>
                            </div>
                            <div class="vi-tooltip-body">
                              {{ getItemTranslation(item, headerName) }}
                            </div>
                          </div>
                        </div>
                      </td>

                      <td style="padding: 8px 6px;">
                        <span v-if="item.pushStatus === 'success'" class="status-tag success">✓ Đã dán</span>
                        <span v-else-if="item.pushStatus === 'duplicate'" class="status-tag" style="background: rgba(245,158,11,0.16); color: #f59e0b; border: 1px solid rgba(245,158,11,0.4);" title="Bị bỏ qua vì đã có giá trị trùng trong cột trên Sheet">⚠ Trùng</span>
                        <span v-else-if="item.pushStatus === 'error'" class="status-tag error">✕ Lỗi</span>
                        <span v-else-if="item.pushStatus === 'pushing'" class="status-tag pushing">Đang dán...</span>
                        <span v-else class="status-tag waiting">Chờ dán</span>
                      </td>

                      <td style="padding: 8px 4px; text-align: center;">
                        <Loader2 v-if="item.pushStatus === 'pushing'" :size="13" class="spin-icon" style="color: var(--wx-brand-accent);" />
                        <button v-else @click="deleteItemFromGroup(group, idx)" class="row-del-btn" title="Xóa mẫu này">
                          <Trash2 :size="13" />
                        </button>
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>

            </div>
          </div>

        </div>

      </div>

    </div>

    <!-- Modal Hướng dẫn Google Apps Script -->
    <div v-if="showScriptModal" class="lightbox-overlay" @click.self="showScriptModal = false">
      <div style="background: var(--wx-surface-base); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-lg); width: 640px; max-width: 92vw; max-height: 88vh; display: flex; flex-direction: column; padding: 20px; box-shadow: var(--wx-shadow-2xl); overflow: hidden;">
        <div style="display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--wx-border-default); padding-bottom: 12px; margin-bottom: 14px; flex-shrink: 0;">
          <h3 style="margin: 0; font-size: 15px; color: var(--wx-brand-accent);">Hướng Dẫn 1-Click Cài Đặt Google Apps Script</h3>
          <button @click="showScriptModal = false" style="background: none; border: none; color: var(--wx-text-muted); cursor: pointer; font-size: 16px;">✕</button>
        </div>
        
        <div style="overflow-y: auto; flex: 1; padding-right: 4px;">
          <ol style="padding-left: 20px; font-size: 12.5px; line-height: 1.6; color: var(--wx-text-secondary); margin: 0 0 12px 0;">
            <li>Mở file Google Sheet của anh trên trình duyệt.</li>
            <li>Chọn <strong>Tiện ích mở rộng (Extensions)</strong> → <strong>Apps Script</strong>.</li>
            <li>Xóa toàn bộ mã cũ và <strong>Dán đoạn mã dưới đây</strong> vào:</li>
          </ol>

          <div style="background: var(--wx-surface-sunken); border: 1px solid var(--wx-border-default); border-radius: 6px; padding: 12px; margin-bottom: 14px; max-height: 280px; overflow: auto;">
            <pre style="margin: 0; font-family: monospace; font-size: 11.5px; color: #a5f3fc;"><code>{{ appsScriptCode }}</code></pre>
          </div>

          <ol style="padding-left: 20px; font-size: 12.5px; line-height: 1.6; color: var(--wx-text-secondary); margin: 0 0 14px 0;" start="4">
            <li>Bấm <strong>Triển khai (Deploy)</strong> → <strong>Quản lý các bản triển khai (Manage deployments)</strong>.</li>
            <li>Bấm ✏️ (Chỉnh sửa) → Ở mục <em>Phiên bản</em> chọn <strong>Phiên bản mới (New version)</strong>.</li>
            <li>Mục <strong>Who has access (Quyền truy cập)</strong>: Chọn <strong style="color: #ef4444;">Bất kỳ ai (Anyone)</strong>.</li>
            <li>Bấm <strong>Triển khai</strong> → Quay lại Tool bấm <strong>"Đẩy Mẫu Đã Chọn Lên Sheet"</strong>.</li>
          </ol>
        </div>

        <div style="display: flex; justify-content: space-between; align-items: center; padding-top: 12px; margin-top: 12px; border-top: 1px solid var(--wx-border-default); flex-shrink: 0;">
          <BaseButton variant="primary" size="sm" @click="copyAppsScript">
            <Copy :size="13" /> {{ copied ? '🎉 Đã Copy Code!' : 'Copy Toàn Bộ Mã Code' }}
          </BaseButton>
          <BaseButton variant="secondary" size="sm" @click="showScriptModal = false">Đóng</BaseButton>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { FileSpreadsheet, Sparkles, RefreshCw, Play, Loader2, Check, Trash2, Code, Copy, Folder, Plus, Sliders, PanelLeftOpen, PanelLeftClose, Upload } from 'lucide-vue-next'
import BaseButton from './common/BaseButton.vue'
import { FetchGoogleSheetStructure, PushGoogleSheetRow, PushGoogleSheetBatch, GetGoogleAppsScriptTemplate, GenerateAIContentText, GetGlobalSettings, SaveGlobalSettings } from '../../wailsjs/go/main/App'

const props = defineProps<{
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning', duration?: number) => void
}>()

const STORAGE_KEY = 'traffictool_google_sheet_sync_config'

const sheetUrl = ref('')
const webAppUrl = ref('')
const tabName = ref('')
const tabGid = ref('')

const inputTopics = ref('')
const variantCount = ref(3)

// Kẹp số mẫu nhập tay trong [1, 500] để tránh nhập 0/âm/quá lớn gây treo.
const onVariantCountInput = (event: Event) => {
  const raw = parseInt((event.target as HTMLInputElement).value, 10)
  if (Number.isNaN(raw)) return
  variantCount.value = Math.min(500, Math.max(1, raw))
}

interface DynamicTabInfo {
  name: string
  gid: string
  headers: string[]
  rawHeaders?: string[]
}

const availableTabs = ref<DynamicTabInfo[]>([])
const activeTabName = ref('')
const activeHeaders = ref<string[]>([])
const activeRawHeaders = ref<string[]>([])
const selectedHeaderNames = ref<Set<string>>(new Set())
const enforceLeadingEmoji = ref(true)

const EMOJI_LIST = ['😷', '🙈', '👿', '💅', '🔥', '✨', '🤣', '💬', '🌟', '📌', '💡', '🎉', '❤️', '🙌', '😎', '🥳', '🚀', '🎯', '⚡', '💯']

const hasAnyEmoji = (str: string): boolean => {
  if (!str) return false
  const emojiRegex = /(\p{Extended_Pictographic}|\p{Emoji_Presentation}|[\u{1F300}-\u{1F9FF}]|[\u{2600}-\u{26FF}]|[\u{2700}-\u{27BF}])/u
  return emojiRegex.test(str)
}

const addEmojiIfMissing = (str: string): string => {
  if (!str) return str
  let trimmed = str.trim()
  if (enforceLeadingEmoji.value && !hasAnyEmoji(trimmed)) {
    const randomEmoji = EMOJI_LIST[Math.floor(Math.random() * EMOJI_LIST.length)]
    trimmed = `${randomEmoji} ${trimmed}`
  }
  return trimmed
}

interface GeneratedRowItem {
  id: string
  selected: boolean
  topicName: string
  columnData: Record<string, string>
  columnTranslations?: Record<string, string>
  pushStatus: 'waiting' | 'pushing' | 'success' | 'error' | 'duplicate'
}

const getItemTranslation = (item: GeneratedRowItem, colName: string): string => {
  if (!item || !item.columnTranslations) return ''
  return item.columnTranslations[colName] || ''
}

const isLastRow = (group: TopicGroup, idx: number): boolean => {
  const items = visibleGroupItems(group)
  if (items.length <= 1) return false
  return idx >= items.length - 1
}

interface TopicGroup {
  topicId: string
  topicName: string
  isCollapsed: boolean
  isGeneratingMore: boolean
  items: GeneratedRowItem[]
}

const isGeneratingAI = ref(false)
const cancelGenerationFlag = ref(false)
const isPushing = ref(false)
const topicGroups = ref<TopicGroup[]>([])

// Bộ chọn ô/card lẻ để tạo lại
const selectedCells = ref<Set<string>>(new Set())

const isCellSelected = (itemId: string, colName: string): boolean => {
  return selectedCells.value.has(`${itemId}::${colName}`)
}

const toggleCellSelection = (item: GeneratedRowItem, colName: string) => {
  const key = `${item.id}::${colName}`
  if (selectedCells.value.has(key)) {
    selectedCells.value.delete(key)
  } else {
    selectedCells.value.add(key)
  }
  // Đồng bộ item.selected nếu có ít nhất 1 ô của hàng này được chọn
  item.selected = activeHeaders.value.some(h => selectedCells.value.has(`${item.id}::${h}`))
}

const isRowAllCellsSelected = (item: GeneratedRowItem): boolean => {
  return activeHeaders.value.every(h => isCellSelected(item.id, h))
}

const toggleRowCellsSelection = (item: GeneratedRowItem) => {
  const allSelected = isRowAllCellsSelected(item)
  item.selected = !allSelected
  activeHeaders.value.forEach(h => {
    const key = `${item.id}::${h}`
    if (allSelected) {
      selectedCells.value.delete(key)
    } else {
      selectedCells.value.add(key)
    }
  })
}

const hasSelectedCellsInGroup = (group: TopicGroup): boolean => {
  if (!group || !group.items) return false
  return group.items.some(item => {
    return activeHeaders.value.some(h => isCellSelected(item.id, h))
  })
}

const showScriptModal = ref(false)
const appsScriptCode = ref('')
const copied = ref(false)

// Thu gọn cột trái để tập trung xem bảng kết quả bên phải
const isLeftPanelCollapsed = ref(false)

const tabColumnSelections = ref<Record<string, string[]>>({})

// Prompt riêng cho từng cột, key = "tênTab::tênCột". Rỗng thì dùng mặc định (viết theo tên cột).
const columnPrompts = ref<Record<string, string>>({})

const columnPromptKey = (colName: string): string => `${activeTabName.value}::${colName}`

const getColumnPrompt = (colName: string): string => {
  return columnPrompts.value[columnPromptKey(colName)] || ''
}

const setColumnPrompt = (colName: string, val: string) => {
  columnPrompts.value[columnPromptKey(colName)] = val
  saveConfig()
}

const toggleHeaderSelection = (name: string) => {
  const newSet = new Set(selectedHeaderNames.value)
  if (newSet.has(name)) {
    newSet.delete(name)
  } else {
    newSet.add(name)
  }
  selectedHeaderNames.value = newSet
  if (activeTabName.value) {
    tabColumnSelections.value[activeTabName.value] = Array.from(newSet)
  }
  saveConfig()
}

const isAllHeadersSelected = computed(() => {
  return activeHeaders.value.length > 0 && selectedHeaderNames.value.size === activeHeaders.value.length
})

const toggleSelectAllHeaders = () => {
  if (isAllHeadersSelected.value) {
    selectedHeaderNames.value = new Set()
    if (activeTabName.value) {
      tabColumnSelections.value[activeTabName.value] = []
    }
  } else {
    selectedHeaderNames.value = new Set(activeHeaders.value)
    if (activeTabName.value) {
      tabColumnSelections.value[activeTabName.value] = Array.from(activeHeaders.value)
    }
  }
  saveConfig()
}

const selectTab = (tab: DynamicTabInfo) => {
  if (activeTabName.value && selectedHeaderNames.value) {
    tabColumnSelections.value[activeTabName.value] = Array.from(selectedHeaderNames.value)
  }

  activeTabName.value = tab.name
  tabName.value = tab.name
  tabGid.value = tab.gid
  activeHeaders.value = tab.headers || []
  activeRawHeaders.value = tab.rawHeaders || tab.headers || []

  // Khôi phục các cột đã tích chọn trước đó của tab này nếu có
  if (tabColumnSelections.value[tab.name] && tabColumnSelections.value[tab.name].length > 0) {
    const savedCols = tabColumnSelections.value[tab.name].filter(h => tab.headers.includes(h))
    selectedHeaderNames.value = new Set(savedCols.length > 0 ? savedCols : tab.headers)
  } else {
    selectedHeaderNames.value = new Set(tab.headers || [])
    tabColumnSelections.value[tab.name] = Array.from(selectedHeaderNames.value)
  }
  saveConfig()
}

const fetchSheetStructure = async (isManual: boolean = false) => {
  // Bắt buộc phải có Web App URL — đó là cầu nối DUY NHẤT đọc/ghi được đúng Sheet thật
  // của anh (mọi tab, mọi cột). Không còn data mẫu cứng như trước.
  if (!webAppUrl.value.trim()) {
    if (isManual) {
      props.showToast('Chưa có Web App URL. Bấm "Lấy mã Apps Script" để cài rồi dán URL vào!', 'warning')
      showScriptModal.value = true
    }
    availableTabs.value = []
    activeHeaders.value = []
    return
  }
  try {
    const res = await FetchGoogleSheetStructure(webAppUrl.value, sheetUrl.value)
    if (res && res.length > 0) {
      availableTabs.value = res

      const found = res.find((t: DynamicTabInfo) => (tabGid.value && t.gid === tabGid.value) || (tabName.value && t.name === tabName.value)) || res[0]
      selectTab(found)

      if (isManual) {
        props.showToast(`🎉 Đọc được ${res.length} Tab thật từ Google Sheet của anh!`, 'success')
      }
    } else {
      availableTabs.value = []
      activeHeaders.value = []
      if (isManual) props.showToast('Web App không trả về tab nào. Kiểm tra lại URL và quyền truy cập (Anyone).', 'warning')
    }
  } catch (e) {
    availableTabs.value = []
    activeHeaders.value = []
    if (isManual) props.showToast('Lỗi đọc Sheet: ' + String(e), 'error')
  }
}

const totalItemCount = computed(() => {
  return topicGroups.value.reduce((acc, g) => acc + g.items.length, 0)
})

const totalSelectedCount = computed(() => {
  return topicGroups.value.reduce((acc, g) => acc + g.items.filter(i => i.selected).length, 0)
})

const isAllGlobalSelected = computed(() => {
  return totalItemCount.value > 0 && totalSelectedCount.value === totalItemCount.value
})

const toggleSelectAllGlobal = () => {
  const nextVal = !isAllGlobalSelected.value
  topicGroups.value.forEach(group => {
    group.items.forEach(item => item.selected = nextVal)
  })
}

const isGroupAllSelected = (group: TopicGroup): boolean => {
  if (!group || group.items.length === 0) return false
  return group.items.every(item => {
    return activeHeaders.value.every(h => isCellSelected(item.id, h))
  })
}

const toggleGroupSelection = (group: TopicGroup) => {
  const allSelected = isGroupAllSelected(group)
  group.items.forEach(item => {
    item.selected = !allSelected
    activeHeaders.value.forEach(h => {
      const key = `${item.id}::${h}`
      if (allSelected) {
        selectedCells.value.delete(key)
      } else {
        selectedCells.value.add(key)
      }
    })
  })
}

const deleteGroup = (gIdx: number) => {
  topicGroups.value.splice(gIdx, 1)
}

const deleteItemFromGroup = (group: TopicGroup, itemIdx: number) => {
  group.items.splice(itemIdx, 1)
}

const loadSavedConfig = async () => {
  const keysToTry = [
    STORAGE_KEY,
    'google_sheet_sync_config_v5',
    'google_sheet_sync_config_v4',
    'google_sheet_sync_config_v3'
  ]
  for (const key of keysToTry) {
    const data = localStorage.getItem(key)
    if (data) {
      try {
        const cfg = JSON.parse(data)
        if (cfg.sheetUrl) sheetUrl.value = cfg.sheetUrl
        if (cfg.webAppUrl) webAppUrl.value = cfg.webAppUrl
        if (cfg.tabName) tabName.value = cfg.tabName
        if (cfg.tabGid) tabGid.value = cfg.tabGid
        if (cfg.variantCount) variantCount.value = cfg.variantCount
        if (cfg.inputTopics) inputTopics.value = cfg.inputTopics
        if (cfg.tabColumnSelections) tabColumnSelections.value = cfg.tabColumnSelections
        if (cfg.columnPrompts) columnPrompts.value = cfg.columnPrompts
        if (cfg.webAppUrl) break
      } catch (e) {}
    }
  }

  try {
    const settingsStr = await GetGlobalSettings()
    if (settingsStr) {
      const parsed = JSON.parse(settingsStr)
      const cfg = parsed.googleSheetConfig || parsed.sheetConfig
      if (cfg) {
        if (cfg.sheetUrl) sheetUrl.value = cfg.sheetUrl
        if (cfg.webAppUrl) webAppUrl.value = cfg.webAppUrl
        if (cfg.tabName) tabName.value = cfg.tabName
        if (cfg.tabGid) tabGid.value = cfg.tabGid
        if (cfg.variantCount) variantCount.value = cfg.variantCount
        if (cfg.inputTopics) inputTopics.value = cfg.inputTopics
        if (cfg.tabColumnSelections) tabColumnSelections.value = cfg.tabColumnSelections
        if (cfg.columnPrompts) columnPrompts.value = cfg.columnPrompts
      }
    }
  } catch (e) {}
}

let saveTimeout: any = null
const saveConfig = async () => {
  if (saveTimeout) clearTimeout(saveTimeout)
  saveTimeout = setTimeout(async () => {
    const cfgObj = {
      sheetUrl: sheetUrl.value,
      webAppUrl: webAppUrl.value,
      tabName: tabName.value,
      tabGid: tabGid.value,
      variantCount: variantCount.value,
      inputTopics: inputTopics.value,
      tabColumnSelections: tabColumnSelections.value,
      columnPrompts: columnPrompts.value
    }

    localStorage.setItem(STORAGE_KEY, JSON.stringify(cfgObj))

    try {
      const settingsStr = await GetGlobalSettings()
      let parsed: Record<string, any> = {}
      if (settingsStr) {
        try {
          parsed = JSON.parse(settingsStr)
        } catch (e) {}
      }
      parsed.googleSheetConfig = cfgObj
      await SaveGlobalSettings(JSON.stringify(parsed, null, 2))
    } catch (e) {}
  }, 500)
}

watch([sheetUrl, webAppUrl, tabName, tabGid, variantCount, inputTopics, tabColumnSelections, columnPrompts], () => {
  saveConfig()
}, { deep: true })

const onUrlInput = () => {
  fetchSheetStructure(false)
}

const copyAppsScript = () => {
  navigator.clipboard.writeText(appsScriptCode.value)
  copied.value = true
  setTimeout(() => copied.value = false, 2000)
}

const clearResults = () => {
  topicGroups.value = []
}

// Các cột được tick để AI sinh nội dung. CHỈ sinh đúng các cột này, mọi cột không tick
// để TRỐNG hoàn toàn — không điền chữ giả/placeholder.
const getColumnsToGenerate = (): string[] => {
  return activeHeaders.value.filter(h => selectedHeaderNames.value.has(h))
}

// Danh sách các cột hiển thị trên Bảng xem trước: CHỈ hiện các Cột được tích chọn!
const displayHeaders = computed(() => {
  const selected = activeHeaders.value.filter(h => selectedHeaderNames.value.has(h))
  return selected.length > 0 ? selected : activeHeaders.value
})

// Lọc bỏ hoàn toàn các dòng mẫu rỗng (không có dữ liệu hoặc chỉ có "...")
const visibleGroupItems = (group: TopicGroup): GeneratedRowItem[] => {
  if (!group || !group.items) return []
  return group.items.filter(item => {
    if (!item.columnData) return false
    return Object.values(item.columnData).some(val => val && String(val).trim() !== '' && String(val).trim() !== '...')
  })
}

// Gọi AI 1 lần cho 1 mẫu: trả về map { tên cột -> nội dung } & { tên cột_vi -> bản dịch tiếng Việt }
// Hỗ trợ truyền danh sách historyList các mẫu đã sinh trước đó để AI làm bộ nhớ đệm tránh trùng lặp ý tưởng.
const generateColumnsForTopic = async (
  topic: string,
  variantLabel: string,
  cols: string[],
  historyList: Record<string, string>[] = [],
  selfContextText = ''
): Promise<{ columnData: Record<string, string>, columnTranslations: Record<string, string> }> => {
  const columnData: Record<string, string> = {}
  const columnTranslations: Record<string, string> = {}
  if (cols.length === 0) return { columnData, columnTranslations }

  // Mỗi cột có thể có yêu cầu (prompt) riêng. Cột nào để trống thì viết tự do theo tên cột.
  const colLines = cols.map(c => {
    const custom = getColumnPrompt(c).trim()
    const emojiReq = enforceLeadingEmoji.value ? ' (chèn 1-3 biểu tượng Emoji/Icon sinh động, phù hợp ngữ cảnh bài viết như 😷, 🙈, 👿, 💅, 🔥...)' : ''
    return custom ? `- ${c}: ${custom}${emojiReq}` : `- ${c}: (viết nội dung hấp dẫn, ngắn gọn, phù hợp tên cột${emojiReq})`
  }).join('\n')

  const topicContext = (topic && topic !== 'Nội dung AI sinh theo cột') ? `Chủ đề: "${topic}" (${variantLabel}).\n` : `(${variantLabel}).\n`
  const emojiGlobalRule = enforceLeadingEmoji.value ? '\nYÊU CẦU: Tự nhiên chèn các biểu tượng Emoji/Icon cảm xúc sinh động, phù hợp ngữ cảnh ở các vị trí thích hợp trong bài viết.' : ''
  
  let historyContext = ''
  if (historyList && historyList.length > 0) {
    // Chỉ lấy tối đa 5 mẫu gần nhất làm bộ nhớ đệm (Sliding Window Memory) để tránh làm đầy cửa sổ ngữ cảnh, quá tải Token hoặc làm chậm tốc độ phản hồi của API
    const recentHistory = historyList.slice(-5)
    const historyText = recentHistory.map((h, i) => {
      const parts = Object.entries(h).map(([col, val]) => `+ ${col}: "${val}"`).join(', ')
      return `Mẫu đã sinh #${i + 1}: { ${parts} }`
    }).join('\n')
    historyContext = `\n\nĐÂY LÀ CÁC MẪU BẠN ĐÃ VIẾT TRƯỚC ĐÓ CHO CHỦ ĐỀ NÀY (HÃY ĐỌC KỸ ĐỂ LÀM BỘ NHỚ ĐỆM - TUYỆT ĐỐI TRÁNH TRÙNG LẶP Ý TƯỞNG, HÃY VIẾT CÁC MẪU MỚI KHÁC BIỆT HOÀN TOÀN, ĐA DẠNG GÓC NHÌN VÀ SÁNG TẠO HƠN):\n${historyText}`
  }

  const prompt = `Bạn là trợ lý viết nội dung chuyên nghiệp. ${topicContext}Hãy viết nội dung cho ĐÚNG các cột sau, TUÂN THỦ yêu cầu riêng của mỗi cột (phần sau dấu hai chấm):
${colLines}${emojiGlobalRule}${selfContextText}${historyContext}

CHỈ trả về đúng một object JSON hợp lệ với cấu trúc key:
- "tên_cột": "nội dung của cột đó"
- "tên_cột_vi": "NẾU nội dung trên là TIẾNG NƯỚC NGOÀI (tiếng Anh, v.v.), hãy cung cấp BẢN DỊCH TIẾNG VIỆT tự nhiên, mượt mà của câu đó vào key này. Nếu nội dung đã là tiếng Việt thì để rỗng \"\"."

Không thêm bất kỳ chữ giải thích nào khác, không bọc trong markdown.`

  const aiText = await GenerateAIContentText('', prompt)
  if (!aiText) return { columnData, columnTranslations }

  // Cắt lấy phần JSON (phòng khi AI kèm ```json ... ``` hoặc chữ thừa).
  let jsonStr = aiText.trim()
  const firstBrace = jsonStr.indexOf('{')
  const lastBrace = jsonStr.lastIndexOf('}')
  if (firstBrace !== -1 && lastBrace !== -1 && lastBrace > firstBrace) {
    jsonStr = jsonStr.slice(firstBrace, lastBrace + 1)
  }

  try {
    const parsed = JSON.parse(jsonStr)
    for (const c of cols) {
      if (parsed[c] != null && String(parsed[c]).trim() !== '') {
        columnData[c] = addEmojiIfMissing(String(parsed[c]).trim())
      }
      const viKey = `${c}_vi`
      if (parsed[viKey] != null && String(parsed[viKey]).trim() !== '') {
        columnTranslations[c] = String(parsed[viKey]).trim()
      }
    }
  } catch (e) {
    console.warn('AI trả về không phải JSON hợp lệ, bỏ qua mẫu này:', e)
  }

  return { columnData, columnTranslations }
}

// Hàm bọc tự động thử lại (retry) khi gọi AI sinh nội dung nếu gặp lỗi rate limit hoặc lỗi mạng
const generateColumnsForTopicWithRetry = async (
  topic: string,
  variantLabel: string,
  cols: string[],
  historyList: Record<string, string>[] = [],
  selfContextText = '',
  retries = 3,
  delay = 1000
): Promise<{ columnData: Record<string, string>, columnTranslations: Record<string, string> }> => {
  for (let attempt = 1; attempt <= retries; attempt++) {
    try {
      const res = await generateColumnsForTopic(topic, variantLabel, cols, historyList, selfContextText)
      if (res && Object.keys(res.columnData).length > 0) {
        return res
      }
      throw new Error("Không lấy được dữ liệu hợp lệ từ AI")
    } catch (e) {
      if (attempt === retries || cancelGenerationFlag.value) {
        throw e
      }
      console.warn(`[GoogleSheetSync] Thử lại sinh content lần ${attempt + 1}/${retries} sau lỗi:`, e)
      await new Promise(resolve => setTimeout(resolve, delay * attempt))
    }
  }
  return { columnData: {}, columnTranslations: {} }
}

// BƯỚC 1: Sinh nội dung cho các Cột được tick của Tab đang chọn
const generateAllTopicGroups = async () => {
  if (activeHeaders.value.length === 0) {
    props.showToast('Vui lòng bấm "Kiểm Tra Link" để nạp danh sách Cột từ Sheet!', 'warning')
    return
  }

  const colsToGen = getColumnsToGenerate()
  if (colsToGen.length === 0) {
    props.showToast('Vui lòng tích chọn ít nhất 1 cột để AI sinh nội dung!', 'warning')
    return
  }

  let lines = inputTopics.value.split('\n').map(l => l.trim()).filter(l => l.length > 0)
  if (lines.length === 0) {
    lines = ['Nội dung AI sinh theo cột']
  }

  isGeneratingAI.value = true
  cancelGenerationFlag.value = false
  clearResults()

  // Commit TỪNG nhóm/mẫu vào topicGroups ngay khi sinh xong (không gom rồi gán 1 lần
  // cuối). Nếu 1 call AI lỗi (rate limit/mạng) hoặc người dùng bấm Dừng, mọi mẫu đã
  // sinh trước đó vẫn được giữ nguyên trên bảng — không mất công + tiền API.
  let failCount = 0
  try {
    for (let lIdx = 0; lIdx < lines.length; lIdx++) {
      if (cancelGenerationFlag.value) break
      const topic = lines[lIdx]
      const count = variantCount.value || 1

      const group: TopicGroup = {
        topicId: `group_${Date.now()}_${lIdx}`,
        topicName: topic,
        isCollapsed: false,
        isGeneratingMore: false,
        items: []
      }
      topicGroups.value.push(group)

      for (let v = 0; v < count; v++) {
        if (cancelGenerationFlag.value) break
        const id = `item_${Date.now()}_${lIdx}_${v}`
        const colData: Record<string, string> = {}
        for (const h of activeHeaders.value) colData[h] = ''

        // Thu thập lịch sử các mẫu đã được sinh thành công trước đó trong nhóm này để làm bộ nhớ đệm
        const historyList = group.items.map(item => item.columnData)

        // Mỗi mẫu bọc riêng: 1 mẫu lỗi chỉ bỏ mẫu đó, không văng cả lô. Gặp lỗi rate
        // limit (429) thì lùi dần rồi thử lại vài lần trước khi bỏ qua.
        let genData: Record<string, string> = {}
        let genTrans: Record<string, string> = {}
        try {
          const r = await generateColumnsForTopicWithRetry(topic, `Mẫu biến thể #${v + 1}`, colsToGen, historyList)
          genData = r.columnData
          genTrans = r.columnTranslations
        } catch (e) {
          failCount++
          continue
        }
        for (const [col, val] of Object.entries(genData)) {
          colData[col] = val
        }

        const hasValidText = Object.values(colData).some(v => v && String(v).trim() !== '' && String(v).trim() !== '...')
        if (hasValidText) {
          group.items.push({
            id,
            selected: true,
            topicName: topic,
            columnData: colData,
            columnTranslations: genTrans,
            pushStatus: 'waiting'
          })
        }
      }
    }

    if (cancelGenerationFlag.value) {
      props.showToast(`Đã dừng. Giữ lại ${totalItemCount.value} mẫu đã sinh.`, 'info')
    } else if (failCount > 0) {
      props.showToast(`Sinh xong ${totalItemCount.value} mẫu (bỏ qua ${failCount} mẫu lỗi API).`, 'warning', 5000)
    } else {
      props.showToast(`🎉 AI đã sinh xong ${totalItemCount.value} mẫu cho ${topicGroups.value.length} nhóm thuộc Tab "${activeTabName.value}"!`, 'success')
    }

  } catch (err) {
    props.showToast('Lỗi sinh nội dung AI: ' + String(err), 'error')
  } finally {
    isGeneratingAI.value = false
    cancelGenerationFlag.value = false
  }
}

const addExtraSampleToGroup = async (group: TopicGroup) => {
  const colsToGen = getColumnsToGenerate()
  if (colsToGen.length === 0) {
    props.showToast('Vui lòng tích chọn ít nhất 1 cột để AI sinh nội dung!', 'warning')
    return
  }
  group.isGeneratingMore = true
  try {
    const vIdx = group.items.length + 1
    const colData: Record<string, string> = {}
    for (const h of activeHeaders.value) colData[h] = ''

    const historyList = group.items.map(item => item.columnData)
    const { columnData: genData, columnTranslations: genTrans } = await generateColumnsForTopic(group.topicName, `Mẫu bổ sung #${vIdx}`, colsToGen, historyList)
    for (const [col, val] of Object.entries(genData)) {
      colData[col] = val
    }

    group.items.push({
      id: `item_extra_${Date.now()}`,
      selected: true,
      topicName: group.topicName,
      columnData: colData,
      columnTranslations: genTrans,
      pushStatus: 'waiting'
    })

    props.showToast(`Đã tạo thêm Mẫu #${vIdx} cho nhóm "${group.topicName}"!`, 'success')
  } finally {
    group.isGeneratingMore = false
  }
}

const pushSelectedRowsToSheet = async () => {
  const selectedItemsToPush: GeneratedRowItem[] = []
  topicGroups.value.forEach(group => {
    group.items.forEach(item => {
      if (item.selected) selectedItemsToPush.push(item)
    })
  })

  if (selectedItemsToPush.length === 0) {
    props.showToast('Vui lòng tích chọn ít nhất 1 mẫu để đẩy lên Google Sheet!', 'warning')
    return
  }

  if (!webAppUrl.value.trim()) {
    props.showToast('Vui lòng dán Web App URL của Google Apps Script để đẩy dữ liệu!', 'warning')
    showScriptModal.value = true
    return
  }

  isPushing.value = true
  let successCount = 0

  try {
    const headerListToUse = activeRawHeaders.value.length > 0 ? activeRawHeaders.value : activeHeaders.value
    
    // Đánh dấu 'pushing' cho tất cả hàng được chọn
    selectedItemsToPush.forEach(item => {
      item.pushStatus = 'pushing'
    })

    // Xây dựng mảng dữ liệu 2 chiều cho batch
    const rowsData = selectedItemsToPush.map(item => {
      return headerListToUse.map(h => (h ? (item.columnData[h] || '') : ''))
    })

    let duplicateCount = 0
    try {
      const resp: any = await PushGoogleSheetBatch(webAppUrl.value, tabGid.value, tabName.value, rowsData, headerListToUse)
      // results[]: trạng thái từng dòng theo đúng thứ tự đã gửi ("inserted" / "duplicate").
      // Apps Script bỏ qua dòng trùng (giá trị đã tồn tại ở chính cột đó) nên đánh dấu riêng.
      const results: string[] = Array.isArray(resp?.results) ? resp.results : []
      selectedItemsToPush.forEach((item, i) => {
        if (results[i] === 'duplicate') {
          item.pushStatus = 'duplicate'
          duplicateCount++
        } else {
          item.pushStatus = 'success'
          successCount++
        }
      })
      // Fallback khi Apps Script cũ chưa trả results (chưa cập nhật mã): coi như đã dán hết.
      if (results.length === 0) {
        selectedItemsToPush.forEach(item => { item.pushStatus = 'success' })
        successCount = selectedItemsToPush.length
      }
    } catch (err) {
      console.error('Lỗi đẩy batch:', err)
      props.showToast(`Lỗi đẩy dữ liệu: ${String(err)}`, 'error')
      selectedItemsToPush.forEach(item => {
        item.pushStatus = 'error'
      })
    }

    if (successCount > 0 && duplicateCount > 0) {
      props.showToast(`🎉 Đã dán ${successCount} mẫu lên Tab "${activeTabName.value}". Bỏ qua ${duplicateCount} mẫu trùng dữ liệu.`, 'success')
    } else if (successCount > 0) {
      props.showToast(`🎉 Đã đẩy thành công ${successCount}/${selectedItemsToPush.length} mẫu lên Tab "${activeTabName.value}" của Google Sheet!`, 'success')
    } else if (duplicateCount > 0) {
      props.showToast(`Tất cả ${duplicateCount} mẫu đã trùng dữ liệu sẵn có trên Sheet nên không dán mẫu nào.`, 'warning')
    } else {
      props.showToast('Lỗi đẩy dữ liệu lên Google Sheet!', 'error')
    }

  } catch (err) {
    props.showToast('Lỗi tiến trình đẩy Sheet: ' + String(err), 'error')
  } finally {
    isPushing.value = false
  }
}

const regenerateSelectedCellsInGroup = async (group: TopicGroup) => {
  const colsToGen = getColumnsToGenerate()
  if (colsToGen.length === 0) {
    props.showToast('Vui lòng tích chọn ít nhất 1 cột để AI sinh nội dung!', 'warning')
    return
  }

  // Lọc lấy các hàng có ô được chọn
  const itemsToProcess = group.items.filter(item => {
    return activeHeaders.value.some(h => isCellSelected(item.id, h))
  })

  if (itemsToProcess.length === 0) {
    props.showToast('Vui lòng tích chọn ít nhất 1 ô nội dung (card) để tạo lại!', 'warning')
    return
  }

  group.isGeneratingMore = true
  let successCount = 0

  try {
    for (let i = 0; i < group.items.length; i++) {
      const item = group.items[i]
      
      // Lấy danh sách các cột cần tạo lại của mẫu này
      const itemColsToRegen = activeHeaders.value.filter(h => isCellSelected(item.id, h))
      if (itemColsToRegen.length === 0) continue

      item.pushStatus = 'pushing'

      // Thu thập các cột đã có của mẫu này làm ngữ cảnh (nếu có)
      const existingContextParts: string[] = []
      activeHeaders.value.forEach(h => {
        if (!isCellSelected(item.id, h) && item.columnData[h]) {
          existingContextParts.push(`- Cột ${h} đã có sẵn nội dung: "${item.columnData[h]}"`)
        }
      })
      const selfContextText = existingContextParts.length > 0
        ? `\nLƯU Ý: Mẫu này đã có sẵn các thông tin sau, hãy viết nội dung mới cho phù hợp và ăn khớp logic với các cột này:\n${existingContextParts.join('\n')}`
        : ''

      // Lấy lịch sử các mẫu khác trong nhóm làm bộ nhớ đệm
      const otherItems = group.items.filter(it => it.id !== item.id)
      const historyList = otherItems.map(it => it.columnData)

      try {
        const { columnData: genData, columnTranslations: genTrans } = await generateColumnsForTopic(
          group.topicName,
          `Tạo lại Mẫu #${i + 1}`,
          itemColsToRegen,
          historyList,
          selfContextText
        )

        if (!item.columnTranslations) {
          item.columnTranslations = {}
        }

        // Cập nhật các cột được sinh lại
        for (const col of itemColsToRegen) {
          if (genData[col] != null) {
            item.columnData[col] = genData[col]
          }
          if (genTrans[col] != null) {
            item.columnTranslations[col] = genTrans[col]
          }
          // Bỏ chọn ô này sau khi sinh thành công
          selectedCells.value.delete(`${item.id}::${col}`)
        }

        item.pushStatus = 'waiting'
        successCount++
      } catch (err) {
        item.pushStatus = 'error'
        console.error(`Lỗi tạo lại mẫu #${i + 1}:`, err)
      }
    }

    if (successCount > 0) {
      props.showToast(`Đã tạo lại thành công các ô đã chọn trong nhóm "${group.topicName}"!`, 'success')
    } else {
      props.showToast('Không có ô nào được tạo lại thành công.', 'error')
    }
  } finally {
    group.isGeneratingMore = false
  }
}

const openScriptModal = async () => {
  try {
    appsScriptCode.value = await GetGoogleAppsScriptTemplate()
  } catch (e) {
    console.error('Lỗi nạp mã Apps Script:', e)
  }
  showScriptModal.value = true
}

onMounted(async () => {
  await loadSavedConfig()
  await fetchSheetStructure(false)
})
</script>

<style scoped src="./BrowserAIPage.scoped.css"></style>
<style scoped src="./GoogleSheetSyncPage.scoped.css"></style>

