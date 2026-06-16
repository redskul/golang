package parser

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"time"
)

var (
	// HANA trace log format: [timestamp] [severity] [component] [thread] message
	reHANALog = regexp.MustCompile(
		`^\[(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?)\]\s+` +
			`([A-Z]+)\s+` +
			`(\S+)\s+` +
			`(\S+)\s+(.+)$`,
	)
	// Alternate format without thread
	reHANALog2 = regexp.MustCompile(
		`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+([A-Z]+)\s+(\S+)\s+(.+)$`,
	)
	// Alert format
	reAlertLog = regexp.MustCompile(
		`^(\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?)\s+\[([A-Z]+)\]\s+(.+)$`,
	)

	reErrorCode = regexp.MustCompile(`\b(error|err)\s*(?:code|no\.?|number)?\s*[=:]?\s*(\d+)\b`)
)

// errorExplanations maps known HANA error patterns to explanations
var errorExplanations = map[string]string{
	"bad_alloc":                   "C++ bad_alloc exception: HANA failed to allocate memory. System may be running low on RAM.",
	"out of memory":               "System out of memory: no heap space available for requested allocation.",
	"connection refused":          "Client or service connection rejected. Check that target service is running and port is accessible.",
	"deadlock":                    "Transaction deadlock detected between two or more sessions. One transaction was rolled back as victim.",
	"lock timeout":                "A transaction waited too long for a lock. Increase lock_wait_timeout or review transaction patterns.",
	"log full":                    "Redo log is full. Backup log segment immediately and investigate log growth.",
	"disk full":                   "Disk volume is full. Free space on data/log volume immediately.",
	"license":                     "License validation issue. Check that HANA license is valid and within limits.",
	"permission denied":           "OS-level permission denied. Check file system permissions for <sid>adm user.",
	"trace full":                  "Trace directory is full. Clean old trace files and increase trace disk limit.",
	"savepoint":                   "Savepoint operation (checkpoint to data volume). Long savepoints may indicate I/O bottleneck.",
	"backup":                      "Backup-related event. Monitor backup completion and check backup catalog.",
	"replication":                 "System replication event. Check HSR lag and network between primary/secondary.",
	"assertion":                   "Internal HANA assertion failed. This indicates a bug or data inconsistency. Report to SAP Support.",
	"stack overflow":              "Call stack overflow. Possible infinite recursion in SQL or HANA internal code.",
	"service restart":             "HANA service was restarted. Check preceding errors for the root cause.",
	"crash":                       "Service crashed. Review crash dump in trace directory.",
	"failover":                    "HA failover triggered. Review why primary became unavailable.",
	"cpu":                         "High CPU utilization detected. Review long-running queries and parallel execution.",
	"slow query":                  "Long-running query detected. Check M_EXPENSIVE_STATEMENTS and optimize the SQL.",
	"index":                       "Index operation (create/drop/update). May cause temporary performance impact.",
	"compaction":                  "Delta merge or compaction running. Normal operation but may use CPU/memory.",
	"network":                     "Network communication error. Check connectivity between HANA nodes/services.",
	"authentication":              "Authentication failure. Verify credentials and user account status.",
	"sql error":                   "SQL execution error. Review SQL statement and parameters.",
	"internal error":              "HANA internal error. Collect diagnostics and contact SAP Support.",
	"watchdog":                    "Watchdog detected unresponsive service. Service may be restarted.",
	"preemptive oom":              "Preventive OOM kill: HANA proactively terminated a process to avoid system OOM.",
}

// ParseLogFile parses a HANA trace/log file
func ParseLogFile(r io.Reader, filename string) (*LogFile, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	lf := &LogFile{
		Filename: filename,
		ParsedAt: time.Now(),
	}
	lf.ServiceName = inferServiceFromFilename(filename)

	buckets := make(map[time.Time]*LogTimelineBucket)
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++
		if strings.TrimSpace(line) == "" {
			continue
		}

		entry := parseLogLine(line, lineNum)
		if entry == nil {
			continue
		}
		entry.TraceFile = filename

		// Classify
		switch entry.Severity {
		case SeverityError, SeverityFatal:
			lf.ErrorCount++
			entry.IsError = true
		case SeverityWarning:
			lf.WarnCount++
		}

		// Add explanation
		entry.Explanation = explainLogMessage(entry.Message)

		// Extract error code
		if m := reErrorCode.FindStringSubmatch(strings.ToLower(entry.Message)); len(m) > 2 {
			entry.ErrorCode = m[2]
		}

		lf.Entries = append(lf.Entries, *entry)

		// Bucket by minute for timeline
		bucket := entry.Timestamp.Truncate(time.Minute)
		if _, ok := buckets[bucket]; !ok {
			buckets[bucket] = &LogTimelineBucket{Time: bucket}
		}
		switch entry.Severity {
		case SeverityError, SeverityFatal:
			buckets[bucket].Errors++
		case SeverityWarning:
			buckets[bucket].Warnings++
		default:
			buckets[bucket].Infos++
		}
	}

	// Flatten timeline buckets sorted by time
	for _, b := range buckets {
		lf.Timeline = append(lf.Timeline, *b)
	}
	sortTimeline(lf.Timeline)

	lf.Summary = buildLogSummary(lf)
	return lf, nil
}

func parseLogLine(line string, num int) *LogEntry {
	// Try primary format
	if m := reHANALog.FindStringSubmatch(line); len(m) > 5 {
		t := parseLogTimestamp(m[1])
		return &LogEntry{
			Timestamp: t,
			Severity:  parseSeverity(m[2]),
			Component: m[3],
			Thread:    m[4],
			Message:   strings.TrimSpace(m[5]),
			LineNum:   num,
		}
	}
	// Try alternate format
	if m := reHANALog2.FindStringSubmatch(line); len(m) > 4 {
		t := parseLogTimestamp(m[1])
		return &LogEntry{
			Timestamp: t,
			Severity:  parseSeverity(m[2]),
			Component: m[3],
			Message:   strings.TrimSpace(m[4]),
			LineNum:   num,
		}
	}
	// Try alert format
	if m := reAlertLog.FindStringSubmatch(line); len(m) > 3 {
		t := parseLogTimestamp(m[1])
		sev := parseSeverity(m[2])
		return &LogEntry{
			Timestamp: t,
			Severity:  sev,
			Component: "ALERT",
			Message:   strings.TrimSpace(m[3]),
			LineNum:   num,
		}
	}
	// Plain line — treat as INFO if not empty
	if len(strings.TrimSpace(line)) > 5 {
		return &LogEntry{
			Timestamp: time.Now(),
			Severity:  SeverityInfo,
			Component: "UNKNOWN",
			Message:   strings.TrimSpace(line),
			LineNum:   num,
		}
	}
	return nil
}

func parseLogTimestamp(s string) time.Time {
	layouts := []string{
		"2006-01-02 15:04:05.000000",
		"2006-01-02 15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}

func parseSeverity(s string) Severity {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "FATAL", "F":
		return SeverityFatal
	case "ERROR", "E", "ERR":
		return SeverityError
	case "WARNING", "WARN", "W":
		return SeverityWarning
	case "DEBUG", "D":
		return SeverityDebug
	default:
		return SeverityInfo
	}
}

func explainLogMessage(msg string) string {
	lower := strings.ToLower(msg)
	for pattern, explanation := range errorExplanations {
		if strings.Contains(lower, pattern) {
			return explanation
		}
	}
	return ""
}

func buildLogSummary(lf *LogFile) string {
	total := len(lf.Entries)
	if total == 0 {
		return "Empty log file"
	}
	return strings.Join([]string{
		"Log file: " + lf.Filename,
		"Service: " + lf.ServiceName,
		"Total entries: " + itoa(total),
		"Errors: " + itoa(lf.ErrorCount),
		"Warnings: " + itoa(lf.WarnCount),
	}, " | ")
}

func sortTimeline(tl []LogTimelineBucket) {
	// Simple insertion sort (timeline is usually nearly sorted)
	for i := 1; i < len(tl); i++ {
		for j := i; j > 0 && tl[j].Time.Before(tl[j-1].Time); j-- {
			tl[j], tl[j-1] = tl[j-1], tl[j]
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
