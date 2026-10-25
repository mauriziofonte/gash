#!/usr/bin/env python3
"""agentbench: measure agent Bash strategies against a gash binary, with relative acceptance rules.

Usage:
  GASH_BENCH_BIN=/path/to/gash python3 -I agentbench.py --root DIR [case ...]
  DIR holds corpus/ (made by gen.py). Use a disk-backed directory (check `df -T DIR`), not tmpfs.

Variant kinds:
  baseline = the raw shape agents run today (measured in telemetry)
  current  = what gash offers today
  target   = the function/flag specified by the plan (reported "not implemented" until it exists)
  ideal    = best plain-shell form, for reference
A variant is skipped when a required tool is missing. Acceptance rules compare a target with a baseline
measured in the same run on the same machine, so they hold on any hardware.
Output: <root>/results/agentbench.json and <root>/results/AGENTBENCH.md
"""

from __future__ import annotations

import argparse
import http.server
import json
import os
import shutil
import statistics
import subprocess
import sys
import threading
import time

PORT = 18765
DOMAINS = "example.com wikipedia.org github.com cloudflare.com debian.org python.org mozilla.org kernel.org"
LINT20 = " ".join(f"proj/lint/j{i:02d}.js" for i in range(20))
STAT_PATHS = ("proj proj/CHANGES.md proj/lint proj/lint/f00.php proj/src/mod0001 data/db.sqlite data/big.json logs/app.log "
              "proj/.gitignore proj/missing.txt proj/vendor proj/node_modules")
SQL = "SELECT id,name,email,city,score FROM users"
URLS = " ".join(f"http://127.0.0.1:{PORT}/p{i}" for i in range(1, 21))
NOT_IMPL_MARKERS = ("unknown command", "unknown flag", "unknown option", "unknown_option", "not found", "invalid_option",
                    "unknown shorthand", "Unknown", "unrecognized")

# (case, intent, cold paths, prepare, variants[(label, kind, cmd, requires)])
CASES = [
    ("count-files", "count-files", ["proj/src"], None, [
        ("find | wc -l", "baseline", "find proj/src -type f | wc -l", []),
        ("llm_tree --stats --depth 1", "current", "$G llm_tree --stats --depth 1 proj/src", []),
        ("llm_find --count", "target", "$G llm_find '*' proj/src --type f --count", []),
    ]),
    ("disk-usage", "disk-usage", ["proj"], None, [
        ("du -sh */ | sort -rh | head", "baseline", "du -sh proj/*/ | sort -rh | head", []),
        ("llm_tree --stats --depth 1", "current", "$G llm_tree --stats --depth 1 proj", []),
        ("llm_tree --top 10", "target", "$G llm_tree --top 10 proj", []),
    ]),
    ("find-sorted", "list-tree-sorted", ["proj/src"], None, [
        ("find -name | sort | head -50", "baseline", "find proj/src -type f -name '*.php' | sort | head -50", []),
        ("llm_find -n 50", "current", "$G llm_find '*.php' proj/src -n 50", []),
        ("llm_find -n 50 --relative", "target", "$G llm_find '*.php' proj/src -n 50 --relative", []),
    ]),
    ("grep-recursive", "grep-recursive-exclude", ["proj"], None, [
        ("grep -rn --include | grep -v vendor | head", "baseline", "grep -rn TODO_MARKER proj --include=*.php | grep -v vendor | head -20", []),
        ("rg -n -g", "ideal", "rg -n TODO_MARKER -g '*.php' -g '!vendor' proj | head -20", ["rg"]),
        ("llm_grep --ext php -n 20", "current", "$G llm_grep TODO_MARKER proj --ext php -n 20", []),
        ("llm_grep --ext php -n 20 --relative", "target", "$G llm_grep TODO_MARKER proj --ext php -n 20 --relative", []),
    ]),
    ("multi-count", "grep-count", ["logs/app.log"], None, [
        ("6x grep -c", "baseline", "for p in ERROR WARN RUN_FINISHED payment timeout 'id=999999'; do grep -c \"$p\" logs/app.log; done", []),
        ("llm_count (1 pass)", "target",
         "$G llm_count logs/app.log --pat ERROR --pat WARN --pat RUN_FINISHED --pat payment --pat timeout --pat id=999999", []),
    ]),
    ("last-matches", "tail-log", ["logs/app.log"], None, [
        ("grep | tail -20", "baseline", "grep ERROR logs/app.log | tail -20", []),
        ("tac | grep -m20", "ideal", "tac logs/app.log | grep -m20 ERROR", ["tac"]),
        ("llm_log --last-match", "target", "$G llm_log logs/app.log --last-match ERROR -n 20", []),
    ]),
    ("wait-file-match", "poll-sleep-loop", None, None, [
        ("while ! grep -q; do sleep 5", "baseline",
         "M=MARK_$$_$RANDOM; (sleep 2; echo $M >> logs/app.log) & while ! grep -q $M logs/app.log; do sleep 5; done; wait", []),
        ("llm_wait --file-match", "target",
         "M=MARK_$$_$RANDOM; (sleep 2; echo $M >> logs/app.log) & $G llm_wait --timeout 30 --file-match logs/app.log $M; r=$?; wait; exit $r", []),
    ]),
    ("stat-12-paths", "stat-exists", None, None, [
        ("ls -lad loop + test + wc -l", "baseline",
         "for p in " + STAT_PATHS + "; do ls -lad \"$p\" 2>&1; done; [ -f proj/missing.txt ] && echo yes || echo no; wc -l proj/CHANGES.md", []),
        ("llm_stat --lines", "target", "$G llm_stat --lines " + STAT_PATHS, []),
    ]),
    ("read-around", "range-read", None, None, [
        ("grep -n then sed -n (2 calls)", "baseline",
         "n=$(grep -n -m1 TODO_MARKER proj/lint/f01.php | cut -d: -f1); sed -n \"$((n-5)),$((n+5))p\" proj/lint/f01.php", []),
        ("llm_read --around", "target", "$G llm_read proj/lint/f01.php --around TODO_MARKER -C 5", []),
    ]),
    ("lint-php-60", "syntax-check", None, None, [
        ("for f; do php -l; done", "baseline", "for f in proj/lint/f*.php; do php -l \"$f\"; done", ["php"]),
        ("php -l (all files)", "ideal", "php -l proj/lint/f*.php | grep -v '^No syntax'; true", ["php"]),
        ("llm_lint --ext php", "target", "$G llm_lint proj/lint --ext php", ["php"]),
    ]),
    ("lint-sh-60", "syntax-check", None, None, [
        ("for f; do bash -n; done", "baseline", "for f in proj/lint/s*.sh; do bash -n \"$f\"; done", []),
        ("llm_lint --ext sh", "target", "$G llm_lint proj/lint --ext sh", []),
    ]),
    ("lint-js-60", "syntax-check", None, None, [
        ("for f; do node --check; done", "baseline", "for f in proj/lint/j*.js; do node --check \"$f\"; done", ["node"]),
        ("llm_lint --ext js", "target", "$G llm_lint proj/lint --ext js", ["node"]),
    ]),
    ("json-shape", "json-file-inspect", ["data/big.json"], None, [
        ("python3 json.load + keys", "baseline",
         "python3 -I -c \"import json;d=json.load(open('data/big.json'));print(len(d));print(sorted(d[0]))\"", []),
        ("jq length, keys", "ideal", "jq -c 'length, (.[0]|keys)' data/big.json", ["jq"]),
        ("llm_json --shape", "target", "$G llm_json data/big.json --shape", []),
    ]),
    ("csv-shape", "csv-inspect", ["data/table.csv"], None, [
        ("python3 csv list()", "baseline",
         "python3 -I -c \"import csv;r=list(csv.reader(open('data/table.csv')));print(len(r)-1, r[0]);print(r[1:3])\"", []),
        ("llm_csv --head 2 --count", "target", "$G llm_csv data/table.csv --head 2 --count", []),
    ]),
    ("db-100-rows", "db-query-reshape", None, None, [
        ("llm_db_query | python3 -> tsv", "baseline",
         f"$G llm_db_query '{SQL}' -f data/db.sqlite -n 100 | python3 -I -c \"import json,sys;d=json.load(sys.stdin)['data'];"
         "print('\\t'.join(d[0]));[print('\\t'.join(map(str,r.values()))) for r in d]\"", []),
        ("sqlite3 -tabs -header", "ideal", f"sqlite3 -readonly -tabs -header data/db.sqlite '{SQL} LIMIT 100'", ["sqlite3"]),
        ("llm_db_query (envelope)", "current", f"$G llm_db_query '{SQL}' -f data/db.sqlite -n 100", []),
        ("llm_db_query --format tsv", "target", f"$G llm_db_query '{SQL}' -f data/db.sqlite -n 100 --format tsv", []),
    ]),
    ("replace-20-files", "replace-python", None, "cp -p pristine/j*.js proj/lint/", [
        ("20x python3 heredoc replace+assert", "baseline",
         "for f in " + LINT20 + "; do python3 -I - \"$f\" <<'EOF'\nimport sys\np=sys.argv[1]\ns=open(p).read()\nassert s.count('export const')>=1\n"
         "open(p,'w').write(s.replace('export const','export let'))\nEOF\ndone", []),
        ("20x sed -i", "ideal", "for f in " + LINT20 + "; do sed -i 's/export const/export let/g' \"$f\"; done", []),
        ("agent_replace", "target", "$G agent_replace " + LINT20 + " --old 'export const' --new 'export let' --min 1", []),
    ]),
    ("git-overview", "git-status", ["proj/.git"], None, [
        ("status|wc + log -3 + diff --stat|tail", "baseline", "cd proj && git status --short | wc -l; git log --oneline -3; git diff --stat | tail -1", []),
        ("llm_git_status + log + diff", "current", "$G llm_git_status proj; $G llm_git_log -n 3 proj; $G llm_git_diff proj", []),
        ("llm_git_overview", "target", "$G llm_git_overview proj", []),
    ]),
    ("git-status", "git-status", ["proj/.git"], None, [
        ("git status --short", "baseline", "git -C proj status --short", []),
        ("llm_git_status", "current", "$G llm_git_status proj", []),
    ]),
    ("git-show-stat", "git-show-diff", None, None, [
        ("git show --stat", "baseline", "git -C proj show --stat HEAD~1", []),
        ("llm_git_show --stat", "target", "$G llm_git_show HEAD~1 --stat proj", []),
    ]),
    ("git-commit-push", "git-commit-push", None, "echo x >> proj/CHANGES.md", [
        ("add && commit && push && log -1", "baseline", "cd proj && git add CHANGES.md && git commit -q -m 'docs: bench' && git push -q && git log --oneline -1", []),
        ("agent_git_commit --push", "target", "$G agent_git_commit -m 'docs: bench' --add CHANGES.md --push proj", []),
    ]),
    ("http-20", "http-status", None, None, [
        ("for u; do curl -w http_code; done", "baseline", "for u in " + URLS + "; do curl -s -o /dev/null -w \"%{http_code} $u\\n\" \"$u\"; done", ["curl"]),
        ("xargs -P20 curl", "ideal", "printf '%s\\n' " + URLS + " | xargs -P 20 -I{} curl -s -o /dev/null -w '%{http_code} {}\\n' {}", ["curl"]),
        ("llm_http --status-only", "target", "$G llm_http --status-only " + URLS, []),
    ]),
    ("dns-8x3", "dns-lookup", None, None, [
        ("for d; for t; dig +short", "baseline", f"for d in {DOMAINS}; do for t in A MX TXT; do dig +short $t $d; done; done", ["dig"]),
        ("llm_dns --type A,MX,TXT", "target", f"$G llm_dns --type A,MX,TXT {DOMAINS}", []),
    ]),
    ("tool-versions", "tool-version", None, None, [
        ("separate --version probes", "baseline", "git --version; python3 --version; bash --version | head -1; curl --version | head -1; go version 2>/dev/null; true", []),
        ("llm_tools", "target", "$G llm_tools git python3 bash curl go", []),
    ]),
    ("sysinfo", "remote-sysinfo", None, None, [
        ("uptime; free; df; nproc", "baseline", "uptime; free -h; df -h /; nproc; nvidia-smi --query-gpu=name,memory.used,memory.total --format=csv,noheader 2>/dev/null; true", []),
        ("llm_sysinfo", "target", "$G llm_sysinfo", []),
    ]),
    ("textstats", "text-stats", None, None, [
        ("wc -w; wc -c; gzip -c | wc -c", "baseline", "wc -w proj/CHANGES.md; wc -c proj/CHANGES.md; gzip -c proj/CHANGES.md | wc -c", []),
        ("llm_textstats", "target", "$G llm_textstats proj/CHANGES.md", []),
    ]),
    ("hash-300", "hash-compare", None, None, [
        ("sha256sum (1 process)", "baseline", "sha256sum proj/src/mod000*/*", []),
        ("llm_hash", "target", "$G llm_hash proj/src/mod000*/*", []),
    ]),
    ("startup", "overhead", None, None, [
        ("bash -c true", "ideal", "true", []),
        ("python3 -I -c pass", "ideal", "python3 -I -c pass", []),
        ("gash version", "current", "$G version", []),
        ("llm_env --filter HOME", "current", "$G llm_env --filter '^HOME$'", []),
    ]),
]

# (case, target label, baseline label, max time ratio target/baseline or None, max output bytes or None, extra)
# extra: "abs_s=X" absolute ceiling in seconds (used only where the baseline is a scripted delay)
ACCEPT = [
    ("count-files", "llm_find --count", "find | wc -l", 1.5, 200, ""),
    ("disk-usage", "llm_tree --top 10", "du -sh */ | sort -rh | head", 1.2, 300, ""),
    ("find-sorted", "llm_find -n 50 --relative", "find -name | sort | head -50", 0.5, "1.1x", ""),
    ("grep-recursive", "llm_grep --ext php -n 20 --relative", "grep -rn --include | grep -v vendor | head", 0.4, "1.1x", "cold"),
    ("multi-count", "llm_count (1 pass)", "6x grep -c", 1.0, None, ""),
    ("last-matches", "llm_log --last-match", "grep | tail -20", 0.25, "1.1x", ""),
    ("wait-file-match", "llm_wait --file-match", "while ! grep -q; do sleep 5", None, None, "abs_s=2.3"),
    ("stat-12-paths", "llm_stat --lines", "ls -lad loop + test + wc -l", 0.7, "1.3x", ""),
    ("read-around", "llm_read --around", "grep -n then sed -n (2 calls)", 2.0, "1.5x", ""),
    ("lint-php-60", "llm_lint --ext php", "for f; do php -l; done", 0.15, 300, ""),
    ("lint-sh-60", "llm_lint --ext sh", "for f; do bash -n; done", 0.5, 300, ""),
    ("lint-js-60", "llm_lint --ext js", "for f; do node --check; done", 0.3, 300, ""),
    ("json-shape", "llm_json --shape", "python3 json.load + keys", 1.0, 500, ""),
    ("csv-shape", "llm_csv --head 2 --count", "python3 csv list()", 0.2, 600, ""),
    ("db-100-rows", "llm_db_query --format tsv", "llm_db_query | python3 -> tsv", 0.5, "1.05x", ""),
    ("replace-20-files", "agent_replace", "20x python3 heredoc replace+assert", 0.1, 1000, ""),
    ("git-overview", "llm_git_overview", "status|wc + log -3 + diff --stat|tail", 1.5, 400, ""),
    ("git-show-stat", "llm_git_show --stat", "git show --stat", 1.5, "1.0x", ""),
    ("git-commit-push", "agent_git_commit --push", "add && commit && push && log -1", 1.3, 150, ""),
    ("http-20", "llm_http --status-only", "for u; do curl -w http_code; done", 0.1, None, ""),
    ("dns-8x3", "llm_dns --type A,MX,TXT", "for d; for t; dig +short", 0.35, "0.3x", ""),
    ("tool-versions", "llm_tools", "separate --version probes", 1.0, None, ""),
    ("sysinfo", "llm_sysinfo", "uptime; free; df; nproc", 0.5, 250, ""),
    ("textstats", "llm_textstats", "wc -w; wc -c; gzip -c | wc -c", 1.0, 200, ""),
    ("hash-300", "llm_hash", "sha256sum (1 process)", 1.0, None, ""),
]


def evict(root: str, paths: list[str]) -> None:
    for base in paths:
        p = os.path.join(root, "corpus", base)
        items = [p] if os.path.isfile(p) else [os.path.join(d, f) for d, _x, fs in os.walk(p) for f in fs]
        for fp in items:
            try:
                fd = os.open(fp, os.O_RDONLY)
                os.posix_fadvise(fd, 0, 0, os.POSIX_FADV_DONTNEED)
                os.close(fd)
            except OSError:
                pass


class Slow(http.server.BaseHTTPRequestHandler):
    def do_GET(self):  # noqa: N802
        time.sleep(0.1)  # simulated RTT + server time
        self.send_response(200)
        self.send_header("Content-Length", "3")
        self.end_headers()
        self.wfile.write(b"ok\n")

    def log_message(self, *a):
        pass


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", required=True)
    ap.add_argument("--runs", type=int, default=5)
    ap.add_argument("cases", nargs="*")
    a = ap.parse_args()
    root = os.path.abspath(a.root)
    corpus = os.path.join(root, "corpus")
    gbin = os.environ.get("GASH_BENCH_BIN", "")
    if not (gbin and os.access(gbin, os.X_OK)):
        print("set GASH_BENCH_BIN to an executable gash binary (go build -o <dir>/gash ./cmd/gash)", file=sys.stderr)
        return 2
    env = dict(os.environ, G=gbin, LC_ALL="C.UTF-8")
    pristine = os.path.join(corpus, "pristine")
    if not os.path.isdir(pristine):
        os.makedirs(pristine)
        for i in range(60):
            shutil.copy2(os.path.join(corpus, "proj", "lint", f"j{i:02d}.js"), pristine)
    http.server.ThreadingHTTPServer.request_queue_size = 64  # default 5 causes SYN retransmits under 20 parallel connects
    srv = http.server.ThreadingHTTPServer(("127.0.0.1", PORT), Slow)
    threading.Thread(target=srv.serve_forever, daemon=True).start()

    def run(cmd: str) -> tuple[float, int, int, str]:
        t0 = time.perf_counter()
        p = subprocess.run(["bash", "-c", cmd], cwd=corpus, env=env, capture_output=True)
        return time.perf_counter() - t0, len(p.stdout) + len(p.stderr), p.returncode, p.stderr.decode("utf-8", "replace")[:400]

    results = []
    for cid, intent, cold, prep, variants in CASES:
        if a.cases and cid not in a.cases:
            continue
        for label, kind, cmd, req in variants:
            r = {"case": cid, "intent": intent, "label": label, "kind": kind, "cmd": cmd}
            missing = [t for t in req if not shutil.which(t)]
            if missing:
                r["status"] = "skipped: missing " + ",".join(missing)
                results.append(r)
                print(f"{cid:18} {label:40} {r['status']}", flush=True)
                continue
            if prep:
                run(prep)
            dt, ob, rc, err = run(cmd)
            if kind == "target" and rc != 0 and any(m in err for m in NOT_IMPL_MARKERS):
                r["status"] = "not implemented"
                results.append(r)
                print(f"{cid:18} {label:40} not implemented", flush=True)
                continue
            r.update(status="ok" if rc == 0 else f"rc={rc}", out_bytes=ob, rc=rc)
            n = a.runs if dt < 3 else 2
            ts = []
            for _ in range(n):
                if prep:
                    run(prep)
                ts.append(run(cmd)[0])
            r["warm_s"] = statistics.median(ts)
            if cold:
                cs = []
                for _ in range(3):
                    if prep:
                        run(prep)
                    evict(root, cold)
                    cs.append(run(cmd)[0])
                r["cold_s"] = statistics.median(cs)
            if prep:
                run(prep)
            results.append(r)
            print(f"{cid:18} {label:40} warm {r['warm_s'] * 1000:9.1f} ms" + (f"  cold {r['cold_s'] * 1000:9.1f} ms" if cold else "")
                  + f"  out {ob:7d} B  {r['status']}", flush=True)
    srv.shutdown()
    verdicts = evaluate(results)
    out = os.path.join(root, "results")
    os.makedirs(out, exist_ok=True)
    json.dump({"results": results, "verdicts": verdicts}, open(os.path.join(out, "agentbench.json"), "w"), indent=1)
    write_md(results, verdicts, os.path.join(out, "AGENTBENCH.md"))
    print("\n".join(f"{v['case']:18} {v['verdict']:16} {v['detail']}" for v in verdicts))
    return 0


def evaluate(results: list[dict]) -> list[dict]:
    idx = {(r["case"], r["label"]): r for r in results}
    out = []
    for cid, tl, bl, tmax, omax, extra in ACCEPT:
        t, b = idx.get((cid, tl)), idx.get((cid, bl))
        if t is None or b is None:
            continue
        if t.get("status") != "ok" or b.get("status") != "ok":
            out.append({"case": cid, "verdict": "NA", "detail": f"target {t.get('status')}, baseline {b.get('status')}"})
            continue
        key = "cold_s" if extra == "cold" and "cold_s" in t else "warm_s"
        notes, ok = [], True
        if tmax is not None:
            ratio = t[key] / b[key] if b[key] else 0
            ok &= ratio <= tmax
            notes.append(f"time {ratio:.2f}x (max {tmax}x, {key})")
        if extra.startswith("abs_s="):
            lim = float(extra.split("=")[1])
            ok &= t["warm_s"] <= lim
            notes.append(f"time {t['warm_s']:.2f}s (max {lim}s; baseline {b['warm_s']:.2f}s)")
        if omax is not None:
            lim = b["out_bytes"] * float(omax[:-1]) if isinstance(omax, str) else omax
            ok &= t["out_bytes"] <= lim
            notes.append(f"out {t['out_bytes']} B (max {int(lim)} B)")
        out.append({"case": cid, "verdict": "PASS" if ok else "FAIL", "detail": "; ".join(notes)})
    return out


def write_md(rs: list[dict], vs: list[dict], path: str) -> None:
    L = ["# agentbench", "", "| case | variant | kind | warm ms | cold ms | out B | status |", "|---|---|---|---|---|---|---|"]
    for r in rs:
        w = f"{r['warm_s'] * 1000:.1f}" if "warm_s" in r else ""
        c = f"{r['cold_s'] * 1000:.1f}" if "cold_s" in r else ""
        L.append(f"| {r['case']} | {r['label'].replace('|', '/')} | {r['kind']} | {w} | {c} | {r.get('out_bytes', '')} | {r['status']} |")
    L += ["", "| case | verdict | detail |", "|---|---|---|"]
    L += [f"| {v['case']} | {v['verdict']} | {v['detail']} |" for v in vs]
    open(path, "w").write("\n".join(L) + "\n")


if __name__ == "__main__":
    sys.exit(main())
