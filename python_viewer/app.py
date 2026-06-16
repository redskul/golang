#!/usr/bin/env python3
"""
SAP HANA Diagnostic Viewer (Python edition)

Scans a real HANA trace directory for crash dumps, OOM events, and trace
logs, parses them, and serves a graphical web dashboard (Flask + Chart.js)
explaining what happened, why, and where.

Usage:
    python3 app.py --dir /usr/sap/HDB/HDB00/trace
    python3 app.py                      # demo mode with sample data
"""
import argparse
import json
import threading
from datetime import datetime

from flask import Flask, render_template, request, redirect, url_for, jsonify

import scanner
import sample_data

app = Flask(__name__)
STATE = {"report": None, "trace_dir": "", "lock": threading.Lock()}


def rescan():
    with STATE["lock"]:
        if STATE["trace_dir"]:
            STATE["report"] = scanner.scan_trace_dir(STATE["trace_dir"])
        elif STATE["report"] is None:
            STATE["report"] = sample_data.generate()


def current_report():
    if STATE["report"] is None:
        rescan()
    return STATE["report"]


def fmt_time(dt):
    if isinstance(dt, str):
        return dt
    return dt.strftime("%Y-%m-%d %H:%M:%S")


app.jinja_env.filters["fmt_time"] = fmt_time
app.jinja_env.filters["fmt_bytes"] = scanner.fmt_bytes
app.jinja_env.filters["tojson_safe"] = lambda v: json.dumps(v, default=str)


def pct(used, total):
    return (used / total * 100) if total else 0


app.jinja_env.globals["pct"] = pct


@app.context_processor
def inject_now():
    return {"now": fmt_time(datetime.now())}


@app.route("/")
def index():
    report = current_report()
    return render_template("index.html", report=report, trace_dir=STATE["trace_dir"], page="dashboard")


@app.route("/crash/<int:idx>")
def crash_detail(idx):
    report = current_report()
    if idx < 0 or idx >= len(report["crash_dumps"]):
        return "Not found", 404
    return render_template("crash.html", crash=report["crash_dumps"][idx], idx=idx, page="crash")


@app.route("/oom/<int:idx>")
def oom_detail(idx):
    report = current_report()
    if idx < 0 or idx >= len(report["oom_events"]):
        return "Not found", 404
    return render_template("oom.html", oom=report["oom_events"][idx], idx=idx, page="oom")


@app.route("/logs")
def logs():
    report = current_report()
    severity = request.args.get("severity", "")
    search = request.args.get("q", "")
    entries = []
    for lf in report["log_files"]:
        for e in lf["entries"]:
            if severity and e["severity"] != severity:
                continue
            if search and search.lower() not in e["message"].lower():
                continue
            entries.append(e)
    return render_template("logs.html", log_files=report["log_files"], entries=entries,
                            severity=severity, search=search, page="logs")


@app.route("/api/report")
def api_report():
    return jsonify(json.loads(json.dumps(current_report(), default=str)))


@app.route("/api/rescan", methods=["POST"])
def api_rescan():
    rescan()
    return redirect(url_for("index"))


@app.route("/api/upload", methods=["POST"])
def api_upload():
    f = request.files.get("file")
    if not f:
        return "missing file", 400
    name = f.filename
    text = f.read().decode("utf-8", errors="ignore")
    host = STATE["trace_dir"] or "uploaded"
    sid = current_report().get("sid", "HDB")

    with STATE["lock"]:
        report = current_report()
        kind = scanner.classify_file(name, text[:65536])
        if kind == "crash" or "crash" in name.lower():
            report["crash_dumps"].append(scanner.parse_crash_dump(name, text, host, sid))
        elif kind == "oom" or "oom" in name.lower():
            report["oom_events"].append(scanner.parse_oom_event(name, text, host, sid))
        else:
            lf = scanner.parse_log_file(name, text, host)
            if lf["entries"]:
                report["log_files"].append(lf)
        report["top_issues"] = scanner.derive_issues(report)
        report["health_score"] = scanner.compute_health_score(report)

    return redirect(url_for("index"))


def main():
    parser = argparse.ArgumentParser(description="SAP HANA Diagnostic Viewer")
    parser.add_argument("--dir", default="", help="HANA trace directory to scan (real data). "
                                                    "If omitted, runs in demo mode.")
    parser.add_argument("--port", type=int, default=5000)
    args = parser.parse_args()

    STATE["trace_dir"] = args.dir
    rescan()

    mode = f"REAL DATA from {args.dir}" if args.dir else "DEMO MODE (sample data)"
    print(f"HANA Diagnostic Viewer ({mode})")
    print(f"  Dashboard : http://localhost:{args.port}/")
    print(f"  Log Viewer: http://localhost:{args.port}/logs")
    print(f"  JSON API  : http://localhost:{args.port}/api/report")

    app.run(host="0.0.0.0", port=args.port, debug=False)


if __name__ == "__main__":
    main()
