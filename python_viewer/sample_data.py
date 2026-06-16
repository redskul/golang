"""Synthetic sample data for demo mode (used only when no --dir is given)."""
from datetime import datetime, timedelta


def generate():
    now = datetime.now()
    base = now - timedelta(hours=6)

    crash = {
        "timestamp": base + timedelta(hours=2), "service_name": "hdbindexserver", "process_id": 18432,
        "signal": "SIGSEGV", "signal_num": 11, "host": "hanadb01.corp.local", "sid": "HDB",
        "crash_reason": "Null pointer dereference in Join Evaluator during parallel hash join",
        "root_cause": "Invalid memory access (SIGSEGV) inside the Join Evaluator during a parallel hash join. "
                       "A race condition allowed a worker thread to access a join result buffer already freed "
                       "by the coordinator thread.",
        "summary": "hdbindexserver (PID 18432) crashed with SIGSEGV during parallel hash join execution",
        "stack_trace": [
            {"frame_num": 0, "address": "0x00007f8a2c4b3210", "function": "JoinEvaluator::HashJoin::executeParallel(JoinContext*)",
             "file": "JoinEvaluator.cpp", "line": 1847, "is_hana_code": True, "description": "Join Execution Engine"},
            {"frame_num": 1, "address": "0x00007f8a2b8f3d20", "function": "TRexAlgebra::Operator::run(PlanContext*)",
             "file": "TRexOperator.cpp", "line": 892, "is_hana_code": True, "description": "SQL Algebra Engine"},
            {"frame_num": 2, "address": "0x00007f8a1f24c900", "function": "std::thread::_Invoker<...>::operator()()",
             "file": "", "line": 0, "is_hana_code": False, "description": ""},
        ],
        "source_file": "(sample data)",
    }

    oom = {
        "timestamp": base + timedelta(hours=3, minutes=59), "host": "hanadb01.corp.local", "sid": "HDB",
        "service_name": "hdbindexserver",
        "used_memory": 268100000000, "free_memory": 335456000, "memory_limit": 268435456000,
        "requested_size": 1073741824,
        "root_cause": "Memory exhausted at 99.9% of limit (249.69 GB used of 250.00 GB limit). "
                       "Process could not allocate 1.00 GB more.",
        "consumers": [
            {"name": "CS_COLUMN_STORE", "used_bytes": 107374182400, "percentage": 40.1},
            {"name": "CS_JOIN_ENGINE_RESULT", "used_bytes": 53687091200, "percentage": 20.0},
            {"name": "RS_ROW_STORE", "used_bytes": 32212254720, "percentage": 12.0},
            {"name": "SQL_PLAN_CACHE", "used_bytes": 21474836480, "percentage": 8.0},
        ],
        "source_file": "(sample data)",
    }

    log_entries = [
        {"timestamp": base, "severity": "INFO", "component": "NameServer", "message": "Starting name server",
         "explanation": "", "line_num": 1, "is_error": False},
        {"timestamp": base + timedelta(minutes=45), "severity": "WARNING", "component": "MemoryManager",
         "message": "Memory usage at 78% of global_allocation_limit",
         "explanation": "System out of memory: no heap space available for the requested allocation.",
         "line_num": 40, "is_error": False},
        {"timestamp": base + timedelta(hours=1, minutes=10), "severity": "FATAL", "component": "MemoryManager",
         "message": "bad_alloc: Cannot allocate 1073741824 bytes. Killing process hdbindexserver (PID 18900)",
         "explanation": "C++ bad_alloc exception: HANA failed to allocate memory. System may be low on RAM.",
         "line_num": 65, "is_error": True},
        {"timestamp": base + timedelta(hours=2, minutes=5), "severity": "FATAL", "component": "IndexServer",
         "message": "crash: SIGSEGV in JoinEvaluator::HashJoin::executeParallel",
         "explanation": "Service crashed. Review the crash dump in the trace directory.",
         "line_num": 95, "is_error": True},
    ]
    log_file = {
        "filename": "nameserver_alert_hanadb01.30001.000.trc", "host": "hanadb01.corp.local",
        "entries": log_entries,
        "error_count": sum(1 for e in log_entries if e["severity"] in ("ERROR", "FATAL")),
        "warn_count": sum(1 for e in log_entries if e["severity"] == "WARNING"),
        "source_file": "(sample data)",
    }

    report = {
        "generated_at": now, "host": "hanadb01.corp.local", "sid": "HDB", "trace_dir": "",
        "crash_dumps": [crash], "oom_events": [oom], "log_files": [log_file],
    }

    import scanner
    report["top_issues"] = scanner.derive_issues(report)
    report["health_score"] = scanner.compute_health_score(report)
    return report
