#!/usr/bin/env python3
"""Read-only message-template dependency check before an explicitly chosen down range.

ROLLBACK_PREFLIGHT_DSN must name the database explicitly. Exit 0 means no
surviving template references in this scope, 1 means references block a down,
2 means this tool cannot establish the result. Never runs migrate/down/force.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
from urllib.parse import parse_qsl, unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
# These policies reproduce the checked historical DELETE predicates, including
# 079's template-qualified rule deletion. A changed down must be reviewed again.
POLICIES = {
    62: ("000062_ticket_cc.down.sql", "e52eafecc9fa18ee92ac3f842c7a62bfd0579e651e12c8331829523d6a14fd18",
         ["ticket_cc_notify"], [("ticket", "ticket.cc_added", None)]),
    64: ("000064_ticket_timeout.down.sql", "32873b3eb74bf473f59629f01a797b64e84902956f9a759a188f9c011fc2ff13",
         ["ops_cron_alert", "ticket_timeout_notify"], [("ops", "ops.cron_failed", None), ("ticket", "ticket.timeout", None)]),
    71: ("000071_sla_due_at.down.sql", "fc6716db905fb742775a0d01dcaed2a65d3dab860ab9cd9e6d654d2e4f42ee94",
         ["ticket_deadline_notify"], [("ticket", "ticket.deadline", None)]),
    79: ("000079_action_notify_seed.down.sql", "76470fefece3bd11d255de589a9c027955a17e2bb83f915839d173947be08667",
         ["ticket_action_notify"], [("ticket", "ticket.action_notify", "ticket_action_notify")]),
    80: ("000080_storage_usage_alert_seed.down.sql", "6fe0fb231bbe820d97d7c2bf88225deb1b5239eceb333b103f533c8f3cb07934",
         ["ops_storage_usage_alert"], [("ops", "ops.storage_usage_high", None)]),
}


def connection(value):
    if not value or re.search(r"[\x00-\x20\x7f]", value) or re.search(r"%(?![0-9a-fA-F]{2})", value):
        raise ValueError("ROLLBACK_PREFLIGHT_DSN must be an explicit PostgreSQL URI; no connection default is used")
    try:
        uri = urlsplit(value)
        port = uri.port
    except ValueError:
        raise ValueError("invalid PostgreSQL URI or port") from None
    if uri.scheme not in ("postgres", "postgresql") or not uri.hostname or uri.fragment:
        raise ValueError("invalid PostgreSQL URI")
    if not uri.username or uri.path in ("", "/") or "/" in unquote(uri.path[1:]):
        raise ValueError("URI must include one database name and an explicit user")
    if port is not None and not 1 <= port <= 65535:
        raise ValueError("invalid PostgreSQL port")
    decoded = [uri.hostname, unquote(uri.username), unquote(uri.path[1:])]
    if uri.password is not None:
        decoded.append(unquote(uri.password))
    if any(re.search(r"[\x00-\x1f\x7f]", part) for part in decoded):
        raise ValueError("URI components cannot contain control characters")
    if not re.fullmatch(r"[A-Za-z0-9_.:-]+", uri.hostname):
        raise ValueError("URI must name one TCP host; host lists and socket paths are unsupported")
    try:
        query = parse_qsl(uri.query, keep_blank_values=True, strict_parsing=True)
    except ValueError:
        raise ValueError("invalid URI query; accepts only sslmode/connect_timeout") from None
    if len({k for k, _ in query}) != len(query) or any(k not in ("sslmode", "connect_timeout") for k, _ in query):
        raise ValueError("URI query accepts only sslmode/connect_timeout, without duplicates")
    values = dict(query)
    if values.get("sslmode", "prefer") not in ("disable", "allow", "prefer", "require", "verify-ca", "verify-full"):
        raise ValueError("invalid sslmode")
    timeout = values.get("connect_timeout", "5")
    if not re.fullmatch(r"[1-9][0-9]?", timeout) or int(timeout) > 60:
        raise ValueError("connect_timeout must be 1..60 seconds")
    # Clear libpq environment overrides. Credentials never enter argv/output.
    env = {k: v for k, v in os.environ.items() if not k.startswith("PG")}
    env.update(PGHOST=uri.hostname, PGPORT=str(port or 5432), PGDATABASE=unquote(uri.path[1:]),
               PGUSER=unquote(uri.username), PGSSLMODE=values.get("sslmode", "prefer"),
               PGCONNECT_TIMEOUT=timeout, PGAPPNAME="zhuzhao-template-rollback-preflight")
    if uri.password is not None:
        env["PGPASSWORD"] = unquote(uri.password)
    return env, int(timeout)


def catalog():
    files = {}
    for path in (ROOT / "migrations").glob("*.down.sql"):
        match = re.fullmatch(r"([0-9]{6})_.+\.down\.sql", path.name)
        if not match or int(match[1]) in files:
            raise ValueError("invalid or duplicate local down migration number")
        files[int(match[1])] = path
    if not files:
        raise ValueError("local migrations are missing")
    return files


def snapshot_sql():
    # No user-supplied text is interpolated into SQL. A single read-only snapshot
    # sees checkpoint, incoming FKs and all rules used by the range simulation.
    return """BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL statement_timeout = '5s';
SET LOCAL lock_timeout = '2s';
SELECT json_build_object(
 'read_only', current_setting('transaction_read_only'),
 'checkpoint', (SELECT json_agg(s) FROM public.schema_migrations s),
 'incoming_foreign_keys', COALESCE((SELECT json_agg(f) FROM (
   SELECT c.conname AS name, n.nspname AS schema, r.relname AS relation,
    ARRAY(SELECT a.attname FROM unnest(c.conkey) WITH ORDINALITY k(num, ord)
      JOIN pg_catalog.pg_attribute a ON a.attrelid=c.conrelid AND a.attnum=k.num ORDER BY k.ord) AS columns,
    ARRAY(SELECT a.attname FROM unnest(c.confkey) WITH ORDINALITY k(num, ord)
      JOIN pg_catalog.pg_attribute a ON a.attrelid=c.confrelid AND a.attnum=k.num ORDER BY k.ord) AS target_columns,
    c.confdeltype AS on_delete, c.confupdtype AS on_update,
    c.convalidated AS validated, c.condeferrable AS deferrable
   FROM pg_catalog.pg_constraint c
   JOIN pg_catalog.pg_class r ON r.oid=c.conrelid
   JOIN pg_catalog.pg_namespace n ON n.oid=r.relnamespace
   WHERE c.contype='f' AND c.confrelid='public.message_templates'::regclass
   ORDER BY n.nspname, r.relname, c.conname
 ) f), '[]'::json),
 'rules', COALESCE((SELECT json_agg(r ORDER BY r.id) FROM (
   SELECT id, biz_type, event_action, template_code, recipient_type, enabled, version, created_by
   FROM public.message_rules
 ) r), '[]'::json)
);
ROLLBACK;
"""


def inspect(snapshot, target, files):
    if not isinstance(snapshot, dict) or snapshot.get("read_only") != "on":
        raise ValueError("database did not confirm a read-only transaction")
    checkpoints = snapshot.get("checkpoint")
    if not isinstance(checkpoints, list) or len(checkpoints) != 1:
        raise ValueError("schema_migrations must contain exactly one checkpoint")
    state = checkpoints[0]
    if not isinstance(state, dict):
        raise ValueError("invalid checkpoint snapshot")
    current = state.get("version")
    if type(current) is not int or type(state.get("dirty")) is not bool or state["dirty"]:
        raise ValueError("invalid or dirty checkpoint; inspect actual data before any checkpoint repair")
    if not 61 <= target < current or current not in files or target not in files:
        raise ValueError("supported target is an existing version >=61 and below the clean current local version")
    if any(v not in files for v in range(target + 1, current + 1)):
        raise ValueError("local down range is incomplete")
    plan = []
    for version in range(current, target, -1):
        path = files[version]
        data = path.read_bytes()
        if version in POLICIES:
            name, sha, codes, predicates = POLICIES[version]
            if path.name != name or hashlib.sha256(data).hexdigest() != sha:
                raise ValueError(f"checked down migration {version} changed; review its policy before use")
            plan.append((version, path, sha, codes, predicates))
        elif re.search(r"\bmessage_(templates|rules)\b", data.decode(), re.I):
            raise ValueError(f"unmodeled message-template/rule migration {version}; review before use")
    fks = snapshot.get("incoming_foreign_keys")
    if not isinstance(fks, list) or not fks:
        raise ValueError("expected template foreign-key guard is missing")
    for fk in fks:
        if not isinstance(fk, dict):
            raise ValueError("invalid foreign-key snapshot")
        if (fk.get("schema"), fk.get("relation"), fk.get("columns"), fk.get("target_columns"),
            fk.get("on_delete"), fk.get("on_update"), fk.get("validated"), fk.get("deferrable")) != (
                "public", "message_rules", ["template_code"], ["code"], "a", "a", True, False):
            raise ValueError("unmodeled incoming template foreign key; preflight cannot certify this schema")
    rules = snapshot.get("rules")
    fields = {"id", "biz_type", "event_action", "template_code", "recipient_type", "enabled", "version", "created_by"}
    if (not isinstance(rules, list) or any(not isinstance(r, dict) or set(r) != fields for r in rules)
            or any(type(r["id"]) is not int or type(r["version"]) is not int or r["version"] < 1
                   or type(r["enabled"]) is not bool
                   or (r["created_by"] is not None and type(r["created_by"]) is not int)
                   or any(not isinstance(r[key], str) for key in ("biz_type", "event_action", "template_code", "recipient_type"))
                   for r in rules) or len({r["id"] for r in rules}) != len(rules)):
        raise ValueError("invalid rule snapshot")
    steps = []
    for version, path, sha, codes, predicates in plan:
        removed = [r for r in rules if any(r["biz_type"] == biz and r["event_action"] == action
                   and (template is None or r["template_code"] == template) for biz, action, template in predicates)]
        removed_ids = {r["id"] for r in removed}
        rules = [r for r in rules if r["id"] not in removed_ids]
        blockers = [r for r in rules if r["template_code"] in codes]
        steps.append({"migration": version, "source": str(path.relative_to(ROOT)), "sha256": sha,
                      "templates_to_remove": codes, "rules_matching_down_delete": removed,
                      "surviving_references": blockers})
    blocked = [step["migration"] for step in steps if step["surviving_references"]]
    return {"status": "BLOCKED_TEMPLATE_REFERENCES" if blocked else "NO_TEMPLATE_REFERENCE_BLOCKERS",
            "scope": "message-template foreign-key references only; not approval of an entire rollback",
            "current_version": current, "target_version": target, "dirty": False,
            "read_only": True, "down_executed": False,
            "first_blocking_migration": blocked[0] if blocked else None,
            "requires_code_rollback": target <= 74,
            "incoming_foreign_keys": fks, "steps": steps,
            "warnings": (["Target <=74 also requires compatible code rollback; see runbook section 6"] if target <= 74 else []),
            "operator_actions": [
                "Review every rule matching a historical down DELETE; matching a seed key does not prove it was not customized.",
                "Review template customizations separately; this check does not certify that deleting seed templates is acceptable.",
                "For surviving references, explicitly decide to keep the template or rebind custom rules before rerunning preflight.",
                "This snapshot does not lock out later writes. Coordinate a maintenance window and rerun immediately before down.",
                "After a failed down, inspect actual data and dirty checkpoint together; do not blindly force a version or delete user configuration.",
            ]}, bool(blocked)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target-version", type=int, required=True, help="desired version after down; minimum 61")
    args = parser.parse_args()
    try:
        if args.target_version < 61:
            raise ValueError("target below 61 crosses the checked FK/table boundary and is unsupported")
        files = catalog()
        env, timeout = connection(os.environ.get("ROLLBACK_PREFLIGHT_DSN", ""))
        psql = shutil.which("psql")
        if not psql:
            raise ValueError("psql is required")
        run = subprocess.run([psql, "-X", "-w", "-qAt", "-v", "ON_ERROR_STOP=1"],
                             input=snapshot_sql(), text=True, capture_output=True, env=env, timeout=timeout + 15)
        if run.returncode:
            # Connection/server errors may echo credentials or user-created names.
            # Keep them out of a reportable result rather than persisting stderr.
            raise ValueError("PostgreSQL read failed; verify credentials, required schema and SELECT permissions; no down was run")
        result, blocked = inspect(json.loads(run.stdout), args.target_version, files)
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 1 if blocked else 0
    except (ValueError, TypeError, KeyError, OSError, subprocess.TimeoutExpired) as exc:
        print(json.dumps({"status": "PREFLIGHT_UNPROVEN", "error": str(exc), "down_executed": False}, ensure_ascii=False))
        return 2


if __name__ == "__main__":
    sys.exit(main())
