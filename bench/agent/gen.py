#!/usr/bin/env python3
"""Generate the synthetic corpus for the GASH strategy benchmark (deterministic, seed 42).

Layout under <root>/corpus:
  proj/            git repo: src/ (~18k files), vendor/ (~8k), node_modules/ (~8k), lint/ (php/sh/js), history + dirty tree
  remote.git       bare remote for push tests
  logs/app.log     ~150 MB log
  data/big.json    ~25 MB JSON array; data/table.csv ~40 MB; data/db.sqlite 50k rows
"""

from __future__ import annotations

import json
import os
import random
import sqlite3
import subprocess
import sys

R = random.Random(42)
WORDS = ("alpha beta gamma delta epsilon order invoice customer payment shipping cart product stock price tax total "
         "user session token cache queue worker retry timeout config router controller model view service repository").split()
LEVELS = ["INFO"] * 80 + ["DEBUG"] * 12 + ["WARN"] * 6 + ["ERROR"] * 2


def text(n_lines: int, kind: str) -> str:
    out = []
    for i in range(n_lines):
        w = " ".join(R.choice(WORDS) for _ in range(R.randint(3, 12)))
        if kind == "php":
            out.append(f"    public function {R.choice(WORDS)}{i}(): string {{ return '{w}'; }}")
        elif kind == "js":
            out.append(f"export const {R.choice(WORDS)}{i} = () => '{w}';")
        else:
            out.append(w)
        if R.random() < 0.002:
            out.append("// TODO_MARKER refactor " + w)
    return "\n".join(out) + "\n"


def write_tree(base: str, n_files: int, n_dirs: int) -> None:
    exts = ["php"] * 50 + ["js"] * 25 + ["md"] * 10 + ["json"] * 10 + ["txt"] * 5
    for d in range(n_dirs):
        os.makedirs(os.path.join(base, f"mod{d:04d}"), exist_ok=True)
    for i in range(n_files):
        ext = R.choice(exts)
        p = os.path.join(base, f"mod{R.randrange(n_dirs):04d}", f"file{i:05d}.{ext}")
        if ext == "php":
            body = "<?php\n\nclass C%d\n{\n%s}\n" % (i, text(R.randint(20, 120), "php"))
        elif ext == "json":
            body = json.dumps({"id": i, "name": R.choice(WORDS), "items": [R.choice(WORDS) for _ in range(R.randint(5, 80))]}, indent=2)
        else:
            body = text(R.randint(20, 150), ext)
        with open(p, "w") as fh:
            fh.write(body)


def git(cwd: str, *args: str) -> None:
    subprocess.run(["git", *args], cwd=cwd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def main() -> int:
    root = os.path.abspath(sys.argv[1] if len(sys.argv) > 1 else os.path.join(os.path.dirname(__file__), ".."))
    c = os.path.join(root, "corpus")
    proj = os.path.join(c, "proj")
    if os.path.exists(proj):
        print("corpus exists, skip")
        return 0
    os.makedirs(proj)
    write_tree(os.path.join(proj, "src"), 18000, 1800)
    write_tree(os.path.join(proj, "vendor"), 8000, 600)
    write_tree(os.path.join(proj, "node_modules"), 8000, 900)
    lint = os.path.join(proj, "lint")
    os.makedirs(lint)
    for i in range(60):
        with open(os.path.join(lint, f"f{i:02d}.php"), "w") as fh:
            fh.write("<?php\n\ndeclare(strict_types=1);\n\nclass L%d\n{\n%s}\n" % (i, text(80, "php")))
        with open(os.path.join(lint, f"s{i:02d}.sh"), "w") as fh:
            fh.write("#!/usr/bin/env bash\nset -euo pipefail\n" + "".join(f"echo \"{R.choice(WORDS)} {j}\"\n" for j in range(80)))
        with open(os.path.join(lint, f"j{i:02d}.js"), "w") as fh:
            fh.write(text(80, "js"))
    # git history: bulk import in batches, then small commits
    git(proj, "init", "-q", "-b", "main")
    git(proj, "config", "user.name", "Maurizio Fonte")
    git(proj, "config", "user.email", "fonte.maurizio@gmail.com")
    with open(os.path.join(proj, ".gitignore"), "w") as fh:
        fh.write("node_modules/\n")
    git(proj, "add", ".gitignore", "lint")
    git(proj, "commit", "-q", "-m", "chore: init")
    for top in ("vendor", "src"):
        mods = sorted(os.listdir(os.path.join(proj, top)))
        for k in range(0, len(mods), 300):
            git(proj, "add", *[f"{top}/{m}" for m in mods[k:k + 300]])
            git(proj, "commit", "-q", "-m", f"feat: import {top} batch {k // 300}")
    for k in range(120):
        p = os.path.join(proj, "CHANGES.md")
        with open(p, "a") as fh:
            fh.write(f"- change {k}\n")
        git(proj, "add", "CHANGES.md")
        git(proj, "commit", "-q", "-m", f"docs: change {k}")
    subprocess.run(["git", "init", "-q", "--bare", os.path.join(c, "remote.git")], check=True)
    git(proj, "remote", "add", "origin", os.path.join(c, "remote.git"))
    git(proj, "push", "-q", "-u", "origin", "main")
    # dirty tree: 50 modified tracked, 10 untracked
    tracked = subprocess.run(["git", "ls-files", "src"], cwd=proj, capture_output=True, text=True).stdout.split()
    for p in R.sample(tracked, 50):
        with open(os.path.join(proj, p), "a") as fh:
            fh.write("// dirty\n")
    for i in range(10):
        with open(os.path.join(proj, f"untracked{i}.txt"), "w") as fh:
            fh.write("u\n")
    # log
    os.makedirs(os.path.join(c, "logs"))
    with open(os.path.join(c, "logs", "app.log"), "w") as fh:
        lines = []
        for i in range(1_400_000):
            lines.append(f"2026-10-25T{(i // 3600) % 24:02d}:{(i // 60) % 60:02d}:{i % 60:02d} {R.choice(LEVELS)} "
                         f"[{R.choice(WORDS)}] {' '.join(R.choice(WORDS) for _ in range(R.randint(8, 16)))} id={i}")
            if i % 1000 == 999:
                fh.write("\n".join(lines) + "\n")
                lines = []
        fh.write("\n".join(lines) + ("\n" if lines else ""))
        fh.write("2026-10-25T23:59:59 INFO [worker] RUN_FINISHED exit=0\n")
    # data
    os.makedirs(os.path.join(c, "data"))
    rows = [{"id": i, "name": f"{R.choice(WORDS)} {R.choice(WORDS)}", "email": f"u{i}@example.test", "city": R.choice(WORDS),
             "tags": [R.choice(WORDS) for _ in range(5)], "score": round(R.random() * 100, 2)} for i in range(160_000)]
    with open(os.path.join(c, "data", "big.json"), "w") as fh:
        json.dump(rows, fh)
    with open(os.path.join(c, "data", "table.csv"), "w") as fh:
        fh.write("id,name,email,city,score,note\n")
        for i in range(500_000):
            fh.write(f"{i},{R.choice(WORDS)} {R.choice(WORDS)},u{i}@example.test,{R.choice(WORDS)},{R.random() * 100:.2f},"
                     f"{' '.join(R.choice(WORDS) for _ in range(6))}\n")
    db = sqlite3.connect(os.path.join(c, "data", "db.sqlite"))
    db.execute("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT, city TEXT, score REAL, created_at TEXT)")
    db.executemany("INSERT INTO users VALUES (?,?,?,?,?,?)",
                   [(r["id"], r["name"], r["email"], r["city"], r["score"], f"2026-{1 + r['id'] % 12:02d}-{1 + r['id'] % 28:02d}")
                    for r in rows[:50_000]])
    db.commit()
    db.close()
    print("corpus ready:", c)
    return 0


if __name__ == "__main__":
    sys.exit(main())
