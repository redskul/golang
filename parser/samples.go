package parser

import (
	"math/rand"
	"time"
)

// GenerateSampleData creates realistic synthetic HANA diagnostic data for demo purposes
func GenerateSampleData() *DiagnosticReport {
	now := time.Now()
	baseTime := now.Add(-6 * time.Hour)

	report := &DiagnosticReport{
		GeneratedAt: now,
		Host:        "hanadb01.corp.local",
		SID:         "HDB",
	}

	// --- Crash Dumps ---
	report.CrashDumps = []CrashDump{
		{
			Timestamp:   baseTime.Add(2 * time.Hour),
			ServiceName: "hdbindexserver",
			ProcessID:   18432,
			Signal:      "SIGSEGV",
			SignalNum:   11,
			Host:        "hanadb01.corp.local",
			SID:         "HDB",
			Severity:    SeverityFatal,
			CrashReason: "Null pointer dereference in Join Evaluator during parallel hash join",
			RootCause:   "Invalid memory access (SIGSEGV). The hdbindexserver process attempted to dereference a null pointer at address 0x0000000000000018 inside the Join Evaluator. This occurred during the execution of a hash join with parallel workers. A race condition in the parallel join coordinator allowed a worker thread to access a join result buffer that was already freed by the coordinator thread.",
			Summary:     "hdbindexserver (PID 18432) crashed with SIGSEGV during parallel hash join execution",
			StackTrace: []StackFrame{
				{FrameNum: 0, Address: "0x00007f8a2c4b3210", Function: "JoinEvaluator::HashJoin::executeParallel(JoinContext*, WorkerPool*)", File: "JoinEvaluator.cpp", Line: 1847, IsHANACode: true, Description: "Join Execution Engine"},
				{FrameNum: 1, Address: "0x00007f8a2c4a8f40", Function: "JoinEvaluator::ParallelWorker::run()", File: "JoinEvaluator.cpp", Line: 2103, IsHANACode: true, Description: "Join Execution Engine"},
				{FrameNum: 2, Address: "0x00007f8a2b991c30", Function: "Execution::QueryPlanNode::execute(ExecutionContext&)", File: "ExecutionNode.cpp", Line: 445, IsHANACode: true, Description: "Query Execution Engine"},
				{FrameNum: 3, Address: "0x00007f8a2b8f3d20", Function: "TRexAlgebra::Operator::run(PlanContext*)", File: "TRexOperator.cpp", Line: 892, IsHANACode: true, Description: "SQL Algebra Engine (query plan execution)"},
				{FrameNum: 4, Address: "0x00007f8a2a113b10", Function: "AttributeEngine::Column::readCompressed(ReadContext&)", File: "AttributeEngine.cpp", Line: 3301, IsHANACode: true, Description: "Column Store Attribute Engine"},
				{FrameNum: 5, Address: "0x00007f8a1f24c900", Function: "std::thread::_Invoker<...>::operator()()", Library: "libstdc++.so.6", Description: "C++ Standard Library"},
				{FrameNum: 6, Address: "0x00007f8a1c3d2000", Function: "start_thread", Library: "libpthread.so.0"},
				{FrameNum: 7, Address: "0x00007f8a1b9f4000", Function: "clone", Library: "libc.so.6"},
			},
			Threads: []ThreadInfo{
				{ThreadID: 18432, Name: "SQL Executor", State: "CRASHED", IsCrash: true},
				{ThreadID: 18433, Name: "Delta Merge", State: "RUNNING"},
				{ThreadID: 18434, Name: "Savepoint", State: "WAITING"},
				{ThreadID: 18435, Name: "Log Flusher", State: "RUNNING"},
			},
			Registers: map[string]string{
				"RAX": "0x0000000000000000", "RBX": "0x00007f8a2d401820",
				"RCX": "0x00007f8a2c4b3218", "RDX": "0x0000000000000018",
				"RSP": "0x00007f8a1d3ffc80", "RBP": "0x00007f8a1d3ffca0",
				"RIP": "0x00007f8a2c4b3210",
			},
		},
		{
			Timestamp:   baseTime.Add(4*time.Hour + 30*time.Minute),
			ServiceName: "hdbindexserver",
			ProcessID:   19001,
			Signal:      "SIGABRT",
			SignalNum:   6,
			Host:        "hanadb01.corp.local",
			SID:         "HDB",
			Severity:    SeverityFatal,
			CrashReason: "Internal HANA assertion failure in persistence layer during savepoint",
			RootCause:   "Process called abort() after an internal assertion check failed in the persistence layer. The assertion 'page->getState() == PAGE_CLEAN' failed, indicating the savepoint manager encountered a dirty page in an unexpected state. This may be caused by a prior memory corruption event or a software defect in the page state machine.",
			Summary:     "hdbindexserver (PID 19001) aborted due to persistence assertion failure during savepoint",
			StackTrace: []StackFrame{
				{FrameNum: 0, Address: "0x00007f8b1a2c3400", Function: "__GI_raise", Library: "libc.so.6"},
				{FrameNum: 1, Address: "0x00007f8b1a2c3500", Function: "__GI_abort", Library: "libc.so.6"},
				{FrameNum: 2, Address: "0x00007f8b2d113f00", Function: "hdb::persistence::Savepoint::assertPageState(Page*, PageState)", File: "Savepoint.cpp", Line: 2847, IsHANACode: true, Description: "Persistence Layer (log/data volume I/O)"},
				{FrameNum: 3, Address: "0x00007f8b2d112a00", Function: "hdb::persistence::Savepoint::writeDirtyPages()", File: "Savepoint.cpp", Line: 1923, IsHANACode: true, Description: "Persistence Layer (log/data volume I/O)"},
				{FrameNum: 4, Address: "0x00007f8b2d100b00", Function: "hdb::persistence::SavepointManager::run()", File: "SavepointManager.cpp", Line: 441, IsHANACode: true, Description: "Persistence Layer (log/data volume I/O)"},
				{FrameNum: 5, Address: "0x00007f8b1f24c900", Function: "std::thread::_Invoker<...>::operator()()", Library: "libstdc++.so.6"},
			},
		},
	}

	// --- OOM Events ---
	oomBase := baseTime.Add(3 * time.Hour)
	timeline := make([]MemoryDataPoint, 60)
	for i := 0; i < 60; i++ {
		used := 180.0 + float64(i)*1.8 + rand.Float64()*5
		limit := 256.0
		timeline[i] = MemoryDataPoint{
			Timestamp: oomBase.Add(time.Duration(i) * time.Minute),
			UsedMB:    used,
			FreeMB:    limit - used,
			LimitMB:   limit,
		}
	}

	report.OOMEvents = []OOMEvent{
		{
			Timestamp:     oomBase.Add(59 * time.Minute),
			Host:          "hanadb01.corp.local",
			SID:           "HDB",
			ServiceName:   "hdbindexserver",
			ProcessID:     18900,
			TotalMemory:   268435456000, // 250 GB
			UsedMemory:    268100000000,
			FreeMemory:    335456000,
			RequestedSize: 1073741824, // 1 GB
			MemoryLimit:   268435456000,
			Allocator:     "PoolAllocator",
			OOMType:       "global allocation limit",
			KilledProcess: "hdbindexserver",
			RootCause:     "Memory exhausted at 99.9% of limit (249.69 GB used of 250.00 GB limit). The HANA process hdbindexserver could not allocate 1.00 GB of additional memory.",
			Recommendation: "1. Increase global_allocation_limit in indexserver.ini to at least 312.50 GB.\n2. Review top memory consumers and reduce working set.\n3. Consider adding RAM or enabling HANA Dynamic Tiering.\n4. Check for memory leaks using M_HEAP_MEMORY view.",
			Timeline:      timeline,
			Consumers: []MemoryConsumer{
				{Name: "CS_COLUMN_STORE", UsedBytes: 107374182400, PeakBytes: 112000000000, Percentage: 40.1, Category: "Column Store"},
				{Name: "CS_JOIN_ENGINE_RESULT", UsedBytes: 53687091200, PeakBytes: 60000000000, Percentage: 20.0, Category: "Result Cache"},
				{Name: "RS_ROW_STORE", UsedBytes: 32212254720, PeakBytes: 35000000000, Percentage: 12.0, Category: "Row Store"},
				{Name: "SQL_PLAN_CACHE", UsedBytes: 21474836480, PeakBytes: 22000000000, Percentage: 8.0, Category: "SQL Engine"},
				{Name: "CS_ATTRIBUTE_DICT", UsedBytes: 16106127360, PeakBytes: 17000000000, Percentage: 6.0, Category: "Column Store"},
				{Name: "BACKUP_BUFFER", UsedBytes: 10737418240, PeakBytes: 12000000000, Percentage: 4.0, Category: "Backup"},
				{Name: "INDEX_STRUCTURES", UsedBytes: 8053063680, PeakBytes: 9000000000, Percentage: 3.0, Category: "Index Structures"},
				{Name: "STATISTICS_SERVER", UsedBytes: 5368709120, PeakBytes: 6000000000, Percentage: 2.0, Category: "Statistics"},
				{Name: "OTHER", UsedBytes: 12884901888, PeakBytes: 14000000000, Percentage: 4.9, Category: "Other"},
			},
			PoolStats: []MemoryPoolStat{
				{PoolName: "ColumnStore", UsedBytes: 107374182400, AllocBytes: 115000000000, FreeBytes: 7625817600, Fragmented: 6.6},
				{PoolName: "JoinEngine", UsedBytes: 53687091200, AllocBytes: 56000000000, FreeBytes: 2312908800, Fragmented: 4.1},
				{PoolName: "RowStore", UsedBytes: 32212254720, AllocBytes: 35000000000, FreeBytes: 2787745280, Fragmented: 7.9},
				{PoolName: "PlanCache", UsedBytes: 21474836480, AllocBytes: 22000000000, FreeBytes: 525163520, Fragmented: 2.4},
			},
			StackTrace: []StackFrame{
				{FrameNum: 0, Address: "0x00007f8b2d100b00", Function: "MemoryManager::allocate(size_t)", File: "MemoryManager.cpp", Line: 891, IsHANACode: true},
				{FrameNum: 1, Address: "0x00007f8b2d101c00", Function: "JoinEvaluator::HashTable::build(InputIterator&)", File: "HashJoin.cpp", Line: 344, IsHANACode: true, Description: "Join Execution Engine"},
				{FrameNum: 2, Address: "0x00007f8b2c4b3210", Function: "JoinEvaluator::HashJoin::executeParallel(JoinContext*)", File: "JoinEvaluator.cpp", Line: 1701, IsHANACode: true},
				{FrameNum: 3, Address: "0x00007f8b2b8f3d20", Function: "TRexAlgebra::Operator::run(PlanContext*)", File: "TRexOperator.cpp", Line: 892, IsHANACode: true},
			},
		},
	}

	// --- Log Files ---
	logEntries := generateSampleLogEntries(baseTime)
	report.LogFiles = []LogFile{
		{
			Filename:    "nameserver_alert_hanadb01.30001.000.trc",
			ServiceName: "hdb nameserver",
			Host:        "hanadb01.corp.local",
			ParsedAt:    now,
			Entries:     logEntries,
			ErrorCount:  countBySeverity(logEntries, SeverityError) + countBySeverity(logEntries, SeverityFatal),
			WarnCount:   countBySeverity(logEntries, SeverityWarning),
			Summary:     "nameserver alert trace — 6 errors, 14 warnings over 6-hour window",
			Timeline:    generateLogTimeline(logEntries),
		},
	}

	// --- Issues ---
	report.TopIssues = []Issue{
		{
			ID:          "ISS-001",
			Title:       "Repeated hdbindexserver crashes due to race condition in parallel hash join",
			Severity:    SeverityFatal,
			Timestamp:   baseTime.Add(2 * time.Hour),
			Source:      "crash_dump",
			Description: "hdbindexserver crashed twice within 4 hours. The first crash (SIGSEGV) originated in JoinEvaluator::HashJoin::executeParallel, and the second (SIGABRT) in the persistence layer during savepoint. The crashes may be related — the SIGSEGV could have corrupted page state, causing the later assertion failure.",
			RootCause:   "Race condition between parallel join worker threads and the join coordinator. A worker thread accessed a freed result buffer while the coordinator thread was cleaning up after a join phase transition.",
			Impact:      "Database service unavailability. All active sessions disconnected. Manual restart required. Potential data inconsistency if savepoint was not completed before crash.",
			Resolution: []string{
				"Apply SAP Note 3421891 - Fix for parallel hash join race condition in HANA 2.0 SPS07+",
				"Temporarily disable parallel hash join: ALTER SYSTEM ALTER CONFIGURATION ('indexserver.ini','SYSTEM') SET ('joins','enable_parallel_hash_join') = 'false' WITH RECONFIGURE",
				"Review SAP Note 3200000 for HANA crash dump analysis best practices",
				"Open a P1 SAP Support message with crash dump and trace files attached",
			},
			References: []string{"SAP Note 3421891", "SAP Note 3200000", "KBA 2380176"},
		},
		{
			ID:          "ISS-002",
			Title:       "OOM event: global allocation limit exhausted (99.9% usage)",
			Severity:    SeverityError,
			Timestamp:   oomBase.Add(59 * time.Minute),
			Source:      "oom_event",
			Description: "HANA consumed 99.9% of the 250 GB global allocation limit. Column Store (40.1%) and Join Engine result sets (20%) are the largest consumers. Memory grew steadily over 60 minutes before exhaustion.",
			RootCause:   "A long-running query generated a large intermediate hash table (Join Engine result: 50 GB) that could not fit in available memory. No memory pressure signal was triggered in time to abort the query before OOM.",
			Impact:      "hdbindexserver process killed by HANA OOM handler. All sessions lost. Service required restart.",
			Resolution: []string{
				"Increase global_allocation_limit: ALTER SYSTEM ALTER CONFIGURATION ('indexserver.ini','SYSTEM') SET ('memorymanager','global_allocation_limit') = '300000' WITH RECONFIGURE",
				"Enable memory pressure reaction: SET ('memorymanager','oom_dump_time_delta') = '30'",
				"Limit SQL result size: SET ('sqlscript','max_result_size_mb') = '10240'",
				"Analyze query using: SELECT * FROM M_EXPENSIVE_STATEMENTS ORDER BY TOTAL_MEMORY_SIZE DESC",
				"Consider partitioning large tables to reduce column store footprint",
			},
			References: []string{"SAP Note 1999997", "SAP Note 2222200", "SAP HANA Administration Guide: Memory Management"},
		},
		{
			ID:          "ISS-003",
			Title:       "System replication lag exceeded threshold (>30s)",
			Severity:    SeverityWarning,
			Timestamp:   baseTime.Add(time.Hour),
			Source:      "log_file",
			Description: "Multiple warnings in nameserver trace indicate system replication (HSR) shipping lag exceeded 30 seconds. This could lead to data loss in failover scenario beyond the RPO window.",
			RootCause:   "High redo log generation rate during peak load combined with network congestion between primary and secondary nodes.",
			Impact:      "RPO (Recovery Point Objective) may be exceeded in the event of a failover. Secondary system may be several minutes behind primary.",
			Resolution: []string{
				"Check network bandwidth between primary/secondary: ping -s 65000 <secondary_host>",
				"Review log shipping mode: SELECT * FROM M_SERVICE_REPLICATION",
				"Consider switching to asynchronous replication temporarily during peak load",
				"Add network bandwidth or enable log compression: SET ('system_replication','enable_log_compression') = 'true'",
			},
			References: []string{"SAP Note 2100000", "SAP HANA System Replication Guide"},
		},
	}

	report.HealthScore = 28 // Poor health
	return report
}

func generateSampleLogEntries(base time.Time) []LogEntry {
	entries := []LogEntry{
		{Timestamp: base, Severity: SeverityInfo, Component: "NameServer", Thread: "t001", Message: "Starting name server on port 30001", LineNum: 1},
		{Timestamp: base.Add(5 * time.Minute), Severity: SeverityInfo, Component: "SystemReplication", Thread: "t002", Message: "System replication initialized, mode=sync, tier=1", LineNum: 5},
		{Timestamp: base.Add(15 * time.Minute), Severity: SeverityWarning, Component: "SystemReplication", Thread: "t003", Message: "System replication shipping lag 32s exceeds threshold 30s", LineNum: 12, Explanation: "System replication event. Check HSR lag and network between primary/secondary.", IsError: false},
		{Timestamp: base.Add(20 * time.Minute), Severity: SeverityWarning, Component: "SystemReplication", Thread: "t003", Message: "System replication shipping lag 45s exceeds threshold 30s", LineNum: 18, Explanation: "System replication event. Check HSR lag and network between primary/secondary."},
		{Timestamp: base.Add(30 * time.Minute), Severity: SeverityInfo, Component: "Savepoint", Thread: "t010", Message: "Savepoint started (critical phase), LSN=18473829", LineNum: 25},
		{Timestamp: base.Add(31 * time.Minute), Severity: SeverityInfo, Component: "Savepoint", Thread: "t010", Message: "Savepoint finished, duration=45s, written=12.3GB", LineNum: 26},
		{Timestamp: base.Add(45 * time.Minute), Severity: SeverityWarning, Component: "MemoryManager", Thread: "t020", Message: "Memory usage at 78% of global_allocation_limit (195000MB/250000MB)", LineNum: 40, Explanation: "System out of memory: no heap space available for requested allocation."},
		{Timestamp: base.Add(55 * time.Minute), Severity: SeverityWarning, Component: "MemoryManager", Thread: "t020", Message: "Memory usage at 89% of global_allocation_limit (222500MB/250000MB)", LineNum: 48},
		{Timestamp: base.Add(1*time.Hour + 5*time.Minute), Severity: SeverityError, Component: "MemoryManager", Thread: "t020", Message: "Memory usage critical: 97% of global_allocation_limit (242500MB/250000MB). Triggering OOM handler.", LineNum: 60, IsError: true, Explanation: "System out of memory: no heap space available for requested allocation."},
		{Timestamp: base.Add(1*time.Hour + 10*time.Minute), Severity: SeverityFatal, Component: "MemoryManager", Thread: "t020", Message: "bad_alloc: Cannot allocate 1073741824 bytes. global_allocation_limit reached. Killing process hdbindexserver (PID 18900)", LineNum: 65, IsError: true, Explanation: "C++ bad_alloc exception: HANA failed to allocate memory. System may be running low on RAM."},
		{Timestamp: base.Add(1*time.Hour + 12*time.Minute), Severity: SeverityError, Component: "NameServer", Thread: "t001", Message: "Service hdbindexserver on hanadb01:30003 is not responding — initiating restart", LineNum: 70, IsError: true, Explanation: "Service crashed. Review crash dump in trace directory."},
		{Timestamp: base.Add(1*time.Hour + 20*time.Minute), Severity: SeverityInfo, Component: "NameServer", Thread: "t001", Message: "Service hdbindexserver restarted successfully on port 30003", LineNum: 80},
		{Timestamp: base.Add(2*time.Hour + 5*time.Minute), Severity: SeverityFatal, Component: "IndexServer", Thread: "t100", Message: "crash: SIGSEGV in JoinEvaluator::HashJoin::executeParallel at JoinEvaluator.cpp:1847", LineNum: 95, IsError: true, Explanation: "Service crashed. Review crash dump in trace directory."},
		{Timestamp: base.Add(2*time.Hour + 6*time.Minute), Severity: SeverityError, Component: "NameServer", Thread: "t001", Message: "hdbindexserver (PID 18432) crashed with signal 11 (SIGSEGV). Core dump written to /hana/shared/HDB/HDB/trace/", LineNum: 97, IsError: true},
		{Timestamp: base.Add(2*time.Hour + 15*time.Minute), Severity: SeverityInfo, Component: "NameServer", Thread: "t001", Message: "Automatic service restart: hdbindexserver starting on port 30003", LineNum: 100},
		{Timestamp: base.Add(3 * time.Hour), Severity: SeverityInfo, Component: "DeltaMerge", Thread: "t050", Message: "Delta merge started on table SALES_ORDERS (partition 1-4)", LineNum: 120},
		{Timestamp: base.Add(3*time.Hour + 20*time.Minute), Severity: SeverityInfo, Component: "DeltaMerge", Thread: "t050", Message: "Delta merge completed, merged 4.2M rows, duration=18m", LineNum: 125},
		{Timestamp: base.Add(3*time.Hour + 30*time.Minute), Severity: SeverityWarning, Component: "Persistence", Thread: "t030", Message: "Log backup overdue: last backup was 4h ago, log segment usage at 72%", LineNum: 130, Explanation: "Redo log is full. Backup log segment immediately and investigate log growth."},
		{Timestamp: base.Add(4*time.Hour + 30*time.Minute), Severity: SeverityFatal, Component: "Persistence", Thread: "t030", Message: "Assertion failed: page->getState() == PAGE_CLEAN at Savepoint.cpp:2847. Calling abort().", LineNum: 160, IsError: true, Explanation: "Internal HANA assertion failed. This indicates a bug or data inconsistency. Report to SAP Support."},
		{Timestamp: base.Add(4*time.Hour + 31*time.Minute), Severity: SeverityError, Component: "NameServer", Thread: "t001", Message: "hdbindexserver (PID 19001) aborted (SIGABRT). Core dump written.", LineNum: 162, IsError: true},
		{Timestamp: base.Add(4*time.Hour + 40*time.Minute), Severity: SeverityInfo, Component: "NameServer", Thread: "t001", Message: "Service hdbindexserver restarted, performing crash recovery...", LineNum: 165},
		{Timestamp: base.Add(5 * time.Hour), Severity: SeverityInfo, Component: "IndexServer", Thread: "t200", Message: "Crash recovery completed. 3 uncommitted transactions rolled back.", LineNum: 180},
		{Timestamp: base.Add(5*time.Hour + 30*time.Minute), Severity: SeverityWarning, Component: "License", Thread: "t300", Message: "HANA license check: 85% of licensed memory capacity used", LineNum: 200, Explanation: "License validation issue. Check that HANA license is valid and within limits."},
		{Timestamp: base.Add(6 * time.Hour), Severity: SeverityInfo, Component: "NameServer", Thread: "t001", Message: "System stable. All services running. Uptime: 1h 20m", LineNum: 220},
	}
	// Add explanations
	for i := range entries {
		if entries[i].Explanation == "" {
			entries[i].Explanation = explainLogMessage(entries[i].Message)
		}
	}
	return entries
}

func generateLogTimeline(entries []LogEntry) []LogTimelineBucket {
	buckets := make(map[time.Time]*LogTimelineBucket)
	for _, e := range entries {
		t := e.Timestamp.Truncate(30 * time.Minute)
		if _, ok := buckets[t]; !ok {
			buckets[t] = &LogTimelineBucket{Time: t}
		}
		switch e.Severity {
		case SeverityError, SeverityFatal:
			buckets[t].Errors++
		case SeverityWarning:
			buckets[t].Warnings++
		default:
			buckets[t].Infos++
		}
	}
	var result []LogTimelineBucket
	for _, b := range buckets {
		result = append(result, *b)
	}
	sortTimeline(result)
	return result
}

func countBySeverity(entries []LogEntry, sev Severity) int {
	n := 0
	for _, e := range entries {
		if e.Severity == sev {
			n++
		}
	}
	return n
}
