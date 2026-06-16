package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hana-viewer/parser"
)

// Server holds all HTTP handler state
type Server struct {
	tmpl     *template.Template
	mu       sync.RWMutex
	report   *parser.DiagnosticReport
	traceDir string // real HANA trace directory; empty means demo mode
	lastScan time.Time
	scanErr  error
}

// NewServer creates a new web server with embedded templates.
// If traceDir is non-empty, it is scanned immediately for real HANA crash
// dumps, OOM events, and log files. If traceDir is empty, the server starts
// in demo mode with synthetic sample data.
func NewServer(traceDir string) (*Server, error) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseGlob("templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	s := &Server{
		tmpl:     tmpl,
		traceDir: traceDir,
	}
	if traceDir == "" {
		s.report = parser.GenerateSampleData()
	} else if err := s.rescan(); err != nil {
		return nil, fmt.Errorf("initial scan of %s: %w", traceDir, err)
	}
	return s, nil
}

// rescan re-walks the configured trace directory and rebuilds the report
// from real on-disk files. No-op (returns nil) in demo mode.
func (s *Server) rescan() error {
	if s.traceDir == "" {
		return nil
	}
	report, err := parser.ScanTraceDir(s.traceDir)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastScan = time.Now()
	if err != nil {
		s.scanErr = err
		return err
	}
	s.scanErr = nil
	s.report = report
	return nil
}

func (s *Server) currentReport() *parser.DiagnosticReport {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.report
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/report", s.handleAPIReport)
	mux.HandleFunc("/api/upload", s.handleUpload)
	mux.HandleFunc("/crash/", s.handleCrashDetail)
	mux.HandleFunc("/oom/", s.handleOOMDetail)
	mux.HandleFunc("/logs", s.handleLogs)
	mux.HandleFunc("/api/rescan", s.handleRescan)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}

// handleIndex renders the main dashboard
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data := map[string]interface{}{
		"Report":      s.currentReport(),
		"PageTitle":   "SAP HANA Diagnostic Viewer",
		"CurrentPage": "dashboard",
		"TraceDir":    s.traceDir,
		"ScanErr":     s.scanErr,
	}
	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// handleCrashDetail shows a single crash dump in detail
func (s *Server) handleCrashDetail(w http.ResponseWriter, r *http.Request) {
	report := s.currentReport()
	idx := parseIDFromPath(r.URL.Path, "/crash/")
	if idx < 0 || idx >= len(report.CrashDumps) {
		http.NotFound(w, r)
		return
	}
	data := map[string]interface{}{
		"Crash":       report.CrashDumps[idx],
		"Index":       idx,
		"PageTitle":   "Crash Dump Analysis",
		"CurrentPage": "crash",
	}
	if err := s.tmpl.ExecuteTemplate(w, "crash.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// handleOOMDetail shows a single OOM event in detail
func (s *Server) handleOOMDetail(w http.ResponseWriter, r *http.Request) {
	report := s.currentReport()
	idx := parseIDFromPath(r.URL.Path, "/oom/")
	if idx < 0 || idx >= len(report.OOMEvents) {
		http.NotFound(w, r)
		return
	}
	data := map[string]interface{}{
		"OOM":         report.OOMEvents[idx],
		"Index":       idx,
		"PageTitle":   "OOM Event Analysis",
		"CurrentPage": "oom",
	}
	if err := s.tmpl.ExecuteTemplate(w, "oom.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// handleLogs shows the log viewer
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	severity := r.URL.Query().Get("severity")
	search := r.URL.Query().Get("q")

	report := s.currentReport()
	var entries []parser.LogEntry
	for _, lf := range report.LogFiles {
		for _, e := range lf.Entries {
			if severity != "" && !strings.EqualFold(string(e.Severity), severity) {
				continue
			}
			if search != "" && !strings.Contains(strings.ToLower(e.Message), strings.ToLower(search)) {
				continue
			}
			entries = append(entries, e)
		}
	}

	data := map[string]interface{}{
		"LogFiles":    report.LogFiles,
		"Entries":     entries,
		"Severity":    severity,
		"Search":      search,
		"PageTitle":   "Log Viewer",
		"CurrentPage": "logs",
	}
	if err := s.tmpl.ExecuteTemplate(w, "logs.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

// handleAPIReport returns the full diagnostic report as JSON
func (s *Server) handleAPIReport(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.currentReport())
}

// handleRescan re-walks the configured trace directory and redirects back
// to the dashboard. No-op in demo mode.
func (s *Server) handleRescan(w http.ResponseWriter, r *http.Request) {
	if err := s.rescan(); err != nil {
		http.Error(w, "rescan failed: "+err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleUpload accepts file uploads for parsing
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, "form parse error: "+err.Error(), 400)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file: "+err.Error(), 400)
		return
	}
	defer file.Close()

	name := header.Filename
	ext := strings.ToLower(filepath.Ext(name))
	body, err := io.ReadAll(io.LimitReader(file, 64<<20))
	if err != nil {
		http.Error(w, "read error: "+err.Error(), 400)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case strings.Contains(strings.ToLower(name), "oom") || strings.Contains(strings.ToLower(name), "out_of_memory"):
		ev, err := parser.ParseOOMEvent(strings.NewReader(string(body)), name)
		if err != nil {
			http.Error(w, "OOM parse error: "+err.Error(), 400)
			return
		}
		s.report.OOMEvents = append(s.report.OOMEvents, *ev)

	case strings.Contains(strings.ToLower(name), "crash") || ext == ".dmp":
		cd, err := parser.ParseCrashDump(strings.NewReader(string(body)), name)
		if err != nil {
			http.Error(w, "crash dump parse error: "+err.Error(), 400)
			return
		}
		s.report.CrashDumps = append(s.report.CrashDumps, *cd)

	default:
		lf, err := parser.ParseLogFile(strings.NewReader(string(body)), name)
		if err != nil {
			http.Error(w, "log parse error: "+err.Error(), 400)
			return
		}
		s.report.LogFiles = append(s.report.LogFiles, *lf)
	}
	s.report.TopIssues = parser.DeriveIssues(s.report)
	s.report.HealthScore = parser.ComputeHealthScore(s.report)

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// parseIDFromPath extracts a zero-based integer index from a URL path suffix
func parseIDFromPath(path, prefix string) int {
	s := strings.TrimPrefix(path, prefix)
	s = strings.TrimRight(s, "/")
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			return -1
		}
	}
	return n
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"formatBytes": func(b int64) string {
			const (
				KB = 1024
				MB = 1024 * KB
				GB = 1024 * MB
			)
			switch {
			case b >= GB:
				return fmt.Sprintf("%.2f GB", float64(b)/GB)
			case b >= MB:
				return fmt.Sprintf("%.2f MB", float64(b)/MB)
			case b >= KB:
				return fmt.Sprintf("%.2f KB", float64(b)/KB)
			default:
				return fmt.Sprintf("%d B", b)
			}
		},
		"severityClass": func(s parser.Severity) string {
			switch s {
			case parser.SeverityFatal:
				return "badge-fatal"
			case parser.SeverityError:
				return "badge-error"
			case parser.SeverityWarning:
				return "badge-warning"
			case parser.SeverityInfo:
				return "badge-info"
			default:
				return "badge-debug"
			}
		},
		"severityIcon": func(s parser.Severity) string {
			switch s {
			case parser.SeverityFatal:
				return "💀"
			case parser.SeverityError:
				return "🔴"
			case parser.SeverityWarning:
				return "🟡"
			case parser.SeverityInfo:
				return "🔵"
			default:
				return "⚪"
			}
		},
		"add": func(a, b int) int { return a + b },
		"pct": func(used, total int64) float64 {
			if total == 0 {
				return 0
			}
			return float64(used) / float64(total) * 100
		},
		"toJSON": func(v interface{}) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"inc": func(i int) int { return i + 1 },
		"healthColor": func(score int) string {
			switch {
			case score >= 80:
				return "#22c55e"
			case score >= 60:
				return "#f59e0b"
			case score >= 40:
				return "#f97316"
			default:
				return "#ef4444"
			}
		},
		"healthLabel": func(score int) string {
			switch {
			case score >= 80:
				return "HEALTHY"
			case score >= 60:
				return "DEGRADED"
			case score >= 40:
				return "CRITICAL"
			default:
				return "FAILING"
			}
		},
		"slice": func(s []parser.MemoryDataPoint, start, end int) []parser.MemoryDataPoint {
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
	}
}
