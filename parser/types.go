package parser

import "time"

// Severity levels for log entries
type Severity string

const (
	SeverityFatal   Severity = "FATAL"
	SeverityError   Severity = "ERROR"
	SeverityWarning Severity = "WARNING"
	SeverityInfo    Severity = "INFO"
	SeverityDebug   Severity = "DEBUG"
)

// CrashDump represents a parsed SAP HANA crash dump
type CrashDump struct {
	Timestamp   time.Time      `json:"timestamp"`
	ServiceName string         `json:"service_name"`
	ProcessID   int            `json:"process_id"`
	Signal      string         `json:"signal"`
	SignalNum   int            `json:"signal_num"`
	ExitCode    int            `json:"exit_code"`
	CrashReason string         `json:"crash_reason"`
	RootCause   string         `json:"root_cause"`
	StackTrace  []StackFrame   `json:"stack_trace"`
	Registers   map[string]string `json:"registers"`
	MemoryMap   []MemoryRegion `json:"memory_map"`
	Threads     []ThreadInfo   `json:"threads"`
	Summary     string         `json:"summary"`
	Severity    Severity       `json:"severity"`
	Host        string         `json:"host"`
	SID         string         `json:"sid"`
}

// StackFrame represents a single frame in the call stack
type StackFrame struct {
	FrameNum   int    `json:"frame_num"`
	Address    string `json:"address"`
	Function   string `json:"function"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Library    string `json:"library"`
	Offset     string `json:"offset"`
	IsHANACode bool   `json:"is_hana_code"`
	Description string `json:"description"`
}

// MemoryRegion represents a region in the process memory map
type MemoryRegion struct {
	StartAddr string `json:"start_addr"`
	EndAddr   string `json:"end_addr"`
	Size      int64  `json:"size"`
	Perms     string `json:"perms"`
	Name      string `json:"name"`
}

// ThreadInfo represents a thread from the crash dump
type ThreadInfo struct {
	ThreadID  int          `json:"thread_id"`
	Name      string       `json:"name"`
	State     string       `json:"state"`
	IsCrash   bool         `json:"is_crash"`
	Stack     []StackFrame `json:"stack"`
}

// OOMEvent represents an Out-of-Memory event
type OOMEvent struct {
	Timestamp     time.Time          `json:"timestamp"`
	Host          string             `json:"host"`
	SID           string             `json:"sid"`
	ServiceName   string             `json:"service_name"`
	ProcessID     int                `json:"process_id"`
	TotalMemory   int64              `json:"total_memory_bytes"`
	UsedMemory    int64              `json:"used_memory_bytes"`
	FreeMemory    int64              `json:"free_memory_bytes"`
	RequestedSize int64              `json:"requested_size_bytes"`
	MemoryLimit   int64              `json:"memory_limit_bytes"`
	Allocator     string             `json:"allocator"`
	Consumers     []MemoryConsumer   `json:"top_consumers"`
	Timeline      []MemoryDataPoint  `json:"memory_timeline"`
	RootCause     string             `json:"root_cause"`
	Recommendation string            `json:"recommendation"`
	KilledProcess string             `json:"killed_process"`
	OOMType       string             `json:"oom_type"`
	StackTrace    []StackFrame       `json:"stack_trace"`
	PoolStats     []MemoryPoolStat   `json:"pool_stats"`
}

// MemoryConsumer represents a top memory consumer at OOM time
type MemoryConsumer struct {
	Name       string  `json:"name"`
	UsedBytes  int64   `json:"used_bytes"`
	PeakBytes  int64   `json:"peak_bytes"`
	Percentage float64 `json:"percentage"`
	Category   string  `json:"category"`
}

// MemoryDataPoint is a single point in memory usage timeline
type MemoryDataPoint struct {
	Timestamp time.Time `json:"timestamp"`
	UsedMB    float64   `json:"used_mb"`
	FreeMB    float64   `json:"free_mb"`
	LimitMB   float64   `json:"limit_mb"`
}

// MemoryPoolStat tracks stats for a memory pool
type MemoryPoolStat struct {
	PoolName    string  `json:"pool_name"`
	UsedBytes   int64   `json:"used_bytes"`
	AllocBytes  int64   `json:"alloc_bytes"`
	FreeBytes   int64   `json:"free_bytes"`
	NumAllocs   int64   `json:"num_allocs"`
	Fragmented  float64 `json:"fragmentation_pct"`
}

// LogEntry represents a single HANA log line
type LogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Severity    Severity  `json:"severity"`
	Component   string    `json:"component"`
	Thread      string    `json:"thread"`
	TraceFile   string    `json:"trace_file"`
	Message     string    `json:"message"`
	Explanation string    `json:"explanation"`
	IsError     bool      `json:"is_error"`
	ErrorCode   string    `json:"error_code"`
	LineNum     int       `json:"line_num"`
}

// LogFile represents a parsed HANA trace/log file
type LogFile struct {
	Filename   string     `json:"filename"`
	ServiceName string    `json:"service_name"`
	Host       string     `json:"host"`
	ParsedAt   time.Time  `json:"parsed_at"`
	Entries    []LogEntry `json:"entries"`
	ErrorCount int        `json:"error_count"`
	WarnCount  int        `json:"warn_count"`
	Summary    string     `json:"summary"`
	Timeline   []LogTimelineBucket `json:"timeline"`
}

// LogTimelineBucket groups log entries by time bucket
type LogTimelineBucket struct {
	Time     time.Time `json:"time"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Infos    int       `json:"infos"`
}

// DiagnosticReport is the top-level aggregate report
type DiagnosticReport struct {
	GeneratedAt  time.Time   `json:"generated_at"`
	Host         string      `json:"host"`
	SID          string      `json:"sid"`
	CrashDumps   []CrashDump `json:"crash_dumps"`
	OOMEvents    []OOMEvent  `json:"oom_events"`
	LogFiles     []LogFile   `json:"log_files"`
	HealthScore  int         `json:"health_score"`
	TopIssues    []Issue     `json:"top_issues"`
}

// Issue represents a diagnosed problem
type Issue struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Severity    Severity `json:"severity"`
	Description string   `json:"description"`
	RootCause   string   `json:"root_cause"`
	Impact      string   `json:"impact"`
	Resolution  []string `json:"resolution_steps"`
	References  []string `json:"references"`
	Timestamp   time.Time `json:"timestamp"`
	Source      string   `json:"source"`
}
