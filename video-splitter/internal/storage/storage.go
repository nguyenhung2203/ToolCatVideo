package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"video-splitter/internal/project"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (không cần CGO)
)

// Store bọc kết nối SQLite lưu project/clip/config.
// Dùng cách lưu clips + config dạng JSON blob trong cột thay vì chuẩn hóa nhiều bảng:
// đơn giản, đủ cho nhu cầu "mở lại không mất việc", tránh migration phức tạp khi
// EditOps còn tiến hóa qua các giai đoạn edit.
type Store struct {
	db *sql.DB
}

// Open mở (hoặc tạo) database tại dbPath và khởi tạo schema.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("tạo thư mục db: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("mở sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// DefaultDBPath trả về đường dẫn db mặc định trong thư mục dữ liệu app của user.
func DefaultDBPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "video-splitter", "projects.db")
}

func (s *Store) init() error {
	const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id          TEXT PRIMARY KEY,
	source_path TEXT NOT NULL,
	name        TEXT,
	duration    REAL,
	width       INTEGER,
	height      INTEGER,
	fps         REAL,
	status      TEXT,
	config_json TEXT,
	clips_json  TEXT,
	created_at  INTEGER,
	updated_at  INTEGER
);
CREATE INDEX IF NOT EXISTS idx_projects_source ON projects(source_path);
CREATE INDEX IF NOT EXISTS idx_projects_updated ON projects(updated_at DESC);

CREATE TABLE IF NOT EXISTS export_history (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id  TEXT,
	out_dir     TEXT,
	clip_count  INTEGER,
	ok_count    INTEGER,
	created_at  INTEGER
);
CREATE INDEX IF NOT EXISTS idx_export_project ON export_history(project_id);
`
	_, err := s.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("khởi tạo schema: %w", err)
	}
	return nil
}

// Close đóng kết nối db.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SaveProject upsert một project (kèm clips + config đóng gói JSON).
func (s *Store) SaveProject(p *project.Project) error {
	if p.ID == "" {
		return fmt.Errorf("project thiếu ID")
	}
	now := time.Now().Unix()
	if p.CreatedAt == 0 {
		p.CreatedAt = now
	}
	p.UpdatedAt = now

	configJSON, err := json.Marshal(p.Config)
	if err != nil {
		return fmt.Errorf("mã hóa config: %w", err)
	}
	clipsJSON, err := json.Marshal(p.Clips)
	if err != nil {
		return fmt.Errorf("mã hóa clips: %w", err)
	}

	const q = `
INSERT INTO projects (id, source_path, name, duration, width, height, fps, status, config_json, clips_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	source_path=excluded.source_path,
	name=excluded.name,
	duration=excluded.duration,
	width=excluded.width,
	height=excluded.height,
	fps=excluded.fps,
	status=excluded.status,
	config_json=excluded.config_json,
	clips_json=excluded.clips_json,
	updated_at=excluded.updated_at;
`
	_, err = s.db.Exec(q, p.ID, p.SourcePath, p.Name, p.Duration, p.Width, p.Height, p.FPS,
		p.Status, string(configJSON), string(clipsJSON), p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("lưu project: %w", err)
	}
	return nil
}

// GetProject đọc một project theo ID. Trả về (nil, nil) nếu không tồn tại.
func (s *Store) GetProject(id string) (*project.Project, error) {
	const q = `SELECT id, source_path, name, duration, width, height, fps, status, config_json, clips_json, created_at, updated_at FROM projects WHERE id = ?`
	row := s.db.QueryRow(q, id)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// FindProjectBySource tìm project theo đường dẫn video nguồn (mới nhất). (nil, nil) nếu không có.
func (s *Store) FindProjectBySource(sourcePath string) (*project.Project, error) {
	const q = `SELECT id, source_path, name, duration, width, height, fps, status, config_json, clips_json, created_at, updated_at FROM projects WHERE source_path = ? ORDER BY updated_at DESC LIMIT 1`
	row := s.db.QueryRow(q, sourcePath)
	p, err := scanProject(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return p, err
}

// ProjectSummary là bản tóm tắt project cho danh sách (không kèm clips).
type ProjectSummary struct {
	ID         string  `json:"id"`
	SourcePath string  `json:"sourcePath"`
	Name       string  `json:"name"`
	Duration   float64 `json:"duration"`
	Status     string  `json:"status"`
	ClipCount  int     `json:"clipCount"`
	UpdatedAt  int64   `json:"updatedAt"`
}

// ListProjects trả về danh sách tóm tắt các project, mới cập nhật trước.
func (s *Store) ListProjects() ([]ProjectSummary, error) {
	const q = `SELECT id, source_path, name, duration, status, clips_json, updated_at FROM projects ORDER BY updated_at DESC`
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("liệt kê project: %w", err)
	}
	defer rows.Close()

	var out []ProjectSummary
	for rows.Next() {
		var sm ProjectSummary
		var clipsJSON string
		if err := rows.Scan(&sm.ID, &sm.SourcePath, &sm.Name, &sm.Duration, &sm.Status, &clipsJSON, &sm.UpdatedAt); err != nil {
			return nil, err
		}
		var clips []project.Clip
		if clipsJSON != "" {
			_ = json.Unmarshal([]byte(clipsJSON), &clips)
		}
		sm.ClipCount = len(clips)
		out = append(out, sm)
	}
	return out, rows.Err()
}

// DeleteProject xóa một project theo ID.
func (s *Store) DeleteProject(id string) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

// AddExportRecord ghi một dòng lịch sử xuất video.
func (s *Store) AddExportRecord(projectID, outDir string, clipCount, okCount int) error {
	_, err := s.db.Exec(
		`INSERT INTO export_history (project_id, out_dir, clip_count, ok_count, created_at) VALUES (?, ?, ?, ?, ?)`,
		projectID, outDir, clipCount, okCount, time.Now().Unix())
	return err
}

// rowScanner khớp cả *sql.Row và *sql.Rows để dùng chung logic scan.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProject(row rowScanner) (*project.Project, error) {
	var p project.Project
	var configJSON, clipsJSON string
	err := row.Scan(&p.ID, &p.SourcePath, &p.Name, &p.Duration, &p.Width, &p.Height, &p.FPS,
		&p.Status, &configJSON, &clipsJSON, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if configJSON != "" {
		_ = json.Unmarshal([]byte(configJSON), &p.Config)
	}
	if clipsJSON != "" {
		_ = json.Unmarshal([]byte(clipsJSON), &p.Clips)
	}
	return &p, nil
}
