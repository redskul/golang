package parser

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reCrashTimestamp = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`)
	reSignal         = regexp.MustCompile(`[Ss]ignal\s+(\d+)\s*\((\w+)\)`)
	reFrame          = regexp.MustCompile(`#(\d+)\s+(0x[0-9a-fA-F]+)\s+in\s+(.+?)\s+(?:at\s+(.+):(\d+)|from\s+(.+))?`)
	reFrameSimple    = regexp.MustCompile(`#(\d+)\s+(0x[0-9a-fA-F]+)\s*(.*)`)
	rePID            = regexp.MustCompile(`[Pp]rocess\s+(\d+)`)
	reService        = regexp.MustCompile(`(?:service|process)[:\s]+([a-z_]+server|hdbindexserver|hdbnameserver|hdbcompileserver|hdbpreprocessor|hdbwebdispatcher|hdxdaemon)`)
	reRegister       = regexp.MustCompile(`([A-Z0-9]{2,4})\s*=\s*(0x[0-9a-fA-F]+|\d+)`)
	reSID            = regexp.MustCompile(`SID\s*[=:]\s*([A-Z0-9]{3})`)
	reHost           = regexp.MustCompile(`[Hh]ost(?:name)?\s*[=:]\s*([a-zA-Z0-9_\-\.]+)`)
)

// signalDescriptions maps signal numbers to human-readable descriptions
var signalDescriptions = map[int]string{
	1:  "SIGHUP - Hangup detected on controlling terminal",
	2:  "SIGINT - Interrupt from keyboard (Ctrl+C)",
	3:  "SIGQUIT - Quit from keyboard",
	4:  "SIGILL - Illegal Instruction (corrupt binary or CPU bug)",
	5:  "SIGTRAP - Trace/breakpoint trap",
	6:  "SIGABRT - Process called abort() - assertion failure or internal error",
	7:  "SIGBUS - Bus error (misaligned memory access)",
	8:  "SIGFPE - Floating point exception (divide by zero)",
	9:  "SIGKILL - Kill signal (sent by OS or admin, e.g. OOM killer)",
	11: "SIGSEGV - Segmentation fault (invalid memory access)",
	13: "SIGPIPE - Broken pipe (write to pipe with no readers)",
	14: "SIGALRM - Timer signal from alarm",
	15: "SIGTERM - Termination signal (graceful shutdown requested)",
	16: "SIGUSR1 - User-defined signal 1",
	17: "SIGUSR2 - User-defined signal 2",
	31: "SIGSYS - Bad system call",
}

// crashRootCauses maps signal+context to root cause explanations
var crashRootCauses = map[string]string{
	"SIGSEGV": "Null pointer dereference or illegal memory access. HANA may have attempted to read/write memory outside its allocated regions. Common causes: corrupted heap, use-after-free, buffer overflow in C++ layer.",
	"SIGABRT": "Process called std::abort() or assert() failed. This is an intentional crash triggered by HANA's internal consistency checks detecting a fatal data inconsistency.",
	"SIGKILL": "Process was forcefully terminated by the Linux kernel (OOM killer) or an administrator. Check if system ran out of memory.",
	"SIGBUS":  "Misaligned memory access or hardware fault. Possibly memory-mapped file corruption or faulty RAM.",
	"SIGFPE":  "Arithmetic error such as division by zero or integer overflow in HANA calculation engine.",
	"SIGILL":  "Illegal CPU instruction. Possible binary corruption or CPU compatibility mismatch.",
}

// knownHANAFunctions maps function prefixes to component explanations
var knownHANAFunctions = map[string]string{
	"TRexUtils":        "TRex Utility Layer (HANA row/column store utilities)",
	"TRexAlgebra":      "SQL Algebra Engine (query plan execution)",
	"Execution":        "Query Execution Engine",
	"AttributeEngine":  "Column Store Attribute Engine (compressed column I/O)",
	"JoinEvaluator":    "Join Execution Engine (hash/merge/nested-loop joins)",
	"ltt":              "Lock/Transaction Table (MVCC concurrency control)",
	"TrexStore":        "Column Store Manager",
	"persistence":      "Persistence Layer (log/data volume I/O)",
	"Backup":           "Backup & Recovery Engine",
	"Statistics":       "Statistics Server",
	"IndexServer":      "Index Server Main Process",
	"NameServer":       "Name Server (topology, catalog, routing)",
	"malloc":           "Memory Allocator (libc or HANA custom allocator)",
	"std::":            "C++ Standard Library",
	"boost::":          "Boost C++ Library",
}

// ParseCrashDump parses a HANA crash dump from a reader
func ParseCrashDump(r io.Reader, filename string) (*CrashDump, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	dump := &CrashDump{
		Registers: make(map[string]string),
		Severity:  SeverityFatal,
	}

	var inStack bool
	var inRegs bool
	var currentThread *ThreadInfo
	var allLines []string

	for scanner.Scan() {
		line := scanner.Text()
		allLines = append(allLines, line)

		// Parse timestamp
		if dump.Timestamp.IsZero() {
			if m := reCrashTimestamp.FindString(line); m != "" {
				if t, err := time.Parse("2006-01-02 15:04:05", m); err == nil {
					dump.Timestamp = t
				}
			}
		}

		// Parse signal
		if m := reSignal.FindStringSubmatch(line); len(m) > 2 {
			dump.SignalNum, _ = strconv.Atoi(m[1])
			dump.Signal = m[2]
		}

		// Parse PID
		if dump.ProcessID == 0 {
			if m := rePID.FindStringSubmatch(line); len(m) > 1 {
				dump.ProcessID, _ = strconv.Atoi(m[1])
			}
		}

		// Parse service name
		if dump.ServiceName == "" {
			if m := reService.FindStringSubmatch(strings.ToLower(line)); len(m) > 1 {
				dump.ServiceName = m[1]
			}
		}

		// Parse SID
		if dump.SID == "" {
			if m := reSID.FindStringSubmatch(line); len(m) > 1 {
				dump.SID = m[1]
			}
		}

		// Parse host
		if dump.Host == "" {
			if m := reHost.FindStringSubmatch(line); len(m) > 1 {
				dump.Host = m[1]
			}
		}

		// Detect thread sections
		if strings.Contains(line, "Thread") && strings.Contains(line, "crashed") {
			if currentThread != nil {
				dump.Threads = append(dump.Threads, *currentThread)
			}
			threadID := extractInt(line)
			currentThread = &ThreadInfo{
				ThreadID: threadID,
				IsCrash:  true,
				State:    "CRASHED",
			}
			inStack = true
			inRegs = false
			continue
		}

		if strings.Contains(line, "Thread") && regexp.MustCompile(`Thread\s+\d+`).MatchString(line) {
			if currentThread != nil && currentThread.IsCrash {
				dump.Threads = append(dump.Threads, *currentThread)
			}
			threadID := extractInt(line)
			if currentThread == nil || !currentThread.IsCrash {
				currentThread = &ThreadInfo{
					ThreadID: threadID,
					State:    "RUNNING",
				}
			}
			inStack = true
			inRegs = false
			continue
		}

		// Detect register section
		if strings.Contains(strings.ToLower(line), "registers") {
			inRegs = true
			inStack = false
			continue
		}

		// Parse stack frames
		if inStack {
			if frame, ok := parseStackFrame(line); ok {
				if currentThread != nil {
					currentThread.Stack = append(currentThread.Stack, frame)
				} else {
					dump.StackTrace = append(dump.StackTrace, frame)
				}
				continue
			}
		}

		// Parse registers
		if inRegs {
			for _, m := range reRegister.FindAllStringSubmatch(line, -1) {
				if len(m) > 2 {
					dump.Registers[m[1]] = m[2]
				}
			}
		}
	}

	if currentThread != nil {
		dump.Threads = append(dump.Threads, *currentThread)
	}

	// Extract crash thread stack as primary stack
	for _, t := range dump.Threads {
		if t.IsCrash {
			dump.StackTrace = t.Stack
			break
		}
	}

	// If no thread-specific stack, use whatever we collected
	if len(dump.StackTrace) == 0 && len(allLines) > 0 {
		for _, line := range allLines {
			if frame, ok := parseStackFrame(line); ok {
				dump.StackTrace = append(dump.StackTrace, frame)
			}
		}
	}

	// Set root cause based on signal
	if desc, ok := crashRootCauses[dump.Signal]; ok {
		dump.RootCause = desc
	} else if dump.SignalNum > 0 {
		if desc, ok := signalDescriptions[dump.SignalNum]; ok {
			dump.RootCause = desc
		}
	}

	// Try to infer reason from stack
	dump.CrashReason = inferCrashReason(dump)
	dump.Summary = buildCrashSummary(dump)

	if dump.Timestamp.IsZero() {
		dump.Timestamp = time.Now()
	}
	if dump.ServiceName == "" {
		// Try to extract from filename
		for _, svc := range []string{"indexserver", "nameserver", "compileserver", "preprocessor", "webdispatcher"} {
			if strings.Contains(strings.ToLower(filename), svc) {
				dump.ServiceName = "hdb" + svc
				break
			}
		}
		if dump.ServiceName == "" {
			dump.ServiceName = "unknown"
		}
	}

	return dump, nil
}

func parseStackFrame(line string) (StackFrame, bool) {
	line = strings.TrimSpace(line)
	if m := reFrame.FindStringSubmatch(line); len(m) > 3 {
		num, _ := strconv.Atoi(m[1])
		frame := StackFrame{
			FrameNum: num,
			Address:  m[2],
			Function: cleanFunctionName(m[3]),
		}
		if m[4] != "" {
			frame.File = m[4]
			frame.Line, _ = strconv.Atoi(m[5])
			frame.IsHANACode = isHANASource(m[4])
		}
		if m[6] != "" {
			frame.Library = m[6]
		}
		frame.Description = describeFunction(frame.Function)
		return frame, true
	}
	if m := reFrameSimple.FindStringSubmatch(line); len(m) > 2 && strings.HasPrefix(line, "#") {
		num, _ := strconv.Atoi(m[1])
		return StackFrame{
			FrameNum:    num,
			Address:     m[2],
			Function:    cleanFunctionName(m[3]),
			Description: describeFunction(m[3]),
		}, true
	}
	return StackFrame{}, false
}

func cleanFunctionName(fn string) string {
	// Remove template noise for readability
	fn = strings.TrimSpace(fn)
	if len(fn) > 120 {
		fn = fn[:117] + "..."
	}
	return fn
}

func isHANASource(file string) bool {
	for _, prefix := range []string{"TRex", "hana", "ltt", "Execution", "Join", "Attribute", "persistence"} {
		if strings.Contains(file, prefix) {
			return true
		}
	}
	return false
}

func describeFunction(fn string) string {
	for prefix, desc := range knownHANAFunctions {
		if strings.Contains(fn, prefix) {
			return desc
		}
	}
	if strings.Contains(fn, "malloc") || strings.Contains(fn, "alloc") {
		return "Memory allocation function"
	}
	if strings.Contains(fn, "free") || strings.Contains(fn, "delete") {
		return "Memory deallocation"
	}
	if strings.Contains(fn, "mutex") || strings.Contains(fn, "lock") || strings.Contains(fn, "Lock") {
		return "Synchronization primitive"
	}
	if strings.Contains(fn, "signal") || strings.Contains(fn, "Signal") {
		return "Signal handling"
	}
	return ""
}

func inferCrashReason(dump *CrashDump) string {
	if dump.Signal == "SIGSEGV" {
		// Look for patterns in stack
		for _, f := range dump.StackTrace {
			if strings.Contains(f.Function, "malloc") || strings.Contains(f.Function, "free") {
				return "Heap corruption detected during memory allocation/deallocation"
			}
			if strings.Contains(f.Function, "JoinEvaluator") {
				return "Null pointer dereference in Join Evaluator during query execution"
			}
			if strings.Contains(f.Function, "AttributeEngine") {
				return "Invalid memory access in Column Store Attribute Engine"
			}
		}
		return "Invalid memory access (null pointer or out-of-bounds)"
	}
	if dump.Signal == "SIGABRT" {
		for _, f := range dump.StackTrace {
			if strings.Contains(f.Function, "assert") || strings.Contains(f.Function, "Assert") {
				return "Internal HANA assertion failed - data consistency check violated"
			}
		}
		return "Process aborted due to unrecoverable internal error"
	}
	if dump.Signal == "SIGKILL" {
		return "Process forcefully terminated - likely OOM killer or operator shutdown"
	}
	return fmt.Sprintf("Terminated by signal %s (%d)", dump.Signal, dump.SignalNum)
}

func buildCrashSummary(dump *CrashDump) string {
	service := dump.ServiceName
	if service == "" {
		service = "HANA service"
	}
	signal := dump.Signal
	if signal == "" && dump.SignalNum > 0 {
		signal = fmt.Sprintf("signal %d", dump.SignalNum)
	}
	return fmt.Sprintf("%s (PID %d) crashed with %s. %s", service, dump.ProcessID, signal, dump.CrashReason)
}

func extractInt(s string) int {
	re := regexp.MustCompile(`\d+`)
	m := re.FindString(s)
	n, _ := strconv.Atoi(m)
	return n
}
