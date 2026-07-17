<script setup lang="ts">
import { ref, computed, onMounted, reactive, watch } from 'vue'
import { GetVideoInfo, Analyze, ExportClips, SelectFiles, CancelAnalysis, GetStreamURL, GetDefaultConfig, GenerateThumbnail, SaveProject, LoadProjectBySource, ListProjects, DeleteProject, SelectImageFile, SelectAudioFile, SelectFolder, CancelExport } from '../../wailsjs/go/main/App'
import { project, storage, main } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { useTheme } from '../ui-system/composables/useTheme'

const { isDark, toggleColorScheme } = useTheme()

const videoPaths = ref<string[]>([])
const activeVideoIndex = ref<number>(0)
const videoInfo = ref<project.VideoInfo | null>(null)
const clipsMap = ref<Record<string, project.Clip[]>>({})
const isAnalyzing = ref(false)
const isExporting = ref(false)
const isMultiExportRunning = ref(false)
const outDir = ref('D:\\Output')

const activeAnalyzingPaths = ref<Set<string>>(new Set())
const analyzeProgressMap = ref<Record<string, number>>({})

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

// === Cấu hình dự án ===
const showAdvancedCutSettings = ref(false)
const analyzeJobs = ref(1)

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
  musicPath: '',        // Nhạc nền
  musicVolume: 0.3,
  muteOriginal: false   // Tắt tiếng gốc
})

// Chọn nhạc nền cho cấu hình chỉnh sửa
const pickGlobalMusic = async () => {
  try {
    const p = await SelectAudioFile()
    if (p) {
      globalRemix.musicPath = p
      addLog('Đã chọn nhạc nền: ' + p.split('\\').pop())
    }
  } catch (e) {
    addLog('Lỗi chọn nhạc nền: ' + String(e))
  }
}

const clearGlobalMusic = () => {
  globalRemix.musicPath = ''
  addLog('Đã bỏ nhạc nền.')
}

const globalMusicName = computed(() => {
  return globalRemix.musicPath ? globalRemix.musicPath.split('\\').pop() : ''
})

// === Cấu hình (Settings) ===
const showSettings = ref(false)
const analyzerConfig = reactive(new project.AnalyzerConfig({
  mode: 'smart',
  sceneThreshold: 27.0,
  minClipDuration: 5.0,
  maxClipDuration: 120.0,
  autoAcceptScore: 85,
  reviewMinScore: 60,
  silenceThreshold: -30,
  silenceDuration: 0.5,
  proxyFPS: 15,
  weights: {
    visualChange: 30,
    blackFrame: 35,
    silence: 20,
    layoutChange: 25,
    audioChange: 20,
    continuityPen: 40
  },
  exportPreset: 'fast',
  exportCRF: 23
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

const videoPlayer = ref<HTMLVideoElement | null>(null)
const editPlayer = ref<HTMLVideoElement | null>(null)

const videoCurrentTime = ref(0)
const onVideoTimeUpdate = (e: Event) => {
  const video = e.target as HTMLVideoElement
  videoCurrentTime.value = video.currentTime
}

const activeVideoPath = computed(() => videoPaths.value[activeVideoIndex.value] || '')
const activeClips = computed(() => clipsMap.value[activeVideoPath.value] || [])

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

onMounted(async () => {
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
        muteOriginal: false
      },
      outDir: 'D:\\Output',
      exportJobs: 2,
      analyzeJobs: 1,
      createdAt: Date.now()
    }]
    activeProjectId.value = defaultId
  } else {
    activeProjectId.value = savedActiveId || namedProjects.value[0].id
  }

  // Load dự án hoạt động đầu tiên
  await loadProject(activeProjectId.value)

  await loadDefaultConfig()
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
  })
  EventsOn('export_progress', (p: { done: number, total: number }) => {
    if (!isMultiExportRunning.value) {
      exportProgress.value = { done: p.done, total: p.total }
    }
  })
})

const getThumbUrl = (path: string) => {
  if (!path || !streamPrefix.value) return ''
  return `${streamPrefix.value}${encodeURIComponent(path)}${streamSuffix.value}`
}

// Lắng nghe tiến trình phân tích từ Go Backend
EventsOn('analyze_progress', (data: { path: string, progress: number }) => {
  if (data && data.path) {
    analyzeProgressMap.value[data.path] = data.progress
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
        alert('Tất cả các video bạn chọn đã có sẵn trong danh sách!')
      }
    }
  } catch (err) {
    alert('Lỗi khi mở hộp thoại: ' + err)
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
    alert('Lỗi đọc thông tin video: ' + err)
    videoInfo.value = null
  }
}

const analyzeSingle = async (path: string): Promise<boolean> => {
  try {
    activeAnalyzingPaths.value.add(path)
    analyzeProgressMap.value[path] = 0
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
    clipsMap.value[path] = clips
    analyzeProgressMap.value[path] = 100
    return true
  } catch (err) {
    const errMsg = String(err)
    if (errMsg.includes('context canceled') || errMsg.includes('canceled') || isCancelled.value) {
      console.log('Phân tích đã bị người dùng hủy.')
    } else {
      alert('Lỗi phân tích: ' + err)
    }
    analyzeProgressMap.value[path] = 0
    return false
  } finally {
    activeAnalyzingPaths.value.delete(path)
  }
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

  // Lọc bỏ video đã cắt rồi (đã có clips) — tự động bỏ qua
  const alreadyCut = allTargets.filter(p => clipsMap.value[p] && clipsMap.value[p].length > 0)
  const targets = allTargets.filter(p => !clipsMap.value[p] || clipsMap.value[p].length === 0)

  if (targets.length === 0) {
    alert(`Tất cả ${alreadyCut.length} video đã được cắt rồi! Không cần cắt lại.`)
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
    alert(`Đã dừng cắt tự động! (${processedVideosCount.value}/${targets.length} video đã xử lý)${skippedMsg}`)
  } else {
    alert(`Cắt tự động hoàn tất! ${processedVideosCount.value}/${targets.length} video thành công.${skippedMsg}`)
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
  const clips = activeClips.value
  return clips.length > 0 && clips.every(c => selectedClips.value.has(c.id))
})

const toggleSelectAll = () => {
  if (isAllSelected.value) {
    selectedClips.value = new Set()
  } else {
    selectedClips.value = new Set(activeClips.value.map(c => c.id))
  }
}

// Số job xuất song song (giới hạn tiến trình ffmpeg như spec yêu cầu).
const exportJobs = ref(2)
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

  if (hasExportedCurrentSession.value) {
    const confirmReExport = confirm('Bạn đã vừa xuất các clip này xong. Bạn có muốn tiếp tục xuất lại không?')
    if (!confirmReExport) return
  }

  isExporting.value = true
  isMultiExportRunning.value = true
  exportProgress.value = { done: 0, total: finalTotal }
  
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

      const results = await ExportClips(projName, videoPath, list, outDir.value, analyzerConfig, exportJobs.value)
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
      alert(`Xuất xong: ${okCount}/${finalTotal} clip OK.\nLỗi: ${failedList.map(f => `${f.video} (Clip #${f.index})`).join(', ')}`)
    } else {
      alert(`Xuất thành công ${okCount} clip!`)
    }
  } catch (err) {
    alert('Lỗi xuất video: ' + err)
  } finally {
    isExporting.value = false
    isMultiExportRunning.value = false
  }
}

// Xuất clip của NHIỀU video được tick (mỗi video một thư mục con theo tên video).
// Chỉ xuất video đã có clip (đã phân tích); bỏ qua video chưa quét.
const exportSelectedVideos = async () => {
  const videos = videosForBatch().filter(p => (clipsMap.value[p]?.length || 0) > 0)
  if (videos.length === 0) {
    alert('Chưa có video nào (được chọn) có đoạn cắt để xuất. Hãy cắt tự động trước.')
    return
  }
  isExporting.value = true
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
      const subDir = outDir.value + '\\' + base
      exportProgress.value = { done: 0, total: clips.length }
      const results = await ExportClips(projName, path, clips, subDir, analyzerConfig, exportJobs.value)
      
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
    alert(`Xuất hàng loạt xong: ${okTotal}/${clipTotal} clip từ ${videos.length} video.`)
  } catch (err) {
    alert('Lỗi xuất hàng loạt: ' + err)
  } finally {
    isExporting.value = false
  }
}

const loadProject = async (projId: string) => {
  const proj = namedProjects.value.find(p => p.id === projId)
  if (!proj) return

  activeProjectId.value = projId
  
  videoPaths.value = [...proj.videoPaths]
  selectedVideos.value = new Set(proj.selectedVideos)
  outDir.value = proj.outDir || 'D:\\Output'
  exportJobs.value = proj.exportJobs || 2
  analyzeJobs.value = proj.analyzeJobs || 1
  
  if (proj.globalRemix) {
    Object.assign(globalRemix, proj.globalRemix)
  }

  if (proj.analyzerConfig) {
    Object.assign(analyzerConfig, proj.analyzerConfig)
  } else {
    // Reset to default settings if no project config exists yet
    analyzerConfig.mode = 'smart'
    analyzerConfig.sceneThreshold = 27.0
    analyzerConfig.minClipDuration = 5.0
    analyzerConfig.maxClipDuration = 120.0
    analyzerConfig.exportPreset = 'fast'
    analyzerConfig.exportCRF = 23
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

watch(outDir, () => {
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
    outDir: 'D:\\Output',
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

const deleteNamedProject = (id: string) => {
  if (namedProjects.value.length <= 1) {
    alert("Không thể xóa dự án duy nhất!")
    return
  }
  if (!confirm("Bạn có chắc chắn muốn xóa dự án này? (Dữ liệu phân đoạn của các video đã phân tích vẫn được lưu ở SQLite)")) {
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

const removeVideo = (index: number) => {
  const path = videoPaths.value[index]
  if (!confirm(`Bạn có muốn xóa video "${path.split('\\').pop()}" khỏi dự án này không?`)) {
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

const playExportedClip = async (clip: project.Clip) => {
  isPlayingExported.value = true
  currentPlayingClipIdx.value = clip.index
  const cleanPath = outDir.value + '\\' + clip.id + '.mp4'
  activeExportedSrc.value = await GetStreamURL(cleanPath)
}

const stopPlayingExported = () => {
  isPlayingExported.value = false
  activeExportedSrc.value = ''
}

const removeClip = (index: number) => {
  if (clipsMap.value[activeVideoPath.value]) {
    clipsMap.value[activeVideoPath.value].splice(index, 1)
    reindexClips()
  }
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
}

// Gộp clip index với clip liền trước nó.
const mergeWithPrev = (index: number) => {
  if (index <= 0) return
  mergeWithNext(index - 1)
}

// === LƯU / MỞ PROJECT (SQLite qua backend) ===

// Lưu phiên làm việc hiện tại (clip + config) để mở lại sau.
const saveProject = async () => {
  if (!activeVideoPath.value || activeClips.value.length === 0) {
    addLog('Chưa có clip để lưu.')
    return
  }
  try {
    await SaveProject(activeVideoPath.value, activeClips.value, analyzerConfig)
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
  audio: { volume: 1, mute: false, musicPath: '', musicVolume: 0.3, fadeIn: 0, fadeOut: 0 },
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
  }
  addLog('Đã áp dụng cấu hình chế cháo cho tất cả clip của video hiện tại.')
  alert('Đã áp dụng cấu hình chế cháo cho tất cả clip của video hiện tại!')
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
    alert('Chưa có video nào (đã tick) được phân tích để áp chỉnh sửa.')
    return
  }
  addLog(`Đã áp chỉnh sửa cho ${clipCount} clip của ${videoCount} video.`)
  alert(`Đã áp bộ chỉnh sửa cho ${clipCount} clip thuộc ${videoCount} video.`)
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
        <svg class="header-icon" viewBox="0 0 24 24"><path fill="currentColor" d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z"/></svg>
        <h1>Smart Video Splitter Studio</h1>
        
        <!-- Bảng chọn dự án -->
        <div class="project-selector-container">
          <label class="project-selector-lbl">Dự án:</label>
          <select :value="activeProjectId" @change="e => loadProject((e.target as HTMLSelectElement).value)" class="project-dropdown">
            <option v-for="p in namedProjects" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
          <button @click="openCreateProject" class="btn-create-proj-mini" title="Tạo dự án mới">
            ➕ Mới
          </button>
          <button @click="openManageProjects" class="btn-manage-proj-mini" title="Quản lý dự án">
            ⚙️ Quản lý
          </button>
        </div>
      </div>
      <div class="header-actions">
        <!-- Nút chọn video -->
        <button @click="handleSelectFiles" class="btn select-btn flex-center">
          <svg class="btn-icon" viewBox="0 0 24 24"><path fill="currentColor" d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/></svg>
          Chọn Video Gốc
        </button>

        <!-- Nút Bắt đầu / Dừng quét phân tích -->
        <button v-if="!isAnalyzing" :disabled="videoPaths.length === 0" @click="analyzeAll" class="btn start-btn flex-center">
          <svg class="btn-icon" viewBox="0 0 24 24"><path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 14.5v-9l6 4.5-6 4.5z"/></svg>
          Bắt Đầu Cắt Tự Động
        </button>
        <button v-else @click="cancelCurrentAnalysis" class="btn stop-analyze-btn flex-center">
          <svg class="btn-icon" viewBox="0 0 24 24"><path fill="currentColor" d="M6 6h12v12H6z"/></svg>
          ⏹ Dừng Ngay ({{ processedVideosCount }}/{{ totalVideosCount }})
        </button>

        <div class="header-divider"></div>

        <!-- Mở dự án gần đây -->
        <button @click="openRecentProjects" class="icon-btn-circle" title="Project đã lưu">
          <svg viewBox="0 0 24 24" width="20" height="20"><path fill="currentColor" d="M13 3a9 9 0 0 0-9 9H1l3.89 3.89.07.14L9 12H6a7 7 0 1 1 7 7c-1.93 0-3.68-.79-4.94-2.06l-1.42 1.42A8.97 8.97 0 0 0 13 21a9 9 0 0 0 0-18zm-1 5v5l4.28 2.54.72-1.21-3.5-2.08V8H12z"/></svg>
        </button>

        <!-- Lưu dự án -->
        <button @click="saveProject" class="icon-btn-circle" title="Lưu phiên làm việc" v-if="activeClips.length > 0">
          <svg viewBox="0 0 24 24" width="20" height="20"><path fill="currentColor" d="M17 3H5c-1.11 0-2 .9-2 2v14c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2V7l-4-4zm-5 16c-1.66 0-3-1.34-3-3s1.34-3 3-3 3 1.34 3 3-1.34 3-3 3zm3-10H5V5h10v4z"/></svg>
        </button>

        <!-- Nút chuyển chế độ Sáng/Tối -->
        <button @click="toggleColorScheme" class="icon-btn-circle theme-toggle-btn" :title="isDark ? 'Chuyển sang giao diện Sáng' : 'Chuyển sang giao diện Tối'">
          <svg v-if="isDark" viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="color: var(--wx-brand-accent);"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>
          <svg v-else viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="color: var(--wx-brand-primary);"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>
        </button>

      </div>
    </header>

    <div class="app-body-vertical">
      <!-- BỐ CỤC TRÊN: DANH SÁCH VIDEO GỐC -->
      <section class="section-top-videos">
        <div class="section-title-bar">
          <div class="title-left">
            <h2>🎥 Danh sách Video Gốc</h2>
            <span class="video-counter">({{ videoPaths.length }} video)</span>
          </div>
          <label class="select-all-label" v-if="videoPaths.length > 0" @click.stop>
            <input type="checkbox" :checked="isAllVideosSelected" @change="toggleSelectAllVideos" class="clip-checkbox" />
            <span>Chọn tất cả video để xử lý</span>
          </label>
        </div>

        <div class="video-horizontal-grid" v-if="videoPaths.length > 0">
          <div v-for="(path, index) in videoPaths" :key="index"
               class="video-card-item"
               :class="{ active: index === activeVideoIndex, 'video-picked': selectedVideos.has(path) }"
               @click="selectVideo(index)"
               :title="path">
            
            <div class="video-card-row">
              <label class="video-select-wrap" @click.stop>
                <input type="checkbox" class="clip-checkbox" :checked="selectedVideos.has(path)" @change="toggleVideoSelected(path)" />
              </label>
              
              <div class="video-card-icon">
                <svg viewBox="0 0 24 24" width="16" height="16"><path fill="currentColor" d="M17 10.5V7c0-.55-.45-1-1-1H4c-.55 0-1 .45-1 1v10c0 .55.45 1 1 1h12c.55 0 1-.45 1-1v-3.5l4 4v-11l-4 4z"/></svg>
              </div>
              
              <span class="video-card-name">{{ path.split('\\').pop() }}</span>
              
              <span class="status-badge-compact done" v-if="clipsMap[path]" :title="`Đã quét ${clipsMap[path].length} clip`">
                ✔ {{ clipsMap[path].length }}
              </span>
              <span class="status-badge-compact pending" v-else title="Chờ quét phân đoạn">
                ⏳
              </span>
              
              <button class="btn-remove-video" @click.stop="removeVideo(index)" title="Xóa video khỏi dự án">✕</button>
            </div>

            <!-- Tiến trình quét -->
            <div class="video-card-progress" v-if="activeAnalyzingPaths.has(path)">
              <div class="progress-bar-track">
                <div class="progress-bar-fill" :style="{ width: (analyzeProgressMap[path] || 0) + '%' }"></div>
              </div>
              <span class="progress-text">{{ analyzeProgressMap[path] || 0 }}%</span>
            </div>
          </div>
        </div>
        <div class="empty-videos-placeholder" v-else>
          <div class="placeholder-content">
            <svg viewBox="0 0 24 24" width="20" height="20" class="placeholder-icon"><path fill="currentColor" d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/></svg>
            <span>Chưa có video gốc nào. Nhấn nút <strong>"Chọn Video Gốc"</strong> để bắt đầu.</span>
          </div>
        </div>
      </section>

      <!-- BỐ CỤC GIỮA: CẤU HÌNH & TRÌNH PHÁT PREVIEW -->
      <section class="section-middle-workspace">
        <!-- Bên Trái: Bảng Cấu Hình Dự Án (Cắt & Sửa) -->
        <div class="project-settings-panel">
          <!-- Phần 1: Cấu hình cắt video -->
          <div class="settings-section-header" style="margin-bottom: 6px;">
            <h3 style="margin: 0; font-size: 13.5px; font-weight: 800; color: var(--accent-color); text-transform: uppercase;">✂️ Cấu hình cắt</h3>
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
                <input type="number" v-model.number="analyzerConfig.minClipDuration" min="1" max="60" @change="analyzerConfig.minClipDuration = clampValue(analyzerConfig.minClipDuration, 1, 60, 5)" class="compact-input" />
              </div>
              <div class="compact-setting-item">
                <label class="setting-title-lbl">Dài nhất (giây):</label>
                <input type="number" v-model.number="analyzerConfig.maxClipDuration" min="10" max="600" @change="analyzerConfig.maxClipDuration = clampValue(analyzerConfig.maxClipDuration, 10, 600, 120)" class="compact-input" />
              </div>
            </div>
            <div class="compact-setting-row" v-else>
              <label class="setting-title-lbl">Thời lượng mỗi clip (giây):</label>
              <input type="number" v-model.number="analyzerConfig.maxClipDuration" min="5" max="600" @change="analyzerConfig.maxClipDuration = clampValue(analyzerConfig.maxClipDuration, 5, 600, 15)" class="compact-input" style="width: 100%;" />
            </div>

            <!-- Cài đặt nâng cao link -->
            <div style="text-align: center; margin: 2px 0;">
              <button class="btn-link-toggle" @click="showAdvancedCutSettings = !showAdvancedCutSettings" style="background: transparent; border: none; color: var(--accent-color); font-size: 11px; font-weight: 700; cursor: pointer;">
                {{ showAdvancedCutSettings ? '▼ Thu gọn tùy chọn khác' : '▶ Xem thêm tùy chọn khác' }}
              </button>
            </div>

            <!-- Tùy chọn khác ẩn/hiện -->
            <div v-if="showAdvancedCutSettings" style="display: flex; flex-direction: column; gap: 8px; background-color: rgba(0,0,0,0.15); padding: 8px; border-radius: 8px; border: 1px solid var(--border-color);">
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
            </div>
          </div>

          <div style="border-bottom: 1px solid var(--border-color); margin: 6px 0;"></div>

          <!-- Phần 2: Cấu hình chỉnh sửa -->
          <div class="settings-section-header" style="margin-bottom: 6px;">
            <h3 style="margin: 0; font-size: 13.5px; font-weight: 800; color: var(--accent-color); text-transform: uppercase;">🎬 Cấu hình sửa</h3>
          </div>

          <div class="remix-options-grid-compact">
            <!-- 1. Lật ngang video (Mirror) -->
            <div class="remix-card-compact" :class="{ enabled: globalRemix.hflip }">
              <label class="toggle-row-compact">
                <input type="checkbox" v-model="globalRemix.hflip" />
                <span>🔄 Lật ngang video (Mirror)</span>
              </label>
            </div>

            <!-- 2. Thay đổi tốc độ video -->
            <div class="remix-card-compact" :class="{ enabled: globalRemix.speed !== 1.0 }">
              <div class="remix-card-row-compact">
                <span class="option-title-compact">⏩ Tốc độ phát:</span>
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
                <span>📐 Tỷ lệ khung hình</span>
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
                <span>🎨 Hiệu ứng màu sắc</span>
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

            <!-- 5. Nhạc nền -->
            <div class="remix-card-compact" :class="{ enabled: globalRemix.musicPath !== '' || globalRemix.muteOriginal }">
              <span class="option-title-compact">🎵 Âm thanh &amp; Nhạc nền</span>
              <div class="audio-controls-compact">
                <label class="toggle-row-mini-compact">
                  <input type="checkbox" v-model="globalRemix.muteOriginal" />
                  <span>Tắt tiếng gốc</span>
                </label>
                <div class="file-picker-row-compact" style="margin-top: 4px;">
                  <button @click="pickGlobalMusic" class="btn-picker-compact">📁 Nhạc nền</button>
                  <button v-if="globalRemix.musicPath" @click="clearGlobalMusic" class="btn-clear-compact">✕</button>
                  <span class="music-name-tag-compact" v-if="globalRemix.musicPath" :title="globalRemix.musicPath">
                    {{ globalMusicName }}
                  </span>
                </div>
                <div class="volume-slider-compact" v-if="globalRemix.musicPath" style="margin-top: 4px;">
                  <span>Âm lượng: {{ Math.round(globalRemix.musicVolume * 100) }}%</span>
                  <input type="range" v-model.number="globalRemix.musicVolume" min="0" max="1" step="0.05" />
                </div>
              </div>
            </div>
          </div>

          <div style="border-bottom: 1px solid var(--border-color); margin: 6px 0;"></div>

          <!-- Phần 3: Cấu hình luồng chạy -->
          <div class="settings-section-header" style="margin-bottom: 6px;">
            <h3 style="margin: 0; font-size: 13.5px; font-weight: 800; color: var(--accent-color); text-transform: uppercase;">⚙️ Cấu hình luồng</h3>
          </div>

          <div class="compact-settings-group-list" style="margin-bottom: 6px;">
            <div class="compact-setting-grid-2">
              <div class="compact-setting-item">
                <label class="setting-title-lbl">Luồng cắt song song:</label>
                <select v-model.number="analyzeJobs" class="compact-select">
                  <option :value="1">1 video (ổn định)</option>
                  <option :value="2">2 video</option>
                  <option :value="3">3 video</option>
                  <option :value="4">4 video (mạnh)</option>
                </select>
              </div>
              <div class="compact-setting-item">
                <label class="setting-title-lbl">Luồng xuất song song:</label>
                <select v-model.number="exportJobs" class="compact-select">
                  <option :value="1">1 clip (yếu)</option>
                  <option :value="2">2 clip</option>
                  <option :value="3">3 clip</option>
                  <option :value="4">4 clip</option>
                  <option :value="6">6 clip</option>
                  <option :value="8">8 clip (mạnh)</option>
                </select>
              </div>
            </div>
          </div>

          <!-- Áp dụng hàng loạt -->
          <div style="margin-top: auto; border-top: 1px solid var(--border-color); padding-top: 8px;">
            <label class="auto-apply-label-compact" style="margin-bottom: 6px; display: flex; align-items: center; gap: 4px; font-size: 11px; color: var(--text-muted); cursor: pointer;">
              <input type="checkbox" v-model="globalRemix.autoApply" />
              <span>Tự động áp dụng khi quét mới</span>
            </label>
            <button @click="applyGlobalRemixToAllActive" class="btn primary-btn flex-center font-bold" style="width: 100%; padding: 6px; border-radius: 6px; background-color: var(--accent-color); color: white; border: none; cursor: pointer; font-size: 11.5px;">
              💾 Áp hiệu ứng cho tất cả clip hiện tại
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
            <!-- Banner video đã xuất -->
            <div class="exported-banner flex-center" v-if="isPlayingExported">
              <svg class="banner-icon" viewBox="0 0 24 24"><path fill="currentColor" d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 14.5v-9l6 4.5-6 4.5z"/></svg>
              <span>Phát thử clip #{{ currentPlayingClipIdx }} đã xuất</span>
              <button @click="stopPlayingExported" class="back-to-original-btn">Xem video gốc</button>
            </div>
            
            <video v-if="activeVideoSrc" ref="videoPlayer" :src="computedVideoSrc" controls class="video-player-mini" @timeupdate="onVideoTimeUpdate"></video>
            <div class="empty-player-screen" v-else>
              <div class="player-emoji">📺</div>
              <span>Chọn một video để xem trước</span>
            </div>

            <!-- Lớp phủ khi đang phân tích AI -->
            <div class="analysis-overlay" v-if="activeAnalyzingPaths.has(activeVideoPath)">
              <div class="overlay-card">
                <div class="spinner"></div>
                <h4>Đang tự động cắt: <span class="processing-name-tag">{{ activeProcessingVideoName }}</span></h4>
                <div class="progress-label flex-between" style="width: 100%;">
                  <span>Tiến trình:</span>
                  <span>{{ analyzeProgressMap[activeVideoPath] || 0 }}%</span>
                </div>
                <div class="progress-container">
                  <div class="progress-bar" :style="{ width: (analyzeProgressMap[activeVideoPath] || 0) + '%' }"></div>
                </div>
              </div>
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
                  :style="{ width: ((clip.endTime - clip.startTime) / videoInfo.Duration * 100) + '%' }"
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

          <!-- Nút hành động riêng lẻ -->
          <div class="single-analyze-trigger-row" v-if="!isAnalyzing && videoPaths.length > 0">
            <button @click="startAnalysis" class="btn single-analyze-btn flex-center">
              🔍 Cắt tự động video đang chọn
            </button>
          </div>
        </div>
      </section>

      <!-- BỐ CỤC DƯỚI: DANH SÁCH CLIPS ĐÃ CẮT & XUẤT BẢN -->
      <section class="section-bottom-clips" :style="{ paddingBottom: selectedClips.size > 0 ? '100px' : '16px' }">
        <div class="section-title-bar-clips">
          <div class="title-left-clips">
            <h2>✂️ Danh sách Video Đã Cắt</h2>
            <span class="clips-counter" v-if="activeClips.length > 0">
              ({{ activeClips.length }} đoạn tìm thấy)
            </span>
          </div>

          <div class="clips-batch-selector-row" v-if="activeClips.length > 0">
            <label class="select-all-label">
              <input type="checkbox" :checked="isAllSelected" @change="toggleSelectAll" class="clip-checkbox" />
              <span>Chọn tất cả clip</span>
            </label>
            <div class="selected-badge" v-if="selectedClips.size > 0">
              Đã chọn: <strong>{{ activeSelectedCount }} / {{ activeClips.length }}</strong> clip của video này (Tổng đã chọn <strong>{{ selectedClips.size }}</strong> clip)
            </div>
          </div>
        </div>

        <div class="clips-view-container">
          <div v-if="activeClips.length > 0" class="clips-grid">
            <div v-for="(clip, idx) in activeClips" :key="clip.id"
              class="clip-modern-card"
              :class="{ 'clip-selected': selectedClips.has(clip.id), 'clip-done': clip.status === 'completed' }">
              
              <div class="clip-card-header-bar">
                <div class="header-left-wrap">
                  <label class="clip-select-wrap" @click.stop>
                    <input type="checkbox" class="clip-checkbox" :checked="selectedClips.has(clip.id)" @change="toggleClipSelected(clip.id)" />
                  </label>
                  <span class="clip-title-tag">Clip #{{ clip.index }}</span>
                  <span class="status-dot done" v-if="clip.status === 'completed'" title="Đã xuất"></span>
                  <span class="status-dot pending" v-else title="Chờ xuất"></span>
                </div>
                <span class="clip-duration-tag">⏱ {{ formatTime(clip.endTime - clip.startTime) }}</span>
              </div>

              <!-- Hình đại diện của clip ngắn -->
              <div class="clip-thumbs-section" v-if="clip.thumbnail">
                <div class="thumb-box-single" @click="jumpToTime(clip.startTime)" title="Bấm để phát thử clip này">
                  <img :src="getThumbUrl(clip.thumbnail)" />
                  <div class="play-overlay">▶</div>
                </div>
              </div>

              <div class="clip-inputs-row">
                <div class="input-block">
                  <span class="input-lbl">Bắt đầu</span>
                  <input type="number" step="1" v-model.number="clip.startTime" @input="updateClipDuration(clip)" @change="refreshClipThumbs(clip)" />
                </div>
                <div class="input-arrow-mini">➔</div>
                <div class="input-block">
                  <span class="input-lbl">Kết thúc</span>
                  <input type="number" step="1" v-model.number="clip.endTime" @input="updateClipDuration(clip)" @change="refreshClipThumbs(clip)" />
                </div>
                <div class="input-actions-block">
                  <button v-if="clip.status === 'completed'" @click="playExportedClip(clip)" class="mini-act-btn btn-watch" title="Phát video đã xuất">📺</button>
                  <button @click="openEdit(idx)" class="mini-act-btn btn-edit" title="Chỉnh sửa (Chèn chữ, watermark...)">✏️ Sửa</button>
                  <button @click="removeClip(idx)" class="mini-act-btn btn-delete" title="Xóa clip">✕</button>
                </div>
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
              <div class="empty-emoji">⏳</div>
              <h3>Đang phân tích video này...</h3>
              <p>Vui lòng chờ hoàn tất, các phân đoạn sẽ hiện ở đây.</p>
            </template>
            <template v-else-if="activeVideoPath">
              <div class="empty-emoji">🎬</div>
              <h3>Video này chưa được cắt</h3>
              <p>Bấm nút <strong>"Bắt Đầu Cắt Tự Động"</strong> ở trên hoặc nút bên dưới để cắt video này.</p>
              <button @click="startAnalysis" class="btn start-btn flex-center" style="margin-top: 12px;" :disabled="isAnalyzing">
                🔍 Cắt video đang chọn
              </button>
            </template>
            <template v-else>
              <div class="empty-emoji">🎬</div>
              <h3>Chưa có video đã cắt nào ở đây</h3>
              <p>Chọn các video gốc phía trên rồi bấm nút <strong>"Bắt Đầu Cắt Tự Động"</strong> để hệ thống tự động cắt cảnh thông minh.</p>
            </template>
          </div>
        </div>

        <!-- PANEL XUẤT BẢN CỐ ĐỊNH Ở CUỐI GÓC DƯỚI CLIPS -->
        <div class="clips-export-publisher-bar" v-if="selectedClips.size > 0">
          <div class="pub-left">
            <div class="pub-input-group">
              <label>📁 Thư mục lưu video xuất:</label>
              <div class="input-with-button-row">
                <input type="text" v-model="outDir" class="pub-text-input" placeholder="D:\Output" style="flex: 1;" />
                <button @click="chooseOutDir" class="btn-picker-folder" title="Chọn thư mục lưu">📂 Chọn thư mục</button>
              </div>
            </div>
          </div>

          <div class="pub-right">
            <!-- Tiến trình xuất -->
            <div v-if="isExporting && exportProgress.total > 0" class="pub-progress-box">
              <div class="progress-info">Đang xuất: {{ exportProgress.done }} / {{ exportProgress.total }} clip</div>
              <div class="progress-bar-container">
                <div class="progress-bar-fill" :style="{ width: (exportProgress.done / exportProgress.total * 100) + '%' }"></div>
              </div>
            </div>

            <button v-if="isExporting" @click="stopExport" class="btn big-export-btn stop-export-btn flex-center">
              <svg viewBox="0 0 24 24" width="20" height="20" class="btn-icon"><rect x="6" y="6" width="12" height="12" fill="currentColor"/></svg>
              <span class="font-bold">Dừng Xuất</span>
            </button>
            <button v-else @click="exportClips" class="btn big-export-btn flex-center">
              <svg viewBox="0 0 24 24" width="20" height="20" class="btn-icon"><path fill="currentColor" d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96zM17 13l-5 5-5-5h3V9h4v4h3z"/></svg>
              <span class="font-bold">
                {{ selectedClips.size > 0 ? `Xuất ${selectedClips.size} Clip Đã Chọn` : `Xuất Toàn Bộ ${activeClips.length} Clip` }}
              </span>
            </button>
          </div>
        </div>
      </section>
    </div>

    <!-- Thanh trạng thái CapCut-style dưới đáy -->
    <footer class="system-status-footer">
      <div class="status-indicator-dot" :class="{ active: isAnalyzing || isExporting }"></div>
      <span class="status-msg-text">{{ statusText }}</span>
    </footer>

    <!-- ===== RECENT PROJECTS MODAL ===== -->
    <Teleport to="body">
      <div class="modal-overlay" v-if="showRecent" @click.self="showRecent = false">
        <div class="settings-modal recent-modal">
          <div class="modal-header">
            <h2>🗂️ Project đã lưu</h2>
            <button class="modal-close" @click="showRecent = false">✕</button>
          </div>
          <div class="modal-body">
            <div v-if="recentProjects.length === 0" class="empty-recent">
              <div class="empty-graphic">🗂️</div>
              <p>Chưa có project nào được lưu. Phân tích một video rồi bấm "Lưu" để lưu phiên làm việc.</p>
            </div>
            <ul v-else class="recent-list">
              <li v-for="proj in recentProjects" :key="proj.id" class="recent-item">
                <div class="recent-info" @click="restoreProject(proj)">
                  <span class="recent-name" :title="proj.sourcePath">{{ proj.name }}</span>
                  <span class="recent-meta">{{ proj.clipCount }} clip · {{ formatTime(proj.duration) }} · {{ proj.status }}</span>
                </div>
                <div class="recent-actions">
                  <button @click="restoreProject(proj)" class="btn-mini open" title="Mở lại">Mở</button>
                  <button @click="deleteProject(proj)" class="btn-mini del" title="Xóa khỏi lịch sử">✕</button>
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
            <h2>➕ Tạo dự án mới</h2>
            <button class="modal-close" @click="showProjectModal = false">✕</button>
          </div>
          <div class="modal-body">
            <div class="setting-item" style="display: flex; flex-direction: column; gap: 8px;">
              <label style="font-weight: 600; font-size: 13px;">Tên dự án mới:</label>
              <input type="text" v-model="newProjectName" class="file-path-input" style="padding: 10px; border-radius: 8px; border: 1px solid var(--l-border); background: var(--l-bg); color: var(--l-text); outline: none;" placeholder="Ví dụ: Kênh Tiktok Review Phim" @keyup.enter="createProject" />
            </div>
            <div class="modal-footer-buttons" style="display: flex; gap: 8px; justify-content: flex-end; margin-top: 16px;">
              <button @click="showProjectModal = false" class="btn cancel-btn" style="background-color: var(--l-bg-sunken); color: var(--l-text); border: 1px solid var(--l-border); padding: 8px 16px; border-radius: 6px; cursor: pointer;">Hủy</button>
              <button @click="createProject" class="btn confirm-btn" :disabled="!newProjectName.trim()" style="background-color: #6366f1; color: #fff; border: none; padding: 8px 16px; border-radius: 6px; cursor: pointer;">Tạo dự án</button>
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
            <h2>⚙️ Quản lý các dự án</h2>
            <button class="modal-close" @click="showManageProjectsModal = false">✕</button>
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
                  <button @click="loadProject(proj.id); showManageProjectsModal = false" class="btn-mini open">Mở</button>
                  <button @click="deleteNamedProject(proj.id)" :disabled="namedProjects.length <= 1" class="btn-mini del" title="Xóa dự án">✕</button>
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
            <h2>✂️ Chỉnh sửa Clip #{{ editingClip.index }}
              <span class="edit-time-sub">{{ formatTime(editingClip.startTime) }} → {{ formatTime(editingClip.endTime) }}</span>
            </h2>
          </div>
          <div class="edit-header-right">
            <button @click="resetEdit" class="btn reset-btn">🔄 Đặt lại</button>
            <button @click="applyEditToAll" class="btn reset-btn" title="Áp bộ chỉnh sửa này cho mọi clip của video hiện tại">📋 Áp cho video này</button>
            <button @click="applyEditToSelectedVideos" class="btn reset-btn" :title="selectedVideos.size > 0 ? `Áp cho mọi clip của ${selectedVideos.size} video đã tick` : 'Áp cho mọi clip của tất cả video đã phân tích'">📦 {{ selectedVideos.size > 0 ? `Áp cho ${selectedVideos.size} video đã chọn` : 'Áp cho tất cả video' }}</button>
            <button @click="showEdit = false" class="btn save-btn">✅ Xong</button>
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
              <h3 class="group-title">📐 Tỉ lệ khung hình</h3>
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
              <h3 class="group-title">🎨 Màu sắc</h3>
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
              <h3 class="group-title">⏩ Tốc độ phát</h3>
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
                <h3 class="group-title">🅰️ Chữ / Phụ đề</h3>
                <button class="mini-add-btn" @click="addText">+ Thêm dòng chữ</button>
              </div>
              <div v-if="editingClip.edit.texts.length === 0" class="group-empty">Chưa có chữ. Bấm "Thêm dòng chữ" để chèn tiêu đề / caption.</div>
              <div v-for="(t, ti) in editingClip.edit.texts" :key="ti" class="text-item-card">
                <div class="text-item-head">
                  <span class="text-item-idx">Dòng #{{ ti + 1 }}</span>
                  <button class="mini-del-btn" @click="removeText(ti)" title="Xóa dòng chữ">✕</button>
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
              <h3 class="group-title">🖼️ Watermark / Logo</h3>
              <label class="toggle-row">
                <input type="checkbox" v-model="editingClip.edit.watermark.enabled" />
                Chèn ảnh watermark / logo
              </label>
              <div v-if="editingClip.edit.watermark.enabled">
                <div class="file-picker-row">
                  <input type="text" class="file-path-input" v-model="editingClip.edit.watermark.imgPath" placeholder="Đường dẫn ảnh PNG/JPG" readonly />
                  <button class="mini-add-btn" @click="pickWatermark">📁 Chọn ảnh</button>
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
              <h3 class="group-title">🎵 Âm thanh & Nhạc nền</h3>
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
                <button class="mini-add-btn" @click="pickMusic">🎵 Chọn nhạc</button>
                <button v-if="editingClip.edit.audio.musicPath" class="mini-del-btn" @click="editingClip.edit.audio.musicPath = ''" title="Bỏ nhạc nền">✕</button>
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
              <h3 class="group-title">🔀 Chuyển cảnh (khi ghép thành 1 video)</h3>
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
          </div>
        </div>
      </div>
    </Teleport>
  </main>
</template>

<style scoped>
/* Core Variables & Reset */
.app-container {
  --bg-app: var(--wx-surface-sunken);
  --bg-panel: var(--wx-surface-base);
  --bg-card: var(--wx-surface-elevated);
  --bg-card-hover: var(--wx-hover-bg);
  --border-color: var(--wx-border-default);
  --accent-color: var(--wx-brand-primary);
  --accent-hover: var(--wx-brand-accent);
  --success-color: var(--wx-success-solid);
  --danger-color: var(--wx-danger-solid);
  --text-main: var(--wx-text-primary);
  --text-muted: var(--wx-text-muted);
  
  font-family: 'Outfit', 'Inter', system-ui, -apple-system, sans-serif;
  background-color: var(--bg-app);
  color: var(--text-main);
  height: 100vh;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

/* Header */
.header {
  background-color: var(--bg-panel);
  border-bottom: 1px solid var(--border-color);
  padding: 12px 24px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  z-index: 100;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.3);
}
.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
}
.project-selector-container {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: 20px;
  background-color: rgba(255, 255, 255, 0.05);
  padding: 4px 10px;
  border-radius: 6px;
  border: 1px solid var(--border-color);
}
.project-selector-lbl {
  font-size: 11px;
  color: var(--text-muted);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
.project-dropdown {
  background: transparent;
  border: none;
  color: var(--text-main);
  font-size: 12.5px;
  font-weight: 700;
  cursor: pointer;
  outline: none;
  padding-right: 4px;
}
.project-dropdown option {
  background-color: var(--bg-panel);
  color: var(--text-main);
}
.btn-create-proj-mini,
.btn-manage-proj-mini {
  background-color: rgba(255, 255, 255, 0.08);
  border: 1px solid var(--border-color);
  color: var(--text-muted);
  font-size: 10.5px;
  padding: 2px 6px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-create-proj-mini:hover,
.btn-manage-proj-mini:hover {
  background-color: rgba(255, 255, 255, 0.15);
  color: var(--text-main);
}
.header-icon {
  width: 26px;
  height: 26px;
  color: var(--accent-color);
}
.header h1 {
  margin: 0;
  font-size: 19px;
  font-weight: 800;
  background: linear-gradient(135deg, #a5b4fc, #818cf8, #6366f1);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  letter-spacing: -0.03em;
}
.header-subtitle {
  font-size: 11.5px;
  color: var(--text-muted);
  background: rgba(255, 255, 255, 0.05);
  padding: 2px 8px;
  border-radius: 6px;
  margin-left: 8px;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.header-divider {
  width: 1px;
  height: 24px;
  background-color: var(--border-color);
  margin: 0 4px;
}

/* Common Components */
.btn {
  padding: 8px 16px;
  border: none;
  border-radius: 8px;
  font-weight: 700;
  font-size: 13.5px;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  display: flex;
  align-items: center;
  gap: 6px;
}
.btn-icon {
  width: 16px;
  height: 16px;
}
.select-btn {
  background-color: transparent;
  border: 1px solid var(--accent-color);
  color: var(--accent-color);
}
.select-btn:hover {
  background-color: var(--hover-bg);
}
.start-btn {
  background: var(--accent-gradient);
  color: #ffffff;
  box-shadow: var(--shadow-sm);
}
.start-btn:hover:not(:disabled) {
  opacity: 0.9;
  transform: translateY(-1px);
  box-shadow: var(--shadow-glow);
}
.start-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
  box-shadow: none;
}
.stop-analyze-btn {
  background: linear-gradient(135deg, #ef4444, #dc2626);
  color: #ffffff;
  box-shadow: 0 0 12px rgba(239, 68, 68, 0.4);
  animation: pulse-stop 1.5s ease-in-out infinite;
  font-weight: 700;
}
.stop-analyze-btn:hover {
  background: linear-gradient(135deg, #dc2626, #b91c1c);
  box-shadow: 0 0 20px rgba(239, 68, 68, 0.6);
  transform: translateY(-1px);
}
@keyframes pulse-stop {
  0%, 100% { box-shadow: 0 0 8px rgba(239, 68, 68, 0.3); }
  50% { box-shadow: 0 0 18px rgba(239, 68, 68, 0.6); }
}
.icon-btn-circle {
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  border-radius: 50%;
  width: 38px;
  height: 38px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-main);
  cursor: pointer;
  transition: all 0.2s;
}
.icon-btn-circle:hover {
  background: var(--hover-bg);
  border-color: var(--accent-hover);
  color: var(--accent-hover);
}

/* Vertical Workspace Layout */
.app-body-vertical {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow-y: auto;
  padding: 16px;
  gap: 16px;
  box-sizing: border-box;
}

/* SECTION TOP: ORIGINAL VIDEOS */
.section-top-videos {
  background-color: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}
.section-title-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.title-left {
  display: flex;
  align-items: center;
  gap: 8px;
}
.title-left h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--accent-color);
}
.video-counter {
  font-size: 12px;
  color: var(--text-muted);
}
.select-all-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  color: var(--text-muted);
  cursor: pointer;
  user-select: none;
  transition: color 0.2s;
}
.select-all-label:hover {
  color: var(--text-main);
}

/* Horizontal scrollable video list */
.video-horizontal-grid {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 6px;
}
.video-horizontal-grid::-webkit-scrollbar {
  height: 6px;
}
.video-horizontal-grid::-webkit-scrollbar-track {
  background: rgba(255, 255, 255, 0.03);
  border-radius: 4px;
}
.video-horizontal-grid::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.15);
  border-radius: 4px;
}
.video-card-item {
  min-width: 250px;
  max-width: 250px;
  background-color: rgba(255, 255, 255, 0.03);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 6px 10px;
  position: relative;
  cursor: pointer;
  transition: all 0.2s ease;
  overflow: hidden;
  height: 36px;
  display: flex;
  align-items: center;
  box-sizing: border-box;
}
.video-card-item:hover {
  background-color: rgba(255, 255, 255, 0.08);
  border-color: #475569;
}
.video-card-item.active {
  border-color: var(--accent-color);
  background-color: rgba(99, 102, 241, 0.12);
  box-shadow: 0 0 10px rgba(99, 102, 241, 0.2);
}
.video-card-item.video-picked {
  background-color: rgba(99, 102, 241, 0.05);
}
.video-card-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
}
.video-select-wrap {
  display: flex;
  align-items: center;
  flex-shrink: 0;
  cursor: pointer;
}
.clip-checkbox {
  width: 16px;
  height: 16px;
  accent-color: var(--accent-color);
  cursor: pointer;
}
.video-card-icon {
  color: var(--text-muted);
  display: flex;
  align-items: center;
  flex-shrink: 0;
}
.video-card-item.active .video-card-icon {
  color: var(--accent-color);
}
.video-card-name {
  font-size: 11.5px;
  font-weight: 700;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--text-main);
  flex: 1;
}
.status-badge-compact {
  font-size: 10px;
  padding: 1px 4px;
  border-radius: 4px;
  font-weight: bold;
  flex-shrink: 0;
}
.status-badge-compact.done {
  background-color: rgba(16, 185, 129, 0.15);
  color: #34d399;
}
.status-badge-compact.pending {
  background-color: rgba(255, 255, 255, 0.05);
  color: var(--text-muted);
}
.btn-remove-video {
  background: transparent;
  border: none;
  color: var(--text-muted);
  font-size: 12px;
  cursor: pointer;
  padding: 2px;
  transition: color 0.2s;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
}
.btn-remove-video:hover {
  color: var(--danger-color);
}
.video-card-progress {
  position: absolute;
  inset: 0;
  background: var(--wx-surface-sunken);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 10px;
  border-radius: 8px;
  gap: 10px;
  z-index: 10;
  box-sizing: border-box;
}
.video-card-progress .progress-bar-track {
  height: 6px;
  background: rgba(128, 128, 128, 0.2);
  border-radius: 3px;
  flex: 1;
  overflow: hidden;
  position: relative;
}
.video-card-progress .progress-bar-fill {
  height: 100%;
  background: var(--accent-color);
  border-radius: 3px;
  transition: width 0.2s ease;
}
.progress-text {
  font-size: 11px;
  color: var(--accent-color);
  font-weight: bold;
  flex-shrink: 0;
  min-width: 32px;
  text-align: right;
}

/* SECTION MIDDLE: SIDE BY SIDE CONFIG & PREVIEW */
.section-middle-workspace {
  display: grid;
  grid-template-columns: 360px 1fr;
  gap: 12px;
  align-items: stretch;
}

.project-settings-panel {
  background-color: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 10px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: 0;
  min-height: 100%;
  box-sizing: border-box;
  overflow-y: auto;
}
.panel-tabs {
  display: flex;
  background-color: rgba(0, 0, 0, 0.2);
  padding: 3px;
  border-radius: 8px;
  border: 1px solid var(--border-color);
  margin-bottom: 12px;
  gap: 2px;
}
.panel-tab-btn {
  flex: 1;
  background: transparent;
  border: none;
  color: var(--text-muted);
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  border-radius: 6px;
  transition: all 0.2s;
  text-align: center;
}
.panel-tab-btn:hover {
  color: var(--text-main);
}
.panel-tab-btn.active {
  background-color: var(--accent-color);
  color: #fff;
  box-shadow: 0 2px 4px rgba(0,0,0,0.2);
}
.panel-tab-content {
  flex: 1;
  overflow-y: auto;
  padding-right: 2px;
  max-height: 380px;
}
.panel-tab-content::-webkit-scrollbar {
  width: 4px;
}
.panel-tab-content::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.08);
  border-radius: 2px;
}

/* Tab 1: Compact cut settings */
.compact-settings-group-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.compact-setting-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.setting-title-lbl {
  font-size: 11px;
  font-weight: 700;
  color: var(--text-muted);
}
.cut-mode-compact-buttons {
  display: flex;
  gap: 4px;
  background-color: rgba(0, 0, 0, 0.2);
  padding: 3px;
  border-radius: 6px;
  border: 1px solid var(--border-color);
}
.btn-cut-mode-pill {
  flex: 1;
  background: transparent;
  border: none;
  color: var(--text-muted);
  padding: 5px 8px;
  font-size: 11px;
  font-weight: 700;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s;
}
.btn-cut-mode-pill:hover {
  color: #fff;
}
.btn-cut-mode-pill.active {
  background-color: var(--accent-color);
  color: #fff;
}
.compact-setting-grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}
.compact-setting-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.compact-input {
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 6px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 700;
  width: 100%;
}
.compact-input:focus {
  border-color: var(--accent-color);
  outline: none;
}
.compact-select {
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 6px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
}

/* Tab 2: Compact Edit presets settings */
.remix-options-grid-compact {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.remix-card-compact {
  background-color: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 8px 10px;
  transition: all 0.2s;
}
.remix-card-compact.enabled {
  border-left: 3px solid var(--accent-color);
  border-color: rgba(99, 102, 241, 0.2);
  background-color: rgba(99, 102, 241, 0.02);
}
.toggle-row-compact {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 700;
  color: var(--text-main);
  cursor: pointer;
  user-select: none;
}
.toggle-row-compact input {
  width: 14px;
  height: 14px;
  accent-color: var(--accent-color);
}
.remix-card-row-compact {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.option-title-compact {
  font-size: 12px;
  font-weight: 700;
  color: var(--text-main);
}
.compact-input-speed {
  width: 50px;
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 4px;
  border-radius: 4px;
  font-size: 11.5px;
  text-align: center;
}
.speed-presets-compact {
  display: flex;
  gap: 4px;
  margin-top: 6px;
}
.btn-preset-mini {
  flex: 1;
  background-color: rgba(255, 255, 255, 0.04);
  border: 1px solid var(--border-color);
  color: var(--text-muted);
  font-size: 10px;
  padding: 2px 4px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.15s;
}
.btn-preset-mini:hover {
  background-color: rgba(255, 255, 255, 0.1);
  color: #fff;
}
.btn-preset-mini.active {
  background-color: var(--accent-color);
  border-color: var(--accent-color);
  color: #fff;
}
.aspect-controls-compact,
.color-controls-compact {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
  background-color: rgba(0, 0, 0, 0.15);
  padding: 6px;
  border-radius: 6px;
}
.compact-select-mini {
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 4px;
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
  width: 100%;
}
.compact-select-mini option {
  background-color: var(--bg-panel);
  color: var(--text-main);
}
.sliders-grid-compact {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px;
}
.slider-item-compact {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.slider-item-compact span {
  font-size: 9.5px;
  color: var(--text-muted);
}
.slider-item-compact input[type="range"] {
  width: 100%;
  accent-color: var(--accent-color);
}
.audio-controls-compact {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-top: 4px;
}
.toggle-row-mini-compact {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  color: var(--text-muted);
  cursor: pointer;
}
.toggle-row-mini-compact input {
  width: 13px;
  height: 13px;
}
.file-picker-row-compact {
  display: flex;
  gap: 4px;
  align-items: center;
}
.btn-picker-compact {
  background-color: rgba(99, 102, 241, 0.15);
  border: 1px solid rgba(99, 102, 241, 0.3);
  color: var(--accent-color);
  font-size: 10px;
  padding: 3px 6px;
  border-radius: 4px;
  cursor: pointer;
}
.btn-picker-compact:hover {
  background-color: rgba(99, 102, 241, 0.25);
}
.btn-clear-compact {
  background-color: rgba(244, 63, 94, 0.15);
  border: 1px solid rgba(244, 63, 94, 0.3);
  color: var(--danger-color);
  padding: 3px 6px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 10px;
}
.music-name-tag-compact {
  font-size: 9.5px;
  color: #fbbf24;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background-color: rgba(251, 191, 36, 0.08);
  padding: 2px 4px;
  border-radius: 4px;
  flex: 1;
}
.volume-slider-compact {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.volume-slider-compact span {
  font-size: 9.5px;
  color: var(--text-muted);
}
.volume-slider-compact input {
  width: 100%;
}

/* Left remix config panel */
.remix-config-panel {
  background-color: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}
.remix-config-panel .panel-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 8px;
}
.remix-config-panel h3 {
  margin: 0;
  font-size: 14px;
  font-weight: 800;
  color: #fbbf24; /* Warning/Remix color */
  text-transform: uppercase;
  letter-spacing: 0.02em;
}
.auto-apply-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 11.5px;
  color: var(--text-muted);
  cursor: pointer;
}
.remix-options-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: 380px;
  overflow-y: auto;
  padding-right: 4px;
}
.remix-options-grid::-webkit-scrollbar {
  width: 4px;
}
.remix-options-grid::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.1);
  border-radius: 2px;
}
.remix-card {
  background-color: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 10px;
  transition: all 0.2s;
}
.remix-card.enabled {
  border-color: rgba(251, 191, 36, 0.4);
  background-color: rgba(251, 191, 36, 0.03);
}
.toggle-row {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  user-select: none;
  font-weight: 700;
  font-size: 12.5px;
  color: var(--text-main);
}
.toggle-row input {
  width: 15px;
  height: 15px;
}
.option-title {
  font-size: 13px;
  font-weight: 700;
}
.option-desc {
  font-size: 11px;
  color: var(--text-muted);
  margin: 4px 0 0 0;
  line-height: 1.35;
}
.remix-card-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.input-with-unit-mini {
  display: flex;
  align-items: center;
  gap: 4px;
}
.input-with-unit-mini input {
  width: 60px;
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 4px 6px;
  border-radius: 4px;
  font-size: 12px;
  text-align: center;
}
.speed-presets {
  display: flex;
  gap: 6px;
  margin-top: 8px;
}
.btn-preset {
  background-color: rgba(255, 255, 255, 0.05);
  border: 1px solid var(--border-color);
  color: var(--text-muted);
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 4px;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-preset:hover {
  background-color: rgba(255, 255, 255, 0.12);
  color: #fff;
}
.btn-preset.active {
  background-color: #fbbf24;
  border-color: #fbbf24;
  color: #000;
  font-bold: true;
}
.aspect-controls,
.color-controls {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
  background-color: rgba(0, 0, 0, 0.15);
  padding: 8px;
  border-radius: 6px;
}
.remix-select {
  background-color: var(--bg-panel);
  border: 1px solid var(--border-color);
  color: #fff;
  padding: 5px 8px;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
}
.sliders-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
}
.slider-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.slider-item label {
  font-size: 10px;
  color: var(--text-muted);
}
.slider-item input[type="range"] {
  width: 100%;
  accent-color: var(--accent-color);
}
.audio-controls {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 6px;
}
.toggle-row-mini {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11.5px;
  cursor: pointer;
}
.toggle-row-mini input {
  width: 14px;
  height: 14px;
}
.btn-picker {
  background-color: rgba(99, 102, 241, 0.15);
  border: 1px solid rgba(99, 102, 241, 0.3);
  color: var(--accent-color);
  font-size: 11px;
  padding: 4px 8px;
  border-radius: 4px;
  cursor: pointer;
}
.btn-picker:hover {
  background-color: rgba(99, 102, 241, 0.25);
}
.file-picker-row {
  display: flex;
  gap: 6px;
  align-items: center;
}
.btn-clear {
  background-color: rgba(244, 63, 94, 0.15);
  border: 1px solid rgba(244, 63, 94, 0.3);
  color: #fda4af;
  padding: 4px 8px;
  border-radius: 4px;
  cursor: pointer;
}
.music-name-tag {
  font-size: 10.5px;
  color: #fbbf24;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background-color: rgba(251, 191, 36, 0.1);
  padding: 3px 6px;
  border-radius: 4px;
  margin-top: 4px;
}
.volume-slider {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: 4px;
}
.volume-slider label {
  font-size: 10px;
  color: var(--text-muted);
}
.volume-slider input {
  width: 100%;
}
.remix-panel-footer {
  margin-top: auto;
  border-top: 1px solid var(--border-color);
  padding-top: 10px;
}

/* Right preview column */
.preview-workspace {
  background-color: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
}
.workspace-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.active-video-details {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  font-size: 11.5px;
}
.video-label-now {
  color: var(--text-muted);
}
.video-name-now {
  font-weight: 800;
  color: var(--accent-color);
}
.video-res-now {
  background-color: rgba(255, 255, 255, 0.05);
  padding: 2px 6px;
  border-radius: 4px;
  color: var(--text-muted);
}
.player-container-mini {
  background-color: #000;
  border-radius: 8px;
  overflow: hidden;
  position: relative;
  aspect-ratio: 16/9;
  width: 100%;
  max-height: 380px;
  border: 1px solid var(--border-color);
  box-shadow: 0 4px 12px rgba(0,0,0,0.4);
}
.video-player-mini {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.empty-videos-placeholder {
  border: 1px dashed var(--border-color);
  border-radius: 10px;
  padding: 20px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: 12.5px;
  background-color: rgba(255, 255, 255, 0.01);
}
.empty-videos-placeholder .placeholder-content {
  display: flex;
  align-items: center;
  gap: 8px;
}
.empty-videos-placeholder .placeholder-icon {
  color: var(--accent-color);
}
.workspace-meta-empty {
  min-height: 22px;
  display: flex;
  align-items: center;
}
.empty-player-screen {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
  font-size: 12.5px;
  background-color: rgba(0, 0, 0, 0.5);
  gap: 8px;
}
.player-emoji {
  font-size: 32px;
}
.exported-banner {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  background-color: rgba(16, 185, 129, 0.95);
  color: #fff;
  padding: 6px 12px;
  font-size: 11px;
  z-index: 5;
  display: flex;
  justify-content: space-between;
}
.back-to-original-btn {
  background-color: #fff;
  color: #047857;
  border: none;
  font-size: 9.5px;
  padding: 2px 6px;
  border-radius: 4px;
  cursor: pointer;
  font-weight: 700;
}
.visual-timeline-container-mini {
  background-color: rgba(0,0,0,0.2);
  border: 1px solid var(--border-color);
  border-radius: 8px;
  padding: 8px;
}
.timeline-header {
  font-size: 10px;
  color: var(--text-muted);
  margin-bottom: 4px;
}
.visual-timeline {
  display: flex;
  height: 24px;
  background-color: rgba(255,255,255,0.05);
  border-radius: 4px;
  overflow: hidden;
  border: 1px solid var(--border-color);
  position: relative;
}
.timeline-playhead {
  position: absolute;
  top: 0;
  bottom: 0;
  width: 3px;
  background-color: var(--wx-danger-solid, #ef4444);
  box-shadow: 0 0 6px rgba(239, 68, 68, 0.9);
  pointer-events: none;
  z-index: 10;
  transform: translateX(-50%);
  transition: left 0.05s linear;
}
.timeline-segment {
  height: 100%;
  border-right: 1px solid var(--bg-panel);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 9px;
  font-weight: 800;
  color: #fff;
  background-color: rgba(99, 102, 241, 0.4);
  transition: all 0.2s;
}
.timeline-segment:hover {
  background-color: var(--accent-color);
  filter: brightness(1.2);
}
.timeline-segment-placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10.5px;
  color: var(--text-muted);
  background-color: rgba(255, 255, 255, 0.02);
  font-weight: 500;
  font-style: italic;
}
.single-analyze-trigger-row {
  margin-top: auto;
}
.single-analyze-btn {
  background-color: rgba(255, 255, 255, 0.05);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  width: 100%;
  padding: 8px;
  font-size: 12.5px;
}
.single-analyze-btn:hover {
  background-color: var(--accent-color);
  color: #fff;
  border-color: var(--accent-color);
}

/* SECTION BOTTOM: CLIPS GRID & EXPORT PANEL */
.section-bottom-clips {
  background-color: var(--bg-panel);
  border: 1px solid var(--border-color);
  border-radius: 12px;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  flex: 1;
}
.section-title-bar-clips {
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid var(--border-color);
  padding-bottom: 10px;
}
.title-left-clips {
  display: flex;
  align-items: center;
  gap: 8px;
}
.title-left-clips h2 {
  margin: 0;
  font-size: 15px;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--accent-color);
}
.clips-counter {
  font-size: 12px;
  color: var(--text-muted);
}
.clips-batch-selector-row {
  display: flex;
  align-items: center;
  gap: 16px;
}
.selected-badge {
  font-size: 11.5px;
  background-color: rgba(16, 185, 129, 0.15);
  color: #34d399;
  padding: 2px 8px;
  border-radius: 6px;
}

/* Grid list of split clips */
.clips-view-container {
  flex: 1;
  overflow-y: auto;
  min-height: 250px;
}
.clips-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 12px;
}
.clip-modern-card {
  background-color: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--border-color);
  border-radius: 10px;
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  transition: all 0.2s;
}
.clip-modern-card:hover {
  background-color: rgba(255, 255, 255, 0.05);
  border-color: #475569;
}
.clip-modern-card.clip-selected {
  border-color: var(--accent-color);
  background-color: rgba(99, 102, 241, 0.05);
}
.clip-modern-card.clip-done {
  border-color: rgba(16, 185, 129, 0.4);
}
.clip-card-header-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.header-left-wrap {
  display: flex;
  align-items: center;
  gap: 6px;
}
.clip-title-tag {
  font-size: 13px;
  font-weight: 800;
  color: var(--text-main);
}
.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.status-dot.done {
  background-color: var(--success-color);
}
.status-dot.pending {
  background-color: var(--danger-color);
}
.clip-duration-tag {
  font-size: 11.5px;
  color: var(--text-muted);
  font-weight: 700;
}
.clip-thumbs-section {
  display: block;
}
.thumb-box-single {
  position: relative;
  aspect-ratio: 16/9;
  background-color: #000;
  border-radius: 6px;
  overflow: hidden;
  border: 1px solid var(--border-color);
  cursor: pointer;
  width: 100%;
}
.thumb-box-single img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.2s;
}
.thumb-box-single:hover img {
  transform: scale(1.05);
}
.thumb-box-single .play-overlay {
  position: absolute;
  inset: 0;
  background: rgba(0,0,0,0.35);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  opacity: 0;
  transition: opacity 0.2s;
}
.thumb-box-single:hover .play-overlay {
  opacity: 1;
}
.clip-inputs-row {
  display: flex;
  align-items: flex-end;
  gap: 6px;
}
.input-block {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.input-lbl {
  font-size: 9px;
  color: var(--text-muted);
  text-transform: uppercase;
}
.input-block input {
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 4px 6px;
  border-radius: 4px;
  font-size: 11.5px;
  font-family: monospace;
  width: 100%;
  min-width: 0;
  box-sizing: border-box;
  -moz-appearance: textfield;
}
.input-block input::-webkit-outer-spin-button,
.input-block input::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
.input-block input:focus {
  border-color: var(--accent-color);
  outline: none;
}
.input-arrow-mini {
  color: var(--text-muted);
  font-size: 11px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* Applied effects display */
.clip-effects-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  min-height: 18px;
  align-items: center;
}
.eff-chip {
  font-size: 9.5px;
  font-weight: 800;
  padding: 1px 6px;
  border-radius: 4px;
  background-color: rgba(255,255,255,0.06);
  color: var(--text-muted);
  border: 1px solid var(--border-color);
}
.eff-chip.chip-lật {
  color: #fca5a5;
  background-color: rgba(239, 68, 68, 0.1);
  border-color: rgba(239, 68, 68, 0.2);
}
.eff-chip.chip-màu {
  color: #fcd34d;
  background-color: rgba(245, 158, 11, 0.1);
  border-color: rgba(245, 158, 11, 0.2);
}
.eff-chip.chip-nhạc {
  color: #6ee7b7;
  background-color: rgba(16, 185, 129, 0.1);
  border-color: rgba(16, 185, 129, 0.2);
}
.eff-chip-empty {
  font-size: 10px;
  color: var(--text-muted);
  font-style: italic;
}

.input-actions-block {
  display: flex;
  align-items: center;
  gap: 4px;
  height: 24px;
}
.mini-act-btn {
  background-color: rgba(255, 255, 255, 0.05);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 4px 6px;
  border-radius: 4px;
  font-size: 11px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  white-space: nowrap;
  height: 100%;
  box-sizing: border-box;
  transition: all 0.2s;
}
.mini-act-btn:hover {
  background-color: var(--accent-color);
  color: #fff;
  border-color: var(--accent-color);
}
.mini-act-btn.btn-watch {
  background-color: rgba(16, 185, 129, 0.1);
  color: #34d399;
  border-color: rgba(16, 185, 129, 0.2);
}
.mini-act-btn.btn-watch:hover {
  background-color: rgba(16, 185, 129, 0.2);
}
.mini-act-btn.btn-delete:hover {
  background-color: var(--danger-color);
  border-color: var(--danger-color);
  color: #fff;
}

.empty-clips-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px;
  color: var(--text-muted);
  text-align: center;
}
.empty-emoji {
  font-size: 38px;
  margin-bottom: 8px;
}
.empty-clips-panel p {
  font-size: 12px;
  max-width: 400px;
  line-height: 1.5;
  margin-top: 4px;
}

/* EXPORT PUBLISHER BAR */
.clips-export-publisher-bar {
  position: fixed;
  bottom: 20px;
  left: 50%;
  transform: translateX(-50%);
  width: calc(100% - 40px);
  max-width: 1100px;
  z-index: 1000;
  background: var(--wx-glass-heavy-bg, rgba(30, 41, 59, 0.9));
  backdrop-filter: blur(12px);
  border: 1px solid var(--wx-border-default);
  border-radius: 12px;
  padding: 12px 24px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 20px;
  animation: slideUpBar 0.3s cubic-bezier(0.34, 1.56, 0.64, 1) both;
}

@keyframes slideUpBar {
  from {
    transform: translate(-50%, 100%) scale(0.95);
    opacity: 0;
  }
  to {
    transform: translate(-50%, 0) scale(1);
    opacity: 1;
  }
}
.pub-left {
  display: flex;
  gap: 16px;
  flex: 1;
}
.pub-input-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
}
.pub-input-group label {
  font-size: 11px;
  color: var(--text-muted);
  font-weight: 700;
}
.pub-text-input {
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 12.5px;
}
.pub-select-input {
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 12.5px;
  cursor: pointer;
}
.pub-right {
  display: flex;
  align-items: center;
  gap: 16px;
  min-width: 320px;
  justify-content: flex-end;
}
.pub-progress-box {
  display: flex;
  flex-direction: column;
  gap: 4px;
  flex: 1;
}
.progress-info {
  font-size: 11px;
  color: var(--success-color);
  text-align: right;
  font-weight: 700;
}
.progress-bar-container {
  height: 6px;
  background-color: rgba(255,255,255,0.05);
  border-radius: 4px;
  overflow: hidden;
  border: 1px solid var(--border-color);
}
.progress-bar-fill {
  height: 100%;
  background: linear-gradient(90deg, var(--success-color), #34d399);
  transition: width 0.3s;
}
.big-export-btn {
  background: linear-gradient(135deg, var(--success-color), #059669);
  color: #fff;
  padding: 10px 24px;
  border-radius: 8px;
  font-size: 14px;
  box-shadow: 0 4px 15px rgba(16, 185, 129, 0.3);
}
.big-export-btn:hover:not(:disabled) {
  background: linear-gradient(135deg, #34d399, var(--success-color));
}
.input-with-button-row {
  display: flex;
  gap: 8px;
  align-items: center;
  width: 100%;
}
.btn-picker-folder {
  background-color: var(--wx-surface-sunken);
  border: 1px solid var(--border-color);
  color: var(--text-main);
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 12.5px;
  font-weight: 700;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
  transition: all 0.2s;
  height: 36px;
  box-sizing: border-box;
}
.btn-picker-folder:hover {
  border-color: var(--accent-color);
  background-color: rgba(99, 102, 241, 0.1);
}
.stop-export-btn {
  background: linear-gradient(135deg, var(--danger-color), #dc2626) !important;
  box-shadow: 0 4px 15px rgba(239, 68, 68, 0.3) !important;
}
.stop-export-btn:hover {
  background: linear-gradient(135deg, #f87171, var(--danger-color)) !important;
}

/* WELCOME EMPTY STATE */
.empty-state-welcome {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
.welcome-box {
  text-align: center;
  background-color: var(--bg-panel);
  border: 1px solid var(--border-color);
  padding: 40px;
  border-radius: 16px;
  max-width: 480px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.4);
}
.welcome-graphic {
  font-size: 60px;
  margin-bottom: 16px;
}
.welcome-box h2 {
  font-size: 20px;
  font-weight: 800;
  margin: 0 0 10px 0;
}
.welcome-box p {
  color: var(--text-muted);
  font-size: 13.5px;
  line-height: 1.5;
  margin: 0 0 24px 0;
}
.select-btn-big {
  background-color: var(--accent-color);
  color: #fff;
  padding: 12px 24px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 700;
  width: 100%;
}
.select-btn-big:hover {
  background-color: var(--accent-hover);
}

/* SYSTEM STATUS FOOTER */
.system-status-footer {
  background-color: #0b0f19;
  border-top: 1px solid var(--border-color);
  padding: 6px 16px;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.status-indicator-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background-color: var(--text-muted);
}
.status-indicator-dot.active {
  background-color: var(--success-color);
  box-shadow: 0 0 8px var(--success-color);
  animation: pulseStatus 1.5s infinite;
}
@keyframes pulseStatus {
  0% { transform: scale(0.9); opacity: 0.6; }
  50% { transform: scale(1.1); opacity: 1; }
  100% { transform: scale(0.9); opacity: 0.6; }
}
.status-msg-text {
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>

<!-- CSS Global cho Teleport (Settings Modal + Edit screen) — LIGHT THEME -->
<style>
/* Bảng màu sáng dùng chung cho các phần teleport (nằm ngoài .app-container
   nên không thừa hưởng biến CSS của component). */
.modal-overlay,
.edit-screen {
  --l-bg: var(--wx-surface-base);
  --l-bg-soft: var(--wx-surface-elevated);
  --l-bg-sunken: var(--wx-surface-sunken);
  --l-border: var(--wx-border-default);
  --l-text: var(--wx-text-primary);
  --l-text-muted: var(--wx-text-secondary);
  --l-accent: var(--wx-brand-primary);
}

/* Modal Overlay */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(15, 18, 30, 0.35);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9000;
  animation: fadeIn 0.2s ease;
}
.settings-modal {
  background: var(--l-bg);
  border: 1px solid var(--l-border);
  border-radius: 16px;
  width: 560px;
  max-height: 85vh;
  display: flex;
  flex-direction: column;
  box-shadow: 0 24px 60px rgba(15, 18, 30, 0.25);
  animation: modalSlideIn 0.3s cubic-bezier(0.16, 1, 0.3, 1);
  color: var(--l-text);
}
.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px 16px;
  border-bottom: 1px solid var(--l-border);
}
.modal-header h2 {
  margin: 0;
  font-size: 17px;
  font-weight: 700;
  color: var(--l-text);
}
.modal-close {
  background: transparent;
  border: none;
  color: var(--l-text-muted);
  font-size: 18px;
  cursor: pointer;
  width: 32px;
  height: 32px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all 0.2s;
}
.modal-close:hover {
  background: var(--l-bg-sunken);
  color: var(--l-text);
}
.modal-body {
  flex: 1;
  overflow-y: auto;
  padding: 18px 24px;
  text-align: left;
}
.modal-footer {
  display: flex;
  justify-content: space-between;
  padding: 16px 24px;
  border-top: 1px solid var(--l-border);
}

/* Settings Groups */
.settings-group {
  margin-bottom: 16px;
}
.settings-group:last-child {
  margin-bottom: 0;
}
.mode-name em {
  font-style: normal;
  font-weight: 600;
  font-size: 10.5px;
  color: #6366f1;
}
.group-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--l-accent);
  margin: 0 0 12px 0;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.settings-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

/* Mode selector (3 chế độ phân tích) */
.mode-selector {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.mode-option {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px 14px;
  border: 1px solid var(--l-border);
  border-radius: 10px;
  cursor: pointer;
  background: var(--l-bg-soft);
  transition: all 0.2s;
}
.mode-option:hover {
  border-color: #b9bdc9;
}
.mode-option.active {
  border-color: var(--l-accent);
  background: rgba(99, 102, 241, 0.08);
}
.mode-option input[type="radio"] {
  accent-color: var(--l-accent);
  flex-shrink: 0;
}
.mode-content {
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.mode-name {
  font-size: 13px;
  font-weight: 700;
  color: var(--l-text);
}
.mode-desc {
  font-size: 11px;
  color: var(--l-text-muted);
  line-height: 1.4;
}
.setting-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.setting-item label {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--l-text);
}
.setting-item .hint {
  display: block;
  font-size: 11px;
  color: var(--l-text-muted);
  font-weight: 400;
  margin-top: 2px;
  line-height: 1.35;
}
.input-with-unit {
  display: flex;
  align-items: center;
  gap: 6px;
}
.input-with-unit input {
  flex: 1;
  padding: 7px 10px;
  background: var(--l-bg);
  border: 1px solid var(--l-border);
  border-radius: 6px;
  color: var(--l-text);
  font-size: 13px;
  font-weight: 500;
}
.input-with-unit input:focus {
  border-color: var(--l-accent);
  outline: none;
  box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.15);
}
.unit {
  font-size: 11px;
  color: var(--l-text-muted);
  min-width: 32px;
}
.setting-item select {
  padding: 7px 10px;
  background: var(--l-bg);
  border: 1px solid var(--l-border);
  border-radius: 6px;
  color: var(--l-text);
  font-size: 13px;
  cursor: pointer;
}
.setting-item select:focus {
  border-color: var(--l-accent);
  outline: none;
}

/* Buttons in modal */
.reset-btn {
  background: transparent;
  border: 1px solid var(--l-border);
  color: var(--l-text-muted);
  padding: 8px 16px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}
.reset-btn:hover {
  background: var(--l-bg-sunken);
  border-color: #b9bdc9;
  color: var(--l-text);
}
.save-btn {
  background: linear-gradient(135deg, #6366f1, #4f46e5);
  border: none;
  color: white;
  padding: 8px 24px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.25);
}
.save-btn:hover {
  background: linear-gradient(135deg, #818cf8, #6366f1);
}
.settings-btn {
  background: transparent !important;
  border: 1px solid #d1d5db !important;
  padding: 8px !important;
  border-radius: 8px !important;
  color: #6b7280 !important;
  cursor: pointer !important;
  display: flex !important;
  align-items: center !important;
  justify-content: center !important;
  box-shadow: none !important;
  transition: all 0.2s !important;
}
.settings-btn:hover {
  border-color: #6366f1 !important;
  color: #6366f1 !important;
  background: rgba(99, 102, 241, 0.06) !important;
}

@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}
@keyframes modalSlideIn {
  from { opacity: 0; transform: scale(0.95) translateY(10px); }
  to { opacity: 1; transform: scale(1) translateY(0); }
}

/* Recent Projects Modal */
.recent-modal {
  width: 520px;
}
.empty-recent {
  text-align: center;
  padding: 40px 20px;
  color: var(--l-text-muted);
}
.empty-recent .empty-graphic {
  font-size: 40px;
  margin-bottom: 12px;
}
.empty-recent p {
  font-size: 13px;
  line-height: 1.5;
}
.recent-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.recent-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  background: var(--l-bg-soft);
  border: 1px solid var(--l-border);
  border-radius: 10px;
  transition: border-color 0.2s;
}
.recent-item:hover {
  border-color: #b9bdc9;
}
.recent-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 4px;
  overflow: hidden;
  cursor: pointer;
}
.recent-name {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--l-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.recent-meta {
  font-size: 11.5px;
  color: var(--l-text-muted);
}
.recent-actions {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
}
.btn-mini {
  border: none;
  border-radius: 6px;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}
.btn-mini.open {
  background: #6366f1;
  color: #fff;
}
.btn-mini.open:hover {
  background: #4f46e5;
}
.btn-mini.del {
  background: var(--l-bg-sunken);
  color: #ef4444;
}
.btn-mini.del:hover {
  background: #ef4444;
  color: #fff;
}

/* Edit clip modal */
.edit-modal {
  width: 640px;
}
.toggle-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--l-text);
  cursor: pointer;
  user-select: none;
  margin-bottom: 12px;
}
.toggle-row input {
  width: 16px;
  height: 16px;
  accent-color: #6366f1;
  cursor: pointer;
}
.toggle-row.inline {
  margin-bottom: 0;
}

/* ===== MÀN HÌNH CHỈNH SỬA CLIP (GĐ6 full screen) — LIGHT ===== */
.edit-screen {
  position: fixed;
  inset: 0;
  z-index: 9500;
  background: var(--l-bg-soft);
  display: flex;
  flex-direction: column;
  animation: fadeIn 0.2s ease;
  color: var(--l-text);
}
.edit-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 14px 24px;
  background: var(--l-bg);
  border-bottom: 1px solid var(--l-border);
  flex-shrink: 0;
}
.edit-header-left {
  display: flex;
  align-items: center;
  gap: 16px;
}
.edit-header-left h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 700;
  color: var(--l-text);
  display: flex;
  align-items: center;
  gap: 10px;
}
.edit-time-sub {
  font-size: 12px;
  color: var(--l-text-muted);
  font-weight: 500;
  font-family: monospace;
}
.edit-back-btn {
  display: flex;
  align-items: center;
  gap: 6px;
  background: var(--l-bg-soft);
  border: 1px solid var(--l-border);
  color: var(--l-text);
  padding: 8px 14px;
  border-radius: 8px;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}
.edit-back-btn svg {
  width: 16px;
  height: 16px;
}
.edit-back-btn:hover {
  background: var(--l-bg-sunken);
  color: var(--l-text);
}
.edit-header-right {
  display: flex;
  gap: 10px;
}
.edit-body {
  flex: 1;
  display: flex;
  overflow: hidden;
}
.edit-preview {
  width: 46%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
  border-right: 1px solid var(--l-border);
  overflow-y: auto;
}
.edit-preview-frame {
  background: #000;
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid var(--l-border);
  margin: 0 auto;
  width: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
}
.edit-preview-frame.ar-16-9 { aspect-ratio: 16/9; }
.edit-preview-frame.ar-9-16 { aspect-ratio: 9/16; max-width: 300px; }
.edit-preview-frame.ar-1-1 { aspect-ratio: 1/1; max-width: 420px; }
.edit-preview-video {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.edit-filter-summary {
  background: var(--l-bg);
  border: 1px solid var(--l-border);
  border-radius: 10px;
  padding: 14px;
}
.summary-title {
  font-size: 12px;
  color: var(--l-text-muted);
  font-weight: 600;
}
.summary-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin-top: 8px;
}
.summary-chip {
  font-size: 11px;
  font-weight: 700;
  padding: 3px 10px;
  border-radius: 12px;
  background: rgba(99, 102, 241, 0.1);
  color: #4f46e5;
  border: 1px solid rgba(99, 102, 241, 0.25);
}
.summary-empty {
  font-size: 12px;
  color: var(--l-text-muted);
  font-style: italic;
}
.preview-note {
  font-size: 11px;
  color: var(--l-text-muted);
  line-height: 1.4;
  margin: 8px 0 0 0;
}
.edit-controls {
  flex: 1;
  padding: 24px;
  overflow-y: auto;
}
.group-title-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
}
.group-title-row .group-title {
  margin: 0;
}
.mini-add-btn {
  background: rgba(99, 102, 241, 0.1);
  color: #4f46e5;
  border: 1px solid rgba(99, 102, 241, 0.3);
  border-radius: 6px;
  padding: 6px 12px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.2s;
}
.mini-add-btn:hover {
  background: rgba(99, 102, 241, 0.18);
}
.mini-del-btn {
  background: var(--l-bg-sunken);
  color: #ef4444;
  border: none;
  border-radius: 6px;
  width: 26px;
  height: 26px;
  font-size: 13px;
  cursor: pointer;
  flex-shrink: 0;
}
.mini-del-btn:hover {
  background: #ef4444;
  color: #fff;
}
.group-empty {
  font-size: 12px;
  color: var(--l-text-muted);
  font-style: italic;
  padding: 8px 0;
}
.text-item-card {
  background: var(--l-bg);
  border: 1px solid var(--l-border);
  border-radius: 10px;
  padding: 12px;
  margin-bottom: 10px;
}
.text-item-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.text-item-idx {
  font-size: 12px;
  font-weight: 700;
  color: #4f46e5;
}
.text-content-input,
.file-path-input {
  width: 100%;
  box-sizing: border-box;
  padding: 8px 10px;
  background: var(--l-bg);
  border: 1px solid var(--l-border);
  border-radius: 6px;
  color: var(--l-text);
  font-size: 13px;
  margin-bottom: 10px;
}
.text-content-input:focus,
.file-path-input:focus {
  border-color: var(--l-accent);
  outline: none;
}
.file-picker-row {
  display: flex;
  gap: 8px;
  align-items: center;
}
.file-picker-row .file-path-input {
  margin-bottom: 0;
}
.setting-item input:disabled {
  opacity: 0.4;
}
</style>