#!/usr/bin/env python3
"""Validate Synapse artifact JSON files and parameter consistency."""
import json, os, sys, yaml
from pathlib import Path
from collections import defaultdict

ARTIFACT_DIRS = ["pipeline","dataset","linkedService","notebook","trigger","dataflow","sqlscript"]
REQUIRED = {"pipeline":["name","properties"],"dataset":["name","properties"],
            "linkedService":["name","properties"],"notebook":["name","properties"],
            "trigger":["name","properties"],"dataflow":["name","properties"],
            "sqlscript":["name","properties"]}
ENVS = ["dev","test","prod"]

class Err:
    def __init__(s, f, m, sev="error"): s.file, s.msg, s.sev = f, m, sev
    def __str__(s): return f"{'❌' if s.sev=='error' else '⚠️'} [{s.sev.upper()}] {s.file}: {s.msg}"

def check_json(root):
    errs = []
    for d in ARTIFACT_DIRS:
        p = root / d
        if not p.exists(): continue
        for f in p.rglob("*.json"):
            try: json.load(open(f))
            except json.JSONDecodeError as e: errs.append(Err(str(f.relative_to(root)), f"Invalid JSON: {e}"))
    return errs

def check_fields(root):
    errs = []
    for d, req in REQUIRED.items():
        p = root / d
        if not p.exists(): continue
        for f in p.rglob("*.json"):
            try:
                data = json.load(open(f))
                for field in req:
                    if field not in data: errs.append(Err(str(f.relative_to(root)), f"Missing field: '{field}'"))
            except: pass
    return errs

def check_ls_refs(root):
    errs, ls_names = [], set()
    lsd = root / "linkedService"
    if lsd.exists():
        for f in lsd.rglob("*.json"):
            try: ls_names.add(json.load(open(f)).get("name", f.stem))
            except: pass
    dsd = root / "dataset"
    if dsd.exists():
        for f in dsd.rglob("*.json"):
            try:
                ref = json.load(open(f)).get("properties",{}).get("linkedServiceName",{}).get("referenceName")
                if ref and ref not in ls_names:
                    errs.append(Err(str(f.relative_to(root)), f"Dangling linked service ref: '{ref}'", "warning"))
            except: pass
    return errs

def check_params(root):
    errs, keys = [], {}
    pd = root / "parameters"
    if not pd.exists(): return [Err("parameters/","Directory missing","warning")]
    for env in ENVS:
        pf = pd / f"{env}.parameters.yaml"
        if not pf.exists(): errs.append(Err(str(pf.relative_to(root)), f"Missing for {env}")); continue
        try:
            data = yaml.safe_load(open(pf))
            if isinstance(data, dict): keys[env] = set(data.keys())
            else: errs.append(Err(str(pf.relative_to(root)), "Not a mapping"))
        except yaml.YAMLError as e: errs.append(Err(str(pf.relative_to(root)), f"Bad YAML: {e}"))
    if len(keys) >= 2:
        all_k = set().union(*keys.values())
        for env, k in keys.items():
            miss = all_k - k
            if miss: errs.append(Err(f"parameters/{env}.parameters.yaml", f"Missing keys: {', '.join(sorted(miss))}", "warning"))
    return errs

def check_cycles(root):
    errs, graph = [], defaultdict(set)
    pd = root / "pipeline"
    if not pd.exists(): return errs
    for f in pd.rglob("*.json"):
        try:
            data = json.load(open(f))
            name = data.get("name", f.stem)
            for act in data.get("properties",{}).get("activities",[]):
                if act.get("type") == "ExecutePipeline":
                    ref = act.get("typeProperties",{}).get("pipeline",{}).get("referenceName")
                    if ref: graph[name].add(ref)
        except: pass
    visited, stack = set(), set()
    def dfs(n, path):
        visited.add(n); stack.add(n); path.append(n)
        for nb in graph.get(n, set()):
            if nb not in visited:
                if dfs(nb, path): return True
            elif nb in stack:
                cycle = path[path.index(nb):] + [nb]
                errs.append(Err(f"pipeline/{n}.json", f"Cycle: {' → '.join(cycle)}")); return True
        path.pop(); stack.discard(n); return False
    for n in graph:
        if n not in visited: dfs(n, [])
    return errs

def main():
    root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(".")
    print(f"Validating: {root.resolve()}\n")
    all_errs = []
    for name, fn in [("JSON Syntax",check_json),("Required Fields",check_fields),
                      ("Linked Service Refs",check_ls_refs),("Parameters",check_params),
                      ("Pipeline Cycles",check_cycles)]:
        print(f"  {name}...", end=" ")
        errs = fn(root); all_errs.extend(errs)
        if errs:
            print()
            for e in errs: print(f"    {e}")
        else: print("✅")
    ec = sum(1 for e in all_errs if e.sev=="error")
    wc = sum(1 for e in all_errs if e.sev=="warning")
    print(f"\n{'='*50}\n{ec} errors, {wc} warnings")
    sys.exit(1 if ec else 0)

if __name__ == "__main__": main()
