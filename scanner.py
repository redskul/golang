"""
HANA trace directory scanner and parser.

Walks a real SAP HANA trace directory, classifies every file as a crash
dump, an out-of-memory (OOM) event, or a plain trace/log file, parses it,
and produces a diagnostic report dict consumed by the Flask web UI.

No synthetic/sample data is used here — every value comes from the files
found on disk.
"""
import os
import re
import socket
from datetime import datetime

SIGNAL_DESCRIPTIONS = {
    4: "SIGILL - Illegal Instruction (corrupt binary or CPU bug)",
    6: "SIGABRT - Process called abort() - assertion failure or internal error",
    7: "SIGBUS - Bus error (misaligned memory access)",
    8: "SIGFPE - Floating point exception (divide by zero)",
    9: "SIGKILL - Kill signal (sent by OS or admin, e.g. OOM killer)",
    11: "SIGSEGV - Segmentation fault (invalid memory access)",
    13: "SIGPIPE - Broken pipe",
    15: "SIGTERM - Termination signal (graceful shutdown requested)",
}

CRASH_ROOT_CAUSES = {
    "SIGSEGV": "Null pointer dereference or illegal memory access. HANA attempted to read/write memory "
               "outside its allocated regions. Common causes: corrupted heap, use-after-free, buffer overflow.",
    "SIGABRT": "Process called abort()/assert(). This is an intentional crash triggered by HANA's internal "
               "consistency checks detecting a fatal data inconsistency.",
    "SIGKILL": "Process was forcefully terminated by the Linux kernel (OOM killer) or an administrator.",
    "SIGBUS":  "Misaligned memory access or hardware fault.",
    "SIGFPE":  "Arithmetic error (division by zero or integer overflow).",
    "SIGILL":  "Illegal CPU instruction. Possible binary corruption.",
}

ERROR_EXPLANATIONS = {
    "bad_alloc": "C++ bad_alloc exception: HANA failed to allocate memory. System may be low on RAM.",
    "out of memory": "System out of memory: no heap space available for the requested allocation.",
    "deadlock": "Transaction deadlock detected between two or more sessions.",
    "lock timeout": "A transaction waited too long for a lock. Review lock_wait_timeout and transaction patterns.",
    "log full": "Redo log is full. Backup the log segment immediately.",
    "disk full": "Disk volume is full. Free space on data/log volume immediately.",
    "savepoint": "Savepoint (checkpoint) operation. Long savepoints may indicate an I/O bottleneck.",
    "replication": "System replication event. Check HSR lag and network between primary/secondary.",
    "assertion": "Internal HANA assertion failed - indicates a bug or data inconsistency.",
    "crash": "Service crashed. Review the crash dump in the trace directory.",
    "network": "Network communication error between HANA nodes/services.",
}

FRAME_RE = re.compile(r"#(\d+)\s+(0x[0-9a-fA-F]+)\s+in\s+(.+?)\s+(?:at\s+(.+):(\d+))?$")
FRAME_SIMPLE_RE = re.compile(r"#(\d+)\s+(0x[0-9a-fA-F]+)\s*(.*)")
SIGNAL_RE = re.compile(r"[Ss]ignal\s+(\d+)\s*\((\w+)\)")
PID_RE = re.compile(r"[Pp]rocess\s+(\d+)")
SERVICE_RE = re.compile(r"(hdbindexserver|hdbnameserver|hdbcompileserver|hdbpreprocessor|hdbwebdispatcher|indexserver|nameserver|compileserver|preprocessor)")
MEM_RE = lambda label: re.compile(rf"{label}[ \t:]+(\d+(?:\.\d+)?)[ \t]*(KB|MB|GB|B)?", re.I)
TIMESTAMP_RE = re.compile(r"(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2})")
LOG_LINE_RE = re.compile(r"^\[?(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:\.\d+)?)\]?\s+(\w+)\s+(\S+)\s+(.*)$")


def fmt_bytes(n):
    if not n:
        return "0 B"
    for unit, size in (("GB", 1024**3), ("MB", 1024**2), ("KB", 1024)):
        if n >= size:
            return f"{n / size:.2f} {unit}"
    return f"{n} B"


def parse_mem(val, unit):
    f = float(val)
    mult = {"GB": 1024**3, "MB": 1024**2, "KB": 1024}.get((unit or "").upper(), 1)
    return int(f * mult)


def classify_file(name, head_text):
    lower = name.lower()
    text = head_text.lower()

    # Filename conventions take priority — they are unambiguous.
    if "crashdump" in lower or lower.startswith("core.") or lower.endswith(".dmp"):
        return "crash"
    if "oom" in lower or "out_of_memory" in lower:
        return "oom"

    # A file made up of structured log lines (timestamp + severity + component)
    # is a regular trace/log file even if a line inside it mentions OOM/crash
    # keywords — those become individual log entries, not a dedicated dump.
    sample_lines = head_text.splitlines()[:40]
    structured = sum(1 for l in sample_lines if LOG_LINE_RE.match(l.strip()))
    if structured >= 2:
        return "log"

    if any(k in text for k in ("received signal", "backtrace", "stack dump")) and \
       any(k in text for k in ("sigsegv", "sigabrt", "signal 11", "signal 6", "crashed")):
        return "crash"
    if "global_allocation_limit" in text or ("bad_alloc" in text and "allocat" in text) or \
       ("out of memory" in text):
        return "oom"
    if text.strip():
        return "log"
    return "unknown"


def is_candidate(name):
    lower = name.lower()
    if lower.endswith((".trc", ".log", ".txt", ".dmp")):
        return True
    if "crashdump" in lower or lower.startswith("core.") or ".oom." in lower:
        return True
    return False


def parse_stack_frame(line):
    line = line.strip()
    m = FRAME_RE.match(line)
    if m:
        num, addr, func, file_, lineno = m.groups()
        return {
            "frame_num": int(num), "address": addr, "function": func.strip(),
            "file": file_ or "", "line": int(lineno) if lineno else 0,
            "is_hana_code": bool(file_ and any(p in file_ for p in
                ("TRex", "Join", "Attribute", "Execution", "persistence", "Savepoint"))),
            "description": describe_function(func),
        }
    m = FRAME_SIMPLE_RE.match(line)
    if m and line.startswith("#"):
        num, addr, func = m.groups()
        return {
            "frame_num": int(num), "address": addr, "function": func.strip(),
            "file": "", "line": 0, "is_hana_code": False,
            "description": describe_function(func),
        }
    return None


def describe_function(fn):
    mapping = {
        "JoinEvaluator": "Join Execution Engine",
        "AttributeEngine": "Column Store Attribute Engine",
        "TRexAlgebra": "SQL Algebra Engine",
        "persistence": "Persistence Layer (log/data volume I/O)",
        "Savepoint": "Persistence Layer (log/data volume I/O)",
        "malloc": "Memory allocator",
        "MemoryManager": "Memory Manager",
    }
    for k, v in mapping.items():
        if k in fn:
            return v
    return ""


def parse_crash_dump(path, text, host, sid):
    ts = None
    m = TIMESTAMP_RE.search(text)
    if m:
        for layout in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%dT%H:%M:%S"):
            try:
                ts = datetime.strptime(m.group(1), layout)
                break
            except ValueError:
                continue
    sig_num, sig_name = 0, ""
    m = SIGNAL_RE.search(text)
    if m:
        sig_num, sig_name = int(m.group(1)), m.group(2)
    pid = 0
    m = PID_RE.search(text)
    if m:
        pid = int(m.group(1))
    service = ""
    m = SERVICE_RE.search(text.lower())
    if m:
        service = m.group(1)
        if not service.startswith("hdb"):
            service = "hdb" + service

    frames = []
    for line in text.splitlines():
        f = parse_stack_frame(line)
        if f:
            frames.append(f)

    root_cause = CRASH_ROOT_CAUSES.get(sig_name, SIGNAL_DESCRIPTIONS.get(sig_num, "Unknown crash cause."))
    reason = root_cause.split(".")[0]
    summary = f"{service or 'HANA service'} (PID {pid}) crashed with {sig_name or sig_num}. {reason}."

    return {
        "timestamp": ts or datetime.now(),
        "service_name": service or os.path.basename(path),
        "process_id": pid,
        "signal": sig_name,
        "signal_num": sig_num,
        "host": host, "sid": sid,
        "severity": "FATAL",
        "crash_reason": reason,
        "root_cause": root_cause,
        "summary": summary,
        "stack_trace": frames,
        "source_file": path,
    }


def parse_oom_event(path, text, host, sid):
    ts = None
    m = TIMESTAMP_RE.search(text)
    if m:
        for layout in ("%Y-%m-%d %H:%M:%S", "%Y-%m-%dT%H:%M:%S"):
            try:
                ts = datetime.strptime(m.group(1), layout)
                break
            except ValueError:
                continue
    used = free = limit = requested = 0
    for label, key in (("[Uu]sed", "used"), ("[Ff]ree", "free"), ("[Ll]imit", "limit"), ("[Rr]equested?", "requested")):
        m = MEM_RE(label).search(text)
        if m:
            val = parse_mem(m.group(1), m.group(2))
            if key == "used":
                used = val
            elif key == "free":
                free = val
            elif key == "limit":
                limit = val
            elif key == "requested":
                requested = val

    service = ""
    m = SERVICE_RE.search(text.lower())
    if m:
        service = m.group(1)
        if not service.startswith("hdb"):
            service = "hdb" + service

    consumers = []
    for line in text.splitlines():
        cm = re.match(r"^\s*(\S.{2,40}?)\s{2,}(\d+(?:\.\d+)?)\s*(KB|MB|GB|B)?", line)
        if cm and cm.group(1).lower() not in ("name", "---"):
            consumers.append({
                "name": cm.group(1).strip(),
                "used_bytes": parse_mem(cm.group(2), cm.group(3)),
            })
    total = sum(c["used_bytes"] for c in consumers) or 1
    for c in consumers:
        c["percentage"] = round(c["used_bytes"] / total * 100, 1)
    consumers.sort(key=lambda c: -c["used_bytes"])

    if used and limit:
        root_cause = (f"Memory exhausted at {used/limit*100:.1f}% of limit "
                       f"({fmt_bytes(used)} used of {fmt_bytes(limit)} limit). "
                       f"Process could not allocate {fmt_bytes(requested)} more.")
    else:
        root_cause = "HANA process ran out of available memory and could not satisfy an allocation request."

    return {
        "timestamp": ts or datetime.now(),
        "host": host, "sid": sid,
        "service_name": service or os.path.basename(path),
        "used_memory": used, "free_memory": free, "memory_limit": limit, "requested_size": requested,
        "root_cause": root_cause,
        "consumers": consumers[:10],
        "source_file": path,
    }


def parse_log_file(path, text, host):
    entries = []
    error_count = warn_count = 0
    for i, line in enumerate(text.splitlines(), 1):
        m = LOG_LINE_RE.match(line.strip())
        if not m:
            continue
        ts_str, sev, comp, msg = m.groups()
        sev = sev.upper()
        if sev in ("F", "FATAL"):
            sev = "FATAL"
        elif sev in ("E", "ERR", "ERROR"):
            sev = "ERROR"
        elif sev in ("W", "WARN", "WARNING"):
            sev = "WARNING"
        elif sev in ("D", "DEBUG"):
            sev = "DEBUG"
        else:
            sev = "INFO"

        ts = None
        for layout in ("%Y-%m-%d %H:%M:%S.%f", "%Y-%m-%d %H:%M:%S", "%Y-%m-%dT%H:%M:%S"):
            try:
                ts = datetime.strptime(ts_str, layout)
                break
            except ValueError:
                continue

        explanation = ""
        low = msg.lower()
        for pat, exp in ERROR_EXPLANATIONS.items():
            if pat in low:
                explanation = exp
                break

        if sev in ("ERROR", "FATAL"):
            error_count += 1
        elif sev == "WARNING":
            warn_count += 1

        entries.append({
            "timestamp": ts or datetime.now(), "severity": sev, "component": comp,
            "message": msg.strip(), "explanation": explanation, "line_num": i,
            "is_error": sev in ("ERROR", "FATAL"),
        })

    return {
        "filename": os.path.basename(path), "host": host,
        "entries": entries, "error_count": error_count, "warn_count": warn_count,
        "source_file": path,
    }


def scan_path(path):
    """Scan a single file or a directory. Accepts whatever the user types in."""
    if os.path.isfile(path):
        return scan_single_file(path)
    return scan_trace_dir(path)


def scan_single_file(path):
    host = socket.gethostname()
    sid = "HDB"
    report = {
        "generated_at": datetime.now(), "host": host, "sid": sid,
        "trace_dir": path,
        "crash_dumps": [], "oom_events": [], "log_files": [],
    }
    try:
        with open(path, "r", errors="ignore") as f:
            text = f.read(8 * 1024 * 1024)
    except OSError as e:
        report["scan_error"] = str(e)
        report["top_issues"] = []
        report["health_score"] = 0
        return report

    kind = classify_file(os.path.basename(path), text[:65536])
    if kind == "crash":
        report["crash_dumps"].append(parse_crash_dump(path, text, host, sid))
    elif kind == "oom":
        report["oom_events"].append(parse_oom_event(path, text, host, sid))
    else:
        lf = parse_log_file(path, text, host)
        if lf["entries"]:
            report["log_files"].append(lf)

    report["top_issues"] = derive_issues(report)
    report["health_score"] = compute_health_score(report)
    return report


def scan_trace_dir(trace_dir):
    host = socket.gethostname()
    sid = "HDB"
    for part in trace_dir.replace("\\", "/").split("/"):
        if len(part) == 3 and part.isupper() and part.isalnum():
            sid = part
            break

    report = {
        "generated_at": datetime.now(), "host": host, "sid": sid,
        "trace_dir": trace_dir,
        "crash_dumps": [], "oom_events": [], "log_files": [],
    }

    for root, _, files in os.walk(trace_dir):
        for name in files:
            if not is_candidate(name):
                continue
            full = os.path.join(root, name)
            try:
                if os.path.getsize(full) > 200 * 1024 * 1024:
                    continue
                with open(full, "r", errors="ignore") as f:
                    text = f.read(4 * 1024 * 1024)
            except OSError:
                continue

            kind = classify_file(name, text[:65536])
            if kind == "crash":
                report["crash_dumps"].append(parse_crash_dump(full, text, host, sid))
            elif kind == "oom":
                report["oom_events"].append(parse_oom_event(full, text, host, sid))
            elif kind == "log":
                lf = parse_log_file(full, text, host)
                if lf["entries"]:
                    report["log_files"].append(lf)

    report["top_issues"] = derive_issues(report)
    report["health_score"] = compute_health_score(report)
    return report


def crash_top_frame_location(cd):
    if cd["stack_trace"]:
        f = cd["stack_trace"][0]
        if f.get("file"):
            return f"{f['file']}:{f['line']} ({f['function']})"
        return f["function"]
    return "unknown frame (no stack trace parsed)"


def crash_prevention(cd):
    sig = cd.get("signal", "")
    base = [f"Apply the latest HANA revision/patch — {sig} crashes in this component are frequently fixed in later SPS/patch levels."]
    if sig == "SIGSEGV":
        base.append("If this is reproducible, disable the specific optimizer feature (e.g. parallel join) via configuration until patched.")
    elif sig == "SIGABRT":
        base.append("Run a consistency check (HANA's built-in check tables/check catalog) to rule out underlying data corruption.")
    elif sig == "SIGKILL":
        base.append("Treat this as a memory problem: see the OOM prevention steps below and check kernel OOM-killer logs (dmesg).")
    base.append("Set up automatic crash-dump forwarding/alerting so recurrences are caught immediately, not discovered later.")
    return base


def derive_issues(report):
    issues = []
    for i, cd in enumerate(report["crash_dumps"]):
        issues.append({
            "id": f"CRASH-{i+1:03d}", "severity": "FATAL", "source": "crash_dump",
            "timestamp": cd["timestamp"],
            "title": f"{cd['service_name']} crashed with {cd['signal'] or cd['signal_num']} (PID {cd['process_id']})",
            "description": cd["summary"], "root_cause": cd["root_cause"],
            "impact": "Database service interrupted; active sessions disconnected. Restart required.",
            "resolution": [
                f"Collect crash dump and trace files for PID {cd['process_id']} from {cd['source_file']}",
                "Search SAP Notes for known issues matching this signal/component.",
                "Open an SAP incident if the crash recurs.",
            ],
            "prevention": crash_prevention(cd),
            "location": f"{cd['service_name']} · {crash_top_frame_location(cd)}",
        })
    for i, oom in enumerate(report["oom_events"]):
        issues.append({
            "id": f"OOM-{i+1:03d}", "severity": "ERROR", "source": "oom_event",
            "timestamp": oom["timestamp"],
            "title": f"{oom['service_name']} out-of-memory event",
            "description": f"Process requested {fmt_bytes(oom['requested_size'])} but only "
                            f"{fmt_bytes(oom['free_memory'])} was free.",
            "root_cause": oom["root_cause"],
            "impact": "Service may have terminated or rejected allocations; sessions may be disconnected.",
            "resolution": [
                "Increase global_allocation_limit in indexserver.ini.",
                "Review top memory consumers (M_HEAP_MEMORY, M_EXPENSIVE_STATEMENTS).",
                "Consider adding RAM or partitioning large tables.",
            ],
            "prevention": [
                "Set a memory threshold alert (e.g. 85% of global_allocation_limit) so you get paged before exhaustion, not after.",
                "Cap per-statement memory with statement_memory_limit to stop one runaway query from starving the system.",
                "Right-size global_allocation_limit against actual physical RAM, leaving headroom for OS and other processes.",
                "Schedule regular review of M_EXPENSIVE_STATEMENTS to catch memory-heavy queries before they cause an OOM.",
            ],
            "location": f"{oom['service_name']} · top consumer: {oom['consumers'][0]['name'] if oom['consumers'] else 'unknown'}",
        })
    for lf in report["log_files"]:
        for e in lf["entries"]:
            if e["severity"] not in ("ERROR", "FATAL"):
                continue
            issues.append({
                "id": f"LOG-{lf['filename']}-L{e['line_num']}", "severity": e["severity"], "source": "log_file",
                "timestamp": e["timestamp"],
                "title": f"{e['component']}: {e['message'][:80]}",
                "description": e["message"],
                "root_cause": e["explanation"] or "No automatic explanation available — review manually.",
                "impact": f"See {lf['filename']} for context.",
                "resolution": [f"Review {lf['filename']} around line {e['line_num']}."],
                "prevention": ["Add a log-monitoring alert for this message pattern so recurrence is caught immediately."],
                "location": f"{e['component']} · {lf['filename']}:{e['line_num']}",
            })
    issues.sort(key=lambda i: i["timestamp"], reverse=True)
    return issues


def compute_health_score(report):
    score = 100
    score -= len(report["crash_dumps"]) * 20
    score -= len(report["oom_events"]) * 15
    for lf in report["log_files"]:
        score -= lf["error_count"] * 2
        score -= lf["warn_count"] * 1
    return max(0, min(100, score))
