<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, reactive, computed, watch } from 'vue'
import { Plus, Trash2, Save, Copy, Music, Image as ImageIcon, FileText, Layers, SlidersHorizontal, Film, Check, Wand2, Palette, Volume2, Type, Sparkles, Play, Pause, RotateCcw, Video, Folder, X } from 'lucide-vue-next'
import { SelectImageFile, SelectAudioFile, SelectFolder, GetStreamURL } from '../../wailsjs/go/main/App'

const props = defineProps<{
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void
  activeVideoSrc?: string
  outputDir?: string
  isExporting?: boolean
  exportProgress?: { done: number; total: number }
  exportStatusText?: string
  etaText?: string
}>()

const emit = defineEmits<{
  (e: 'back'): void
  (e: 'export'): void
  (e: 'update:outputDir', dir: string): void
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

type DragTarget = 'watermark' | 'subtitle' | `text:${string}` | null

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
  texts: [] as DraftText[],
  randomText: false,
})

const selectedTextId = ref('')
const playerFrameRef = ref<HTMLElement | null>(null)
const dragTarget = ref<DragTarget>(null)
const isTimelineDragging = ref(false)
let timelineRAF = 0
let pendingTimelineClientX: number | null = null

const clamp01 = (value: number) => Math.min(1, Math.max(0, Number.isFinite(value) ? value : 0))
const coordExpr = (axis: 'x' | 'y', value: number, kind: 'watermark' | 'text') => {
  const v = clamp01(value).toFixed(4)
  if (kind === 'watermark') return axis === 'x' ? `(W-w)*${v}` : `(H-h)*${v}`
  return axis === 'x' ? `(w-text_w)*${v}` : `(h-text_h)*${v}`
}

const parseNormalizedExpr = (value: unknown, axis: 'x' | 'y', kind: 'watermark' | 'text', fallback: number) => {
  const raw = String(value || '').replace(/\s+/g, '')
  const base = kind === 'watermark'
    ? (axis === 'x' ? '\\(W-w\\)' : '\\(H-h\\)')
    : (axis === 'x' ? '\\(w-text_w\\)' : '\\(h-text_h\\)')
  const match = raw.match(new RegExp(`^${base}\\*([0-9.]+)$`, 'i'))
  if (match) return clamp01(Number(match[1]))

  if (kind === 'watermark') {
    if (raw === '(W-w)/2' || raw === '(H-h)/2') return 0.5
    if (raw === '20') return 0.03
    if (raw.includes('W-w-20') || raw.includes('H-h-20')) return 0.97
  } else {
    if (raw.includes('/2')) return 0.5
    if (axis === 'y' && /^\d+(\.\d+)?$/.test(raw)) return 0.08
    if (axis === 'y' && raw.includes('h-text_h-')) return 0.9
  }
  return fallback
}

const selectedText = computed(() => draft.texts.find(t => t.id === selectedTextId.value) || null)
const visibleTexts = computed(() => draft.texts.filter(t => {
  if (t.selected === false) return false
  const end = t.endTime > 0 ? t.endTime : Number.POSITIVE_INFINITY
  return t.content.trim() && currentSec.value >= Math.max(0, t.startTime) && currentSec.value <= end
}))

const selectedTextsList = computed(() => draft.texts.filter(t => t.selected !== false))
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
  const copy = { ...text, id: `text-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`, x: clamp01(text.x + 0.04), y: clamp01(text.y + 0.04), selected: true }
  draft.texts.push(copy)
  selectedTextId.value = copy.id
}

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
  window.dispatchEvent(new Event('remix_scenarios_updated'))
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

// Tua video theo % vị trí click trên timeline. Đồng bộ lớp nền blur (nếu có) theo cùng mốc.
// Bề rộng cột nhãn track (🎬 Video V1...) bên trái — vùng KHÔNG tua được. Phải khớp
// .track-head width trong CSS (84px) để playhead + click quy đổi cùng một mốc gốc.
const TRACK_HEAD_W = 84

const tracksRef = ref<HTMLElement | null>(null)
let isVideoSeeking = false
let pendingSeekTime: number | null = null

const performVideoSeek = (t: number) => {
  if (!videoRef.value) return
  if (isVideoSeeking) {
    pendingSeekTime = t
    return
  }
  isVideoSeeking = true
  pendingSeekTime = null

  const v = videoRef.value
  const bg = bgVideoRef.value

  const onSeeked = () => {
    v.removeEventListener('seeked', onSeeked)
    isVideoSeeking = false
    if (pendingSeekTime !== null) {
      const nextTime = pendingSeekTime
      pendingSeekTime = null
      performVideoSeek(nextTime)
    }
  }

  v.addEventListener('seeked', onSeeked, { once: true })
  
  if ('fastSeek' in v && typeof (v as any).fastSeek === 'function') {
    try {
      (v as any).fastSeek(t)
    } catch (e) {
      v.currentTime = t
    }
  } else {
    v.currentTime = t
  }
  if (bg) {
    try { bg.currentTime = t } catch (e) {}
  }
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
  const frame = playerFrameRef.value.getBoundingClientRect()
  const left = e.clientX - frame.left - dragOffsetX
  const top = e.clientY - frame.top - dragOffsetY
  if (dragTarget.value === 'watermark' || dragTarget.value.startsWith('text:')) {
    const usableW = Math.max(1, frame.width - dragElementW)
    const usableH = Math.max(1, frame.height - dragElementH)
    const x = left / usableW
    const y = top / usableH
    if (dragTarget.value === 'watermark') setWatermarkPosition(x, y)
    else {
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

  if (selectedTextId.value) {
    onOverlayWheel(e, `text:${selectedTextId.value}`)
    return
  }
  if (dragTarget.value) {
    onOverlayWheel(e, dragTarget.value)
    return
  }
  if (draft.wmEnabled) {
    onOverlayWheel(e, 'watermark')
    return
  }
  if (draft.subEnabled || draft.subAutoGen) {
    onOverlayWheel(e, 'subtitle')
    return
  }
}

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
  return {
    transform: videoTransform.value,
    filter: colorFilterStyle.value,
    objectFit: videoObjectFit.value as any
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
  zIndex: '7',
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
    wmEnabled: false, wmPath: '', wmScale: 0.2, wmOpacity: 1.0, wmX: 0.97, wmY: 0.97,
    muteOriginal: false, musicPath: '', musicVolume: 0.3, musicLoop: false,
    subEnabled: false, subPath: '', subFontSize: 24, subFontColor: '#ffffff', subOutlineColor: '#000000', subMarginV: 40,
    subX: 0.5, subY: 0.9, subHasCustomPosition: false,
    subAutoGen: false, subSourceLang: 'auto', subTargetLang: '', texts: [],
    randomText: false,
  })
  selectedTextId.value = ''
}

const draftToEdit = (): any => ({
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
  randomText: draft.randomText,
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
  watermark: {
    enabled: draft.wmEnabled,
    imgPath: draft.wmPath,
    x: coordExpr('x', draft.wmX, 'watermark'),
    y: coordExpr('y', draft.wmY, 'watermark'),
    scale: draft.wmScale,
    opacity: draft.wmOpacity,
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
  }
  if (e.audio) { draft.muteOriginal = !!e.audio.mute; draft.musicPath = e.audio.musicPath || (e.audio.musicTracks && e.audio.musicTracks[0]) || ''; draft.musicVolume = e.audio.musicVolume ?? 0.3; draft.musicLoop = !!e.audio.musicLoop }
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
  selectedTextId.value = draft.texts[0]?.id || ''
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

const exportCurrentScenario = () => {
  saveScenario(true)
  emit('export')
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

const fileName = (p: string) => p ? (p.split('\\').pop() || p) : ''
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
            <option v-for="s in scenarios" :key="s.id" :value="s.id">
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
          <span style="font-size: 11px; color: var(--wx-text-secondary); white-space: nowrap;">Thư mục lưu:</span>
          <input
            type="text"
            :value="outputDir || 'D:\\Output'"
            @change="emit('update:outputDir', ($event.target as HTMLInputElement).value)"
            style="background: transparent; border: none; color: var(--wx-text-primary); font-size: 11px; font-family: monospace; width: 140px; outline: none; white-space: nowrap; overflow: hidden; text-overflow: ellipsis;"
          />
          <button
            type="button"
            @click="pickOutputDir"
            title="Chọn thư mục lưu xuất video"
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
        <button
          class="btn capcut-export-now-btn"
          :disabled="isExporting"
          @click="exportCurrentScenario"
          :title="isExporting ? 'Đang tiến hành xuất video...' : 'Lưu kịch bản và tiến hành xuất video ngay'"
          :style="isExporting ? 'opacity: 0.9; cursor: not-allowed; background: linear-gradient(135deg, #6366f1, #a855f7);' : ''"
        >
          <template v-if="isExporting">
            <div style="width: 14px; height: 14px; border: 2px solid #fff; border-top-color: transparent; border-radius: 50%; animation: spin 0.8s linear infinite; margin-right: 6px; display: inline-block; vertical-align: middle;"></div>
            <span>Đang xuất {{ Math.round(((exportProgress?.done || 0) / (exportProgress?.total || 1)) * 100) }}%</span>
          </template>
          <template v-else>
            <Video :size="14" />
            <span>Lưu và Xuất Video</span>
          </template>
        </button>
      </div>
    </header>

    <!-- REAL-TIME EXPORT PROGRESS BANNER (Hiển thị nổi bật ngay trên tab Chỉnh Sửa khi đang xuất video) -->
    <div
      v-if="isExporting"
      style="background: linear-gradient(135deg, rgba(99, 102, 241, 0.25), rgba(168, 85, 247, 0.25)); border-bottom: 1.5px solid var(--wx-brand-primary); padding: 10px 20px; display: flex; align-items: center; justify-content: space-between; gap: 16px; box-shadow: 0 4px 16px rgba(0,0,0,0.3); z-index: 100;"
    >
      <div style="display: flex; align-items: center; gap: 14px; flex: 1; min-width: 0;">
        <div style="width: 22px; height: 22px; border: 3px solid #6366f1; border-top-color: transparent; border-radius: 50%; animation: spin 0.8s linear infinite; flex-shrink: 0;"></div>
        <div style="display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0;">
          <div style="display: flex; justify-content: space-between; align-items: center; font-size: 13px; font-weight: 700; color: #ffffff;">
            <span>🚀 Đang tiến hành xuất video: {{ exportProgress?.done || 0 }}/{{ exportProgress?.total || 1 }} clip</span>
            <span style="color: #a855f7; font-size: 15px; font-weight: 800;">
              {{ Math.round(((exportProgress?.done || 0) / (exportProgress?.total || 1)) * 100) }}%
            </span>
          </div>
          <!-- Progress Bar Track -->
          <div style="width: 100%; height: 8px; background: rgba(0,0,0,0.4); border-radius: 4px; overflow: hidden; border: 1px solid rgba(255,255,255,0.15);">
            <div
              :style="{ width: Math.round(((exportProgress?.done || 0) / (exportProgress?.total || 1)) * 100) + '%' }"
              style="height: 100%; background: linear-gradient(90deg, #6366f1, #a855f7); transition: width 0.3s ease; box-shadow: 0 0 10px #6366f1;"
            ></div>
          </div>
        </div>
      </div>
      <div style="display: flex; flex-direction: column; align-items: flex-end; font-size: 12px; font-weight: 600; color: rgba(255,255,255,0.9); flex-shrink: 0;">
        <span>⏱ Dự kiến xong: {{ etaText || 'Đang tính...' }}</span>
        <span style="font-size: 11px; opacity: 0.75; margin-top: 2px;">{{ exportStatusText || 'Đang xử lý...' }}</span>
      </div>
    </div>

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

            <div class="text-editor-section">
              <div class="text-section-head">
                <span class="text-section-title">Chèn text ({{ draft.texts.length }})</span>
                <div class="text-head-actions">
                  <button v-if="selectedTextId" class="btn-text-action deselect" @click="selectedTextId = ''" title="Bỏ chọn text (Hoặc bấm ESC)"><X :size="11" /> Bỏ chọn</button>
                  <button class="btn-text-action add" @click="addText"><Plus :size="11" /> Thêm chữ</button>
                </div>
              </div>

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
                </div>
                <div class="text-detail-footer">
                  <span class="drag-help-inline">Kéo text trực tiếp trên video. End=0 là hiện tới hết.</span>
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

            <!-- Video chính phát xem trực tiếp -->
            <video
              v-if="activeVideoSrc"
              ref="videoRef"
              :src="activeVideoSrc"
              class="real-capcut-video"
              :style="videoStyle"
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

            <!-- Logo watermark thật — kéo trực tiếp & Ctrl + Cuộn chuột để phóng to/thu nhỏ. -->
            <img
              v-if="draft.wmEnabled && wmStreamUrl"
              :src="wmStreamUrl"
              :style="wmStyle"
              class="draggable-overlay"
              alt="logo"
              @pointerdown="beginOverlayDrag('watermark', $event)"
              @pointermove="updateOverlayDrag"
              @pointerup="finishOverlayDrag"
              @pointercancel="cancelOverlayDrag"
              @lostpointercapture="cancelOverlayDrag"
              @wheel.prevent="onOverlayWheel($event, 'watermark')"
            />

            <!-- Phụ đề mẫu — kéo & Ctrl + Cuộn chuột để đổi cỡ chữ. -->
            <div
              v-if="draft.subEnabled || draft.subAutoGen"
              :style="subStyle"
              class="draggable-overlay subtitle-preview-overlay"
              @pointerdown="beginOverlayDrag('subtitle', $event)"
              @pointermove="updateOverlayDrag"
              @pointerup="finishOverlayDrag"
              @pointercancel="cancelOverlayDrag"
              @lostpointercapture="cancelOverlayDrag"
              @wheel.prevent="onOverlayWheel($event, 'subtitle')"
            >Phụ đề mẫu xem trước</div>

            <!-- Text overlay thật — kéo & Ctrl + Cuộn chuột để tăng/giảm cỡ chữ. -->
            <div
              v-for="text in visibleTexts"
              :key="text.id"
              :style="textStyle(text)"
              class="draggable-overlay text-preview-overlay"
              @pointerdown="beginOverlayDrag(`text:${text.id}`, $event)"
              @pointermove="updateOverlayDrag"
              @pointerup="finishOverlayDrag"
              @pointercancel="cancelOverlayDrag"
              @lostpointercapture="cancelOverlayDrag"
              @wheel.prevent="onOverlayWheel($event, `text:${text.id}`)"
            >{{ text.content }}</div>


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
            <li :class="{ active: draft.hflip }">Lật ngang video (Mirror)</li>
            <li :class="{ active: draft.speed !== 1.0 }">Tốc độ phát: {{ draft.speed }}x</li>
            <li :class="{ active: draft.zoomEnabled }">Zoom Ken Burns: {{ draft.zoomFactor }}x</li>
            <li :class="{ active: draft.cropEnabled }">Cắt rìa Crop: {{ Math.round(draft.cropPercent * 100) }}%</li>
            <li :class="{ active: draft.rotateEnabled }">Xoay nghiêng: {{ draft.rotateDegrees }}°</li>
            <li :class="{ active: draft.trimStart > 0 || draft.trimEnd > 0 }">Cắt đầu {{ draft.trimStart }}s / đuôi {{ draft.trimEnd }}s</li>
            <li :class="{ active: draft.aspectEnabled }">Khung {{ draft.aspectRatio }} ({{ draft.aspectMode }})</li>
            <li :class="{ active: draft.colorEnabled }">Màu sắc: {{ draft.colorPreset ? draft.colorPreset.toUpperCase() : 'Chỉnh màu' }}</li>
            <li :class="{ active: draft.noiseEnabled }">Hạt nhiễu Noise (Mức {{ draft.noiseStrength }})</li>
            <li :class="{ active: draft.wmEnabled && !!draft.wmPath }">Logo Watermark (Cỡ {{ Math.round(draft.wmScale * 100) }}%)</li>
            <li :class="{ active: !!draft.musicPath }">Nhạc nền MP3</li>
            <li :class="{ active: draft.muteOriginal }">Tắt tiếng video gốc</li>
            <li :class="{ active: draft.subEnabled && !!draft.subPath }">Ghép phụ đề file (.SRT / .ASS)</li>
            <li :class="{ active: draft.subAutoGen }">Whisper AI tự tạo phụ đề</li>
            <li :class="{ active: selectedTextsCount > 0 }">Chèn chữ tùy chọn (Đã tích {{ selectedTextsCount }}/{{ draft.texts.length }} câu{{ selectedTextsCount > 1 ? ' · Random 1 chữ/clip' : '' }})</li>
            <li :class="{ active: draft.stripMeta }">Xóa Metadata chống quét</li>
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

        <!-- Track 4: Text overlays -->
        <div class="timeline-track text-track" v-if="selectedTextsList.length">
          <div class="track-head">Text T1</div>
          <div class="track-content text-track-content">
            <button
              v-for="text in selectedTextsList"
              :key="text.id"
              type="button"
              class="track-block text-block"
              :class="{ selected: selectedTextId === text.id }"
              :style="textBlockStyle(text)"
              :title="`${text.content} · ${formatTimelineMark(text.startTime)} → ${text.endTime > 0 ? formatTimelineMark(text.endTime) : 'hết video'}`"
              @pointerdown="selectedTextId = text.id"
            >
              <span>{{ text.content || 'Text' }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.capcut-studio-page {
  background: var(--wx-surface-base, #0f172a);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
  border-radius: var(--wx-radius-lg, 12px);
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  overflow: hidden;
  box-shadow: var(--wx-shadow-md);
  color: var(--wx-text-primary, #f8fafc);
  font-family: var(--wx-font-primary, 'Nunito', -apple-system, BlinkMacSystemFont, sans-serif);
}

/* Header */
.capcut-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 16px;
  background: var(--wx-surface-sunken, #090d16);
  border-bottom: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
  gap: 12px;
  flex-shrink: 0;
}

.capcut-header-left {
  display: flex;
  align-items: center;
  gap: 10px;
}

.capcut-brand-icon {
  width: 32px;
  height: 32px;
  border-radius: var(--wx-radius-md, 8px);
  background: linear-gradient(135deg, var(--wx-brand-primary, #2563eb), var(--wx-brand-accent, #06b6d4));
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.35);
}

.capcut-title {
  margin: 0;
  font-size: 14px;
  font-weight: 800;
  font-family: var(--wx-font-display, 'Nunito', sans-serif);
  color: var(--wx-text-primary, #ffffff);
  display: flex;
  flex-direction: column;
  line-height: 1.25;
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
  color: var(--wx-brand-accent, #06b6d4);
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
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  border-radius: var(--wx-radius-md, 8px);
  color: var(--wx-text-secondary, #94a3b8);
  cursor: pointer;
  transition: all var(--wx-d-fast, 0.15s) var(--wx-ease-standard, ease);
}

.preset-chip:hover {
  border-color: var(--wx-brand-primary, #2563eb);
  color: var(--wx-text-primary, #ffffff);
  background: rgba(37, 99, 235, 0.1);
}

.preset-chip--active {
  background: var(--wx-brand-primary, #2563eb);
  border-color: var(--wx-brand-primary, #2563eb);
  color: #ffffff;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.4);
}

.capcut-preset-select {
  height: 28px;
  padding: 0 10px;
  font-size: 11.5px;
  font-weight: 700;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-brand-accent, #06b6d4);
  border-radius: var(--wx-radius-md, 8px);
  color: var(--wx-text-primary, #ffffff);
  outline: none;
  cursor: pointer;
  max-width: 220px;
  transition: all 0.15s ease;
}

.capcut-preset-select:hover {
  border-color: var(--wx-brand-primary, #2563eb);
  box-shadow: 0 0 8px rgba(6, 182, 212, 0.3);
}

.capcut-preset-select option {
  background: var(--wx-surface-base, #0f172a);
  color: #ffffff;
  padding: 6px;
}

.capcut-add-chip-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 10px;
  font-size: 11px;
  font-weight: 600;
  background: rgba(6, 182, 212, 0.1);
  border: 1px solid rgba(6, 182, 212, 0.3);
  color: var(--wx-brand-accent, #06b6d4);
  border-radius: var(--wx-radius-md, 8px);
  cursor: pointer;
  transition: all 0.15s ease;
}

.capcut-add-chip-btn:hover {
  background: var(--wx-brand-accent, #06b6d4);
  border-color: var(--wx-brand-accent, #06b6d4);
  color: #ffffff;
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
  border-radius: var(--wx-radius-md, 8px);
  border: none;
  background: linear-gradient(135deg, var(--wx-success-solid, #10b981), #059669);
  color: #ffffff;
  cursor: pointer;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.35);
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.capcut-save-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 14px rgba(16, 185, 129, 0.5);
}

.capcut-sec-btn, .capcut-danger-btn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  height: 32px;
  padding: 0 10px;
  font-size: 11.5px;
  font-weight: 600;
  border-radius: var(--wx-radius-md, 8px);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  background: var(--wx-surface-sunken, #0e1626);
  color: var(--wx-text-secondary, #cbd5e1);
  cursor: pointer;
  transition: all 0.15s ease;
}

.capcut-sec-btn:hover {
  border-color: var(--wx-brand-accent, #06b6d4);
  color: var(--wx-brand-accent, #06b6d4);
  background: rgba(6, 182, 212, 0.08);
}

.capcut-danger-btn:hover {
  border-color: var(--wx-danger-solid, #ef4444);
  color: var(--wx-danger-solid, #ef4444);
  background: rgba(239, 68, 68, 0.1);
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
  background: var(--wx-surface-base, #0f172a);
  border-right: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
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
  font-family: var(--wx-font-display, 'Nunito', sans-serif);
  color: var(--wx-brand-accent, #06b6d4);
  text-transform: uppercase;
  letter-spacing: 0.02em;
}

.category-nav-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-subtle, rgba(255, 255, 255, 0.05));
  padding: 4px;
  border-radius: var(--wx-radius-lg, 12px);
}

.cat-nav-btn {
  flex: 1 1 45%;
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 10px;
  font-size: 11px;
  font-weight: 600;
  border: none;
  background: transparent;
  color: var(--wx-text-muted, #94a3b8);
  border-radius: var(--wx-radius-md, 8px);
  cursor: pointer;
  transition: all 0.15s ease;
}

.cat-nav-btn:hover {
  color: var(--wx-text-primary, #ffffff);
  background: rgba(255, 255, 255, 0.04);
}

.cat-nav-btn.active {
  background: linear-gradient(135deg, var(--wx-brand-primary, #2563eb), var(--wx-brand-accent, #06b6d4));
  color: #ffffff;
  font-weight: 700;
  box-shadow: 0 2px 8px rgba(37, 99, 235, 0.35);
}

.scenario-block-card {
  background: var(--wx-glass-light-bg, rgba(30, 41, 59, 0.6));
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
  border-radius: var(--wx-radius-lg, 12px);
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  backdrop-filter: blur(12px);
}

.block-heading {
  margin: 0 0 4px;
  font-size: 12.5px;
  font-weight: 700;
  font-family: var(--wx-font-display, 'Nunito', sans-serif);
  color: var(--wx-brand-accent, #06b6d4);
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
  width: 34px;
  height: 18px;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.12));
  border-radius: var(--wx-radius-full, 9999px);
  position: relative;
  transition: all 0.2s ease;
}

.switch-slider::after {
  content: '';
  width: 14px;
  height: 14px;
  background: #ffffff;
  border-radius: 50%;
  position: absolute;
  top: 1px;
  left: 1px;
  transition: transform 0.2s ease;
  box-shadow: 0 1px 3px rgba(0,0,0,0.4);
}

.toggle-switch-lbl input:checked + .switch-slider {
  background: var(--wx-brand-primary, #2563eb);
  border-color: var(--wx-brand-primary, #2563eb);
  box-shadow: 0 0 8px rgba(37, 99, 235, 0.4);
}

.toggle-switch-lbl input:checked + .switch-slider::after {
  transform: translateX(16px);
}

.lbl-txt {
  font-size: 11.5px;
  font-weight: 600;
  color: var(--wx-text-primary, #ffffff);
  white-space: nowrap;
}

.custom-range-slider {
  width: 100%;
  accent-color: var(--wx-brand-primary, #2563eb);
  cursor: pointer;
}

.quick-speed-pills {
  display: flex;
  gap: 4px;
}

.sp-pill {
  padding: 2px 8px;
  font-size: 10.5px;
  font-weight: 600;
  border-radius: var(--wx-radius-sm, 4px);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  background: var(--wx-surface-sunken, #0e1626);
  color: var(--wx-text-secondary, #94a3b8);
  cursor: pointer;
  transition: all 0.15s ease;
}

.sp-pill.active {
  background: var(--wx-brand-primary, #2563eb);
  border-color: var(--wx-brand-primary, #2563eb);
  color: #ffffff;
}

.asp-pill, .c-chip {
  padding: 3px 6px;
  font-size: 10.5px;
  font-weight: 600;
  border-radius: var(--wx-radius-sm, 4px);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  background: var(--wx-surface-sunken, #0e1626);
  color: var(--wx-text-secondary, #94a3b8);
  cursor: pointer;
  transition: all 0.15s ease;
  white-space: nowrap;
}

.asp-pill {
  flex: 1;
  text-align: center;
}

.asp-pill.active, .c-chip.active {
  background: linear-gradient(135deg, var(--wx-brand-primary, #2563eb), var(--wx-brand-accent, #06b6d4));
  border-color: transparent;
  color: #ffffff;
  font-weight: 700;
}

.aspect-pills-row {
  display: flex;
  gap: 4px;
  width: 100%;
}

.preset-color-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.sub-fields-group {
  margin-top: 6px;
  padding: 8px;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-subtle, rgba(255, 255, 255, 0.05));
  border-radius: var(--wx-radius-md, 8px);
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.inline-field-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.inline-field-row > span {
  white-space: nowrap;
  font-size: 11px;
  font-weight: 600;
  color: var(--wx-text-primary, #ffffff);
  flex-shrink: 0;
}

.slider-field-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.slider-info-row {
  font-size: 10.5px;
  color: var(--wx-text-secondary, #cbd5e1);
}

.scenario-select {
  height: 28px;
  padding: 0 8px;
  font-size: 11.5px;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  border-radius: var(--wx-radius-md, 8px);
  color: var(--wx-text-primary, #ffffff);
  outline: none;
}

.scenario-input-num {
  width: 60px;
  height: 26px;
  padding: 0 6px;
  font-size: 11.5px;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  border-radius: var(--wx-radius-md, 8px);
  color: var(--wx-text-primary, #ffffff);
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
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  border-radius: var(--wx-radius-md, 8px);
  color: var(--wx-text-primary, #ffffff);
  cursor: pointer;
  transition: all 0.15s ease;
}

.btn-picker:hover {
  border-color: var(--wx-brand-accent, #06b6d4);
  color: var(--wx-brand-accent, #06b6d4);
}

.file-name-hint {
  font-size: 10.5px;
  color: var(--wx-text-muted, #94a3b8);
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

.pos-action-group {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin-top: 5px;
}

.pos-grid-3 {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 5px;
}

.pos-grid-2 {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 5px;
}

.pos-grid-4 {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 4px;
  margin-top: 4px;
}

.pos-mini-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  height: 27px;
  padding: 0 6px;
  font-size: 10.5px;
  font-weight: 700;
  box-sizing: border-box;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.12));
  border-radius: var(--wx-radius-sm, 4px);
  color: var(--wx-text-secondary, #cbd5e1);
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.pos-mini-btn:hover {
  border-color: var(--wx-brand-accent, #06b6d4);
  color: #ffffff;
  background: rgba(6, 182, 212, 0.12);
}

.pos-mini-btn.active {
  background: var(--wx-brand-primary, #2563eb);
  border-color: var(--wx-brand-primary, #2563eb);
  color: #ffffff;
}

/* Center Panel (Player Monitor) */
.capcut-center-panel {
  flex: 1;
  background: var(--wx-surface-sunken, #090d16);
  display: flex;
  flex-direction: column;
  user-select: none;
}

.player-monitor-header {
  height: 32px;
  padding: 0 12px;
  background: var(--wx-surface-base, #0f172a);
  border-bottom: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.monitor-title {
  font-size: 11.5px;
  font-weight: 700;
  font-family: var(--wx-font-display, 'Nunito', sans-serif);
  color: var(--wx-text-secondary, #cbd5e1);
}

.monitor-ratio-badge {
  font-size: 10.5px;
  font-weight: 700;
  background: var(--wx-brand-primary, #2563eb);
  color: #ffffff;
  padding: 2px 8px;
  border-radius: var(--wx-radius-sm, 4px);
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
  border: 1.5px solid var(--wx-border-default, rgba(255, 255, 255, 0.12));
  border-radius: var(--wx-radius-lg, 12px);
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: var(--wx-shadow-lg, 0 10px 25px rgba(0,0,0,0.5));
  container-type: size;
}

.real-capcut-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
  transition: transform 0.2s ease, filter 0.2s ease;
  position: relative;
  z-index: 2;
}

/* Nền blur phủ full khung cho aspect mode 'blur' (mô phỏng gblur của ffmpeg). */
.capcut-blur-bg {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  object-fit: cover;
  filter: blur(20px) brightness(0.7);
  transform: scale(1.1);
  z-index: 1;
  pointer-events: none;
}

.video-preview-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  color: var(--wx-text-muted, #64748b);
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
  background: rgba(15, 23, 42, 0.85);
  color: var(--wx-brand-accent, #06b6d4);
  border: 1px solid rgba(6, 182, 212, 0.3);
  padding: 2px 8px;
  border-radius: var(--wx-radius-sm, 4px);
  backdrop-filter: blur(6px);
}

.player-transport-bar {
  height: 40px;
  background: var(--wx-surface-base, #0f172a);
  border-top: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
}

.timecode {
  font-size: 11px;
  font-family: var(--wx-font-mono, monospace);
  color: var(--wx-brand-accent, #06b6d4);
  font-weight: 700;
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
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  background: var(--wx-surface-sunken, #0e1626);
  color: var(--wx-text-primary, #ffffff);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.15s ease;
}

.transport-btn:hover {
  border-color: var(--wx-brand-primary, #2563eb);
  transform: scale(1.05);
}

.transport-btn.play {
  width: 34px;
  height: 34px;
  background: linear-gradient(135deg, var(--wx-brand-primary, #2563eb), var(--wx-brand-accent, #06b6d4));
  border-color: transparent;
  box-shadow: 0 0 12px rgba(37, 99, 235, 0.5);
}

.aspect-tag {
  font-size: 11px;
  font-weight: 600;
  color: var(--wx-text-muted, #94a3b8);
}

/* Right Panel */
.capcut-right-panel {
  width: 240px;
  background: var(--wx-surface-base, #0f172a);
  border-left: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
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
  color: var(--wx-text-muted, #94a3b8);
}

.inspector-input {
  width: 100%;
  height: 32px;
  padding: 0 10px;
  font-size: 12px;
  font-weight: 700;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.1));
  border-radius: var(--wx-radius-md, 8px);
  color: var(--wx-text-primary, #ffffff);
  outline: none;
  box-sizing: border-box;
  transition: all 0.15s ease;
}

.inspector-input:focus {
  border-color: var(--wx-brand-primary, #2563eb);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.25);
}

.inspector-summary-card {
  background: var(--wx-glass-light-bg, rgba(30, 41, 59, 0.6));
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
  border-radius: var(--wx-radius-lg, 12px);
  padding: 10px;
  backdrop-filter: blur(8px);
}

.summary-heading {
  margin: 0 0 8px;
  font-size: 11.5px;
  font-weight: 700;
  font-family: var(--wx-font-display, 'Nunito', sans-serif);
  color: var(--wx-brand-accent, #06b6d4);
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
  color: var(--wx-text-muted, #64748b);
  display: flex;
  align-items: center;
  gap: 4px;
}

.summary-checklist li.active {
  color: var(--wx-success-solid, #10b981);
  font-weight: 600;
}

.capcut-save-btn-large {
  margin-top: auto;
  height: 40px;
  font-size: 12.5px;
  font-weight: 800;
  font-family: var(--wx-font-display, 'Nunito', sans-serif);
  background: linear-gradient(135deg, var(--wx-brand-primary, #2563eb), var(--wx-brand-accent, #06b6d4));
  color: #ffffff;
  border: none;
  border-radius: var(--wx-radius-lg, 12px);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  cursor: pointer;
  box-shadow: 0 4px 14px rgba(37, 99, 235, 0.35);
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.capcut-save-btn-large:hover {
  transform: translateY(-1px);
  box-shadow: 0 6px 20px rgba(37, 99, 235, 0.5);
}

.drag-help {
  margin: 5px 0 0;
  color: var(--wx-text-muted, #94a3b8);
  font-size: 10px;
  line-height: 1.35;
}

.editor-compact-row {
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
}

.editor-compact-row label,
.check-inline {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: var(--wx-text-secondary, #cbd5e1);
  font-size: 10.5px;
}

.color-input {
  width: 28px;
  height: 24px;
  padding: 1px;
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.12));
  border-radius: 4px;
  background: var(--wx-surface-sunken, #0e1626);
  cursor: pointer;
}

.auto-gen-box {
  padding: 8px;
  border-radius: 6px;
  background: rgba(37, 99, 235, 0.1);
}

.auto-gen-box .auto-gen-hint {
  margin: 0 0 6px;
  color: #38bdf8;
  font-size: 11px;
}

.text-editor-section {
  margin-top: 12px;
  padding-top: 10px;
  border-top: 1px dashed var(--wx-border-default, rgba(255, 255, 255, 0.12));
}

.text-section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  color: var(--wx-text-primary, #f8fafc);
  margin-bottom: 8px;
}

.text-section-title {
  font-size: 11px;
  font-weight: 700;
  color: var(--wx-text-primary, #f8fafc);
}

.text-head-actions {
  display: flex;
  align-items: center;
  gap: 4px;
}

.btn-text-action {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  padding: 3px 8px;
  font-size: 10.5px;
  font-weight: 700;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
}

.btn-text-action.add {
  color: #ffffff;
  background: var(--wx-brand-primary, #2563eb);
  border: none;
}

.btn-text-action.add:hover {
  background: #1d4ed8;
}

.btn-text-action.deselect {
  color: var(--wx-text-muted, #94a3b8);
  background: rgba(255, 255, 255, 0.06);
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.12));
}

.btn-text-action.deselect:hover {
  color: #f8fafc;
  background: rgba(255, 255, 255, 0.12);
}

.text-empty {
  margin-top: 7px;
  padding: 8px;
  border: 1px dashed var(--wx-border-default, rgba(255, 255, 255, 0.12));
  border-radius: 6px;
  color: var(--wx-text-muted, #94a3b8);
  font-size: 10.5px;
  text-align: center;
}

.text-chip-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 7px;
}

.text-select-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 11px;
  font-size: 10.5px;
  font-weight: 600;
  border-radius: 16px;
  background: var(--wx-surface-sunken, #0e1626);
  border: 1.5px solid var(--wx-border-default, rgba(255, 255, 255, 0.12));
  color: var(--wx-text-muted, #94a3b8);
  cursor: pointer;
  transition: all 0.18s ease;
  user-select: none;
}

.text-select-chip:hover {
  border-color: rgba(6, 182, 212, 0.4);
  color: #f8fafc;
}

.text-select-chip.checked {
  border-color: var(--wx-brand-accent, #06b6d4);
  color: #ffffff;
  background: rgba(6, 182, 212, 0.12);
  box-shadow: 0 0 8px rgba(6, 182, 212, 0.2);
}

.text-select-chip.active {
  border-color: var(--wx-brand-primary, #2563eb);
  box-shadow: 0 0 0 2px rgba(37, 99, 235, 0.4);
}

.chip-check-indicator {
  font-size: 10px;
  font-weight: 800;
  color: var(--wx-brand-accent, #06b6d4);
}

.text-detail-card {
  display: flex;
  flex-direction: column;
  gap: 7px;
  margin-top: 7px;
  padding: 8px;
  border: 1px solid rgba(6, 182, 212, 0.24);
  border-radius: 7px;
  background: rgba(6, 182, 212, 0.06);
}

.text-detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-bottom: 4px;
  border-bottom: 1px solid rgba(6, 182, 212, 0.18);
}

.text-detail-title {
  font-size: 11px;
  font-weight: 700;
  color: var(--wx-brand-accent, #06b6d4);
}

.btn-done-text, .btn-hide-card {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  font-size: 10.5px;
  font-weight: 700;
  color: #10b981;
  background: rgba(16, 185, 129, 0.15);
  border: 1px solid rgba(16, 185, 129, 0.35);
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.btn-done-text:hover, .btn-hide-card:hover {
  background: rgba(16, 185, 129, 0.3);
  color: #ffffff;
}

.text-detail-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-top: 4px;
  gap: 6px;
}

.drag-help-inline {
  font-size: 10px;
  color: var(--wx-text-muted, #64748b);
}

.scenario-textarea {
  width: 100%;
  min-height: 48px;
  padding: 6px 8px;
  box-sizing: border-box;
  resize: vertical;
  border: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.12));
  border-radius: 5px;
  outline: none;
  background: var(--wx-surface-sunken, #0e1626);
  color: var(--wx-text-primary, #f8fafc);
  font: inherit;
  font-size: 11px;
}

.scenario-textarea:focus {
  border-color: var(--wx-brand-accent, #06b6d4);
}

.pos-mini-btn.danger {
  color: #fca5a5;
  border-color: rgba(239, 68, 68, 0.35);
}

.draggable-overlay {
  user-select: none;
  -webkit-user-drag: none;
}

.player-monitor-frame.is-overlay-dragging {
  outline: 1px solid rgba(56, 189, 248, 0.65);
  outline-offset: -1px;
}

.overlay-badge-strip {
  z-index: 9;
  pointer-events: none;
}

/* Timeline */
.capcut-timeline-bar {
  min-height: 95px;
  height: auto;
  max-height: 155px;
  background: var(--wx-surface-sunken, #090d16);
  border-top: 1px solid var(--wx-border-default, rgba(255, 255, 255, 0.08));
  display: flex;
  flex-direction: column;
  user-select: none;
  flex-shrink: 0;
}

.timeline-ruler {
  position: relative;
  height: 18px;
  flex: 0 0 18px;
  background: var(--wx-surface-base, #0f172a);
  border-bottom: 1px solid var(--wx-border-subtle, rgba(255, 255, 255, 0.05));
  overflow: hidden;
}

.ruler-mark {
  position: absolute;
  top: 3px;
  transform: translateX(-50%);
  font-size: 9.5px;
  font-family: var(--wx-font-mono, monospace);
  color: var(--wx-text-muted, #64748b);
  white-space: nowrap;
  pointer-events: none;
}

.timeline-tracks {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 4px 0;
  position: relative;
  cursor: grab;
  touch-action: none;
  overflow-y: auto;
}

.timeline-tracks.dragging {
  cursor: grabbing;
}

.timeline-playhead {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 2px;
  background: #ef4444;
  box-shadow: 0 0 4px rgba(239, 68, 68, 0.8);
  pointer-events: none;
  z-index: 6;
  transition: left 80ms linear;
}

.timeline-playhead.dragging {
  transition: none;
}
.timeline-playhead::before {
  content: '';
  position: absolute;
  top: -2px;
  left: -3px;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #ef4444;
}

.timeline-track {
  display: flex;
  align-items: center;
  height: 22px;
}

.track-head {
  width: 84px;
  flex: 0 0 84px;
  padding-left: 4px;
  box-sizing: border-box;
  font-size: 10.5px;
  font-weight: 700;
  color: var(--wx-text-secondary, #94a3b8);
}

.track-content {
  position: relative;
  flex: 1;
  min-width: 0;
  height: 100%;
}

.track-block {
  height: 100%;
  box-sizing: border-box;
  border-radius: var(--wx-radius-sm, 4px);
  display: flex;
  align-items: center;
  padding: 0 8px;
  font-size: 10px;
  font-weight: 600;
  border: 1px solid rgba(255, 255, 255, 0.15);
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.3);
  overflow: hidden;
}

.track-block span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.text-track-content {
  overflow: hidden;
}

.text-block {
  position: absolute;
  top: 0;
  min-width: 12px;
  border: 1px solid rgba(192, 132, 252, 0.65);
  background: linear-gradient(90deg, #8b5cf6, #c026d3);
  color: #ffffff;
  cursor: grab;
}

.text-block.selected {
  border-color: #ffffff;
  box-shadow: 0 0 0 1px #38bdf8, 0 2px 8px rgba(139, 92, 246, 0.5);
}

.video-block { background: linear-gradient(90deg, var(--wx-brand-primary, #2563eb), var(--wx-brand-accent, #06b6d4)); color: #ffffff; width: 92%; }
.audio-block { background: linear-gradient(90deg, #10b981, #059669); color: #ffffff; width: 80%; }
.sub-block { background: linear-gradient(90deg, #f59e0b, #d97706); color: #ffffff; width: 70%; }

.capcut-export-now-btn {
  background: linear-gradient(135deg, #10b981, #059669);
  color: #ffffff;
  border: none;
  font-weight: 600;
  font-size: 12px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 0 14px;
  height: 32px;
  border-radius: 6px;
  cursor: pointer;
  box-shadow: 0 2px 8px rgba(16, 185, 129, 0.35);
  transition: all 0.2s ease;
}
.capcut-export-now-btn:hover {
  background: linear-gradient(135deg, #059669, #047857);
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.5);
}
</style>
