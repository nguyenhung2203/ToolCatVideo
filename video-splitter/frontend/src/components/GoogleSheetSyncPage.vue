<template>
  <div class="browser-ai-page">
    
    <!-- Header trên cùng chuẩn CapCut/TrafficTool -->
    <div class="dl-page-header">
      <h2 class="dl-page-title">
        <FileSpreadsheet :size="22" class="dl-title-icon" style="color: #22c55e;" />
        <span>Đồng Bộ Google Sheet & Sinh Content AI</span>
        <span class="dl-title-sub">Tự động đọc cấu trúc Tab & Cột trên Google Sheet của anh để chọn lọc sinh AI</span>
      </h2>

      <div style="display: flex; gap: 8px;">
        <button @click="showScriptModal = true" class="img-dir-btn" title="Xem hướng dẫn cài đặt Apps Script 1-Click">
          <Code :size="14" /> Lấy mã Apps Script (1-Click)
        </button>
      </div>
    </div>

    <!-- Workspace Grid 2 Cột chuẩn (3.6fr / 6.4fr) -->
    <div class="workspace-grid" style="grid-template-columns: 3.6fr 6.4fr;">
      
      <!-- Cột Trái: Cấu Hình Yêu Cầu & Tham Số -->
      <div class="config-panel" style="gap: 14px;">
        
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
                style="flex: 1;"
              />
              <button @click="fetchSheetStructure(true)" class="img-dir-btn btn-accent-blue" title="Bấm để tự đọc tất cả các Tab & Cột">
                <RefreshCw :size="13" /> Kiểm Tra Link
              </button>
            </div>
          </div>

          <div class="form-row" style="margin-bottom: 10px;">
            <label style="font-size: 11.5px; font-weight: 600; color: var(--wx-text-secondary);">
              Google Apps Script Web App URL:
            </label>
            <input
              type="text"
              v-model="webAppUrl"
              @input="saveConfig"
              @change="fetchSheetStructure(false)"
              placeholder="https://script.google.com/macros/s/AKfycb.../exec"
              class="img-text-input"
            />
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
          <h4 class="panel-section-title" style="display: flex; align-items: center; justify-content: space-between;">
            <span style="display: flex; align-items: center; gap: 6px;">
              <span class="step-num">2</span>
              <span>CHỌN CỘT AI SINH NỘI DUNG</span>
            </span>
            <span style="font-size: 11px; color: #34d399; text-transform: none; font-weight: normal;">
              ({{ selectedHeaderNames.size }}/{{ activeHeaders.length }} cột chọn)
            </span>
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

          <!-- Chọn Số Mẫu Biến Thể -->
          <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 12px; padding-top: 10px; border-top: 1px dashed var(--wx-border-default);">
            <span style="font-size: 11.5px; font-weight: 600; color: var(--wx-text-secondary);">Số mẫu biến thể AI / chủ đề:</span>
            <div class="img-source-pills" style="gap: 4px;">
              <button v-for="v in [1, 2, 3, 5]" :key="v" :class="{ active: variantCount === v }" @click="variantCount = v" class="img-source-pill" style="height: 26px; padding: 0 10px; font-size: 11px;">
                {{ v }} mẫu
              </button>
            </div>
          </div>
        </div>

        <!-- BƯỚC 3: NHẬP CHỦ ĐỀ & BẮT ĐẦU -->
        <div class="step-card" style="flex: 1; display: flex; flex-direction: column;">
          <h4 class="panel-section-title" style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px;">
            <span style="display: flex; align-items: center; gap: 6px;">
              <span class="step-num">3</span>
              <span>NHẬP CHỦ ĐỀ / TỪ KHÓA</span>
            </span>
            <label style="font-size: 11px; text-transform: none; color: #38bdf8; font-weight: normal; cursor: pointer;">
              <input type="checkbox" v-model="useAI" style="accent-color: var(--wx-brand-primary);" /> Bật AI
            </label>
          </h4>
          
          <textarea
            v-model="inputTopics"
            placeholder="Nhập danh sách các chủ đề (mỗi câu 1 dòng)...
Ví dụ:
The Baby Who Would Not Stop Laughing
The Locked Attic Mystery
A Stranger Bought the Entire Shop"
            class="img-text-input prompt-auto-textarea"
            style="flex: 1; min-height: 100px;"
          ></textarea>
        </div>

        <!-- Nút Hành Động Bước 1 -->
        <div class="config-footer-row" style="padding-top: 6px;">
          <button
            @click="generateAllTopicGroups"
            :disabled="isGeneratingAI || !inputTopics.trim()"
            class="img-action-btn start-generate-btn"
            style="width: 100%; height: 44px; font-size: 13.5px; background: linear-gradient(135deg, #8b5cf6, #6366f1); box-shadow: 0 4px 15px rgba(139, 92, 246, 0.4);"
          >
            <Sparkles v-if="!isGeneratingAI" :size="17" />
            <Loader2 v-else :size="17" class="spin-icon" />
            <span>{{ isGeneratingAI ? 'AI Đang Sinh Nội Dung Nhanh...' : '🤖 1. Sinh Content AI (Theo Tab & Cột Đã Chọn)' }}</span>
          </button>
        </div>

      </div>

      <!-- Cột Phải: Workspace Bảng Xem Trước, Chọn Lọc & Đẩy Sheet -->
      <div class="preview-panel" style="justify-content: flex-start; align-items: stretch; padding: var(--wx-space-4);">
        
        <!-- Màn hình chờ (Placeholder) khi chưa có kết quả -->
        <div v-if="topicGroups.length === 0" class="state-placeholder" style="margin: auto;">
          <div class="placeholder-decor">
            <FileSpreadsheet :size="60" style="color: #22c55e;" />
          </div>
          <h3 class="state-title">Tab đang chọn: {{ activeTabName || 'Chưa chọn' }}</h3>
          <p class="state-description">
            Đã sẵn sàng tạo nội dung cho {{ activeHeaders.length }} cột trên Tab <strong>{{ activeTabName }}</strong>. Điền danh sách chủ đề bên trái và nhấn "Sinh Content AI" để bắt đầu.
          </p>
        </div>

        <!-- Màn hình Kết Quả Gom Nhóm & Chọn Lọc -->
        <div v-else style="display: flex; flex-direction: column; gap: 12px; height: 100%;">
          
          <!-- Header Bar trên cùng của Bảng -->
          <div style="display: flex; align-items: center; justify-content: space-between; background: var(--wx-surface-sunken); border: 1.5px solid var(--wx-border-default); padding: 10px 14px; border-radius: var(--wx-radius-md);">
            <div style="display: flex; align-items: center; gap: 10px;">
              <input type="checkbox" :checked="isAllGlobalSelected" @change="toggleSelectAllGlobal" style="width: 16px; height: 16px; cursor: pointer; accent-color: var(--wx-brand-primary);" />
              <span style="font-size: 13px; font-weight: 700; color: var(--wx-text-primary);">
                Chọn tất cả ({{ totalSelectedCount }}/{{ totalItemCount }} mẫu)
              </span>
              <span v-if="totalSelectedCount > 0" style="font-size: 11.5px; background: rgba(16, 185, 129, 0.2); color: #34d399; padding: 3px 10px; border-radius: 12px; font-weight: 700; border: 1px solid rgba(16, 185, 129, 0.4);">
                Sẵn sàng đẩy lên Tab "{{ activeTabName }}"
              </span>
            </div>

            <div style="display: flex; align-items: center; gap: 8px;">
              <button @click="clearResults" class="img-dir-btn" style="height: 34px; font-size: 12px; color: #f87171; border-color: rgba(239, 68, 68, 0.3);">
                <Trash2 :size="13" /> Xóa tất cả
              </button>

              <!-- Nút Đẩy Sheet -->
              <button
                @click="pushSelectedRowsToSheet"
                :disabled="isPushing || totalSelectedCount === 0"
                class="img-action-btn start-generate-btn"
                style="height: 36px; padding: 0 18px; font-size: 13px; background: linear-gradient(135deg, #10b981, #059669); box-shadow: 0 4px 14px rgba(16, 185, 129, 0.35);"
              >
                <Play v-if="!isPushing" :size="15" />
                <Loader2 v-else :size="15" class="spin-icon" />
                <span>{{ isPushing ? 'Đang Đẩy Lên Sheet...' : `🚀 2. Đẩy (${totalSelectedCount}) Mẫu Đã Chọn Vào Tab "${activeTabName}"` }}</span>
              </button>
            </div>
          </div>

          <!-- Danh sách các Nhóm Chủ Đề (Collapsible Group Cards) -->
          <div class="custom-scroll-grid" style="flex: 1; overflow-y: auto; display: flex; flex-direction: column; gap: 14px; padding-right: 4px;">
            <div v-for="(group, gIdx) in topicGroups" :key="group.topicId" class="topic-group-box">
              
              <!-- Card Group Title Header -->
              <div class="topic-group-header">
                <div style="display: flex; align-items: center; gap: 10px;">
                  <input
                    type="checkbox"
                    :checked="isGroupAllSelected(group)"
                    @change="toggleGroupSelection(group)"
                    style="width: 16px; height: 16px; cursor: pointer; accent-color: var(--wx-brand-primary);"
                  />
                  <Folder :size="16" style="color: #a855f7;" />
                  <span style="font-size: 13.5px; font-weight: 700; color: var(--wx-text-primary);">
                    Nhóm #{{ gIdx + 1 }}: <strong style="color: #38bdf8;">{{ group.topicName }}</strong>
                  </span>
                  <span style="font-size: 11.5px; color: var(--wx-text-muted);">
                    ({{ group.items.filter(i => i.selected).length }}/{{ group.items.length }} mẫu được chọn)
                  </span>
                </div>

                <div style="display: flex; align-items: center; gap: 6px;">
                  <button
                    @click="addExtraSampleToGroup(group)"
                    :disabled="group.isGeneratingMore"
                    class="img-dir-btn"
                    style="height: 28px; padding: 0 10px; font-size: 11.5px; color: #c084fc; border-color: rgba(168, 85, 247, 0.4);"
                  >
                    <Loader2 v-if="group.isGeneratingMore" :size="12" class="spin-icon" />
                    <Plus v-else :size="12" />
                    <span>{{ group.isGeneratingMore ? 'Đang tạo...' : 'Sinh Thêm 1 Mẫu' }}</span>
                  </button>

                  <button @click="group.isCollapsed = !group.isCollapsed" class="img-dir-btn" style="height: 28px; padding: 0 10px; font-size: 11.5px;">
                    {{ group.isCollapsed ? 'Mở ▲' : 'Ẩn ▼' }}
                  </button>

                  <button @click="deleteGroup(gIdx)" class="img-dir-btn" style="height: 28px; padding: 0 8px; color: #ef4444; border-color: rgba(239,68,68,0.3);">
                    <Trash2 :size="13" />
                  </button>
                </div>
              </div>

              <!-- Bảng Chi Tiết Mẫu CỘT ĐỘNG 100% của Nhóm -->
              <div v-if="!group.isCollapsed" style="overflow-x: auto;">
                <table class="clean-data-table">
                  <thead>
                    <tr>
                      <th style="width: 42px; text-align: center;">Chọn</th>
                      <th style="width: 55px;">Mẫu</th>
                      
                      <!-- Render tiêu đề Cột Động -->
                      <th v-for="headerName in activeHeaders" :key="headerName" style="min-width: 140px;">
                        {{ headerName }}
                      </th>

                      <th style="width: 90px;">Trạng Thái</th>
                      <th style="width: 40px; text-align: center;"></th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="(item, idx) in group.items" :key="item.id" :class="{ row_active: item.selected }">
                      <td style="text-align: center; padding: 8px 4px;">
                        <input type="checkbox" v-model="item.selected" style="width: 15px; height: 15px; cursor: pointer; accent-color: var(--wx-brand-primary);" />
                      </td>
                      <td style="font-weight: 700; color: #c084fc; padding: 8px 4px;">#{{ idx + 1 }}</td>

                      <!-- Render ô sửa chữ theo các Cột Động -->
                      <td v-for="headerName in activeHeaders" :key="headerName" style="padding: 6px;">
                        <textarea
                          v-model="item.columnData[headerName]"
                          class="clean-cell-textarea"
                          rows="2"
                          placeholder="..."
                        ></textarea>
                      </td>

                      <td style="padding: 8px 6px;">
                        <span v-if="item.pushStatus === 'success'" class="status-tag success">✓ Đã dán</span>
                        <span v-else-if="item.pushStatus === 'error'" class="status-tag error">✕ Lỗi</span>
                        <span v-else-if="item.pushStatus === 'pushing'" class="status-tag pushing">Đang dán...</span>
                        <span v-else class="status-tag waiting">Chờ dán</span>
                      </td>

                      <td style="padding: 8px 4px; text-align: center;">
                        <button @click="deleteItemFromGroup(group, idx)" class="row-del-btn" title="Xóa mẫu này">
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
      <div style="background: var(--wx-surface-base); border: 1.5px solid var(--wx-border-default); border-radius: var(--wx-radius-lg); width: 600px; max-width: 90vw; padding: 20px; box-shadow: var(--wx-shadow-2xl);">
        <div style="display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--wx-border-default); padding-bottom: 12px; margin-bottom: 14px;">
          <h3 style="margin: 0; font-size: 15px; color: var(--wx-brand-accent);">Hướng Dẫn 1-Click Cài Đặt Google Apps Script</h3>
          <button @click="showScriptModal = false" style="background: none; border: none; color: var(--wx-text-muted); cursor: pointer; font-size: 16px;">✕</button>
        </div>
        
        <ol style="padding-left: 20px; font-size: 12.5px; line-height: 1.6; color: var(--wx-text-secondary); margin: 0 0 12px 0;">
          <li>Mở file Google Sheet của anh trên trình duyệt.</li>
          <li>Chọn <strong>Tiện ích mở rộng (Extensions)</strong> → <strong>Apps Script</strong>.</li>
          <li>Xóa toàn bộ mã cũ và **Dán đoạn mã dưới đây** vào:</li>
        </ol>

        <div style="position: relative; background: var(--wx-surface-sunken); border: 1px solid var(--wx-border-default); border-radius: 6px; padding: 12px; margin-bottom: 14px;">
          <pre style="margin: 0; font-family: monospace; font-size: 11.5px; color: #a5f3fc; overflow-x: auto;"><code>{{ appsScriptCode }}</code></pre>
          <button @click="copyAppsScript" class="img-dir-btn" style="position: absolute; top: 8px; right: 8px; height: 26px; font-size: 11px;">
            <Copy :size="12" /> {{ copied ? 'Đã copy!' : 'Copy Code' }}
          </button>
        </div>

        <ol style="padding-left: 20px; font-size: 12.5px; line-height: 1.6; color: var(--wx-text-secondary); margin: 0 0 14px 0;" start="4">
          <li>Bấm **Triển khai (Deploy)** → **Triển khai dưới dạng ứng dụng web (New deployment)**.</li>
          <li>Mục **Who has access (Quyền truy cập)**: Chọn <strong style="color: #ef4444;">Bất kỳ ai (Anyone)</strong>.</li>
          <li>Bấm **Triển khai** → Copy lấy đường **URL Web App** thu được dán vào ô bên trái trên Tool.</li>
        </ol>

        <div style="display: flex; justify-content: flex-end;">
          <button @click="showScriptModal = false" class="img-dir-btn">Đóng</button>
        </div>
      </div>
    </div>

  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { FileSpreadsheet, Sparkles, RefreshCw, Play, Loader2, Check, Trash2, Code, Copy, Folder, Plus, Sliders } from 'lucide-vue-next'
// @ts-ignore
const props = defineProps<{
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void
}>()

const STORAGE_KEY = 'traffictool_google_sheet_sync_config'

const sheetUrl = ref('https://docs.google.com/spreadsheets/d/1v3Mag53ppCe2v8y5MfCrjZ9SHGyNdts3TCdOtgTWfow/edit?gid=1228770940#gid=1228770940')
const webAppUrl = ref('')
const tabName = ref('WEB - THỦY')
const tabGid = ref('1228770940')

const inputTopics = ref('')
const useAI = ref(true)
const variantCount = ref(3)

interface DynamicTabInfo {
  name: string
  gid: string
  headers: string[]
}

const availableTabs = ref<DynamicTabInfo[]>([])
const activeTabName = ref('WEB - THỦY')
const activeHeaders = ref<string[]>([])
const selectedHeaderNames = ref<Set<string>>(new Set())

interface GeneratedRowItem {
  id: string
  selected: boolean
  topicName: string
  columnData: Record<string, string>
  pushStatus: 'waiting' | 'pushing' | 'success' | 'error'
}

interface TopicGroup {
  topicId: string
  topicName: string
  isCollapsed: boolean
  isGeneratingMore: boolean
  items: GeneratedRowItem[]
}

const isGeneratingAI = ref(false)
const isPushing = ref(false)
const topicGroups = ref<TopicGroup[]>([])

const showScriptModal = ref(false)
const appsScriptCode = ref('')
const copied = ref(false)

const toggleHeaderSelection = (name: string) => {
  const newSet = new Set(selectedHeaderNames.value)
  if (newSet.has(name)) {
    newSet.delete(name)
  } else {
    newSet.add(name)
  }
  selectedHeaderNames.value = newSet
}

const selectTab = (tab: DynamicTabInfo) => {
  activeTabName.value = tab.name
  tabName.value = tab.name
  tabGid.value = tab.gid
  activeHeaders.value = tab.headers || []
  selectedHeaderNames.value = new Set(tab.headers || [])
}

const fetchSheetStructure = async (isManual: boolean = false) => {
  if (!sheetUrl.value.trim()) return
  try {
    const res = await FetchGoogleSheetStructure(webAppUrl.value, sheetUrl.value)
    if (res && res.length > 0) {
      availableTabs.value = res
      
      const found = res.find((t: DynamicTabInfo) => (tabGid.value && t.gid === tabGid.value) || (tabName.value && t.name === tabName.value)) || res[0]
      selectTab(found)

      if (isManual) {
        props.showToast(`🎉 Tự động bóc tách được ${res.length} Tab & ${activeHeaders.value.length} Cột từ Sheet!`, 'success')
      }
    }
  } catch (e) {
    if (isManual) props.showToast('Không thể bóc tách cấu trúc Sheet', 'warning')
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

const isGroupAllSelected = (group: TopicGroup) => {
  return group.items.length > 0 && group.items.every(i => i.selected)
}

const toggleGroupSelection = (group: TopicGroup) => {
  const nextVal = !isGroupAllSelected(group)
  group.items.forEach(i => i.selected = nextVal)
}

const deleteGroup = (gIdx: number) => {
  topicGroups.value.splice(gIdx, 1)
}

const deleteItemFromGroup = (group: TopicGroup, itemIdx: number) => {
  group.items.splice(itemIdx, 1)
}

const loadSavedConfig = () => {
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
        if (cfg.webAppUrl) break
      } catch (e) {}
    }
  }
}

const saveConfig = () => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({
    sheetUrl: sheetUrl.value,
    webAppUrl: webAppUrl.value,
    tabName: tabName.value,
    tabGid: tabGid.value,
    variantCount: variantCount.value,
    inputTopics: inputTopics.value
  }))
}

watch([sheetUrl, webAppUrl, tabName, tabGid, variantCount, inputTopics], () => {
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

// BƯỚC 1: Sinh nội dung cho các Cột Động của Tab đang chọn
const generateAllTopicGroups = async () => {
  const lines = inputTopics.value.split('\n').map(l => l.trim()).filter(l => l.length > 0)
  if (lines.length === 0) {
    props.showToast('Vui lòng nhập danh sách từ khóa/chủ đề', 'warning')
    return
  }

  if (activeHeaders.value.length === 0) {
    props.showToast('Vui lòng bấm "Kiểm Tra Link" để nạp danh sách Cột từ Sheet!', 'warning')
    return
  }

  isGeneratingAI.value = true
  clearResults()

  try {
    const groups: TopicGroup[] = []
    let globalCounter = 1

    for (let lIdx = 0; lIdx < lines.length; lIdx++) {
      const topic = lines[lIdx]
      const count = variantCount.value || 1
      const groupItems: GeneratedRowItem[] = []

      for (let v = 0; v < count; v++) {
        const id = `item_${Date.now()}_${lIdx}_${v}`
        const colData: Record<string, string> = {}

        for (const h of activeHeaders.value) {
          const upperH = h.toUpperCase()
          if (upperH === 'STT') {
            colData[h] = String(globalCounter)
          } else if (upperH.includes('LINK')) {
            colData[h] = `https://baby.tandoori.com/article-${Date.now()}-${globalCounter}`
          } else if (upperH.includes('TRẠNG THÁI')) {
            colData[h] = 'Sẵn sàng'
          } else if (upperH.includes('LOẠI')) {
            colData[h] = 'Video'
          } else {
            colData[h] = `Mẫu ${v + 1} cho "${topic}"`
          }
        }

        if (useAI.value && selectedHeaderNames.value.size > 0) {
          try {
            const colsToGen = Array.from(selectedHeaderNames.value).filter(h => !['STT', 'LINK BÀI VIẾT', 'LINK WEB', 'TRẠNG THÁI'].includes(h.toUpperCase()))
            if (colsToGen.length > 0) {
              const prompt = `Đối với chủ đề video "${topic}" (Mẫu biến thể #${v + 1}), hãy viết nội dung cho các phần sau:\n` +
                colsToGen.map((colName, idx) => `${idx + 1}. [${colName}]:`).join('\n') +
                `\n\nYêu cầu: Viết hấp dẫn, kèm icon emoji phù hợp. Trả về đúng thứ tự các phần.`

              const aiText = await GenerateAIContentText('', prompt)
              if (aiText) {
                const parts = aiText.split('\n').map(p => p.replace(/^[0-9.-]+\s*/, '').replace(/^\[.*?\]:\s*/, '').trim()).filter(p => p.length > 0)
                colsToGen.forEach((colName, idx) => {
                  if (parts[idx]) {
                    colData[colName] = parts[idx]
                  }
                })
              }
            }
          } catch (e) {
            console.log('AI offline:', e)
          }
        }

        groupItems.push({
          id,
          selected: true,
          topicName: topic,
          columnData: colData,
          pushStatus: 'waiting'
        })

        globalCounter++
      }

      groups.push({
        topicId: `group_${Date.now()}_${lIdx}`,
        topicName: topic,
        isCollapsed: false,
        isGeneratingMore: false,
        items: groupItems
      })
    }

    topicGroups.value = groups
    props.showToast(`🎉 AI đã sinh xong ${totalItemCount.value} mẫu cho ${groups.length} nhóm thuộc Tab "${activeTabName.value}"!`, 'success')

  } catch (err) {
    props.showToast('Lỗi sinh nội dung AI: ' + String(err), 'error')
  } finally {
    isGeneratingAI.value = false
  }
}

const addExtraSampleToGroup = async (group: TopicGroup) => {
  group.isGeneratingMore = true
  try {
    const vIdx = group.items.length + 1
    const colData: Record<string, string> = {}

    for (const h of activeHeaders.value) {
      const upperH = h.toUpperCase()
      if (upperH === 'STT') {
        colData[h] = String(totalItemCount.value + 1)
      } else if (upperH.includes('LINK')) {
        colData[h] = `https://baby.tandoori.com/article-${Date.now()}`
      } else if (upperH.includes('TRẠNG THÁI')) {
        colData[h] = 'Sẵn sàng'
      } else {
        colData[h] = `Mẫu bổ sung #${vIdx} cho "${group.topicName}"`
      }
    }

    if (useAI.value && selectedHeaderNames.value.size > 0) {
      try {
        const colsToGen = Array.from(selectedHeaderNames.value).filter(h => !['STT', 'LINK BÀI VIẾT', 'LINK WEB', 'TRẠNG THÁI'].includes(h.toUpperCase()))
        if (colsToGen.length > 0) {
          const prompt = `Viết thêm 1 mẫu mới cho chủ đề "${group.topicName}" cho các phần:\n` +
            colsToGen.map((colName, idx) => `${idx + 1}. [${colName}]:`).join('\n')
          const aiText = await GenerateAIContentText('', prompt)
          if (aiText) {
            const parts = aiText.split('\n').map(p => p.replace(/^[0-9.-]+\s*/, '').replace(/^\[.*?\]:\s*/, '').trim()).filter(p => p.length > 0)
            colsToGen.forEach((colName, idx) => {
              if (parts[idx]) colData[colName] = parts[idx]
            })
          }
        }
      } catch (e) {}
    }

    group.items.push({
      id: `item_extra_${Date.now()}`,
      selected: true,
      topicName: group.topicName,
      columnData: colData,
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
    for (let i = 0; i < selectedItemsToPush.length; i++) {
      const item = selectedItemsToPush[i]
      item.pushStatus = 'pushing'

      const rowData: string[] = activeHeaders.value.map(h => item.columnData[h] || '')

      try {
        await PushGoogleSheetRow(webAppUrl.value, tabGid.value, tabName.value, rowData)
        item.pushStatus = 'success'
        successCount++
      } catch (err) {
        console.error('Lỗi đẩy dòng:', err)
        item.pushStatus = 'error'
      }
    }

    if (successCount > 0) {
      props.showToast(`🎉 Đã đẩy thành công ${successCount}/${selectedItemsToPush.length} mẫu lên Tab "${activeTabName.value}" của Google Sheet!`, 'success')
    } else {
      props.showToast('Lỗi đẩy dữ liệu lên Google Sheet!', 'error')
    }

  } catch (err) {
    props.showToast('Lỗi tiến trình đẩy Sheet: ' + String(err), 'error')
  } finally {
    isPushing.value = false
  }
}

onMounted(async () => {
  loadSavedConfig()
  await fetchSheetStructure(false)
  try {
    appsScriptCode.value = await GetGoogleAppsScriptTemplate()
  } catch (e) {}
})
</script>

<style scoped src="./BrowserAIPage.scoped.css"></style>
<style scoped>

/* Step Cards */
.step-card {
  background: var(--wx-surface-sunken);
  border: 1.5px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.step-num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--wx-brand-accent);
  color: #fff;
  font-size: 11px;
  font-weight: 700;
}

.btn-accent-blue {
  background: color-mix(in srgb, var(--wx-brand-primary) 20%, transparent) !important;
  color: #38bdf8 !important;
  border-color: rgba(56, 189, 248, 0.4) !important;
}

.btn-accent-blue:hover {
  background: color-mix(in srgb, var(--wx-brand-primary) 35%, transparent) !important;
}

/* Tab Select Pills Grid */
.tab-pills-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
  gap: 6px;
  margin-top: 4px;
}

.tab-select-pill {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 6px 10px;
  font-size: 12px;
  font-weight: 600;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.04);
  color: var(--wx-text-muted);
  border: 1px solid var(--wx-border-default);
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.tab-select-pill:hover {
  color: var(--wx-text-primary);
  border-color: rgba(255,255,255,0.2);
}

.tab-select-pill.active {
  background: color-mix(in srgb, var(--wx-brand-primary) 20%, transparent);
  border-color: #8b5cf6;
  color: #a855f7;
  font-weight: 700;
  box-shadow: 0 0 10px rgba(139, 92, 246, 0.25);
}

/* Column Chips Grid */
.column-chips-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 6px;
  margin-top: 4px;
  max-height: 160px;
  overflow-y: auto;
  padding-right: 2px;
}

.column-chip-card {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: rgba(0, 0, 0, 0.3);
  border: 1px solid var(--wx-border-default);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
}

.column-chip-card:hover {
  border-color: rgba(255, 255, 255, 0.25);
}

.column-chip-card.selected {
  background: color-mix(in srgb, var(--wx-brand-primary) 14%, transparent);
  border-color: #6366f1;
}

.chip-checkbox {
  width: 14px;
  height: 14px;
  accent-color: #6366f1;
  cursor: pointer;
  flex-shrink: 0;
}

.chip-title {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--wx-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* Topic Group Box & Header */
.topic-group-box {
  background: var(--wx-surface-sunken);
  border: 1.5px solid var(--wx-border-default);
  border-radius: var(--wx-radius-md);
  overflow: hidden;
}

.topic-group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  background: #111827;
  border-bottom: 1px solid var(--wx-border-default);
}

/* Clean Data Table & Textarea Inputs */
.clean-data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
  text-align: left;
}

.clean-data-table th {
  background: #0f172a;
  padding: 10px 10px;
  color: #94a3b8;
  font-weight: 700;
  border-bottom: 1px solid var(--wx-border-default);
}

.clean-data-table td {
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
  vertical-align: middle;
}

.clean-data-table tr.row_active {
  background: rgba(99, 102, 241, 0.04);
}

.clean-cell-textarea {
  width: 100%;
  background: #090d16;
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 6px;
  padding: 8px 10px;
  color: #f8fafc;
  font-size: 12px;
  line-height: 1.5;
  font-family: inherit;
  outline: none;
  resize: vertical;
  min-height: 52px;
  box-sizing: border-box;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.clean-cell-textarea:focus {
  border-color: #38bdf8;
  background: #000;
  box-shadow: 0 0 0 2px rgba(56, 189, 248, 0.2);
}

.status-tag {
  display: inline-block;
  padding: 3px 8px;
  border-radius: 10px;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}

.status-tag.success {
  background: rgba(34, 197, 94, 0.2);
  color: #4ade80;
}

.status-tag.error {
  background: rgba(239, 68, 68, 0.2);
  color: #f87171;
}

.status-tag.pushing {
  background: rgba(56, 189, 248, 0.2);
  color: #38bdf8;
}

.status-tag.waiting {
  color: var(--wx-text-muted);
}

.row-del-btn {
  background: none;
  border: none;
  color: var(--wx-text-muted);
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  transition: all 0.15s ease;
}

.row-del-btn:hover {
  color: #ef4444;
  background: rgba(239, 68, 68, 0.15);
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  100% { transform: rotate(360deg); }
}
</style>
