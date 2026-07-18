import { ref, computed, watch, type Ref } from 'vue'
import { SearchImages, DownloadImages, CancelImageDownload, GetDefaultImageDownloadDir, SelectFolder } from '../../../wailsjs/go/main/App'
import { imagedownloader } from '../../../wailsjs/go/models'
import { EventsOn } from '../../../wailsjs/runtime/runtime'

// === Types ===
export type ImageSource = 'duckduckgo' | 'pixabay' | 'unsplash' | 'pexels'

export interface ImageProgressItem {
  id: string
  title: string
  percent: number
  done: number
  total: number
  status: 'downloading' | 'done' | 'error'
  filePath?: string
}

export const IMAGE_SOURCES: { value: ImageSource; label: string; icon: string; needsKey: boolean; hint: string }[] = [
  {
    value: 'duckduckgo',
    label: 'DuckDuckGo',
    icon: '🦆',
    needsKey: false,
    hint: 'Miễn phí, không cần API Key. Tổng hợp ảnh từ nhiều nguồn.',
  },
  {
    value: 'pixabay',
    label: 'Pixabay',
    icon: '🎨',
    needsKey: true,
    hint: 'Ảnh miễn phí bản quyền. Cần API Key (đăng ký miễn phí tại pixabay.com/api/docs)',
  },
  {
    value: 'unsplash',
    label: 'Unsplash',
    icon: '📷',
    needsKey: true,
    hint: 'Ảnh chụp chất lượng cao. Cần Access Key (unsplash.com/developers)',
  },
  {
    value: 'pexels',
    label: 'Pexels',
    icon: '🖼️',
    needsKey: true,
    hint: 'Ảnh + video miễn phí chất lượng cao. Cần API Key (pexels.com/api)',
  },
]

/**
 * Composable quản lý tìm kiếm và tải ảnh theo chủ đề.
 * @param showToast - hàm hiển thị thông báo từ component cha
 */
export function useImageDownloader(
  showToast: (msg: string, type: 'success' | 'error' | 'info' | 'warning') => void,
) {
  // === State ===
  const showImagePanel = ref(false)
  const imageQuery = ref('')
  const imageSource = ref<ImageSource>('duckduckgo')
  const imageApiKey = ref('')
  const imageMaxCount = ref(50)
  const isSearching = ref(false)
  const isDownloading = ref(false)
  const searchResult = ref<imagedownloader.ImageSearchResult | null>(null)
  const imageDir = ref('')
  const selectedImageIds = ref<Set<string>>(new Set())
  const progressMap = ref<Map<string, ImageProgressItem>>(new Map())
  const searchLog = ref<string[]>([])
  const downloadDoneCount = ref(0)
  const downloadTotalCount = ref(0)

  // === Computed ===
  const selectedSource = computed(() => IMAGE_SOURCES.find(s => s.value === imageSource.value))
  const sourceNeedsKey = computed(() => selectedSource.value?.needsKey ?? false)

  const selectedCount = computed(() => selectedImageIds.value.size)

  const allEntries = computed(() => searchResult.value?.entries ?? [])

  const downloadProgress = computed(() => {
    if (downloadTotalCount.value === 0) return 0
    return Math.round((downloadDoneCount.value / downloadTotalCount.value) * 100)
  })

  const getEnvKeyForSource = (src: ImageSource): string => {
    if (src === 'pixabay') return (import.meta.env.VITE_PIXABAY_API_KEY as string) || (import.meta.env.VITE_PIXABAY_KEY as string) || ''
    if (src === 'unsplash') return (import.meta.env.VITE_UNSPLASH_ACCESS_KEY as string) || (import.meta.env.VITE_UNSPLASH_API_KEY as string) || (import.meta.env.VITE_UNSPLASH_KEY as string) || ''
    if (src === 'pexels') return (import.meta.env.VITE_PEXELS_API_KEY as string) || (import.meta.env.VITE_PEXELS_KEY as string) || ''
    return ''
  }

  const hasEnvKey = computed(() => {
    return !!getEnvKeyForSource(imageSource.value)
  })

  watch(imageSource, (newSource) => {
    imageApiKey.value = getEnvKeyForSource(newSource)
  }, { immediate: true })

  // === Helpers ===
  const addLog = (msg: string) => {
    searchLog.value.unshift('[' + new Date().toLocaleTimeString() + '] ' + msg)
    if (searchLog.value.length > 50) searchLog.value = searchLog.value.slice(0, 50)
  }

  // === Actions ===
  const toggleSelectAll = () => {
    if (selectedImageIds.value.size === allEntries.value.length) {
      selectedImageIds.value = new Set()
    } else {
      selectedImageIds.value = new Set(allEntries.value.map(e => e.id))
    }
  }

  const toggleImage = (id: string) => {
    const s = new Set(selectedImageIds.value)
    if (s.has(id)) s.delete(id)
    else s.add(id)
    selectedImageIds.value = s
  }

  const openImagePanel = async () => {
    showImagePanel.value = true
    if (!imageDir.value) {
      try {
        imageDir.value = await GetDefaultImageDownloadDir()
      } catch (_) { }
    }
  }

  const searchImages = async () => {
    const query = imageQuery.value.trim()
    if (!query) {
      showToast('Vui lòng nhập chủ đề/từ khóa tìm kiếm!', 'warning')
      return
    }

    isSearching.value = true
    searchResult.value = null
    selectedImageIds.value = new Set()
    progressMap.value = new Map()

    const apiKey = imageApiKey.value.trim() || getEnvKeyForSource(imageSource.value)

    try {
      const result = await SearchImages(
        query,
        imageSource.value,
        apiKey,
        imageMaxCount.value,
      )
      searchResult.value = result
      // Tự động chọn tất cả kết quả
      selectedImageIds.value = new Set(result.entries.map(e => e.id))
      showToast(`Tìm thấy ${result.entries.length} ảnh từ ${result.source}!`, 'success')
    } catch (err) {
      showToast('Lỗi tìm kiếm: ' + String(err), 'error')
    } finally {
      isSearching.value = false
    }
  }

  const startImageDownload = async () => {
    if (selectedImageIds.value.size === 0) {
      showToast('Chưa chọn ảnh nào để tải!', 'warning')
      return
    }

    const toDownload = allEntries.value.filter(e => selectedImageIds.value.has(e.id))
    isDownloading.value = true
    downloadDoneCount.value = 0
    downloadTotalCount.value = toDownload.length
    progressMap.value = new Map()

    try {
      const results = await DownloadImages(toDownload, imageDir.value)
      const okCount = results.filter(r => r.ok).length
      showToast(`Đã tải xong ${okCount}/${results.length} ảnh!`, okCount > 0 ? 'success' : 'error')
    } catch (err) {
      showToast('Lỗi tải ảnh: ' + String(err), 'error')
    } finally {
      isDownloading.value = false
    }
  }

  const cancelImageDl = async () => {
    try {
      await CancelImageDownload()
      isDownloading.value = false
      showToast('Đã hủy tải ảnh!', 'warning')
    } catch (_) { }
  }

  const pickImageDir = async () => {
    try {
      const dir = await SelectFolder()
      if (dir) imageDir.value = dir
    } catch (_) { }
  }

  // === Event Listeners ===
  const initImageEvents = () => {
    EventsOn('image_search_log', (msg: string) => {
      addLog(msg)
    })

    EventsOn('image_download_progress', (data: any) => {
      if (data && data.id) {
        downloadDoneCount.value = data.done || 0
        downloadTotalCount.value = data.total || downloadTotalCount.value
        progressMap.value.set(data.id, {
          id: data.id,
          title: data.title || '',
          percent: data.percent || 0,
          done: data.done || 0,
          total: data.total || 0,
          status: 'downloading',
        })
      }
    })

    EventsOn('image_download_done', (data: any) => {
      if (data) {
        downloadDoneCount.value = data.ok || 0
        downloadTotalCount.value = data.total || downloadTotalCount.value
      }
    })
  }

  return {
    // State
    showImagePanel,
    imageQuery,
    imageSource,
    imageApiKey,
    imageMaxCount,
    isSearching,
    isDownloading,
    searchResult,
    imageDir,
    selectedImageIds,
    progressMap,
    searchLog,
    downloadDoneCount,
    downloadTotalCount,
    // Computed
    selectedSource,
    sourceNeedsKey,
    selectedCount,
    allEntries,
    downloadProgress,
    hasEnvKey,
    // Constants
    IMAGE_SOURCES,
    // Actions
    toggleSelectAll,
    toggleImage,
    openImagePanel,
    searchImages,
    startImageDownload,
    cancelImageDl,
    pickImageDir,
    initImageEvents,
  }
}
