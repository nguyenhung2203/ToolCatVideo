import { ref, computed, watch, type Ref } from 'vue'
import { ProbeOnlineURL, DownloadOnlineVideos, CancelDownload, GetDefaultDownloadDir, SelectFolder, GetGlobalSettings, SaveGlobalSettings } from '../../../wailsjs/go/main/App'
import { downloader } from '../../../wailsjs/go/models'
import { EventsOn } from '../../../wailsjs/runtime/runtime'

// === Types ===
export interface DlProgressItem {
  videoId: string
  title: string
  percent: number
  speed: string
  eta: string
  status: 'downloading' | 'done' | 'error'
  filePath?: string
  error?: string
}

export type DlLinkType = 'auto' | 'video' | 'profile' | 'playlist'
export type DlFetchOrder = 'newest' | 'oldest'

/**
 * Composable quản lý tải video online (yt-dlp).
 * @param showToast - hàm hiển thị thông báo từ component cha
 * @param videoPaths - ref danh sách đường dẫn video trong project (để auto-add)
 */
export function useDownloader(
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void,
  videoPaths: Ref<string[]>,
) {
  // === State ===
  const showDownloadPanel = ref(false)
  const downloadUrl = ref('')
  const downloadMode = ref<'link' | 'search'>('link')
  const isProbing = ref(false)
  const isDownloading = ref(false)
  const probeResult = ref<downloader.URLProbeResult | null>(null)
  const downloadDir = ref('')

  // Tự động nhận diện loại downloadMode từ nội dung input
  watch(downloadUrl, (newVal) => {
    const val = newVal.trim()
    if (!val) {
      downloadMode.value = 'link'
      return
    }
    const isUrl = val.startsWith('http://') || val.startsWith('https://') || val.includes('.') && !val.includes(' ')
    downloadMode.value = isUrl ? 'link' : 'search'
  })
  const cookieBrowser = ref('')
  const dlSelectedIds = ref<Set<string>>(new Set())
  const dlSortBy = ref<'views' | 'likes' | 'date' | 'duration'>('views')
  const dlKeyword = ref('')
  const dlMinViews = ref(0)
  const dlProgressMap = ref<Map<string, DlProgressItem>>(new Map())
  const searchSource = ref<'youtube' | 'tiktok' | 'facebook' | 'all'>('youtube')

  // Cấu hình dò link
  const dlLinkType = ref<DlLinkType>('auto')
  const dlMaxCount = ref(30)         // giới hạn số video khi dò profile/playlist
  const dlFetchOrder = ref<DlFetchOrder>('newest')  // thứ tự lấy: mới nhất / cũ nhất

  const isSettingsLoaded = ref(false)

  const saveSettings = async () => {
    if (!isSettingsLoaded.value) return
    try {
      const settingsStr = await GetGlobalSettings()
      let gSettings: any = {}
      if (settingsStr) {
        gSettings = JSON.parse(settingsStr)
      }
      gSettings.videoDownloaderConfig = {
        downloadDir: downloadDir.value,
        cookieBrowser: cookieBrowser.value,
        dlLinkType: dlLinkType.value,
        searchSource: searchSource.value,
        dlMaxCount: dlMaxCount.value,
        dlFetchOrder: dlFetchOrder.value
      }
      await SaveGlobalSettings(JSON.stringify(gSettings))
    } catch (e) {
      console.error('Lỗi tự động lưu VideoDownloaderConfig:', e)
    }
  }

  let saveTimeout: any = null
  const triggerSaveSettings = () => {
    if (!isSettingsLoaded.value) return
    if (saveTimeout) clearTimeout(saveTimeout)
    saveTimeout = setTimeout(() => {
      saveSettings()
    }, 800)
  }

  watch([downloadDir, cookieBrowser, dlLinkType, searchSource, dlMaxCount, dlFetchOrder], () => {
    triggerSaveSettings()
  })

  // === Computed ===
  const detectedLinkType = computed((): DlLinkType => {
    const url = downloadUrl.value.trim().toLowerCase()
    if (!url) return 'auto'
    // TikTok profile: /@username
    if (/tiktok\.com\/@[\w.]+\/?$/i.test(url)) return 'profile'
    // TikTok single video: /@username/video/xxx or /video/xxx
    if (/tiktok\.com\/.*\/video\/\d+/i.test(url)) return 'video'
    // YouTube single video
    if (/youtube\.com\/watch\?v=/i.test(url) || /youtu\.be\/[\w-]+$/i.test(url)) return 'video'
    // YouTube channel/playlist
    if (/youtube\.com\/(channel|c|user|@|playlist)/i.test(url)) return 'profile'
    // Facebook video
    if (/facebook\.com\/.*\/videos\//i.test(url) || /facebook\.com\/.*\/reel\//i.test(url) || /fb\.watch/i.test(url)) return 'video'
    // Facebook page/profile
    if (/facebook\.com\/([\w.]+)\/?$/i.test(url)) return 'profile'
    return 'auto'
  })

  // Loại link thực tế (ưu tiên chọn tay, fallback detect tự động)
  const effectiveLinkType = computed(() => {
    if (dlLinkType.value !== 'auto') return dlLinkType.value
    return detectedLinkType.value
  })

  // Có phải profile/playlist không (để hiện cấu hình giới hạn)
  const isProfileOrPlaylist = computed(() => {
    if (downloadMode.value === 'search') {
      return true
    }
    const url = downloadUrl.value.trim().toLowerCase()
    if (url && !url.startsWith('http://') && !url.startsWith('https://')) {
      return true
    }
    const t = effectiveLinkType.value
    return t === 'profile' || t === 'playlist'
  })

  // === Computed ===
  const filteredDlEntries = computed(() => {
    if (!probeResult.value) return []
    let list = [...probeResult.value.entries]
    if (dlKeyword.value.trim()) {
      const kw = dlKeyword.value.trim().toLowerCase()
      list = list.filter(e => (e.title || '').toLowerCase().includes(kw))
    }
    if (dlMinViews.value > 0) {
      list = list.filter(e => (e.viewCount || 0) >= dlMinViews.value)
    }
    list.sort((a, b) => {
      switch (dlSortBy.value) {
        case 'views': return (b.viewCount || 0) - (a.viewCount || 0)
        case 'likes': return (b.likeCount || 0) - (a.likeCount || 0)
        case 'date': return (b.uploadDate || '').localeCompare(a.uploadDate || '')
        case 'duration': return (b.duration || 0) - (a.duration || 0)
        default: return 0
      }
    })
    return list
  })

  const dlSelectedCount = computed(() => dlSelectedIds.value.size)

  // === Helpers ===
  const formatViewCount = (n: number | undefined): string => {
    if (!n) return '0'
    if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M'
    if (n >= 1_000) return (n / 1_000).toFixed(1) + 'K'
    return String(n)
  }

  const formatDlDate = (d: string | undefined): string => {
    if (!d || d.length !== 8) return d || ''
    return d.slice(0, 4) + '-' + d.slice(4, 6) + '-' + d.slice(6, 8)
  }

  // === Actions ===
  const dlToggleAll = () => {
    if (dlSelectedIds.value.size === filteredDlEntries.value.length) {
      dlSelectedIds.value = new Set()
    } else {
      dlSelectedIds.value = new Set(filteredDlEntries.value.map(e => e.id))
    }
  }

  const dlToggle = (id: string) => {
    const s = new Set(dlSelectedIds.value)
    if (s.has(id)) s.delete(id)
    else s.add(id)
    dlSelectedIds.value = s
  }

  const openDownloadPanel = async () => {
    showDownloadPanel.value = true
    try {
      const settingsStr = await GetGlobalSettings()
      if (settingsStr) {
        const gSettings = JSON.parse(settingsStr)
        if (gSettings.videoDownloaderConfig) {
          const cfg = gSettings.videoDownloaderConfig
          if (cfg.downloadDir) downloadDir.value = cfg.downloadDir
          if (cfg.cookieBrowser !== undefined) cookieBrowser.value = cfg.cookieBrowser
          if (cfg.dlLinkType) dlLinkType.value = cfg.dlLinkType
          if (cfg.searchSource) searchSource.value = cfg.searchSource
          if (cfg.dlMaxCount) dlMaxCount.value = cfg.dlMaxCount
          if (cfg.dlFetchOrder) dlFetchOrder.value = cfg.dlFetchOrder
        }
      }
    } catch (e) {
      console.error('Lỗi nạp cấu hình VideoDownloaderConfig:', e)
    }

    isSettingsLoaded.value = true

    if (!downloadDir.value) {
      try { downloadDir.value = await GetDefaultDownloadDir() } catch (_) {}
    }
  }

  const probeUrl = async () => {
    const rawUrl = downloadUrl.value.trim()

    if (!rawUrl) {
      if (downloadMode.value === 'link') {
        showToast('Vui lòng dán link video!', 'warning')
      } else {
        showToast('Vui lòng nhập từ khóa tìm kiếm!', 'warning')
      }
      return
    }

    isProbing.value = true
    probeResult.value = null
    dlSelectedIds.value = new Set()
    dlProgressMap.value = new Map()

    const maxCount = isProfileOrPlaylist.value ? dlMaxCount.value : 0
    const sortOrder = dlFetchOrder.value

    try {
      const result = await ProbeOnlineURL(
        rawUrl,
        cookieBrowser.value,
        maxCount,
        sortOrder,
        downloadMode.value === 'link' ? "" : searchSource.value,
      )
      probeResult.value = result
      if (result.type === 'video' && result.entries.length === 1) {
        dlSelectedIds.value = new Set([result.entries[0].id])
      }
      showToast(`Tìm thấy ${result.entries.length} video từ ${result.platform}!`, 'success')
    } catch (err) {
      showToast('Lỗi dò link: ' + String(err), 'error')
    } finally {
      isProbing.value = false
    }
  }

  const startDownload = async () => {
    if (dlSelectedIds.value.size === 0) {
      showToast('Chưa chọn video nào để tải!', 'warning')
      return
    }
    const entries = filteredDlEntries.value.filter(e => dlSelectedIds.value.has(e.id))
    isDownloading.value = true

    for (const e of entries) {
      dlProgressMap.value.set(e.id, {
        videoId: e.id, title: e.title, percent: 0, speed: '', eta: '', status: 'downloading'
      })
    }

    try {
      await DownloadOnlineVideos(entries, downloadDir.value, cookieBrowser.value)
      showToast('Đã tải xong tất cả video đã chọn!', 'success')
    } catch (err) {
      showToast('Lỗi tải video: ' + String(err), 'error')
    } finally {
      isDownloading.value = false
    }
  }

  const cancelDl = async () => {
    try {
      await CancelDownload()
      isDownloading.value = false
      showToast('Đã hủy tải video!', 'warning')
    } catch (_) {}
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    showToast('Đã sao chép đường dẫn video gốc!', 'success')
  }

  const pickDownloadDir = async () => {
    try {
      const dir = await SelectFolder()
      if (dir) downloadDir.value = dir
    } catch (_) {}
  }

  const addDownloadedToProject = (filePath: string) => {
    if (!filePath || videoPaths.value.includes(filePath)) return
    videoPaths.value = [...videoPaths.value, filePath]
    showToast('Đã thêm video vào project: ' + filePath.split('\\').pop(), 'success')
  }

  // === Event Listeners (gọi 1 lần khi composable được khởi tạo) ===
  const initDownloadEvents = (addLog: (msg: string) => void) => {
    EventsOn('download_progress', (data: any) => {
      if (data && data.videoId) {
        const existing = dlProgressMap.value.get(data.videoId)
        dlProgressMap.value.set(data.videoId, {
          videoId: data.videoId,
          title: data.title || existing?.title || '',
          percent: data.percent || 0,
          speed: data.speed || '',
          eta: data.eta || '',
          status: 'downloading',
        })
      }
    })

    EventsOn('download_complete', (data: any) => {
      if (data && data.videoId) {
        const existing = dlProgressMap.value.get(data.videoId)
        dlProgressMap.value.set(data.videoId, {
          videoId: data.videoId,
          title: data.title || existing?.title || '',
          percent: 100,
          speed: '',
          eta: '',
          status: data.ok ? 'done' : 'error',
          filePath: data.filePath || '',
          error: data.error || '',
        })
        if (data.ok && data.filePath) {
          addDownloadedToProject(data.filePath)
        }
      }
    })

    EventsOn('download_log', (msg: string) => {
      addLog('[Tải Video] ' + msg)
    })
  }

  // === Remove selected entries from results ===
  function removeSelectedEntries() {
    if (!probeResult.value || dlSelectedIds.value.size === 0) return
    const toRemove = new Set(dlSelectedIds.value)
    probeResult.value = {
      ...probeResult.value,
      entries: probeResult.value.entries.filter((e: any) => !toRemove.has(e.id)),
    }
    dlSelectedIds.value = new Set()
  }

  return {
    // State
    showDownloadPanel,
    downloadUrl,
    downloadMode,
    isProbing,
    isDownloading,
    probeResult,
    downloadDir,
    cookieBrowser,
    dlSelectedIds,
    dlSortBy,
    dlKeyword,
    dlMinViews,
    dlProgressMap,
    dlLinkType,
    dlMaxCount,
    dlFetchOrder,
    searchSource,
    // Computed
    filteredDlEntries,
    dlSelectedCount,
    detectedLinkType,
    effectiveLinkType,
    isProfileOrPlaylist,
    // Helpers
    formatViewCount,
    formatDlDate,
    // Actions
    dlToggleAll,
    dlToggle,
    openDownloadPanel,
    probeUrl,
    startDownload,
    cancelDl,
    pickDownloadDir,
    addDownloadedToProject,
    initDownloadEvents,
    copyToClipboard,
    removeSelectedEntries,
  }
}
