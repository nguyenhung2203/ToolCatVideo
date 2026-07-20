package browserai

type TaskState string

const (
	TaskStateIdle          TaskState = "idle"
	TaskStateLaunching     TaskState = "launching"
	TaskStateLoginRequired TaskState = "login_required"
	TaskStateReady         TaskState = "ready"
	TaskStateSubmitting    TaskState = "submitting"
	TaskStateGenerating    TaskState = "generating"
	TaskStateDownloading   TaskState = "downloading"
	TaskStateCompleted     TaskState = "completed"
	TaskStateFailed        TaskState = "failed"
	TaskStateCancelled     TaskState = "cancelled"
	TaskStateSelectionRequired TaskState = "selection_required"
)

type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
)

type Provider string

const (
	ProviderGemini Provider = "gemini"
	ProviderFlow   Provider = "flow"
)

type LoginStatus string

const (
	LoginUnknown  LoginStatus = "unknown"
	LoginRequired LoginStatus = "required"
	LoginReady    LoginStatus = "ready"
)

type GenerateRequest struct {
	Provider            Provider  `json:"provider"`
	MediaType           MediaType `json:"mediaType"`
	Prompt              string    `json:"prompt"`
	AspectRatio         string    `json:"aspectRatio"`
	OutputDir           string    `json:"outputDir"`
	FileName            string    `json:"fileName"`
	TimeoutSecond       int       `json:"timeoutSecond"`
	OpenBrowser         bool      `json:"openBrowser"`
	ShowChrome          bool      `json:"showChrome"`
	Model               string    `json:"model"`
	BatchSize           string    `json:"batchSize"`
	ConfirmBeforeCreate string    `json:"confirmBeforeCreate"`
	Resolution          string    `json:"resolution"`
	DelaySecond         float64   `json:"delaySecond"`
	InputImagePath      string    `json:"inputImagePath"`
	InputImageBase64    string    `json:"inputImageBase64"`
	InputImagePaths     []string  `json:"inputImagePaths"`
	InputImageBase64s   []string  `json:"inputImageBase64s"`
}

type GenerateResult struct {
	TaskID      string    `json:"taskId"`
	Provider    Provider  `json:"provider"`
	MediaType   MediaType `json:"mediaType"`
	FilePath    string    `json:"filePath"`
	FileName    string    `json:"fileName"`
	FilePaths   []string  `json:"filePaths"`
	MimeType    string    `json:"mimeType"`
	FileSize    int64     `json:"fileSize"`
	DurationMS  int64     `json:"durationMs"`
	CompletedAt string    `json:"completedAt"`
}

type StatusEvent struct {
	TaskID   string    `json:"taskId"`
	State    TaskState `json:"state"`
	Message  string    `json:"message"`
	Progress int       `json:"progress"`
	Detail   string    `json:"detail,omitempty"`
}

type BrowserStatus struct {
	IsOpen          bool   `json:"isOpen"`
	CurrentProvider string `json:"currentProvider"`
	ProfileDir      string `json:"profileDir"`
}

type TaskInfo struct {
	TaskID    string    `json:"taskId"`
	State     TaskState `json:"state"`
	Message   string    `json:"message"`
	StartedAt string    `json:"startedAt"`
}

type SelectionEvent struct {
	TaskID   string   `json:"taskId"`
	Previews []string `json:"previews"`
}
