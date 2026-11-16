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


def observe_read(state, enforce_read_contract=True):
    """Synthetic selected read bindings; never actual authorization or AL access."""
    def result(decision):
        return {"decision": decision, "additional_effects": 0}

    if not enforce_read_contract:
        return result("allow_read")
    command, receipt = state["command"], state["receipt"]
    params, target = command["params"], command["target"]
    if not state["actor_active"] or not state["read_allowed"]:
        return result("held_revoked")
    if (target["owner"] != "activelist"
            or target["resource_type"] != "activelist_document"
            or target["resource_id"] != params["document_id"]
            or target["type_name"] != params["type_name"]
            or target["generation"] != params["generation"]):
        return result("reject_target_binding")
    requested = set(params["fields"])
    allowed = set(state["allowed_fields"])
    if not requested.issubset(allowed):
        return result("reject_requested_fields")
    event_id = params.get("source_event_id")
    fact = state.get("source_fact")
    if event_id and (not fact or fact["id"] != event_id
                     or fact["source"] != state["registered_source"]
                     or fact["data"]["document_id"] != params["document_id"]
                     or fact["data"]["generation"] != params["generation"]
                     or params["type_name"] != state["registered_type"]
                     or params["generation"] != state["registered_generation"]
                     or fact["subject"] != f'{params["type_name"]}/{params["document_id"]}'):
        return result("reject_event_binding")
    for key in ("operation_id", "invocation_id", "capability_id", "contract_version", "canonical_input_hash"):
        if receipt[key] != command[key]:
            return result("reject_receipt_binding")
    query = receipt["query_result"]
    if any(query[key] != params[key] for key in ("type_name", "document_id", "generation")):
        return result("reject_result_binding")
    if not set(query["values"]).issubset(requested & allowed):
        return result("reject_result_fields")
    if event_id:
        if (query.get("source_event_id") != event_id
                or query.get("source_event_revision") != fact["data"]["document_revision"]):
            return result("reject_event_binding")
        if int(query["document_revision"]) < int(fact["data"]["document_revision"]):
            return result("reject_stale_observation")
    elif "source_event_id" in query or "source_event_revision" in query:
        return result("reject_event_binding")
    encoded = json.dumps(query, ensure_ascii=False, separators=(",", ":")).encode("utf-8")
    if len(encoded) > 32768:
        return result("reject_result_budget")
    return result("allow_read")


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


def canonical_push_bytes(value):
    """Encode selected JSON values as Node's existing safe-integer canonicalizer."""
    def normalized(item):
        if item is None or isinstance(item, (str, bool)):
            return item
        if isinstance(item, (int, float)):
            if (abs(item) > 9007199254740991 or not math.isfinite(item)
                    or (isinstance(item, float) and not item.is_integer())):
                raise ValueError("unsafe or non-integer number")
            # JSON 1.0/-0.0 and 1/0 have the same integer value in Node.
            return int(item)
        if isinstance(item, list):
            return [normalized(child) for child in item]
        if isinstance(item, dict) and all(isinstance(key, str) for key in item):
            return {key: normalized(child) for key, child in item.items()}
        raise ValueError("not a selected JSON value")

    selected = normalized(value)
    check_values(selected)
    return json.dumps(selected, ensure_ascii=False, sort_keys=True,
                      separators=(",", ":")).encode("utf-8")


def observe_push(state, commit_guard=True, duplicate_guard=True,
                 ack_binding_guard=True, source_scope_guard=True):
    """Finite receiver/sender oracle. Committed facts are synthetic test inputs."""
    delivery = state["delivery"]
    event = delivery["event"]
    payload_hash = hashlib.sha256(canonical_push_bytes(event)).hexdigest()

    def rejected(status, quarantined=False):
        return {"status": status, "ack": None, "new_inbox_rows": 0,
                "outbox_acknowledged": False, "quarantined": quarantined}

    if not state["trusted_context"]:
        return rejected(401)
    if not state["source_trusted"]:
        return rejected(403)
    if source_scope_guard and (delivery["consumer_id"] != state["registered_consumer"]
                              or event["source"] != state["registered_source"]):
        return rejected(403)
    if not state["db_available"]:
        return rejected(503)
    if not state["capacity_available"]:
        return rejected(429)
    existing = state["existing_record"]
    if existing and (existing["source"] != event["source"]
                     or existing["event_id"] != event["id"]):
        return rejected(503)  # A lookup not bound to this unique key proves nothing.
    if existing and duplicate_guard:
        if existing["payload_sha256"] != payload_hash:
            return rejected(409, quarantined=True)
        stage_ref, rows = existing["stage_commit_ref"], 0
    else:
        if commit_guard and not state["db_commit_succeeded"]:
            return rejected(503)
        stage_ref = state["new_stage_commit_ref"]
        rows = int(state["db_commit_succeeded"])
    ack = {"schema_version": "1", "delivery_id": delivery["delivery_id"],
           "source": event["source"], "event_id": event["id"],
           "payload_sha256": payload_hash, "stage_commit_ref": stage_ref,
           "state": "durably_staged"}
    received_ack = state.get("ack_override", ack)
    accepted = (state["ack_authenticated"]
                and state["delivery_fence"] == state["current_fence"]
                and received_ack["delivery_id"] == delivery["delivery_id"]
                and received_ack["source"] == event["source"]
                and received_ack["event_id"] == event["id"]
                and received_ack["payload_sha256"] == payload_hash
                and received_ack["state"] == "durably_staged")
    return {"status": 200, "ack": ack, "new_inbox_rows": rows,
            "outbox_acknowledged": accepted if ack_binding_guard else True,
            "quarantined": False}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--disable-replay-read-guard", action="store_true",
                        help="deliberate design-model mutation: the three replay cases must fail")
    parser.add_argument("--disable-read-contract-guard", action="store_true",
                        help="deliberate design-model mutation: selected read binding cases must fail")
    for guard in ("commit", "duplicate", "ack-binding", "source-scope"):
        parser.add_argument("--disable-push-" + guard + "-guard", action="store_true",
                            help="deliberate push-model mutation: relevant cases must fail")
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
    read_rows = []
    for case in schema["x-read-semantic-cases"]:
        actual = observe_read(case["input"], not args.disable_read_contract_guard)
        read_rows.append({"id": case["id"], "pass": (
            actual["decision"] == case["expected_decision"]
            and actual["additional_effects"] == case["expected_additional_effects"]), **actual})
    regression = json.loads((HERE / "replay-read-regression.json").read_text())
    mutation_caught = [case["id"] for case in regression["cases"]
                       if observe(case["input_state"], False)["decision"] != case["expected_decision"]]
    read_mutation_caught = [case["id"] for case in schema["x-read-semantic-cases"]
                            if observe_read(case["input"], False)["decision"] != case["expected_decision"]]
    expected_read_negatives = [case["id"] for case in schema["x-read-semantic-cases"]
                               if case["expected_decision"] != "allow_read"]
    push_rows = []
    for case in schema["x-push-semantic-cases"]:
        actual = observe_push(case["input_state"],
                              not args.disable_push_commit_guard,
                              not args.disable_push_duplicate_guard,
                              not args.disable_push_ack_binding_guard,
                              not args.disable_push_source_scope_guard)
        push_rows.append({"id": case["id"], "pass": actual == case["expected"], **actual})
    push_canonical_rows = []
    for case in schema["x-push-canonical-cases"]:
        try:
            encoded = canonical_push_bytes(case["value"]).decode("utf-8")
            passed = case["expected_valid"] and encoded == case["expected_canonical_utf8"]
        except ValueError:
            passed = not case["expected_valid"]
        push_canonical_rows.append({"id": case["id"], "pass": passed})
    push_mutation_expected = {
        "commit_guard": ["push_no_ack_before_commit"],
        "duplicate_guard": ["push_duplicate_original_stage", "push_duplicate_new_delivery",
                            "push_same_id_changed_payload"],
        "ack_binding_guard": ["push_ack_wrong_delivery", "push_ack_wrong_source",
                              "push_ack_wrong_event", "push_ack_wrong_payload",
                              "push_ack_stale_fence", "push_ack_unsigned"],
        "source_scope_guard": ["push_unregistered_consumer", "push_unregistered_source"],
    }
    push_mutation_caught = {
        guard: [case["id"] for case in schema["x-push-semantic-cases"]
                if observe_push(case["input_state"], **{guard: False}) != case["expected"]]
        for guard in push_mutation_expected
    }
    unchanged = SCHEMA.read_bytes() == before
    failures = [row for row in semantic_rows + raw_rows + read_rows + push_rows + push_canonical_rows
                if not row["pass"]]
    if len(mutation_caught) != 3 or not unchanged:
        failures.append({"id": "mutation_or_readonly_invariant", "pass": False})
    if not expected_read_negatives or read_mutation_caught != expected_read_negatives:
        failures.append({"id": "read_contract_mutation_invariant", "pass": False})
    if push_mutation_caught != push_mutation_expected:
        failures.append({"id": "push_contract_mutation_invariant", "pass": False})
    print(json.dumps({
        "semantic_cases": len(semantic_rows),
        "semantic_passed": sum(row["pass"] for row in semantic_rows),
        "raw_cases": len(raw_rows), "raw_passed": sum(row["pass"] for row in raw_rows),
        "read_contract_cases": len(read_rows),
        "read_contract_passed": sum(row["pass"] for row in read_rows),
        "replay_read_guard_mutation_caught": mutation_caught,
        "read_contract_guard_mutation_caught": read_mutation_caught,
        "push_contract_cases": len(push_rows),
        "push_contract_passed": sum(row["pass"] for row in push_rows),
        "push_canonical_cases": len(push_canonical_rows),
        "push_canonical_passed": sum(row["pass"] for row in push_canonical_rows),
        "push_contract_guard_mutation_caught": push_mutation_caught,
        "schema_unchanged": unchanged, "schema_sha256": hashlib.sha256(before).hexdigest(),
        "failures": failures,
        "proof_scope": "Synthetic contract decision model and strict raw JSON, not product runtime or real authorization.",
    }, ensure_ascii=False, indent=2))
    raise SystemExit(bool(failures))


if __name__ == "__main__":
    main()
