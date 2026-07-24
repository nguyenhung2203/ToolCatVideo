package googlesheet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// SheetLinkInfo chứa thông tin bóc tách từ URL Google Sheet
type SheetLinkInfo struct {
	SpreadsheetID string `json:"spreadsheetId"`
	Gid           string `json:"gid"`
	TabName       string `json:"tabName"`
	RawURL        string `json:"rawUrl"`
}

// SheetTabInfo đại diện cho 1 Tab thực tế trên Google Sheet với danh sách Cột
type SheetTabInfo struct {
	Name    string   `json:"name"`
	Gid     string   `json:"gid"`
	Headers []string `json:"headers"`
}

// Service quản lý kết nối và gửi dữ liệu tới Google Sheets
type Service struct {
	client *http.Client
}

// NewService tạo instance mới cho Service
func NewService() *Service {
	return &Service{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// interpretWebAppError chuyển phản hồi lỗi thô của Google (thường là cả trang HTML dài) thành
// thông báo ngắn gọn, đúng nguyên nhân. Lỗi phổ biến nhất: deploy Web App KHÔNG chọn quyền
// "Anyone" → Google chặn, trả trang đăng nhập/từ chối (HTTP 403 hoặc HTML) thay vì JSON.
func interpretWebAppError(statusCode int, body string) error {
	lower := strings.ToLower(body)
	isHTML := strings.Contains(lower, "<!doctype html") || strings.Contains(lower, "<html")
	looksLikeLogin := strings.Contains(lower, "accounts.google.com") ||
		strings.Contains(lower, "truy cập bị từ chối") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "sign in") ||
		strings.Contains(lower, "đăng nhập")

	if statusCode == http.StatusForbidden || statusCode == http.StatusUnauthorized || looksLikeLogin || (isHTML && statusCode != http.StatusOK) {
		return fmt.Errorf("Google chặn truy cập Web App (HTTP %d). Nguyên nhân thường gặp: khi Triển khai (Deploy) chưa đặt quyền \"Who has access\" = \"Anyone\" (Bất kỳ ai). Hãy vào Apps Script → Deploy → Manage deployments → sửa quyền thành Anyone → deploy lại → lấy URL /exec mới", statusCode)
	}

	// Không phải lỗi quyền → cắt ngắn body cho dễ đọc (tránh dump cả trang HTML).
	snippet := strings.TrimSpace(body)
	if len(snippet) > 200 {
		snippet = snippet[:200] + "..."
	}
	return fmt.Errorf("Web App trả về lỗi HTTP %d: %s", statusCode, snippet)
}

// ParseLink bóc tách Spreadsheet ID và GID (ID Tab) từ đường link Google Sheet bất kỳ
func (s *Service) ParseLink(rawURL string) (*SheetLinkInfo, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("đường link Google Sheet không được để trống")
	}

	info := &SheetLinkInfo{
		RawURL: rawURL,
	}

	// Match Spreadsheet ID
	reID := regexp.MustCompile(`/spreadsheets/d/([a-zA-Z0-9-_]+)`)
	matchesID := reID.FindStringSubmatch(rawURL)
	if len(matchesID) > 1 {
		info.SpreadsheetID = matchesID[1]
	} else {
		return nil, fmt.Errorf("không tìm thấy Spreadsheet ID hợp lệ trong đường link")
	}

	// Match GID (ID của tab)
	reGID := regexp.MustCompile(`[?&]gid=([0-9]+)`)
	matchesGID := reGID.FindStringSubmatch(rawURL)
	if len(matchesGID) > 1 {
		info.Gid = matchesGID[1]
	} else {
		reGIDFrag := regexp.MustCompile(`#gid=([0-9]+)`)
		matchesGIDFrag := reGIDFrag.FindStringSubmatch(rawURL)
		if len(matchesGIDFrag) > 1 {
			info.Gid = matchesGIDFrag[1]
		} else {
			info.Gid = "0"
		}
	}

	if info.Gid == "1228770940" {
		info.TabName = "WEB - THỦY"
	}

	return info, nil
}

// FetchSheetStructure đọc toàn bộ danh sách Tab & Cột THẬT từ Google Sheet qua Apps Script
// Web App URL. Bắt buộc phải có Web App URL — không có thì báo lỗi rõ ràng (không trả data
// mẫu để tránh ngụy trang trạng thái "chưa kết nối").
func (s *Service) FetchSheetStructure(ctx context.Context, webAppURL string, spreadsheetID string, defaultGid string) ([]SheetTabInfo, error) {
	webAppURL = strings.TrimSpace(webAppURL)

	if webAppURL == "" {
		return nil, fmt.Errorf("chưa có Web App URL. Bấm \"Lấy mã Apps Script\" để cài đặt rồi dán URL vào ô \"Google Apps Script Web App URL\"")
	}

	if _, err := url.ParseRequestURI(webAppURL); err != nil {
		return nil, fmt.Errorf("Web App URL không hợp lệ: %v", err)
	}

	payload := WebAppPayload{Action: "get_sheet_info"}
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("lỗi đóng gói dữ liệu: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webAppURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("lỗi tạo request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("không kết nối được Web App: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lỗi đọc phản hồi Web App: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, interpretWebAppError(resp.StatusCode, string(respBody))
	}

	// Google có thể trả HTTP 200 kèm trang HTML đăng nhập khi quyền deploy sai.
	bodyLower := strings.ToLower(strings.TrimSpace(string(respBody)))
	if strings.HasPrefix(bodyLower, "<!doctype html") || strings.HasPrefix(bodyLower, "<html") {
		return nil, interpretWebAppError(resp.StatusCode, string(respBody))
	}

	var res struct {
		Status  string         `json:"status"`
		Message string         `json:"message"`
		Sheets  []SheetTabInfo `json:"sheets"`
	}
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("phản hồi Web App không đúng chuẩn JSON: %s", string(respBody))
	}

	if res.Status == "error" {
		return nil, fmt.Errorf("Apps Script báo lỗi: %s", res.Message)
	}

	if len(res.Sheets) == 0 {
		return nil, fmt.Errorf("Web App không trả về tab nào. Kiểm tra lại mã Apps Script đã cập nhật đúng chưa")
	}

	return res.Sheets, nil
}

// FetchSheetHeaders tải 1 dòng đầu tiên của tab qua Google CSV API để tự bóc tách Tên & Cột của Tab
func (s *Service) FetchSheetHeaders(ctx context.Context, spreadsheetID string, gid string) ([]string, string, error) {
	if spreadsheetID == "" {
		return nil, "", fmt.Errorf("không có spreadsheetID")
	}
	if gid == "" {
		gid = "0"
	}

	csvURL := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", spreadsheetID, gid)
	req, err := http.NewRequestWithContext(ctx, "GET", csvURL, nil)
	if err != nil {
		return nil, "", err
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("không thể đọc CSV (status %d)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	lines := strings.Split(string(body), "\n")
	if len(lines) == 0 {
		return nil, "", fmt.Errorf("file rỗng")
	}

	firstLine := lines[0]
	cols := strings.Split(firstLine, ",")
	cleanCols := []string{}
	for i := range cols {
		val := strings.Trim(strings.TrimSpace(cols[i]), `"`)
		if val != "" {
			cleanCols = append(cleanCols, val)
		}
	}

	detectedTabName := ""
	headerStr := strings.ToUpper(firstLine)
	if strings.Contains(headerStr, "TIÊU ĐỀ") || strings.Contains(headerStr, "BÌNH LUẬN") || strings.Contains(headerStr, "LINK BÀI VIẾT") {
		detectedTabName = "WEB - THỦY"
	} else if strings.Contains(headerStr, "HASHTAG") || strings.Contains(headerStr, "CAPTION") || strings.Contains(headerStr, "CMT ĐĂNG") {
		detectedTabName = "CONTEN THỦY"
	}

	return cleanCols, detectedTabName, nil
}

// WebAppPayload định dạng dữ liệu gửi sang Google Apps Script
type WebAppPayload struct {
	Action  string     `json:"action"`            // "append_row", "append_batch", "get_sheet_info"
	Gid     string     `json:"gid,omitempty"`     // GID của tab
	TabName string     `json:"tabName,omitempty"` // Tên tab (nếu chỉ định theo tên)
	Row     []string   `json:"row,omitempty"`     // 1 dòng dữ liệu
	Rows    [][]string `json:"rows,omitempty"`    // Nhiều dòng dữ liệu
}

// WebAppResponse phản hồi từ Google Apps Script Web App
type WebAppResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// PushRowToWebApp gửi 1 dòng dữ liệu lên Apps Script Web App URL
func (s *Service) PushRowToWebApp(ctx context.Context, webAppURL string, gid string, tabName string, row []string) error {
	payload := WebAppPayload{
		Action:  "append_row",
		Gid:     gid,
		TabName: tabName,
		Row:     row,
	}

	return s.sendRequest(ctx, webAppURL, payload)
}

// PushBatchToWebApp gửi danh sách nhiều dòng dữ liệu lên Apps Script Web App URL
func (s *Service) PushBatchToWebApp(ctx context.Context, webAppURL string, gid string, tabName string, rows [][]string) error {
	payload := WebAppPayload{
		Action:  "append_batch",
		Gid:     gid,
		TabName: tabName,
		Rows:    rows,
	}

	return s.sendRequest(ctx, webAppURL, payload)
}

func (s *Service) sendRequest(ctx context.Context, webAppURL string, payload WebAppPayload) error {
	webAppURL = strings.TrimSpace(webAppURL)
	if webAppURL == "" {
		return fmt.Errorf("vui lòng nhập Web App URL (từ Google Apps Script)")
	}

	_, err := url.ParseRequestURI(webAppURL)
	if err != nil {
		return fmt.Errorf("đường dẫn Web App URL không hợp lệ: %v", err)
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("lỗi đóng gói dữ liệu JSON: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webAppURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("lỗi tạo request HTTP: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("lỗi kết nối tới Web App: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("lỗi đọc phản hồi từ Web App: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return interpretWebAppError(resp.StatusCode, string(respBody))
	}

	var res WebAppResponse
	if err := json.Unmarshal(respBody, &res); err != nil {
		if strings.Contains(strings.ToLower(string(respBody)), "success") {
			return nil
		}
		return interpretWebAppError(resp.StatusCode, string(respBody))
	}

	if res.Status != "success" && res.Status != "ok" {
		msg := res.Message
		if msg == "" {
			msg = "xảy ra lỗi không xác định trên Apps Script"
		}
		return fmt.Errorf("lỗi ghi Google Sheet: %s", msg)
	}

	return nil
}

// GetAppsScriptTemplate Code Apps Script tự động bóc tách tất cả Tab & Cột trên Google Sheet
func (s *Service) GetAppsScriptTemplate() string {
	return `// --- MÃ GOOGLE APPS SCRIPT CHO GOOGLE SHEET (TỰ ĐỘNG BÓC TÁCH TAB & CỘT) ---
// Hướng dẫn: Mở Google Sheet -> Tiện ích mở rộng -> Apps Script -> Dán mã này -> Triển khai Web App (Quyền: Bất kỳ ai)

function doGet(e) {
  return handleGetSheets();
}

function doPost(e) {
  try {
    var data = {};
    if (e && e.postData && e.postData.contents) {
      data = JSON.parse(e.postData.contents);
    }
    
    if (data.action === "get_sheet_info") {
      return handleGetSheets();
    }

    var ss = SpreadsheetApp.getActiveSpreadsheet();
    var sheet;

    if (data.gid) {
      var sheets = ss.getSheets();
      for (var i = 0; i < sheets.length; i++) {
        if (sheets[i].getSheetId().toString() === data.gid.toString()) {
          sheet = sheets[i];
          break;
        }
      }
    }
    if (!sheet && data.tabName) {
      sheet = ss.getSheetByName(data.tabName);
    }
    if (!sheet) {
      sheet = ss.getActiveSheet();
    }

    if (data.action === "append_batch" && data.rows && data.rows.length > 0) {
      for (var r = 0; r < data.rows.length; r++) {
        sheet.appendRow(data.rows[r]);
      }
    } else if (data.row && data.row.length > 0) {
      sheet.appendRow(data.row);
    }

    return ContentService.createTextOutput(JSON.stringify({ status: "success", message: "Đã chèn thành công" }))
      .setMimeType(ContentService.MimeType.JSON);
  } catch (error) {
    return ContentService.createTextOutput(JSON.stringify({ status: "error", message: error.toString() }))
      .setMimeType(ContentService.MimeType.JSON);
  }
}

function handleGetSheets() {
  try {
    var ss = SpreadsheetApp.getActiveSpreadsheet();
    var sheets = ss.getSheets();
    var result = [];
    for (var i = 0; i < sheets.length; i++) {
      var s = sheets[i];
      var lastCol = s.getLastColumn();
      var headers = [];
      if (lastCol > 0) {
        var row1 = s.getRange(1, 1, 1, lastCol).getValues()[0];
        for (var c = 0; c < row1.length; c++) {
          var name = row1[c] ? row1[c].toString().trim() : "";
          if (name !== "") {
            headers.push(name);
          }
        }
      }
      result.push({
        name: s.getName(),
        gid: s.getSheetId().toString(),
        headers: headers
      });
    }
    return ContentService.createTextOutput(JSON.stringify({ status: "success", sheets: result }))
      .setMimeType(ContentService.MimeType.JSON);
  } catch (err) {
    return ContentService.createTextOutput(JSON.stringify({ status: "error", message: err.toString() }))
      .setMimeType(ContentService.MimeType.JSON);
  }
}`
}
