<script setup lang="ts">
import { ref, computed, onMounted, reactive, watch, onUnmounted } from 'vue'
import { GetVideoInfo, Analyze, ExportClips, SelectFiles, CancelAnalysis, GetStreamURL, GetDefaultConfig, GenerateThumbnail, SaveProject, LoadProjectBySource, ListProjects, DeleteProject, SelectImageFile, SelectAudioFile, SelectFolder, CancelExport, SaveGlobalSettings, GetGlobalSettings, ExtractClipFrames, GenerateAIThumbnail } from '../../wailsjs/go/main/App'
import { project, storage, main } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useTheme } from '../ui-system/composables/useTheme'
import BaseDropdown from './common/BaseDropdown.vue'
import VideoDownloader from './VideoDownloader.vue'
import ImageDownloader from './ImageDownloader.vue'
import BrowserAIImagePage from './BrowserAIImagePage.vue'
import BrowserAIVideoPage from './BrowserAIVideoPage.vue'
import {
  Video, Scissors, Download, Settings, Sun, Moon, History, Save,
  Plus, Trash2, Trash, RefreshCw, X, Check, Key, ChevronDown, ChevronUp,
  Play, Square, Pause, RotateCcw, Copy, FolderOpen, Music,
  Image as ImageIcon, Type, Layers, Zap, AlertTriangle, Info,
  Clock, Film, Monitor, Loader2, ArrowRight, Upload, BarChart2,
  Sparkles, Tag, FileVideo, ListVideo, LayoutGrid, SlidersHorizontal,
  Cpu, FlipHorizontal2, Timer, Volume2, VolumeX, Repeat,
  Star, Pencil, Move, Chrome
} from 'lucide-vue-next'

const { isDark, toggleColorScheme } = useTheme()

// === Toast Notifications (Hệ thống thông báo đẹp) ===
interface ToastMessage {
  id: string
  message: string
  type: 'success' | 'error' | 'info' | 'warning'
  duration?: number
}
const toasts = ref<ToastMessage[]>([])
const showToast = (message: string, type: 'success' | 'error' | 'info' | 'warning' = 'info', duration = 3500) => {
  const id = 'toast_' + Date.now() + '_' + Math.random().toString(36).substring(2, 9)
  toasts.value.push({ id, message, type, duration })
  setTimeout(() => {
    toasts.value = toasts.value.filter(t => t.id !== id)
  }, duration)
}

// === Google AI Chrome Integration ===
const showBrowserAIModal = ref(false)
const browserAIInitialMediaType = ref('image')

const handleApplyAIImage = (filePath: string) => {
  if (editingClip.value) {
    editingClip.value.thumbnail = filePath
    saveActiveProjectState()
    showToast("Đã áp dụng ảnh AI làm ảnh bìa cho clip này!", "success")
  } else {
    showToast(`Đã lưu ảnh AI thành công tại: ${filePath}`, "success")
  }
}

const handleApplyAIVideo = async (filePath: string) => {
  const existingPaths = new Set(videoPaths.value)
  if (!existingPaths.has(filePath)) {
    const wasEmpty = videoPaths.value.length === 0
    videoPaths.value = [...videoPaths.value, filePath]
    if (wasEmpty) {
      activeVideoIndex.value = 0
      activeVideoSrc.value = await GetStreamURL(filePath)
      await loadVideoInfo(filePath)
    }
    showToast('Đã thêm video AI thành công vào danh sách nguồn!', 'success')
  } else {
    showToast('Video này đã có sẵn trong danh sách nguồn!', 'warning')
  }
}

const openVideoAI = () => {
  browserAIInitialMediaType.value = 'video'
  showBrowserAIModal.value = true
}

// === Custom Confirm Dialog (Hộp thoại xác nhận đẹp) ===
const confirmDialogState = reactive({
  show: false,
  message: '',
  resolve: null as ((val: boolean) => void) | null
})
const showCustomConfirm = (message: string): Promise<boolean> => {
  confirmDialogState.message = message
  confirmDialogState.show = true
  return new Promise<boolean>((res) => {
    confirmDialogState.resolve = res
  })
}
const handleConfirmResolve = (val: boolean) => {
  if (confirmDialogState.resolve) {
    confirmDialogState.resolve(val)
  }
  confirmDialogState.show = false
  confirmDialogState.resolve = null
}

const videoPaths = ref<string[]>([])
const activeVideoIndex = ref<number>(0)
const isVideoListExpanded = ref(false)
const videoInfo = ref<project.VideoInfo | null>(null)
const clipsMap = ref<Record<string, project.Clip[]>>({})
const isAnalyzing = ref(false)
const isExporting = ref(false)
const isMultiExportRunning = ref(false)
const globalSettingsOutDir = ref('D:\\Output')
const outDir = ref('D:\\Output')
const outImageDir = ref('D:\\Output')
const autoCreateSubfolders = ref(true)
const exportWithThumbnails = ref(true)
const exportStatusText = ref('Đang chuẩn bị...')
const videoDoneCount = ref(0)
const thumbDoneCount = ref(0)

const activeAnalyzingPaths = ref<Set<string>>(new Set())
const analyzeProgressMap = ref<Record<string, number>>({})
const analyzeStartTimes = ref<Record<string, number>>({})
const exportStartTime = ref<number | null>(null)

const isPlayingExported = ref(false)
const activeVideoSrc = ref('')
const activeExportedSrc = ref('')
const currentPlayingClipIdx = ref(0)

const totalVideosCount = ref(0)
const processedVideosCount = ref(0)
const activeProcessingVideoName = ref('')
const streamPrefix = ref('')
const streamSuffix = ref('')
const clampValue = (val: number | null | undefined, min: number, max: number, fallback: number): number => {
  if (val === null || val === undefined || isNaN(val)) return fallback
  return Math.min(Math.max(val, min), max)
}
const isCancelled = ref(false)

// === Project (persistence GĐ3) ===
const recentProjects = ref<storage.ProjectSummary[]>([])
const showRecent = ref(false)

// === Named Projects Manager ===
interface NamedProject {
  id: string
  name: string
  videoPaths: string[]
  selectedVideos: string[]
  globalRemix: {
    autoApply: boolean
    hflip: boolean
    aspectEnabled: boolean
    aspectRatio: string
    aspectMode: string
    speed: number
    colorEnabled: boolean
    colorPreset: string
    colorBrightness: number
    colorContrast: number
    colorSaturation: number
    musicPath: string
    musicVolume: number
    muteOriginal: boolean
    musicLoop?: boolean
    musicTracks?: string[]
  }
  outDir: string
  exportJobs: number
  analyzeJobs: number
  createdAt: number
  analyzerConfig?: any
}

const namedProjects = ref<NamedProject[]>([])
const activeProjectId = ref<string>('')
const showProjectModal = ref(false)
const showManageProjectsModal = ref(false)
const newProjectName = ref('')

// === Edit clip (GĐ5): panel chỉnh tỉ lệ/màu/tốc độ ===
const showEdit = ref(false)
const editClipIdx = ref(-1)

const geminiAPIKey = ref('')
const browserAIShowChrome = ref(true)
const browserAIDelay = ref(1.0)

// === AI Thumbnail Generator state and functions ===
const aiThumbState = reactive({
  isExtractingFrames: false,
  extractedFrames: [] as string[],
  selectedFrames: new Set<string>(),
  userPrompt: '',
  aspectRatio: '9:16',
  isGenerating: false,
  generatedImage: '',
  statusText: ''
})

const getClipFrames = async () => {
  if (!editingClip.value) return
  aiThumbState.isExtractingFrames = true
  aiThumbState.statusText = 'Đang trích xuất ảnh mẫu...'
  try {
    const videoPath = activeVideoPath.value
    const startTime = editingClip.value.startTime
    const endTime = editingClip.value.endTime
    const frames = await ExtractClipFrames(videoPath, startTime, endTime)
    aiThumbState.extractedFrames = frames
    aiThumbState.selectedFrames = new Set(frames)
    aiThumbState.statusText = 'Đã trích xuất xong 3 ảnh mẫu.'
  } catch (err) {
    showToast('Lỗi trích xuất ảnh mẫu: ' + err, 'error')
    aiThumbState.statusText = 'Lỗi trích xuất ảnh mẫu.'
  } finally {
    aiThumbState.isExtractingFrames = false
  }
}

const generateAIThumbnailImg = async () => {
  if (!editingClip.value) return
  if (!geminiAPIKey.value) {
    showToast('Vui lòng cấu hình Gemini API Key trong phần Cài đặt chung.', 'warning')
    return
  }
  if (!aiThumbState.userPrompt.trim()) {
    showToast('Vui lòng nhập mô tả chủ đề cho thumbnail.', 'warning')
    return
  }

  aiThumbState.isGenerating = true
  aiThumbState.statusText = 'Đang gửi cho Gemini tối ưu hóa prompt...'
  try {
    const selectedFramesList = Array.from(aiThumbState.selectedFrames)
    const videoPath = activeVideoPath.value
    const clipIdx = editingClip.value.index
    const aspect = aiThumbState.aspectRatio

    aiThumbState.statusText = 'Đang sinh ảnh bằng Imagen 4 (Google AI)...'
    const editJSON = JSON.stringify(editingClip.value?.edit || {})
    const resultImgPath = await GenerateAIThumbnail(
      geminiAPIKey.value,
      aiThumbState.userPrompt,
      selectedFramesList,
      videoPath,
      clipIdx,
      aspect,
      editJSON
    )

    aiThumbState.generatedImage = resultImgPath
    // Áp dụng luôn làm thumbnail của clip
    editingClip.value.thumbnail = resultImgPath
    saveActiveProjectState()
    showToast('Tạo ảnh bìa AI thành công và đã áp dụng!', 'success')
    aiThumbState.statusText = 'Đã tạo ảnh bìa AI thành công!'
  } catch (err) {
    showToast('Lỗi tạo ảnh bìa AI: ' + err, 'error')
    aiThumbState.statusText = 'Lỗi tạo ảnh bìa AI.'
  } finally {
    aiThumbState.isGenerating = false
  }
}

// === Cấu hình dự án ===
const showAdvancedCutSettings = ref(false)
const analyzeJobs = ref(1)
const exportJobs = ref(2)

// Trạng thái hiển thị ở thanh đáy workspace.
const statusText = ref('Sẵn sàng')

// Chỉ cập nhật thanh trạng thái ở đáy workspace — không hiện toast popup nữa
// (người dùng thấy phiền vì các thông báo tiến trình nhảy lên liên tục).
const addLog = (text: string) => {
  statusText.value = text
}



// === Cấu hình Chỉnh sửa video ===
const globalRemix = reactive({
  autoApply: true,      // Tự động áp dụng khi cắt xong
  hflip: false,         // Lật ngang video
  aspectEnabled: false, // Bật đổi tỉ lệ
  aspectRatio: '9:16',  // Tỉ lệ
  aspectMode: 'blur',   // Cách lấp (blur/crop/pad)
  speed: 1.0,           // Tốc độ (ví dụ 1.02, 1.05)
  colorEnabled: false,  // Bật chỉnh màu
  colorPreset: '',      // Preset màu
  colorBrightness: 0,
  colorContrast: 0,
  colorSaturation: 1.0,
  musicPath: '',        // Nhạc nền (bài nhạc đầu hoặc đơn)
  musicVolume: 0.3,
  muteOriginal: false,  // Tắt tiếng gốc
  musicLoop: false,     // Tự động lặp lại nhạc nền nếu ngắn hơn video
  musicTracks: [] as string[] // Danh sách nhiều bài nhạc nền để ghép nối tiếp
})

// Chọn nhạc nền cho cấu hình chỉnh sửa
const pickGlobalMusic = async () => {
  try {
    const p = await SelectAudioFile()
    if (p) {
      if (!globalRemix.musicTracks) {
        globalRemix.musicTracks = []
      }
      globalRemix.musicTracks.push(p)
      // Cập nhật musicPath để tương thích với các logic cũ chỉ dùng 1 file
      globalRemix.musicPath = globalRemix.musicTracks[0]
      addLog('Đã thêm nhạc nền: ' + p.split('\\').pop())
    }
  } catch (e) {
    addLog('Lỗi chọn nhạc nền: ' + String(e))
  }
}

const clearGlobalMusic = () => {
  globalRemix.musicTracks = []
  globalRemix.musicPath = ''
  addLog('Đã bỏ tất cả nhạc nền.')
}

const removeMusicTrack = (idx: number) => {
  if (globalRemix.musicTracks) {
    globalRemix.musicTracks.splice(idx, 1)
    if (globalRemix.musicTracks.length === 0) {
      globalRemix.musicPath = ''
    } else {
      globalRemix.musicPath = globalRemix.musicTracks[0]
    }
    addLog('Đã xóa 1 đoạn nhạc nền.')
  }
}

const moveMusicTrack = (idx: number, direction: number) => {
  if (!globalRemix.musicTracks) return
  const targetIdx = idx + direction
  if (targetIdx < 0 || targetIdx >= globalRemix.musicTracks.length) return
  const temp = globalRemix.musicTracks[idx]
  globalRemix.musicTracks[idx] = globalRemix.musicTracks[targetIdx]
  globalRemix.musicTracks[targetIdx] = temp
  globalRemix.musicPath = globalRemix.musicTracks[0]
}

const globalMusicName = computed(() => {
  if (!globalRemix.musicTracks || globalRemix.musicTracks.length === 0) {
    return ''
  }
  if (globalRemix.musicTracks.length === 1) {
    return globalRemix.musicTracks[0].split('\\').pop() || ''
  }
  return `${globalRemix.musicTracks.length} bài hát đã chọn`
})

// === Cấu hình (Settings) ===
const showSettings = ref(false)

const globalSettingsConfig = ref<any>(null)

const activeView = ref<'split' | 'download-video' | 'download-image' | 'ai-image' | 'ai-video'>('split')

const analyzerConfig = reactive(new project.AnalyzerConfig({
  mode: 'smart',
  sceneThreshold: 20.0,
  minClipDuration: 1.0,
  maxClipDuration: 60.0,
  autoAcceptScore: 60,
  reviewMinScore: 35,
  silenceThreshold: -30,
  silenceDuration: 0.5,
  proxyFPS: 4,
  weights: {
    visualChange: 45,
    blackFrame: 35,
    silence: 30,
    layoutChange: 25,
    audioChange: 20,
    continuityPen: 10
  },
  exportPreset: 'fast',
  exportCRF: 23,
  prompt: '',
  hardwareAccel: 'none'
}))

const loadDefaultConfig = async () => {
  try {
    const cfg = await GetDefaultConfig()
    Object.assign(analyzerConfig, cfg)
  } catch (e) {
    console.error('Lỗi tải config mặc định:', e)
  }
}

const resetConfig = async () => {
  // Lấy lại mặc định từ Go để không lệch với backend.
  await loadDefaultConfig()
  addLog('Đã đặt lại cấu hình về mặc định.')
}

const onMinDurationChange = () => {
  let val = Number(analyzerConfig.minClipDuration)
  if (isNaN(val) || val < 1) val = 1
  if (val > 60) val = 60
  analyzerConfig.minClipDuration = val

  if (analyzerConfig.minClipDuration > analyzerConfig.maxClipDuration) {
    analyzerConfig.maxClipDuration = analyzerConfig.minClipDuration
  }
}

const onMaxDurationChange = () => {
  let val = Number(analyzerConfig.maxClipDuration)
  if (isNaN(val) || val < 10) val = 10
  if (val > 600) val = 600
  analyzerConfig.maxClipDuration = val

  if (analyzerConfig.maxClipDuration < analyzerConfig.minClipDuration) {
    analyzerConfig.minClipDuration = analyzerConfig.maxClipDuration
  }
}

const isSettingsLoaded = ref(false)

const saveGlobalSettings = async () => {
  if (!isSettingsLoaded.value) return
  try {
    const settingsStr = await GetGlobalSettings()
    let gSettings: any = {}
    if (settingsStr) {
      gSettings = JSON.parse(settingsStr)
    }

    gSettings.analyzerConfig = JSON.parse(JSON.stringify(analyzerConfig))
    gSettings.globalRemix = JSON.parse(JSON.stringify(globalRemix))
    gSettings.exportJobs = exportJobs.value
    gSettings.analyzeJobs = analyzeJobs.value
    gSettings.outDir = outDir.value
    gSettings.outImageDir = outImageDir.value
    gSettings.autoCreateSubfolders = autoCreateSubfolders.value
    gSettings.exportWithThumbnails = exportWithThumbnails.value
    gSettings.geminiAPIKey = geminiAPIKey.value
    gSettings.browserAIShowChrome = browserAIShowChrome.value
    gSettings.browserAIDelay = browserAIDelay.value

    await SaveGlobalSettings(JSON.stringify(gSettings))
    globalSettingsConfig.value = JSON.parse(JSON.stringify(analyzerConfig))
  } catch (e) {
    console.error('Lỗi tự động lưu cài đặt chung:', e)
  }
}

let saveTimeout: any = null
watch([analyzerConfig, globalRemix, exportJobs, analyzeJobs, outDir, outImageDir, autoCreateSubfolders, exportWithThumbnails, geminiAPIKey, browserAIShowChrome, browserAIDelay], () => {
  if (!isSettingsLoaded.value) return
  if (saveTimeout) clearTimeout(saveTimeout)
  saveTimeout = setTimeout(() => {
    saveGlobalSettings()
  }, 800)
}, { deep: true })

const videoPlayer = ref<HTMLVideoElement | null>(null)
const editPlayer = ref<HTMLVideoElement | null>(null)

const videoCurrentTime = ref(0)
const onVideoTimeUpdate = (e: Event) => {
  const video = e.target as HTMLVideoElement
  videoCurrentTime.value = video.currentTime
}

const activeVideoPath = computed(() => videoPaths.value[activeVideoIndex.value] || '')
const activeClips = computed(() => clipsMap.value[activeVideoPath.value] || [])

// Khi có nhiều video được tick, hiện TẤT CẢ clips gộp lại (thêm tên video nguồn).
// Khi chỉ có 1 video hoặc không tick → hiện clips của video đang chọn.
const displayClips = computed(() => {
  const sel = selectedVideos.value
  if (sel.size > 1) {
    const all: any[] = []
    for (const path of videoPaths.value) {
      if (!sel.has(path)) continue
      const clips = clipsMap.value[path]
      if (!clips || clips.length === 0) continue
      const videoName = path.split('\\').pop() || path
      for (const c of clips) {
        all.push({ ...c, _videoName: videoName, _videoPath: path })
      }
    }
    return all
  }
  // Fallback: chỉ 1 video → dùng activeClips
  const clips = clipsMap.value[activeVideoPath.value] || []
  const videoName = activeVideoPath.value.split('\\').pop() || ''
  return clips.map((c: any) => ({ ...c, _videoName: videoName, _videoPath: activeVideoPath.value }))
})

const isMultiVideoDisplay = computed(() => selectedVideos.value.size > 1)
const displayClipsCount = computed(() => displayClips.value.length)

// Có ít nhất một video (trong số được tick, hoặc tất cả nếu chưa tick) đã phân tích
// xong (có phân đoạn) → mới cho hiện nút Xuất hàng loạt.
const hasAnalyzedForBatch = computed(() => {
  const targets = selectedVideos.value.size > 0
    ? videoPaths.value.filter(p => selectedVideos.value.has(p))
    : videoPaths.value
  return targets.some(p => (clipsMap.value[p]?.length || 0) > 0)
})

const computedVideoSrc = computed(() => {
  if (isPlayingExported.value && activeExportedSrc.value) {
    return activeExportedSrc.value
  }
  return activeVideoSrc.value
})

// === QUẢN LÝ PROMPT THUMBNAIL MẪU (PRESETS) ===
interface PromptPreset {
  id: string
  name: string
  content: string
}

const promptPresets = ref<PromptPreset[]>([
  { id: 'default_auto', name: '✨ Tối ưu tự động', content: 'A premium, eye-catching, and highly engaging thumbnail with a professional modern look, vibrant colors, clean lighting, and clear focal point.' },
  { id: '1', name: '🎬 Kịch tính / Điện ảnh', content: 'Dramatic cinematic scene, high suspense, emotional facial expression, extreme close-up, vivid colors, neon lighting accents, dark background, YouTube Shorts thumbnail style.' },
  { id: '2', name: '🎨 Hoạt họa / Anime', content: 'Vibrant anime visual style, cute character, colorful background, soft lighting, 4k digital art illustration, highly detailed, eye-catching style.' },
  { id: '3', name: '📸 Vlog / Đời thường', content: 'Modern casual lifestyle vlog style, bright natural lighting, happy emotion, clean background, high clarity, realistic mobile-first photography.' },
  { id: '4', name: '🔥 Xu hướng / Viral', content: 'High contrast trending vertical thumbnail, ultra-clear visual detail, bold composition, dynamic lighting, optimized for mobile screens, premium aesthetics.' }
])

const selectedPromptPresetId = ref('')
const newPresetName = ref('')
const showAddPresetForm = ref(false)

const loadPromptPresets = () => {
  const data = localStorage.getItem('prompt_presets_list')
  if (data) {
    try {
      let loaded = JSON.parse(data) as PromptPreset[]
      // Loại bỏ các mẫu cũ nhạy cảm nếu có
      loaded = loaded.filter(p => !p.name.includes('Gái') && !p.content.includes('Hot girl') && !p.content.includes('sexy') && !p.name.includes('Hot Girl'))
      
      // Nếu sau khi filter bị thiếu các mẫu mặc định hoặc trống, hãy nạp lại các mẫu mới sạch sẽ
      const defaultPresetsList = [
        { id: 'default_auto', name: '✨ Tối ưu tự động', content: 'A premium, eye-catching, and highly engaging thumbnail with a professional modern look, vibrant colors, clean lighting, and clear focal point.' },
        { id: '1', name: '🎬 Kịch tính / Điện ảnh', content: 'Dramatic cinematic scene, high suspense, emotional facial expression, extreme close-up, vivid colors, neon lighting accents, dark background, YouTube Shorts thumbnail style.' },
        { id: '2', name: '🎨 Hoạt họa / Anime', content: 'Vibrant anime visual style, cute character, colorful background, soft lighting, 4k digital art illustration, highly detailed, eye-catching style.' },
        { id: '3', name: '📸 Vlog / Đời thường', content: 'Modern casual lifestyle vlog style, bright natural lighting, happy emotion, clean background, high clarity, realistic mobile-first photography.' },
        { id: '4', name: '🔥 Xu hướng / Viral', content: 'High contrast trending vertical thumbnail, ultra-clear visual detail, bold composition, dynamic lighting, optimized for mobile screens, premium aesthetics.' }
      ]
      
      // Bổ sung các mẫu mặc định còn thiếu
      for (const def of defaultPresetsList) {
        if (!loaded.some(l => l.id === def.id)) {
          loaded.push(def)
        } else {
          // Cập nhật nội dung sạch mới cho các ID mặc định
          const idx = loaded.findIndex(l => l.id === def.id)
          if (idx !== -1) {
            loaded[idx] = def
          }
        }
      }
      
      // Đảm bảo sắp xếp đúng thứ tự default lên trước
      loaded.sort((a, b) => {
        const order: Record<string, number> = { 'default_auto': 0, '1': 1, '2': 2, '3': 3, '4': 4 }
        const oa = order[a.id] !== undefined ? order[a.id] : 99
        const ob = order[b.id] !== undefined ? order[b.id] : 99
        return oa - ob
      })

      promptPresets.value = loaded
      savePromptPresets()
    } catch (e) {
      console.error(e)
    }
  }
}

const savePromptPresets = () => {
  localStorage.setItem('prompt_presets_list', JSON.stringify(promptPresets.value))
}

const selectPresetTag = (preset: PromptPreset) => {
  if (selectedPromptPresetId.value === preset.id) {
    selectedPromptPresetId.value = ''
  } else {
    selectedPromptPresetId.value = preset.id
    analyzerConfig.prompt = preset.content
  }
}

const deletePresetById = (id: string) => {
  promptPresets.value = promptPresets.value.filter(p => p.id !== id)
  savePromptPresets()
  if (selectedPromptPresetId.value === id) {
    selectedPromptPresetId.value = ''
  }
  showToast('Đã xóa mẫu prompt.', 'info')
}

const addNewPreset = () => {
  if (!newPresetName.value.trim()) {
    showToast('Vui lòng nhập tên mẫu prompt!', 'warning')
    return
  }
  if (!analyzerConfig.prompt.trim()) {
    showToast('Vui lòng nhập nội dung prompt trước khi lưu thành mẫu!', 'warning')
    return
  }
  const id = Date.now().toString()
  promptPresets.value.push({
    id,
    name: newPresetName.value.trim(),
    content: analyzerConfig.prompt.trim()
  })
  savePromptPresets()
  selectedPromptPresetId.value = id
  newPresetName.value = ''
  showAddPresetForm.value = false
  showToast('Đã lưu mẫu prompt mới!', 'success')
}


onMounted(async () => {
  loadPromptPresets()
  // 1. Tải cấu hình cài đặt chung toàn cục (settings.json)
  try {
    const globalSettingsStr = await GetGlobalSettings()
    if (globalSettingsStr) {
      const gSettings = JSON.parse(globalSettingsStr)
      if (gSettings.analyzerConfig) {
        Object.assign(analyzerConfig, gSettings.analyzerConfig)
        globalSettingsConfig.value = JSON.parse(JSON.stringify(gSettings.analyzerConfig))
      }
      if (gSettings.globalRemix) {
        Object.assign(globalRemix, gSettings.globalRemix)
        if (globalRemix.musicLoop === undefined) globalRemix.musicLoop = false
        if (!globalRemix.musicTracks) globalRemix.musicTracks = []
      }
      if (gSettings.exportJobs !== undefined) exportJobs.value = gSettings.exportJobs
      if (gSettings.analyzeJobs !== undefined) analyzeJobs.value = gSettings.analyzeJobs
      if (gSettings.outDir !== undefined) {
        outDir.value = gSettings.outDir
        globalSettingsOutDir.value = gSettings.outDir
      }
      if (gSettings.outImageDir !== undefined) {
        outImageDir.value = gSettings.outImageDir
      }
      if (gSettings.autoCreateSubfolders !== undefined) {
        autoCreateSubfolders.value = gSettings.autoCreateSubfolders
      }
      if (gSettings.exportWithThumbnails !== undefined) {
        exportWithThumbnails.value = gSettings.exportWithThumbnails
      }
      if (gSettings.geminiAPIKey !== undefined) {
        geminiAPIKey.value = gSettings.geminiAPIKey
      }
      if (gSettings.browserAIShowChrome !== undefined) {
        browserAIShowChrome.value = gSettings.browserAIShowChrome
      }
      if (gSettings.browserAIDelay !== undefined) {
        browserAIDelay.value = gSettings.browserAIDelay
      }
    } else {
      await loadDefaultConfig()
    }
  } catch (e) {
    console.error('Lỗi tải cấu hình toàn cục:', e)
    await loadDefaultConfig()
  }

  isSettingsLoaded.value = true // Đã load xong, bắt đầu tự động theo dõi và lưu cài đặt từ đây

  // Tải danh sách project
  const savedProjs = localStorage.getItem('namedProjectsList')
  const savedActiveId = localStorage.getItem('activeProjectId')
  
  if (savedProjs) {
    try {
      namedProjects.value = JSON.parse(savedProjs)
    } catch (e) {
      console.error("Lỗi parse namedProjectsList:", e)
    }
  }
  
  if (namedProjects.value.length === 0) {
    const defaultId = 'proj_default'
    namedProjects.value = [{
      id: defaultId,
      name: 'Dự án mặc định',
      videoPaths: [],
      selectedVideos: [],
      globalRemix: {
        autoApply: true,
        hflip: false,
        aspectEnabled: false,
        aspectRatio: '9:16',
        aspectMode: 'blur',
        speed: 1.0,
        colorEnabled: false,
        colorPreset: '',
        colorBrightness: 0,
        colorContrast: 0,
        colorSaturation: 1.0,
        musicPath: '',
        musicVolume: 0.3,
        muteOriginal: false,
        musicLoop: false,
        musicTracks: []
      },
      outDir: globalSettingsOutDir.value,
      exportJobs: 2,
      analyzeJobs: 1,
      createdAt: Date.now()
    }]
    activeProjectId.value = defaultId
  } else {
    activeProjectId.value = savedActiveId || namedProjects.value[0].id
  }

  // Load dự án hoạt động đầu tiên (ghi đè cấu hình toàn cục nếu dự án đã lưu cấu hình riêng)
  await loadProject(activeProjectId.value)
  addLog("Hệ thống Splitter đã sẵn sàng.")
  try {
    // Dùng placeholder để tách prefix/suffix quanh vị trí path đã encode.
    // GetStreamURL trả về dạng: ...?path=<PLACEHOLDER>&token=xxx
    const sample = await GetStreamURL('__PH__')
    const idx = sample.indexOf('__PH__')
    if (idx >= 0) {
      streamPrefix.value = sample.slice(0, idx)
      streamSuffix.value = sample.slice(idx + '__PH__'.length)
    }
  } catch (err) {
    console.error("Lỗi khởi tạo stream prefix:", err)
    addLog("Lỗi kết nối Stream Server local: " + String(err))
  }

  // Lắng nghe sự kiện log từ Backend Go
  EventsOn('analyze_log', (msg: string) => {
    addLog("[Phân Tích] " + msg)
  })
  EventsOn('export_log', (msg: string) => {
    addLog("[Xuất Bản] " + msg)
    if (msg.includes("Đang xử lý Video:") || (msg.includes("Clip #") && (msg.includes("đang") || msg.includes("Đang") || msg.includes("Hoàn thành") || msg.includes("Ghép")))) {
      exportStatusText.value = msg
    }
    
    // Đổ cảnh báo/lỗi ra Toast trực quan
    if (msg.includes("Lỗi sinh ảnh AI") || msg.includes("Lỗi sinh ảnh") || msg.includes("Lỗi trích xuất") || msg.includes("Cảnh báo - Chưa cấu hình API Key")) {
      showToast(msg, 'error', 6000)
    } else if (msg.includes("Cảnh báo")) {
      showToast(msg, 'warning', 5000)
    }
    
    // Đếm số lượng video và thumbnail đã hoàn thành từ logs
    if (msg.includes("Đang vẽ ảnh bìa AI")) {
      videoDoneCount.value = Math.min(exportProgress.value.total, videoDoneCount.value + 1)
    }
    if (msg.includes("Hoàn thành!")) {
      if (videoDoneCount.value < exportProgress.value.total) {
        videoDoneCount.value = Math.min(exportProgress.value.total, videoDoneCount.value + 1)
      }
      thumbDoneCount.value = Math.min(exportProgress.value.total, thumbDoneCount.value + 1)
    }
  })
  EventsOn('export_progress', (p: { done: number, total: number, clipId?: string, ok?: boolean, outPath?: string }) => {
    if (!isMultiExportRunning.value) {
      exportProgress.value = { done: p.done, total: p.total }
    }
    // Cập nhật status clip ngay khi clip đó xuất xong — hiện ✅ + nút Phát tức thì
    if (p.clipId && p.ok) {
      for (const path of Object.keys(clipsMap.value)) {
        const clip = (clipsMap.value[path] || []).find((c: any) => c.id === p.clipId)
        if (clip) {
          clip.status = 'completed'
          // Lưu đường dẫn file video đã cắt để nút "Phát" hoạt động ngay
          if (p.outPath) {
            clip.exportedPath = p.outPath
          }
          break
        }
      }
    }
  })

  // Đếm ngược thời gian hoàn tất & Cập nhật tiến độ mượt mà (interpolation)
  timerInterval = setInterval(() => {
    const now = Date.now()
    
    // Cập nhật currentTimeRef mỗi 1 giây
    if (Math.abs(now - currentTimeRef.value) >= 1000) {
      currentTimeRef.value = now
    }

    // Xử lý tiến độ "nhích nhích" mượt mà cho tất cả video đang chạy
    for (const path of activeAnalyzingPaths.value) {
      const target = analyzeProgressMap.value[path] || 0
      let current = displayProgressMap.value[path] || 0
      
      if (current < target) {
        // Trượt mượt về phía đích thực tế do BE báo cáo (không tự ý chế tiến độ giả)
        const step = (target - current) * 0.12
        current += step > 0.08 ? step : 0.08
        if (current >= target) current = target
      }
      
      displayProgressMap.value[path] = current
    }
  }, 100)
})

onUnmounted(() => {
  if (timerInterval) clearInterval(timerInterval)
})

const currentTimeRef = ref(Date.now())
const displayProgressMap = ref<Record<string, number>>({})
let timerInterval: any = null

const analyzeETAMap = ref<Record<string, { remaining: number, lastUpdate: number, lastProgress: number, hasRealDuration?: boolean }>>({})
const exportETARecord = ref<{ remaining: number, lastUpdate: number, lastProgress: number } | null>(null)
const videoDurationMap = ref<Record<string, number>>({})
const videoInfoMap = ref<Record<string, project.VideoInfo>>({})

const getAnalyzeETA = (path: string): string => {
  const now = currentTimeRef.value // dynamic dependency
  const startTime = analyzeStartTimes.value[path]
  const progress = displayProgressMap.value[path] || 0

  if (!startTime) {
    return 'Đang chuẩn bị...'
  }
  if (progress >= 98) {
    return 'Đang hoàn tất...'
  }

  const duration = videoDurationMap.value[path] || (path === activeVideoPath.value && videoInfo.value?.Duration) || 0
  const info = videoInfoMap.value[path] || (path === activeVideoPath.value && videoInfo.value)

  // Calculate default initial estimate based on duration, mode, and realistic startup overheads across all 3 phases
  let initialRemaining = 45 // fallback default
  if (duration > 0) {
    const mode = analyzerConfig.mode
    if (mode === 'fast' || mode === 'fixed') {
      // Fast mode: KHÔNG tạo proxy, chạy thẳng PySceneDetect trên video gốc
      // Nhanh hơn nhưng ít chính xác hơn — thời gian ≈ duration / 60 (PySceneDetect CPU)
      initialRemaining = 5 + duration / 60.0
    } else if (mode === 'precise') {
      initialRemaining = 15 + duration / 10.0 // precise: proxy + WAV + Librosa
    } else {
      initialRemaining = 10 + duration / 18.0 // smart: proxy 320×180 + multi-detector
    }
    if (initialRemaining < 3) initialRemaining = 3

    // Apply complexity factor based on resolution and FPS
    if (info && info.Width && info.Height && info.FPS) {
      const pixelRate = info.Width * info.Height * info.FPS
      const standardRate = 1920 * 1080 * 30 // standard 1080p 30fps
      let complexity = pixelRate / standardRate
      if (complexity < 0.25) complexity = 0.25
      if (complexity > 4.0) complexity = 4.0 // Cap at 4.0x since scaling is not strictly linear for GPU decoding
      initialRemaining = initialRemaining * complexity
    }
  }

  // Scale initial estimate based on parallel analyze threads (gentle sub-linear scaling since cores share resources)
  const jobCount = analyzeJobs.value || 1
  const concurrencyMultiplier = 1.0 + (jobCount - 1) * 0.25
  initialRemaining = initialRemaining * concurrencyMultiplier

  let eta = analyzeETAMap.value[path]
  const nowMs = Date.now()

  // Estimate Phase 3 (Thumbnail extraction) overhead: min 3 seconds, average duration / 75 seconds.
  const phase3Overhead = duration > 0 ? Math.max(3, duration / 75) : 5

  // Calculate raw estimate based on progress (only use linear projection after 15% to avoid initial noise)
  let rawRemaining = initialRemaining
  if (progress >= 15 && progress < 92) {
    const elapsed = (nowMs - startTime) / 1000
    // Phase 1 + 2 maps to 0% - 90% progress range. Scale progress accordingly.
    const phase12ProgressRatio = Math.min(0.99, progress / 90)
    const totalEstimatedPhase12 = elapsed / phase12ProgressRatio
    const remainingPhase12 = Math.max(0, totalEstimatedPhase12 - elapsed)
    
    rawRemaining = remainingPhase12 + phase3Overhead
  } else if (progress >= 92 && progress < 98) {
    // Phase 3 maps to 92% - 98% progress range. Interpolate remaining Phase 3 time.
    const phase3Percent = (progress - 92) / (98 - 92)
    rawRemaining = Math.max(1, phase3Overhead * (1 - phase3Percent))
  }

  if (!eta) {
    eta = {
      remaining: rawRemaining,
      lastUpdate: nowMs,
      lastProgress: progress,
      hasRealDuration: duration > 0
    }
    analyzeETAMap.value[path] = eta
  } else if (duration > 0 && !eta.hasRealDuration) {
    eta.remaining = rawRemaining
    eta.lastUpdate = nowMs
    eta.lastProgress = progress
    eta.hasRealDuration = true
  } else {
    eta.remaining = rawRemaining
    eta.lastUpdate = nowMs
    eta.lastProgress = progress
  }

  // displayRemaining = luôn dùng rawRemaining (tính từ elapsed thực tế)
  // Không dùng "countdown từ lastUpdate" vì sẽ drift về 0 khi progress đứng yên
  let displayRemaining = Math.max(1, rawRemaining)

  // Khi đang ở Bước 1 (progress <= 14%): đếm ngược từ initialRemaining - elapsed
  if (progress <= 14) {
    const elapsed = (nowMs - startTime) / 1000
    displayRemaining = Math.max(1, initialRemaining - elapsed)
  }

  // Khi progress 15-92%: rawRemaining đã được tính đúng theo elapsed → dùng trực tiếp
  // Khi progress 92-98%: interpolate phase3 overhead

  // Sàn: không bao giờ hiện < phase3Overhead khi chưa vào Phase 3
  if (progress < 92 && displayRemaining < phase3Overhead) {
    displayRemaining = phase3Overhead
  }

  const m = Math.floor(displayRemaining / 60)
  const s = Math.floor(displayRemaining % 60)

  if (displayRemaining <= 5) {
    return 'Sắp xong...'
  }
  return `Còn khoảng ${m > 0 ? `${m}ph ` : ''}${s}s`
}

const getExportETA = (): string => {
  const now = currentTimeRef.value // dynamic dependency
  const startTime = exportStartTime.value
  const done = exportProgress.value.done
  const total = exportProgress.value.total

  if (!startTime || total <= 0) {
    return 'Đang chuẩn bị...'
  }
  if (done === 0) {
    return 'Đang xử lý...'
  }
  if (done >= total) {
    return 'Hoàn thành'
  }

  const nowMs = Date.now()
  const elapsed = (nowMs - startTime) / 1000
  const avgTimePerClip = elapsed / done
  const remainingClips = total - done
  const rawRemaining = avgTimePerClip * remainingClips

  let eta = exportETARecord.value
  if (!eta) {
    eta = {
      remaining: rawRemaining,
      lastUpdate: nowMs,
      lastProgress: done
    }
    exportETARecord.value = eta
  } else if (done !== eta.lastProgress) {
    const secondsPass = (nowMs - eta.lastUpdate) / 1000
    const currentCountdown = Math.max(1, eta.remaining - secondsPass)
    const smoothed = currentCountdown * 0.6 + rawRemaining * 0.4
    
    eta.remaining = smoothed
    eta.lastUpdate = nowMs
    eta.lastProgress = done
  }

  const secondsSinceLastUpdate = (nowMs - eta.lastUpdate) / 1000
  const displayRemaining = Math.max(1, eta.remaining - secondsSinceLastUpdate)

  const m = Math.floor(displayRemaining / 60)
  const s = Math.floor(displayRemaining % 60)

  if (displayRemaining <= 1.5) {
    return 'Còn vài giây...'
  }
  return `Còn khoảng ${m > 0 ? `${m}ph ` : ''}${s}s`
}

const formatExportStatusMsg = (msg: string): string => {
  if (!msg) return ''
  if (msg.includes('Đang bắt đầu...')) return msg
  let cleaned = msg.replace("Đang xử lý Video: ", "👉 ")
  cleaned = cleaned.replace(": Đang cắt video...", "")
  cleaned = cleaned.replace(": Đang vẽ ảnh bìa AI...", "")
  cleaned = cleaned.replace(": Hoàn thành! (THÀNH CÔNG)", " (Hoàn tất)")
  cleaned = cleaned.replace(": Hoàn thành! (ĐÃ CÓ)", " (Hoàn tất)")
  cleaned = cleaned.replace(": Hoàn thành! (THẤT BẠI)", " (Lỗi)")
  return cleaned
}

const getThumbUrl = (path: string) => {
  if (!path || !streamPrefix.value) return ''
  return `${streamPrefix.value}${encodeURIComponent(path)}${streamSuffix.value}`
}

EventsOn('analyze_progress', (data: { path: string, progress: number }) => {
  if (data && data.path) {
    analyzeProgressMap.value[data.path] = data.progress
  }
})

// Nhận TẤT CẢ clip ngay sau Bước 2 (phát hiện điểm cắt xong, chưa có ảnh xem trước)
// → Hiện clip lên giao diện ngay lập tức, không đợi ảnh xem trước
EventsOn('clips_detected', (data: { path: string, clips: any[] }) => {
  if (data && data.path && data.clips) {
    const processed = data.clips.map((raw: any) => {
      const c = new project.Clip(raw)
      c.startTime = Math.round(c.startTime)
      c.endTime = Math.round(c.endTime)
      c.duration = Math.round(c.endTime - c.startTime)
      applyGlobalRemixToClip(c)
      return c
    })
    clipsMap.value[data.path] = processed
  }
})

// Cập nhật ảnh xem trước cho từng clip đã hiện (Bước 3 chạy nền)
// → Ảnh tự động "hiện dần" trên giao diện mà clip đã có sẵn
EventsOn('clip_thumb_update', (data: { path: string, clipId: string, thumbnail: string, thumbEnd: string }) => {
  if (data && data.path && data.clipId) {
    const clips = clipsMap.value[data.path]
    if (clips) {
      const clip = clips.find((c: any) => c.id === data.clipId)
      if (clip) {
        clip.thumbnail = data.thumbnail || ''
        clip.thumbEnd = data.thumbEnd || ''
      }
    }
  }
})



const handleSelectFiles = async () => {
  try {
    const selected = await SelectFiles()
    if (selected && selected.length > 0) {
      const existingPaths = new Set(videoPaths.value)
      const newPaths: string[] = []
      
      for (const path of selected) {
        if (!existingPaths.has(path)) {
          newPaths.push(path)
        }
      }
      
      if (newPaths.length > 0) {
        const wasEmpty = videoPaths.value.length === 0
        videoPaths.value = [...videoPaths.value, ...newPaths]
        
        if (wasEmpty) {
          activeVideoIndex.value = 0
          activeVideoSrc.value = await GetStreamURL(videoPaths.value[0])
          await loadVideoInfo(videoPaths.value[0])
        }
      } else {
        showToast('Tất cả các video bạn chọn đã có sẵn trong danh sách!', 'warning')
      }
    }
  } catch (err) {
    showToast('Lỗi khi mở hộp thoại: ' + err, 'error')
  }
}

const selectVideo = async (index: number) => {
  activeVideoIndex.value = index
  isPlayingExported.value = false
  videoCurrentTime.value = 0
  const path = videoPaths.value[index]
  activeVideoSrc.value = await GetStreamURL(path)
  await loadVideoInfo(path)
}

const loadVideoInfo = async (path: string) => {
  try {
    videoInfo.value = await GetVideoInfo(path)
  } catch (err) {
    showToast('Lỗi đọc thông tin video: ' + err, 'error')
    videoInfo.value = null
  }
}

const analyzeSingle = async (path: string): Promise<boolean> => {
  try {
    activeAnalyzingPaths.value.add(path)
    analyzeProgressMap.value[path] = 0
    displayProgressMap.value[path] = 0
    delete analyzeETAMap.value[path]
    analyzeStartTimes.value[path] = Date.now()
    
    // Clear old clips to ensure the new analysis results completely overwrite the old ones
    clipsMap.value[path] = []
    
    // Fetch video info to pre-calculate ETA duration
    try {
      const info = await GetVideoInfo(path)
      if (info && info.Duration) {
        videoDurationMap.value[path] = info.Duration
        videoInfoMap.value[path] = info
      }
    } catch (e) {
      console.error("Lỗi đọc duration cho ETA:", e)
    }

    const clips = await Analyze(path, analyzerConfig)
    // Làm tròn mốc thời gian về 1 chữ số thập phân — backend trả số lẻ dài
    // (vd 485.13333333) rất khó nhìn/khó chỉnh trên ô nhập.
    for (const c of clips) {
      c.startTime = Math.round(c.startTime)
      c.endTime = Math.round(c.endTime)
      c.duration = Math.round(c.endTime - c.startTime)
      // Tự động áp dụng cấu hình chỉnh sửa video
      applyGlobalRemixToClip(c)
    }

    // Nếu clip đã hiện trước đó (từ event clips_detected), chỉ cập nhật thumbnail
    // để không ghi đè thay đổi user đã làm (chia, gộp, chỉnh sửa...)
    const existing = clipsMap.value[path]
    if (existing && existing.length > 0) {
      for (const newClip of clips) {
        const old = existing.find((c: any) => c.id === newClip.id)
        if (old) {
          old.thumbnail = newClip.thumbnail
          old.thumbEnd = newClip.thumbEnd
        }
      }
    } else {
      clipsMap.value[path] = clips
    }
    analyzeProgressMap.value[path] = 100
    return true
  } catch (err) {
    const errMsg = String(err)
    if (errMsg.includes('context canceled') || errMsg.includes('canceled') || isCancelled.value) {
      console.log('Phân tích đã bị người dùng hủy.')
    } else {
      showToast('Lỗi phân tích: ' + err, 'error')
    }
    analyzeProgressMap.value[path] = 0
    return false
  } finally {
    activeAnalyzingPaths.value.delete(path)
  }
}

// === Thêm nguyên video làm clip (để chỉ chỉnh sửa, không cần cắt) ===
const addWholeVideoAsClip = async (path: string) => {
  try {
    const info = await GetVideoInfo(path)
    if (!info || !info.Duration || info.Duration <= 0) {
      showToast('Không đọc được thông tin video: ' + path.split('\\').pop(), 'error')
      return
    }
    const clipId = 'whole_' + Date.now() + '_' + Math.random().toString(36).slice(2, 8)
    const clip = new project.Clip({
      id: clipId,
      index: 0,
      startTime: 0,
      endTime: Math.round(info.Duration),
      duration: Math.round(info.Duration),
      status: 'accepted',
      thumbnail: '',
      thumbEnd: '',
      confidence: 100,
      tier: 'auto',
      reason: 'Nguyên video (để chỉnh sửa)',
      signals: ['whole_video'],
      edit: undefined as any,
    })
    ensureEdit(clip)
    applyGlobalRemixToClip(clip)
    clipsMap.value[path] = [clip]
    showToast(`Đã thêm nguyên video làm 1 clip (${formatTime(info.Duration)})`, 'success')
  } catch (err) {
    showToast('Lỗi: ' + String(err), 'error')
  }
}

const exportWithoutSplitting = async () => {
  const targets = videoPaths.value.filter(path => selectedVideos.value.has(path))
  const finalTargets = targets.length > 0 ? targets : videoPaths.value
  
  if (finalTargets.length === 0) {
    showToast('Vui lòng chọn hoặc thêm video gốc trước', 'warning')
    return
  }
  
  // Tự động thêm toàn bộ video làm clip nếu chưa có
  for (const path of finalTargets) {
    if (!clipsMap.value[path] || clipsMap.value[path].length === 0) {
      await addWholeVideoAsClip(path)
    }
  }
  
  // Tự động tích chọn các clip này để chuẩn bị xuất
  finalTargets.forEach(path => {
    const list = clipsMap.value[path] || []
    list.forEach(c => selectedClips.value.add(c.id))
  })
  
  // Kích hoạt xuất toàn bộ các clip đã chọn
  await exportClips()
}

const startAnalysis = async () => {
  if (!activeVideoPath.value) return
  isAnalyzing.value = true
  isCancelled.value = false
  totalVideosCount.value = 1
  processedVideosCount.value = 0
  activeProcessingVideoName.value = activeVideoPath.value.split('\\').pop() || ''
  await analyzeSingle(activeVideoPath.value)
  isAnalyzing.value = false
}

const cancelCurrentAnalysis = async () => {
  isCancelled.value = true
  try {
    await CancelAnalysis()
  } catch (err) {
    console.error("Lỗi khi hủy phân tích:", err)
  }
}

const analyzeAll = async () => {
  const allTargets = videosForBatch()
  if (allTargets.length === 0) return

  // Tự động bỏ qua các video đã được cắt và có phân đoạn (clips) hiển thị.
  const targets = allTargets.filter(p => {
    return !clipsMap.value[p] || clipsMap.value[p].length === 0
  })
  const alreadyCut = allTargets.filter(p => !targets.includes(p))

  if (targets.length === 0) {
    showToast(`Tất cả ${alreadyCut.length} video đã được cắt rồi! Không cần cắt lại.`, 'info')
    return
  }

  const skippedMsg = alreadyCut.length > 0
    ? `\n(Bỏ qua ${alreadyCut.length} video đã cắt trước đó)`
    : ''

  isAnalyzing.value = true
  isCancelled.value = false
  totalVideosCount.value = targets.length
  processedVideosCount.value = 0

  // Chạy song song giới hạn bởi số luồng analyzeJobs
  const queue = [...targets]
  const activeWorkers: Promise<void>[] = []

  const worker = async () => {
    while (queue.length > 0 && !isCancelled.value) {
      const path = queue.shift()
      if (!path) continue

      activeProcessingVideoName.value = path.split('\\').pop() || ''
      const success = await analyzeSingle(path)
      if (success) {
        processedVideosCount.value++
      }
      if (isCancelled.value) {
        break
      }
    }
  }

  const workerCount = Math.min(analyzeJobs.value, targets.length)
  for (let w = 0; w < workerCount; w++) {
    activeWorkers.push(worker())
  }

  await Promise.all(activeWorkers)

  isAnalyzing.value = false
  if (isCancelled.value) {
    showToast(`Đã dừng cắt tự động! (${processedVideosCount.value}/${targets.length} video đã xử lý)${skippedMsg}`, 'warning')
  } else {
    showToast(`Cắt tự động hoàn tất! ${processedVideosCount.value}/${targets.length} video thành công.${skippedMsg}`, 'success')
  }
}

// === Chọn video hàng loạt (phân tích / xuất / áp chỉnh sửa cho nhiều video) ===
// Lưu đường dẫn video được tick. Rỗng = coi như thao tác trên TẤT CẢ video.
const selectedVideos = ref<Set<string>>(new Set())

const toggleVideoSelected = (path: string) => {
  const s = new Set(selectedVideos.value)
  if (s.has(path)) s.delete(path)
  else s.add(path)
  selectedVideos.value = s
}

const isDraggingSelect = ref(false)
const dragSelectMode = ref<'select' | 'deselect'>('select')

const handleVideoMousedown = (path: string, event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (target.closest('.clip-checkbox') || target.closest('.btn-remove-video') || target.closest('input') || target.closest('button')) {
    return
  }

  isDraggingSelect.value = true
  const hasPath = selectedVideos.value.has(path)
  
  if (hasPath) {
    dragSelectMode.value = 'deselect'
    const s = new Set(selectedVideos.value)
    s.delete(path)
    selectedVideos.value = s
  } else {
    dragSelectMode.value = 'select'
    const s = new Set(selectedVideos.value)
    s.add(path)
    selectedVideos.value = s
  }

  const handleGlobalMouseup = () => {
    isDraggingSelect.value = false
    window.removeEventListener('mouseup', handleGlobalMouseup)
  }
  window.addEventListener('mouseup', handleGlobalMouseup)
}

const handleVideoMouseenter = (path: string) => {
  if (!isDraggingSelect.value) return

  const s = new Set(selectedVideos.value)
  if (dragSelectMode.value === 'select') {
    s.add(path)
  } else {
    s.delete(path)
  }
  selectedVideos.value = s
}

const isAllVideosSelected = computed(() =>
  videoPaths.value.length > 0 && videoPaths.value.every(p => selectedVideos.value.has(p))
)

const toggleSelectAllVideos = () => {
  if (isAllVideosSelected.value) selectedVideos.value = new Set()
  else selectedVideos.value = new Set(videoPaths.value)
}

// Danh sách video sẽ xử lý hàng loạt: các video được tick, hoặc tất cả nếu chưa tick cái nào.
const videosForBatch = () =>
  selectedVideos.value.size > 0
    ? videoPaths.value.filter(p => selectedVideos.value.has(p))
    : videoPaths.value

const selectedClips = ref<Set<string>>(new Set())

const toggleClipSelected = (clipId: string) => {
  const s = new Set(selectedClips.value)
  if (s.has(clipId)) {
    s.delete(clipId)
  } else {
    s.add(clipId)
  }
  selectedClips.value = s
}

const isAllSelected = computed(() => {
  const clips = displayClips.value
  return clips.length > 0 && clips.every((c: any) => selectedClips.value.has(c.id))
})

const toggleSelectAll = () => {
  if (isAllSelected.value) {
    selectedClips.value = new Set()
  } else {
    selectedClips.value = new Set(displayClips.value.map((c: any) => c.id))
  }
}

const exportProgress = ref({ done: 0, total: 0 })

const clipsForExport = () => {
  return selectedClips.value.size > 0
    ? activeClips.value.filter(c => selectedClips.value.has(c.id))
    : activeClips.value
}

const hasExportedCurrentSession = ref(false)

watch([activeProjectId, activeVideoPath], () => {
  hasExportedCurrentSession.value = false
})

const chooseOutDir = async () => {
  try {
    const folder = await SelectFolder()
    if (folder) {
      outDir.value = folder
      addLog(`Thư mục lưu video xuất: ${folder}`)
    }
  } catch (err) {
    addLog('Lỗi chọn thư mục: ' + err)
  }
}

const chooseOutImageDir = async () => {
  try {
    const folder = await SelectFolder()
    if (folder) {
      outImageDir.value = folder
      addLog(`Thư mục lưu ảnh thumbnail: ${folder}`)
    }
  } catch (err) {
    addLog('Lỗi chọn thư mục: ' + err)
  }
}

const openAILogin = async (provider: string) => {
  try {
    showToast(`Đang mở Chrome để đăng nhập ${provider === 'gemini' ? 'Gemini' : 'Google Flow'}...`, "info")
    // @ts-ignore
    await import('../../wailsjs/go/browserai/Service').then(async (srv) => {
      await srv.OpenGoogleAI(provider, true) // Always show browser for logins
    })
    showToast("Đã mở Chrome. Vui lòng đăng nhập tài khoản Google của bạn.", "success")
  } catch (err) {
    showToast("Không mở được Chrome: " + err, "error")
  }
}

const clearAILogin = async () => {
  const ok = confirm("Bạn có chắc chắn muốn xóa phiên đăng nhập Google? Thao tác này sẽ xóa sạch cookie đăng nhập của Gemini / Google Flow trên Chrome.")
  if (ok) {
    try {
      // @ts-ignore
      await import('../../wailsjs/go/browserai/Service').then(async (srv) => {
        await srv.ClearBrowserProfile()
      })
      showToast("Đã xóa sạch phiên đăng nhập Google AI!", "success")
    } catch (err) {
      showToast("Lỗi xóa phiên đăng nhập: " + err, "error")
    }
  }
}

const stopExport = async () => {
  try {
    await CancelExport()
    isExporting.value = false
  } catch (err) {
    addLog('Lỗi khi dừng xuất video: ' + err)
  }
}

const getSelectedClipsGroupedByVideo = () => {
  const groups: Record<string, project.Clip[]> = {}
  for (const path of videoPaths.value) {
    const clips = clipsMap.value[path] || []
    const selected = clips.filter(c => selectedClips.value.has(c.id))
    if (selected.length > 0) {
      groups[path] = selected
    }
  }
  return groups
}

const activeSelectedCount = computed(() => {
  return activeClips.value.filter(c => selectedClips.value.has(c.id)).length
})

const exportClips = async () => {
  const clipsToExportMap = getSelectedClipsGroupedByVideo()
  const totalClipsToExport = Object.values(clipsToExportMap).reduce((acc, list) => acc + list.length, 0)
  
  if (totalClipsToExport === 0) {
    if (!activeVideoPath.value || activeClips.value.length === 0) return
    clipsToExportMap[activeVideoPath.value] = activeClips.value
  }

  const groupedKeys = Object.keys(clipsToExportMap)
  const finalTotal = Object.values(clipsToExportMap).reduce((acc, list) => acc + list.length, 0)

  const alreadyCompleted = []
  for (const [videoPath, list] of Object.entries(clipsToExportMap)) {
    for (const clip of list) {
      if (clip.status === 'completed') {
        const vidName = videoPath.split('\\').pop() || 'Video'
        alreadyCompleted.push(`${vidName} (Clip #${clip.index})`)
      }
    }
  }

  if (alreadyCompleted.length > 0) {
    const listStr = alreadyCompleted.slice(0, 3).join(', ') + (alreadyCompleted.length > 3 ? ` và ${alreadyCompleted.length - 3} clip khác` : '')
    const confirmReExport = await showCustomConfirm(`Phát hiện ${alreadyCompleted.length} clip đã được xuất trước đó (${listStr}). Bạn có muốn tiếp tục xuất lại để ghi đè không?`)
    if (!confirmReExport) return
  }

  isExporting.value = true
  isMultiExportRunning.value = true
  exportProgress.value = { done: 0, total: finalTotal }
  exportStatusText.value = 'Đang bắt đầu...'
  videoDoneCount.value = 0
  thumbDoneCount.value = 0
  exportStartTime.value = Date.now()
  
  try {
    const proj = namedProjects.value.find(p => p.id === activeProjectId.value)
    const projName = proj ? proj.name : 'Project'

    let globalDone = 0
    let okCount = 0
    let failedList: { index: number, video: string }[] = []

    for (const videoPath of groupedKeys) {
      const list = clipsToExportMap[videoPath]
      
      // Luôn áp cấu hình chế cháo trước khi xuất
      if (globalRemix.autoApply) {
        list.forEach(c => applyGlobalRemixToClip(c))
      }

      let lastDoneForThisVideo = 0
      const unlisten = EventsOn('export_progress', (data: any) => {
        const isBelong = list.some(c => c.id === data.clipId)
        if (isBelong) {
          const delta = data.done - lastDoneForThisVideo
          if (delta > 0) {
            globalDone += delta
            lastDoneForThisVideo = data.done
            exportProgress.value = { done: globalDone, total: finalTotal }
          }
        }
      })

      const exportDestDir = autoCreateSubfolders.value ? outDir.value + '\\video' : outDir.value
      const imageDestDir = exportWithThumbnails.value 
        ? (autoCreateSubfolders.value ? outDir.value + '\\image' : outImageDir.value)
        : ''
      const results = await ExportClips(projName, videoPath, list, exportDestDir, imageDestDir, analyzerConfig, exportJobs.value)
      unlisten()

      const stopped = results.some(r => r.error === 'Tiến trình xuất bị dừng' || r.error === 'Tiến trình bị dừng')
      if (stopped) {
        addLog('Tiến trình xuất video đã bị dừng.')
        return
      }

      const okIds = new Set(results.filter(r => r.ok).map(r => r.clipId))
      const allClipsOfThisVideo = clipsMap.value[videoPath] || []
      allClipsOfThisVideo.forEach(c => {
        if (okIds.has(c.id)) c.status = 'completed'
      })

      okCount += results.filter(r => r.ok).length
      results.filter(r => !r.ok).forEach(f => {
        failedList.push({ index: f.index, video: videoPath.split('\\').pop() || 'Video' })
      })
    }

    if (okCount > 0) {
      hasExportedCurrentSession.value = true
    }

    if (failedList.length > 0) {
      showToast(`Xuất xong: ${okCount}/${finalTotal} clip OK. Lỗi: ${failedList.map(f => `${f.video} (Clip #${f.index})`).join(', ')}`, 'warning', 5000)
    } else {
      showToast(`Xuất thành công ${okCount} clip!`, 'success')
    }
  } catch (err) {
    showToast('Lỗi xuất video: ' + err, 'error')
  } finally {
    isExporting.value = false
    isMultiExportRunning.value = false
    exportStartTime.value = null
  }
}

// Xuất clip của NHIỀU video được tick (mỗi video một thư mục con theo tên video).
// Chỉ xuất video đã có clip (đã phân tích); bỏ qua video chưa quét.
const exportSelectedVideos = async () => {
  const videos = videosForBatch().filter(p => (clipsMap.value[p]?.length || 0) > 0)
  if (videos.length === 0) {
    showToast('Chưa có video nào (được chọn) có đoạn cắt để xuất. Hãy cắt tự động trước.', 'warning')
    return
  }
  isExporting.value = true
  exportStartTime.value = Date.now()
  let okTotal = 0, clipTotal = 0
  try {
    const proj = namedProjects.value.find(p => p.id === activeProjectId.value)
    const projName = proj ? proj.name : 'Project'

    for (const path of videos) {
      const clips = clipsMap.value[path] || []
      
      // Áp cấu hình chế cháo trước khi xuất hàng loạt
      if (globalRemix.autoApply) {
        clips.forEach(c => applyGlobalRemixToClip(c))
      }

      const base = (path.split('\\').pop() || 'video').replace(/\.[^.]+$/, '')
      const exportDestDir = autoCreateSubfolders.value ? outDir.value + '\\' + base + '\\video' : outDir.value + '\\' + base
      const imageDestDir = exportWithThumbnails.value
        ? (autoCreateSubfolders.value ? outDir.value + '\\' + base + '\\image' : outImageDir.value + '\\' + base)
        : ''
      exportProgress.value = { done: 0, total: clips.length }
      exportStatusText.value = 'Đang bắt đầu...'
      videoDoneCount.value = 0
      thumbDoneCount.value = 0
      const results = await ExportClips(projName, path, clips, exportDestDir, imageDestDir, analyzerConfig, exportJobs.value)
      
      const stopped = results.some(r => r.error === 'Tiến trình xuất bị dừng' || r.error === 'Tiến trình bị dừng')
      if (stopped) {
        addLog('Tiến trình xuất hàng loạt đã bị dừng.')
        return
      }

      const okIds = new Set(results.filter(r => r.ok).map(r => r.clipId))
      clips.forEach(c => { if (okIds.has(c.id)) c.status = 'completed' })
      okTotal += results.filter(r => r.ok).length
      clipTotal += results.length
    }
    showToast(`Xuất hàng loạt xong: ${okTotal}/${clipTotal} clip từ ${videos.length} video.`, 'success')
  } catch (err) {
    showToast('Lỗi xuất hàng loạt: ' + err, 'error')
  } finally {
    isExporting.value = false
    exportStartTime.value = null
  }
}

const loadProject = async (projId: string) => {
  const proj = namedProjects.value.find(p => p.id === projId)
  if (!proj) return

  activeProjectId.value = projId
  
  videoPaths.value = [...proj.videoPaths]
  selectedVideos.value = new Set(proj.selectedVideos)
  outDir.value = proj.outDir && proj.outDir !== 'D:\\Output' ? proj.outDir : globalSettingsOutDir.value
  exportJobs.value = proj.exportJobs || 2
  analyzeJobs.value = proj.analyzeJobs || 1
  
  if (proj.globalRemix) {
    Object.assign(globalRemix, proj.globalRemix)
    if (globalRemix.musicLoop === undefined) globalRemix.musicLoop = false
    if (!globalRemix.musicTracks) globalRemix.musicTracks = []
  }

  if (proj.analyzerConfig) {
    Object.assign(analyzerConfig, proj.analyzerConfig)
  } else {
    // Kế thừa từ settings.json thay vì reset cứng
    if (globalSettingsConfig.value) {
      Object.assign(analyzerConfig, JSON.parse(JSON.stringify(globalSettingsConfig.value)))
    } else {
      analyzerConfig.mode = 'smart'
      analyzerConfig.sceneThreshold = 20.0
      analyzerConfig.minClipDuration = 1.0
      analyzerConfig.maxClipDuration = 30.0
      analyzerConfig.autoAcceptScore = 60
      analyzerConfig.reviewMinScore = 35
      analyzerConfig.exportPreset = 'fast'
      analyzerConfig.exportCRF = 23
      analyzerConfig.hardwareAccel = 'none'
    }
  }

  clipsMap.value = {}
  for (const path of videoPaths.value) {
    try {
      const pData = await LoadProjectBySource(path)
      if (pData && pData.clips) {
        clipsMap.value[path] = pData.clips
      }
    } catch(err) {
      console.error("Lỗi load clips cho video:", path, err)
    }
  }

  if (videoPaths.value.length > 0) {
    activeVideoIndex.value = 0
    activeVideoSrc.value = await GetStreamURL(videoPaths.value[0])
    await loadVideoInfo(videoPaths.value[0])
  } else {
    activeVideoIndex.value = -1
    activeVideoSrc.value = ''
    videoInfo.value = null
  }
  
  localStorage.setItem('activeProjectId', projId)
  addLog(`Đã chuyển sang dự án: ${proj.name}`)
}

const saveActiveProjectState = () => {
  if (!activeProjectId.value) return
  const index = namedProjects.value.findIndex(p => p.id === activeProjectId.value)
  if (index === -1) return

  namedProjects.value[index].videoPaths = [...videoPaths.value]
  namedProjects.value[index].selectedVideos = [...selectedVideos.value]
  namedProjects.value[index].outDir = outDir.value
  namedProjects.value[index].exportJobs = exportJobs.value
  namedProjects.value[index].analyzeJobs = analyzeJobs.value
  namedProjects.value[index].globalRemix = JSON.parse(JSON.stringify(globalRemix))
  namedProjects.value[index].analyzerConfig = JSON.parse(JSON.stringify(analyzerConfig))

  localStorage.setItem('namedProjectsList', JSON.stringify(namedProjects.value))
  localStorage.setItem('activeProjectId', activeProjectId.value)
}

watch(analyzerConfig, () => {
  saveActiveProjectState()
}, { deep: true })

watch(videoPaths, () => {
  saveActiveProjectState()
}, { deep: true })

watch(outDir, (newVal) => {
  if (newVal) {
    globalSettingsOutDir.value = newVal
  }
  saveActiveProjectState()
})

watch(exportJobs, () => {
  saveActiveProjectState()
})

watch(analyzeJobs, () => {
  saveActiveProjectState()
})

watch(selectedVideos, () => {
  saveActiveProjectState()
}, { deep: true })

watch(globalRemix, () => {
  saveActiveProjectState()
}, { deep: true })

watch(() => activeClips.value.map(c => c.id).join(','), (newIdsStr) => {
  const validIds = new Set(newIdsStr.split(',').filter(Boolean))
  const newSelected = new Set<string>()
  for (const id of selectedClips.value) {
    if (validIds.has(id)) {
      newSelected.add(id)
    }
  }
  selectedClips.value = newSelected
})

const openCreateProject = () => {
  newProjectName.value = ''
  showProjectModal.value = true
}

const createProject = () => {
  const name = newProjectName.value.trim()
  if (!name) return
  const id = 'proj_' + Date.now()
  const newProj: NamedProject = {
    id,
    name,
    videoPaths: [],
    selectedVideos: [],
    globalRemix: {
      autoApply: true,
      hflip: false,
      aspectEnabled: false,
      aspectRatio: '9:16',
      aspectMode: 'blur',
      speed: 1.0,
      colorEnabled: false,
      colorPreset: '',
      colorBrightness: 0,
      colorContrast: 0,
      colorSaturation: 1.0,
      musicPath: '',
      musicVolume: 0.3,
      muteOriginal: false
    },
    outDir: globalSettingsOutDir.value,
    exportJobs: 2,
    analyzeJobs: 1,
    createdAt: Date.now(),
    analyzerConfig: JSON.parse(JSON.stringify(analyzerConfig))
  }
  namedProjects.value.push(newProj)
  saveActiveProjectState()
  loadProject(id)
  showProjectModal.value = false
}

const openManageProjects = () => {
  showManageProjectsModal.value = true
}

const deleteNamedProject = async (id: string) => {
  if (namedProjects.value.length <= 1) {
    showToast("Không thể xóa dự án duy nhất!", 'error')
    return
  }
  if (!await showCustomConfirm("Bạn có chắc chắn muốn xóa dự án này? (Dữ liệu phân đoạn của các video đã phân tích vẫn được lưu ở SQLite)")) {
    return
  }
  
  const index = namedProjects.value.findIndex(p => p.id === id)
  if (index === -1) return
  
  namedProjects.value.splice(index, 1)
  
  if (activeProjectId.value === id) {
    const nextProj = namedProjects.value[0]
    loadProject(nextProj.id)
  } else {
    saveActiveProjectState()
  }
}

const removeVideo = async (index: number) => {
  const path = videoPaths.value[index]
  if (!await showCustomConfirm(`Bạn có muốn xóa video "${path.split('\\').pop()}" khỏi dự án này không?`)) {
    return
  }
  videoPaths.value.splice(index, 1)
  selectedVideos.value.delete(path)
  delete clipsMap.value[path]

  if (activeVideoIndex.value >= videoPaths.value.length) {
    activeVideoIndex.value = Math.max(0, videoPaths.value.length - 1)
  }
  
  if (videoPaths.value.length > 0) {
    selectVideo(activeVideoIndex.value)
  } else {
    activeVideoIndex.value = -1
    activeVideoSrc.value = ''
    videoInfo.value = null
  }
  addLog(`Đã xóa video khỏi danh sách: ${path.split('\\').pop()}`)
}

const removeSelectedVideos = async () => {
  const count = selectedVideos.value.size
  if (count === 0) return
  if (!await showCustomConfirm(`Bạn có chắc muốn xóa tất cả ${count} video đã chọn khỏi dự án không?`)) {
    return
  }
  
  const activePathBefore = activeVideoPath.value
  const remaining = videoPaths.value.filter(p => !selectedVideos.value.has(p))
  
  selectedVideos.value.forEach(path => {
    delete clipsMap.value[path]
  })
  
  videoPaths.value = remaining
  selectedVideos.value = new Set()
  
  if (videoPaths.value.length > 0) {
    const newIdx = videoPaths.value.indexOf(activePathBefore)
    if (newIdx >= 0) {
      activeVideoIndex.value = newIdx
    } else {
      activeVideoIndex.value = 0
    }
    selectVideo(activeVideoIndex.value)
  } else {
    activeVideoIndex.value = -1
    activeVideoSrc.value = ''
    videoInfo.value = null
  }
  
  addLog(`Đã xóa ${count} video khỏi danh sách.`)
}

const playExportedClip = async (clip: any) => {
  isPlayingExported.value = true
  currentPlayingClipIdx.value = clip.index
  // Dùng đường dẫn chính xác từ backend (lưu khi xuất xong)
  const filePath = clip.exportedPath || (outDir.value + '\\' + clip.id + '.mp4')
  activeExportedSrc.value = await GetStreamURL(filePath)
}

const stopPlayingExported = () => {
  isPlayingExported.value = false
  activeExportedSrc.value = ''
}

const removeClip = async (index: number) => {
  if (clipsMap.value[activeVideoPath.value]) {
    clipsMap.value[activeVideoPath.value].splice(index, 1)
    reindexClips()
    showToast('Đã xóa clip.', 'info')
    await saveProject()
  }
}

const removeSelectedClips = async () => {
  const count = selectedClips.value.size
  if (count === 0) return
  if (!await showCustomConfirm(`Bạn có chắc muốn xóa tất cả ${count} clip đã chọn không?`)) {
    return
  }
  
  const idsToDelete = new Set(selectedClips.value)
  
  for (const path in clipsMap.value) {
    const clips = clipsMap.value[path]
    if (!clips) continue
    clipsMap.value[path] = clips.filter(c => !idsToDelete.has(c.id))
  }
  
  for (const path in clipsMap.value) {
    const clips = clipsMap.value[path]
    if (!clips) continue
    clips.forEach((c, i) => {
      c.index = i + 1
      c.id = `clip_${i + 1}`
    })
  }
  
  selectedClips.value = new Set()
  showToast(`Đã xóa thành công ${count} clip đã chọn.`, 'success')
  await saveProject()
}

const onClipTimeChange = async (clip: project.Clip) => {
  updateClipDuration(clip)
  await refreshClipThumbs(clip)
  await saveProject()
}

const updateClipDuration = (clip: project.Clip) => {
  if (clip.startTime === null || clip.startTime === undefined || isNaN(clip.startTime)) {
    clip.startTime = 0
  }
  if (clip.endTime === null || clip.endTime === undefined || isNaN(clip.endTime)) {
    clip.endTime = videoInfo.value ? Math.round(videoInfo.value.Duration) : 10
  }
  
  const maxDur = videoInfo.value ? Math.round(videoInfo.value.Duration) : 99999
  
  clip.startTime = Math.max(0, Math.min(Math.round(clip.startTime), maxDur - 1))
  clip.endTime = Math.max(clip.startTime + 1, Math.min(Math.round(clip.endTime), maxDur))
  clip.duration = Math.round(clip.endTime - clip.startTime)
}

// Đánh lại index + id cho toàn bộ clip của video hiện tại sau khi chia/gộp/xóa.
const reindexClips = () => {
  const clips = clipsMap.value[activeVideoPath.value]
  if (!clips) return
  clips.forEach((c, i) => {
    c.index = i + 1
    c.id = `clip_${i + 1}`
  })
}

// Refresh thumbnail đầu + cuối cho một clip (gọi sau khi sửa mốc thời gian).
const refreshClipThumbs = async (clip: project.Clip) => {
  try {
    clip.thumbnail = await GenerateThumbnail(activeVideoPath.value, clip.startTime)
    const endT = Math.max(clip.startTime, clip.endTime - 0.1)
    clip.thumbEnd = await GenerateThumbnail(activeVideoPath.value, endT)
  } catch (e) {
    console.error('Lỗi tạo thumbnail:', e)
  }
}

// Chia một clip tại thời điểm splitAt (giây, tuyệt đối trong video).
const splitClip = async (index: number, splitAt: number) => {
  const clips = clipsMap.value[activeVideoPath.value]
  if (!clips) return
  const clip = clips[index]
  // Phải nằm trong lòng clip (chừa biên nhỏ để không tạo clip 0 giây).
  if (splitAt <= clip.startTime + 0.05 || splitAt >= clip.endTime - 0.05) {
    addLog('Điểm chia phải nằm trong clip (cách hai đầu ít nhất 0.05s).')
    return
  }
  const second = project.Clip.createFrom({
    ...clip,
    startTime: splitAt,
    endTime: clip.endTime,
    duration: clip.endTime - splitAt,
    status: 'pending',
    tier: 'review',
    confidence: 0,
    reason: 'Chia thủ công',
    thumbnail: '',
    thumbEnd: clip.thumbEnd,
  })
  clip.endTime = splitAt
  clip.duration = splitAt - clip.startTime
  clips.splice(index + 1, 0, second)
  reindexClips()
  await refreshClipThumbs(clip)
  await refreshClipThumbs(second)
  addLog(`Đã chia Clip #${index + 1} tại ${formatTime(splitAt)}.`)
  await saveProject()
}

// Chia clip tại vị trí đang phát của video player.
const splitAtPlayhead = (index: number) => {
  const t = videoPlayer.value?.currentTime
  if (t === undefined) {
    addLog('Chưa có vị trí phát để chia.')
    return
  }
  splitClip(index, t)
}

// Gộp clip index với clip liền sau nó.
const mergeWithNext = async (index: number) => {
  const clips = clipsMap.value[activeVideoPath.value]
  if (!clips || index >= clips.length - 1) return
  const curr = clips[index]
  const next = clips[index + 1]
  curr.endTime = next.endTime
  curr.duration = curr.endTime - curr.startTime
  curr.thumbEnd = next.thumbEnd
  curr.status = 'pending'
  clips.splice(index + 1, 1)
  reindexClips()
  addLog(`Đã gộp Clip #${index + 1} với clip kế tiếp.`)
  await saveProject()
}

// Gộp clip index với clip liền trước nó.
const mergeWithPrev = (index: number) => {
  if (index <= 0) return
  mergeWithNext(index - 1)
}

// === LƯU / MỞ PROJECT (SQLite qua backend) ===

// Lưu phiên làm việc hiện tại (clip + config) để mở lại sau.
const saveProject = async () => {
  if (!activeVideoPath.value) {
    addLog('Chưa có video để lưu.')
    return
  }
  try {
    const clips = clipsMap.value[activeVideoPath.value] || []
    await SaveProject(activeVideoPath.value, clips, analyzerConfig)
    addLog('Đã lưu phiên làm việc.')
  } catch (e) {
    addLog('Lỗi lưu project: ' + String(e))
  }
}

// Tải danh sách project đã lưu (mở modal "gần đây").
const openRecentProjects = async () => {
  try {
    recentProjects.value = await ListProjects() || []
    showRecent.value = true
  } catch (e) {
    addLog('Lỗi tải danh sách project: ' + String(e))
  }
}

// Khôi phục một project đã lưu vào workspace.
const restoreProject = async (summary: storage.ProjectSummary) => {
  try {
    const p = await LoadProjectBySource(summary.sourcePath)
    if (!p) {
      addLog('Không tìm thấy project.')
      return
    }
    // Thêm video vào danh sách nếu chưa có, rồi nạp clip + config.
    if (!videoPaths.value.includes(p.sourcePath)) {
      videoPaths.value = [...videoPaths.value, p.sourcePath]
    }
    activeVideoIndex.value = videoPaths.value.indexOf(p.sourcePath)
    clipsMap.value[p.sourcePath] = p.clips || []
    if (p.config) Object.assign(analyzerConfig, p.config)
    activeVideoSrc.value = await GetStreamURL(p.sourcePath)
    await loadVideoInfo(p.sourcePath)
    showRecent.value = false
    addLog(`Đã mở lại "${p.name}" (${(p.clips || []).length} clip).`)
  } catch (e) {
    addLog('Lỗi mở project: ' + String(e))
  }
}

// Xóa một project khỏi lịch sử.
const deleteProject = async (summary: storage.ProjectSummary) => {
  try {
    await DeleteProject(summary.id)
    recentProjects.value = recentProjects.value.filter(p => p.id !== summary.id)
    addLog('Đã xóa project khỏi lịch sử.')
  } catch (e) {
    addLog('Lỗi xóa project: ' + String(e))
  }
}

// === EDIT CLIP (GĐ5+6): tỉ lệ / màu / tốc độ / chữ / watermark / nhạc nền ===

// EditOps trung tính (khớp DefaultEditOps bên Go) — dùng khi clip chưa có edit.
const defaultEdit = (): project.EditOps => project.EditOps.createFrom({
  aspect: { enabled: false, ratio: '9:16', mode: 'crop' },
  color: { enabled: false, brightness: 0, contrast: 0, saturation: 1, preset: '' },
  speed: 1.0,
  hflip: false, // Lật ngang mặc định tắt
  texts: [],
  watermark: { enabled: false, imgPath: '', x: '', y: '', opacity: 1, scale: 0.2 },
  audio: { volume: 1, mute: false, musicPath: '', musicVolume: 0.3, fadeIn: 0, fadeOut: 0, musicLoop: false, musicTracks: [] },
  transition: { type: '', duration: 0 }
})

// Đảm bảo clip có đối tượng edit hợp lệ (clip cũ từ phân tích/khôi phục có thể thiếu field mới).
const ensureEdit = (clip: project.Clip) => {
  const d = defaultEdit()
  if (!clip.edit) clip.edit = d
  if (!clip.edit.aspect) clip.edit.aspect = d.aspect
  if (!clip.edit.color) clip.edit.color = d.color
  if (!clip.edit.speed || clip.edit.speed <= 0) clip.edit.speed = 1.0
  if (clip.edit.hflip === undefined) clip.edit.hflip = false
  if (!clip.edit.texts) clip.edit.texts = []
  if (!clip.edit.watermark) clip.edit.watermark = d.watermark
  if (!clip.edit.audio) clip.edit.audio = d.audio
  if (clip.edit.audio.musicLoop === undefined) clip.edit.audio.musicLoop = false
  if (!clip.edit.audio.musicTracks) clip.edit.audio.musicTracks = []
  if (!clip.edit.transition) clip.edit.transition = d.transition
}

// Áp dụng cấu hình chế cháo chống bản quyền cho một clip
const applyGlobalRemixToClip = (clip: project.Clip) => {
  ensureEdit(clip)
  if (globalRemix.autoApply) {
    clip.edit.hflip = globalRemix.hflip
    clip.edit.aspect.enabled = globalRemix.aspectEnabled
    clip.edit.aspect.ratio = globalRemix.aspectRatio
    clip.edit.aspect.mode = globalRemix.aspectMode
    clip.edit.speed = globalRemix.speed
    clip.edit.color.enabled = globalRemix.colorEnabled
    clip.edit.color.preset = globalRemix.colorPreset
    clip.edit.color.brightness = globalRemix.colorBrightness
    clip.edit.color.contrast = globalRemix.colorContrast
    clip.edit.color.saturation = globalRemix.colorSaturation
    clip.edit.audio.musicPath = globalRemix.musicPath
    clip.edit.audio.musicVolume = globalRemix.musicVolume
    clip.edit.audio.mute = globalRemix.muteOriginal
    clip.edit.audio.musicLoop = globalRemix.musicLoop
    clip.edit.audio.musicTracks = globalRemix.musicTracks ? [...globalRemix.musicTracks] : []
  }
}

// Áp dụng cấu hình chế cháo cho toàn bộ clip của video hiện tại
const applyGlobalRemixToAllActive = () => {
  const clips = activeClips.value
  if (!clips || clips.length === 0) {
    addLog('Chưa có clip nào để áp dụng cấu hình.')
    return
  }
  for (const c of clips) {
    ensureEdit(c)
    c.edit.hflip = globalRemix.hflip
    c.edit.aspect.enabled = globalRemix.aspectEnabled
    c.edit.aspect.ratio = globalRemix.aspectRatio
    c.edit.aspect.mode = globalRemix.aspectMode
    c.edit.speed = globalRemix.speed
    c.edit.color.enabled = globalRemix.colorEnabled
    c.edit.color.preset = globalRemix.colorPreset
    c.edit.color.brightness = globalRemix.colorBrightness
    c.edit.color.contrast = globalRemix.colorContrast
    c.edit.color.saturation = globalRemix.colorSaturation
    c.edit.audio.musicPath = globalRemix.musicPath
    c.edit.audio.musicVolume = globalRemix.musicVolume
    c.edit.audio.mute = globalRemix.muteOriginal
    c.edit.audio.musicLoop = globalRemix.musicLoop
    c.edit.audio.musicTracks = globalRemix.musicTracks ? [...globalRemix.musicTracks] : []
  }
  addLog('Đã áp dụng cấu hình chế cháo cho tất cả clip của video hiện tại.')
  showToast('Đã áp dụng cấu hình chế cháo cho tất cả clip của video hiện tại!', 'success')
}

const editingClip = computed(() => {
  const clips = activeClips.value
  if (editClipIdx.value < 0 || editClipIdx.value >= clips.length) return null
  return clips[editClipIdx.value]
})

const openEdit = (index: number) => {
  const clip = activeClips.value[index]
  if (!clip) return
  ensureEdit(clip)
  editClipIdx.value = index
  showEdit.value = true

  // Reset AI thumbnail generator state
  aiThumbState.isExtractingFrames = false
  aiThumbState.extractedFrames = []
  aiThumbState.selectedFrames = new Set()
  aiThumbState.userPrompt = analyzerConfig.prompt || ''
  aiThumbState.isGenerating = false
  aiThumbState.generatedImage = ''
  aiThumbState.statusText = ''
  aiThumbState.aspectRatio = clip.edit?.aspect?.ratio || '9:16'

  // Đưa player về đầu clip để xem trước khi chỉnh.
  jumpToTime(clip.startTime)
}

const closeEdit = () => {
  showEdit.value = false
  editClipIdx.value = -1
}

// Điều hướng clip trước/sau ngay trong màn hình chỉnh sửa.
const editPrevClip = () => {
  if (editClipIdx.value > 0) openEdit(editClipIdx.value - 1)
}
const editNextClip = () => {
  if (editClipIdx.value < activeClips.value.length - 1) openEdit(editClipIdx.value + 1)
}

// --- Chữ / phụ đề (TextOp) ---
const addText = () => {
  const clip = editingClip.value
  if (!clip) return
  ensureEdit(clip)
  clip.edit.texts.push(project.TextOp.createFrom({
    content: 'Nội dung chữ', fontSize: 48, color: 'white',
    x: '(w-text_w)/2', y: 'h-text_h-80', startTime: 0, endTime: 0, bgBox: true,
  }))
}
const removeText = (i: number) => {
  const clip = editingClip.value
  if (!clip) return
  clip.edit.texts.splice(i, 1)
}
// Preset vị trí nhanh cho một dòng chữ.
const setTextPos = (t: project.TextOp, pos: string) => {
  if (pos === 'top') { t.x = '(w-text_w)/2'; t.y = '60' }
  else if (pos === 'center') { t.x = '(w-text_w)/2'; t.y = '(h-text_h)/2' }
  else { t.x = '(w-text_w)/2'; t.y = 'h-text_h-80' }
}

// --- Watermark ---
const pickWatermark = async () => {
  const clip = editingClip.value
  if (!clip) return
  try {
    const p = await SelectImageFile()
    if (p) {
      clip.edit.watermark.imgPath = p
      clip.edit.watermark.enabled = true
    }
  } catch (e) { addLog('Lỗi chọn ảnh: ' + String(e)) }
}
const wmName = computed(() => {
  const p = editingClip.value?.edit?.watermark?.imgPath || ''
  return p ? p.split('\\').pop() : ''
})

// --- Nhạc nền ---
const pickMusic = async () => {
  const clip = editingClip.value
  if (!clip) return
  try {
    const p = await SelectAudioFile()
    if (p) clip.edit.audio.musicPath = p
  } catch (e) { addLog('Lỗi chọn nhạc: ' + String(e)) }
}
const clearMusic = () => {
  const clip = editingClip.value
  if (clip) clip.edit.audio.musicPath = ''
}
const musicName = computed(() => {
  const p = editingClip.value?.edit?.audio?.musicPath || ''
  return p ? p.split('\\').pop() : ''
})

// Tóm tắt các hiệu ứng đang bật trên clip đang chỉnh (hiển thị chip ở màn hình sửa).
const editSummary = computed<string[]>(() => {
  const clip = editingClip.value
  const tags: string[] = []
  const e = clip?.edit
  if (!e) return tags
  if (e.hflip) tags.push('lật')
  if (e.aspect?.enabled) tags.push(e.aspect.ratio)
  if (e.color?.enabled || e.color?.preset) tags.push('màu')
  if (e.speed && e.speed !== 1) tags.push(e.speed + '×')
  if (e.texts?.length) tags.push('chữ ' + e.texts.length)
  if (e.watermark?.enabled && e.watermark?.imgPath) tags.push('logo')
  if (e.audio?.musicPath) tags.push('nhạc')
  if ((e.audio?.fadeIn || 0) > 0 || (e.audio?.fadeOut || 0) > 0) tags.push('fade')
  if (e.transition?.type && e.transition?.duration > 0) tags.push('trans')
  return tags
})

// Chip tóm tắt hiệu ứng cho MỘT clip bất kỳ (hiển thị ở thẻ clip danh sách).
const clipEditChips = (clip: project.Clip): string[] => {
  const tags: string[] = []
  const e = clip.edit
  if (!e) return tags
  if (e.hflip) tags.push('lật')
  if (e.aspect?.enabled) tags.push(e.aspect.ratio)
  if (e.color?.enabled || e.color?.preset) tags.push('màu')
  if (e.speed && e.speed !== 1) tags.push(e.speed + '×')
  if (e.texts?.length) tags.push('chữ ' + e.texts.length)
  if (e.watermark?.enabled && e.watermark?.imgPath) tags.push('logo')
  if (e.audio?.musicPath) tags.push('nhạc')
  if ((e.audio?.fadeIn || 0) > 0 || (e.audio?.fadeOut || 0) > 0) tags.push('fade')
  if (e.transition?.type && e.transition?.duration > 0) tags.push('trans')
  return tags
}

// Lớp tỉ lệ khung cho khung xem trước (khi bật đổi tỉ lệ, khung preview mô phỏng theo).
const previewAspectClass = computed(() => {
  const a = editingClip.value?.edit?.aspect
  if (!a?.enabled) return '16-9'
  if (a.ratio === '9:16') return '9-16'
  if (a.ratio === '1:1') return '1-1'
  return '16-9'
})

// Vị trí watermark: ánh xạ preset góc <-> biểu thức x/y của ffmpeg overlay.
const wmPosition = computed<string>({
  get() {
    const w = editingClip.value?.edit?.watermark
    if (!w) return 'tr'
    const x = w.x, y = w.y
    if (x === '20' && y === '20') return 'tl'
    if (x === 'W-w-20' && y === '20') return 'tr'
    if (x === '20' && y === 'H-h-20') return 'bl'
    if (x === '(W-w)/2' && y === '(H-h)/2') return 'c'
    return 'br'
  },
  set(pos: string) {
    const w = editingClip.value?.edit?.watermark
    if (!w) return
    const map: Record<string, [string, string]> = {
      tl: ['20', '20'],
      tr: ['W-w-20', '20'],
      bl: ['20', 'H-h-20'],
      br: ['W-w-20', 'H-h-20'],
      c: ['(W-w)/2', '(H-h)/2'],
    }
    const [x, y] = map[pos] || map.br
    w.x = x
    w.y = y
  },
})

// Áp thao tác edit của clip đang mở cho TẤT CẢ clip của video hiện tại
// (thường dùng để đổi cùng tỉ lệ / màu cho cả bộ).
const applyEditToAll = () => {
  const src = editingClip.value
  if (!src) return
  const clips = clipsMap.value[activeVideoPath.value]
  if (!clips) return
  for (const c of clips) {
    // Deep-clone qua createFrom để không chia sẻ tham chiếu giữa các clip.
    c.edit = project.EditOps.createFrom(JSON.parse(JSON.stringify(src.edit)))
  }
  addLog(`Đã áp chỉnh sửa cho tất cả ${clips.length} clip.`)
}

// Áp bộ chỉnh sửa của clip đang mở cho MỌI clip của MỌI video đang được tick
// (chỉnh sửa hàng loạt nhiều video). Chỉ áp lên video đã phân tích (có clip).
const applyEditToSelectedVideos = () => {
  const src = editingClip.value
  if (!src) return
  const targets = videosForBatch()
  let videoCount = 0
  let clipCount = 0
  for (const path of targets) {
    const clips = clipsMap.value[path]
    if (!clips || clips.length === 0) continue
    for (const c of clips) {
      c.edit = project.EditOps.createFrom(JSON.parse(JSON.stringify(src.edit)))
      clipCount++
    }
    videoCount++
  }
  if (videoCount === 0) {
    showToast('Chưa có video nào (đã tick) được phân tích để áp chỉnh sửa.', 'warning')
    return
  }
  addLog(`Đã áp chỉnh sửa cho ${clipCount} clip của ${videoCount} video.`)
  showToast(`Đã áp bộ chỉnh sửa cho ${clipCount} clip thuộc ${videoCount} video.`, 'success')
}

const resetEdit = () => {
  const clip = editingClip.value
  if (!clip) return
  clip.edit = defaultEdit()
  addLog(`Đã đặt lại chỉnh sửa Clip #${clip.index}.`)
}

const jumpToTime = (time: number) => {
  if (videoPlayer.value) {
    videoPlayer.value.currentTime = time
    videoPlayer.value.play().catch(() => {})
  }
}

const formatTime = (seconds: number) => {
  if (seconds === undefined || seconds === null || seconds < 0) return '00:00'
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  
  const pad = (num: number) => String(num).padStart(2, '0')
  
  if (h > 0) {
    return `${pad(h)}:${pad(m)}:${pad(s)}`
  }
  return `${pad(m)}:${pad(s)}`
}

const formatSize = (bytes: number) => {
  if (!bytes) return ''
  const mb = bytes / (1024 * 1024)
  if (mb >= 1024) {
    return (mb / 1024).toFixed(2) + ' GB'
  }
  return mb.toFixed(1) + ' MB'
}
</script>

<template>
  <main class="app-container">
    <!-- Navbar / Header trên cùng -->
    <header class="header">
      <div class="header-left">
        <Video class="header-icon" :size="26" />
        <h1>Smart Splitter</h1>
        
        <!-- Bảng chọn dự án -->
        <div class="project-selector-container" :class="{ 'panel-disabled': isAnalyzing || isExporting }">
          <label class="project-selector-lbl">Dự án:</label>
          <select :disabled="isAnalyzing || isExporting" :value="activeProjectId" @change="e => loadProject((e.target as HTMLSelectElement).value)" class="project-dropdown">
            <option v-for="p in namedProjects" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
          <button :disabled="isAnalyzing || isExporting" @click="openCreateProject" class="btn-create-proj-mini flex-center" title="Tạo dự án mới">
            <Plus :size="11" />
            Mới
          </button>
          <button :disabled="isAnalyzing || isExporting" @click="openManageProjects" class="btn-manage-proj-mini flex-center" title="Quản lý dự án">
            <Settings :size="11" />
            Quản lý
          </button>
        </div>
      </div>
      <div class="header-actions">
        <!-- Menu chọn chức năng chính (Segmented tabs cực đẹp) -->
        <div class="nav-segmented-control">
          <button class="nav-segment-btn" :class="{ active: activeView === 'split' }" @click="activeView = 'split'">
            <Scissors :size="13" /> Cắt Video
          </button>
          <button class="nav-segment-btn" :class="{ active: activeView === 'download-video' }" @click="activeView = 'download-video'">
            <Download :size="13" /> Tải Video
          </button>
          <button class="nav-segment-btn" :class="{ active: activeView === 'download-image' }" @click="activeView = 'download-image'">
            <ImageIcon :size="13" /> Tải Ảnh
          </button>
          <button class="nav-segment-btn" :class="{ active: activeView === 'ai-image' }" @click="activeView = 'ai-image'">
            <Sparkles :size="13" style="color: var(--wx-brand-accent);" /> Tạo Ảnh AI
          </button>
          <button class="nav-segment-btn" :class="{ active: activeView === 'ai-video' }" @click="activeView = 'ai-video'">
            <Sparkles :size="13" style="color: var(--wx-brand-accent);" /> Tạo Video AI
          </button>
        </div>

 
        <!-- Mở dự án gần đây -->
        <button :disabled="isAnalyzing || isExporting" @click="openRecentProjects" class="icon-btn-circle" title="Project đã lưu">
          <History :size="20" />
        </button>

        <!-- Lưu dự án -->
        <button :disabled="isAnalyzing || isExporting" @click="saveProject" class="icon-btn-circle" title="Lưu phiên làm việc" v-if="activeClips.length > 0">
          <Save :size="20" />
        </button>
 
        <!-- Nút chuyển chế độ Sáng/Tối -->
        <button @click="toggleColorScheme" class="icon-btn-circle theme-toggle-btn" :title="isDark ? 'Chuyển sang giao diện Sáng' : 'Chuyển sang giao diện Tối'">
          <Sun v-if="isDark" :size="20" style="color: var(--wx-brand-accent);" />
          <Moon v-else :size="20" style="color: var(--wx-brand-primary);" />
        </button>

        <!-- Nút Cài đặt chung -->
        <button :disabled="isAnalyzing || isExporting" @click="showSettings = true" class="icon-btn-circle" title="Cài đặt chung">
          <Settings :size="20" />
        </button>

      </div>
    </header>

    <div class="app-body-vertical" :class="{ 'no-scroll': activeView !== 'split' }">
      <template v-if="activeView === 'split'">
      <!-- BỐ CỤC TRÊN: DANH SÁCH VIDEO GỐC -->
      <section class="section-top-videos">
        <div class="section-title-bar" style="display: flex; align-items: center; justify-content: space-between; gap: 12px;">
          <div class="title-left" style="display: flex; align-items: center; gap: 8px;">
            <h2>Danh sách Video Gốc</h2>
            <span class="video-counter" style="margin-right: 8px;">({{ videoPaths.length }} video)</span>

            <!-- Xóa video đã chọn & Chọn tất cả (bên trái) -->
            <label v-if="videoPaths.length > 0" :class="{ 'panel-disabled': isAnalyzing || isExporting }" class="select-all-label" @click.stop style="display: inline-flex; align-items: center; gap: 6px; height: 28px; font-size: 12px;">
              <input type="checkbox" :checked="isAllVideosSelected" @change="toggleSelectAllVideos" class="clip-checkbox" />
              <span>Chọn tất cả video để xử lý</span>
            </label>

            <button 
              v-if="videoPaths.length > 0 && selectedVideos.size > 0" 
              :disabled="isAnalyzing || isExporting"
              @click.stop="removeSelectedVideos" 
              class="btn-danger-compact" 
              style="height: 28px; box-sizing: border-box; padding: 5px 10px; font-size: 11px;"
            >
              Xóa {{ selectedVideos.size }} video đã chọn
            </button>
          </div>

          <div class="title-right-actions" style="display: flex; align-items: center; gap: 8px;">
            <!-- Nút chọn video gốc (bên phải) -->
            <button :disabled="isAnalyzing || isExporting" @click="handleSelectFiles" class="btn select-btn flex-center">
              <Plus :size="12" />
              Chọn Video Gốc
            </button>
            <!-- Nút Bắt Đầu Cắt Tự Động / Dừng Ngay (bên phải) -->
            <button v-if="!isAnalyzing" :disabled="videoPaths.length === 0 || isExporting" @click="analyzeAll" class="btn btn-analyze flex-center font-bold">
              <svg viewBox="0 0 24 24" width="12" height="12"><path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 14.5v-9l6 4.5-6 4.5z"/></svg>
              Cắt Video
            </button>
            <button v-else @click="cancelCurrentAnalysis" class="btn stop-analyze-btn flex-center font-bold">
              <svg viewBox="0 0 24 24" width="12" height="12"><path fill="currentColor" d="M6 6h12v12H6z"/></svg>
              Dừng Ngay ({{ processedVideosCount }}/{{ totalVideosCount }})
            </button>

            <!-- Nút Chỉ Xuất Video Gốc (Không Cắt) -->
            <button v-if="!isAnalyzing" :disabled="videoPaths.length === 0 || isExporting" @click="exportWithoutSplitting" class="btn btn-export-direct flex-center font-bold" title="Xuất trực tiếp các video gốc đang chọn (áp dụng cấu hình hiệu ứng/tốc độ ở bên trái, không cắt nhỏ)">
              <svg viewBox="0 0 24 24" width="12" height="12"><path fill="currentColor" d="M19 12v7H5v-7H3v7c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2v-7h-2zm-6 .67l2.59-2.58L17 11.5l-5 5-5-5 1.41-1.41L11 12.67V3h2v9.67z"/></svg>
              Xuất Video
            </button>
          </div>
        </div>

        <div class="video-horizontal-grid" :class="{ 'expanded-grid': isVideoListExpanded }" v-if="videoPaths.length > 0">
          <div v-for="(path, index) in videoPaths" :key="index"
               class="video-card-item"
               :class="{ active: index === activeVideoIndex, 'video-picked': selectedVideos.has(path) }"
               @click="selectVideo(index)"
               @mousedown="handleVideoMousedown(path, $event)"
               @mouseenter="handleVideoMouseenter(path)"
               :title="path">
            
            <div class="video-card-row">
              <label class="video-select-wrap" @click.stop>
                <input type="checkbox" class="clip-checkbox" :checked="selectedVideos.has(path)" @change="toggleVideoSelected(path)" :disabled="isAnalyzing || isExporting" />
              </label>
              
              <div class="video-card-icon">
                <FileVideo :size="16" />
              </div>
              
              <span class="video-card-name">{{ path.split('\\').pop() }}</span>
              
              <span class="status-dot dot-green" v-if="clipsMap[path] && clipsMap[path].length > 0" :title="`Đã cắt ${clipsMap[path].length} clip`"></span>
              <span class="status-dot dot-red" v-else title="Chưa cắt"></span>

              <button class="btn-remove-video" @click.stop="removeVideo(index)" title="Xóa video khỏi dự án" :disabled="isAnalyzing || isExporting" :style="isAnalyzing || isExporting ? { opacity: 0.4, pointerEvents: 'none' } : {}">
                <X :size="10" />
              </button>
            </div>

            <!-- Tiến trình quét -->
            <div class="video-card-progress" v-if="activeAnalyzingPaths.has(path)">
              <div class="progress-bar-track">
                <div class="progress-bar-fill" :style="{ width: (displayProgressMap[path] || 0) + '%' }"></div>
              </div>
              <span class="progress-text">{{ Math.round(displayProgressMap[path] || 0) }}% <span class="progress-eta" style="font-size: 9.5px; opacity: 0.8; margin-left: 4px;">({{ getAnalyzeETA(path) }})</span></span>
            </div>
          </div>
        </div>

        <!-- Nút Xem thêm / Thu gọn cho danh sách video gốc -->
        <div v-if="videoPaths.length > 5" style="display: flex; justify-content: center; margin-top: 4px; margin-bottom: 4px;">
          <button 
            type="button" 
            @click="isVideoListExpanded = !isVideoListExpanded" 
            class="btn-expand-list"
          >
            <span style="margin-right: 4px;">{{ isVideoListExpanded ? 'Thu gọn danh sách' : `Xem thêm (${videoPaths.length} video)` }}</span>
            <ChevronUp v-if="isVideoListExpanded" :size="12" />
            <ChevronDown v-else :size="12" />
          </button>
        </div>
        <div class="empty-videos-placeholder" v-else style="height: 48px; display: flex; align-items: center; justify-content: center; padding: 0 16px; box-sizing: border-box;">
          <div class="placeholder-content" style="display:flex; flex-direction:row; align-items:center; justify-content:center; gap:16px; width: 100%; height: 100%;">
            <div style="display: flex; align-items: center; gap: 8px;">
              <FileVideo :size="16" style="opacity: 0.5; color: var(--accent-color);" />
              <span style="font-size: 12px; color: var(--l-text-muted); font-weight: 500;">Chưa có video gốc nào trong dự án.</span>
            </div>
          </div>
        </div>
      </section>

      <!-- BỐ CỤC GIỮA: CẤU HÌNH & TRÌNH PHÁT PREVIEW -->
      <section class="section-middle-workspace">
        <!-- Bên Trái: Bảng Cấu Hình Dự Án (Cắt & Sửa) -->
        <div class="project-settings-panel" :class="{ 'panel-disabled': isAnalyzing || isExporting }">
          <div class="settings-section-header" style="margin-bottom: 6px;">
            <Scissors :size="14" style="color:var(--accent-color);" />
            <h3>Cấu hình cắt</h3>
          </div>
          
          <div class="compact-settings-group-list">
            <!-- Chế độ cắt -->
            <div class="compact-setting-row">
              <div class="cut-mode-compact-buttons">
                <button class="btn-cut-mode-pill" :class="{ active: analyzerConfig.mode === 'fast' }" @click="analyzerConfig.mode = 'fast'" title="Cắt nhanh theo điểm mốc cơ bản">Nhanh</button>
                <button class="btn-cut-mode-pill" :class="{ active: analyzerConfig.mode === 'smart' }" @click="analyzerConfig.mode = 'smart'" title="Cắt thông minh tự cân bằng">Tự động</button>
                <button class="btn-cut-mode-pill" :class="{ active: analyzerConfig.mode === 'precise' }" @click="analyzerConfig.mode = 'precise'" title="Cắt kỹ lưỡng chi tiết">Kỹ</button>
                <button class="btn-cut-mode-pill" :class="{ active: analyzerConfig.mode === 'fixed' }" @click="analyzerConfig.mode = 'fixed'" title="Cắt đều video thành các đoạn có độ dài cố định">Cắt đều</button>
              </div>
            </div>

            <!-- Thời lượng -->
            <div class="compact-setting-grid-2" v-if="analyzerConfig.mode !== 'fixed'">
              <div class="compact-setting-item">
                <label class="setting-title-lbl">Ngắn nhất (giây):</label>
                <input type="number" v-model.number="analyzerConfig.minClipDuration" min="1" max="60" @change="onMinDurationChange" class="compact-input" />
              </div>
              <div class="compact-setting-item">
                <label class="setting-title-lbl">Dài nhất (giây):</label>
                <input type="number" v-model.number="analyzerConfig.maxClipDuration" min="10" max="600" @change="onMaxDurationChange" class="compact-input" />
              </div>
            </div>
            <div class="compact-setting-row" v-else>
              <label class="setting-title-lbl">Thời lượng mỗi clip (giây):</label>
              <input type="number" v-model.number="analyzerConfig.maxClipDuration" min="5" max="600" @change="analyzerConfig.maxClipDuration = clampValue(analyzerConfig.maxClipDuration, 5, 600, 15)" class="compact-input" style="width: 100%;" />
            </div>

            <!-- Cài đặt nâng cao link -->
            <div style="text-align: center; margin: 2px 0;">
              <button class="btn-link-toggle" @click="showAdvancedCutSettings = !showAdvancedCutSettings" style="background: transparent; border: none; color: var(--accent-color); font-size: 11px; font-weight: 700; cursor: pointer; display: inline-flex; align-items: center; gap: 4px; justify-content: center;">
                {{ showAdvancedCutSettings ? 'Thu gọn tùy chọn khác' : 'Xem thêm tùy chọn khác' }}
                <svg viewBox="0 0 24 24" width="10" height="10" style="transition: transform 0.2s;" :style="{ transform: showAdvancedCutSettings ? 'rotate(180deg)' : 'rotate(0)' }"><path fill="currentColor" d="M7 10l5 5 5-5z"/></svg>
              </button>
            </div>

            <!-- Tùy chọn khác ẩn/hiện -->
            <div v-if="showAdvancedCutSettings" style="display: flex; flex-direction: column; gap: 8px; background-color: var(--wx-surface-sunken); padding: 8px; border-radius: 8px; border: 1px solid var(--border-color);">
              <div class="compact-setting-grid-2">
                <div class="compact-setting-item">
                  <label class="setting-title-lbl">Độ nhạy cắt (5-80):</label>
                  <input type="number" v-model.number="analyzerConfig.sceneThreshold" min="5" max="80" @change="analyzerConfig.sceneThreshold = clampValue(analyzerConfig.sceneThreshold, 5, 80, 27)" class="compact-input" />
                </div>
                <div class="compact-setting-item">
                  <label class="setting-title-lbl">Độ nét (CRF):</label>
                  <input type="number" v-model.number="analyzerConfig.exportCRF" min="0" max="51" @change="analyzerConfig.exportCRF = clampValue(analyzerConfig.exportCRF, 0, 51, 23)" class="compact-input" />
                </div>
              </div>
              <div class="compact-setting-row">
                <label class="setting-title-lbl">Tốc độ xuất file:</label>
                <select v-model="analyzerConfig.exportPreset" class="compact-select">
                  <option value="ultrafast">Rất nhanh (máy yếu)</option>
                  <option value="fast">Nhanh (mặc định)</option>
                  <option value="medium">Vừa phải</option>
                  <option value="slow">Chậm (chất lượng cao)</option>
                </select>
              </div>
              <div class="compact-setting-row" style="margin-top: 8px;">
                <label class="setting-title-lbl">Tăng tốc phần cứng (GPU):</label>
                <select v-model="analyzerConfig.hardwareAccel" class="compact-select">
                  <option value="auto">🚀 Tự động phát hiện (khuyên dùng)</option>
                  <option value="nvidia">NVIDIA (Card rời NVIDIA)</option>
                  <option value="intel">Intel (Card tích hợp Intel)</option>
                  <option value="amd">AMD (Card rời AMD)</option>
                  <option value="none">❌ Không tăng tốc (chỉ dùng CPU)</option>
                </select>
              </div>
            </div>
          </div>

          <div style="border-bottom: 1px solid var(--border-color); margin: 6px 0;"></div>

          <!-- Phần 2: Cấu hình chỉnh sửa -->
          <div class="settings-section-header" style="margin-bottom: 6px;">
            <Film :size="14" style="color:var(--accent-color);" />
            <h3>Cấu hình sửa</h3>
          </div>

          <div class="remix-options-grid-compact">
            <!-- 1. Lật ngang video (Mirror) -->
            <div class="remix-card-compact" :class="{ enabled: globalRemix.hflip }">
              <label class="toggle-row-compact">
                <input type="checkbox" v-model="globalRemix.hflip" />
                <span>Lật ngang video (Mirror)</span>
              </label>
            </div>

            <!-- 2. Thay đổi tốc độ video -->
            <div class="remix-card-compact" :class="{ enabled: globalRemix.speed !== 1.0 }">
              <div class="remix-card-row-compact">
                <span class="option-title-compact">Tốc độ phát:</span>
                <div class="input-with-unit-mini">
                  <input type="number" v-model.number="globalRemix.speed" min="0.5" max="2" step="0.01" @change="globalRemix.speed = clampValue(globalRemix.speed, 0.5, 2.0, 1.0)" class="compact-input-speed" />
                  <span class="unit">×</span>
                </div>
              </div>
              <div class="speed-presets-compact">
                <button class="btn-preset-mini" :class="{active: globalRemix.speed === 1.0}" @click="globalRemix.speed = 1.0">Gốc</button>
                <button class="btn-preset-mini" :class="{active: globalRemix.speed === 1.02}" @click="globalRemix.speed = 1.02">1.02×</button>
                <button class="btn-preset-mini" :class="{active: globalRemix.speed === 1.05}" @click="globalRemix.speed = 1.05">1.05×</button>
                <button class="btn-preset-mini" :class="{active: globalRemix.speed === 1.1}" @click="globalRemix.speed = 1.1">1.1×</button>
              </div>
            </div>

            <!-- 3. Tỷ lệ khung hình -->
            <div class="remix-card-compact" :class="{ enabled: globalRemix.aspectEnabled }">
              <label class="toggle-row-compact" style="margin-bottom: 4px;">
                <input type="checkbox" v-model="globalRemix.aspectEnabled" />
                <span>Tỷ lệ khung hình</span>
              </label>
              <div class="aspect-controls-compact" v-if="globalRemix.aspectEnabled">
                <select v-model="globalRemix.aspectRatio" class="compact-select-mini">
                  <option value="9:16">9:16 (Dọc)</option>
                  <option value="1:1">1:1 (Vuông)</option>
                  <option value="16:9">16:9 (Ngang)</option>
                </select>
                <select v-model="globalRemix.aspectMode" class="compact-select-mini">
                  <option value="blur">Nền mờ</option>
                  <option value="crop">Cắt đầy khung</option>
                  <option value="pad">Viền đen</option>
                </select>
              </div>
            </div>

            <!-- 4. Hiệu ứng màu sắc -->
            <div class="remix-card-compact" :class="{ enabled: globalRemix.colorEnabled }">
              <label class="toggle-row-compact" style="margin-bottom: 4px;">
                <input type="checkbox" v-model="globalRemix.colorEnabled" />
                <span>Hiệu ứng màu sắc</span>
              </label>
              <div class="color-controls-compact" v-if="globalRemix.colorEnabled">
                <select v-model="globalRemix.colorPreset" class="compact-select-mini" style="width: 100%; margin-bottom: 6px;">
                  <option value="">Không có filter</option>
                  <option value="warm">Tông Ấm (warm)</option>
                  <option value="cool">Tông Lạnh (cool)</option>
                  <option value="vivid">Rực rỡ (vivid)</option>
                  <option value="bw">Trắng đen</option>
                </select>
                <div class="sliders-grid-compact">
                  <div class="slider-item-compact">
                    <span>Sáng: {{ globalRemix.colorBrightness }}</span>
                    <input type="range" v-model.number="globalRemix.colorBrightness" min="-0.3" max="0.3" step="0.05" />
                  </div>
                  <div class="slider-item-compact">
                    <span>Bão hòa: {{ globalRemix.colorSaturation }}x</span>
                    <input type="range" v-model.number="globalRemix.colorSaturation" min="0.5" max="2.0" step="0.1" />
                  </div>
                </div>
              </div>
            </div>



            <!-- 💡 Chủ đề Video (Prompt AI) -->
            <div class="remix-card-compact" style="grid-column: 1 / -1; margin-top: 4px; border: 1px dashed var(--border-color); background: var(--l-bg-soft); padding: 10px 12px; border-radius: 8px; display: flex; flex-direction: column; gap: 8px;">
              
              <!-- Tiêu đề + Hành động -->
              <div style="display: flex; align-items: center; justify-content: space-between;">
                <span class="option-title-compact" style="color: var(--wx-brand-accent); font-weight: 700; margin-bottom: 0; font-size: 12.5px;">💡 Chủ đề Video (Prompt AI)</span>
                
                <!-- Nút thêm mẫu -->
                <button @click="showAddPresetForm = !showAddPresetForm" class="text-btn" style="font-size: 11px; color: var(--accent-color); background: none; border: none; cursor: pointer; padding: 2px 6px; border-radius: 4px; background: rgba(99, 102, 241, 0.08); display: flex; align-items: center; gap: 4px;" title="Lưu prompt hiện tại thành mẫu mới">
                  💾 Lưu mẫu hiện tại
                </button>
              </div>

              <!-- Form thêm mẫu mới -->
              <div v-if="showAddPresetForm" style="display: flex; flex-direction: column; gap: 6px; padding: 8px; background: rgba(255,255,255,0.02); border-radius: 6px; border: 1px solid rgba(255,255,255,0.06); animation: fadeIn 0.2s ease;">
                <input type="text" v-model="newPresetName" placeholder="Tên mẫu gợi nhớ (ví dụ: Điện ảnh sắc nét)..." style="font-size: 11.5px; height: 28px; padding: 4px 8px; border-radius: 4px; background: var(--l-bg); border: 1px solid var(--border-color); color: var(--l-text); width: 100%; box-sizing: border-box; outline: none;" />
                <div style="display: flex; gap: 6px; justify-content: flex-end;">
                  <button @click="showAddPresetForm = false" style="font-size: 10.5px; padding: 3px 10px; border-radius: 4px; border: 1px solid var(--border-color); background: none; color: var(--l-text); cursor: pointer;">Hủy</button>
                  <button @click="addNewPreset" style="font-size: 10.5px; padding: 3px 10px; border-radius: 4px; border: none; background: var(--accent-color); color: white; cursor: pointer; font-weight: 600;">Lưu</button>
                </div>
              </div>

              <!-- Chọn nhanh mẫu bằng Tag/Pill trực quan -->
              <div style="display: flex; flex-direction: column; gap: 4px;">
                <div style="display: flex; flex-wrap: wrap; gap: 6px; margin-top: 2px;">
                  <div v-for="preset in promptPresets" :key="preset.id" 
                       style="position: relative; display: inline-flex; align-items: center;">
                    
                    <!-- Tag bấm để chọn -->
                    <span @click="selectPresetTag(preset)"
                          style="font-size: 11px; padding: 4px 10px; border-radius: 20px; cursor: pointer; transition: all 0.2s; user-select: none; font-weight: 500;"
                          :style="{
                            background: selectedPromptPresetId === preset.id ? 'rgba(99, 102, 241, 0.15)' : 'rgba(255,255,255,0.03)',
                            border: selectedPromptPresetId === preset.id ? '1px solid var(--accent-color)' : '1px solid rgba(255,255,255,0.08)',
                            color: selectedPromptPresetId === preset.id ? 'var(--accent-color)' : 'var(--l-text-muted)'
                          }">
                      {{ preset.name }}
                    </span>

                    <!-- Nút xóa tag nếu không phải mặc định -->
                    <button v-if="!['default_auto','1','2','3','4'].includes(preset.id)"
                            @click.stop="deletePresetById(preset.id)"
                            style="margin-left: -6px; background: var(--wx-danger-solid); color: var(--wx-text-inverse); border: none; border-radius: 50%; width: 12px; height: 12px; font-size: 8px; display: flex; align-items: center; justify-content: center; cursor: pointer; z-index: 2; box-shadow: 0 1px 3px var(--wx-surface-overlay); padding: 0;"
                            title="Xóa mẫu này">
                      ✕
                    </button>
                  </div>
                </div>
              </div>

              <!-- Nội dung Prompt -->
              <div style="display: flex; flex-direction: column; gap: 4px;">
                <span style="font-size: 11px; color: var(--l-text-muted); font-weight: 500;">Nội dung mô tả (được gửi cho AI):</span>
                <textarea v-model="analyzerConfig.prompt" 
                          placeholder="Mô tả nội dung cần phân tích (ví dụ: cô gái xinh nhảy múa, bối cảnh sang trọng...). Để trống sẽ tự nhận diện theo hình ảnh video." 
                          class="text-content-input" 
                          style="margin-bottom: 0; font-size: 11.5px; height: 60px; min-height: 45px; padding: 8px; border-radius: 6px; box-sizing: border-box; width: 100%; resize: vertical; font-family: inherit; line-height: 1.45; background: rgba(0,0,0,0.2); border: 1px solid var(--border-color); color: var(--l-text); outline: none;"></textarea>
              </div>
            </div>
          </div>



            <!-- Áp dụng hàng loạt -->
            <div style="margin-top: auto; border-top: 1px solid var(--border-color); padding-top: 8px;">
              <label class="auto-apply-label-compact" style="margin-bottom: 6px; display: flex; align-items: center; gap: 4px; font-size: 11px; color: var(--text-muted); cursor: pointer;">
                <input type="checkbox" v-model="globalRemix.autoApply" />
                <span>Tự động áp dụng khi quét mới</span>
              </label>
              <button @click="applyGlobalRemixToAllActive" class="btn primary-btn flex-center font-bold" style="width: 100%; padding: 6px; border-radius: 6px; background-color: var(--accent-color); color: white; border: none; cursor: pointer; font-size: 11.5px; display: flex; align-items: center; justify-content: center; gap: 4px;">
                <svg viewBox="0 0 24 24" width="12" height="12"><path fill="currentColor" d="M17 3H5c-1.11 0-2 .9-2 2v14c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2V7l-4-4zm-5 16c-1.66 0-3-1.34-3-3s1.34-3 3-3 3 1.34 3 3-1.34 3-3 3zm3-10H5V5h10v4z"/></svg>
                Áp hiệu ứng cho tất cả clip hiện tại
              </button>
            </div>
        </div>

        <!-- Bên Phải: Trình phát Video & Visual Timeline -->
        <div class="preview-workspace">
          <div class="workspace-meta" v-if="videoInfo">
            <div class="active-video-details">
              <span class="video-label-now">Đang chọn:</span>
              <span class="video-name-now">{{ activeVideoPath.split('\\').pop() }}</span>
              <span class="video-res-now">{{ videoInfo.Width }}x{{ videoInfo.Height }} · {{ videoInfo.FPS.toFixed(1) }} FPS · {{ formatTime(videoInfo.Duration) }}</span>
            </div>
          </div>
          <div class="workspace-meta-empty" v-else>
            <div class="active-video-details">
              <span class="video-label-now">Trình xem trước</span>
            </div>
          </div>

          <div class="player-container-mini">
            
            
            <video v-if="activeVideoSrc" ref="videoPlayer" :src="computedVideoSrc" controls class="video-player-mini" @timeupdate="onVideoTimeUpdate"></video>
            <div class="empty-player-screen" v-else>
              <div class="player-emoji">
                <Monitor :size="40" style="color: var(--text-muted);" />
              </div>
              <span>Chọn một video để xem trước</span>
            </div>

          </div>

          <!-- Timeline phân đoạn -->
          <div class="visual-timeline-container-mini" v-if="videoInfo">
            <div class="timeline-header flex-between">
              <span>Thanh phân đoạn trực quan (Bấm để tua video)</span>
              <span>Tổng số: <strong>{{ activeClips.length }}</strong> đoạn</span>
            </div>
            <div class="visual-timeline">
              <!-- Vạch chỉ thời gian video đang chạy -->
              <div 
                class="timeline-playhead" 
                :style="{ left: (videoCurrentTime / videoInfo.Duration * 100) + '%' }"
              ></div>

              <template v-if="activeClips.length > 0">
                <div 
                  v-for="(clip, idx) in activeClips" 
                  :key="clip.id"
                  class="timeline-segment"
                  :style="{ width: ((clip.endTime - clip.startTime) / videoInfo.Duration * 100) + '%', flexShrink: 0 }"
                  @click="jumpToTime(clip.startTime)"
                  :title="`Clip #${clip.index}: ${clip.startTime}s - ${clip.endTime}s`"
                >
                  <span>#{{ clip.index }}</span>
                </div>
              </template>
              <div v-else class="timeline-segment-placeholder">
                <span>Chưa có phân đoạn video được cắt</span>
              </div>
            </div>
          </div>

          <!-- Trình ghép nhạc nền & Lặp nhạc (Tối ưu hóa không gian) -->
          <div class="music-merging-panel" v-if="videoPaths.length > 0" :class="{ 'panel-disabled': isAnalyzing || isExporting }" style="margin-top: 8px; background: rgba(30, 41, 59, 0.4); border: 1px solid rgba(255,255,255,0.06); padding: 10px 14px; border-radius: 10px; backdrop-filter: blur(8px);">
            <div style="display: flex; align-items: center; justify-content: space-between; margin-bottom: 6px; flex-wrap: wrap; gap: 8px;">
              <div style="display: flex; align-items: center; gap: 12px; flex-wrap: wrap;">
                <h3 style="margin: 0; font-size: 14px; font-weight: 700; color: var(--wx-brand-accent); display: flex; align-items: center; gap: 6px;">
                  <Music :size="15" />
                  Trình Ghép Nhạc Nền & Lặp Nhạc (Áp dụng hàng loạt)
                </h3>
                <button @click="pickGlobalMusic" class="btn active-btn" style="font-size: 11.5px; padding: 6px 12px; display: inline-flex; align-items: center; gap: 4px; border-radius: 6px; cursor: pointer; border: none; font-weight: 600;">
                  <Plus :size="13" />
                  Thêm nhạc nền
                </button>
                <button v-if="globalRemix.musicTracks && globalRemix.musicTracks.length > 0" @click="clearGlobalMusic" class="btn cancel-btn" style="font-size: 11.5px; padding: 5px 12px; border-radius: 6px; cursor: pointer; border: 1px solid rgba(255,255,255,0.08); background: rgba(255,255,255,0.02); color: var(--l-text-muted); display: inline-flex; align-items: center; gap: 4px;">
                  <X :size="12" />
                  Xóa tất cả
                </button>
              </div>
              <div style="display: flex; gap: 16px; font-size: 12.5px; align-items: center;">
                <label style="display: flex; align-items: center; gap: 6px; cursor: pointer; color: var(--l-text);">
                  <input type="checkbox" v-model="globalRemix.muteOriginal" style="width: 15px; height: 15px; border-radius: 4px;" />
                  <VolumeX :size="14" style="color: var(--text-muted);" />
                  <span>Tắt tiếng gốc</span>
                </label>
                <label style="display: flex; align-items: center; gap: 6px; cursor: pointer; color: var(--l-text);">
                  <input type="checkbox" v-model="globalRemix.musicLoop" style="width: 15px; height: 15px; border-radius: 4px;" />
                  <Repeat :size="14" style="color: var(--wx-brand-accent);" />
                  <span>Tự động lặp lại nhạc</span>
                </label>
              </div>
            </div>

            <!-- Danh sách bài hát đã ghép -->
            <div class="music-tracks-list" style="display: flex; flex-direction: column; gap: 8px; max-height: 200px; overflow-y: auto;" :style="{ marginBottom: globalRemix.musicTracks && globalRemix.musicTracks.length > 0 ? '12px' : '0' }">
              <div v-if="!globalRemix.musicTracks || globalRemix.musicTracks.length === 0" style="padding: 20px; text-align: center; color: var(--l-text-muted); font-size: 12.5px; border: 1px dashed rgba(255,255,255,0.08); border-radius: 8px;">
                Chưa chọn nhạc nền nào. Bấm nút "Thêm nhạc nền" ở trên để chọn các file nhạc nền ghép nối tiếp.
              </div>
              <div v-else v-for="(track, index) in globalRemix.musicTracks" :key="index" style="display: flex; align-items: center; justify-content: space-between; background: rgba(255,255,255,0.02); border: 1px solid rgba(255,255,255,0.05); padding: 8px 12px; border-radius: 8px; transition: all 0.2s;">
                <div style="display: flex; align-items: center; gap: 8px; flex: 1; min-width: 0;">
                  <span style="font-size: 11px; background: var(--accent-color); color: white; padding: 2px 6px; border-radius: 4px; font-weight: 700;">#{{ index + 1 }}</span>
                  <span style="font-size: 12.5px; font-weight: 600; color: var(--l-text); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" :title="track">
                    {{ track.split('\\').pop() }}
                  </span>
                </div>
                <div style="display: flex; align-items: center; gap: 6px;">
                  <button @click="moveMusicTrack(index, -1)" :disabled="index === 0" class="mini-icon-btn" style="padding: 4px; background: none; border: none; color: var(--l-text-muted); cursor: pointer;" title="Lên">▲</button>
                  <button @click="moveMusicTrack(index, 1)" :disabled="index === globalRemix.musicTracks.length - 1" class="mini-icon-btn" style="padding: 4px; background: none; border: none; color: var(--l-text-muted); cursor: pointer;" title="Xuống">▼</button>
                  <button @click="removeMusicTrack(index)" class="mini-icon-btn" style="padding: 4px; background: none; border: none; color: var(--wx-brand-accent); cursor: pointer;" title="Xóa">✕</button>
                </div>
              </div>
            </div>

            <!-- Dòng chỉnh âm lượng (chỉ hiện khi có bài hát) -->
            <div style="display: flex; justify-content: flex-end;" v-if="globalRemix.musicTracks && globalRemix.musicTracks.length > 0">
              <div style="display: flex; align-items: center; gap: 10px; width: 320px; max-width: 100%;">
                <span style="font-size: 12px; color: var(--l-text-muted); white-space: nowrap;">Âm lượng nhạc: {{ Math.round(globalRemix.musicVolume * 100) }}%</span>
                <input type="range" v-model.number="globalRemix.musicVolume" min="0" max="1" step="0.05" style="flex: 1; height: 4px; cursor: pointer;" />
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- BỐ CỤC DƯỚI: DANH SÁCH CLIPS ĐÃ CẮT & XUẤT BẢN -->
      <section class="section-bottom-clips" :class="{ 'panel-disabled': isAnalyzing || isExporting }" :style="{ paddingBottom: selectedClips.size > 0 ? '100px' : '16px' }">
        <div class="section-title-bar-clips">
          <div class="title-left-clips">
            <h2 style="display: flex; align-items: center; gap: 6px;">
              <Scissors :size="18" style="color: var(--wx-brand-accent);" />
              Danh sách Video Đã Cắt
            </h2>
            <span class="clips-counter" v-if="displayClipsCount > 0">
              ({{ displayClipsCount }} đoạn tìm thấy<template v-if="isMultiVideoDisplay"> từ {{ selectedVideos.size }} video</template>)
            </span>
          </div>

          <div class="clips-batch-selector-row" v-if="displayClipsCount > 0">
            <label class="select-all-label">
              <input type="checkbox" :checked="isAllSelected" @change="toggleSelectAll" class="clip-checkbox" />
              <span>Chọn tất cả clip</span>
            </label>
            <div class="selected-badge" v-if="selectedClips.size > 0">
              Đã chọn: <strong>{{ selectedClips.size }}</strong> clip
            </div>
          </div>
        </div>

        <div class="clips-view-container">
          <div v-if="displayClipsCount > 0" class="clips-grid">
            <div v-for="(clip, idx) in displayClips" :key="clip.id"
              class="clip-modern-card"
              :class="{ 'clip-selected': selectedClips.has(clip.id), 'clip-done': clip.status === 'completed' }">
              
              <div class="clip-card-header-bar">
                <div class="header-left-wrap">
                  <label class="clip-select-wrap" @click.stop>
                    <input type="checkbox" class="clip-checkbox" :checked="selectedClips.has(clip.id)" @change="toggleClipSelected(clip.id)" :disabled="isExporting" />
                  </label>
                  <span class="clip-title-tag">Clip #{{ clip.index }}</span>
                  <span class="status-dot done" v-if="clip.status === 'completed'" title="Đã xuất"></span>
                  <span class="status-dot pending" v-else title="Chờ xuất"></span>
                </div>
                <span class="clip-duration-tag" style="display: inline-flex; align-items: center; gap: 4px;">
                  <Clock :size="12" />
                  {{ formatTime(clip.endTime - clip.startTime) }}
                </span>
              </div>

              <!-- Tên video nguồn — chỉ hiện khi đang xem nhiều video -->
              <div class="clip-source-video-tag" v-if="isMultiVideoDisplay && clip._videoName" :title="clip._videoPath" style="display: flex; align-items: center; gap: 4px;">
                <Video :size="12" />
                {{ clip._videoName.length > 30 ? clip._videoName.substring(0, 30) + '...' : clip._videoName }}
              </div>

              <!-- Hình đại diện của clip ngắn -->
              <div class="clip-thumbs-section" v-if="clip.thumbnail">
                <div class="thumb-box-single" @click="jumpToTime(clip.startTime)" title="Bấm để phát thử clip này">
                  <img :src="getThumbUrl(clip.thumbnail)" />
                  <div class="play-overlay">
                    <Play :size="18" fill="currentColor" />
                  </div>
                </div>
              </div>

              <div class="clip-time-inputs-row">
                <div class="input-block">
                  <span class="input-lbl">Bắt đầu</span>
                  <input type="number" step="1" v-model.number="clip.startTime" :disabled="isExporting" @input="updateClipDuration(clip)" @change="onClipTimeChange(clip)" />
                </div>
                <div class="input-arrow-mini">➔</div>
                <div class="input-block">
                  <span class="input-lbl">Kết thúc</span>
                  <input type="number" step="1" v-model.number="clip.endTime" :disabled="isExporting" @input="updateClipDuration(clip)" @change="onClipTimeChange(clip)" />
                </div>
              </div>

              <div class="clip-card-actions-row">
                <!-- <button v-if="!isExporting" @click="openEdit(idx)" class="mini-act-btn btn-edit" title="Chỉnh sửa (Chèn chữ, watermark...)">
                  ✏️ Sửa
                </button> -->
                <button v-if="!isExporting" @click="removeClip(idx)" class="mini-act-btn btn-delete flex-center" title="Xóa clip" style="display: inline-flex; align-items: center; gap: 4px;">
                  <Trash2 :size="12" />
                  Xóa
                </button>
              </div>

              <!-- Chips các hiệu ứng đã áp trên clip này -->
              <div class="clip-effects-chips">
                <span v-for="(chip, ci) in clipEditChips(clip)" :key="ci" class="eff-chip" :class="'chip-' + chip">{{ chip }}</span>
                <span class="eff-chip-empty" v-if="clipEditChips(clip).length === 0">Chưa áp hiệu ứng</span>
              </div>
            </div>
          </div>

          <div class="empty-clips-panel" v-else>
            <template v-if="activeVideoPath && activeAnalyzingPaths.has(activeVideoPath)">
              <div class="empty-emoji">
                <Loader2 :size="40" class="spin-hourglass" style="color: var(--wx-brand-accent);" />
              </div>
              <h3>Đang phân tích video này...</h3>
              <p>Vui lòng chờ hoàn tất, các phân đoạn sẽ hiện ở đây.</p>
            </template>
            <template v-else-if="activeVideoPath">
              <div class="empty-emoji">
                <Scissors :size="40" style="color: var(--wx-brand-primary);" />
              </div>
              <h3>Video này chưa được cắt</h3>
              <p>Bấm nút <strong>"Bắt Đầu Cắt Tự Động"</strong> ở trên hoặc nút bên dưới để cắt video này.</p>
            </template>
            <template v-else>
              <div class="empty-emoji">
                <Film :size="40" style="color: var(--text-muted);" />
              </div>
              <h3>Chưa có video đã cắt nào ở đây</h3>
              <p>Chọn các video gốc phía trên rồi bấm nút <strong>"Bắt Đầu Cắt Tự Động"</strong> để hệ thống tự động cắt cảnh thông minh.</p>
            </template>
          </div>
        </div>
      </section>

      <!-- PANEL XUẤN BẢN CỐ ĐỊNH Ở CUỐI GÓC DƯỚI CLIPS -->
      <div class="clips-export-publisher-bar" v-if="selectedClips.size > 0" :class="{ 'panel-disabled': isAnalyzing }">
        <div class="pub-left" style="display: flex; align-items: center; flex: none; flex-shrink: 0;">
          <label class="toggle-row inline" style="cursor: pointer; font-size: 13px; display: inline-flex; align-items: center; gap: 8px; margin-bottom: 0; user-select: none; font-weight: 600; color: var(--l-text); white-space: nowrap; flex-shrink: 0;">
            <input type="checkbox" v-model="exportWithThumbnails" style="width: 16px; height: 16px; accent-color: var(--wx-brand-primary);" />
            Xuất kèm ảnh Thumbnail
          </label>
        </div>

        <!-- Thư mục xuất chung mặc định (chỉ hiện khi chưa xuất) -->
        <div v-if="!isExporting" style="display: flex; align-items: center; gap: 8px; flex: 1; margin: 0 16px; min-width: 0; align-self: center;">
          <span style="font-size: 13px; font-weight: 600; color: var(--l-text); white-space: nowrap; line-height: 1; display: inline-flex; align-items: center;">
            Lưu vào:
          </span>
          <input type="text" v-model="outDir" class="file-path-input dl-pub-dir-input" readonly :title="outDir" />
          <button class="btn dl-pub-dir-btn-icon" @click="chooseOutDir" title="Chọn thư mục">
            <FolderOpen :size="14" />
          </button>
        </div>

        <div class="pub-right" :style="{ display: 'flex', alignItems: 'center', gap: '16px', flex: isExporting ? '1' : 'none', justifyContent: 'flex-end', flexShrink: 0 }">
          <!-- Tiến trình xuất -->
          <div v-if="isExporting && exportProgress.total > 0" class="pub-progress-box" 
               :style="{
                 display: 'flex', 
                 flexDirection: 'row',
                 flexWrap: 'nowrap',
                 alignItems: 'center', 
                 gap: '12px', 
                 flex: 1, 
                 minWidth: '580px', 
                 padding: '6px 12px', 
                 userSelect: 'none', 
                 fontSize: '12.5px',
                 borderRadius: '8px',
                 border: '1px solid rgba(255, 255, 255, 0.06)',
                 background: `linear-gradient(to right, rgba(16, 185, 129, 0.12) 0%, rgba(16, 185, 129, 0.12) ${exportProgress.done / exportProgress.total * 100}%, rgba(255, 255, 255, 0.01) ${exportProgress.done / exportProgress.total * 100}%)`
               }">
            <!-- Tổng tiến độ Video -->
            <span style="font-weight: 700; white-space: nowrap; display: flex; align-items: center; gap: 4px;">
              🎬 Cắt Video: <strong style="color: var(--success-color);">{{ videoDoneCount }}/{{ exportProgress.total }}</strong>
            </span>
            
            <span style="color: var(--wx-border-default)">|</span>
            
            <!-- Tổng tiến độ Thumbnail -->
            <span style="font-weight: 700; white-space: nowrap; display: flex; align-items: center; gap: 4px;">
              🎨 Ảnh bìa AI: 
              <template v-if="exportWithThumbnails">
                <strong style="color: var(--wx-brand-accent);">{{ thumbDoneCount }}/{{ exportProgress.total }}</strong>
              </template>
              <span v-else style="color: var(--text-muted); font-size: 11px; font-weight: normal; font-style: italic;">(Tắt)</span>
            </span>
            
            <span style="color: var(--wx-border-default)">|</span>

            <!-- Tên Video đang xử lý -->
            <div style="color: var(--l-text-muted); font-style: italic; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; flex: 1; text-align: left;" :title="exportStatusText">
              {{ formatExportStatusMsg(exportStatusText) }}
            </div>
            
            <span style="color: rgba(255,255,255,0.15)">|</span>
            
            <!-- ETA -->
            <span class="eta-badge" style="font-size: 11px; font-weight: bold; color: var(--accent-color); padding: 2px 6px; background: rgba(99, 102, 241, 0.1); border-radius: 4px; white-space: nowrap;">
              {{ getExportETA() }}
            </span>
          </div>

          <button v-if="!isExporting" @click="removeSelectedClips" class="btn delete-selected-btn-bar flex-center" title="Xóa các clip đã chọn">
            <Trash2 :size="13" />
            Xóa {{ selectedClips.size }} Clip
          </button>
          <button v-if="isExporting" @click="stopExport" class="btn big-export-btn stop-export-btn flex-center">
            <svg viewBox="0 0 24 24" width="20" height="20" class="btn-icon"><rect x="6" y="6" width="12" height="12" fill="currentColor"/></svg>
            <span class="font-bold">Dừng Xuất</span>
          </button>
          <button v-else @click="exportClips" class="btn big-export-btn flex-center">
            <svg viewBox="0 0 24 24" width="20" height="20" class="btn-icon"><path fill="currentColor" d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96zM17 13l-5 5-5-5h3V9h4v4h3z"/></svg>
            <span class="font-bold">
              {{ selectedClips.size > 0 ? `Xuất ${selectedClips.size} Clip Đã Chọn` : `Xuất Toàn Bộ ${displayClipsCount} Clip` }}
            </span>
          </button>
        </div>
      </div>
      </template>

      <!-- BỐ CỤC CHO PHẦN TẢI VIDEO ONLINE INLINE -->
      <VideoDownloader v-show="activeView === 'download-video'" :video-paths="videoPaths" :show-toast="showToast" @back="activeView = 'split'" />

      <!-- BỐ CỤC CHO PHẦN TẢI ẢNH CHỦ ĐỀ INLINE -->
      <ImageDownloader v-show="activeView === 'download-image'" :show-toast="showToast" @back="activeView = 'split'" />

      <!-- BỐ CỤC CHO PHẦN TẠO ẢNH AI INLINE -->
      <BrowserAIImagePage v-show="activeView === 'ai-image'" :default-output-dir="outDir" :show-chrome="browserAIShowChrome" @back="activeView = 'split'" @apply-image="handleApplyAIImage" @show-toast="showToast" />

      <!-- BỐ CỤC CHO PHẦN TẠO VIDEO AI INLINE -->
      <BrowserAIVideoPage v-show="activeView === 'ai-video'" :default-output-dir="outDir" :show-chrome="browserAIShowChrome" @back="activeView = 'split'" @apply-video="handleApplyAIVideo" @show-toast="showToast" />

    </div>

    <!-- Thanh trạng thái CapCut-style dưới đáy -->
    <footer class="system-status-footer">
      <div class="status-indicator-dot" :class="{ active: isAnalyzing || isExporting }"></div>
      <span class="status-msg-text">{{ statusText }}</span>
    </footer>

    <!-- ===== TOAST NOTIFICATIONS ===== -->
    <Teleport to="body">
      <div class="toast-container">
        <TransitionGroup name="toast-slide">
          <div class="toast-item" :class="t.type" v-for="t in toasts" :key="t.id">
            <div class="toast-icon-wrap">
              <Check v-if="t.type === 'success'" :size="16" class="toast-icon" />
              <X v-else-if="t.type === 'error'" :size="16" class="toast-icon" />
              <AlertTriangle v-else-if="t.type === 'warning'" :size="16" class="toast-icon" />
              <Info v-else :size="16" class="toast-icon" />
            </div>
            <div class="toast-content">{{ t.message }}</div>
            <button class="toast-close-btn" @click="toasts = toasts.filter(item => item.id !== t.id)"><X :size="14" /></button>
          </div>
        </TransitionGroup>
      </div>
    </Teleport>

    <!-- ===== CUSTOM CONFIRM DIALOG ===== -->
    <Teleport to="body">
      <div class="modal-overlay" v-if="confirmDialogState.show" @click.self="handleConfirmResolve(false)" style="z-index: 99999;">
        <div class="settings-modal recent-modal confirm-dialog-modal" style="width: 420px; max-width: 90%;">
          <div class="modal-header" style="padding: 16px 20px; border-bottom: 1px solid var(--l-border);">
            <h2 style="font-size: 15px; font-weight: 700; display: flex; align-items: center; gap: 6px; color: var(--wx-danger-solid);">
              <AlertTriangle :size="18" />
              Xác nhận
            </h2>
            <button class="modal-close" @click="handleConfirmResolve(false)"><X :size="16" /></button>
          </div>
          <div class="modal-body" style="padding: 20px; font-size: 13.5px; line-height: 1.5; font-weight: 600;">
            {{ confirmDialogState.message }}
          </div>
          <div class="modal-footer-buttons" style="display: flex; gap: 10px; justify-content: flex-end; padding: 12px 20px; border-top: 1px solid var(--l-border); background: var(--l-bg-sunken); border-bottom-left-radius: 16px; border-bottom-right-radius: 16px;">
            <button @click="handleConfirmResolve(false)" class="btn cancel-btn flex-center" style="background-color: var(--l-bg-soft); color: var(--l-text); border: 1px solid var(--l-border); padding: 6px 14px; border-radius: 6px; cursor: pointer; font-size: 12px; font-weight: 700; display: inline-flex; align-items: center; gap: 4px;">
              <X :size="13" /> Hủy
            </button>
            <button @click="handleConfirmResolve(true)" class="btn confirm-btn flex-center" style="background-color: var(--wx-danger-solid); color: var(--wx-text-inverse); border: none; padding: 6px 14px; border-radius: 6px; cursor: pointer; font-size: 12px; font-weight: 700; box-shadow: var(--wx-shadow-lift); display: inline-flex; align-items: center; gap: 4px;">
              <Check :size="13" /> Đồng ý
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- ===== RECENT PROJECTS MODAL ===== -->
    <Teleport to="body">
      <div class="modal-overlay" v-if="showRecent" @click.self="showRecent = false">
        <div class="settings-modal recent-modal">
          <div class="modal-header">
            <h2 style="display: flex; align-items: center; gap: 6px;">
              <FolderOpen :size="18" style="color: var(--wx-brand-accent);" />
              Project đã lưu
            </h2>
            <button class="modal-close" @click="showRecent = false"><X :size="16" /></button>
          </div>
          <div class="modal-body">
            <div v-if="recentProjects.length === 0" class="empty-recent">
              <div class="empty-graphic">
                <FolderOpen :size="40" style="color: var(--wx-brand-accent); margin-bottom: 8px;" />
              </div>
              <p>Chưa có project nào được lưu. Phân tích một video rồi bấm "Lưu" để lưu phiên làm việc.</p>
            </div>
            <ul v-else class="recent-list">
              <li v-for="proj in recentProjects" :key="proj.id" class="recent-item">
                <div class="recent-info" @click="restoreProject(proj)">
                  <span class="recent-name" :title="proj.sourcePath">{{ proj.name }}</span>
                  <span class="recent-meta">{{ proj.clipCount }} clip · {{ formatTime(proj.duration) }} · {{ proj.status }}</span>
                </div>
                <div class="recent-actions">
                  <button @click="restoreProject(proj)" class="btn-mini open flex-center" title="Mở lại" style="display: inline-flex; align-items: center; gap: 4px;">
                    <FolderOpen :size="12" /> Mở
                  </button>
                  <button @click="deleteProject(proj)" class="btn-mini del flex-center" title="Xóa khỏi lịch sử" style="display: inline-flex; align-items: center; gap: 4px;">
                    <Trash2 :size="12" />
                  </button>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- ===== NAMED PROJECT CREATION MODAL ===== -->
    <Teleport to="body">
      <div class="modal-overlay" v-if="showProjectModal" @click.self="showProjectModal = false">
        <div class="settings-modal recent-modal">
          <div class="modal-header">
            <h2 style="display: flex; align-items: center; gap: 6px;">
              <Plus :size="18" style="color: var(--wx-brand-accent);" />
              Tạo dự án mới
            </h2>
            <button class="modal-close" @click="showProjectModal = false"><X :size="16" /></button>
          </div>
          <div class="modal-body">
            <div class="setting-item" style="display: flex; flex-direction: column; gap: 8px;">
              <label style="font-weight: 600; font-size: 13px;">Tên dự án mới:</label>
              <input type="text" v-model="newProjectName" class="file-path-input" style="padding: 10px; border-radius: 8px; border: 1px solid var(--l-border); background: var(--l-bg); color: var(--l-text); outline: none;" placeholder="Ví dụ: Kênh Tiktok Review Phim" @keyup.enter="createProject" />
            </div>
            <div class="modal-footer-buttons" style="display: flex; gap: 8px; justify-content: flex-end; margin-top: 16px;">
              <button @click="createProject" class="btn confirm-btn flex-center" :disabled="!newProjectName.trim()" style="background-color: var(--wx-brand-primary); color: var(--wx-text-inverse); border: none; padding: 8px 16px; border-radius: 6px; cursor: pointer; display: inline-flex; align-items: center; gap: 4px;">
                <Plus :size="14" /> Tạo dự án
              </button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- ===== NAMED PROJECT MANAGEMENT MODAL ===== -->
    <Teleport to="body">
      <div class="modal-overlay" v-if="showManageProjectsModal" @click.self="showManageProjectsModal = false">
        <div class="settings-modal recent-modal">
          <div class="modal-header">
            <h2 style="display: flex; align-items: center; gap: 6px;">
              <Settings :size="18" style="color: var(--wx-brand-accent);" />
              Quản lý các dự án
            </h2>
            <button class="modal-close" @click="showManageProjectsModal = false"><X :size="16" /></button>
          </div>
          <div class="modal-body">
            <ul class="recent-list">
              <li v-for="proj in namedProjects" :key="proj.id" class="recent-item">
                <div class="recent-info">
                  <span class="recent-name" style="font-weight: 700;">{{ proj.name }}</span>
                  <span class="recent-meta">
                    {{ proj.videoPaths.length }} video · Tạo ngày: {{ new Date(proj.createdAt).toLocaleDateString() }}
                  </span>
                </div>
                <div class="recent-actions">
                  <button @click="loadProject(proj.id); showManageProjectsModal = false" class="btn-mini open flex-center" style="display: inline-flex; align-items: center; gap: 4px;">
                    <FolderOpen :size="12" /> Mở
                  </button>
                  <button @click="deleteNamedProject(proj.id)" :disabled="namedProjects.length <= 1" class="btn-mini del flex-center" title="Xóa dự án" style="display: inline-flex; align-items: center; gap: 4px;">
                    <Trash2 :size="12" />
                  </button>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- ===== MÀN HÌNH CHỈNH SỬA CLIP (GĐ6 — full screen) ===== -->
    <Teleport to="body">
      <div class="edit-screen" v-if="showEdit && editingClip">
        <!-- Thanh tiêu đề màn hình chỉnh sửa -->
        <header class="edit-header">
          <div class="edit-header-left">
            <button class="edit-back-btn" @click="showEdit = false" title="Quay lại">
              <svg viewBox="0 0 24 24"><path fill="currentColor" d="M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20z"/></svg>
              Quay lại
            </button>
            <h2 style="display: flex; align-items: center; gap: 6px;">
              <Scissors :size="18" style="color: var(--wx-brand-accent);" />
              Chỉnh sửa Clip #{{ editingClip.index }}
              <span class="edit-time-sub">{{ formatTime(editingClip.startTime) }} → {{ formatTime(editingClip.endTime) }}</span>
            </h2>
          </div>
          <div class="edit-header-right">
            <button @click="resetEdit" class="btn reset-btn" style="display: inline-flex; align-items: center; gap: 4px;">
              <RotateCcw :size="14" /> Đặt lại
            </button>
            <button @click="applyEditToAll" class="btn reset-btn" title="Áp bộ chỉnh sửa này cho mọi clip của video hiện tại" style="display: inline-flex; align-items: center; gap: 4px;">
              <Copy :size="14" /> Áp cho video này
            </button>
            <button @click="applyEditToSelectedVideos" class="btn reset-btn" :title="selectedVideos.size > 0 ? `Áp cho mọi clip của ${selectedVideos.size} video đã tick` : 'Áp cho mọi clip của tất cả video đã phân tích'" style="display: inline-flex; align-items: center; gap: 4px;">
              <Layers :size="14" /> {{ selectedVideos.size > 0 ? `Áp cho ${selectedVideos.size} video đã chọn` : 'Áp cho tất cả video' }}
            </button>
            <button @click="showEdit = false" class="btn save-btn" style="display: inline-flex; align-items: center; gap: 4px;">
              <Check :size="14" /> Xong
            </button>
          </div>
        </header>

        <div class="edit-body">
          <!-- Cột trái: xem trước video -->
          <div class="edit-preview">
            <div class="edit-preview-frame" :class="'ar-' + previewAspectClass">
              <video ref="editPlayer" :src="activeVideoSrc" controls class="edit-preview-video"></video>
            </div>
            <div class="edit-filter-summary">
              <span class="summary-title">Tóm tắt hiệu ứng đang áp:</span>
              <div class="summary-chips">
                <span v-for="(chip, i) in editSummary" :key="i" class="summary-chip">{{ chip }}</span>
                <span v-if="editSummary.length === 0" class="summary-empty">Chưa áp hiệu ứng nào.</span>
              </div>
              <p class="preview-note">Xem trước phát video gốc — hiệu ứng thực tế sẽ được áp khi xuất/ghép.</p>
            </div>
          </div>

          <!-- Cột phải: bảng điều khiển hiệu ứng -->
          <div class="edit-controls">
            <!-- Tỉ lệ khung -->
            <div class="settings-group">
              <h3 class="group-title" style="display: flex; align-items: center; gap: 6px;">
                <Monitor :size="15" />
                Tỉ lệ khung hình
              </h3>
              <label class="toggle-row">
                <input type="checkbox" v-model="editingClip.edit.aspect.enabled" />
                Đổi tỉ lệ khung (cho Reels / TikTok / Shorts)
              </label>
              <div class="settings-grid" v-if="editingClip.edit.aspect.enabled">
                <div class="setting-item">
                  <label>Tỉ lệ</label>
                  <select v-model="editingClip.edit.aspect.ratio">
                    <option value="9:16">9:16 (dọc — TikTok/Reels)</option>
                    <option value="1:1">1:1 (vuông)</option>
                    <option value="16:9">16:9 (ngang)</option>
                  </select>
                </div>
                <div class="setting-item">
                  <label>Cách lấp khung
                    <span class="hint">crop = cắt cho đầy; pad = viền đen; blur = nền mờ.</span>
                  </label>
                  <select v-model="editingClip.edit.aspect.mode">
                    <option value="crop">Crop (cắt đầy khung)</option>
                    <option value="pad">Pad (viền đen)</option>
                    <option value="blur">Blur (nền mờ)</option>
                  </select>
                </div>
              </div>
            </div>

            <!-- Màu sắc -->
            <div class="settings-group">
              <h3 class="group-title" style="display: flex; align-items: center; gap: 6px;">
                <SlidersHorizontal :size="15" />
                Màu sắc
              </h3>
              <label class="toggle-row">
                <input type="checkbox" v-model="editingClip.edit.color.enabled" />
                Bật chỉnh màu
              </label>
              <div v-if="editingClip.edit.color.enabled">
                <div class="settings-grid">
                  <div class="setting-item">
                    <label>Độ sáng <span class="hint">-1.0 → 1.0 (0 = giữ nguyên)</span></label>
                    <input type="number" v-model.number="editingClip.edit.color.brightness" min="-1" max="1" step="0.05" />
                  </div>
                  <div class="setting-item">
                    <label>Tương phản <span class="hint">-1.0 → 2.0 (0 = giữ nguyên)</span></label>
                    <input type="number" v-model.number="editingClip.edit.color.contrast" min="-1" max="2" step="0.05" />
                  </div>
                  <div class="setting-item">
                    <label>Độ bão hòa <span class="hint">0 → 3.0 (1 = giữ nguyên)</span></label>
                    <input type="number" v-model.number="editingClip.edit.color.saturation" min="0" max="3" step="0.05" />
                  </div>
                  <div class="setting-item">
                    <label>Preset filter</label>
                    <select v-model="editingClip.edit.color.preset">
                      <option value="">Không</option>
                      <option value="warm">Ấm (warm)</option>
                      <option value="cool">Lạnh (cool)</option>
                      <option value="vivid">Rực (vivid)</option>
                      <option value="bw">Đen trắng</option>
                      <option value="vintage">Vintage</option>
                    </select>
                  </div>
                </div>
              </div>
            </div>

            <!-- Tốc độ -->
            <div class="settings-group">
              <h3 class="group-title" style="display: flex; align-items: center; gap: 6px;">
                <Timer :size="15" />
                Tốc độ phát
              </h3>
              <div class="settings-grid">
                <div class="setting-item">
                  <label>Hệ số tốc độ
                    <span class="hint">1 = gốc; 2 = nhanh gấp đôi; 0.5 = chậm một nửa. Video + tiếng đồng bộ.</span>
                  </label>
                  <div class="input-with-unit">
                    <input type="number" v-model.number="editingClip.edit.speed" min="0.25" max="4" step="0.25" />
                    <span class="unit">×</span>
                  </div>
                </div>
              </div>
            </div>

            <!-- Chữ / phụ đề (GĐ6) -->
            <div class="settings-group">
              <div class="group-title-row">
                <h3 class="group-title" style="display: flex; align-items: center; gap: 6px;">
                  <Type :size="15" />
                  Chữ / Phụ đề
                </h3>
                <button class="mini-add-btn flex-center" @click="addText" style="display: inline-flex; align-items: center; gap: 4px;">
                  <Plus :size="12" /> Thêm dòng chữ
                </button>
              </div>
              <div v-if="editingClip.edit.texts.length === 0" class="group-empty">Chưa có chữ. Bấm "Thêm dòng chữ" để chèn tiêu đề / caption.</div>
              <div v-for="(t, ti) in editingClip.edit.texts" :key="ti" class="text-item-card">
                <div class="text-item-head">
                  <span class="text-item-idx">Dòng #{{ ti + 1 }}</span>
                  <button class="mini-del-btn flex-center" @click="removeText(ti)" title="Xóa dòng chữ" style="display: inline-flex; align-items: center; gap: 4px;">
                    <X :size="12" />
                  </button>
                </div>
                <input type="text" class="text-content-input" v-model="t.content" placeholder="Nhập nội dung chữ (hỗ trợ tiếng Việt)" />
                <div class="settings-grid">
                  <div class="setting-item">
                    <label>Cỡ chữ</label>
                    <input type="number" v-model.number="t.fontSize" min="8" max="200" step="2" />
                  </div>
                  <div class="setting-item">
                    <label>Màu chữ</label>
                    <select v-model="t.color">
                      <option value="white">Trắng</option>
                      <option value="black">Đen</option>
                      <option value="yellow">Vàng</option>
                      <option value="red">Đỏ</option>
                      <option value="#00e0ff">Xanh neon</option>
                    </select>
                  </div>
                  <div class="setting-item">
                    <label>Vị trí</label>
                    <select v-model="t.y">
                      <option value="h-text_h-80">Dưới</option>
                      <option value="(h-text_h)/2">Giữa</option>
                      <option value="80">Trên</option>
                    </select>
                  </div>
                  <div class="setting-item">
                    <label>Nền hộp mờ</label>
                    <label class="toggle-row inline"><input type="checkbox" v-model="t.bgBox" /> Dễ đọc hơn</label>
                  </div>
                  <div class="setting-item">
                    <label>Hiện từ giây <span class="hint">0 = từ đầu clip</span></label>
                    <input type="number" v-model.number="t.startTime" min="0" step="0.5" />
                  </div>
                  <div class="setting-item">
                    <label>Ẩn ở giây <span class="hint">0 = tới cuối clip</span></label>
                    <input type="number" v-model.number="t.endTime" min="0" step="0.5" />
                  </div>
                </div>
              </div>
            </div>

            <!-- Watermark / logo (GĐ6) -->
            <div class="settings-group">
              <h3 class="group-title" style="display: flex; align-items: center; gap: 6px;">
                <Layers :size="15" />
                Watermark / Logo
              </h3>
              <label class="toggle-row">
                <input type="checkbox" v-model="editingClip.edit.watermark.enabled" />
                Chèn ảnh watermark / logo
              </label>
              <div v-if="editingClip.edit.watermark.enabled">
                <div class="file-picker-row">
                  <input type="text" class="file-path-input" v-model="editingClip.edit.watermark.imgPath" placeholder="Đường dẫn ảnh PNG/JPG" readonly />
                  <button class="mini-add-btn flex-center" @click="pickWatermark" style="display: inline-flex; align-items: center; gap: 4px;">
                    <FolderOpen :size="12" /> Chọn ảnh
                  </button>
                </div>
                <div class="settings-grid">
                  <div class="setting-item">
                    <label>Vị trí</label>
                    <select v-model="wmPosition">
                      <option value="tr">Góc trên phải</option>
                      <option value="tl">Góc trên trái</option>
                      <option value="br">Góc dưới phải</option>
                      <option value="bl">Góc dưới trái</option>
                      <option value="c">Giữa</option>
                    </select>
                  </div>
                  <div class="setting-item">
                    <label>Kích thước <span class="hint">tỉ lệ so với gốc</span></label>
                    <div class="input-with-unit">
                      <input type="number" v-model.number="editingClip.edit.watermark.scale" min="0.05" max="3" step="0.05" />
                      <span class="unit">×</span>
                    </div>
                  </div>
                  <div class="setting-item">
                    <label>Độ mờ <span class="hint">0 = trong suốt, 1 = đặc</span></label>
                    <input type="number" v-model.number="editingClip.edit.watermark.opacity" min="0.1" max="1" step="0.05" />
                  </div>
                </div>
              </div>
            </div>

            <!-- Âm thanh + nhạc nền (GĐ6) -->
            <div class="settings-group">
              <h3 class="group-title" style="display: flex; align-items: center; gap: 6px;">
                <Music :size="15" />
                Âm thanh & Nhạc nền
              </h3>
              <div class="settings-grid">
                <div class="setting-item">
                  <label>Âm lượng gốc <span class="hint">1 = giữ nguyên</span></label>
                  <input type="number" v-model.number="editingClip.edit.audio.volume" min="0" max="3" step="0.1" :disabled="editingClip.edit.audio.mute" />
                </div>
                <div class="setting-item">
                  <label>Tắt tiếng gốc</label>
                  <label class="toggle-row inline"><input type="checkbox" v-model="editingClip.edit.audio.mute" /> Mute</label>
                </div>
                <div class="setting-item">
                  <label>Fade in (giây)</label>
                  <input type="number" v-model.number="editingClip.edit.audio.fadeIn" min="0" max="10" step="0.5" />
                </div>
                <div class="setting-item">
                  <label>Fade out (giây)</label>
                  <input type="number" v-model.number="editingClip.edit.audio.fadeOut" min="0" max="10" step="0.5" />
                </div>
              </div>
              <div class="file-picker-row" style="margin-top: 12px;">
                <input type="text" class="file-path-input" v-model="editingClip.edit.audio.musicPath" placeholder="Nhạc nền (mp3/wav) — trộn với tiếng gốc" readonly />
                <button class="mini-add-btn flex-center" @click="pickMusic" style="display: inline-flex; align-items: center; gap: 4px;">
                  <Music :size="12" /> Chọn nhạc
                </button>
                <button v-if="editingClip.edit.audio.musicPath" class="mini-del-btn flex-center" @click="editingClip.edit.audio.musicPath = ''" title="Bỏ nhạc nền" style="display: inline-flex; align-items: center; gap: 4px;">
                  <X :size="12" />
                </button>
              </div>
              <div class="settings-grid" v-if="editingClip.edit.audio.musicPath">
                <div class="setting-item">
                  <label>Âm lượng nhạc nền <span class="hint">1 = gốc; giảm để không lấn tiếng</span></label>
                  <input type="number" v-model.number="editingClip.edit.audio.musicVolume" min="0" max="2" step="0.1" />
                </div>
              </div>
            </div>

            <!-- Transition (GĐ7 — dùng khi ghép) -->
            <div class="settings-group">
              <h3 class="group-title" style="display: flex; align-items: center; gap: 6px;">
                <Move :size="15" />
                Chuyển cảnh (khi ghép thành 1 video)
              </h3>
              <div class="settings-grid">
                <div class="setting-item">
                  <label>Kiểu chuyển cảnh
                    <span class="hint">Áp tại mối nối khi bật "Ghép thành 1 video".</span>
                  </label>
                  <select v-model="editingClip.edit.transition.type">
                    <option value="">Không (nối cứng)</option>
                    <option value="fade">Fade (mờ dần)</option>
                    <option value="fadeblack">Fade qua đen</option>
                    <option value="slideleft">Trượt trái</option>
                    <option value="slideright">Trượt phải</option>
                    <option value="wipeleft">Wipe trái</option>
                    <option value="circleopen">Mở vòng tròn</option>
                    <option value="dissolve">Tan (dissolve)</option>
                  </select>
                </div>
                <div class="setting-item">
                  <label>Thời lượng (giây)</label>
                  <div class="input-with-unit">
                    <input type="number" v-model.number="editingClip.edit.transition.duration" min="0" max="3" step="0.1" />
                    <span class="unit">s</span>
                  </div>
                </div>
              </div>
              <p class="preview-note">Transition được đặt ở cấp clip nhưng áp đồng nhất toàn bộ mối nối khi ghép (lấy cấu hình đầu tiên tìm thấy).</p>
            </div>

            <!-- 🎨 Ảnh Thumbnail AI -->
            <div class="settings-group">
              <h3 class="group-title" style="display: flex; align-items: center; gap: 6px;">
                <Sparkles :size="15" />
                Ảnh Thumbnail AI (Gemini + Imagen 4)
              </h3>
              
              <div v-if="!geminiAPIKey" class="group-empty" style="padding: 10px; font-size: 12px; color: var(--wx-brand-accent); display: flex; align-items: center; gap: 4px;">
                <AlertTriangle :size="14" />
                Vui lòng cấu hình <strong>Google Gemini API Key</strong> trong phần <strong>Cài đặt chung</strong> ở Header trước để kích hoạt tính năng này.
              </div>
              <div v-else>
                <div style="margin-bottom: 12px;">
                  <button @click="getClipFrames" :disabled="aiThumbState.isExtractingFrames" class="btn select-btn flex-center" style="font-size:12px; padding: 6px 12px; width: 100%; justify-content: center; background: var(--bg-panel-dark); cursor: pointer;">
                    <svg viewBox="0 0 24 24" width="14" height="14" style="margin-right:4px;"><path fill="currentColor" d="M21 19V5c0-1.1-.9-2-2-2H5c-1.1 0-2 .9-2 2v14c0 1.1.9 2 2 2h14c1.1 0 2-.9 2-2zM8.5 13.5l2.5 3.01L14.5 12l4.5 6H5l3.5-4.5z"/></svg>
                    {{ aiThumbState.isExtractingFrames ? 'Đang trích xuất ảnh mẫu...' : 'Lấy 3 ảnh mẫu từ Clip này' }}
                  </button>
                </div>

                <!-- Hiển thị 3 ảnh mẫu trích xuất -->
                <div v-if="aiThumbState.extractedFrames.length > 0" style="margin-bottom: 15px;">
                  <label class="input-lbl" style="margin-bottom: 6px;">Chọn ảnh mẫu gửi cho AI tham chiếu phong cách:</label>
                  <div style="display: grid; grid-template-columns: repeat(3, 1fr); gap: 8px;">
                    <div v-for="(frame, fi) in aiThumbState.extractedFrames" :key="frame" 
                      style="position: relative; border-radius: 4px; overflow: hidden; border: 2px solid var(--border-color); cursor: pointer;"
                      :style="{ borderColor: aiThumbState.selectedFrames.has(frame) ? 'var(--wx-brand-primary)' : 'var(--border-color)' }"
                      @click="aiThumbState.selectedFrames.has(frame) ? aiThumbState.selectedFrames.delete(frame) : aiThumbState.selectedFrames.add(frame)">
                      <img :src="getThumbUrl(frame)" style="width:100%; height:80px; object-fit:cover; display:block;" />
                      <div style="position: absolute; top: 4px; right: 4px; background: rgba(0,0,0,0.6); border-radius: 50%; width: 18px; height: 18px; display: flex; align-items: center; justify-content: center; color: white; font-size: 10px; font-weight: bold;">
                        <span v-if="aiThumbState.selectedFrames.has(frame)">✓</span>
                      </div>
                    </div>
                  </div>
                </div>

                <!-- Input Prompt -->
                <div style="margin-bottom: 12px;">
                  <label class="input-lbl">Chủ đề hoặc ý tưởng thiết kế (Prompt):</label>
                  <textarea v-model="aiThumbState.userPrompt" placeholder="Ví dụ: Một cô gái xinh đẹp cá tính, phong cách anime rực rỡ, ánh sáng neon hồng xanh lung linh..." 
                    style="width: 100%; height: 70px; box-sizing: border-box; padding: 8px; border-radius: 4px; border: 1px solid var(--border-color); background: var(--bg-card); color: var(--text-main); font-size: 12px; font-family: inherit; resize: vertical;"></textarea>
                </div>

                <div class="settings-grid" style="grid-template-columns: 1fr 1fr; gap: 10px; margin-bottom: 12px;">
                  <div class="setting-item">
                    <label>Tỉ lệ ảnh sinh</label>
                    <select v-model="aiThumbState.aspectRatio" style="width:100%; height: 35px; border-radius: 4px; border: 1px solid var(--border-color); background: var(--bg-card); color: var(--text-main); padding: 0 8px;">
                      <option value="9:16">9:16 (Dọc - TikTok/Reels)</option>
                      <option value="1:1">1:1 (Vuông)</option>
                      <option value="16:9">16:9 (Ngang)</option>
                    </select>
                  </div>
                  <div class="setting-item" style="display: flex; align-items: flex-end;">
                    <button @click="generateAIThumbnailImg" :disabled="aiThumbState.isGenerating || !aiThumbState.userPrompt.trim()" class="btn start-btn flex-center" style="font-size:12px; padding: 0 12px; width: 100%; height: 35px; justify-content: center; background: var(--wx-brand-accent); color: white; cursor: pointer;">
                      🎨 {{ aiThumbState.isGenerating ? 'Đang tạo...' : 'Tạo bằng AI' }}
                    </button>
                  </div>
                </div>

                <!-- Status text / Loading -->
                <div v-if="aiThumbState.statusText" style="font-size: 12px; text-align: center; margin-top: 8px; color: var(--wx-brand-primary);">
                  <span v-if="aiThumbState.isGenerating" style="display:inline-block; animation: spin 1s linear infinite; margin-right: 5px;">🌀</span>
                  {{ aiThumbState.statusText }}
                </div>

                <!-- Preview Generated Image -->
                <div v-if="aiThumbState.generatedImage" style="margin-top: 15px; border-top: 1px solid var(--border-color); padding-top: 12px; text-align: center;">
                  <span class="input-lbl" style="margin-bottom: 6px; display: block;">Ảnh Thumbnail AI đã tạo:</span>
                  <img :src="getThumbUrl(aiThumbState.generatedImage)" style="max-width: 100%; max-height: 250px; border-radius: 6px; box-shadow: 0 4px 10px rgba(0,0,0,0.3); border: 2px solid var(--wx-brand-accent);" />
                  <p style="font-size: 11px; color: var(--text-muted); margin-top: 5px; margin-bottom: 0;">Ảnh bìa đã được tự động lưu và áp dụng cho Clip này.</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Teleport>



    <!-- ===== MODAL CÀI ĐẶT CHUNG (Global Settings) ===== -->
    <Teleport to="body">
      <div class="modal-overlay" v-if="showSettings" @click.self="showSettings = false">
        <div class="settings-modal" style="width: 520px; max-height: 85vh;">
          <div class="modal-header">
            <h2 style="display:flex; align-items:center; gap:6px;">
              <Settings :size="18" style="color:var(--wx-brand-accent);" />
              Cài đặt chung
            </h2>
            <button class="modal-close" @click="showSettings = false"><X :size="16" /></button>
          </div>
          <div class="modal-body" style="overflow-y: auto;">
            <div class="settings-group" style="margin-bottom: 20px; padding-bottom: 15px; border-bottom: 1px solid var(--l-border);">
              <h4 style="margin-top: 0; margin-bottom: 10px; color: var(--wx-brand-accent); font-size: 13.5px; font-weight: 700; display: flex; align-items: center; gap: 6px;">
                <Key :size="15" />
                Cấu hình AI Thumbnail (Google Gemini)
              </h4>
              <div class="setting-item" style="margin-bottom: 8px;">
                <label style="font-size: 12.5px; font-weight: 600;">Google Gemini API Key:</label>
                <input type="password" v-model="geminiAPIKey" class="text-content-input" placeholder="Dán Gemini API Key của bạn..." style="margin-bottom: 0;" />
              </div>
            </div>

            <div class="settings-group" style="margin-bottom: 15px; padding-bottom: 10px; border-bottom: 1px solid var(--l-border);">
              <h4 style="margin-top: 0; margin-bottom: 12px; color: var(--wx-brand-accent); font-size: 13.5px; font-weight: 700; display: flex; align-items: center; gap: 6px;">
                <Chrome :size="15" />
                Cấu hình Google AI (Chrome)
              </h4>
              
              <!-- Checkbox hiển thị Chrome -->
              <div style="margin-bottom: 10px; display: flex; align-items: center;">
                <label class="toggle-row inline" style="font-size: 12.5px; font-weight: 600; display: flex; align-items: center; gap: 8px; cursor: pointer; user-select: none; margin-bottom: 0;">
                  <input type="checkbox" v-model="browserAIShowChrome" style="width:16px; height:16px;" />
                  Hiện trình duyệt Chrome khi chạy Google AI tự động
                </label>
              </div>
              
              <!-- Độ trễ & Đăng nhập chung một dòng -->
              <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 20px; width: 100%; margin-top: 5px;">
                <!-- Độ trễ tự động hóa -->
                <div style="display: flex; flex-direction: row; align-items: center; justify-content: space-between;">
                  <span style="font-size: 12.5px; font-weight: 600; color: var(--text-muted); white-space: nowrap;">Độ trễ:</span>
                  <div style="display: flex; align-items: center; gap: 6px;">
                    <input type="number" v-model="browserAIDelay" min="0.1" max="10.0" step="0.1" class="text-content-input" placeholder="Mặc định: 1.0" style="width: 65px; text-align: center; padding: 4px 8px; height: 30px; margin-bottom: 0;" />
                    <span style="font-size: 12px; color: var(--l-text-muted); white-space: nowrap;">giây</span>
                  </div>
                </div>
                
                <!-- Quản lý phiên đăng nhập Google -->
                <div style="display: flex; flex-direction: row; align-items: center; justify-content: space-between; gap: 8px;">
                  <span style="font-size: 12.5px; font-weight: 600; color: var(--text-muted); white-space: nowrap;">Đăng nhập Google:</span>
                  <div style="display: flex; gap: 8px; align-items: center;">
                    <button @click="openAILogin('flow')" class="mini-add-btn flex-center" style="padding: 6px 12px; white-space: nowrap; cursor: pointer; display: inline-flex; align-items: center; gap: 4px; font-size: 11.5px; height: 30px;">
                      <Chrome :size="13" /> Đăng nhập
                    </button>
                    <button @click="clearAILogin" class="mini-add-btn flex-center" style="padding: 6px 12px; white-space: nowrap; cursor: pointer; display: inline-flex; align-items: center; gap: 4px; font-size: 11.5px; height: 30px; color: var(--wx-danger-solid); border-color: color-mix(in srgb, var(--wx-danger-solid) 30%, transparent);">
                      <Trash2 :size="13" /> Đăng xuất
                    </button>
                  </div>
                </div>
              </div>
            </div>
            
            <div class="settings-group">
              <h4 style="margin-top: 0; margin-bottom: 15px; color: var(--wx-brand-primary); font-size: 13.5px; font-weight: 700; display: flex; align-items: center; gap: 6px;">
                <Cpu :size="15" />
                Cấu hình hiệu năng & hệ thống
              </h4>
              <div class="settings-grid" style="grid-template-columns: 1fr; gap: 15px; margin-bottom: 15px;">
                <!-- Checkbox tự động tạo thư mục con -->
                <div class="setting-item" style="margin-bottom: 5px;">
                  <label class="toggle-row inline" style="font-size: 12.5px; font-weight: 600; display: flex; align-items: center; gap: 8px;">
                    <input type="checkbox" v-model="autoCreateSubfolders" style="width:16px; height:16px;" />
                    Tự động tạo thư mục con (Video / Ảnh) bên trong thư mục xuất
                  </label>
                </div>

                
                <div class="settings-grid" style="grid-template-columns: 1fr 1fr; gap: 15px; margin-top: 5px;">
                  <div>
                    <label style="font-size: 12.5px; font-weight: 600; white-space: nowrap;">Cắt song song:</label>
                    <select v-model="analyzeJobs" style="width: 100%; height: 35px; border-radius: 6px; border: 1px solid var(--l-border); background: var(--l-bg); color: var(--l-text);">
                      <option :value="1">1 video (ổn định)</option>
                      <option :value="2">2 video</option>
                      <option :value="3">3 video</option>
                      <option :value="4">4 video</option>
                    </select>
                  </div>
                  <div>
                    <label style="font-size: 12.5px; font-weight: 600; white-space: nowrap;">Xuất song song:</label>
                    <select v-model="exportJobs" style="width: 100%; height: 35px; border-radius: 6px; border: 1px solid var(--l-border); background: var(--l-bg); color: var(--l-text);">
                      <option :value="1">1 clip</option>
                      <option :value="2">2 clip</option>
                      <option :value="3">3 clip</option>
                      <option :value="4">4 clip</option>
                      <option :value="6">6 clip</option>
                      <option :value="8">8 clip (mạnh)</option>
                    </select>
                  </div>
                </div>
              </div>
            </div>
          </div>
          <div class="modal-footer" style="justify-content: flex-end;">
            <button class="btn save-btn flex-center" @click="showSettings = false" style="display: inline-flex; align-items: center; gap: 4px;">
              <Check :size="15" /> Hoàn tất
            </button>
          </div>
        </div>
      </div>
    </Teleport>

  </main>
</template>

<style scoped src="./VideoSplitter.scoped.css"></style>

<style src="./VideoSplitter.global.css"></style>
