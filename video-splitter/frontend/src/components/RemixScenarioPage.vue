<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, reactive, computed, watch } from 'vue'
import { Plus, Trash2, Save, Copy, Music, Image as ImageIcon, FileText, Layers, SlidersHorizontal, Film, Check, Wand2, Palette, Volume2, Type, Sparkles, Play, Pause, RotateCcw, Video, Folder, X, Square, Loader2, ArrowUp, ArrowDown, GripVertical } from 'lucide-vue-next'
import { SelectImageFile, SelectAudioFile, SelectFolder, GetStreamURL } from '../../wailsjs/go/main/App'

const props = defineProps<{
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void
  activeVideoSrc?: string
  outputDir?: string
  isExporting?: boolean
  exportProgress?: { done: number; total: number }
  exportStatusText?: string
  etaText?: string
  videos?: string[]
  activeVideoIndex?: number
}>()

interface RemixScenario {
  id: string
  name: string
  edit: any
}

const emit = defineEmits<{
  (e: 'back'): void
  (e: 'export', scenarioId: string): void
  (e: 'stop-export'): void
  (e: 'update:outputDir', dir: string): void
  (e: 'selectVideo', index: number): void
  (e: 'selectExternalVideo'): void
}>()

interface RemixScenario {
  id: string
  name: string
  edit: any
}

interface DraftText {
  id: string
  content: string
  fontSize: number
  color: string
  x: number
  y: number
  startTime: number
  endTime: number
  bgBox: boolean
  selected?: boolean
}

type DragTarget = 'video-pan' | 'watermark' | 'subtitle' | 'card' | `text:${string}` | null

const STORAGE_KEY = 'remix_scenarios_list'

const scenarios = ref<RemixScenario[]>([])
const editingId = ref<string>('')
const draftName = ref('')
const activeCategoryTab = ref<'video' | 'color' | 'audio' | 'subtitle'>('video')

// Real Video Player State
const videoRef = ref<HTMLVideoElement | null>(null)
const bgVideoRef = ref<HTMLVideoElement | null>(null) // lớp nền blur cho aspect mode 'blur'
const isPlaying = ref(false)
const currentTimeStr = ref('00:00:00')
const durationTimeStr = ref('00:00:00')
// Giá trị số (giây) để tính vị trí playhead + tua. currentTimeStr/durationTimeStr là bản
// đã format để hiện; hai ref dưới là số thực dùng cho timeline động.
const currentSec = ref(0)
const durationSec = ref(0)

// % vị trí playhead trên timeline (0-100). Kẹp trong [0,100] tránh tràn khi chưa có duration.
const playheadPercent = computed(() => {
  if (durationSec.value <= 0) return 0
  return Math.min(100, Math.max(0, (currentSec.value / durationSec.value) * 100))
})

// Cách khung xử lý video theo aspect mode (khớp ffmpeg.go:428-449):
//   pad  → video co vừa khung + viền đen (object-fit contain, nền #000 sẵn có).
//   blur → video chính co giữa (contain) + lớp video nền phóng to làm mờ phủ full.
//   crop → video phủ đầy khung, phần thừa bị cắt (object-fit cover).
// Khi KHÔNG bật aspect → giữ contain như cũ.
const videoObjectFit = computed(() => {
  if (!draft.aspectEnabled) return 'contain'
  if (draft.aspectMode === 'crop') return 'cover'
  return 'contain' // pad + blur đều để video chính co vừa
})

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
  videoPanX: 0.5,
  videoPanY: 0.5,
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
  wmX: 0.97,
  wmY: 0.97,
  wmStartTime: 0,
  wmEndTime: 0,
  muteOriginal: false,
  musicPath: '',
  musicVolume: 0.3,
  musicLoop: false,
  subEnabled: false,
  subPath: '',
  subFontSize: 24,
  subFontColor: '#ffffff',
  subOutlineColor: '#000000',
  subMarginV: 40,
  subX: 0.5,
  subY: 0.9,
  subHasCustomPosition: false,
  subAutoGen: false,
  subSourceLang: 'auto',
  subTargetLang: '',
  textEnabled: false,
  texts: [] as DraftText[],
  randomText: false,
  cardEnabled: false,
  cardAboveText: false, // false = Text nằm trên Card (mặc định); true = Card nằm trên Text
  cardMode: 'preset' as 'preset' | 'color' | 'image',
  cardPreset: 'glass',
  cardImgPath: '',
  cardColor: '#0f172a',
  cardColor2: '#1e293b',
  cardOpacity: 0.85,
  cardX: 0.5,
  cardY: 0.75,
  cardWidth: 82,
  cardHeight: 22,
  cardBorderRadius: 12,
  cardStartTime: 0,
  cardEndTime: 0,
})

const selectedTextId = ref('')
const playerFrameRef = ref<HTMLElement | null>(null)
const dragTarget = ref<DragTarget>(null)
const isTimelineDragging = ref(false)
let timelineRAF = 0

const clamp01 = (v: number) => Math.max(0, Math.min(1, Number.isFinite(v) ? v : 0))

const coordExpr = (axis: 'x' | 'y', val: number, targetType: 'text' | 'watermark') => {
  const norm = clamp01(val)
  if (targetType === 'watermark') {
    if (axis === 'x') return norm > 0.5 ? 'main_w-overlay_w-20' : '20'
    return norm > 0.5 ? 'main_h-overlay_h-20' : '20'
  }
  if (axis === 'x') return '(w-text_w)/2'
  if (norm <= 0.2) return '0.08*h'
  if (norm >= 0.7) return 'h-text_h-0.08*h'
  return '(h-text_h)/2'
}

const parseNormalizedExpr = (expr: unknown, axis: 'x' | 'y', targetType: 'text' | 'watermark', fallback: number) => {
  const raw = String(expr || '').trim()
  if (!raw) return fallback
  const num = Number.parseFloat(raw)
  if (!Number.isNaN(num)) return clamp01(num)

  if (targetType === 'watermark') {
    if (axis === 'x') return raw.includes('main_w-overlay_w') ? 0.97 : 0.03
    if (axis === 'y') return raw.includes('main_h-overlay_h') ? 0.97 : 0.03
  } else {
    if (axis === 'x') return 0.5
    if (axis === 'y' && raw.includes('0.08*h')) return raw.includes('h-text_h') ? 0.9 : 0.08
    if (axis === 'y' && /^\d+(\.\d+)?$/.test(raw)) return 0.08
    if (axis === 'y' && raw.includes('h-text_h-')) return 0.9
  }
  return fallback
}

const selectedText = computed(() => draft.texts.find(t => t.id === selectedTextId.value) || null)
const visibleTexts = computed(() => {
  if (!draft.textEnabled) return []
  return draft.texts.filter(t => {
    if (t.selected === false) return false
    const end = t.endTime > 0 ? t.endTime : Number.POSITIVE_INFINITY
    return t.content.trim() && currentSec.value >= Math.max(0, t.startTime) && currentSec.value <= end
  })
})

const selectedTextsList = computed(() => {
  if (!draft.textEnabled) return []
  return draft.texts.filter(t => t.selected !== false)
})
const selectedTextsCount = computed(() => selectedTextsList.value.length)

const toggleTextSelected = (text: DraftText) => {
  if (selectedTextId.value !== text.id) {
    selectedTextId.value = text.id
    text.selected = true
  } else {
    text.selected = !(text.selected !== false)
  }
}

const addText = () => {
  draft.textEnabled = true
  const id = `text-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`
  draft.texts.push({ id, content: 'Nội dung chữ', fontSize: 48, color: '#ffffff', x: 0.5, y: 0.82, startTime: 0, endTime: 0, bgBox: true, selected: true })
  selectedTextId.value = id
}

const removeText = (id: string) => {
  const index = draft.texts.findIndex(t => t.id === id)
  if (index >= 0) draft.texts.splice(index, 1)
  if (selectedTextId.value === id) selectedTextId.value = draft.texts[0]?.id || ''
}

const duplicateText = (text: DraftText) => {
  draft.textEnabled = true
  const copy = { ...text, id: `text-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`, x: clamp01(text.x + 0.04), y: clamp01(text.y + 0.04), selected: true }
  draft.texts.push(copy)
  selectedTextId.value = copy.id
}

const moveTextUp = (text: DraftText) => {
  const idx = draft.texts.findIndex(t => t.id === text.id)
  if (idx < draft.texts.length - 1) {
    const temp = draft.texts[idx]
    draft.texts[idx] = draft.texts[idx + 1]
    draft.texts[idx + 1] = temp
  }
}

const moveTextDown = (text: DraftText) => {
  const idx = draft.texts.findIndex(t => t.id === text.id)
  if (idx > 0) {
    const temp = draft.texts[idx]
    draft.texts[idx] = draft.texts[idx - 1]
    draft.texts[idx - 1] = temp
  }
}

// Xử lý Nắm Kéo Thả (Drag & Drop) Hoán Đổi Vị Trí Track Lớp Hiển Thị Trên Timeline
const draggedTrackType = ref<string | null>(null)

const onTrackDragStart = (e: DragEvent, type: string) => {
  draggedTrackType.value = type
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', type)
  }
}

const onTrackDragOver = (e: DragEvent) => {
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
}

const onTrackDrop = (e: DragEvent, targetType: string) => {
  e.preventDefault()
  const sourceType = draggedTrackType.value || e.dataTransfer?.getData('text/plain')
  draggedTrackType.value = null
  if (!sourceType || sourceType === targetType) return

  // 1. Kéo hoán đổi giữa Nền Card và Text
  if ((sourceType === 'card' && targetType.startsWith('text')) || (sourceType.startsWith('text') && targetType === 'card')) {
    draft.cardAboveText = !draft.cardAboveText
    return
  }

  // 2. Kéo hoán đổi vị trí giữa các dòng Text với nhau
  if (sourceType.startsWith('text:') && targetType.startsWith('text:')) {
    const srcId = sourceType.replace('text:', '')
    const tgtId = targetType.replace('text:', '')
    const srcIdx = draft.texts.findIndex(t => t.id === srcId)
    const tgtIdx = draft.texts.findIndex(t => t.id === tgtId)
    if (srcIdx >= 0 && tgtIdx >= 0 && srcIdx !== tgtIdx) {
      const temp = draft.texts[srcIdx]
      draft.texts[srcIdx] = draft.texts[tgtIdx]
      draft.texts[tgtIdx] = temp
    }
  }
}

// Chỉ giữ các mẫu THỰC SỰ render được (có style CSS .card-preview-overlay.preset-* và
// khớp bộ render PNG ở exporter/card.go). Bỏ các mẫu hoa văn cũ chưa có style/render để
// preview luôn == video xuất.
const patternPresets = [
  { id: 'glass', name: 'Kính Mờ Glass' },
  { id: 'gradient-purple', name: 'Tím Neon Gradient' },
  { id: 'gold', name: 'Hoàng Gia Gold' },
  { id: 'ribbon', name: 'Băng Rôn Đỏ' },
  { id: 'vintage', name: 'Giấy Cổ Vintage' },
  { id: 'neon', name: 'Viền Neon Cyan' },
  { id: 'stripes', name: 'Sọc Chéo Stripes' },
]

const setWatermarkPosition = (x: number, y: number) => {
  draft.wmX = clamp01(x)
  draft.wmY = clamp01(y)
}

const setSubtitlePosition = (x: number, y: number) => {
  draft.subX = clamp01(x)
  draft.subY = clamp01(y)
  draft.subHasCustomPosition = true
}

const setTextPosition = (text: DraftText, x: number, y: number) => {
  text.x = clamp01(x)
  text.y = clamp01(y)
}


function load() {
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

function persist() {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(scenarios.value))
  window.dispatchEvent(new Event('remix_scenarios_updated'))
}

const cardStreamUrl = ref('')
watch(() => draft.cardImgPath, async (p) => {
  if (!p) { cardStreamUrl.value = ''; return }
  try { cardStreamUrl.value = await GetStreamURL(p) } catch (e) { cardStreamUrl.value = '' }
}, { immediate: true })

const pickCardImage = async () => {
  try {
    const res = await SelectImageFile()
    if (res) {
      draft.cardImgPath = res
      draft.cardMode = 'image'
      draft.cardEnabled = true
    }
  } catch (e) {
    console.error(e)
  }
}

const isCardVisible = computed(() => {
  if (!draft.cardEnabled) return false
  const st = Math.max(0, Number(draft.cardStartTime) || 0)
  const et = Number(draft.cardEndTime) > st ? Number(draft.cardEndTime) : Number.POSITIVE_INFINITY
  return currentSec.value >= st && currentSec.value <= et
})

const isWmVisible = computed(() => {
  if (!draft.wmEnabled || !draft.wmPath) return false
  const st = Math.max(0, Number(draft.wmStartTime) || 0)
  const et = Number(draft.wmEndTime) > st ? Number(draft.wmEndTime) : Number.POSITIVE_INFINITY
  return currentSec.value >= st && currentSec.value <= et
})

const cardStyle = computed(() => {
  const w = Math.min(100, Math.max(10, Number(draft.cardWidth) || 82))
  const h = Math.min(100, Math.max(5, Number(draft.cardHeight) || 22))
  const x = clamp01(Number(draft.cardX) ?? 0.5)
  const y = clamp01(Number(draft.cardY) ?? 0.8)
  const op = String(Math.min(1, Math.max(0.05, Number(draft.cardOpacity) || 0.85)))
  const r = Number(draft.cardBorderRadius) || 12

  const base: Record<string, string> = {
    position: 'absolute',
    width: `${w}%`,
    height: `${h}%`,
    left: `${x * (100 - Math.min(95, Math.max(10, w)))}%`,
    top: `${y * (100 - Math.min(95, Math.max(5, h)))}%`,
    borderRadius: `${r}px`,
    opacity: op,
    pointerEvents: 'auto',
    cursor: dragTarget.value === 'card' ? 'grabbing' : 'grab',
    touchAction: 'none',
    zIndex: draft.cardAboveText ? '15' : '3',
  }
  if (draft.cardMode === 'color') {
    if (draft.cardColor2 && draft.cardColor2 !== draft.cardColor) {
      base.background = `linear-gradient(135deg, ${draft.cardColor}, ${draft.cardColor2})`
    } else {
      base.backgroundColor = draft.cardColor || '#0f172a'
    }
  }
  return base
})

const cardBlockStyle = computed(() => {
  const total = Math.max(0.001, durationSec.value > 0 ? durationSec.value : 10)
  const start = Math.min(total, Math.max(0, draft.cardStartTime || 0))
  const end = draft.cardEndTime > start ? Math.min(total, draft.cardEndTime) : total
  return {
    left: `${(start / total) * 100}%`,
    width: `${Math.max(1.2, ((end - start) / total) * 100)}%`,
  }
})

const wmBlockStyle = computed(() => {
  const total = Math.max(0.001, durationSec.value > 0 ? durationSec.value : 10)
  const start = Math.min(total, Math.max(0, draft.wmStartTime || 0))
  const end = draft.wmEndTime > start ? Math.min(total, draft.wmEndTime) : total
  return {
    left: `${(start / total) * 100}%`,
    width: `${Math.max(1.2, ((end - start) / total) * 100)}%`,
  }
})

const textTrackRows = computed(() => {
  const texts = selectedTextsList.value
  if (!texts.length) return []
  
  const total = Math.max(0.001, durationSec.value > 0 ? durationSec.value : 10)
  const rows: DraftText[][] = []

  for (const t of texts) {
    const st = Math.max(0, t.startTime || 0)
    const et = t.endTime > st ? Math.min(total, t.endTime) : total
    
    let placed = false
    for (const row of rows) {
      const overlap = row.some(existing => {
        const est = Math.max(0, existing.startTime || 0)
        const eet = existing.endTime > est ? Math.min(total, existing.endTime) : total
        return !(et <= est || st >= eet)
      })
      if (!overlap) {
        row.push(t)
        placed = true
        break
      }
    }
    if (!placed) {
      rows.push([t])
    }
  }
  return rows
})

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

const formatTimelineMark = (sec: number) => {
  if (!Number.isFinite(sec) || sec < 0) return '00:00'
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  if (h > 0) return `${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}

const timelineMarks = computed(() => {
  const total = Math.max(0, durationSec.value)
  return Array.from({ length: 7 }, (_, index) => ({
    ratio: index / 6,
    label: formatTimelineMark(total * index / 6),
  }))
})

const textBlockStyle = (text: DraftText) => {
  const total = Math.max(0.001, durationSec.value)
  const start = Math.min(total, Math.max(0, text.startTime || 0))
  const end = text.endTime > start ? Math.min(total, text.endTime) : total
  return {
    left: `${start / total * 100}%`,
    width: `${Math.max(0.8, (end - start) / total * 100)}%`,
  }
}

const videoBlockStyle = computed(() => {
  const total = Math.max(0.001, durationSec.value)
  const start = Math.min(total, Math.max(0, draft.trimStart || 0))
  const end = draft.trimEnd > 0 ? Math.max(start, total - draft.trimEnd) : total
  return {
    position: 'absolute' as const,
    left: `${(start / total) * 100}%`,
    width: `${Math.max(0.5, ((end - start) / total) * 100)}%`,
    top: '0',
    bottom: '0',
  }
})

const onTimeUpdate = () => {
  if (videoRef.value) {
    currentSec.value = videoRef.value.currentTime
    currentTimeStr.value = formatSeconds(videoRef.value.currentTime)
  }
}

const onMetadataLoaded = () => {
  if (videoRef.value) {
    durationSec.value = videoRef.value.duration
    durationTimeStr.value = formatSeconds(videoRef.value.duration)
    videoRef.value.playbackRate = draft.speed
  }
}

// Tua video theo % vị trí click trên timeline. Đồng bộ lớp nền blur (nỗi có) theo cùng mốc.
// Bề rộng cột nhãn track (🎬 Video V1...) bên trái — vùng KHÔNG tua được. Phải khớp
// .track-head width trong CSS (84px) để playhead + click quy đổi cùng một mốc gốc.
const TRACK_HEAD_W = 84

const tracksRef = ref<HTMLElement | null>(null)
let pendingSeekTime: number | null = null
let seekRAF = 0

const performVideoSeek = (t: number) => {
  if (!videoRef.value) return
  pendingSeekTime = t

  if (seekRAF) return
  seekRAF = requestAnimationFrame(() => {
    seekRAF = 0
    if (pendingSeekTime === null || !videoRef.value) return
    const targetT = pendingSeekTime
    pendingSeekTime = null

    try {
      if ('fastSeek' in videoRef.value && typeof (videoRef.value as any).fastSeek === 'function') {
        (videoRef.value as any).fastSeek(targetT)
      } else {
        videoRef.value.currentTime = targetT
      }
    } catch (e) {
      try { videoRef.value.currentTime = targetT } catch (err) {}
    }

    if (bgVideoRef.value) {
      try { bgVideoRef.value.currentTime = targetT } catch (e) {}
    }
  })
}

const applyTimelineSeek = (clientX: number) => {
  if (!videoRef.value || durationSec.value <= 0 || !tracksRef.value) return
  const rect = tracksRef.value.getBoundingClientRect()
  const usable = rect.width - TRACK_HEAD_W
  if (usable <= 0) return
  const ratio = clamp01((clientX - rect.left - TRACK_HEAD_W) / usable)
  const t = ratio * durationSec.value

  currentSec.value = t
  currentTimeStr.value = formatSeconds(t)

  performVideoSeek(t)
}

const seekFromClick = (e: MouseEvent) => {
  applyTimelineSeek(e.clientX)
}

const onTimelinePointerDown = (e: PointerEvent) => {
  if (durationSec.value <= 0) return
  isTimelineDragging.value = true
  
  // Tạm dừng video trong lúc kéo đểseek mượt mà, kéo đến đâu khung hình hiển thị đến đó lập tức
  if (videoRef.value && !videoRef.value.paused) {
    videoRef.value.pause()
  }
  if (bgVideoRef.value && !bgVideoRef.value.paused) {
    bgVideoRef.value.pause()
  }
  isPlaying.value = false

  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
  applyTimelineSeek(e.clientX)
}

const onTimelinePointerMove = (e: PointerEvent) => {
  if (!isTimelineDragging.value) return
  // Seek trực tiếp realtime ngay theo tọa độ con trỏ chuột
  applyTimelineSeek(e.clientX)
}

const finishTimelineDrag = (e: PointerEvent) => {
  if (!isTimelineDragging.value) return
  applyTimelineSeek(e.clientX)
  isTimelineDragging.value = false
  const el = e.currentTarget as HTMLElement
  if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId)

  // Tự động phát tiếp ngay khi thả chuột
  if (videoRef.value) {
    void videoRef.value.play().then(() => {
      if (bgVideoRef.value) void bgVideoRef.value.play().catch(() => {})
      isPlaying.value = true
    }).catch(() => {})
  }
}

const cancelTimelineDrag = () => {
  isTimelineDragging.value = false
}

let dragPointerId = -1
let dragOffsetX = 0
let dragOffsetY = 0
let dragElementW = 0
let dragElementH = 0

let initialPanX = 0.5
let initialPanY = 0.5
let panStartX = 0
let panStartY = 0

const beginVideoPanDrag = (e: PointerEvent) => {
  const targetEl = e.target as HTMLElement
  if (targetEl && targetEl.closest('.draggable-overlay')) return
  selectedTextId.value = '' // Bỏ chọn text ngay khi nhấp vào video
  if (!playerFrameRef.value) return
  e.preventDefault()
  e.stopPropagation()

  const frameRect = playerFrameRef.value.getBoundingClientRect()
  dragTarget.value = 'video-pan'
  dragPointerId = e.pointerId
  panStartX = e.clientX
  panStartY = e.clientY
  initialPanX = draft.videoPanX ?? 0.5
  initialPanY = draft.videoPanY ?? 0.5
  dragElementW = frameRect.width
  dragElementH = frameRect.height

  const el = e.currentTarget as HTMLElement
  el.setPointerCapture(e.pointerId)
}

const beginOverlayDrag = (target: Exclude<DragTarget, null>, e: PointerEvent) => {
  if (!playerFrameRef.value) return
  e.preventDefault()
  e.stopPropagation()
  const el = e.currentTarget as HTMLElement
  const rect = el.getBoundingClientRect()
  dragTarget.value = target
  dragPointerId = e.pointerId
  dragOffsetX = e.clientX - rect.left
  dragOffsetY = e.clientY - rect.top
  dragElementW = rect.width
  dragElementH = rect.height
  el.setPointerCapture(e.pointerId)
  if (target.startsWith('text:')) selectedTextId.value = target.slice(5)
}

const updateOverlayDrag = (e: PointerEvent) => {
  if (!dragTarget.value || e.pointerId !== dragPointerId || !playerFrameRef.value) return
  e.preventDefault()
  e.stopPropagation()

  if (dragTarget.value === 'video-pan') {
    // Tự động bật aspect & zoom nhẹ nếu chưa bật để thao tác kéo lọt khung có hiệu lực tức thì
    if (!draft.aspectEnabled) draft.aspectEnabled = true
    if (!draft.zoomEnabled) {
      draft.zoomEnabled = true
      if (draft.zoomFactor < 1.05) draft.zoomFactor = 1.05
    }

    const deltaX = e.clientX - panStartX
    const deltaY = e.clientY - panStartY
    // Hệ số độ nhạy chuột (0.35) giúp kéo trượt chuẩn xác từng mm, không bị giật nhanh
    const sensitivity = 0.35
    const normDx = (deltaX / Math.max(1, dragElementW)) * sensitivity
    const normDy = (deltaY / Math.max(1, dragElementH)) * sensitivity
    draft.videoPanX = clamp01(initialPanX - normDx)
    draft.videoPanY = clamp01(initialPanY - normDy)
    return
  }
  const frame = playerFrameRef.value.getBoundingClientRect()
  const left = e.clientX - frame.left - dragOffsetX
  const top = e.clientY - frame.top - dragOffsetY
  if (dragTarget.value === 'watermark' || dragTarget.value === 'card' || dragTarget.value.startsWith('text:')) {
    const usableW = Math.max(1, frame.width - dragElementW)
    const usableH = Math.max(1, frame.height - dragElementH)
    const x = left / usableW
    const y = top / usableH
    if (dragTarget.value === 'watermark') setWatermarkPosition(x, y)
    else if (dragTarget.value === 'card') {
      draft.cardX = clamp01(x)
      draft.cardY = clamp01(y)
    } else {
      const text = draft.texts.find(t => t.id === dragTarget.value?.slice(5))
      if (text) setTextPosition(text, x, y)
    }
  } else {
    // Phụ đề dùng tâm làm neo, trùng với ASS \pos(x,y).
    const halfW = dragElementW / 2
    const halfH = dragElementH / 2
    const minX = halfW / Math.max(1, frame.width)
    const maxX = 1 - minX
    const minY = halfH / Math.max(1, frame.height)
    const maxY = 1 - minY
    const x = Math.min(maxX, Math.max(minX, (left + halfW) / Math.max(1, frame.width)))
    const y = Math.min(maxY, Math.max(minY, (top + halfH) / Math.max(1, frame.height)))
    setSubtitlePosition(x, y)
  }
}

const finishOverlayDrag = (e: PointerEvent) => {
  if (!dragTarget.value || e.pointerId !== dragPointerId) return
  updateOverlayDrag(e)
  const el = e.currentTarget as HTMLElement
  if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId)
  dragTarget.value = null
  dragPointerId = -1
}

const selectCardPreset = (id: string) => {
  draft.cardPreset = id
  draft.cardEnabled = true
  draft.cardMode = 'preset'

  if (draft.cardEndTime <= draft.cardStartTime) {
    draft.cardEndTime = durationSec.value > 0 ? durationSec.value : 0
  }

  const end = draft.cardEndTime > 0 ? draft.cardEndTime : Number.POSITIVE_INFINITY
  if (currentSec.value < (draft.cardStartTime || 0) || currentSec.value > end) {
    currentSec.value = draft.cardStartTime || 0
    currentTimeStr.value = formatSeconds(currentSec.value)
    performVideoSeek(currentSec.value)
  }
}

const setCardColor = (c1: string, c2: string) => {
  draft.cardColor = c1
  draft.cardColor2 = c2
  draft.cardEnabled = true
  draft.cardMode = 'color'

  const end = draft.cardEndTime > 0 ? draft.cardEndTime : Number.POSITIVE_INFINITY
  if (currentSec.value < (draft.cardStartTime || 0) || currentSec.value > end) {
    currentSec.value = draft.cardStartTime || 0
    currentTimeStr.value = formatSeconds(currentSec.value)
    performVideoSeek(currentSec.value)
  }
}

let resizeSession: {
  target: string
  handle: 'nw' | 'ne' | 'sw' | 'se' | 'n' | 's' | 'w' | 'e'
  pointerId: number
  startX: number
  startY: number
  initialCardWidth: number
  initialCardHeight: number
  initialCardX: number
  initialCardY: number
  initialWmScale: number
  initialFontSize: number
  initialRectWidth: number
  initialRectHeight: number
} | null = null

const beginResizeHandle = (target: string, handle: 'nw' | 'ne' | 'sw' | 'se' | 'n' | 's' | 'w' | 'e', e: PointerEvent) => {
  if (!playerFrameRef.value) return
  e.preventDefault()
  e.stopPropagation()

  const targetEl = (e.currentTarget as HTMLElement).closest('.draggable-overlay') as HTMLElement
  const rect = targetEl ? targetEl.getBoundingClientRect() : { width: 100, height: 100 }
  const text = target.startsWith('text:') ? draft.texts.find(t => t.id === target.slice(5)) : null

  resizeSession = {
    target,
    handle,
    pointerId: e.pointerId,
    startX: e.clientX,
    startY: e.clientY,
    initialCardWidth: draft.cardWidth,
    initialCardHeight: draft.cardHeight,
    initialCardX: draft.cardX,
    initialCardY: draft.cardY,
    initialWmScale: draft.wmScale || 1.0,
    initialFontSize: text ? text.fontSize : (target === 'subtitle' ? draft.subFontSize : 24),
    initialRectWidth: rect.width,
    initialRectHeight: rect.height,
  }

  if (target.startsWith('text:')) selectedTextId.value = target.slice(5)
  ;(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId)
}

const updateResizeHandle = (e: PointerEvent) => {
  if (!resizeSession || e.pointerId !== resizeSession.pointerId || !playerFrameRef.value) return
  e.preventDefault()
  e.stopPropagation()

  const frameRect = playerFrameRef.value.getBoundingClientRect()
  if (frameRect.width <= 0 || frameRect.height <= 0) return

  const deltaX = e.clientX - resizeSession.startX
  const deltaY = e.clientY - resizeSession.startY

  const deltaPercentW = (deltaX / frameRect.width) * 100
  const deltaPercentH = (deltaY / frameRect.height) * 100

  const handle = resizeSession.handle
  const target = resizeSession.target

  if (target === 'card') {
    let newWidth = resizeSession.initialCardWidth
    let newHeight = resizeSession.initialCardHeight
    let newX = resizeSession.initialCardX
    let newY = resizeSession.initialCardY

    // X axis
    if (handle.includes('e')) {
      newWidth = Math.min(100, Math.max(5, resizeSession.initialCardWidth + deltaPercentW))
    } else if (handle.includes('w')) {
      const wChange = -deltaPercentW
      newWidth = Math.min(100, Math.max(5, resizeSession.initialCardWidth + wChange))
      const xShift = (deltaX / frameRect.width)
      newX = clamp01(resizeSession.initialCardX + xShift)
    }

    // Y axis
    if (handle.includes('s')) {
      newHeight = Math.min(100, Math.max(5, resizeSession.initialCardHeight + deltaPercentH))
    } else if (handle.includes('n')) {
      const hChange = -deltaPercentH
      newHeight = Math.min(100, Math.max(5, resizeSession.initialCardHeight + hChange))
      const yShift = (deltaY / frameRect.height)
      newY = clamp01(resizeSession.initialCardY + yShift)
    }

    draft.cardWidth = Math.round(newWidth * 10) / 10
    draft.cardHeight = Math.round(newHeight * 10) / 10
    draft.cardX = Math.round(newX * 1000) / 1000
    draft.cardY = Math.round(newY * 1000) / 1000
  } else if (target === 'watermark') {
    const dist = handle.includes('w') || handle.includes('n') ? -deltaX : deltaX
    const scaleFactor = 1 + (dist / Math.max(1, resizeSession.initialRectWidth))
    const newScale = Math.min(3.0, Math.max(0.1, resizeSession.initialWmScale * scaleFactor))
    draft.wmScale = Math.round(newScale * 100) / 100
  } else if (target.startsWith('text:')) {
    const text = draft.texts.find(t => t.id === target.slice(5))
    if (text) {
      const dist = handle.includes('w') || handle.includes('n') ? -deltaX : deltaX
      const scaleFactor = 1 + (dist / Math.max(1, resizeSession.initialRectWidth))
      const newFontSize = Math.min(140, Math.max(10, resizeSession.initialFontSize * scaleFactor))
      text.fontSize = Math.round(newFontSize)
    }
  } else if (target === 'subtitle') {
    const dist = handle.includes('w') || handle.includes('n') ? -deltaX : deltaX
    const scaleFactor = 1 + (dist / Math.max(1, resizeSession.initialRectWidth))
    const newFontSize = Math.min(100, Math.max(10, resizeSession.initialFontSize * scaleFactor))
    draft.subFontSize = Math.round(newFontSize)
  }
}

const finishResizeHandle = (e: PointerEvent) => {
  if (!resizeSession || e.pointerId !== resizeSession.pointerId) return
  updateResizeHandle(e)
  const el = e.currentTarget as HTMLElement
  if (el.hasPointerCapture(e.pointerId)) el.releasePointerCapture(e.pointerId)
  resizeSession = null
}

const cancelOverlayDrag = () => {
  dragTarget.value = null
  dragPointerId = -1
}

// Giữ Ctrl + Cuộn chuột để phóng to / thu nhỏ phần tử overlay (Watermark, Subtitle, Text)
const onOverlayWheel = (e: WheelEvent, targetType: string) => {
  if (!e.ctrlKey && !e.metaKey) return
  e.preventDefault()
  e.stopPropagation()

  const isZoomIn = e.deltaY < 0

  if (targetType === 'watermark') {
    const step = 0.02
    const nextScale = draft.wmScale + (isZoomIn ? step : -step)
    draft.wmScale = Math.min(0.8, Math.max(0.03, Number(nextScale.toFixed(2))))
  } else if (targetType === 'subtitle') {
    const step = 2
    const nextSize = draft.subFontSize + (isZoomIn ? step : -step)
    draft.subFontSize = Math.min(160, Math.max(10, nextSize))
  } else if (targetType === 'card') {
    const step = 2
    draft.cardWidth = Math.min(98, Math.max(15, draft.cardWidth + (isZoomIn ? step : -step)))
    draft.cardHeight = Math.min(80, Math.max(8, draft.cardHeight + (isZoomIn ? step : -step)))
  } else if (targetType.startsWith('text:')) {
    const textId = targetType.replace('text:', '')
    const targetText = draft.texts.find((t: DraftText) => t.id === textId)
    if (targetText) {
      selectedTextId.value = targetText.id
      const step = 2
      const nextSize = targetText.fontSize + (isZoomIn ? step : -step)
      targetText.fontSize = Math.min(160, Math.max(8, nextSize))
    }
  }
}

const onFrameWheel = (e: WheelEvent) => {
  if (!e.ctrlKey && !e.metaKey) return
  e.preventDefault()

  if (dragTarget.value) {
    onOverlayWheel(e, dragTarget.value)
    return
  }
  const targetEl = e.target as HTMLElement | null
  if (targetEl && targetEl.closest('.text-preview-overlay') && selectedTextId.value) {
    onOverlayWheel(e, `text:${selectedTextId.value}`)
    return
  }
  if (targetEl && targetEl.closest('.wm-preview-overlay') && draft.wmEnabled) {
    onOverlayWheel(e, 'watermark')
    return
  }
}

// Timeline Card Block Dragging & Resizing Logic
interface TimelineBlockDragSession {
  type: 'card' | 'watermark' | 'text'
  textObj?: DraftText
  action: 'move' | 'resize-left' | 'resize-right'
  startPointerX: number
  initialStartTime: number
  initialEndTime: number
  totalDuration: number
  pointerId: number
  targetEl: HTMLElement | null
}

const blockDragSession = ref<TimelineBlockDragSession | null>(null)

const onBlockPointerDown = (
  type: 'card' | 'watermark' | 'text',
  action: 'move' | 'resize-left' | 'resize-right',
  e: PointerEvent,
  textObj?: DraftText
) => {
  e.stopPropagation()
  e.preventDefault()

  const el = e.currentTarget as HTMLElement
  el.setPointerCapture(e.pointerId)

  const totalDuration = durationSec.value > 0 ? durationSec.value : 10
  let initialStartTime = 0
  let initialEndTime = totalDuration

  if (type === 'card') {
    initialStartTime = draft.cardStartTime || 0
    initialEndTime = draft.cardEndTime > 0 ? draft.cardEndTime : totalDuration
  } else if (type === 'watermark') {
    initialStartTime = draft.wmStartTime || 0
    initialEndTime = draft.wmEndTime > 0 ? draft.wmEndTime : totalDuration
  } else if (type === 'text' && textObj) {
    selectedTextId.value = textObj.id
    initialStartTime = textObj.startTime || 0
    initialEndTime = textObj.endTime > 0 ? textObj.endTime : totalDuration
  }

  blockDragSession.value = {
    type,
    textObj,
    action,
    startPointerX: e.clientX,
    initialStartTime,
    initialEndTime,
    totalDuration,
    pointerId: e.pointerId,
    targetEl: el,
  }
}

const onBlockPointerMove = (e: PointerEvent) => {
  if (!blockDragSession.value) return
  const session = blockDragSession.value
  if (e.pointerId !== session.pointerId) return

  const usableWidth = tracksRef.value ? Math.max(1, tracksRef.value.getBoundingClientRect().width - TRACK_HEAD_W) : 500
  const dx = e.clientX - session.startPointerX
  const dt = (dx / usableWidth) * session.totalDuration

  let newStart = session.initialStartTime
  let newEnd = session.initialEndTime

  if (session.action === 'resize-left') {
    newStart = Math.max(0, Math.min((session.initialEndTime > 0 ? session.initialEndTime : session.totalDuration) - 0.1, session.initialStartTime + dt))
    newStart = Math.round(newStart * 10) / 10
    newEnd = session.initialEndTime
  } else if (session.action === 'resize-right') {
    newEnd = Math.max(session.initialStartTime + 0.1, Math.min(session.totalDuration, session.initialEndTime + dt))
    if (newEnd >= session.totalDuration - 0.05) {
      newEnd = 0
    } else {
      newEnd = Math.round(newEnd * 10) / 10
    }
    newStart = session.initialStartTime
  } else if (session.action === 'move') {
    const blockLen = session.initialEndTime - session.initialStartTime
    newStart = Math.max(0, Math.min(session.totalDuration - blockLen, session.initialStartTime + dt))
    newStart = Math.round(newStart * 10) / 10
    if (session.initialEndTime > 0 && session.initialEndTime < session.totalDuration) {
      newEnd = Math.round((newStart + blockLen) * 10) / 10
    } else {
      newEnd = 0
    }
  }

  if (session.type === 'card') {
    draft.cardStartTime = newStart
    draft.cardEndTime = newEnd
  } else if (session.type === 'watermark') {
    draft.wmStartTime = newStart
    draft.wmEndTime = newEnd
  } else if (session.type === 'text' && session.textObj) {
    session.textObj.startTime = newStart
    session.textObj.endTime = newEnd
  }

  const seekTime = session.action === 'resize-right' && newEnd > 0 ? newEnd : newStart
  currentSec.value = seekTime
  currentTimeStr.value = formatSeconds(seekTime)
  performVideoSeek(seekTime)
}

const onBlockPointerUp = (e: PointerEvent) => {
  if (!blockDragSession.value) return
  const session = blockDragSession.value
  if (session.targetEl && session.targetEl.hasPointerCapture(e.pointerId)) {
    session.targetEl.releasePointerCapture(e.pointerId)
  }
  blockDragSession.value = null
}

const onCardBlockPointerDown = (action: 'move' | 'resize-left' | 'resize-right', e: PointerEvent) => onBlockPointerDown('card', action, e)
const onCardBlockPointerMove = (e: PointerEvent) => onBlockPointerMove(e)
const onCardBlockPointerUp = (e: PointerEvent) => onBlockPointerUp(e)

const onGlobalKeyDown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') {
    selectedTextId.value = ''
  }
}

const onFrameClick = (e: MouseEvent) => {
  const target = e.target as HTMLElement | null
  if (target && !target.closest('.draggable-overlay')) {
    selectedTextId.value = ''
  }
}

onMounted(() => {
  window.addEventListener('keydown', onGlobalKeyDown)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKeyDown)
  if (timelineRAF) cancelAnimationFrame(timelineRAF)
})

watch(() => draft.speed, (newSpeed) => {
  if (videoRef.value) {
    videoRef.value.playbackRate = newSpeed
  }
})

// Màu preview khớp ffmpeg (ffmpeg.go colorPreset/colorEq): eq=brightness:contrast:saturation
// + preset. warm/cool = colorbalance (dịch kênh RGB) chứ KHÔNG phải sepia/hue-rotate như bản cũ
// → dùng sepia nhẹ + hue-rotate hướng đúng để xấp xỉ cảm giác dịch kênh; vintage = curves preset.
// CSS không có colorbalance/curves nên đây là xấp xỉ thị giác gần nhất, KHÔNG khớp pixel tuyệt đối.
const colorFilterStyle = computed(() => {
  if (!draft.colorEnabled) return 'none'
  const b = 1 + draft.colorBrightness
  const c = 1 + draft.colorContrast
  const s = draft.colorSaturation
  // warm ffmpeg: rs=+0.10 gs=+0.02 bs=-0.08 → đỏ/vàng tăng, xanh dương giảm → ngả ấm.
  if (draft.colorPreset === 'warm') return `brightness(${b}) contrast(${c}) saturate(${s * 1.05}) sepia(0.18) hue-rotate(-8deg)`
  // cool ffmpeg: rs=-0.08 bs=+0.12 → xanh dương tăng, đỏ giảm → ngả lạnh.
  if (draft.colorPreset === 'cool') return `brightness(${b}) contrast(${c}) saturate(${s * 1.05}) sepia(0.12) hue-rotate(165deg)`
  // vivid ffmpeg: eq=saturation=1.4:contrast=1.1
  if (draft.colorPreset === 'vivid') return `brightness(${b}) contrast(${c * 1.1}) saturate(${s * 1.4})`
  // bw ffmpeg: hue=s=0 (khử toàn bộ bão hòa)
  if (draft.colorPreset === 'bw') return `brightness(${b}) contrast(${c}) grayscale(1)`
  // vintage ffmpeg: curves=preset=vintage → giảm tương phản, ngả vàng-lục, nâng vùng tối.
  if (draft.colorPreset === 'vintage') return `brightness(${b * 1.05}) contrast(${c * 0.9}) saturate(${s * 0.85}) sepia(0.35)`
  return `brightness(${b}) contrast(${c}) saturate(${s})`
})

// Transform preview khớp ffmpeg: mirror(hflip) + zoom(scale) + rotate + crop.
// Điểm mấu chốt (bản cũ thiếu): ffmpeg crop cắt p% MỖI MÉP rồi scale lại full khung
//   (ffmpeg.go:388-394) → tương đương phóng to 1/(1-2p) về tâm. rotate/zoom cũng phải
//   nhân thêm scale-bù nếu không preview sẽ LÒI VIỀN ĐEN mà output thật thì không.
const videoTransform = computed(() => {
  const parts: string[] = []
  if (draft.hflip) parts.push('scaleX(-1)')
  // Gộp mọi hệ số phóng về 1 scale duy nhất để không lệch giữa các bước.
  let scale = 1
  if (draft.zoomEnabled && draft.zoomFactor > 0) scale *= draft.zoomFactor
  if (draft.cropEnabled) {
    const p = Math.min(0.45, Math.max(0, draft.cropPercent))
    scale *= 1 / (1 - 2 * p) // crop-bù: cắt p mỗi mép rồi phóng lại full
  }
  if (draft.rotateEnabled && draft.rotateDegrees) {
    // scale-bù khi xoay để góc video không để lộ nền: 1/(cos+sin*ratio) xấp xỉ bằng
    // cách nhân thêm theo |góc|. Dùng công thức đủ dùng cho góc nhỏ (±vài độ).
    const rad = Math.abs(draft.rotateDegrees) * Math.PI / 180
    scale *= (Math.abs(Math.cos(rad)) + Math.abs(Math.sin(rad)) * (16 / 9))
  }
  if (scale !== 1) parts.push(`scale(${scale.toFixed(4)})`)
  if (draft.rotateEnabled && draft.rotateDegrees) parts.push(`rotate(${draft.rotateDegrees}deg)`)
  return parts.join(' ')
})

const videoStyle = computed(() => {
  const px = Math.min(100, Math.max(0, (draft.videoPanX ?? 0.5) * 100))
  const py = Math.min(100, Math.max(0, (draft.videoPanY ?? 0.5) * 100))

  return {
    transform: videoTransform.value,
    transformOrigin: `${px}% ${py}%`,
    filter: colorFilterStyle.value,
    objectFit: videoObjectFit.value as any,
    objectPosition: `${px}% ${py}%`,
    cursor: dragTarget.value === 'video-pan' ? 'grabbing' : 'grab',
  }
})

// Logo watermark là file cục bộ → webview không load path trực tiếp được, phải qua
// GetStreamURL (cùng cơ chế với video nguồn). Watch wmPath để nạp lại URL khi đổi ảnh.
const wmStreamUrl = ref('')
watch(() => draft.wmPath, async (p) => {
  if (!p) { wmStreamUrl.value = ''; return }
  try { wmStreamUrl.value = await GetStreamURL(p) } catch (e) { wmStreamUrl.value = '' }
}, { immediate: true })

// Logo/text dùng cùng hệ tọa độ chuẩn hóa với biểu thức FFmpeg đã lưu.
const wmStyle = computed(() => ({
  position: 'absolute',
  width: `${Math.round(Math.min(0.8, Math.max(0.03, draft.wmScale)) * 100)}%`,
  opacity: String(Math.min(1, Math.max(0.05, draft.wmOpacity))),
  left: `${clamp01(draft.wmX) * (100 - Math.min(80, Math.max(3, draft.wmScale * 100)))}%`,
  top: `${clamp01(draft.wmY) * 100}%`,
  transform: `translateY(-${clamp01(draft.wmY) * 100}%)`,
  pointerEvents: 'auto',
  cursor: dragTarget.value === 'watermark' ? 'grabbing' : 'grab',
  touchAction: 'none',
  zIndex: '5',
} as Record<string, string>))

const subStyle = computed(() => ({
  position: 'absolute',
  left: `${clamp01(draft.subX) * 100}%`,
  top: `${clamp01(draft.subY) * 100}%`,
  transform: 'translate(-50%, -50%)',
  fontSize: `${Math.max(10, draft.subFontSize)}px`,
  color: draft.subFontColor || '#ffffff',
  WebkitTextStroke: `1px ${draft.subOutlineColor || '#000000'}`,
  textShadow: `0 1px 3px ${draft.subOutlineColor || '#000000'}`,
  fontWeight: '700',
  whiteSpace: 'nowrap',
  pointerEvents: 'auto',
  cursor: dragTarget.value === 'subtitle' ? 'grabbing' : 'grab',
  touchAction: 'none',
  zIndex: '6',
  maxWidth: '92%',
  textAlign: 'center',
} as Record<string, string>))

const textStyle = (text: DraftText) => ({
  position: 'absolute',
  left: `${clamp01(text.x) * 100}%`,
  top: `${clamp01(text.y) * 100}%`,
  transform: 'translate(-50%, -50%)',
  color: text.color || '#ffffff',
  fontSize: `${Math.max(8, text.fontSize)}px`,
  fontWeight: '700',
  lineHeight: '1.2',
  whiteSpace: 'pre-wrap',
  textAlign: 'center',
  maxWidth: '90%',
  padding: text.bgBox ? '5px 9px' : '2px',
  background: text.bgBox ? 'rgba(0, 0, 0, 0.5)' : 'transparent',
  border: selectedTextId.value === text.id ? '1px dashed #38bdf8' : '1px solid transparent',
  borderRadius: '4px',
  cursor: dragTarget.value === `text:${text.id}` ? 'grabbing' : 'grab',
  touchAction: 'none',
  zIndex: String(10 + Math.max(0, draft.texts.findIndex(t => t.id === text.id))),
} as Record<string, string>)

const cssToASSColor = (value: string, fallback: string) => {
  const match = String(value || '').trim().match(/^#?([0-9a-f]{6})$/i)
  if (!match) return fallback
  const rgb = match[1]
  return `&H${rgb.slice(4, 6)}${rgb.slice(2, 4)}${rgb.slice(0, 2)}`
}

const assToCSSColor = (value: unknown, fallback: string) => {
  const match = String(value || '').match(/&H(?:[0-9a-f]{2})?([0-9a-f]{6})/i)
  if (!match) return fallback
  const bgr = match[1]
  return `#${bgr.slice(4, 6)}${bgr.slice(2, 4)}${bgr.slice(0, 2)}`
}

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
    wmEnabled: false, wmPath: '', wmScale: 0.2, wmOpacity: 1.0, wmX: 0.97, wmY: 0.97, wmStartTime: 0, wmEndTime: 0,
    muteOriginal: false, musicPath: '', musicVolume: 0.3, musicLoop: false,
    subEnabled: false, subPath: '', subFontSize: 24, subFontColor: '#ffffff', subOutlineColor: '#000000', subMarginV: 40,
    subX: 0.5, subY: 0.9, subHasCustomPosition: false,
    subAutoGen: false, subSourceLang: 'auto', subTargetLang: '', textEnabled: false, texts: [],
    randomText: false, cardEnabled: false, cardAboveText: false, cardMode: 'preset', cardPreset: 'glass', cardImgPath: '',
    cardColor: '#0f172a', cardColor2: '#1e293b', cardOpacity: 0.85, cardX: 0.5, cardY: 0.75,
    cardWidth: 82, cardHeight: 22, cardBorderRadius: 12, cardStartTime: 0, cardEndTime: 0,
  })
  selectedTextId.value = ''
}

const draftToEdit = (): any => ({
  hflip: draft.hflip,
  speed: draft.speed,
  aspect: { enabled: draft.aspectEnabled, ratio: draft.aspectRatio, mode: draft.aspectMode, panX: clamp01(draft.videoPanX), panY: clamp01(draft.videoPanY) },
  color: { enabled: draft.colorEnabled, preset: draft.colorPreset, brightness: draft.colorBrightness, contrast: draft.colorContrast, saturation: draft.colorSaturation },
  zoomPan: { enabled: draft.zoomEnabled, zoom: draft.zoomFactor, dir: draft.zoomDir },
  crop: { enabled: draft.cropEnabled, percent: draft.cropPercent },
  rotate: { enabled: draft.rotateEnabled, degrees: draft.rotateDegrees },
  noise: { enabled: draft.noiseEnabled, strength: draft.noiseStrength },
  trimStart: draft.trimStart,
  trimEnd: draft.trimEnd,
  pitch: draft.pitch,
  stripMeta: draft.stripMeta,
  randomText: draft.randomText,
  textEnabled: draft.textEnabled,
  texts: draft.texts.map(t => ({
    content: t.content,
    fontSize: Math.round(Math.min(300, Math.max(8, t.fontSize))),
    color: t.color || '#ffffff',
    x: coordExpr('x', t.x, 'text'),
    y: coordExpr('y', t.y, 'text'),
    startTime: Math.max(0, t.startTime || 0),
    endTime: t.endTime > 0 ? Math.max(t.startTime || 0, t.endTime) : 0,
    bgBox: !!t.bgBox,
    selected: t.selected !== false,
  })),
  card: {
    enabled: draft.cardEnabled,
    mode: draft.cardMode,
    preset: draft.cardPreset,
    imgPath: draft.cardImgPath,
    color: draft.cardColor,
    color2: draft.cardColor2,
    opacity: draft.cardOpacity,
    x: String(clamp01(draft.cardX)),
    y: String(clamp01(draft.cardY)),
    width: draft.cardWidth,
    height: draft.cardHeight,
    borderRadius: draft.cardBorderRadius,
    startTime: Math.max(0, draft.cardStartTime || 0),
    endTime: draft.cardEndTime > 0 ? Math.max(draft.cardStartTime || 0, draft.cardEndTime) : 0,
    cardAboveText: draft.cardAboveText,
  },
  watermark: {
    enabled: draft.wmEnabled,
    imgPath: draft.wmPath,
    x: coordExpr('x', draft.wmX, 'watermark'),
    y: coordExpr('y', draft.wmY, 'watermark'),
    scale: draft.wmScale,
    opacity: draft.wmOpacity,
    startTime: Math.max(0, draft.wmStartTime || 0),
    endTime: draft.wmEndTime > 0 ? Math.max(draft.wmStartTime || 0, draft.wmEndTime) : 0,
  },
  audio: { mute: draft.muteOriginal, musicPath: draft.musicPath, musicVolume: draft.musicVolume, musicLoop: draft.musicLoop, musicTracks: draft.musicPath ? [draft.musicPath] : [], volume: 1, fadeIn: 0, fadeOut: 0 },
  subtitle: {
    enabled: draft.subEnabled,
    path: draft.subPath,
    fontSize: draft.subFontSize,
    fontColor: cssToASSColor(draft.subFontColor, '&Hffffff'),
    outlineCol: cssToASSColor(draft.subOutlineColor, '&H000000'),
    marginV: draft.subMarginV,
    positionX: clamp01(draft.subX),
    positionY: clamp01(draft.subY),
    hasCustomPosition: draft.subHasCustomPosition,
    autoGen: draft.subAutoGen,
    sourceLang: draft.subSourceLang,
    targetLang: draft.subTargetLang,
  },
})

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
    draft.wmEnabled = !!e.watermark.enabled
    draft.wmPath = e.watermark.imgPath || ''
    draft.wmScale = e.watermark.scale || 0.2
    draft.wmOpacity = e.watermark.opacity ?? 1.0
    draft.wmX = parseNormalizedExpr(e.watermark.x, 'x', 'watermark', 0.97)
    draft.wmY = parseNormalizedExpr(e.watermark.y, 'y', 'watermark', 0.97)
    draft.wmStartTime = Math.max(0, e.watermark.startTime || 0)
    draft.wmEndTime = Math.max(0, e.watermark.endTime || 0)
  }
  if (e.audio) { draft.muteOriginal = !!e.audio.mute; draft.musicPath = e.audio.musicPath || (e.audio.musicTracks && e.audio.musicTracks[0]) || ''; draft.musicVolume = e.audio.musicVolume ?? 0.3; draft.musicLoop = !!e.audio.musicLoop }
  if (e.card) {
    draft.cardEnabled = !!e.card.enabled
    draft.cardMode = e.card.mode || 'preset'
    draft.cardPreset = e.card.preset || 'glass'
    draft.cardImgPath = e.card.imgPath || ''
    draft.cardColor = e.card.color || '#0f172a'
    draft.cardColor2 = e.card.color2 || '#1e293b'
    draft.cardOpacity = e.card.opacity ?? 0.85
    draft.cardX = parseNormalizedExpr(e.card.x, 'x', 'watermark', 0.5)
    draft.cardY = parseNormalizedExpr(e.card.y, 'y', 'watermark', 0.75)
    draft.cardWidth = e.card.width || 82
    draft.cardHeight = e.card.height || 22
    draft.cardBorderRadius = e.card.borderRadius ?? 12
    draft.cardStartTime = Math.max(0, e.card.startTime || 0)
    draft.cardEndTime = Math.max(0, e.card.endTime || 0)
    draft.cardAboveText = !!e.card.cardAboveText
  }
  if (e.subtitle) {
    draft.subEnabled = !!e.subtitle.enabled
    draft.subPath = e.subtitle.path || ''
    draft.subFontSize = e.subtitle.fontSize || 24
    draft.subFontColor = assToCSSColor(e.subtitle.fontColor, '#ffffff')
    draft.subOutlineColor = assToCSSColor(e.subtitle.outlineCol, '#000000')
    draft.subMarginV = e.subtitle.marginV ?? 40
    draft.subHasCustomPosition = !!e.subtitle.hasCustomPosition
    draft.subX = draft.subHasCustomPosition ? clamp01(e.subtitle.positionX ?? 0.5) : 0.5
    draft.subY = draft.subHasCustomPosition ? clamp01(e.subtitle.positionY ?? 0.9) : 0.9
    draft.subAutoGen = !!e.subtitle.autoGen
    draft.subSourceLang = e.subtitle.sourceLang || 'auto'
    draft.subTargetLang = e.subtitle.targetLang || ''
  }
  draft.randomText = !!e.randomText
  draft.textEnabled = e.textEnabled !== undefined ? !!e.textEnabled : (Array.isArray(e.texts) && e.texts.length > 0)
  draft.texts = Array.isArray(e.texts) ? e.texts.map((t: any, index: number) => ({
    id: `text-${Date.now()}-${index}`,
    content: t.content || '',
    fontSize: t.fontSize || 48,
    color: t.color || '#ffffff',
    x: parseNormalizedExpr(t.x, 'x', 'text', 0.5),
    y: parseNormalizedExpr(t.y, 'y', 'text', 0.82),
    startTime: Math.max(0, t.startTime || 0),
    endTime: Math.max(0, t.endTime || 0),
    bgBox: !!t.bgBox,
    selected: t.selected !== false,
  })) : []
  selectedTextId.value = ''
}

function openNew() {
  editingId.value = 'new'
  draftName.value = `Kịch bản ${scenarios.value.length + 1}`
  resetDraft()
}

function openEdit(s: RemixScenario) {
  editingId.value = s.id
  draftName.value = s.name
  editToDraft(s.edit)
}

const saveScenario = (silent: boolean = false) => {
  const isSilent = typeof silent === 'boolean' ? silent : false
  if (!draftName.value.trim()) {
    props.showToast('Nhập tên kịch bản trước đã!', 'warning')
    return
  }
  const edit = draftToEdit()
  if (editingId.value === 'new') {
    const newObj = { id: Date.now().toString(), name: draftName.value.trim(), edit }
    scenarios.value.push(newObj)
    editingId.value = newObj.id
    if (!isSilent) props.showToast('Đã tạo và lưu kịch bản mới.', 'success')
  } else {
    const idx = scenarios.value.findIndex(s => s.id === editingId.value)
    if (idx !== -1) scenarios.value[idx] = { id: editingId.value, name: draftName.value.trim(), edit }
    if (!isSilent) props.showToast('Đã lưu kịch bản.', 'success')
  }
  persist()
}

const onSaveBtnClick = () => {
  saveScenario(false)
}

const exportCurrentScenario = async () => {
  // Bấm xuất -> hiện hộp chọn thư mục lưu video trước, chọn xong mới xuất.
  let dir = ''
  try {
    const mod: any = await import('../../wailsjs/go/main/App')
    if (mod.SelectFolder) dir = await mod.SelectFolder()
  } catch (e) { console.error(e) }
  if (!dir) return // Hủy chọn thư mục -> không xuất.
  emit('update:outputDir', dir)
  saveScenario(true)
  // Truyền id kịch bản ĐANG MỞ để export áp đúng nó (không phụ thuộc tick ở tab Cắt & Xuất).
  emit('export', editingId.value)
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
    }
  } catch (e) { console.error(e) }
}

const pickOutputDir = async () => {
  try {
    const mod: any = await import('../../wailsjs/go/main/App')
    if (mod.SelectFolder) {
      const dir = await mod.SelectFolder()
      if (dir) {
        emit('update:outputDir', dir)
        props.showToast(`Đã chọn thư mục lưu: ${dir}`, 'success')
      }
    }
  } catch (e) { console.error(e) }
}

const onScenarioChange = (event: Event) => {
  const value = (event.target as HTMLSelectElement).value
  const found = scenarios.value.find(s => s.id === value)
  if (found) openEdit(found)
}

const onVideoChange = (event: Event) => {
  const idx = parseInt((event.target as HTMLSelectElement).value, 10)
  if (!Number.isNaN(idx)) emit('selectVideo', idx)
}

const fileName = (p: string) => p ? (p.split('\\').pop() || p) : ''

onMounted(load)
</script>

<template>
  <div class="capcut-studio-page">
    <header class="capcut-header">
      <!-- Quick Preset Selector Dropdown -->
      <div class="capcut-chips-bar">
        <span class="capcut-chips-lbl">
          <Layers :size="13" /> Kịch bản ({{ scenarios.length }}):
        </span>
        <div style="display: flex; align-items: center; gap: 6px;">
          <select
            :value="editingId"
            @change="onScenarioChange"
            class="capcut-preset-select"
          >
            <option v-for="s in scenarios" :key="s.id" :value="s.id" style="background-color: #0f172a; color: #f8fafc;">
              {{ s.name }}
            </option>
          </select>
          <button @click="openNew" class="capcut-add-chip-btn" title="Tạo kịch bản mới">
            <Plus :size="12" /> Tạo mới
          </button>
        </div>
      </div>

      <div class="capcut-header-actions">
        <div style="display: flex; align-items: center; gap: 6px; background: rgba(0,0,0,0.3); border: 1px solid var(--wx-border-default); border-radius: 6px; padding: 2px 8px; height: 32px; margin-right: 4px;">
          <Video :size="13" style="color: var(--wx-text-secondary); flex-shrink: 0;" />
          <select
            v-if="videos && videos.length > 0"
            :value="activeVideoIndex ?? 0"
            @change="onVideoChange"
            title="Chọn video để chỉnh sửa"
            style="background: transparent; border: none; color: var(--wx-text-primary); font-size: 11px; max-width: 180px; outline: none; cursor: pointer;"
          >
            <option v-for="(p, i) in videos" :key="i" :value="i" style="background-color: #0f172a; color: #f8fafc;">
              {{ fileName(p) }}
            </option>
          </select>
          <span v-else style="font-size: 11px; color: var(--wx-text-secondary); white-space: nowrap;">Chưa có video</span>
          <button
            type="button"
            @click="emit('selectExternalVideo')"
            title="Chọn video từ máy"
            style="background: rgba(255,255,255,0.08); border: 1px solid rgba(255,255,255,0.15); color: #38bdf8; border-radius: 4px; padding: 3px 6px; cursor: pointer; display: flex; align-items: center; justify-content: center;"
          >
            <Folder :size="13" />
          </button>
        </div>
        <button v-if="editingId && editingId !== 'new'" class="btn capcut-sec-btn" @click="duplicateScenario(scenarios.find(s => s.id === editingId)!)" title="Nhân bản kịch bản này">
          <Copy :size="13" /> Nhân bản
        </button>
        <button v-if="editingId && editingId !== 'new'" class="btn capcut-danger-btn" @click="deleteScenario(editingId)" title="Xóa kịch bản này">
          <Trash2 :size="13" /> Xóa
        </button>
        <button class="btn capcut-save-btn" @click="onSaveBtnClick">
          <Save :size="14" /> {{ editingId === 'new' ? 'Lưu Kịch Bản Mới' : 'Lưu Thay Đổi' }}
        </button>
        <!-- Widget xuất video gọn đẹp ngay trên thanh header -->
        <div v-if="isExporting" style="display: flex; align-items: center; gap: 10px; background: rgba(99, 102, 241, 0.15); border: 1.5px solid var(--wx-brand-primary, #6366f1); border-radius: 8px; padding: 4px 10px; height: 34px;">
          <div style="display: flex; align-items: center; gap: 8px;">
            <Loader2 :size="14" class="spin-hourglass" style="color: #c084fc; flex-shrink: 0;" />
            <div style="display: flex; flex-direction: column; justify-content: center; gap: 2px;">
              <div style="font-size: 11px; font-weight: 700; color: #ffffff; line-height: 1; white-space: nowrap;">
                Đang xuất {{ Math.round(((exportProgress?.done || 0) / (exportProgress?.total || 1)) * 100) }}% ({{ exportProgress?.done || 0 }}/{{ exportProgress?.total || 1 }})
              </div>
              <div style="width: 110px; height: 4px; background: rgba(255,255,255,0.2); border-radius: 2px; overflow: hidden;">
                <div :style="{ width: Math.max(8, Math.round(((exportProgress?.done || 0) / (exportProgress?.total || 1)) * 100)) + '%' }" style="height: 100%; background: linear-gradient(90deg, #6366f1, #a855f7); transition: width 0.3s ease;"></div>
              </div>
            </div>
          </div>

          <!-- Nút DỪNG XUẤT (FE & BE) -->
          <button
            type="button"
            @click="emit('stop-export')"
            title="Dừng tiến trình xuất video ngay lập tức (Dừng cả FE và BE)"
            style="background: #ef4444; color: #fff; border: none; border-radius: 6px; padding: 3px 10px; font-size: 11.5px; font-weight: 700; cursor: pointer; display: inline-flex; align-items: center; gap: 4px; transition: all 0.2s; height: 26px;"
          >
            <Square :size="11" fill="currentColor" /> Dừng
          </button>
        </div>

        <button
          v-else
          class="btn capcut-export-now-btn"
          @click="exportCurrentScenario"
          title="Lưu kịch bản và tiến hành xuất video ngay"
        >
          <Video :size="14" />
          <span>Lưu và Xuất Video</span>
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
              <span class="lbl-txt">Zoom &amp; Vị trí góc quay ({{ draft.zoomFactor }}x)</span>
            </label>
            <div v-if="draft.zoomEnabled" style="margin-top: 6px; display: flex; flex-direction: column; gap: 6px; background: rgba(0,0,0,0.2); border: 1px solid rgba(255,255,255,0.08); border-radius: 8px; padding: 8px;">
              <div style="display: flex; align-items: center; justify-content: space-between; font-size: 11px;">
                <span>Phóng to: <strong>{{ draft.zoomFactor }}x</strong></span>
                <div class="quick-speed-pills">
                  <button type="button" class="sp-pill" :class="{ active: draft.zoomFactor === 1.1 }" @click="draft.zoomFactor = 1.1">1.1x</button>
                  <button type="button" class="sp-pill" :class="{ active: draft.zoomFactor === 1.5 }" @click="draft.zoomFactor = 1.5">1.5x</button>
                  <button type="button" class="sp-pill" :class="{ active: draft.zoomFactor === 2.0 }" @click="draft.zoomFactor = 2.0">2.0x</button>
                  <button type="button" class="sp-pill" :class="{ active: draft.zoomFactor === 3.0 }" @click="draft.zoomFactor = 3.0">3.0x</button>
                </div>
              </div>
              <input type="range" step="0.01" min="1.01" max="3.0" v-model.number="draft.zoomFactor" class="custom-range-slider" />
              
              <div style="font-size: 11px; color: var(--wx-text-secondary); margin-top: 2px;">Vị trí góc quay (Crop View Position):</div>
              <div style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 4px;">
                <button type="button" class="sp-pill" :class="{ active: draft.videoPanX <= 0.1 }" @click="draft.videoPanX = 0; draft.videoPanY = 0.5; draft.zoomDir = 'left'">⬅ Trái</button>
                <button type="button" class="sp-pill" :class="{ active: Math.abs(draft.videoPanX - 0.5) < 0.1 && Math.abs(draft.videoPanY - 0.5) < 0.1 }" @click="draft.videoPanX = 0.5; draft.videoPanY = 0.5; draft.zoomDir = 'in'">🎯 Giữa</button>
                <button type="button" class="sp-pill" :class="{ active: draft.videoPanX >= 0.9 }" @click="draft.videoPanX = 1.0; draft.videoPanY = 0.5; draft.zoomDir = 'right'">➡ Phải</button>
                <button type="button" class="sp-pill" :class="{ active: draft.videoPanY <= 0.1 }" @click="draft.videoPanX = 0.5; draft.videoPanY = 0; draft.zoomDir = 'up'" style="grid-column: span 1;">⬆ Trên</button>
                <button type="button" class="sp-pill" :class="{ active: draft.videoPanY >= 0.9 }" @click="draft.videoPanX = 0.5; draft.videoPanY = 1.0; draft.zoomDir = 'down'" style="grid-column: span 2;">⬇ Dưới</button>
              </div>

              <!-- Hướng dẫn Kéo thả Pan trực tiếp như Avatar FB -->
              <div style="margin-top: 6px; padding: 6px 8px; background: rgba(56, 189, 248, 0.1); border: 1px solid rgba(56, 189, 248, 0.2); border-radius: 6px; font-size: 11px; color: #38bdf8; display: flex; flex-direction: column; gap: 4px;">
                <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 2px;">
                  <span>Tọa độ Pan: X: {{ Math.round(draft.videoPanX * 100) }}% | Y: {{ Math.round(draft.videoPanY * 100) }}%</span>
                  <button
                    type="button"
                    @click="draft.videoPanX = 0.5; draft.videoPanY = 0.5"
                    style="background: rgba(255,255,255,0.1); border: 1px solid rgba(255,255,255,0.2); color: #fff; border-radius: 4px; padding: 2px 6px; font-size: 10px; cursor: pointer;"
                    title="Đặt lại vị trí góc quay về chính giữa tâm"
                  >
                    🔄 Reset về tâm
                  </button>
                </div>
              </div>
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

            <label class="toggle-switch-lbl" style="margin-top: 10px;">
              <input type="checkbox" v-model="draft.noiseEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Hạt nhiễu Noise (Mức {{ draft.noiseStrength }})</span>
            </label>
            <div class="inline-field-row" v-if="draft.noiseEnabled" style="margin-top: 4px;">
              <input type="range" step="1" min="1" max="40" v-model.number="draft.noiseStrength" class="custom-range-slider" />
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
                <button class="asp-pill" :class="{ active: draft.aspectRatio === '9:16' }" @click="draft.aspectRatio = '9:16'">9:16 (TikTok)</button>
                <button class="asp-pill" :class="{ active: draft.aspectRatio === '1:1' }" @click="draft.aspectRatio = '1:1'">1:1 (Vuông)</button>
                <button class="asp-pill" :class="{ active: draft.aspectRatio === '16:9' }" @click="draft.aspectRatio = '16:9'">16:9 (YouTube)</button>
              </div>
              <div class="inline-field-row" style="margin-top: 6px;">
                <span>Chế độ lót phông:</span>
                <select v-model="draft.aspectMode" class="scenario-select" style="flex: 1; min-width: 0;">
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
                <button class="c-chip" :class="{ active: draft.colorPreset === 'warm' }" @click="draft.colorPreset = 'warm'">Ấm áp</button>
                <button class="c-chip" :class="{ active: draft.colorPreset === 'cool' }" @click="draft.colorPreset = 'cool'">Mát lạnh</button>
                <button class="c-chip" :class="{ active: draft.colorPreset === 'vivid' }" @click="draft.colorPreset = 'vivid'">Rực rỡ</button>
                <button class="c-chip" :class="{ active: draft.colorPreset === 'bw' }" @click="draft.colorPreset = 'bw'">Đen trắng</button>
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
                <button class="btn-picker" @click="pickWatermark">Chọn Logo</button>
                <span class="file-name-hint">{{ fileName(draft.wmPath) || 'Chưa chọn' }}</span>
              </div>
              <div class="wm-pos-selector">
                <button class="pos-mini-btn" @click="setWatermarkPosition(0, 0)">Trái Trên</button>
                <button class="pos-mini-btn" @click="setWatermarkPosition(1, 0)">Phải Trên</button>
                <button class="pos-mini-btn" @click="setWatermarkPosition(0.5, 0.5)">Giữa</button>
                <button class="pos-mini-btn" @click="setWatermarkPosition(0, 1)">Trái Dưới</button>
                <button class="pos-mini-btn" @click="setWatermarkPosition(1, 1)">Phải Dưới</button>
              </div>
              <div class="slider-field-group">
                <div class="slider-info-row"><span>Kích thước: <strong>{{ Math.round(draft.wmScale * 100) }}%</strong></span></div>
                <input v-model.number="draft.wmScale" type="range" min="0.03" max="0.8" step="0.01" class="custom-range-slider" />
              </div>
              <div class="slider-field-group">
                <div class="slider-info-row"><span>Độ mờ: <strong>{{ Math.round(draft.wmOpacity * 100) }}%</strong></span></div>
                <input v-model.number="draft.wmOpacity" type="range" min="0.05" max="1" step="0.05" class="custom-range-slider" />
              </div>
              <p class="drag-help">Kéo logo trực tiếp trên màn hình xem trước để chọn vị trí chính xác.</p>
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 12px;">
              <input type="checkbox" v-model="draft.muteOriginal" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Tắt tiếng video gốc</span>
            </label>
            <div class="inline-field-row" style="margin-top: 6px;">
              <button class="btn-picker" @click="pickMusic">Nhạc nền MP3</button>
              <span class="file-name-hint">{{ fileName(draft.musicPath) || 'Không có' }}</span>
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 12px;">
              <input type="checkbox" v-model="draft.stripMeta" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Tự động xóa sạch Metadata chống quét</span>
            </label>
          </div>

          <!-- CATEGORY 4: SUBTITLE & TEXT -->
          <div v-else-if="activeCategoryTab === 'subtitle'" class="scenario-block-card">
            <h4 class="block-heading">4. Phụ Đề &amp; Chèn Chữ</h4>

            <label class="toggle-switch-lbl">
              <input type="checkbox" v-model="draft.subEnabled" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Ghép file phụ đề .SRT / .ASS</span>
            </label>
            <div v-if="draft.subEnabled" class="sub-fields-group">
              <div class="inline-field-row">
                <button class="btn-picker" @click="pickSubtitle">Chọn File Sub</button>
                <span class="file-name-hint">{{ fileName(draft.subPath) || 'Chưa chọn' }}</span>
              </div>
              <div class="inline-field-row editor-compact-row">
                <label>Cỡ chữ <input v-model.number="draft.subFontSize" type="number" min="10" max="160" class="scenario-input-num" /></label>
                <label>Màu <input v-model="draft.subFontColor" type="color" class="color-input" /></label>
                <label>Viền <input v-model="draft.subOutlineColor" type="color" class="color-input" /></label>
              </div>
              <div class="pos-grid-4">
                <button type="button" class="pos-mini-btn" @click="setSubtitlePosition(0.5, 0.12)">Trên</button>
                <button type="button" class="pos-mini-btn" @click="setSubtitlePosition(0.5, 0.5)">Giữa</button>
                <button type="button" class="pos-mini-btn" @click="setSubtitlePosition(0.5, 0.88)">Dưới</button>
                <button type="button" class="pos-mini-btn" @click="draft.subHasCustomPosition = false; draft.subX = 0.5; draft.subY = 0.9">Đặt lại</button>
              </div>
              <p class="drag-help">Kéo dòng phụ đề mẫu trực tiếp trên video; vị trí này được giữ khi xuất.</p>
            </div>

            <label class="toggle-switch-lbl" style="margin-top: 12px;">
              <input type="checkbox" v-model="draft.subAutoGen" />
              <span class="switch-slider"></span>
              <span class="lbl-txt">Tự nghe Whisper AI tạo sub khi xuất</span>
            </label>
            <div v-if="draft.subAutoGen" class="sub-fields-group auto-gen-box">
              <p class="auto-gen-hint">Whisper AI tự nghe giọng trong video khi xuất và ghép phụ đề.</p>
              <div class="inline-field-row">
                <span>Dịch sang:</span>
                <select v-model="draft.subTargetLang" class="scenario-select" style="flex: 1; min-width: 0;">
                  <option v-for="l in targetLangs" :key="l.code" :value="l.code">{{ l.name }}</option>
                </select>
              </div>
            </div>

            <div class="text-editor-section" style="margin-bottom: 12px;">
              <div class="text-section-head">
                <div class="text-title-with-toggle">
                  <label class="toggle-switch-lbl inline-toggle" title="Bật / Tắt chèn khung nền card">
                    <input type="checkbox" v-model="draft.cardEnabled" />
                    <span class="switch-slider"></span>
                  </label>
                  <span class="text-section-title">Chèn Nền Card / Banner đệm chữ</span>
                </div>
              </div>

              <div v-if="draft.cardEnabled" class="sub-fields-group">
                <!-- Chọn Mode: Mẫu hoa văn / Tự chọn màu / Tải ảnh -->
                <div class="preset-color-chips">
                  <button class="c-chip" :class="{ active: draft.cardMode === 'preset' }" @click="draft.cardMode = 'preset'">Mẫu Có Sẵn</button>
                  <button class="c-chip" :class="{ active: draft.cardMode === 'color' }" @click="draft.cardMode = 'color'">Tự Chọn Màu Nền</button>
                  <button class="c-chip" :class="{ active: draft.cardMode === 'image' }" @click="draft.cardMode = 'image'">Tải Ảnh Nền</button>
                </div>

                <!-- Mode Presets (Clean Text Chips, No Checkboxes) -->
                <div v-if="draft.cardMode === 'preset'" class="card-presets-grid">
                  <button
                    v-for="p in patternPresets"
                    :key="p.id"
                    type="button"
                    class="card-preset-chip"
                    :class="{ active: draft.cardPreset === p.id }"
                    @click="selectCardPreset(p.id)"
                  >
                    {{ p.name }}
                  </button>
                </div>

                <!-- Mode Color (Color Pickers & Auto-seek Swatches) -->
                <div v-else-if="draft.cardMode === 'color'" class="card-color-picker-group">
                  <div class="inline-field-row editor-compact-row">
                    <label>Màu 1: <input type="color" v-model="draft.cardColor" class="color-picker-input" @input="draft.cardEnabled = true" /></label>
                    <label>Màu 2 (Gradient): <input type="color" v-model="draft.cardColor2" class="color-picker-input" @input="draft.cardEnabled = true" /></label>
                  </div>
                  <div class="quick-swatches">
                    <button type="button" class="swatch-btn" style="background: #000000" title="Đen" @click="setCardColor('#000000', '#000000')"></button>
                    <button type="button" class="swatch-btn" style="background: #0f172a" title="Xanh Đen" @click="setCardColor('#0f172a', '#1e293b')"></button>
                    <button type="button" class="swatch-btn" style="background: #dc2626" title="Đỏ" @click="setCardColor('#dc2626', '#991b1b')"></button>
                    <button type="button" class="swatch-btn" style="background: #eab308" title="Vàng Gold" @click="setCardColor('#eab308', '#ca8a04')"></button>
                    <button type="button" class="swatch-btn" style="background: #9333ea" title="Tím Neon" @click="setCardColor('#9333ea', '#7e22ce')"></button>
                    <button type="button" class="swatch-btn" style="background: #2563eb" title="Xanh Dương" @click="setCardColor('#2563eb', '#1d4ed8')"></button>
                    <button type="button" class="swatch-btn" style="background: #059669" title="Xanh Lá" @click="setCardColor('#059669', '#047857')"></button>
                    <button type="button" class="swatch-btn" style="background: #f97316" title="Cam Nắng" @click="setCardColor('#f97316', '#c2410c')"></button>
                    <button type="button" class="swatch-btn" style="background: #ec4899" title="Hồng Cánh Sen" @click="setCardColor('#ec4899', '#be185d')"></button>
                    <button type="button" class="swatch-btn" style="background: #ffffff" title="Trắng" @click="setCardColor('#ffffff', '#e2e8f0')"></button>
                  </div>
                </div>

                <!-- Mode Image -->
                <div v-else class="inline-field-row">
                  <button class="btn-picker" @click="pickCardImage">Chọn File Ảnh Nền</button>
                  <span class="file-name-hint">{{ fileName(draft.cardImgPath) || 'Chưa chọn ảnh' }}</span>
                </div>

                <!-- Sliders: Độ mờ & Bo góc -->
                <div class="slider-field-group">
                  <div class="slider-info-row"><span>Độ mờ nền: <strong>{{ Math.round(draft.cardOpacity * 100) }}%</strong></span></div>
                  <input v-model.number="draft.cardOpacity" type="range" min="0.1" max="1" step="0.05" class="custom-range-slider" />
                </div>
                <div class="slider-field-group">
                  <div class="slider-info-row"><span>Bo góc Card: <strong>{{ draft.cardBorderRadius }}px</strong></span></div>
                  <input v-model.number="draft.cardBorderRadius" type="range" min="0" max="40" step="1" class="custom-range-slider" />
                </div>

                <!-- Timing & Pos -->
                <div class="inline-field-row editor-compact-row">
                  <label>Bắt đầu <input v-model.number="draft.cardStartTime" type="number" min="0" step="0.1" class="scenario-input-num" /></label>
                  <label>Kết thúc <input v-model.number="draft.cardEndTime" type="number" min="0" step="0.1" class="scenario-input-num" /></label>
                </div>
                <div class="pos-grid-3">
                  <button type="button" class="pos-mini-btn" @click="draft.cardX = 0.5; draft.cardY = 0.12">Trên</button>
                  <button type="button" class="pos-mini-btn" @click="draft.cardX = 0.5; draft.cardY = 0.5">Giữa</button>
                  <button type="button" class="pos-mini-btn" @click="draft.cardX = 0.5; draft.cardY = 0.8">Dưới</button>
                </div>
                <p class="drag-help">Kéo khung nền Card trực tiếp trên video để chỉnh vị trí chính xác. Giữ Ctrl + Cuộn chuột để đổi kích thước.</p>
              </div>
            </div>

            <div class="text-editor-section">
              <div class="text-section-head">
                <div class="text-title-with-toggle">
                  <label class="toggle-switch-lbl inline-toggle" title="Bật / Tắt tính năng chèn text">
                    <input type="checkbox" v-model="draft.textEnabled" />
                    <span class="switch-slider"></span>
                  </label>
                  <span class="text-section-title">Chèn text ({{ draft.texts.length }})</span>
                </div>
                <div class="text-head-actions">
                  <button class="btn-text-action add" @click="addText"><Plus :size="11" /> Thêm chữ</button>
                </div>
              </div>

              <div v-if="draft.textEnabled">
                <div v-if="draft.texts.length === 0" class="text-empty">Chưa có text. Bấm “Thêm chữ” để tạo tiêu đề/caption.</div>
                <div v-else class="text-chip-list">
                  <button
                    v-for="(text, index) in draft.texts"
                    :key="text.id"
                    class="text-select-chip"
                    :class="{
                      checked: text.selected !== false,
                      active: selectedTextId === text.id
                    }"
                    @click="toggleTextSelected(text)"
                    :title="`Bấm để ${text.selected !== false ? 'bỏ chọn' : 'tích chọn'} câu chữ này`"
                  >{{ index + 1 }}. {{ text.content || 'Text trống' }}</button>
                </div>

                <div v-if="selectedText" class="text-detail-card">
                  <div class="text-detail-header">
                    <span class="text-detail-title">Chỉnh sửa chi tiết Text #{{ draft.texts.findIndex(t => t.id === selectedText?.id) + 1 }}</span>
                    <button class="btn-done-text" @click="selectedTextId = ''" title="Bấm để ẩn bảng chỉnh sửa"><Check :size="12" /> Ẩn bảng</button>
                  </div>
                  <textarea v-model="selectedText.content" rows="2" class="scenario-textarea" placeholder="Nhập nội dung chữ..."></textarea>
                  <div class="inline-field-row editor-compact-row">
                    <label>Cỡ <input v-model.number="selectedText.fontSize" type="number" min="8" max="300" class="scenario-input-num" /></label>
                    <label>Màu <input v-model="selectedText.color" type="color" class="color-input" /></label>
                    <label class="check-inline"><input v-model="selectedText.bgBox" type="checkbox" /> Nền mờ</label>
                  </div>
                  <div class="inline-field-row editor-compact-row">
                    <label>Bắt đầu <input v-model.number="selectedText.startTime" type="number" min="0" step="0.1" class="scenario-input-num" /></label>
                    <label>Kết thúc <input v-model.number="selectedText.endTime" type="number" min="0" step="0.1" class="scenario-input-num" /></label>
                  </div>
                  <div class="pos-action-group">
                    <div class="pos-grid-3">
                      <button type="button" class="pos-mini-btn" @click="setTextPosition(selectedText, 0.5, 0.08)">Trên</button>
                      <button type="button" class="pos-mini-btn" @click="setTextPosition(selectedText, 0.5, 0.5)">Giữa</button>
                      <button type="button" class="pos-mini-btn" @click="setTextPosition(selectedText, 0.5, 0.9)">Dưới</button>
                    </div>
                    <div class="pos-grid-2">
                      <button type="button" class="pos-mini-btn" @click="duplicateText(selectedText)"><Copy :size="12" /> Nhân bản</button>
                      <button type="button" class="pos-mini-btn danger" @click="removeText(selectedText.id)"><Trash2 :size="12" /> Xóa</button>
                    </div>
                    <div class="pos-grid-2" style="margin-top: 5px;">
                      <button type="button" class="pos-mini-btn" @click="moveTextUp(selectedText)" title="Đưa câu chữ này đè lên trên chữ khác"><ArrowUp :size="12" /> Lên lớp trên</button>
                      <button type="button" class="pos-mini-btn" @click="moveTextDown(selectedText)" title="Đưa câu chữ này xuống dưới chữ khác"><ArrowDown :size="12" /> Xuống lớp dưới</button>
                    </div>
                  </div>
                  <div class="text-detail-footer">
                    <span class="drag-help-inline">Kéo text trực tiếp trên video. End=0 là hiện tới hết.</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- PANEL 2: Center Player Preview Monitor -->
      <div class="capcut-center-panel">
        <div class="player-monitor-header">
          <span class="monitor-title">Màn Hình Live Preview</span>
        </div>

        <div class="player-monitor-screen-wrap">
          <div
            ref="playerFrameRef"
            class="player-monitor-frame"
            :class="{ 'is-overlay-dragging': !!dragTarget }"
            :style="{
              aspectRatio: draft.aspectEnabled ? draft.aspectRatio.replace(':', '/') : '16/9'
            }"
            @wheel.prevent="onFrameWheel"
          >
            <!-- Nền blur (mode 'blur'): video phủ full khung, làm mờ mạnh, nằm dưới video chính.
                 Khớp cách ffmpeg: split → gblur nền + overlay video chính co giữa. -->
            <video
              v-if="activeVideoSrc && draft.aspectEnabled && draft.aspectMode === 'blur'"
              ref="bgVideoRef"
              :src="activeVideoSrc"
              class="capcut-blur-bg"
              muted
              playsinline
            ></video>

            <!-- Video chính phát xem trực tiếp (Hỗ trợ nhấp giữ & kéo trực tiếp trên video để căn vị trí như avatar FB) -->
            <video
              v-if="activeVideoSrc"
              ref="videoRef"
              :src="activeVideoSrc"
              class="real-capcut-video"
              :class="{ 'is-panning': dragTarget === 'video-pan' }"
              :style="videoStyle"
              title="Nhấp giữ và kéo trực tiếp trên video để căn chỉnh vị trí khung hình (Avatar FB style)"
              @pointerdown="beginVideoPanDrag"
              @pointermove="updateOverlayDrag"
              @pointerup="finishOverlayDrag"
              @pointercancel="cancelOverlayDrag"
              @lostpointercapture="cancelOverlayDrag"
              @timeupdate="onTimeUpdate"
              @loadedmetadata="onMetadataLoaded"
              @ended="isPlaying = false"
              playsinline
            ></video>

            <!-- Chưa nạp video nguồn -->
            <div v-else class="video-preview-placeholder">
              <Film :size="40" style="opacity: 0.4; margin-bottom: 8px;" />
              <span>Chưa nạp video nguồn</span>
              <span class="sub-hint">(Thêm video ở tab "Cắt &amp; Xuất Video" để phát và xem hiệu ứng lật/màu sắc trực tiếp tại đây!)</span>
            </div>

            <!-- Card Nền Overlay (nằm phía sau text) -->
            <div
              v-if="draft.cardEnabled && isCardVisible"
              :style="cardStyle"
              class="draggable-overlay card-preview-overlay"
              :class="[
                `preset-${draft.cardPreset}`,
                { 'active-selected': dragTarget === 'card' }
              ]"
              @pointerdown="beginOverlayDrag('card', $event)"
              @pointermove="updateOverlayDrag"
              @pointerup="finishOverlayDrag"
              @pointercancel="cancelOverlayDrag"
              @lostpointercapture="cancelOverlayDrag"
              @wheel.prevent="onOverlayWheel($event, 'card')"
            >
              <img v-if="draft.cardMode === 'image' && cardStreamUrl" :src="cardStreamUrl" class="card-bg-img" alt="card" />

              <!-- 8 Resize Handles (4 Góc + 4 Cạnh) -->
              <div class="overlay-resize-handle handle-nw" title="Kéo dài/rộng góc trên-trái" @pointerdown.stop="beginResizeHandle('card', 'nw', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-ne" title="Kéo dài/rộng góc trên-phải" @pointerdown.stop="beginResizeHandle('card', 'ne', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-sw" title="Kéo dài/rộng góc dưới-trái" @pointerdown.stop="beginResizeHandle('card', 'sw', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-se" title="Kéo dài/rộng góc dưới-phải" @pointerdown.stop="beginResizeHandle('card', 'se', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-n" title="Kéo dãn viền trên" @pointerdown.stop="beginResizeHandle('card', 'n', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-s" title="Kéo dãn viền dưới" @pointerdown.stop="beginResizeHandle('card', 's', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-w" title="Kéo dãn viền trái" @pointerdown.stop="beginResizeHandle('card', 'w', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-e" title="Kéo dãn viền phải" @pointerdown.stop="beginResizeHandle('card', 'e', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
            </div>

            <!-- Logo watermark thật -->
            <div
              v-if="draft.wmEnabled && wmStreamUrl && isWmVisible"
              :style="wmStyle"
              class="draggable-overlay wm-preview-overlay"
              :class="{ 'active-selected': dragTarget === 'watermark' }"
              @pointerdown="beginOverlayDrag('watermark', $event)"
              @pointermove="updateOverlayDrag"
              @pointerup="finishOverlayDrag"
              @pointercancel="cancelOverlayDrag"
              @lostpointercapture="cancelOverlayDrag"
              @wheel.prevent="onOverlayWheel($event, 'watermark')"
            >
              <img :src="wmStreamUrl" class="overlay-img-content" alt="logo" />

              <!-- 8 Resize Handles (4 Góc + 4 Cạnh) -->
              <div class="overlay-resize-handle handle-nw" title="Phóng to/thu nhỏ góc trên-trái" @pointerdown.stop="beginResizeHandle('watermark', 'nw', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-ne" title="Phóng to/thu nhỏ góc trên-phải" @pointerdown.stop="beginResizeHandle('watermark', 'ne', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-sw" title="Phóng to/thu nhỏ góc dưới-trái" @pointerdown.stop="beginResizeHandle('watermark', 'sw', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-se" title="Phóng to/thu nhỏ góc dưới-phải" @pointerdown.stop="beginResizeHandle('watermark', 'se', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-n" title="Phóng to/thu nhỏ viền trên" @pointerdown.stop="beginResizeHandle('watermark', 'n', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-s" title="Phóng to/thu nhỏ viền dưới" @pointerdown.stop="beginResizeHandle('watermark', 's', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-w" title="Phóng to/thu nhỏ viền trái" @pointerdown.stop="beginResizeHandle('watermark', 'w', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-e" title="Phóng to/thu nhỏ viền phải" @pointerdown.stop="beginResizeHandle('watermark', 'e', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
            </div>

            <!-- Phụ đề mẫu -->
            <div
              v-if="draft.subEnabled || draft.subAutoGen"
              :style="subStyle"
              class="draggable-overlay subtitle-preview-overlay"
              :class="{ 'active-selected': dragTarget === 'subtitle' }"
              @pointerdown="beginOverlayDrag('subtitle', $event)"
              @pointermove="updateOverlayDrag"
              @pointerup="finishOverlayDrag"
              @pointercancel="cancelOverlayDrag"
              @lostpointercapture="cancelOverlayDrag"
              @wheel.prevent="onOverlayWheel($event, 'subtitle')"
            >
              <span>Phụ đề mẫu xem trước</span>
              <div class="overlay-resize-handle handle-nw" @pointerdown.stop="beginResizeHandle('subtitle', 'nw', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-ne" @pointerdown.stop="beginResizeHandle('subtitle', 'ne', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-sw" @pointerdown.stop="beginResizeHandle('subtitle', 'sw', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-se" @pointerdown.stop="beginResizeHandle('subtitle', 'se', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
            </div>

            <!-- Text overlay thật -->
            <div
              v-for="text in visibleTexts"
              :key="text.id"
              :style="textStyle(text)"
              class="draggable-overlay text-preview-overlay"
              :class="{ 'active-selected': selectedTextId === text.id }"
              @pointerdown="beginOverlayDrag(`text:${text.id}`, $event)"
              @pointermove="updateOverlayDrag"
              @pointerup="finishOverlayDrag"
              @pointercancel="cancelOverlayDrag"
              @lostpointercapture="cancelOverlayDrag"
              @wheel.prevent="onOverlayWheel($event, `text:${text.id}`)"
            >
              <span>{{ text.content }}</span>
              <div class="overlay-resize-handle handle-nw" title="Tăng/giảm cỡ chữ" @pointerdown.stop="beginResizeHandle(`text:${text.id}`, 'nw', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-ne" title="Tăng/giảm cỡ chữ" @pointerdown.stop="beginResizeHandle(`text:${text.id}`, 'ne', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-sw" title="Tăng/giảm cỡ chữ" @pointerdown.stop="beginResizeHandle(`text:${text.id}`, 'sw', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
              <div class="overlay-resize-handle handle-se" title="Tăng/giảm cỡ chữ" @pointerdown.stop="beginResizeHandle(`text:${text.id}`, 'se', $event)" @pointermove="updateResizeHandle" @pointerup="finishResizeHandle" @pointercancel="finishResizeHandle"></div>
            </div>


          </div>
        </div>

        <!-- Transport Controls Bar -->
        <div class="player-transport-bar">
          <div class="timecode">{{ currentTimeStr }} / {{ durationTimeStr }}</div>
          <div class="transport-controls">
            <button class="transport-btn" @click="restartVideo" title="Phát lại từ đầu"><RotateCcw :size="14" /></button>
            <button class="transport-btn play-btn" @click="togglePlay" :title="isPlaying ? 'Tạm dừng' : 'Phát'">
              <Pause v-if="isPlaying" :size="16" />
              <Play v-else :size="16" />
            </button>
          </div>
        </div>
      </div>

      <!-- PANEL 3: Right Summary & Scenario List Inspector -->
      <div class="capcut-right-panel">
        <div class="panel-section-title">
          <Layers :size="14" />
          <span>Thông Tin Kịch Bản Mẫu</span>
        </div>

        <div class="scenario-name-editor">
          <label class="inspector-label">Tên Kịch Bản Mẫu:</label>
          <input
            type="text"
            v-model="draftName"
            class="inspector-input"
            placeholder="VD: TikTok Shorts 9:16 Mirror..."
          />
        </div>

        <div class="inspector-summary-card">
          <h4 class="summary-heading">Các hiệu ứng đang bật:</h4>
          <ul class="summary-checklist">
            <li :class="{ active: draft.hflip }" @click="activeCategoryTab = 'video'">Lật ngang video (Mirror)</li>
            <li :class="{ active: draft.speed !== 1.0 }" @click="activeCategoryTab = 'video'">Tốc độ phát: {{ draft.speed }}x</li>
            <li :class="{ active: draft.zoomEnabled }" @click="activeCategoryTab = 'video'">Zoom Ken Burns: {{ draft.zoomFactor }}x</li>
            <li :class="{ active: draft.cropEnabled }" @click="activeCategoryTab = 'video'">Cắt rìa Crop: {{ Math.round(draft.cropPercent * 100) }}%</li>
            <li :class="{ active: draft.rotateEnabled }" @click="activeCategoryTab = 'video'">Xoay nghiêng: {{ draft.rotateDegrees }}°</li>
            <li :class="{ active: draft.trimStart > 0 || draft.trimEnd > 0 }" @click="activeCategoryTab = 'video'">Cắt đầu {{ draft.trimStart }}s / đuôi {{ draft.trimEnd }}s</li>
            <li :class="{ active: draft.aspectEnabled }" @click="activeCategoryTab = 'color'">Khung {{ draft.aspectRatio }} ({{ draft.aspectMode }})</li>
            <li :class="{ active: draft.colorEnabled }" @click="activeCategoryTab = 'color'">Màu sắc: {{ draft.colorPreset ? draft.colorPreset.toUpperCase() : 'Chỉnh màu' }}</li>
            <li :class="{ active: draft.noiseEnabled }" @click="activeCategoryTab = 'video'">Hạt nhiễu Noise (Mức {{ draft.noiseStrength }})</li>
            <li :class="{ active: draft.wmEnabled && !!draft.wmPath }" @click="activeCategoryTab = 'audio'">Logo Watermark (Cỡ {{ Math.round(draft.wmScale * 100) }}%)</li>
            <li :class="{ active: !!draft.musicPath }" @click="activeCategoryTab = 'audio'">Nhạc nền MP3</li>
            <li :class="{ active: draft.muteOriginal }" @click="activeCategoryTab = 'audio'">Tắt tiếng video gốc</li>
            <li :class="{ active: draft.subEnabled && !!draft.subPath }" @click="activeCategoryTab = 'subtitle'">Ghép phụ đề file (.SRT / .ASS)</li>
            <li :class="{ active: draft.subAutoGen }" @click="activeCategoryTab = 'subtitle'">Whisper AI tự tạo phụ đề</li>
            <li :class="{ active: selectedTextsCount > 0 }" @click="activeCategoryTab = 'subtitle'">Chèn chữ</li>
            <li :class="{ active: draft.cardEnabled }" @click="activeCategoryTab = 'subtitle'">Chèn nền Card / Banner đệm chữ</li>
            <li :class="{ active: draft.stripMeta }" @click="activeCategoryTab = 'audio'">Xóa Metadata chống quét</li>
          </ul>
        </div>
      </div>
    </div>

    <!-- ── 3. VISUAL CAPCUT TIMELINE TRACK ────────────────────────── -->
    <div
      class="capcut-timeline-bar"
      @pointerdown="onTimelinePointerDown"
      @pointermove="onTimelinePointerMove"
      @pointerup="finishTimelineDrag"
      @pointercancel="cancelTimelineDrag"
      @lostpointercapture="cancelTimelineDrag"
    >
      <div class="timeline-ruler">
        <span
          v-for="mark in timelineMarks"
          :key="mark.ratio"
          class="ruler-mark"
          :style="{
            left: `calc(84px + (100% - 84px) * ${mark.ratio})`,
            transform: mark.ratio === 1 ? 'translateX(-100%)' : (mark.ratio === 0 ? 'translateX(0)' : 'translateX(-50%)')
          }"
        >{{ mark.label }}</span>
      </div>

      <div
        ref="tracksRef"
        class="timeline-tracks"
        :class="{ dragging: isTimelineDragging }"
      >
        <!-- Playhead bám sát con trỏ khi kéo và tiếp tục chạy cùng video sau khi thả. -->
        <div
          v-if="durationSec > 0"
          class="timeline-playhead"
          :class="{ dragging: isTimelineDragging }"
          :style="{ left: `calc(84px + (100% - 84px) * ${playheadPercent / 100})` }"
        ></div>

        <!-- Track 1: Video -->
        <div class="timeline-track video-track">
          <div class="track-head">Video V1</div>
          <div class="track-content">
            <div class="track-block video-block" :style="videoBlockStyle">
              <span>Clip Video · Tốc độ {{ draft.speed }}x {{ draft.hflip ? '· Lật ngang Mirror' : '' }} {{ draft.aspectEnabled ? '· ' + draft.aspectRatio : '' }}</span>
            </div>
          </div>
        </div>

        <!-- Track 2: Audio -->
        <div class="timeline-track audio-track" v-if="draft.musicPath || draft.muteOriginal">
          <div class="track-head">Audio A1</div>
          <div class="track-content">
            <div class="track-block audio-block">
              <span>{{ draft.musicPath ? fileName(draft.musicPath) : 'Tắt tiếng video gốc' }}</span>
            </div>
          </div>
        </div>

        <!-- Track 3: Subtitle -->
        <div class="timeline-track sub-track" v-if="draft.subEnabled || draft.subAutoGen">
          <div class="track-head">Sub S1</div>
          <div class="track-content">
            <div class="track-block sub-block">
              <span>{{ draft.subAutoGen ? 'Whisper AI Tự tạo Phụ Đề' : fileName(draft.subPath) }}</span>
            </div>
          </div>
        </div>

        <!-- Track 3.2: Logo Watermark -->
        <div class="timeline-track wm-track" v-if="draft.wmEnabled && draft.wmPath">
          <div class="track-head">Logo W1</div>
          <div class="track-content wm-track-content">
            <div
              class="track-block wm-block"
              :style="wmBlockStyle"
              title="Logo Watermark"
              @pointerdown.stop="onBlockPointerDown('watermark', 'move', $event)"
              @pointermove="onBlockPointerMove"
              @pointerup="onBlockPointerUp"
              @pointercancel="onBlockPointerUp"
            >
              <div
                class="block-handle left"
                title="Kéo để chỉnh thời gian Bắt đầu"
                @pointerdown.stop="onBlockPointerDown('watermark', 'resize-left', $event)"
              ></div>
              <span class="block-text-label">{{ fileName(draft.wmPath) || 'Logo Watermark' }}</span>
              <span class="block-time-range">{{ formatTimelineMark(draft.wmStartTime) }} - {{ draft.wmEndTime > 0 ? formatTimelineMark(draft.wmEndTime) : 'hết' }}</span>
              <div
                class="block-handle right"
                title="Kéo để chỉnh thời gian Kết thúc"
                @pointerdown.stop="onBlockPointerDown('watermark', 'resize-right', $event)"
              ></div>
            </div>
          </div>
        </div>

        <!-- Danh sách Track linh hoạt hỗ trợ kéo thả hoán đổi vị trí (Nền C1 vs Text T1, T2...) -->
        <template v-if="draft.cardAboveText">
          <!-- Card Track nằm TRÊN Text -->
          <div
            class="timeline-track card-track"
            v-if="draft.cardEnabled"
            draggable="true"
            @dragstart="onTrackDragStart($event, 'card')"
            @dragover="onTrackDragOver"
            @drop="onTrackDrop($event, 'card')"
          >
            <div class="track-head draggable-head" title="Nắm giữ và kéo lên/xuống để hoán đổi thứ tự lớp với Text">
              <GripVertical :size="13" class="drag-grip-icon" /> Nền C1
            </div>
            <div class="track-content card-track-content">
              <div
                class="track-block card-block"
                :style="cardBlockStyle"
                title="Khung Nền Card"
                @pointerdown.stop="onBlockPointerDown('card', 'move', $event)"
                @pointermove="onBlockPointerMove"
                @pointerup="onBlockPointerUp"
                @pointercancel="onBlockPointerUp"
              >
                <div
                  class="block-handle left"
                  title="Kéo để chỉnh thời gian Bắt đầu"
                  @pointerdown.stop="onBlockPointerDown('card', 'resize-left', $event)"
                ></div>
                <span class="block-text-label">Khung Nền Card / Banner</span>
                <span class="block-time-range">{{ formatTimelineMark(draft.cardStartTime) }} - {{ draft.cardEndTime > 0 ? formatTimelineMark(draft.cardEndTime) : 'hết' }}</span>
                <div
                  class="block-handle right"
                  title="Kéo để chỉnh thời gian Kết thúc"
                  @pointerdown.stop="onBlockPointerDown('card', 'resize-right', $event)"
                ></div>
              </div>
            </div>
          </div>

          <!-- Text Tracks nằm DƯỚI Card -->
          <div
            v-for="(row, rIndex) in textTrackRows"
            :key="`text-row-${rIndex}`"
            class="timeline-track text-track"
            draggable="true"
            @dragstart="onTrackDragStart($event, `text:${row[0]?.id}`)"
            @dragover="onTrackDragOver"
            @drop="onTrackDrop($event, `text:${row[0]?.id}`)"
          >
            <div class="track-head draggable-head" title="Nắm giữ và kéo lên/xuống để hoán đổi thứ tự lớp với Nền Card hoặc Text khác">
              <GripVertical :size="13" class="drag-grip-icon" /> Text T{{ rIndex + 1 }}
            </div>
            <div class="track-content text-track-content">
              <div
                v-for="text in row"
                :key="text.id"
                class="track-block text-block"
                :class="{ selected: selectedTextId === text.id }"
                :style="textBlockStyle(text)"
                :title="`${text.content} · ${formatTimelineMark(text.startTime)} → ${text.endTime > 0 ? formatTimelineMark(text.endTime) : 'hết video'}`"
                @pointerdown.stop="onBlockPointerDown('text', 'move', $event, text)"
                @pointermove="onBlockPointerMove"
                @pointerup="onBlockPointerUp"
                @pointercancel="onBlockPointerUp"
              >
                <div
                  class="block-handle left"
                  title="Kéo để chỉnh thời gian Bắt đầu"
                  @pointerdown.stop="onBlockPointerDown('text', 'resize-left', $event, text)"
                ></div>
                <span class="block-text-label">{{ text.content || 'Text' }}</span>
                <span class="block-time-range">{{ formatTimelineMark(text.startTime) }} - {{ text.endTime > 0 ? formatTimelineMark(text.endTime) : 'hết' }}</span>
                <div
                  class="block-handle right"
                  title="Kéo để chỉnh thời gian Kết thúc"
                  @pointerdown.stop="onBlockPointerDown('text', 'resize-right', $event, text)"
                ></div>
              </div>
            </div>
          </div>
        </template>

        <template v-else>
          <!-- Text Tracks nằm TRÊN Card (Mặc định) -->
          <div
            v-for="(row, rIndex) in textTrackRows"
            :key="`text-row-${rIndex}`"
            class="timeline-track text-track"
            draggable="true"
            @dragstart="onTrackDragStart($event, `text:${row[0]?.id}`)"
            @dragover="onTrackDragOver"
            @drop="onTrackDrop($event, `text:${row[0]?.id}`)"
          >
            <div class="track-head draggable-head" title="Nắm giữ và kéo lên/xuống để hoán đổi thứ tự lớp với Nền Card hoặc Text khác">
              <GripVertical :size="13" class="drag-grip-icon" /> Text T{{ rIndex + 1 }}
            </div>
            <div class="track-content text-track-content">
              <div
                v-for="text in row"
                :key="text.id"
                class="track-block text-block"
                :class="{ selected: selectedTextId === text.id }"
                :style="textBlockStyle(text)"
                :title="`${text.content} · ${formatTimelineMark(text.startTime)} → ${text.endTime > 0 ? formatTimelineMark(text.endTime) : 'hết video'}`"
                @pointerdown.stop="onBlockPointerDown('text', 'move', $event, text)"
                @pointermove="onBlockPointerMove"
                @pointerup="onBlockPointerUp"
                @pointercancel="onBlockPointerUp"
              >
                <div
                  class="block-handle left"
                  title="Kéo để chỉnh thời gian Bắt đầu"
                  @pointerdown.stop="onBlockPointerDown('text', 'resize-left', $event, text)"
                ></div>
                <span class="block-text-label">{{ text.content || 'Text' }}</span>
                <span class="block-time-range">{{ formatTimelineMark(text.startTime) }} - {{ text.endTime > 0 ? formatTimelineMark(text.endTime) : 'hết' }}</span>
                <div
                  class="block-handle right"
                  title="Kéo để chỉnh thời gian Kết thúc"
                  @pointerdown.stop="onBlockPointerDown('text', 'resize-right', $event, text)"
                ></div>
              </div>
            </div>
          </div>

          <!-- Card Track nằm DƯỚI Text -->
          <div
            class="timeline-track card-track"
            v-if="draft.cardEnabled"
            draggable="true"
            @dragstart="onTrackDragStart($event, 'card')"
            @dragover="onTrackDragOver"
            @drop="onTrackDrop($event, 'card')"
          >
            <div class="track-head draggable-head" title="Nắm giữ và kéo lên/xuống để hoán đổi thứ tự lớp với Text">
              <GripVertical :size="13" class="drag-grip-icon" /> Nền C1
            </div>
            <div class="track-content card-track-content">
              <div
                class="track-block card-block"
                :style="cardBlockStyle"
                title="Khung Nền Card"
                @pointerdown.stop="onBlockPointerDown('card', 'move', $event)"
                @pointermove="onBlockPointerMove"
                @pointerup="onBlockPointerUp"
                @pointercancel="onBlockPointerUp"
              >
                <div
                  class="block-handle left"
                  title="Kéo để chỉnh thời gian Bắt đầu"
                  @pointerdown.stop="onBlockPointerDown('card', 'resize-left', $event)"
                ></div>
                <span class="block-text-label">Khung Nền Card / Banner</span>
                <span class="block-time-range">{{ formatTimelineMark(draft.cardStartTime) }} - {{ draft.cardEndTime > 0 ? formatTimelineMark(draft.cardEndTime) : 'hết' }}</span>
                <div
                  class="block-handle right"
                  title="Kéo để chỉnh thời gian Kết thúc"
                  @pointerdown.stop="onBlockPointerDown('card', 'resize-right', $event)"
                ></div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped src="./RemixScenarioPage.scoped.css"></style>

