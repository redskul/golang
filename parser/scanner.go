package parser

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileKind classifies a trace file found on disk
type FileKind int

const (
	KindUnknown FileKind = iota
	KindCrashDump
	KindOOM
	KindLog
)

// ScanTraceDir walks a HANA trace directory, classifies every file, parses
// it with the appropriate parser, and returns a real DiagnosticReport built
// entirely from on-disk evidence (no synthetic data).
func ScanTraceDir(dir string) (*DiagnosticReport, error) {
	report := &DiagnosticReport{
		GeneratedAt: nowFunc(),
	}

	host, sid := inferHostSIDFromDir(dir)
	report.Host = host
	report.SID = sid

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip unreadable files/dirs rather than aborting the whole scan
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !isCandidateFile(info.Name()) {
			return nil
		}
		if info.Size() > 200*1024*1024 {
			// Skip absurdly large files (e.g. full core dumps) — only text trace files are parsed
			return nil
		}

		f, ferr := os.Open(path)
		if ferr != nil {
			return nil
		}
		defer f.Close()

		head := make([]byte, 64*1024)
		n, _ := f.Read(head)
		head = head[:n]
		if _, rerr := f.Seek(0, io.SeekStart); rerr != nil {
			return nil
		}

		kind := classifyFile(info.Name(), head)
		rel, _ := filepath.Rel(dir, path)
		if rel == "" {
			rel = path
		}

		switch kind {
		case KindCrashDump:
			cd, perr := ParseCrashDump(f, rel)
			if perr == nil {
				if cd.Host == "" {
					cd.Host = host
				}
				if cd.SID == "" {
					cd.SID = sid
				}
				report.CrashDumps = append(report.CrashDumps, *cd)
			}
		case KindOOM:
			oom, perr := ParseOOMEvent(f, rel)
			if perr == nil {
				if oom.Host == "" {
					oom.Host = host
				}
				if oom.SID == "" {
					oom.SID = sid
				}
				report.OOMEvents = append(report.OOMEvents, *oom)
			}
		case KindLog:
			lf, perr := ParseLogFile(f, rel)
			if perr == nil && len(lf.Entries) > 0 {
				if lf.Host == "" {
					lf.Host = host
				}
				report.LogFiles = append(report.LogFiles, *lf)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	report.TopIssues = DeriveIssues(report)
	report.HealthScore = ComputeHealthScore(report)
	return report, nil
}

// isCandidateFile filters out files that are obviously not HANA trace/dump files
func isCandidateFile(name string) bool {
	lower := strings.ToLower(name)
	switch filepath.Ext(lower) {
	case ".trc", ".log", ".txt", ".dmp":
		return true
	}
	// HANA crash/OOM dumps are sometimes extension-less, e.g. "indexserver.30003.crashdump.20240101"
	if strings.Contains(lower, "crashdump") || strings.Contains(lower, "core.") || strings.Contains(lower, ".oom.") {
		return true
	}
	return false
}

// classifyFile decides whether a file is a crash dump, OOM dump, or plain log
// based on filename conventions first, then content sniffing.
func classifyFile(name string, head []byte) FileKind {
	lower := strings.ToLower(name)
	content := strings.ToLower(string(head))

	switch {
	case strings.Contains(lower, "crashdump"), strings.Contains(lower, "core."), strings.Contains(lower, ".dmp"):
		return KindCrashDump
	case strings.Contains(lower, "oom") || strings.Contains(lower, "out_of_memory"):
		return KindOOM
	}

	// Content-based sniffing for ambiguous filenames (e.g. generic .trc)
	switch {
	case containsAny(content, "received signal", "backtrace", "stack dump", "thread") && containsAny(content, "sigsegv", "sigabrt", "signal 11", "signal 6", "crashed"):
		return KindCrashDump
	case containsAny(content, "global_allocation_limit", "bad_alloc", "out of memory", "oom") && containsAny(content, "allocat", "memory"):
		return KindOOM
	case len(bytes.TrimSpace(head)) > 0:
		return KindLog
	}
	return KindUnknown
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// inferHostSIDFromDir tries to extract host/SID from a conventional HANA
// trace path like /hana/shared/<SID>/HDB<NN>/trace or /usr/sap/<SID>/HDB<NN>/<host>
func inferHostSIDFromDir(dir string) (host, sid string) {
	host, _ = os.Hostname()
	parts := strings.Split(filepath.ToSlash(dir), "/")
	for _, p := range parts {
		if len(p) == 3 && p == strings.ToUpper(p) && isAlnum(p) {
			sid = p
			break
		}
	}
	if sid == "" {
		sid = "HDB"
	}
	return host, sid
}

func isAlnum(s string) bool {
	for _, c := range s {
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return len(s) > 0
}
