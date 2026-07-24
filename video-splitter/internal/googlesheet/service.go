package googlesheet

import (
	"bytes"
	"context"
	"encoding/csv"
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
	Name       string   `json:"name"`
	Gid        string   `json:"gid"`
	Headers    []string `json:"headers"`
	RawHeaders []string `json:"rawHeaders,omitempty"`
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

	if u, uerr := url.Parse(webAppURL); uerr == nil {
		if apiKey := u.Query().Get("key"); apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else if token := u.Query().Get("token"); token != "" {
			req.Header.Set("X-API-Key", token)
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

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

	// Tự động bổ sung RawHeaders & Headers chuẩn 1-to-1 vị trí cột tuyệt đối
	for i := range res.Sheets {
		if spreadsheetID != "" {
			rawCols := s.fetchRawCsvHeaders(ctx, spreadsheetID, res.Sheets[i].Gid)
			if len(rawCols) > 0 {
				res.Sheets[i].RawHeaders = rawCols
				res.Sheets[i].Headers = rawCols
			}
		}
		// Dự phòng: vá các ô trống không có tên tiêu đề thành "Cột <Chữ cái>" để đảm bảo chỉ số mảng luôn trùng khớp 100% với Cột Google Sheet
		if len(res.Sheets[i].Headers) > 0 {
			for j := range res.Sheets[i].Headers {
				if strings.TrimSpace(res.Sheets[i].Headers[j]) == "" {
					res.Sheets[i].Headers[j] = fmt.Sprintf("Cột %s", colLetter(j+1))
				}
			}
		}
	}

	return res.Sheets, nil
}

// colLetter chuyển chỉ số cột (1-based) thành chữ cái tên cột Excel (1 -> A, 14 -> N, 15 -> O, 16 -> P...)
func colLetter(col int) string {
	result := ""
	for col > 0 {
		col--
		result = string(rune('A'+(col%26))) + result
		col /= 26
	}
	return result
}

// fetchRawCsvHeaders tải trực tiếp dòng 1 qua CSV API của Google Sheet để lấy danh sách cột tuyệt đối
func (s *Service) fetchRawCsvHeaders(ctx context.Context, spreadsheetID string, gid string) []string {
	if spreadsheetID == "" {
		return nil
	}
	if gid == "" {
		gid = "0"
	}
	csvURL := fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s/gviz/tq?tqx=out:csv&gid=%s", spreadsheetID, gid)
	req, err := http.NewRequestWithContext(ctx, "GET", csvURL, nil)
	if err != nil {
		return nil
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	// Sheet riêng tư: endpoint gviz/tq công khai trả trang HTML đăng nhập (thường vẫn
	// HTTP 200 sau redirect). KHÔNG được parse trang đó thành cột — nếu không sẽ ghi đè
	// Headers đúng (từ Apps Script) bằng rác ["<!DOCTYPE html.."]. Phát hiện HTML → bỏ,
	// để nhánh gọi giữ nguyên Headers chuẩn của Apps Script.
	bodyLower := strings.ToLower(strings.TrimSpace(string(body)))
	if strings.HasPrefix(bodyLower, "<!doctype html") || strings.HasPrefix(bodyLower, "<html") ||
		strings.Contains(bodyLower, "<head>") || strings.Contains(bodyLower, "accounts.google.com") {
		return nil
	}
	lines := strings.Split(string(body), "\n")
	if len(lines) == 0 {
		return nil
	}
	// Dùng csv.Reader để parse dòng 1 chuẩn xác theo chuẩn CSV
	r := csv.NewReader(strings.NewReader(lines[0]))
	r.LazyQuotes = true
	record, err := r.Read()
	if err != nil || len(record) == 0 {
		return nil
	}
	rawCols := make([]string, len(record))
	for i, col := range record {
		val := strings.TrimSpace(col)
		if val == "" {
			val = fmt.Sprintf("Cột %s", colLetter(i+1))
		}
		rawCols[i] = val
	}
	return rawCols
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

	// Sheet riêng tư: endpoint public trả trang login HTML (thường vẫn 200 sau redirect).
	// Không được parse HTML thành cột — trả lỗi để caller giữ header đúng từ Apps Script.
	bodyLower := strings.ToLower(strings.TrimSpace(string(body)))
	if strings.HasPrefix(bodyLower, "<!doctype html") || strings.HasPrefix(bodyLower, "<html") {
		return nil, "", fmt.Errorf("Sheet chưa bật chia sẻ công khai (CSV trả về trang đăng nhập)")
	}

	lines := strings.Split(string(body), "\n")
	if len(lines) == 0 {
		return nil, "", fmt.Errorf("file rỗng")
	}

	firstLine := lines[0]
	// Parse chuẩn CSV (không tách thô trên dấu phẩy — vỡ với tên cột chứa "," trong ngoặc kép).
	cleanCols := []string{}
	if rec, rerr := csv.NewReader(strings.NewReader(firstLine)).Read(); rerr == nil {
		for _, c := range rec {
			val := strings.TrimSpace(c)
			if val != "" {
				cleanCols = append(cleanCols, val)
			}
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
	Headers []string   `json:"headers,omitempty"` // Danh sách tên cột để Apps Script map theo Tên Cột chính xác 100%
}

// WebAppResponse phản hồi từ Google Apps Script Web App
type WebAppResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Count   int    `json:"count,omitempty"`
}

// PushRowToWebApp gửi 1 dòng dữ liệu lên Apps Script Web App URL
func (s *Service) PushRowToWebApp(ctx context.Context, webAppURL string, gid string, tabName string, row []string, headers []string) error {
	payload := WebAppPayload{
		Action:  "append_row",
		Gid:     gid,
		TabName: tabName,
		Row:     row,
		Headers: headers,
	}

	return s.sendRequest(ctx, webAppURL, payload)
}

// PushBatchToWebApp gửi danh sách nhiều dòng dữ liệu lên Apps Script Web App URL
func (s *Service) PushBatchToWebApp(ctx context.Context, webAppURL string, gid string, tabName string, rows [][]string, headers []string) error {
	payload := WebAppPayload{
		Action:  "append_batch",
		Gid:     gid,
		TabName: tabName,
		Rows:    rows,
		Headers: headers,
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

	if u, uerr := url.Parse(webAppURL); uerr == nil {
		if apiKey := u.Query().Get("key"); apiKey != "" {
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Authorization", "Bearer "+apiKey)
		} else if token := u.Query().Get("token"); token != "" {
			req.Header.Set("X-API-Key", token)
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

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

    // Đọc danh sách header ở Dòng 1 để map chính xác Cột theo Tên Cột.
    // colMap lưu HÀNG ĐỢI index cho mỗi tên (không chỉ index cuối) để xử lý đúng
    // trường hợp 2+ cột TRÙNG TÊN: mỗi lần gặp tên đó khi ghi sẽ tiêu thụ index kế
    // tiếp theo thứ tự trái→phải, không dồn hết vào 1 ô.
    var lastCol = Math.max(sheet.getLastColumn(), 1);
    var headerRow = sheet.getRange(1, 1, 1, lastCol).getValues()[0];
    var colMap = {};
    for (var c = 0; c < headerRow.length; c++) {
      var hName = headerRow[c] ? headerRow[c].toString().trim().toLowerCase() : "";
      if (hName) {
        if (!colMap[hName]) colMap[hName] = [];
        colMap[hName].push(c + 1);
      }
    }

    function processSingleRow(rowItem, rowHeaders) {
      var nextRow = sheet.getLastRow() + 1;
      var maxIdx = Math.max(lastCol, rowItem.length);
      var padded = new Array(maxIdx);
      for (var i = 0; i < maxIdx; i++) padded[i] = "";

      if (rowHeaders && rowHeaders.length === rowItem.length) {
        // Con trỏ tiêu thụ riêng cho mỗi tên cột (xử lý tên trùng theo thứ tự).
        var cursor = {};
        var mappedCount = 0;
        for (var h = 0; h < rowHeaders.length; h++) {
          var hName = rowHeaders[h] ? rowHeaders[h].toString().trim().toLowerCase() : "";
          var idxList = colMap[hName];
          if (idxList && idxList.length > 0) {
            var pos = cursor[hName] || 0;
            // Nếu số lần gặp tên vượt số cột cùng tên, dùng lại cột cuối (an toàn).
            var cIdx = pos < idxList.length ? idxList[pos] : idxList[idxList.length - 1];
            cursor[hName] = pos + 1;
            padded[cIdx - 1] = rowItem[h];
            mappedCount++;
          }
        }
        // Nếu không map được theo tên cột nào, dán thẳng theo vị trí index mảng (1-to-1)
        if (mappedCount === 0) {
          for (var k = 0; k < rowItem.length; k++) {
            padded[k] = rowItem[k];
          }
        }
      } else {
        for (var k = 0; k < rowItem.length; k++) {
          padded[k] = rowItem[k];
        }
      }
      sheet.getRange(nextRow, 1, 1, padded.length).setValues([padded]);
    }

    if (data.action === "append_batch" && data.rows && data.rows.length > 0) {
      for (var r = 0; r < data.rows.length; r++) {
        processSingleRow(data.rows[r], data.headers);
      }
    } else if (data.row && data.row.length > 0) {
      processSingleRow(data.row, data.headers);
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
      var rawHeaders = [];
      if (lastCol > 0) {
        var row1 = s.getRange(1, 1, 1, lastCol).getValues()[0];
        for (var c = 0; c < row1.length; c++) {
          var name = row1[c] ? row1[c].toString().trim() : "";
          var colName = name !== "" ? name : ("Cột " + getColLetter(c + 1));
          headers.push(colName);
          rawHeaders.push(colName);
        }
      }
      result.push({
        name: s.getName(),
        gid: s.getSheetId().toString(),
        headers: headers,
        rawHeaders: rawHeaders
      });
    }
    return ContentService.createTextOutput(JSON.stringify({ status: "success", sheets: result }))
      .setMimeType(ContentService.MimeType.JSON);
  } catch (err) {
    return ContentService.createTextOutput(JSON.stringify({ status: "error", message: err.toString() }))
      .setMimeType(ContentService.MimeType.JSON);
  }
}

function getColLetter(col) {
  var temp, letter = '';
  while (col > 0) {
    temp = (col - 1) % 26;
    letter = String.fromCharCode(65 + temp) + letter;
    col = (col - temp - 1) / 26;
  }
  return letter;
}`
}
