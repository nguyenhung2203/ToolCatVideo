<script setup lang="ts">
import { ref, onMounted, reactive, computed, watch } from 'vue'
import { Plus, Trash2, Save, Copy, Music, Image as ImageIcon, FileText, Layers, SlidersHorizontal, Film, Check, Wand2, Palette, Volume2, Type, Sparkles, Play, Pause, RotateCcw } from 'lucide-vue-next'
import { SelectImageFile, SelectAudioFile } from '../../wailsjs/go/main/App'

const props = defineProps<{
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void
  activeVideoSrc?: string
}>()

const emit = defineEmits<{
  (e: 'back'): void
}>()

interface RemixScenario {
  id: string
  name: string
  edit: any
}

const STORAGE_KEY = 'remix_scenarios_list'

const scenarios = ref<RemixScenario[]>([])
const editingId = ref<string>('')
const draftName = ref('')
const activeCategoryTab = ref<'video' | 'color' | 'audio' | 'subtitle'>('video')

// Real Video Player State
const videoRef = ref<HTMLVideoElement | null>(null)
const isPlaying = ref(false)
const currentTimeStr = ref('00:00:00')
const durationTimeStr = ref('00:00:00')

const targetLangs = [
  { code: '', name: 'Giữ nguyên gốc (không dịch)' },
  { code: 'vi', name: 'Tiếng Việt' },
  { code: 'en', name: 'Tiếng Anh' },
  { code: 'zh', name: 'Tiếng Trung' },
  { code: 'ja', name: 'Tiếng Nhật' },
  { code: 'ko', name: 'Tiếng Hàn' },
  { code: 'th', name: 'Tiếng Thái' },
  { code: 'es', name: 'Tiếng Tây Ban Nha' },
  { code: 'fr', name: 'Tiếng Pháp' },
]
const sourceLangs = [{ code: 'auto', name: 'Tự nhận diện' }, ...targetLangs.slice(1)]

const draft = reactive({
  hflip: false,
  speed: 1.0,
  aspectEnabled: false,
  aspectRatio: '9:16',
  aspectMode: 'blur',
  colorEnabled: false,
  colorPreset: '',
  colorBrightness: 0,
  colorContrast: 0,
  colorSaturation: 1.0,
  zoomEnabled: false,
  zoomFactor: 1.05,
  zoomDir: 'in',
  cropEnabled: false,
  cropPercent: 0.04,
  rotateEnabled: false,
  rotateDegrees: 1.5,
  noiseEnabled: false,
  noiseStrength: 10,
  trimStart: 0,
  trimEnd: 0,
  pitch: 0,
  stripMeta: false,
  wmEnabled: false,
  wmPath: '',
  wmScale: 0.2,
  wmOpacity: 1.0,
  wmPos: 'br',
  muteOriginal: false,
  musicPath: '',
  musicVolume: 0.3,
  musicLoop: false,
  subEnabled: false,
  subPath: '',
  subFontSize: 24,
  subMarginV: 40,
  subAutoGen: false,
  subSourceLang: 'auto',
  subTargetLang: '',
})

const load = () => {
  const data = localStorage.getItem(STORAGE_KEY)
  if (data) {
    try { scenarios.value = JSON.parse(data) as RemixScenario[] } catch (e) { console.error(e) }
  }
  if (scenarios.value.length > 0) {
    openEdit(scenarios.value[0])
  } else {
    editingId.value = 'new'
    draftName.value = 'Kịch bản mặc định'
    resetDraft()
  }
}

const persist = () => {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(scenarios.value))
}

onMounted(load)

// Live Video Controls
const togglePlay = () => {
  if (!videoRef.value) return
  if (videoRef.value.paused) {
    videoRef.value.play()
    isPlaying.value = true
  } else {
    videoRef.value.pause()
    isPlaying.value = false
  }
}

const restartVideo = () => {
  if (!videoRef.value) return
  videoRef.value.currentTime = 0
  videoRef.value.play()
  isPlaying.value = true
}

const formatSeconds = (sec: number) => {
  if (isNaN(sec)) return '00:00:00'
  const m = Math.floor(sec / 60)
  const s = Math.floor(sec % 60)
  const ms = Math.floor((sec % 1) * 10)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}:${String(ms).padStart(2, '0')}`
}

const onTimeUpdate = () => {
  if (videoRef.value) {
    currentTimeStr.value = formatSeconds(videoRef.value.currentTime)
  }
}

const onMetadataLoaded = () => {
  if (videoRef.value) {
    durationTimeStr.value = formatSeconds(videoRef.value.duration)
    videoRef.value.playbackRate = draft.speed
  }
}

watch(() => draft.speed, (newSpeed) => {
  if (videoRef.value) {
    videoRef.value.playbackRate = newSpeed
  }
})

const colorFilterStyle = computed(() => {
  if (!draft.colorEnabled) return 'none'
  const b = 1 + draft.colorBrightness
  const c = 1 + draft.colorContrast
  const s = draft.colorSaturation
  if (draft.colorPreset === 'warm') return `brightness(${b}) contrast(${c}) sepia(0.3) saturate(${s})`
  if (draft.colorPreset === 'cool') return `brightness(${b}) contrast(${c}) hue-rotate(30deg) saturate(${s})`
  if (draft.colorPreset === 'vivid') return `brightness(${b}) contrast(${c}) saturate(${s * 1.4})`
  if (draft.colorPreset === 'bw') return `brightness(${b}) contrast(${c}) grayscale(1)`
  if (draft.colorPreset === 'vintage') return `brightness(${b}) contrast(${c}) sepia(0.6) saturate(${s})`
  return `brightness(${b}) contrast(${c}) saturate(${s})`
})

const resetDraft = () => {
  Object.assign(draft, {
    hflip: false, speed: 1.0,
    aspectEnabled: false, aspectRatio: '9:16', aspectMode: 'blur',
    colorEnabled: false, colorPreset: '', colorBrightness: 0, colorContrast: 0, colorSaturation: 1.0,
    zoomEnabled: false, zoomFactor: 1.05, zoomDir: 'in',
    cropEnabled: false, cropPercent: 0.04,
    rotateEnabled: false, rotateDegrees: 1.5,
    noiseEnabled: false, noiseStrength: 10,
    trimStart: 0, trimEnd: 0, pitch: 0, stripMeta: false,
    wmEnabled: false, wmPath: '', wmScale: 0.2, wmOpacity: 1.0, wmPos: 'br',
    muteOriginal: false, musicPath: '', musicVolume: 0.3, musicLoop: false,
    subEnabled: false, subPath: '', subFontSize: 24, subMarginV: 40,
    subAutoGen: false, subSourceLang: 'auto', subTargetLang: '',
  })
}

const draftToEdit = (): any => {
  const wmXY: Record<string, [string, string]> = {
    tl: ['20', '20'], tr: ['W-w-20', '20'], bl: ['20', 'H-h-20'], br: ['W-w-20', 'H-h-20'], center: ['(W-w)/2', '(H-h)/2'],
  }
  const [wx, wy] = wmXY[draft.wmPos] || wmXY.br
  return {
    hflip: draft.hflip,
    speed: draft.speed,
    aspect: { enabled: draft.aspectEnabled, ratio: draft.aspectRatio, mode: draft.aspectMode },
    color: { enabled: draft.colorEnabled, preset: draft.colorPreset, brightness: draft.colorBrightness, contrast: draft.colorContrast, saturation: draft.colorSaturation },
    zoomPan: { enabled: draft.zoomEnabled, zoom: draft.zoomFactor, dir: draft.zoomDir },
    crop: { enabled: draft.cropEnabled, percent: draft.cropPercent },
    rotate: { enabled: draft.rotateEnabled, degrees: draft.rotateDegrees },
    noise: { enabled: draft.noiseEnabled, strength: draft.noiseStrength },
    trimStart: draft.trimStart,
    trimEnd: draft.trimEnd,
    pitch: draft.pitch,
    stripMeta: draft.stripMeta,
    watermark: { enabled: draft.wmEnabled, imgPath: draft.wmPath, x: wx, y: wy, scale: draft.wmScale, opacity: draft.wmOpacity },
    audio: { mute: draft.muteOriginal, musicPath: draft.musicPath, musicVolume: draft.musicVolume, musicLoop: draft.musicLoop, musicTracks: draft.musicPath ? [draft.musicPath] : [], volume: 1, fadeIn: 0, fadeOut: 0 },
    subtitle: { enabled: draft.subEnabled, path: draft.subPath, fontSize: draft.subFontSize, fontColor: '', outlineCol: '', marginV: draft.subMarginV, autoGen: draft.subAutoGen, sourceLang: draft.subSourceLang, targetLang: draft.subTargetLang },
  }
}

const editToDraft = (e: any) => {
  resetDraft()
  if (!e) return
  draft.hflip = !!e.hflip
  draft.speed = e.speed ?? 1.0
  if (e.aspect) { draft.aspectEnabled = !!e.aspect.enabled; draft.aspectRatio = e.aspect.ratio || '9:16'; draft.aspectMode = e.aspect.mode || 'blur' }
  if (e.color) { draft.colorEnabled = !!e.color.enabled; draft.colorPreset = e.color.preset || ''; draft.colorBrightness = e.color.brightness || 0; draft.colorContrast = e.color.contrast || 0; draft.colorSaturation = e.color.saturation ?? 1.0 }
  if (e.zoomPan) { draft.zoomEnabled = !!e.zoomPan.enabled; draft.zoomFactor = e.zoomPan.zoom || 1.05; draft.zoomDir = e.zoomPan.dir || 'in' }
  if (e.crop) { draft.cropEnabled = !!e.crop.enabled; draft.cropPercent = e.crop.percent || 0.04 }
  if (e.rotate) { draft.rotateEnabled = !!e.rotate.enabled; draft.rotateDegrees = e.rotate.degrees || 1.5 }
  if (e.noise) { draft.noiseEnabled = !!e.noise.enabled; draft.noiseStrength = e.noise.strength || 10 }
  draft.trimStart = e.trimStart || 0
  draft.trimEnd = e.trimEnd || 0
  draft.pitch = e.pitch || 0
  draft.stripMeta = !!e.stripMeta
  if (e.watermark) {
    draft.wmEnabled = !!e.watermark.enabled; draft.wmPath = e.watermark.imgPath || ''; draft.wmScale = e.watermark.scale || 0.2; draft.wmOpacity = e.watermark.opacity ?? 1.0
    const x = e.watermark.x, y = e.watermark.y
    if (x === '20' && y === '20') draft.wmPos = 'tl'
    else if (y === '20') draft.wmPos = 'tr'
    else if (x === '20') draft.wmPos = 'bl'
    else if (String(x).includes('(W-w)/2')) draft.wmPos = 'center'
    else draft.wmPos = 'br'
  }
  if (e.audio) { draft.muteOriginal = !!e.audio.mute; draft.musicPath = e.audio.musicPath || (e.audio.musicTracks && e.audio.musicTracks[0]) || ''; draft.musicVolume = e.audio.musicVolume ?? 0.3; draft.musicLoop = !!e.audio.musicLoop }
  if (e.subtitle) { draft.subEnabled = !!e.subtitle.enabled; draft.subPath = e.subtitle.path || ''; draft.subFontSize = e.subtitle.fontSize || 24; draft.subMarginV = e.subtitle.marginV ?? 40; draft.subAutoGen = !!e.subtitle.autoGen; draft.subSourceLang = e.subtitle.sourceLang || 'auto'; draft.subTargetLang = e.subtitle.targetLang || '' }
}

const openNew = () => {
  editingId.value = 'new'
  draftName.value = `Kịch bản ${scenarios.value.length + 1}`
  resetDraft()
}

const openEdit = (s: RemixScenario) => {
  editingId.value = s.id
  draftName.value = s.name
  editToDraft(s.edit)
}

const saveScenario = () => {
  if (!draftName.value.trim()) {
    props.showToast('Nhập tên kịch bản trước đã!', 'warning')
    return
  }
  const edit = draftToEdit()
  if (editingId.value === 'new') {
    const newObj = { id: Date.now().toString(), name: draftName.value.trim(), edit }
    scenarios.value.push(newObj)
    editingId.value = newObj.id
    props.showToast('Đã tạo và lưu kịch bản mới.', 'success')
  } else {
    const idx = scenarios.value.findIndex(s => s.id === editingId.value)
    if (idx !== -1) scenarios.value[idx] = { id: editingId.value, name: draftName.value.trim(), edit }
    props.showToast('Đã lưu kịch bản.', 'success')
  }
  persist()
}

const duplicateScenario = (s: RemixScenario) => {
  if (!s) return
  const newObj = { id: Date.now().toString(), name: s.name + ' (bản sao)', edit: JSON.parse(JSON.stringify(s.edit)) }
  scenarios.value.push(newObj)
  openEdit(newObj)
  persist()
  props.showToast('Đã nhân bản kịch bản.', 'info')
}

const deleteScenario = (id: string) => {
  scenarios.value = scenarios.value.filter(s => s.id !== id)
  persist()
  if (scenarios.value.length > 0) {
    openEdit(scenarios.value[0])
  } else {
    openNew()
  }
  props.showToast('Đã xóa kịch bản.', 'info')
}

const pickWatermark = async () => {
  try { const p = await SelectImageFile(); if (p) draft.wmPath = p } catch (e) { console.error(e) }
}
const pickMusic = async () => {
  try { const p = await SelectAudioFile(); if (p) draft.musicPath = p } catch (e) { console.error(e) }
}
const pickSubtitle = async () => {
  try {
    const mod: any = await import('../../wailsjs/go/main/App')
    if (mod.SelectSubtitleFile) {
      const p = await mod.SelectSubtitleFile()
      if (p) draft.subPath = p
    } else {
      props.showToast('Cần build lại app để bật chọn file phụ đề.', 'warning')
    }
  } catch (e) { console.error(e) }
}

const fileName = (p: string) => p ? (p.split('\\').pop() || p) : ''
</script>

<template>
  <div class="capcut-studio-page">
    <!-- ── 1. CAPCUT HEADER BAR ─────────────────────────────────────── -->
    <header class="capcut-header">
      <div class="capcut-header-left">
        <div class="capcut-brand-icon">
          <Film :size="18" />
        </div>
        <h2 class="capcut-title">
          CapCut Studio
          <span class="capcut-sub-title">Chỉnh Sửa Video Chi Tiết &amp; Soạn Kịch Bản</span>
        </h2>
      </div>

      <!-- Quick Preset Selector Chips Bar -->
      <div class="capcut-chips-bar">
        <span class="capcut-chips-lbl">
          <Layers :size="13" /> Kịch bản mẫu ({{ scenarios.length }}):
        </span>
        <div class="capcut-chips-list">
          <span v-for="s in scenarios" :key="s.id" class="preset-chip"
                @click="openEdit(s)"
                :class="{ 'preset-chip--active': editingId === s.id }">
            <span class="chip-label">{{ s.name }}</span>
          </span>
          <button @click="openNew" class="capcut-add-chip-btn">
            <Plus :size="12" /> Tạo mới
          </button>
        </div>
      </div>

      <div class="capcut-header-actions">
        <button v-if="editingId && editingId !== 'new'" class="btn capcut-sec-btn" @click="duplicateScenario(scenarios.find(s => s.id === editingId)!)" title="Nhân bản kịch bản này">
          <Copy :size="13" /> Nhân bản
        </button>
        <button v-if="editingId && editingId !== 'new'" class="btn capcut-danger-btn" @click="deleteScenario(editingId)" title="Xóa kịch bản này">
          <Trash2 :size="13" /> Xóa
        </button>
        <button class="btn capcut-save-btn" @click="saveScenario">
          <Save :size="14" /> {{ editingId === 'new' ? 'Lưu Kịch Bản Mới' : 'Lưu Thay Đổi' }}
        </button>
      </div>
    </header>

    <!-- ── 2. CAPCUT 3-PANEL WORKSPACE ─────────────────────────────── -->
    <div class="capcut-workspace">
      <!-- PANEL 1: Left Tools Category & Inspector -->
      <div class="capcut-left-panel">
        <div class="panel-section-title">
          <SlidersHorizontal :size="14" />
          <span>Bảng Điều Chỉnh Thông Số</span>
        </div>

        <div class="category-nav-bar">
          <button class="cat-nav-btn" :class="{ active: activeCategoryTab === 'video' }" @click="activeCategoryTab = 'video'">
            <Film :size="13" /> Biến đổi &amp; Tốc độ
          </button>
          <button class="cat-nav-btn" :class="{ active: activeCategoryTab === 'color' }" @click="activeCategoryTab = 'color'">
            <Palette :size="13" /> Tỷ lệ &amp; Màu
          </button>
          <button class="cat-nav-btn" :class="{ active: activeCategoryTab === 'audio' }" @click="activeCategoryTab = 'audio'">
            <Volume2 :size="13" /> Logo &amp; Âm thanh
          </button>
          <button class="cat-nav-btn" :class="{ active: activeCategoryTab === 'subtitle' }" @click="activeCategoryTab = 'subtitle'">
            <Type :size="13" /> Phụ đề &amp; AI Sub
          </button>
        </div>

        <div class="form-grid-blocks">
          <!-- CATEGORY 1: VIDEO MOTION & SPEED -->
          <div v-if="activeCategoryTab === 'video'" class="scenario-block-card">
            <h4 class="block-heading">1. Biến Đổi Video &amp; Tốc Độ</h4>

            <label class="toggle-switch-lbl">
              <input type="checkbox" v-model="draft.hflip" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Lật ngang video (Mirror)</span>
            </label>

            <div class="slider-field-group" style="margin-top: 10px;">
              <div class="slider-info-row">
                <span>Tốc độ phát: <strong>{{ draft.speed }}x</strong></span>
                <div class="quick-speed-pills">
                  <button class="sp-pill" :class="{ active: draft.speed === 1.0 }" @click="draft.speed = 1.0">1.0x</button>
                  <button class="sp-pill" :class="{ active: draft.speed === 1.05 }" @click="draft.speed = 1.05">1.05x</button>
                  <button class="sp-pill" :class="{ active: draft.speed === 1.15 }" @click="draft.speed = 1.15">1.15x</button>
                </div>
              </div>
              <input type="range" step="0.01" min="0.5" max="2.0" v-model.number="draft.speed" class="custom-range-slider" />
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 12px;">
              <input type="checkbox" v-model="draft.zoomEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Zoom Ken Burns ({{ draft.zoomFactor }}x)</span>
            </label>
            <div class="inline-field-row" v-if="draft.zoomEnabled" style="margin-top: 4px;">
              <input type="range" step="0.01" min="1.01" max="1.3" v-model.number="draft.zoomFactor" class="custom-range-slider" />
              <select v-model="draft.zoomDir" class="scenario-select">
                <option value="in">Giữa</option><option value="left">Trái</option><option value="right">Phải</option>
              </select>
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 10px;">
              <input type="checkbox" v-model="draft.cropEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Cắt rìa Crop ({{ Math.round(draft.cropPercent * 100) }}%)</span>
            </label>
            <div class="inline-field-row" v-if="draft.cropEnabled" style="margin-top: 4px;">
              <input type="range" step="0.01" min="0.01" max="0.15" v-model.number="draft.cropPercent" class="custom-range-slider" />
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 10px;">
              <input type="checkbox" v-model="draft.rotateEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Xoay nghiêng ({{ draft.rotateDegrees }}°)</span>
            </label>
            <div class="inline-field-row" v-if="draft.rotateEnabled" style="margin-top: 4px;">
              <input type="range" step="0.5" min="-5" max="5" v-model.number="draft.rotateDegrees" class="custom-range-slider" />
            </div>

            <div class="trim-inputs-row" style="margin-top: 12px; border-top: 1px dashed var(--wx-border-default); padding-top: 8px;">
              <div class="trim-item">
                <span>Trim đầu (s):</span>
                <input type="number" step="0.1" min="0" max="5" v-model.number="draft.trimStart" class="scenario-input-num" />
              </div>
              <div class="trim-item">
                <span>Trim đuôi (s):</span>
                <input type="number" step="0.1" min="0" max="5" v-model.number="draft.trimEnd" class="scenario-input-num" />
              </div>
            </div>
          </div>

          <!-- CATEGORY 2: ASPECT & COLOR -->
          <div v-else-if="activeCategoryTab === 'color'" class="scenario-block-card">
            <h4 class="block-heading">2. Tỷ Lệ Khung &amp; Bộ Lọc Màu</h4>

            <label class="toggle-switch-lbl">
              <input type="checkbox" v-model="draft.aspectEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Đổi tỷ lệ khung hình</span>
            </label>
            <div v-if="draft.aspectEnabled" class="sub-fields-group">
              <div class="aspect-pills-row">
                <button class="asp-pill" :class="{ active: draft.aspectRatio === '9:16' }" @click="draft.aspectRatio = '9:16'">9:16 (Dọc TikTok)</button>
                <button class="asp-pill" :class="{ active: draft.aspectRatio === '1:1' }" @click="draft.aspectRatio = '1:1'">1:1 (Vuông)</button>
                <button class="asp-pill" :class="{ active: draft.aspectRatio === '16:9' }" @click="draft.aspectRatio = '16:9'">16:9 (Ngang YT)</button>
              </div>
              <div class="inline-field-row" style="margin-top: 6px;">
                <span>Chế độ lót phông:</span>
                <select v-model="draft.aspectMode" class="scenario-select" style="width: 100%;">
                  <option value="blur">Phông Nền Mờ (Blur)</option>
                  <option value="crop">Cắt Phóng To Tràn Khung</option>
                  <option value="pad">Dải Viền Đen 2 Bên</option>
                </select>
              </div>
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 14px;">
              <input type="checkbox" v-model="draft.colorEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Điều chỉnh màu sắc (Color Grading)</span>
            </label>
            <div v-if="draft.colorEnabled" class="sub-fields-group">
              <div class="preset-color-chips">
                <button class="c-chip" :class="{ active: draft.colorPreset === '' }" @click="draft.colorPreset = ''">Màu tay</button>
                <button class="c-chip" :class="{ active: draft.colorPreset === 'warm' }" @click="draft.colorPreset = 'warm'">☀️ Ấm áp</button>
                <button class="c-chip" :class="{ active: draft.colorPreset === 'cool' }" @click="draft.colorPreset = 'cool'">❄️ Mát lạnh</button>
                <button class="c-chip" :class="{ active: draft.colorPreset === 'vivid' }" @click="draft.colorPreset = 'vivid'">🌈 Rực rỡ</button>
                <button class="c-chip" :class="{ active: draft.colorPreset === 'bw' }" @click="draft.colorPreset = 'bw'">🔳 Đen trắng</button>
              </div>
              <div class="slider-field-group" style="margin-top: 6px;">
                <div class="slider-info-row"><span>Độ sáng: <strong>{{ draft.colorBrightness }}</strong></span></div>
                <input type="range" min="-0.6" max="0.6" step="0.05" v-model.number="draft.colorBrightness" class="custom-range-slider" />
              </div>
              <div class="slider-field-group">
                <div class="slider-info-row"><span>Bão hòa: <strong>{{ draft.colorSaturation }}</strong></span></div>
                <input type="range" min="0.2" max="2.2" step="0.05" v-model.number="draft.colorSaturation" class="custom-range-slider" />
              </div>
            </div>
          </div>

          <!-- CATEGORY 3: LOGO & AUDIO -->
          <div v-else-if="activeCategoryTab === 'audio'" class="scenario-block-card">
            <h4 class="block-heading">3. Logo, Âm Thanh &amp; Metadata</h4>

            <label class="toggle-switch-lbl">
              <input type="checkbox" v-model="draft.wmEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Gắn Logo Watermark</span>
            </label>
            <div v-if="draft.wmEnabled" class="sub-fields-group">
              <div class="inline-field-row">
                <button class="btn-picker" @click="pickWatermark"><ImageIcon :size="12" /> Chọn Logo</button>
                <span class="file-name-hint">{{ fileName(draft.wmPath) || 'Chưa chọn' }}</span>
              </div>
              <div class="wm-pos-selector">
                <button class="pos-mini-btn" :class="{ active: draft.wmPos === 'tl' }" @click="draft.wmPos = 'tl'">↖ Trái Trên</button>
                <button class="pos-mini-btn" :class="{ active: draft.wmPos === 'tr' }" @click="draft.wmPos = 'tr'">↗ Phải Trên</button>
                <button class="pos-mini-btn" :class="{ active: draft.wmPos === 'bl' }" @click="draft.wmPos = 'bl'">↙ Trái Dưới</button>
                <button class="pos-mini-btn" :class="{ active: draft.wmPos === 'br' }" @click="draft.wmPos = 'br'">↘ Phải Dưới</button>
              </div>
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 12px;">
              <input type="checkbox" v-model="draft.muteOriginal" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Tắt tiếng video gốc</span>
            </label>
            <div class="inline-field-row" style="margin-top: 6px;">
              <button class="btn-picker" @click="pickMusic"><Music :size="12" /> Nhạc nền MP3</button>
              <span class="file-name-hint">{{ fileName(draft.musicPath) || 'Không có' }}</span>
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 12px;">
              <input type="checkbox" v-model="draft.stripMeta" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Tự động xóa sạch Metadata chống quét</span>
            </label>
          </div>

          <!-- CATEGORY 4: SUBTITLE -->
          <div v-else-if="activeCategoryTab === 'subtitle'" class="scenario-block-card">
            <h4 class="block-heading">4. Phụ Đề Video &amp; AI Whisper</h4>

            <label class="toggle-switch-lbl">
              <input type="checkbox" v-model="draft.subEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Ghép file phụ đề .SRT / .ASS</span>
            </label>
            <div v-if="draft.subEnabled" class="sub-fields-group">
              <div class="inline-field-row">
                <button class="btn-picker" @click="pickSubtitle"><FileText :size="12" /> Chọn File Sub</button>
                <span class="file-name-hint">{{ fileName(draft.subPath) || 'Chưa chọn' }}</span>
              </div>
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 12px;">
              <input type="checkbox" v-model="draft.subAutoGen" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Tự nghe Whisper AI tạo sub khi xuất</span>
            </label>
            <div v-if="draft.subAutoGen" class="sub-fields-group" style="background: rgba(37, 99, 235, 0.1); padding: 8px; border-radius: 6px;">
              <p class="auto-gen-hint" style="font-size: 11px; color: #38bdf8; margin: 0 0 6px;">Whisper AI tự nghe giọng trong video khi xuất và ghép phụ đề đẹp mắt.</p>
              <div class="inline-field-row">
                <span>Dịch sang:</span>
                <select v-model="draft.subTargetLang" class="scenario-select" style="width: 100%;">
                  <option v-for="l in targetLangs" :key="l.code" :value="l.code">{{ l.name }}</option>
                </select>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- PANEL 2: Center Player Preview Monitor -->
      <div class="capcut-center-panel">
        <div class="player-monitor-header">
          <span class="monitor-title">Màn Hình Live Preview CapCut</span>
          <span class="monitor-ratio-badge" v-if="draft.aspectEnabled">{{ draft.aspectRatio }}</span>
        </div>

        <div class="player-monitor-screen-wrap">
          <div
            class="player-monitor-frame"
            :style="{
              aspectRatio: draft.aspectEnabled ? draft.aspectRatio.replace(':', '/') : '16/9'
            }"
          >
            <!-- Real HTML5 Video Preview Player -->
            <video
              ref="videoRef"
              v-if="activeVideoSrc"
              :src="activeVideoSrc"
              class="real-capcut-video"
              :style="{
                transform: (draft.hflip ? 'scaleX(-1) ' : '') + (draft.zoomEnabled ? `scale(${draft.zoomFactor}) ` : '') + (draft.rotateEnabled ? `rotate(${draft.rotateDegrees}deg)` : ''),
                filter: colorFilterStyle
              }"
              loop
              playsinline
              @timeupdate="onTimeUpdate"
              @loadedmetadata="onMetadataLoaded"
            ></video>

            <!-- Video Placeholder when no video loaded -->
            <div v-else class="video-preview-placeholder">
              <Film :size="42" style="opacity: 0.3;" />
              <span>Chưa nạp video nguồn</span>
              <span class="sub-hint">(Thêm video ở tab "Cắt &amp; Xuất Video" để phát và xem hiệu ứng lật/màu sắc trực tiếp tại đây!)</span>
            </div>

            <!-- Overlay Badges Strip -->
            <div class="overlay-badge-strip">
              <span class="badge" v-if="draft.hflip">⇄ Mirror Lật</span>
              <span class="badge" v-if="draft.speed !== 1.0">⚡ {{ draft.speed }}x</span>
              <span class="badge" v-if="draft.aspectEnabled">📐 {{ draft.aspectRatio }}</span>
              <span class="badge" v-if="draft.colorEnabled">🎨 {{ draft.colorPreset || 'Chỉnh màu' }}</span>
              <span class="badge" v-if="draft.wmEnabled">🏷️ Logo</span>
              <span class="badge" v-if="draft.musicPath">🎵 Nhạc</span>
            </div>
          </div>
        </div>

        <!-- Transport Controls Bar -->
        <div class="player-transport-bar">
          <div class="timecode">{{ currentTimeStr }} / {{ durationTimeStr }}</div>
          <div class="transport-controls">
            <button class="transport-btn" @click="restartVideo" title="Tua lại từ đầu"><RotateCcw :size="13" /></button>
            <button class="transport-btn play" @click="togglePlay">
              <Pause v-if="isPlaying" :size="14" />
              <Play v-else :size="14" style="margin-left: 2px;" />
            </button>
          </div>
          <div class="transport-right">
            <span class="aspect-tag">{{ draft.aspectEnabled ? draft.aspectRatio : 'Tỷ lệ gốc' }}</span>
          </div>
        </div>
      </div>

      <!-- PANEL 3: Right Inspector & Scenario Manager -->
      <div class="capcut-right-panel">
        <div class="panel-section-title">
          <SlidersHorizontal :size="14" />
          <span>Thông Tin Kịch Bản Mẫu</span>
        </div>

        <div class="inspector-name-box">
          <label class="inspector-label">TÊN KỊCH BẢN MẪU:</label>
          <input
            v-model="draftName"
            type="text"
            class="inspector-input"
            placeholder="VD: TikTok Shorts 9:16 Mirror..."
          />
        </div>

        <div class="inspector-summary-card">
          <h4 class="summary-heading">Các hiệu ứng đang bật:</h4>
          <ul class="summary-checklist">
            <li :class="{ active: draft.hflip }"><Check :size="12" /> Lật ngang video (Mirror)</li>
            <li :class="{ active: draft.speed !== 1.0 }"><Check :size="12" /> Tốc độ phát: {{ draft.speed }}x</li>
            <li :class="{ active: draft.aspectEnabled }"><Check :size="12" /> Khung hình {{ draft.aspectRatio }} ({{ draft.aspectMode }})</li>
            <li :class="{ active: draft.colorEnabled }"><Check :size="12" /> Màu sắc: {{ draft.colorPreset || 'Chỉnh màu' }}</li>
            <li :class="{ active: draft.wmEnabled }"><Check :size="12" /> Logo Watermark</li>
            <li :class="{ active: draft.musicPath }"><Check :size="12" /> Nhạc nền MP3</li>
            <li :class="{ active: draft.subEnabled || draft.subAutoGen }"><Check :size="12" /> Phụ đề video</li>
            <li :class="{ active: draft.stripMeta }"><Check :size="12" /> Xóa Metadata chống quét</li>
          </ul>
        </div>

        <button class="btn capcut-save-btn-large" @click="saveScenario">
          <Save :size="15" /> {{ editingId === 'new' ? 'Lưu Thành Kịch Bản Mới' : 'Lưu Thay Đổi Kịch Bản' }}
        </button>
      </div>
    </div>

    <!-- ── 3. VISUAL CAPCUT TIMELINE TRACK ────────────────────────── -->
    <div class="capcut-timeline-bar">
      <div class="timeline-ruler">
        <span class="ruler-mark">00:00</span>
        <span class="ruler-mark">10:00</span>
        <span class="ruler-mark">20:00</span>
        <span class="ruler-mark">30:00</span>
        <span class="ruler-mark">40:00</span>
        <span class="ruler-mark">50:00</span>
        <span class="ruler-mark">01:00:00</span>
      </div>

      <div class="timeline-tracks">
        <!-- Track 1: Video -->
        <div class="timeline-track video-track">
          <div class="track-head">🎬 Video V1</div>
          <div class="track-content">
            <div class="track-block video-block">
              <span>Clip Video · Tốc độ {{ draft.speed }}x {{ draft.hflip ? '· Lật ngang Mirror' : '' }} {{ draft.aspectEnabled ? '· ' + draft.aspectRatio : '' }}</span>
            </div>
          </div>
        </div>

        <!-- Track 2: Audio -->
        <div class="timeline-track audio-track" v-if="draft.musicPath || draft.muteOriginal">
          <div class="track-head">🎵 Audio A1</div>
          <div class="track-content">
            <div class="track-block audio-block">
              <span>{{ draft.musicPath ? fileName(draft.musicPath) : 'Tắt tiếng video gốc' }}</span>
            </div>
          </div>
        </div>

        <!-- Track 3: Subtitle -->
        <div class="timeline-track sub-track" v-if="draft.subEnabled || draft.subAutoGen">
          <div class="track-head">📝 Sub S1</div>
          <div class="track-content">
            <div class="track-block sub-block">
              <span>{{ draft.subAutoGen ? 'Whisper AI Tự tạo Phụ Đề' : fileName(draft.subPath) }}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.capcut-studio-page {
  background: var(--wx-surface-base, #0f172a);
  border: 1px solid var(--wx-border-default, #1e293b);
  border-radius: var(--wx-radius-lg, 12px);
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  overflow: hidden;
  box-shadow: var(--wx-shadow-md);
  color: var(--wx-text-primary, #f8fafc);
}

/* Header */
.capcut-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  background: var(--wx-surface-sunken, #090d16);
  border-bottom: 1px solid var(--wx-border-default, #1e293b);
  gap: 12px;
  flex-shrink: 0;
}

.capcut-header-left {
  display: flex;
  align-items: center;
  gap: 8px;
}

.capcut-brand-icon {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: #2563eb;
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
}

.capcut-title {
  margin: 0;
  font-size: 14px;
  font-weight: 800;
  color: #ffffff;
  display: flex;
  flex-direction: column;
}

.capcut-sub-title {
  font-size: 10.5px;
  font-weight: 500;
  color: var(--wx-text-muted, #64748b);
}

.capcut-chips-bar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.capcut-chips-lbl {
  font-size: 11.5px;
  font-weight: 700;
  color: #38bdf8;
  display: flex;
  align-items: center;
  gap: 4px;
}

.capcut-chips-list {
  display: flex;
  align-items: center;
  gap: 6px;
}

.preset-chip {
  padding: 3px 10px;
  font-size: 11.5px;
  font-weight: 600;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 6px;
  color: #94a3b8;
  cursor: pointer;
  transition: all 0.15s ease;
}

.preset-chip:hover {
  border-color: #2563eb;
  color: #ffffff;
}

.preset-chip--active {
  background: #2563eb;
  border-color: #2563eb;
  color: #ffffff;
}

.capcut-add-chip-btn {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 600;
  background: transparent;
  border: 1px dashed #38bdf8;
  color: #38bdf8;
  border-radius: 6px;
  cursor: pointer;
}

.capcut-header-actions {
  display: flex;
  align-items: center;
  gap: 6px;
}

.capcut-save-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  height: 32px;
  padding: 0 14px;
  font-size: 12.5px;
  font-weight: 700;
  border-radius: 6px;
  border: none;
  background: #10b981;
  color: #ffffff;
  cursor: pointer;
}

.capcut-sec-btn, .capcut-danger-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 32px;
  padding: 0 10px;
  font-size: 11.5px;
  font-weight: 600;
  border-radius: 6px;
  border: 1px solid #334155;
  background: #1e293b;
  color: #cbd5e1;
  cursor: pointer;
}

.capcut-danger-btn:hover {
  border-color: #ef4444;
  color: #ef4444;
}

/* Workspace 3-Panel */
.capcut-workspace {
  display: flex;
  flex: 1;
  overflow: hidden;
}

/* Left Panel */
.capcut-left-panel {
  width: 320px;
  background: #0f172a;
  border-right: 1px solid #1e293b;
  display: flex;
  flex-direction: column;
  padding: 12px;
  gap: 10px;
  overflow-y: auto;
  flex-shrink: 0;
}

.panel-section-title {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  font-weight: 800;
  color: #38bdf8;
  text-transform: uppercase;
}

.category-nav-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  background: #1e293b;
  padding: 3px;
  border-radius: 8px;
}

.cat-nav-btn {
  flex: 1 1 45%;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 5px 8px;
  font-size: 11px;
  font-weight: 600;
  border: none;
  background: transparent;
  color: #94a3b8;
  border-radius: 6px;
  cursor: pointer;
}

.cat-nav-btn.active {
  background: #2563eb;
  color: #ffffff;
}

.scenario-block-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.block-heading {
  margin: 0 0 4px;
  font-size: 12.5px;
  font-weight: 700;
  color: #38bdf8;
}

/* Custom Toggle Switch */
.toggle-switch-lbl {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

.toggle-switch-lbl input {
  display: none;
}

.switch-slider {
  width: 32px;
  height: 18px;
  background: #334155;
  border-radius: 18px;
  position: relative;
  transition: background 0.2s ease;
}

.switch-slider::after {
  content: '';
  width: 14px;
  height: 14px;
  background: #ffffff;
  border-radius: 50%;
  position: absolute;
  top: 2px;
  left: 2px;
  transition: transform 0.2s ease;
}

.toggle-switch-lbl input:checked + .switch-slider {
  background: #10b981;
}

.toggle-switch-lbl input:checked + .switch-slider::after {
  transform: translateX(14px);
}

.lbl-txt {
  font-size: 12px;
  font-weight: 600;
  color: #ffffff;
}

.custom-range-slider {
  width: 100%;
  accent-color: #2563eb;
  cursor: pointer;
}

.quick-speed-pills {
  display: flex;
  gap: 3px;
}

.sp-pill {
  padding: 1px 6px;
  font-size: 10px;
  font-weight: 600;
  border-radius: 4px;
  border: 1px solid #334155;
  background: #0f172a;
  color: #94a3b8;
  cursor: pointer;
}

.sp-pill.active {
  background: #2563eb;
  border-color: #2563eb;
  color: #ffffff;
}

.scenario-select {
  height: 28px;
  padding: 0 6px;
  font-size: 11.5px;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 4px;
  color: #ffffff;
  outline: none;
}

.scenario-input-num {
  width: 60px;
  height: 26px;
  padding: 0 6px;
  font-size: 11.5px;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 4px;
  color: #ffffff;
  outline: none;
}

.btn-picker {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 26px;
  padding: 0 8px;
  font-size: 11px;
  font-weight: 600;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 4px;
  color: #ffffff;
  cursor: pointer;
}

.file-name-hint {
  font-size: 10.5px;
  color: #94a3b8;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 140px;
  white-space: nowrap;
}

.wm-pos-selector {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px;
  margin-top: 4px;
}

.pos-mini-btn {
  padding: 4px;
  font-size: 10px;
  font-weight: 600;
  background: #0f172a;
  border: 1px solid #334155;
  border-radius: 4px;
  color: #94a3b8;
  cursor: pointer;
}

.pos-mini-btn.active {
  background: #2563eb;
  border-color: #2563eb;
  color: #ffffff;
}

/* Center Panel (Player Monitor) */
.capcut-center-panel {
  flex: 1;
  background: #090d16;
  display: flex;
  flex-direction: column;
  user-select: none;
}

.player-monitor-header {
  height: 32px;
  padding: 0 12px;
  background: #0f172a;
  border-bottom: 1px solid #1e293b;
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.monitor-title {
  font-size: 11.5px;
  font-weight: 700;
  color: #cbd5e1;
}

.monitor-ratio-badge {
  font-size: 10.5px;
  font-weight: 700;
  background: #2563eb;
  color: #ffffff;
  padding: 2px 8px;
  border-radius: 4px;
}

.player-monitor-screen-wrap {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  overflow: hidden;
}

.player-monitor-frame {
  max-width: 90%;
  max-height: 90%;
  height: 100%;
  background: #000000;
  border: 1px solid #1e293b;
  border-radius: 8px;
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
}

.real-capcut-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
  transition: transform 0.2s ease, filter 0.2s ease;
}

.video-preview-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: #64748b;
  font-size: 13px;
  font-weight: 600;
  text-align: center;
  padding: 20px;
}

.sub-hint {
  font-size: 11px;
  font-weight: 400;
  max-width: 320px;
  line-height: 1.4;
  opacity: 0.7;
}

.overlay-badge-strip {
  position: absolute;
  top: 10px;
  left: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.badge {
  font-size: 10.5px;
  font-weight: 700;
  background: rgba(0, 0, 0, 0.75);
  color: #38bdf8;
  border: 1px solid rgba(56, 189, 248, 0.3);
  padding: 2px 8px;
  border-radius: 4px;
  backdrop-filter: blur(4px);
}

.player-transport-bar {
  height: 38px;
  background: #0f172a;
  border-top: 1px solid #1e293b;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
}

.timecode {
  font-size: 11px;
  font-family: monospace;
  color: #94a3b8;
}

.transport-controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.transport-btn {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  border: 1px solid #334155;
  background: #1e293b;
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.transport-btn.play {
  width: 32px;
  height: 32px;
  background: #2563eb;
  border-color: #2563eb;
}

.aspect-tag {
  font-size: 11px;
  font-weight: 600;
  color: #94a3b8;
}

/* Right Panel */
.capcut-right-panel {
  width: 240px;
  background: #0f172a;
  border-left: 1px solid #1e293b;
  display: flex;
  flex-direction: column;
  padding: 12px;
  gap: 12px;
  flex-shrink: 0;
}

.inspector-name-box {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.inspector-label {
  font-size: 10.5px;
  font-weight: 800;
  color: #94a3b8;
}

.inspector-input {
  width: 100%;
  height: 32px;
  padding: 0 10px;
  font-size: 12px;
  font-weight: 700;
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 6px;
  color: #ffffff;
  outline: none;
  box-sizing: border-box;
}

.inspector-summary-card {
  background: #1e293b;
  border: 1px solid #334155;
  border-radius: 8px;
  padding: 10px;
}

.summary-heading {
  margin: 0 0 8px;
  font-size: 11.5px;
  font-weight: 700;
  color: #38bdf8;
}

.summary-checklist {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.summary-checklist li {
  font-size: 11px;
  color: #64748b;
  display: flex;
  align-items: center;
  gap: 4px;
}

.summary-checklist li.active {
  color: #10b981;
  font-weight: 600;
}

.capcut-save-btn-large {
  margin-top: auto;
  height: 38px;
  font-size: 12.5px;
  font-weight: 800;
  background: #10b981;
  color: #ffffff;
  border: none;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  cursor: pointer;
}

/* Timeline */
.capcut-timeline-bar {
  height: 95px;
  background: #090d16;
  border-top: 1px solid #1e293b;
  display: flex;
  flex-direction: column;
  user-select: none;
  flex-shrink: 0;
}

.timeline-ruler {
  height: 18px;
  background: #0f172a;
  border-bottom: 1px solid #1e293b;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
}

.ruler-mark {
  font-size: 9.5px;
  font-family: monospace;
  color: #64748b;
}

.timeline-tracks {
  display: flex;
  flex-direction: column;
  gap: 3px;
  padding: 4px 8px;
}

.timeline-track {
  display: flex;
  align-items: center;
  height: 20px;
}

.track-head {
  width: 80px;
  font-size: 10px;
  font-weight: 700;
  color: #94a3b8;
}

.track-content {
  flex: 1;
  height: 100%;
}

.track-block {
  height: 100%;
  border-radius: 4px;
  display: flex;
  align-items: center;
  padding: 0 8px;
  font-size: 10px;
  font-weight: 600;
}

.video-block { background: #b91c1c; color: white; width: 90%; }
.audio-block { background: #0284c7; color: white; width: 80%; }
.sub-block { background: #d97706; color: white; width: 70%; }
</style>
