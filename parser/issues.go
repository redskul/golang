package parser

import (
	"fmt"
	"time"
)

var nowFunc = time.Now

// DeriveIssues inspects parsed crash dumps, OOM events, and log files and
// produces a prioritized list of real diagnosed issues — no canned data.
func DeriveIssues(report *DiagnosticReport) []Issue {
	var issues []Issue

	for i, cd := range report.CrashDumps {
		issues = append(issues, Issue{
			ID:          fmt.Sprintf("CRASH-%03d", i+1),
			Title:       fmt.Sprintf("%s crashed with %s (PID %d)", cd.ServiceName, cd.Signal, cd.ProcessID),
			Severity:    SeverityFatal,
			Timestamp:   cd.Timestamp,
			Source:      "crash_dump",
			Description: cd.Summary,
			RootCause:   cd.RootCause,
			Impact:      crashImpact(cd),
			Resolution:  crashResolution(cd),
			References:  []string{"SAP Note 1992434 (Crash dump analysis)", "SAP Note 1781940 (HANA troubleshooting)"},
		})
	}

	for i, oom := range report.OOMEvents {
		issues = append(issues, Issue{
			ID:          fmt.Sprintf("OOM-%03d", i+1),
			Title:       fmt.Sprintf("%s out-of-memory (%s)", oom.ServiceName, oom.OOMType),
			Severity:    SeverityError,
			Timestamp:   oom.Timestamp,
			Source:      "oom_event",
			Description: fmt.Sprintf("Process requested %s but only %s was free.", formatBytes(oom.RequestedSize), formatBytes(oom.FreeMemory)),
			RootCause:   oom.RootCause,
			Impact:      "Service likely terminated or rejected new allocations; active sessions may have been disconnected.",
			Resolution:  splitRecommendation(oom.Recommendation),
			References:  []string{"SAP Note 1999997 (Memory FAQ)", "SAP Note 2222200 (Memory Management)"},
		})
	}

	for _, lf := range report.LogFiles {
		for _, e := range lf.Entries {
			if e.Severity != SeverityError && e.Severity != SeverityFatal {
				continue
			}
			issues = append(issues, Issue{
				ID:          fmt.Sprintf("LOG-%s-L%d", lf.Filename, e.LineNum),
				Title:       fmt.Sprintf("%s: %s", e.Component, truncate(e.Message, 80)),
				Severity:    e.Severity,
				Timestamp:   e.Timestamp,
				Source:      "log_file",
				Description: e.Message,
				RootCause:   firstNonEmpty(e.Explanation, "No automatic explanation available for this error pattern — review manually."),
				Impact:      "See trace file for surrounding context: " + lf.Filename,
				Resolution:  []string{"Review " + lf.Filename + " around line " + itoa(e.LineNum) + " for full context."},
			})
		}
	}

	sortIssuesByTime(issues)
	return issues
}

func crashImpact(cd CrashDump) string {
	if cd.Signal == "SIGKILL" {
		return "Process was forcefully terminated, likely by the OS OOM killer or an administrator. Service restart required."
	}
	return "Database service interrupted. All active sessions on this service were disconnected. Manual or automatic restart required; check for data consistency issues on next startup."
}

func crashResolution(cd CrashDump) []string {
	steps := []string{
		fmt.Sprintf("Collect full crash dump and trace files from the HANA trace directory for PID %d.", cd.ProcessID),
	}
	switch cd.Signal {
	case "SIGSEGV":
		steps = append(steps, "Search SAP Notes for known SIGSEGV issues in the affected component (see stack trace).", "Check HANA revision for known fixes; consider patching to latest revision.")
	case "SIGABRT":
		steps = append(steps, "Internal assertion failures usually indicate a product defect or data inconsistency — open an SAP incident with the crash dump attached.")
	case "SIGKILL":
		steps = append(steps, "Check system/kernel logs (dmesg, /var/log/messages) around the crash time for OOM killer activity.", "Review HANA global_allocation_limit vs available system RAM.")
	default:
		steps = append(steps, "Review the annotated stack trace to identify the failing component.")
	}
	steps = append(steps, "If the crash recurs, open a P1/P2 SAP Support incident with this report attached.")
	return steps
}

func splitRecommendation(rec string) []string {
	var out []string
	cur := ""
	for _, r := range rec {
		if r == '\n' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) == 0 {
		out = []string{rec}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func sortIssuesByTime(issues []Issue) {
	for i := 1; i < len(issues); i++ {
		for j := i; j > 0 && issues[j].Timestamp.After(issues[j-1].Timestamp); j-- {
			issues[j], issues[j-1] = issues[j-1], issues[j]
		}
	}
}

// ComputeHealthScore derives a 0-100 health score from real findings.
// Starts at 100 and deducts points for each crash, OOM, and error/warning found.
func ComputeHealthScore(report *DiagnosticReport) int {
	score := 100
	score -= len(report.CrashDumps) * 20
	score -= len(report.OOMEvents) * 15

	for _, lf := range report.LogFiles {
		score -= lf.ErrorCount * 2
		score -= lf.WarnCount * 1
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
