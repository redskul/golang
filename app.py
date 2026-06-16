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
import os
import threading
from datetime import datetime

from flask import Flask, render_template, request, redirect, url_for, jsonify

import scanner
import sample_data

app = Flask(__name__)
STATE = {"report": None, "trace_dir": "", "lock": threading.Lock(), "error": ""}


def rescan():
    with STATE["lock"]:
        if not STATE["trace_dir"]:
            if STATE["report"] is None:
                STATE["report"] = sample_data.generate()
            return
        try:
            STATE["report"] = scanner.scan_path(STATE["trace_dir"])
            STATE["error"] = ""
        except Exception as exc:
            STATE["error"] = f"Could not analyze '{STATE['trace_dir']}': {exc}"


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
    return render_template("index.html", report=report, trace_dir=STATE["trace_dir"],
                            error=STATE["error"], page="dashboard")


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
    new_path = request.form.get("path", "").strip()
    if new_path:
        if not os.path.exists(new_path):
            STATE["error"] = f"Path not found on this server: {new_path}"
            return redirect(url_for("index"))
        STATE["trace_dir"] = new_path
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
    parser.add_argument("--dir", default="", help="Path to a HANA trace file or directory to analyze. "
                                                    "Optional — you can instead type the path into the "
                                                    "'File or directory to analyze' box on the dashboard "
                                                    "once the server is running.")
    parser.add_argument("--port", type=int, default=5000)
    parser.add_argument("--host", default="127.0.0.1", help="Bind address (default 127.0.0.1, local-only).")
    args = parser.parse_args()

    if args.dir:
        if not os.path.exists(args.dir):
            print(f"ERROR: path does not exist: {args.dir}")
            raise SystemExit(1)
        STATE["trace_dir"] = args.dir

    rescan()

    mode = f"REAL DATA from {STATE['trace_dir']}" if STATE["trace_dir"] else "no file/directory analyzed yet"
    print(f"\nHANA Diagnostic Viewer ({mode})")
    print(f"  Open in your browser: http://localhost:{args.port}/")
    print("  Enter a trace file or directory path in the 'Analyze' box on that page.")
    if args.host == "127.0.0.1":
        print("  (bound to localhost only — use --host 0.0.0.0 to allow remote access)")

    app.run(host=args.host, port=args.port, debug=False)


if __name__ == "__main__":
    main()
