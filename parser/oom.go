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
	reOOMTimestamp  = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2})`)
	reMemUsed       = regexp.MustCompile(`[Uu]sed[:\s]+(\d+(?:\.\d+)?)\s*(KB|MB|GB|B)?`)
	reMemFree       = regexp.MustCompile(`[Ff]ree[:\s]+(\d+(?:\.\d+)?)\s*(KB|MB|GB|B)?`)
	reMemLimit      = regexp.MustCompile(`[Ll]imit[:\s]+(\d+(?:\.\d+)?)\s*(KB|MB|GB|B)?`)
	reMemRequest    = regexp.MustCompile(`[Rr]equested?[:\s]+(\d+(?:\.\d+)?)\s*(KB|MB|GB|B)?`)
	reConsumer      = regexp.MustCompile(`^\s*(\S+.*?)\s{2,}(\d+(?:\.\d+)?)\s*(KB|MB|GB|B)?\s*(\d+(?:\.\d+)?)?`)
	reOOMType       = regexp.MustCompile(`(global allocation limit|per-process limit|pool.*limit|heap.*limit|bad_alloc)`)
	rePoolStat      = regexp.MustCompile(`Pool\s+"([^"]+)"\s*:\s*used=(\d+)\s*alloc=(\d+)`)
	reKilledProcess = regexp.MustCompile(`[Kk]ill(?:ed|ing)\s+(?:process\s+)?(\w+)`)
)

// ParseOOMEvent parses a HANA OOM trace/dump from a reader
func ParseOOMEvent(r io.Reader, filename string) (*OOMEvent, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	event := &OOMEvent{
		Timestamp: time.Now(),
	}

	var inConsumers bool
	var inStack bool

	for scanner.Scan() {
		line := scanner.Text()

		// Timestamp
		if event.Timestamp.IsZero() || event.Timestamp.Equal(time.Now().Truncate(time.Second)) {
			if m := reOOMTimestamp.FindString(line); m != "" {
				for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
					if t, err := time.Parse(layout, m); err == nil {
						event.Timestamp = t
						break
					}
				}
			}
		}

		// Service/process
		if event.ServiceName == "" {
			if m := reService.FindStringSubmatch(strings.ToLower(line)); len(m) > 1 {
				event.ServiceName = m[1]
			}
		}

		// PID
		if event.ProcessID == 0 {
			if m := rePID.FindStringSubmatch(line); len(m) > 1 {
				event.ProcessID, _ = strconv.Atoi(m[1])
			}
		}

		// SID / Host
		if event.SID == "" {
			if m := reSID.FindStringSubmatch(line); len(m) > 1 {
				event.SID = m[1]
			}
		}
		if event.Host == "" {
			if m := reHost.FindStringSubmatch(line); len(m) > 1 {
				event.Host = m[1]
			}
		}

		// OOM type
		if event.OOMType == "" {
			if m := reOOMType.FindString(strings.ToLower(line)); m != "" {
				event.OOMType = m
			}
		}

		// Memory stats
		if m := reMemUsed.FindStringSubmatch(line); len(m) > 1 && event.UsedMemory == 0 {
			event.UsedMemory = parseMemBytes(m[1], m[2])
		}
		if m := reMemFree.FindStringSubmatch(line); len(m) > 1 && event.FreeMemory == 0 {
			event.FreeMemory = parseMemBytes(m[1], m[2])
		}
		if m := reMemLimit.FindStringSubmatch(line); len(m) > 1 && event.MemoryLimit == 0 {
			event.MemoryLimit = parseMemBytes(m[1], m[2])
		}
		if m := reMemRequest.FindStringSubmatch(line); len(m) > 1 && event.RequestedSize == 0 {
			event.RequestedSize = parseMemBytes(m[1], m[2])
		}

		// Killed process
		if event.KilledProcess == "" {
			if m := reKilledProcess.FindStringSubmatch(line); len(m) > 1 {
				event.KilledProcess = m[1]
			}
		}

		// Allocator
		if strings.Contains(line, "allocator") || strings.Contains(line, "Allocator") {
			if event.Allocator == "" {
				parts := strings.Fields(line)
				for i, p := range parts {
					if strings.EqualFold(p, "allocator:") || strings.EqualFold(p, "allocator") {
						if i+1 < len(parts) {
							event.Allocator = parts[i+1]
						}
						break
					}
				}
			}
		}

		// Pool stats
		if m := rePoolStat.FindStringSubmatch(line); len(m) > 3 {
			used, _ := strconv.ParseInt(m[2], 10, 64)
			alloc, _ := strconv.ParseInt(m[3], 10, 64)
			stat := MemoryPoolStat{
				PoolName:   m[1],
				UsedBytes:  used,
				AllocBytes: alloc,
			}
			if alloc > 0 {
				stat.FreeBytes = alloc - used
				stat.Fragmented = float64(alloc-used) / float64(alloc) * 100
			}
			event.PoolStats = append(event.PoolStats, stat)
		}

		// Consumer table detection
		if strings.Contains(line, "Top") && strings.Contains(line, "consumer") {
			inConsumers = true
			inStack = false
			continue
		}
		if strings.Contains(line, "Stack trace") || strings.Contains(line, "stack trace") {
			inStack = true
			inConsumers = false
			continue
		}
		if strings.Contains(line, "---") && inConsumers {
			inConsumers = false
			continue
		}

		if inConsumers && len(strings.TrimSpace(line)) > 0 {
			if c, ok := parseConsumerLine(line); ok {
				event.Consumers = append(event.Consumers, c)
			}
		}

		if inStack {
			if frame, ok := parseStackFrame(line); ok {
				event.StackTrace = append(event.StackTrace, frame)
			}
		}
	}

	// Compute totals if not set
	if event.TotalMemory == 0 && event.UsedMemory > 0 && event.FreeMemory > 0 {
		event.TotalMemory = event.UsedMemory + event.FreeMemory
	}
	if event.TotalMemory == 0 && event.MemoryLimit > 0 {
		event.TotalMemory = event.MemoryLimit
	}

	// Normalize consumer percentages
	var totalConsumed int64
	for _, c := range event.Consumers {
		totalConsumed += c.UsedBytes
	}
	if totalConsumed > 0 {
		for i := range event.Consumers {
			event.Consumers[i].Percentage = float64(event.Consumers[i].UsedBytes) / float64(totalConsumed) * 100
		}
	}

	event.RootCause = buildOOMRootCause(event)
	event.Recommendation = buildOOMRecommendation(event)

	if event.ServiceName == "" {
		event.ServiceName = inferServiceFromFilename(filename)
	}
	if event.OOMType == "" {
		event.OOMType = "global allocation limit"
	}

	return event, nil
}

func parseMemBytes(valStr, unit string) int64 {
	f, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "GB":
		return int64(f * 1024 * 1024 * 1024)
	case "MB":
		return int64(f * 1024 * 1024)
	case "KB":
		return int64(f * 1024)
	default:
		return int64(f)
	}
}

func parseConsumerLine(line string) (MemoryConsumer, bool) {
	if m := reConsumer.FindStringSubmatch(line); len(m) > 2 {
		name := strings.TrimSpace(m[1])
		if name == "" || name == "Name" || name == "---" {
			return MemoryConsumer{}, false
		}
		val, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return MemoryConsumer{}, false
		}
		c := MemoryConsumer{
			Name:     name,
			UsedBytes: parseMemBytes(fmt.Sprintf("%.2f", val), m[3]),
			Category:  categorizeConsumer(name),
		}
		if len(m) > 4 && m[4] != "" {
			peak, _ := strconv.ParseFloat(m[4], 64)
			c.PeakBytes = parseMemBytes(fmt.Sprintf("%.2f", peak), m[3])
		}
		return c, true
	}
	return MemoryConsumer{}, false
}

func categorizeConsumer(name string) string {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "column") || strings.Contains(name, "attribute") || strings.Contains(name, "cs_"):
		return "Column Store"
	case strings.Contains(name, "row") || strings.Contains(name, "rs_"):
		return "Row Store"
	case strings.Contains(name, "result") || strings.Contains(name, "intermediate"):
		return "Result Cache"
	case strings.Contains(name, "plan") || strings.Contains(name, "sql"):
		return "SQL Engine"
	case strings.Contains(name, "index"):
		return "Index Structures"
	case strings.Contains(name, "backup"):
		return "Backup"
	case strings.Contains(name, "stat"):
		return "Statistics"
	case strings.Contains(name, "heap") || strings.Contains(name, "pool"):
		return "Memory Pool"
	default:
		return "Other"
	}
}

func buildOOMRootCause(e *OOMEvent) string {
	used := e.UsedMemory
	limit := e.MemoryLimit
	if used > 0 && limit > 0 {
		pct := float64(used) / float64(limit) * 100
		return fmt.Sprintf("Memory exhausted at %.1f%% of limit (%s used of %s limit). The HANA process %s could not allocate %s of additional memory.",
			pct, formatBytes(used), formatBytes(limit), e.ServiceName, formatBytes(e.RequestedSize))
	}
	if len(e.Consumers) > 0 {
		top := e.Consumers[0]
		return fmt.Sprintf("Memory exhausted. Largest consumer: %s (%.1f%% of total). Process %s failed to allocate %s.",
			top.Name, top.Percentage, e.ServiceName, formatBytes(e.RequestedSize))
	}
	return "HANA process ran out of available memory and could not satisfy an allocation request."
}

func buildOOMRecommendation(e *OOMEvent) string {
	if e.MemoryLimit > 0 {
		newLimit := int64(float64(e.MemoryLimit) * 1.25)
		return fmt.Sprintf(
			"1. Increase global_allocation_limit in indexserver.ini to at least %s.\n"+
				"2. Review top memory consumers and reduce working set (e.g. partition pruning, result cache limits).\n"+
				"3. Consider adding RAM or enabling HANA Dynamic Tiering.\n"+
				"4. Check for memory leaks using M_HEAP_MEMORY view.",
			formatBytes(newLimit))
	}
	return "Increase available memory, review memory consumers, and consider memory optimization techniques."
}

func inferServiceFromFilename(fn string) string {
	fn = strings.ToLower(fn)
	for _, svc := range []string{"indexserver", "nameserver", "compileserver", "preprocessor", "webdispatcher", "xsengine"} {
		if strings.Contains(fn, svc) {
			return "hdb" + svc
		}
	}
	return "hdbindexserver"
}

func formatBytes(b int64) string {
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
}
