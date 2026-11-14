#!/usr/bin/env python3
"""Read-only candidate contract oracle; not production authorization or execution."""

import argparse
import hashlib
import json
import math
from pathlib import Path


HERE = Path(__file__).resolve().parent
SCHEMA = HERE.parents[1] / "capability-interconnect-detail.schema.json"


def decide(state, enforce_replay_read=True):
    """The state is synthetic authority input, never an externally accepted actor."""
    if not state["trusted_context"] or not state["source_trusted"]:
        return "reject_context", 0
    if state["phase"] == "ingest":
        return ("ack_duplicate_stage" if state.get("event_already_staged")
                else "stage_only_ack_after_commit"), 0
    if state["phase"] == "receipt":
        return ("return_original_receipt" if state["read_allowed"] and state["actor_active"]
                else "not_found"), 0
    # Execution authorization and permission to read an existing result are distinct.
    # This guard also covers replay on a write/execute endpoint, before any old content.
    if (enforce_replay_read and state["existing"]
            and (not state["read_allowed"] or not state["actor_active"])):
        return "not_found", 0
    if not state["actor_active"] or not state["allowed"]:
        return ("forbidden" if state["phase"] == "admit" else "held_revoked"), 0
    if state["existing"] and not state["hash_matches"]:
        return "conflict_input", 0
    if state["existing"] == "succeeded":
        return "return_original_receipt", 0
    if state["existing"] == "unknown":
        return "reconcile_original_operation", 0
    if state["existing"] == "held":
        return "return_held_no_auto_resume", 0
    if not state["supported_version"]:
        return "held_incompatible_version", 0
    if not state["definition_matches"]:
        return "held_definition_changed", 0
    if not state["rule_current"]:
        return "held_rule_replaced", 0
    if not state["generation_matches"]:
        return "held_target_generation_changed", 0
    if state["fence"] < state["current_fence"]:
        return "reject_stale_attempt", 0
    if not state["lease_current"]:
        return ("unknown_reconcile" if state["effect_may_have_started"]
                else "reject_expired_attempt"), 0
    if not state["expected_revision_matches"]:
        return "conflict_revision", 0
    if state["same_rule_in_root"]:
        return "held_causal_reentry", 0
    if not state["root_budget"]:
        return "held_causal_budget", 0
    if not state["provider_quota"]:
        return "held_quota", 0
    return ("success_noop", 0) if state["no_op"] else ("execute_selected_effect", 1)


def observe(state, enforce_replay_read=True):
    decision, effects = decide(state, enforce_replay_read)
    returns_record = decision in {
        "return_original_receipt", "return_held_no_auto_resume", "reconcile_original_operation",
    }
    # A sentinel makes record disclosure a separate assertion from additional effects.
    record = {"status": state["existing"], "content": "synthetic-existing-record"} if returns_record else None
    return {"decision": decision, "additional_effects": effects,
            "returns_existing_record": returns_record, "existing_record": record}


def duplicate_checked(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate key")
        result[key] = value
    return result


def reject_constant(_):
    raise ValueError("non-JSON numeric constant")


def check_values(value):
    if isinstance(value, dict):
        for key, item in value.items():
            check_values(key)
            check_values(item)
    elif isinstance(value, list):
        for item in value:
            check_values(item)
    elif isinstance(value, bool) or value is None:
        return
    elif isinstance(value, (int, float)):
        if not math.isfinite(value) or abs(value) > 9007199254740991:
            raise ValueError("unsafe number")
    elif isinstance(value, str):
        if any(0xD800 <= ord(char) <= 0xDFFF for char in value):
            raise ValueError("lone surrogate")


def raw_valid(raw):
    try:
        value = json.loads(raw, object_pairs_hook=duplicate_checked,
                           parse_constant=reject_constant)
        check_values(value)
        return True
    except (ValueError, OverflowError):
        return False


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--disable-replay-read-guard", action="store_true",
                        help="deliberate design-model mutation: the three replay cases must fail")
    args = parser.parse_args()
    before = SCHEMA.read_bytes()
    schema = json.loads(before)
    semantic_rows = []
    for case in schema["x-semantic-cases"]:
        actual = observe(case["input_state"], not args.disable_replay_read_guard)
        passed = (actual["decision"] == case["expected_decision"]
                  and actual["additional_effects"] == case["expected_additional_effects"])
        if "expected_returns_existing_record" in case:
            passed = passed and actual["returns_existing_record"] == case["expected_returns_existing_record"]
            if not case["expected_returns_existing_record"]:
                passed = passed and actual["existing_record"] is None
        semantic_rows.append({"id": case["id"], "pass": passed, **actual})
    raw_rows = [{"id": case["id"], "pass": raw_valid(case["raw"]) == case["expected_valid"]}
                for case in schema["x-raw-json-cases"]]
    regression = json.loads((HERE / "replay-read-regression.json").read_text())
    mutation_caught = [case["id"] for case in regression["cases"]
                       if observe(case["input_state"], False)["decision"] != case["expected_decision"]]
    unchanged = SCHEMA.read_bytes() == before
    failures = [row for row in semantic_rows + raw_rows if not row["pass"]]
    if len(mutation_caught) != 3 or not unchanged:
        failures.append({"id": "mutation_or_readonly_invariant", "pass": False})
    print(json.dumps({
        "semantic_cases": len(semantic_rows),
        "semantic_passed": sum(row["pass"] for row in semantic_rows),
        "raw_cases": len(raw_rows), "raw_passed": sum(row["pass"] for row in raw_rows),
        "replay_read_guard_mutation_caught": mutation_caught,
        "schema_unchanged": unchanged, "schema_sha256": hashlib.sha256(before).hexdigest(),
        "failures": failures,
        "proof_scope": "Synthetic contract decision model and strict raw JSON, not product runtime or real authorization.",
    }, ensure_ascii=False, indent=2))
    raise SystemExit(bool(failures))


if __name__ == "__main__":
    main()
