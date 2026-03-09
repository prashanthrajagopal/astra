#!/usr/bin/env python3
import argparse
import concurrent.futures
import datetime as dt
import json
import math
import os
import statistics
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path


def http_request(method, url, payload=None, headers=None, timeout=8.0):
    data = None
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url=url, data=data, method=method)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    start = time.perf_counter()
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
            latency_ms = (time.perf_counter() - start) * 1000
            if raw:
                try:
                    body = json.loads(raw)
                except Exception:
                    body = {"raw": raw}
            else:
                body = {}
            return resp.status, body, latency_ms, None
    except urllib.error.HTTPError as e:
        latency_ms = (time.perf_counter() - start) * 1000
        try:
            body = json.loads(e.read().decode("utf-8"))
        except Exception:
            body = {"error": str(e)}
        return e.code, body, latency_ms, None
    except Exception as e:
        latency_ms = (time.perf_counter() - start) * 1000
        return 0, {}, latency_ms, str(e)


def percentile(values, p):
    if not values:
        return 0.0
    ordered = sorted(values)
    k = (len(ordered) - 1) * (p / 100.0)
    f = math.floor(k)
    c = math.ceil(k)
    if f == c:
        return float(ordered[int(k)])
    d0 = ordered[f] * (c - k)
    d1 = ordered[c] * (k - f)
    return float(d0 + d1)


def parse_vm_stats():
    # macOS vm_stat output parsing
    try:
        out = subprocess.check_output(["vm_stat"], text=True)
        page_size = 4096
        for line in out.splitlines():
            if "page size of" in line:
                parts = line.split()
                for i, t in enumerate(parts):
                    if t == "of" and i + 1 < len(parts):
                        page_size = int(parts[i + 1])
                        break
        fields = {}
        for line in out.splitlines():
            if ":" not in line:
                continue
            k, v = line.split(":", 1)
            k = k.strip().replace(".", "")
            v = v.strip().replace(".", "")
            try:
                fields[k] = int(v)
            except ValueError:
                pass
        free_pages = fields.get("Pages free", 0) + fields.get("Pages inactive", 0)
        wired_pages = fields.get("Pages wired down", 0)
        free_mb = (free_pages * page_size) / (1024 * 1024)
        wired_mb = (wired_pages * page_size) / (1024 * 1024)
        return {"free_mb": round(free_mb, 1), "wired_mb": round(wired_mb, 1)}
    except Exception:
        return {"free_mb": -1, "wired_mb": -1}


def spawn_workers(worker_count, repo_root):
    workers = []
    logs_dir = repo_root / "logs"
    logs_dir.mkdir(parents=True, exist_ok=True)
    env = os.environ.copy()
    env["TOOL_RUNTIME"] = env.get("TOOL_RUNTIME", "noop")
    for i in range(worker_count):
        log_path = logs_dir / f"stress-worker-{i+1}.log"
        fh = open(log_path, "a", buffering=1)
        p = subprocess.Popen(
            [str(repo_root / "bin" / "execution-worker")],
            cwd=str(repo_root),
            stdout=fh,
            stderr=subprocess.STDOUT,
            env=env,
        )
        workers.append((p, fh, log_path))
        if (i + 1) % 50 == 0:
            time.sleep(0.15)
    return workers


def stop_workers(workers):
    for p, fh, _ in workers:
        try:
            p.terminate()
        except Exception:
            pass
    time.sleep(1)
    for p, fh, _ in workers:
        try:
            if p.poll() is None:
                p.kill()
        except Exception:
            pass
        try:
            fh.close()
        except Exception:
            pass


def ensure_binaries(repo_root):
    bin_worker = repo_root / "bin" / "execution-worker"
    if not bin_worker.exists():
        print("execution-worker binary missing; building...")
        subprocess.check_call(["go", "build", "-o", str(bin_worker), "./cmd/execution-worker"], cwd=str(repo_root))


def run_load(kind, total, concurrency, fn):
    lat = []
    ok = 0
    fail = 0
    results = []
    lock = threading.Lock()

    def wrapped(idx):
        nonlocal ok, fail
        r = fn(idx)
        with lock:
            lat.append(r.get("latency_ms", 0.0))
            if r.get("ok"):
                ok += 1
            else:
                fail += 1
            results.append(r)

    start = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as ex:
        futs = [ex.submit(wrapped, i) for i in range(total)]
        for _ in concurrent.futures.as_completed(futs):
            pass
    elapsed = time.perf_counter() - start
    return {
        "kind": kind,
        "total": total,
        "ok": ok,
        "fail": fail,
        "elapsed_s": round(elapsed, 3),
        "rps": round(ok / elapsed, 2) if elapsed > 0 else 0,
        "latency_ms": {
            "p50": round(percentile(lat, 50), 2),
            "p95": round(percentile(lat, 95), 2),
            "p99": round(percentile(lat, 99), 2),
            "mean": round(statistics.mean(lat), 2) if lat else 0,
        },
        "results": results,
    }


def main():
    parser = argparse.ArgumentParser(description="Astra stress test")
    parser.add_argument("--base-url", default="http://localhost:8080")
    parser.add_argument("--identity-url", default="http://localhost:8085")
    parser.add_argument("--worker-manager-url", default="http://localhost:8082")
    parser.add_argument("--access-url", default="http://localhost:8086")
    parser.add_argument("--agents", type=int, default=2000)
    parser.add_argument("--goals", type=int, default=1200)
    parser.add_argument("--workers", type=int, default=300)
    parser.add_argument("--concurrency", type=int, default=80)
    parser.add_argument("--monitor-seconds", type=int, default=90)
    parser.add_argument("--monitor-interval", type=int, default=5)
    parser.add_argument("--skip-worker-fanout", action="store_true")
    args = parser.parse_args()

    repo_root = Path(__file__).resolve().parents[1]
    out_dir = repo_root / "tests" / "load" / "results"
    out_dir.mkdir(parents=True, exist_ok=True)

    print("[1/6] Acquiring JWT token...")
    status, body, _, err = http_request(
        "POST",
        f"{args.identity_url}/tokens",
        payload={"subject": "stress-test", "scopes": ["admin"], "ttl_seconds": 7200},
        headers={"Content-Type": "application/json"},
        timeout=5,
    )
    if err or status >= 300 or "token" not in body:
        print("Failed to acquire token", status, err, body)
        sys.exit(1)
    token = body["token"]
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}

    print("[2/6] Creating agents under load...")
    def create_agent(_):
        st, b, l, e = http_request("POST", f"{args.base_url}/agents", payload={"actor_type": "stress-agent", "config": "{}"}, headers=headers)
        return {"ok": st in (200, 201) and "actor_id" in b, "actor_id": b.get("actor_id"), "latency_ms": l, "status": st, "error": e}

    agent_run = run_load("create_agents", args.agents, args.concurrency, create_agent)
    agent_ids = [r["actor_id"] for r in agent_run["results"] if r.get("ok") and r.get("actor_id")]
    if not agent_ids:
        print("No agents created successfully. Aborting.")
        sys.exit(1)

    goal_count = min(args.goals, len(agent_ids))
    selected_agents = agent_ids[:goal_count]

    print("[3/6] Submitting goals under load...")
    def submit_goal(i):
        aid = selected_agents[i]
        st, b, l, e = http_request(
            "POST",
            f"{args.base_url}/agents/{aid}/goals",
            payload={"goal_text": f"stress-goal-{i}"},
            headers=headers,
            timeout=8,
        )
        return {"ok": st in (200, 201) and b.get("status") == "ok", "latency_ms": l, "status": st, "error": e}

    goal_run = run_load("submit_goals", goal_count, args.concurrency, submit_goal)

    ensure_binaries(repo_root)
    workers = []
    if not args.skip_worker_fanout:
        print(f"[4/6] Launching extra execution workers: target={args.workers}...")
        workers = spawn_workers(args.workers, repo_root)
        time.sleep(3)

    print("[5/6] Monitoring runtime health/capacity...")
    monitor = []
    started = time.time()
    while time.time() - started < args.monitor_seconds:
        ts = dt.datetime.utcnow().isoformat() + "Z"
        _, workers_body, _, _ = http_request("GET", f"{args.worker_manager_url}/workers", timeout=3)
        _, approvals_body, _, _ = http_request("GET", f"{args.access_url}/approvals/pending", timeout=3)
        hs, _, l_health, e_health = http_request("GET", f"{args.base_url}/health", timeout=2)
        load1, load5, load15 = os.getloadavg()
        mem = parse_vm_stats()
        monitor.append(
            {
                "ts": ts,
                "workers": len(workers_body) if isinstance(workers_body, list) else -1,
                "pending_approvals": len(approvals_body) if isinstance(approvals_body, list) else (0 if approvals_body is None else -1),
                "api_health_status": hs,
                "api_health_latency_ms": round(l_health, 2),
                "api_health_error": e_health,
                "loadavg": [round(load1, 2), round(load5, 2), round(load15, 2)],
                "mem": mem,
            }
        )
        time.sleep(args.monitor_interval)

    print("[6/6] Cleaning up spawned workers...")
    stop_workers(workers)

    final_workers = monitor[-1]["workers"] if monitor else -1
    max_workers_observed = max((m["workers"] for m in monitor), default=-1)
    max_load1 = max((m["loadavg"][0] for m in monitor), default=0)
    min_free_mem = min((m["mem"].get("free_mb", -1) for m in monitor if m["mem"].get("free_mb", -1) >= 0), default=-1)

    summary = {
        "timestamp": dt.datetime.utcnow().isoformat() + "Z",
        "machine": {
            "platform": sys.platform,
            "cpu_count": os.cpu_count(),
        },
        "config": vars(args),
        "agent_run": {k: v for k, v in agent_run.items() if k != "results"},
        "goal_run": {k: v for k, v in goal_run.items() if k != "results"},
        "monitor": {
            "samples": len(monitor),
            "max_workers_observed": max_workers_observed,
            "final_workers_observed": final_workers,
            "max_load1": max_load1,
            "min_free_mem_mb": min_free_mem,
        },
        "monitor_samples": monitor,
        "notes": [
            "Workers observed includes default workers plus stress workers if launched.",
            "Capacity estimate should be based on success rate + p95 latency + system saturation.",
        ],
    }

    ts_name = dt.datetime.utcnow().strftime("%Y%m%d-%H%M%S")
    out_json = out_dir / f"stress-{ts_name}.json"
    out_json.write_text(json.dumps(summary, indent=2))

    print("\n=== Stress Test Summary ===")
    print(f"Agents: ok={agent_run['ok']} fail={agent_run['fail']} p95={agent_run['latency_ms']['p95']}ms rps={agent_run['rps']}")
    print(f"Goals:  ok={goal_run['ok']} fail={goal_run['fail']} p95={goal_run['latency_ms']['p95']}ms rps={goal_run['rps']}")
    print(f"Workers observed max={max_workers_observed} final={final_workers}")
    print(f"Load avg(1m) max={max_load1}")
    print(f"Min free memory (approx)={min_free_mem} MB")
    print(f"Report: {out_json}")


if __name__ == "__main__":
    main()
