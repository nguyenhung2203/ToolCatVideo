package browserai

type ErrorCode string

const (
	ErrBrowserNotFound     ErrorCode = "browser_not_found"
	ErrBrowserLaunch       ErrorCode = "browser_launch_failed"
	ErrProfileLocked       ErrorCode = "profile_locked"
	ErrLoginRequired       ErrorCode = "login_required"
	ErrSelectorNotFound    ErrorCode = "selector_not_found"
	ErrFeatureUnavailable  ErrorCode = "feature_unavailable"
	ErrGenerationRejected  ErrorCode = "generation_rejected"
	ErrQuotaExceeded       ErrorCode = "quota_exceeded"
	ErrGenerationTimeout   ErrorCode = "generation_timeout"
	ErrGenerationFailed    ErrorCode = "generation_failed"
	ErrDownloadUnavailable ErrorCode = "download_unavailable"
	ErrDownloadInvalid     ErrorCode = "download_invalid"
	ErrCancelled           ErrorCode = "cancelled"
)

type BrowserAIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *BrowserAIError) Error() string {
	return e.Message
}

func NewError(code ErrorCode, message string) *BrowserAIError {
	return &BrowserAIError{
		Code:    code,
		Message: message,
	}
}
