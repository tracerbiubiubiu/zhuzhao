"""Offline golden-case runner; not product/identity protocol conformance."""
import argparse
import json
import re
from pathlib import Path
from uaa_mock import GovernanceMock

HERE = Path(__file__).resolve().parent

def run_case(case, profile, mutant=None):
    configured = dict(profile)
    configured.update(case["profile_overrides"])
    model = GovernanceMock(configured, mutant=mutant)
    failures = []
    for index, original in enumerate(case["steps"]):
        step = dict(original)
        expected = step.pop("expect")
        if step["op"] == "audit_attribution":
            actual = {k: model.audit[-1].get(k) for k in ("actor_ref", "target_ref", "action", "result")}
        elif step["op"] == "audit_request_id":
            actual = bool(re.fullmatch(r"req-[0-9a-f]{32}", model.audit[-1].get("request_id", "")))
        elif step["op"] == "state_summary":
            actual = {"policy_revision": model.policy_revision, "desired": len(model.desired), "receipts": len(model.receipts), "seen": len(model.seen), "mode": model.mode}
        else:
            try:
                actual = model.dispatch(step)
            except Exception as error:
                actual = {"exception": type(error).__name__}
        if actual != expected:
            failures.append({"step": index + 1, "op": step["op"], "expected": expected, "actual": actual})
    output = json.dumps(model.audit, sort_keys=True)
    for forbidden in case.get("forbidden_output", []):
        if forbidden in output:
            failures.append({"op": "audit_minimization", "forbidden_marker": forbidden})
    return {"id": case["id"], "name": case["name"], "pass": not failures, "failures": failures}

def verify(include_mutations=True):
    data = json.loads((HERE / "fixtures.json").read_text())
    results = [run_case(c, data["profile"]) for c in data["cases"]]
    mutants = []
    if include_mutations:
        for mutant in ["ignore_versions", "trust_incomplete", "union_local_managed", "ack_before_policy", "ignore_freshness", "exit_without_approval", "leak_payload", "mark_all_receipts_effective", "drop_duplicate_receipt_link", "restore_old_exit_permissions", "ignore_exit_approval_digest", "ignore_exit_baseline", "caller_controls_action_kind", "ignore_disabled_connector", "nonobject_envelope_guard", "untyped_kind_guard"]:
            caught = [c["id"] for c in data["cases"] if not run_case(c, data["profile"], mutant)["pass"]]
            mutants.append({"name": mutant, "caught_by": caught, "pass": bool(caught)})
    return {"scope": "offline synthetic contract model only", "cases": len(results), "passed": sum(r["pass"] for r in results), "failed": sum(not r["pass"] for r in results), "results": results, "mutations": mutants, "real_network_or_core_calls": 0}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    parser.add_argument("--no-mutations", action="store_true")
    args = parser.parse_args()
    report = verify(not args.no_mutations)
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({k: v for k, v in report.items() if k != "results"}, ensure_ascii=False, indent=2))
    raise SystemExit(bool(report["failed"] or any(not m["pass"] for m in report["mutations"])))

if __name__ == "__main__":
    main()
