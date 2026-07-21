import { ref, computed, onMounted, onUnmounted } from 'vue'
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime'
// @ts-ignore
import * as BrowserAIService from '../../../wailsjs/go/browserai/Service'

export function useBrowserAI(showToast: (msg: string, type: 'success' | 'error' | 'warning' | 'info') => void) {
  const taskId = ref("")
  const taskState = ref("idle") // idle, launching, login_required, ready, submitting, generating, selection_required, downloading, completed, failed, cancelled
  const progress = ref(0)
  const message = ref("")
  const errorMsg = ref("")
  const result = ref<any>(null)
  const selectionPreviews = ref<string[]>([])
  
  const browserOpen = ref(false)
  const browserProvider = ref("")

  const isGenerating = computed(() => {
    return [
      "launching",
      "ready",
      "submitting",
      "generating",
      "downloading"
    ].includes(taskState.value)
  })

  // Sync browser status & active task info
  const updateBrowserStatus = async () => {
    try {
      const status = await BrowserAIService.GetBrowserStatus()
      browserOpen.value = status.isOpen
      browserProvider.value = status.currentProvider

      // Sync active task from backend if Vue component re-rendered
      if (typeof (BrowserAIService as any).GetActiveTaskInfo === 'function') {
        const activeInfo = await (BrowserAIService as any).GetActiveTaskInfo()
        if (activeInfo && activeInfo.state && activeInfo.state !== 'idle') {
          taskId.value = activeInfo.taskId || taskId.value
          taskState.value = activeInfo.state
          if (activeInfo.message) message.value = activeInfo.message
          if (activeInfo.progress) progress.value = activeInfo.progress
          if (activeInfo.previews && activeInfo.previews.length > 0) {
            selectionPreviews.value = activeInfo.previews
          }
        }
      }
    } catch (_) {}
  }

  const getEventTaskId = (event: any) => {
    return event?.taskId || event?.TaskID || ""
  }

  const matchesCurrentTask = (event: any) => {
    const evId = getEventTaskId(event)
    if (!taskId.value && evId) {
      taskId.value = evId
      return true
    }
    return !evId || evId === taskId.value
  }

  // Wails Event Handlers
  const handleStatus = (event: any) => {
    if (matchesCurrentTask(event)) {
      const evState = event.state || event.State
      if (evState) taskState.value = evState
      if (event.message !== undefined || event.Message !== undefined) message.value = event.message || event.Message
      if (event.progress !== undefined || event.Progress !== undefined) progress.value = event.progress !== undefined ? event.progress : event.Progress
    }
  }

  const handleResult = (event: any) => {
    if (matchesCurrentTask(event)) {
      taskState.value = "completed"
      progress.value = 100
      message.value = "Tác vụ hoàn thành!"
      result.value = event
      showToast("Tạo nội dung AI thành công!", "success")
      updateBrowserStatus()
    }
  }

  const handleError = (event: any) => {
    if (matchesCurrentTask(event)) {
      taskState.value = "failed"
      const msg = event.message || event.Message || String(event)
      errorMsg.value = msg
      message.value = "Lỗi: " + msg
      showToast("Tạo nội dung AI thất bại: " + msg, "error")
      updateBrowserStatus()
    }
  }

  const handleLoginRequired = (event: any) => {
    if (matchesCurrentTask(event)) {
      taskState.value = "login_required"
      message.value = "Yêu cầu đăng nhập tài khoản Google."
      showToast("Vui lòng hoàn tất đăng nhập tài khoản Google trên Chrome.", "warning")
      updateBrowserStatus()
    }
  }

  const handleSelectionRequired = (event: any) => {
    if (matchesCurrentTask(event)) {
      taskState.value = "selection_required"
      message.value = "Vui lòng chọn các ảnh bạn muốn tải về máy."
      progress.value = 75
      selectionPreviews.value = event.previews || event.Previews || []
      showToast("Đã tạo ảnh xong! Hãy tích chọn những ảnh bạn muốn tải.", "info")
      updateBrowserStatus()
    }
  }

  const handleBrowserClosed = () => {
    browserOpen.value = false
    browserProvider.value = ""
  }

  onMounted(() => {
    EventsOn("browser-ai:status", handleStatus)
    EventsOn("browser-ai:result", handleResult)
    EventsOn("browser-ai:error", handleError)
    EventsOn("browser-ai:login-required", handleLoginRequired)
    EventsOn("browser-ai:selection-required", handleSelectionRequired)
    EventsOn("browser-ai:browser-closed", handleBrowserClosed)
    updateBrowserStatus()
  })

  onUnmounted(() => {
    EventsOff("browser-ai:status")
    EventsOff("browser-ai:result")
    EventsOff("browser-ai:error")
    EventsOff("browser-ai:login-required")
    EventsOff("browser-ai:selection-required")
    EventsOff("browser-ai:browser-closed")
  })

  // Action methods
  const openBrowser = async (provider: string, showChrome: boolean = true) => {
    try {
      showToast("Đang mở Chrome...", "info")
      await BrowserAIService.OpenGoogleAI(provider, showChrome)
      await updateBrowserStatus()
      showToast("Đã mở Chrome thành công!", "success")
    } catch (err: any) {
      showToast("Không mở được trình duyệt: " + String(err), "error")
    }
  }

  const checkLogin = async (provider: string) => {
    try {
      const loginStatus = await BrowserAIService.CheckLogin(provider)
      if (loginStatus === "ready") {
        showToast("Đã đăng nhập và sẵn sàng!", "success")
        if (taskState.value === "login_required") {
          taskState.value = "ready"
        }
      } else {
        showToast("Chưa đăng nhập Google.", "warning")
      }
    } catch (err: any) {
      showToast("Không kiểm tra được đăng nhập: " + String(err), "error")
    }
  }

  const generate = async (req: {
    provider: string
    mediaType: string
    prompt: string
    aspectRatio: string
    outputDir: string
    fileName: string
    timeoutSecond: number
    showChrome: boolean
    model?: string
    batchSize?: string
    confirmBeforeCreate?: string
    resolution?: string
    delaySecond?: number
    inputImagePath?: string
    inputImageBase64?: string
    inputImagePaths?: string[]
    inputImageBase64s?: string[]
  }) => {
    try {
      taskId.value = ""
      taskState.value = "launching"
      errorMsg.value = ""
      result.value = null
      progress.value = 5
      message.value = "Đang bắt đầu tác vụ tạo nội dung AI..."

      const taskInfo = await BrowserAIService.Generate({
        provider: req.provider,
        mediaType: req.mediaType,
        prompt: req.prompt,
        aspectRatio: req.aspectRatio,
        outputDir: req.outputDir,
        fileName: req.fileName,
        timeoutSecond: req.timeoutSecond,
        openBrowser: true,
        showChrome: req.showChrome,
        model: req.model || "",
        batchSize: req.batchSize || "",
        confirmBeforeCreate: req.confirmBeforeCreate || "",
        resolution: req.resolution || "1K",
        delaySecond: req.delaySecond || 1.0,
        inputImagePath: req.inputImagePath || "",
        inputImageBase64: req.inputImageBase64 || "",
        inputImagePaths: req.inputImagePaths || [],
        inputImageBase64s: req.inputImageBase64s || [],
        logPrefix: ""
      })

      taskId.value = taskInfo.taskId
      if (taskInfo.state && taskInfo.state !== 'idle' && taskState.value === 'launching') {
        taskState.value = taskInfo.state
      }
      if (taskInfo.message) {
        message.value = taskInfo.message
      }
      
      updateBrowserStatus()
    } catch (err: any) {
      taskState.value = "failed"
      errorMsg.value = String(err)
      message.value = "Lỗi: " + String(err)
      showToast("Không khởi tạo được tác vụ: " + String(err), "error")
    }
  }

  const submitSelection = async (selectedIndexes: number[]) => {
    if (!taskId.value) return
    try {
      taskState.value = "downloading"
      progress.value = 85
      message.value = "Đang tải các ảnh đã chọn..."
      await BrowserAIService.SubmitSelection(taskId.value, selectedIndexes)
    } catch (err: any) {
      showToast("Lỗi gửi lựa chọn: " + String(err), "error")
    }
  }

  const cancel = async () => {
    if (!taskId.value) return
    try {
      await BrowserAIService.Cancel(taskId.value)
      taskState.value = "cancelled"
      message.value = "Tác vụ bị hủy bởi người dùng."
      showToast("Đã hủy tác vụ Google AI!", "warning")
    } catch (err: any) {
      showToast("Lỗi hủy tác vụ: " + String(err), "error")
    }
  }

  const clearBrowserProfile = async () => {
    try {
      await BrowserAIService.ClearBrowserProfile()
      browserOpen.value = false
      browserProvider.value = ""
      showToast("Đã xóa sạch phiên đăng nhập Google AI!", "success")
    } catch (err: any) {
      showToast("Lỗi xóa profile: " + String(err), "error")
    }
  }

  const closeBrowser = async () => {
    try {
      await BrowserAIService.CloseBrowser()
      browserOpen.value = false
      browserProvider.value = ""
      showToast("Đã đóng trình duyệt Chrome.", "info")
    } catch (_) {}
  }

  const reset = () => {
    taskId.value = ""
    taskState.value = "idle"
    progress.value = 0
    message.value = ""
    errorMsg.value = ""
    result.value = null
  }

  return {
    taskId,
    state: taskState,
    progress,
    message,
    error: errorMsg,
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
  }
}
