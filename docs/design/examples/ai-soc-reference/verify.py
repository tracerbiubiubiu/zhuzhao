#!/usr/bin/env python3
"""Executable design model, not a product runtime or real provider client."""
import argparse
import copy
import hashlib
import json
import tempfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
READ_TOOLS = ["risk.query", "ticket.linked.query", "knowledge.query", "risk.dispatch.preview"]
POLICY_TOOLS = set(READ_TOOLS + ["risk.dispatch.apply", "operation.receipt"])


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(",", ":")).encode()).hexdigest()


def atomic_json(path, value):
    temporary = path.with_suffix(".pending")
    temporary.write_text(json.dumps(value, sort_keys=True, ensure_ascii=False))
    temporary.replace(path)


class Stop(Exception):
    pass


class ToolEnvironment:
    def __init__(self, data, folder, mutant=None):
        self.mutant = mutant
        self.data = copy.deepcopy(data)
        self.folder = Path(folder)
        self.budget_file = self.folder / "budget.json"
        self.calls = json.loads(self.budget_file.read_text()) if self.budget_file.exists() else []
        self.drafts = []
        self.existing = []
        self.refs = []
        self.risks = [
            {"id": "risk-a", "org": "org-a", "owner": "demo-owner", "revision": 17},
            {"id": "risk-b", "org": "org-a", "owner": "demo-owner", "revision": 12},
            {"id": "risk-hidden", "org": "org-b", "owner": "hidden-owner", "revision": 5},
        ]
        if self.data["data_profile"] == "different_owner_missing":
            self.risks[1]["owner"] = "demo-other-owner"
        if self.data["data_profile"] == "over_limit":
            self.risks = [{"id": "risk-" + str(i).zfill(2), "org": "org-a", "owner": "demo-owner", "revision": 17} for i in range(1, 22)]
        self.source_revisions = {r["id"]: r["revision"] for r in self.risks}
        self.knowledge_file = self.folder / "knowledge-source.json"
        if not self.knowledge_file.exists():
            atomic_json(self.knowledge_file, {
                "knowledge-public": {"id": "knowledge-public", "org": "org-a", "version": "k7", "digest": hashlib.sha256(b"Synthetic approved remediation procedure k7").hexdigest(), "visible": True},
                "knowledge-secret": {"id": "knowledge-secret", "org": "org-b", "version": "k9", "digest": hashlib.sha256(b"Synthetic restricted procedure k9").hexdigest(), "visible": True},
            })
        self.effect_access_file = self.folder / "effect-access.json"
        if not self.effect_access_file.exists():
            atomic_json(self.effect_access_file, {"visible": True})
        self.ledger = self.folder / "effects.json"
        if not self.ledger.exists():
            atomic_json(self.ledger, {})
        self.checkpoint = self.folder / "checkpoint.json"

    def record_call(self, tool):
        if tool not in POLICY_TOOLS:
            raise Stop("denied")
        if len(self.calls) >= self.data["max_tools"]:
            raise Stop("budget_exhausted")
        self.calls.append(tool)
        atomic_json(self.budget_file, self.calls)

    def call(self, tool):
        self.record_call(tool)
        if tool == "risk.query":
            if not self.data["source_available"]:
                raise Stop("degraded")
            return [r for r in self.risks if r["org"] in self.data["orgs"]]
        if tool == "ticket.linked.query":
            return [{"risk_id": "risk-b", "ticket_id": "demo-ticket-9"}] if self.data["data_profile"] == "one_missing" else []
        if tool == "knowledge.query":
            if not self.data["knowledge_available"]:
                raise Stop("held")
            # Filter at provider contract boundary; never send an all-knowledge list to a prompt.
            knowledge = json.loads(self.knowledge_file.read_text()).values()
            return [k for k in knowledge if k["visible"] and k["org"] in self.data["orgs"]]
        if tool == "risk.dispatch.preview":
            return {"definition_ref": "demo-remediation@4", "target_org": "org-a", "validation": "valid"}
        raise ValueError("write and receipt require an exact plan")

    def prepare(self, observations):
        risk_rows, links, knowledge, preview = observations
        linked = {r["risk_id"] for r in links}
        self.drafts = [r["id"] for r in risk_rows if r["id"] not in linked]
        self.existing = [r["id"] for r in risk_rows if r["id"] in linked]
        self.refs = [{"id": k["id"], "version": k["version"], "digest": k["digest"]} for k in knowledge]
        if not knowledge:
            raise Stop("held")
        targets = [r for r in risk_rows if r["id"] in self.drafts]
        if len(targets) > 20 or len({r["owner"] for r in targets}) != 1:
            raise Stop("held")
        plan = {
            "actor": self.data["actor"], "auth_epoch": self.data["auth_epoch"],
            "config_version": self.data["config_version"],
            "contract_version": self.data["contract_version"],
            "skill_version": self.data["skill_version"],
            "definition_ref": preview["definition_ref"], "target_org": preview["target_org"],
            "risk_revisions": {r["id"]: r["revision"] for r in risk_rows if r["id"] in self.drafts},
            "knowledge_refs": self.refs, "operation_id": "demo-operation-1",
        }
        atomic_json(self.checkpoint, {"state": "awaiting_approval", "plan": plan, "plan_hash": digest(plan), "existing_risk_ids": self.existing})
        return plan

    def current_knowledge_refs(self, refs):
        # The provider authority is re-read after preview and checkpoint restore.
        # A digest identifies content, not permission; both current ACL and exact provenance must hold.
        if not self.data["knowledge_available"]:
            return []
        current = json.loads(self.knowledge_file.read_text())
        valid = []
        for ref in refs:
            source = current.get(ref["id"])
            if (source and source["visible"] and source["org"] in self.data["orgs"]
                    and source["version"] == ref["version"] and source["digest"] == ref["digest"]):
                valid.append(copy.deepcopy(ref))
        return valid

    def recheck(self, plan, approval):
        if self.data["scope_change"]:
            raise Stop("denied")
        if self.data["now"] > approval["expires_at"]:
            raise Stop("denied")
        if approval["actor"] != self.data["actor"] or approval["plan_hash"] != digest(plan):
            raise Stop("denied")
        if approval["config_version"] != plan["config_version"] or plan["config_version"] != "cfg-1":
            raise Stop("held")
        if plan["auth_epoch"] != self.data["auth_epoch"]:
            raise Stop("denied")
        for key, revision in plan["risk_revisions"].items():
            if self.source_revisions[key] != revision:
                raise Stop("held")
        if self.mutant != "ignore_knowledge_recheck":
            refs = plan["knowledge_refs"]
            if not refs or self.current_knowledge_refs(refs) != refs:
                raise Stop("held")

    def apply(self, plan, approval, lose_response=False):
        self.record_call("risk.dispatch.apply")
        ledger = json.loads(self.ledger.read_text())
        operation_id = plan["operation_id"]
        if operation_id in ledger:
            record = ledger[operation_id]
            if self.mutant != "ignore_apply_plan_binding" and record["plan_hash"] != digest(plan):
                raise Stop("denied")
            if self.mutant == "revalidate_completed_operation":
                self.recheck(plan, approval)
            if self.mutant == "ignore_apply_replay_authorization":
                return record
            # A completed effect is a read, even when replay enters through apply.
            # Current read authority still applies; old execution approval/knowledge
            # no longer controls whether the already committed status can be read.
            return self.current_receipt_projection(record)
        self.recheck(plan, approval)
        receipt = {"operation_id": operation_id, "plan_hash": digest(plan), "effect_id": "demo-ticket-created-1", "actor": self.data["actor"], "target_org": plan["target_org"]}
        # A single atomic JSON record models one business transaction; it does not validate PG transactions.
        ledger[operation_id] = receipt
        atomic_json(self.ledger, ledger)
        if lose_response:
            raise TimeoutError("synthetic response lost after committed effect")
        return receipt

    def receipt(self, plan):
        self.record_call("operation.receipt")
        record = json.loads(self.ledger.read_text())[plan["operation_id"]]
        return self.current_receipt_projection(record)

    def current_receipt_projection(self, record):
        if self.mutant != "ignore_receipt_authorization":
            if (self.data["scope_change"] or record["actor"] != self.data["actor"]
                    or not json.loads(self.effect_access_file.read_text())["visible"]
                    or record["target_org"] not in self.data["orgs"]):
                raise Stop("denied")
        # Reading committed business status does not re-execute the plan or disclose its knowledge.
        return {"operation_id": record["operation_id"], "status": "committed", "effect_id": record["effect_id"]}


def fixed_steps(env):
    # The formatting-model response is synthetic; tools are chosen by application steps.
    if env.data["action"] == "injected_tool":
        return env.call("admin.destroy")
    observations = [env.call(tool) for tool in READ_TOOLS]
    return env.prepare(observations), 1


def enterprise_stub(env):
    # A serialized external-agent plan simulates the adapter boundary, not any real vendor.
    packet = json.loads(json.dumps({"external_run_id": "demo-external-run-1", "tool_requests": READ_TOOLS}))
    if env.data["action"] == "injected_tool":
        packet["tool_requests"] = ["admin.destroy"]
    observations = []
    for tool in packet["tool_requests"]:
        observations.append(env.call(tool))
    return env.prepare(observations), 2


def bounded_host(env):
    # A scripted proposal model selects the next tool from observations; no actual LLM/SDK is loaded.
    observations, model_requests = [], 0
    while len(observations) < 4:
        model_requests += 1
        tool = READ_TOOLS[len(observations)]
        if env.data["action"] == "injected_tool":
            tool = "admin.destroy"
        observations.append(env.call(tool))
    return env.prepare(observations), model_requests


STRATEGIES = {"fixed_steps": fixed_steps, "enterprise_agent_stub": enterprise_stub, "bounded_host_model": bounded_host}


def run_agent(case, mode, mutant=None):
    with tempfile.TemporaryDirectory(prefix="zz-ai-reference-") as folder:
        env = ToolEnvironment(case["input"], folder, mutant)
        requests, state, receipt_result = 0, None, None
        try:
            if env.data["action"] == "config_retired":
                raise Stop("held")
            plan, requests = STRATEGIES[mode](env)
            if not env.data["approval"]:
                raise Stop("awaiting_approval")
            approval = {
                "actor": env.data["actor"], "plan_hash": digest(plan),
                "config_version": plan["config_version"], "expires_at": env.data["approval_expires"],
            }
            if env.data["action"] == "cancelled":
                raise Stop("cancelled")
            if env.data["action"] == "plan_changed":
                plan["target_org"] = "org-b"
            if env.data["action"] == "risk_revision_changed":
                env.source_revisions["risk-a"] = 18
            if env.data["action"] in {"knowledge_acl_revoked", "knowledge_version_changed", "knowledge_digest_changed"}:
                current = json.loads(env.knowledge_file.read_text())
                if env.data["action"] == "knowledge_acl_revoked":
                    current["knowledge-public"]["visible"] = False
                elif env.data["action"] == "knowledge_version_changed":
                    current["knowledge-public"]["version"] = "k8"
                else:
                    current["knowledge-public"]["digest"] = hashlib.sha256(b"Changed content under the same version").hexdigest()
                atomic_json(env.knowledge_file, current)
            if mutant == "ignore_revocation":
                env.data["scope_change"] = False
            if mutant == "ignore_exact_approval":
                approval["plan_hash"] = digest(plan)
            if env.data["action"].startswith("apply_replay_"):
                # Exercise the write/execute replay route, not only the dedicated receipt read.
                # The first command has already committed before any authority/approval change.
                env.apply(plan, approval)
                action = env.data["action"]
                if action in {"apply_replay_effect_revoked", "apply_replay_effect_revoked_after_restore"}:
                    atomic_json(env.effect_access_file, {"visible": False})
                elif action in {"apply_replay_knowledge_revoked", "apply_replay_knowledge_version_changed", "apply_replay_knowledge_digest_changed"}:
                    current = json.loads(env.knowledge_file.read_text())
                    if action == "apply_replay_knowledge_revoked":
                        current["knowledge-public"]["visible"] = False
                    elif action == "apply_replay_knowledge_version_changed":
                        current["knowledge-public"]["version"] = "k8"
                    else:
                        current["knowledge-public"]["digest"] = hashlib.sha256(b"Changed after committed effect").hexdigest()
                    atomic_json(env.knowledge_file, current)
                elif action == "apply_replay_approval_expired":
                    env.data["now"] = approval["expires_at"] + 1
                elif action == "apply_replay_actor_changed":
                    env.data["actor"] = "demo-other-user"
                elif action == "apply_replay_scope_changed":
                    env.data["scope_change"] = True
                elif action == "apply_replay_target_scope_revoked":
                    env.data["orgs"] = ["org-b"]
                if action != "apply_replay_effect_revoked":
                    saved = json.loads(env.checkpoint.read_text())
                    restored = ToolEnvironment(env.data, folder, mutant)
                    restored.drafts = list(saved["plan"]["risk_revisions"])
                    restored.existing = saved["existing_risk_ids"]
                    restored.refs = saved["plan"]["knowledge_refs"]
                    plan = saved["plan"]
                    env = restored
                if action == "apply_replay_plan_changed":
                    plan["target_org"] = "org-b"
                receipt_result = env.apply(plan, approval)
            elif env.data["action"] in {"receipt_knowledge_revoked", "receipt_actor_changed", "receipt_effect_revoked"}:
                env.apply(plan, approval)
                if env.data["action"] == "receipt_knowledge_revoked":
                    current = json.loads(env.knowledge_file.read_text())
                    current["knowledge-public"]["visible"] = False
                    atomic_json(env.knowledge_file, current)
                    saved = json.loads(env.checkpoint.read_text())
                    restored = ToolEnvironment(env.data, folder, mutant)
                    restored.drafts = list(saved["plan"]["risk_revisions"])
                    restored.existing = saved["existing_risk_ids"]
                    restored.refs = saved["plan"]["knowledge_refs"]
                    env = restored
                elif env.data["action"] == "receipt_actor_changed":
                    env.data["actor"] = "demo-other-user"
                else:
                    atomic_json(env.effect_access_file, {"visible": False})
                    saved = json.loads(env.checkpoint.read_text())
                    restored = ToolEnvironment(env.data, folder, mutant)
                    restored.drafts = list(saved["plan"]["risk_revisions"])
                    restored.existing = saved["existing_risk_ids"]
                    restored.refs = saved["plan"]["knowledge_refs"]
                    env = restored
                receipt_result = env.receipt(plan)
            elif env.data["action"] == "lost_response":
                try:
                    env.apply(plan, approval, lose_response=True)
                except TimeoutError:
                    env.receipt(plan)
            elif env.data["action"] == "resume_after_apply":
                env.apply(plan, approval)
                saved = json.loads(env.checkpoint.read_text())
                restored = ToolEnvironment(env.data, folder, mutant)
                restored.drafts = list(saved["plan"]["risk_revisions"])
                restored.existing = saved["existing_risk_ids"]
                restored.refs = saved["plan"]["knowledge_refs"]
                restored.receipt(saved["plan"])
                env = restored
            else:
                env.apply(plan, approval)
                if env.data["action"] == "duplicate_confirmation":
                    env.apply(plan, approval)
            state = "succeeded"
        except Stop as failure:
            state = str(failure)
        result = {
            "state": state, "effects": len(json.loads(env.ledger.read_text())),
            "draft_risk_ids": env.drafts, "existing_risk_ids": env.existing,
            "knowledge_refs": env.current_knowledge_refs(env.refs) if env.data["actor"] == case["input"]["actor"] else [], "tool_calls": env.calls,
            "synthetic_provider_requests": requests, "receipt_result": receipt_result,
        }
        return result


def load_skill_contract(manifest, skill_bytes, revoked=False, trusted=True, path="SKILL.md", assets=()):
    if not trusted:
        return "reject_trust"
    if revoked:
        return "reject_revoked"
    if ".." in Path(path).parts or Path(path).is_absolute():
        return "reject_path"
    if any(Path(a).suffix in {".py", ".sh", ".js", ".exe"} for a in assets):
        return "reject_executable"
    if hashlib.sha256(skill_bytes).hexdigest() != manifest["files"]["SKILL.md"]:
        return "reject_digest"
    return "loaded"


def run_skill(case, manifest, skill_bytes, mutant=None):
    variant = case["variant"]
    if variant in {"valid", "tampered_bytes", "untrusted_publisher", "revoked", "path_escape", "executable_asset"}:
        data = skill_bytes + b"\nchanged" if variant == "tampered_bytes" else skill_bytes
        return load_skill_contract(
            manifest, data, revoked=variant == "revoked", trusted=variant != "untrusted_publisher",
            path="../outside.md" if variant == "path_escape" else "SKILL.md",
            assets=["scripts/run.sh"] if variant == "executable_asset" else [],
        )
    if variant == "allowed_tools_escalation":
        # Even a trusted document declaring more tools is not an authorization grant.
        hint = set(READ_TOOLS + ["admin.destroy"])
        available = hint & POLICY_TOOLS & set(manifest["required_capabilities"])
        return "deny_tool" if "admin.destroy" not in available else "loaded"
    if variant == "foreign_acl_reference":
        request = "knowledge-secret"
        visible = {"knowledge-public"}
        return "deny_reference" if request not in visible else "loaded"
    if variant == "global_service_account_only":
        provider = {"subject_acl": False, "approved_shared_collection": False}
        return "hold_provider" if not (provider["subject_acl"] or provider["approved_shared_collection"]) else "loaded"
    original_plan = {"risk": "risk-a", "revision": 17}
    current_plan = copy.deepcopy(original_plan)
    checkpoint = {"skill_version": "1.0.0", "skill_digest": hashlib.sha256(skill_bytes).hexdigest(), "input_contract": "1", "plan_hash": digest(original_plan)}
    registry = {"1.0.0": {"manifest": manifest, "revoked": False}}
    current_contract, actor_active = "1", True
    if variant == "compatible_patch_available":
        registry["1.0.1"] = {"manifest": {**manifest, "version": "1.0.1"}, "revoked": False}
    if variant == "missing_pinned_version":
        registry = {"1.0.1": {"manifest": {**manifest, "version": "1.0.1"}, "revoked": False}}
    if variant == "revoked_after_pause":
        registry["1.0.0"]["revoked"] = True
    if variant == "breaking_input_contract":
        current_contract = "2"
    if variant == "actor_revoked":
        actor_active = False
    if variant == "new_plan_old_approval":
        current_plan["revision"] = 18
    if not actor_active:
        return "reject_actor"
    pinned = registry.get(checkpoint["skill_version"])
    if not pinned:
        return "hold_version"
    loaded = load_skill_contract(pinned["manifest"], skill_bytes, revoked=pinned["revoked"])
    if loaded != "loaded":
        return loaded
    if checkpoint["skill_digest"] != pinned["manifest"]["files"]["SKILL.md"]:
        return "hold_version"
    if current_contract != checkpoint["input_contract"]:
        return "hold_contract"
    if digest(current_plan) != checkpoint["plan_hash"]:
        return "reject_approval"
    return "resumed_pinned" if len(registry) > 1 else "resumed"


def soc_decision(subject, action, case, mutant=None):
    if not subject["active"] or not subject["grant"]:
        return "deny"
    if mutant != "ignore_epoch" and subject["epoch"] != case["expected_epoch"]:
        return "deny"
    roles = set(subject["roles"])
    admin = "system:admin" in roles
    if action in {"modify_source_verdict", "invoke_soc_action"}:
        return "deny"
    if action == "ingest_source":
        if "source:ingest" not in roles or subject.get("provider") != case["provider"]:
            return "deny"
        return "ignore_stale" if case["source_revision"] <= case["last_source_revision"] else "allow"
    participates = case["id"] in subject["cases"] and case["org"] in subject["orgs"]
    if mutant == "risk_group_implies_event" and "risk:manage" in roles:
        participates = True
        roles |= {"incident:read", "incident:manage"}
    if not admin and not (participates and "incident:read" in roles):
        return "deny"
    if action == "view_projection":
        # Filter before count/serialize; the notice is constant, even if nothing was hidden.
        refs = [item["id"] for item in case.get("projection_refs", [])
                if item["visible"] or mutant == "ignore_underlying_acl"]
        source_coverage = None
        if case.get("known_source_gap") and case["provider"] in subject.get("source_coverage", []):
            source_coverage = {"provider": case["provider"], "status": "delayed"}
        return {"visible_refs": refs, "visible_count": len(refs),
                "notice": "结果按当前权限范围展示", "source_coverage": source_coverage}
    if action == "read_case":
        return "allow" if case["source_fresh"] else "allow_stale"
    if action in {"read_evidence", "export"} and mutant != "ignore_underlying_acl" and not case["underlying_visible"]:
        return "deny"
    if action == "read_evidence":
        return "allow"
    if action == "export":
        return "allow" if admin or "incident:export" in roles else "deny"
    if action in {"append_local_note", "complete_duty_task"}:
        if not admin and "incident:collaborate" not in roles:
            return "deny"
        if action == "complete_duty_task" and subject.get("duty") != case.get("duty"):
            return "deny"
        return "allow"
    if action in {"assign_task", "manage_members", "set_local_status"}:
        if not admin and "incident:manage" not in roles:
            return "deny"
        if case["scope_version"] != case["expected_scope_version"]:
            return "conflict"
        if action == "assign_task":
            if case["target_org"] != "org-a":
                return "deny"
            if not case["source_fresh"]:
                if case["manual_source_gap_enabled"] and case["assignment_origin"] == "manual" and case["source_gap_ack"] and case["source_gap_reason"].strip():
                    return "allow_local_stale"
                return "hold_source"
        return "allow"
    return "deny"


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    fixtures = json.loads((HERE / "fixtures.json").read_text())
    manifest = json.loads((HERE / "skills/risk-remediation-brief/manifest.json").read_text())
    skill_bytes = (HERE / "skills/risk-remediation-brief/SKILL.md").read_bytes()
    failures, results = [], {"agent": [], "skill": [], "soc": []}
    for mode in STRATEGIES:
        for case in fixtures["agent_cases"]:
            actual = run_agent(case, mode)
            checks = {key: actual[key] == value for key, value in case["expected"].items() if key != "forbidden_refs"}
            serialized = json.dumps(actual)
            checks["no_foreign_reference"] = all(ref not in serialized for ref in case["expected"]["forbidden_refs"])
            passed = all(checks.values())
            results["agent"].append({"id": case["id"], "strategy": mode, "pass": passed, "checks": checks, "actual": actual})
            if not passed:
                failures.append({"suite": "agent", "id": case["id"], "mode": mode, "actual": actual, "expected": case["expected"]})
    for case in fixtures["skill_cases"]:
        actual = run_skill(case, manifest, skill_bytes)
        passed = actual == case["expected"]
        results["skill"].append({"id": case["id"], "pass": passed, "actual": actual})
        if not passed:
            failures.append({"suite": "skill", "id": case["id"], "actual": actual, "expected": case["expected"]})
    for case in fixtures["soc_cases"]:
        actual = soc_decision(case["subject"], case["action"], case["case"])
        passed = actual == case["expected"]
        results["soc"].append({"id": case["id"], "pass": passed, "actual": actual})
        if not passed:
            failures.append({"suite": "soc", "id": case["id"], "actual": actual, "expected": case["expected"]})
    mutations = []
    for mutant, suite in [
        ("ignore_revocation", "agent"), ("ignore_exact_approval", "agent"),
        ("ignore_epoch", "soc"), ("risk_group_implies_event", "soc"), ("ignore_underlying_acl", "soc"),
        ("ignore_knowledge_recheck", "agent"), ("ignore_receipt_authorization", "agent"),
        ("ignore_apply_replay_authorization", "agent"),
        ("ignore_apply_plan_binding", "agent"),
        ("revalidate_completed_operation", "agent"),
    ]:
        caught = []
        cases = fixtures["agent_cases"] if suite == "agent" else fixtures["soc_cases"]
        for case in cases:
            if suite == "agent":
                actual = run_agent(case, "bounded_host_model", mutant)
                if any(actual[key] != value for key, value in case["expected"].items() if key != "forbidden_refs"):
                    caught.append(case["id"])
            elif soc_decision(case["subject"], case["action"], case["case"], mutant) != case["expected"]:
                caught.append(case["id"])
        mutations.append({"mutant": mutant, "caught_by": caught, "pass": bool(caught)})
        if not caught:
            failures.append({"suite": "mutation", "id": mutant})
    output = {
        "schema_version": "1", "data_scope": "synthetic_only", "real_model_or_enterprise_agent_or_soc_calls": 0,
        "counts": {key: len(value) for key, value in results.items()},
        "mutations": mutations, "failures": failures, "results": results,
        "scope_limit": "Executable design model and synthetic provider/host comparison; not product implementation, framework conformance, load test or real integration.",
    }
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(output, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in output.items() if key != "results"}, ensure_ascii=False, indent=2))
    raise SystemExit(bool(failures))


if __name__ == "__main__":
    main()
