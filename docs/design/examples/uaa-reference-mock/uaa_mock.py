"""Single-source, offline governance contract model; not an IAM/IdP server."""
import copy
import hashlib
import json


def fingerprint(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(',', ':')).encode()).hexdigest()


class GovernanceMock:
    def __init__(self, profile, mutant=None):
        self.profile = copy.deepcopy(profile)
        self.mutant = mutant
        self.mode = 'standalone'
        self.desired = {}
        self.projected = {}
        self.seen = {}
        self.receipts = {}
        self.receipt_links = {}
        self.policy_revision = 0
        self.effective_revision = 0
        self.audit = []
        self.outage = False

    def record(self, action, result, event=None, actor_ref='system/mock', target_ref=''):
        # Only a fixed projection enters audit. Raw event data/code/token never does.
        row = {'schema_version': '1', 'audit_id': 'demo-audit-' + str(len(self.audit) + 1),
               'action': action, 'result': result, 'mode': self.mode,
               'source_ref': self.profile['source'], 'policy_revision': self.policy_revision,
               'actor_ref': actor_ref, 'target_ref': target_ref,
               'request_id': 'req-' + fingerprint('request-' + str(len(self.audit) + 1))[:32]}
        if isinstance(event, dict) and event:
            row['source_event_ref'] = fingerprint(str(event.get('event_id', '')))[:16]
        if self.mutant == 'leak_payload' and event:
            row['raw_event'] = event
        self.audit.append(row)
        return result

    def pending_dependencies(self):
        for event in self.desired.values():
            if event['kind'] != 'user_roles':
                continue
            if 'account:' + event['subject'] not in self.desired:
                return True
            if any('role_resources:' + role not in self.desired for role in event['payload']['roles']):
                return True
        return False

    def feed(self, event, caller):
        caller_ref = caller if caller == 'connector/mock-uaa' else 'untrusted-principal'
        finish = lambda result: self.record('feed', result, event, actor_ref=caller_ref)
        if caller != 'connector/mock-uaa' or not self.profile.get('connector_enabled', True):
            return finish('reject_context')
        if not isinstance(event, dict) and self.mutant != 'nonobject_envelope_guard':
            return finish('reject_envelope')
        keys = {'event_id', 'application', 'source', 'kind', 'subject', 'revision', 'complete', 'payload'}
        if (set(event) != keys
                or (self.mutant != 'untyped_kind_guard' and not isinstance(event.get('kind'), str))
                or event.get('kind') not in {'account', 'user_roles', 'role_resources'}
                or type(event.get('revision')) is not int or event['revision'] < 1
                or not all(isinstance(event.get(k), str) and 0 < len(event[k]) <= 128 for k in ['event_id', 'subject', 'source', 'application'])
                or type(event.get('complete')) is not bool or not isinstance(event.get('payload'), dict)):
            return finish('reject_envelope')
        if event['source'] != self.profile['source'] or event['application'] != self.profile['application']:
            return finish('reject_source')
        if not event['complete'] and self.mutant != 'trust_incomplete':
            return finish('reject_incomplete')
        kind, payload = event['kind'], event['payload']
        field = {'account': 'active', 'user_roles': 'roles', 'role_resources': 'resources'}[kind]
        if set(payload) != {field}:
            return finish('reject_envelope')
        if kind == 'account':
            if type(payload['active']) is not bool:
                return finish('reject_envelope')
        elif (not isinstance(payload[field], list) or len(payload[field]) > 50
              or any(not isinstance(x, str) or not 0 < len(x) <= 128 for x in payload[field])
              or len(set(payload[field])) != len(payload[field])):
            return finish('reject_envelope')
        if kind in {'account', 'user_roles'} and event['subject'] not in self.profile['mapping']:
            return finish('reject_identity_mapping')
        if kind == 'role_resources' and not set(payload[field]).issubset(self.profile['managed']):
            return finish('reject_resource')
        digest = fingerprint(event)
        if event['event_id'] in self.seen:
            return finish('duplicate' if self.seen[event['event_id']] == digest else 'reject_event_conflict')
        key = kind + ':' + event['subject']
        previous = self.desired.get(key)
        if previous and self.mutant != 'ignore_versions':
            if event['revision'] < previous['revision']:
                self.seen[event['event_id']] = digest
                self.receipts[event['event_id']] = 'ignore_stale'
                return finish('ignore_stale')
            if event['revision'] == previous['revision']:
                semantic = lambda e: {k: v for k, v in e.items() if k != 'event_id'}
                if semantic(event) != semantic(previous):
                    return finish('reject_version_conflict')
                if self.mutant != 'drop_duplicate_receipt_link':
                    self.seen[event['event_id']] = digest
                    self.receipt_links[event['event_id']] = previous['event_id']
                return finish('duplicate')
        # This in-memory assignment models a joint receipt+desired-state commit.
        # It is not a proof of a production database transaction.
        if previous and self.receipts.get(previous['event_id']) in {'held_dependency', 'committed_pending_effect'}:
            self.receipts[previous['event_id']] = 'superseded'
        self.desired[key] = copy.deepcopy(event)
        self.seen[event['event_id']] = digest
        self.policy_revision += 1
        own_dependency_missing = (kind == 'user_roles' and (
            'account:' + event['subject'] not in self.desired or any(
                'role_resources:' + role not in self.desired for role in payload['roles'])))
        result = 'held_dependency' if own_dependency_missing else 'committed_pending_effect'
        self.receipts[event['event_id']] = 'effective' if self.mutant == 'ack_before_policy' else result
        return finish(result)

    def apply_policy(self, success):
        if self.pending_dependencies():
            return self.record('policy_apply', 'held_dependency')
        if not success:
            return self.record('policy_apply', 'pending_reload')
        self.projected = copy.deepcopy(self.desired)
        self.effective_revision = self.policy_revision
        for event_id, status in self.receipts.items():
            if status in {'held_dependency', 'committed_pending_effect'} or self.mutant == 'mark_all_receipts_effective':
                self.receipts[event_id] = 'effective'
        return self.record('policy_apply', 'effective')

    def authorize(self, step):
        user, capability = step['user'], step['capability']
        actor_ref = user if user in self.profile['local_active'] else 'unknown-subject'
        registered = set(self.profile['managed']) | {c for caps in self.profile['local_permissions'].values() for c in caps}
        target_ref = capability if capability in registered else 'unregistered-capability'
        done = lambda status: self.record('authorize', status, actor_ref=actor_ref, target_ref=target_ref)
        if not self.profile['local_active'].get(user, False):
            return done('deny_disabled')
        if not step.get('token_valid', True) or not step.get('grant_valid', True):
            return done('deny_authorization')
        local = set(self.profile['local_permissions'].get(user, []))
        if self.mode == 'standalone':
            return done('allow' if capability in local else 'deny_authorization')
        subject = next((s for s, u in self.profile['mapping'].items() if u == user), None)
        account = self.desired.get('account:' + str(subject))
        # Tightening is immediate; central active cannot override a local deny.
        if not account or not account['payload']['active']:
            return done('deny_disabled')
        if capability not in self.profile['managed']:
            return done('allow' if capability in local else 'deny_authorization')
        if self.pending_dependencies() or self.effective_revision != self.policy_revision:
            return done('held_sync')
        roles = self.projected.get('user_roles:' + str(subject), {}).get('payload', {}).get('roles', [])
        allowed = set()
        for role in roles:
            allowed.update(self.projected.get('role_resources:' + role, {}).get('payload', {}).get('resources', []))
        if self.mutant == 'union_local_managed':
            allowed.update(local)
        if capability not in allowed:
            return done('deny_authorization')
        if self.mutant != 'ignore_freshness' and self.profile['now'] - self.profile['last_verified'] > self.profile['freshness_window']:
            return done('held_stale')
        if self.outage:
            effect = self.profile['capability_metadata'].get(capability, {}).get('effect')
            if effect not in {'read', 'write'}:
                return done('deny_authorization')
            is_write = step.get('write', False) if self.mutant == 'caller_controls_action_kind' else effect == 'write'
            if is_write:
                return done('held_outage_write')
            if self.profile['outage_policy'] != 'read_only_within_window':
                return done('held_outage_read')
        return done('allow')

    def exit_mode(self):
        if self.mutant != 'exit_without_approval' and not self.profile['exit_approved']:
            return self.record('exit', 'needs_exit_plan')
        if self.mutant != 'exit_without_approval' and not self.profile['local_credentials_ready']:
            return self.record('exit', 'needs_local_credentials')
        plan = self.profile.get('exit_plan')
        if plan is None:
            return self.record('exit', 'needs_exit_baseline')
        fields = {'identity_mapping', 'policy_revision', 'baseline_local_permissions', 'local_permissions'}
        known_users = set(self.profile['local_active'])
        known_capabilities = set(self.profile['capability_metadata'])

        def valid_permissions(value):
            return (isinstance(value, dict) and set(value) == known_users
                    and all(isinstance(caps, list) and all(isinstance(c, str) for c in caps)
                            and len(caps) == len(set(caps)) and set(caps).issubset(known_capabilities)
                            for caps in value.values()))

        if (not isinstance(plan, dict) or set(plan) != fields
                or not isinstance(plan['identity_mapping'], dict)
                or type(plan['policy_revision']) is not int or plan['policy_revision'] < 0
                or not valid_permissions(plan['baseline_local_permissions'])
                or not valid_permissions(plan['local_permissions'])):
            return self.record('exit', 'reject_exit_plan')
        # Approval is supplied by the trusted test harness, never by a business caller.
        if (self.mutant != 'ignore_exit_approval_digest'
                and self.profile.get('exit_plan_approval') != fingerprint(plan)):
            return self.record('exit', 'reject_exit_approval')
        if (self.mutant != 'ignore_exit_baseline'
                and (plan['identity_mapping'] != self.profile['mapping']
                     or plan['policy_revision'] != self.policy_revision
                     or plan['baseline_local_permissions'] != self.profile['local_permissions'])):
            return self.record('exit', 'needs_exit_repreview')
        if self.mutant != 'restore_old_exit_permissions':
            self.profile['local_permissions'] = copy.deepcopy(plan['local_permissions'])
        self.mode = 'standalone'
        return self.record('exit', self.mode)

    def dispatch(self, step):
        # Test-harness control inputs are synthetic authority, never accepted HTTP fields.
        op = step['op']
        if op == 'feed':
            return self.feed(step['event'], step.get('caller', 'connector/mock-uaa'))
        if op == 'activate':
            if not step.get('approved'):
                return self.record('takeover', 'needs_approval')
            mapping = self.profile['mapping']
            if len(set(mapping.values())) != len(mapping) or any(u not in self.profile['local_active'] for u in mapping.values()):
                return self.record('takeover', 'reject_identity_mapping')
            self.mode = 'enterprise'
            return self.record('takeover', self.mode)
        if op == 'apply_policy':
            return self.apply_policy(step.get('success', True))
        if op == 'authorize':
            return self.authorize(step)
        if op == 'receipt':
            allowed = step.get('caller', 'connector/mock-uaa') == 'connector/mock-uaa' and self.profile.get('connector_enabled', True)
            if not allowed:
                return 'not_found'
            event_id = self.receipt_links.get(step['event_id'], step['event_id'])
            return self.receipts.get(event_id, 'not_found')
        if op == 'advance':
            self.profile['now'] += step['seconds']
            return 'advanced'
        if op == 'outage':
            self.outage = True
            return self.record('outage', 'outage_marked')
        if op == 'heartbeat':
            if (step.get('caller', 'connector/mock-uaa') != 'connector/mock-uaa'
                    or not step.get('verified', True)
                    or (self.mutant != 'ignore_disabled_connector' and not self.profile.get('connector_enabled', True))):
                return self.record('freshness', 'reject_context')
            self.profile['last_verified'] = self.profile['now']
            self.outage = False
            return self.record('freshness', 'verified')
        if op == 'recovery':
            if step.get('caller', 'operator-demo') != self.profile['recovery_operator'] or self.profile['now'] > self.profile['recovery_until']:
                return self.record('recovery', 'deny_recovery')
            if step['action'] not in {'inspect', 'repair_connection', 'reconcile'}:
                return self.record('recovery', 'deny_recovery_action')
            return self.record('recovery', 'recovery_allowed')
        if op == 'exit':
            return self.exit_mode()
        if op == 'set_control':
            self.profile.update(copy.deepcopy(step['values']))
            return 'configured'
        if op == 'mode':
            return self.mode
        if op == 'history_owner':
            return self.profile['mapping']['subject-a']
        if op == 'revisions_equal':
            return self.policy_revision == self.effective_revision
        if op == 'restart':
            # JSON reload demonstrates state continuity only; not disk/PG crash durability.
            state = json.loads(json.dumps(self.__dict__))
            self.__dict__.update(state)
            return 'restored'
        raise ValueError('unknown fixture operation')
