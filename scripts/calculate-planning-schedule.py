#!/usr/bin/env python3
"""Repository-local T0-relative effort and deterministic capacity scheduler."""
from __future__ import annotations
import copy
import heapq
import re
import argparse
import collections
import hashlib
import json
import math
import posixpath
from pathlib import Path


def pair(value):
    if not isinstance(value, (str, list, tuple)):
        raise ValueError('invalid effort range type')
    nums = list(value) if isinstance(value, (list, tuple)) else [int(x) for x in re.findall(r'\d+', value)]
    if len(nums) != 2 or any(isinstance(x, bool) or not isinstance(x, (int, float)) for x in nums) or not 0 <= nums[0] <= nums[1]:
        raise ValueError('invalid effort range: ' + str(value))
    return tuple(nums)


def summed(values):
    values = [pair(x) for x in values]
    return tuple(sum(x[i] for x in values) for i in (0, 1))


def validate_dag(graph):
    done, active = set(), set()
    def visit(node):
        if node not in graph:
            raise ValueError('unknown dependency: ' + node)
        if node in active:
            raise ValueError('dependency cycle: ' + node)
        if node in done:
            return
        active.add(node)
        for dependency in sorted(graph[node]):
            visit(dependency)
        active.remove(node)
        done.add(node)
    for node in sorted(graph):
        visit(node)
    return len(done)


def build_local_core_projection(estimate, projection, target, parent_ids, owner_actions):
    """Project the existing local WBS; parent mappings are provenance, not prices.

    This is an engineering-effort ordering with unknown real-provider waiting,
    not a second release plan, a production approval, or a calendar forecast.
    """
    rows, groups = {}, []

    def check_parts(row):
        for field in ('suballocations', 'sub_wbs', 'wbs'):
            if field in row:
                parts = row[field]
                if not parts or summed(x['days'] for x in parts) != pair(row['days']):
                    raise ValueError('local suballocation sum mismatch: ' + row.get('id', field))
                for part in parts:
                    check_parts(part)
        if 'shared_allocation_to_S2' in row:
            gross, shared = pair(row['gross_days']), pair(row['shared_allocation_to_S2'])
            if tuple(gross[i] - shared[i] for i in (0, 1)) != pair(row['days']):
                raise ValueError('local shared allocation mismatch: ' + row['id'])

    seen_groups = set()
    for group in estimate['groups']:
        gid = group['id']
        if gid in seen_groups:
            raise ValueError('duplicate local group: ' + gid)
        seen_groups.add(gid)
        if not group['wbs'] or summed(row['days'] for row in group['wbs']) != pair(group['days']):
            raise ValueError('local group sum mismatch: ' + gid)
        groups.append(dict(id=gid, days=list(pair(group['days']))))
        for row in group['wbs']:
            key = row['id']
            if key in rows:
                raise ValueError('duplicate local WBS row: ' + key)
            check_parts(row)
            rows[key] = dict(group=gid, row=row)
    total = summed(value['row']['days'] for value in rows.values())
    if total != pair(estimate['totals']['raw_days']):
        raise ValueError('local WBS total mismatch')
    rounded = tuple(math.ceil(value) for value in total)
    if rounded != pair(estimate['totals']['rounded_display_days']):
        raise ValueError('local rounded total mismatch')
    if tuple(target['estimate_views']['full_release_net_days']) != rounded:
        raise ValueError('local current target view price drift')
    shared = summed(part.get('shared_allocation_to_S2', [0, 0])
                    for value in rows.values()
                    for part in value['row'].get('suballocations', []))
    conservative = tuple(total[i] + shared[i] for i in (0, 1))
    for name, expected in (
        ('conservative_without_shared_transfer_raw_days', conservative),
        ('conservative_without_shared_transfer_rounded_days', tuple(math.ceil(x) for x in conservative)),
    ):
        if name in estimate['totals'] and pair(estimate['totals'][name]) != expected:
            raise ValueError('local conservative total mismatch: ' + name)

    bindings = projection['node_bindings']
    binding_ids = [value['id'] for value in bindings]
    if len(set(binding_ids)) != len(binding_ids) or set(binding_ids) != set(rows):
        raise ValueError('local node binding coverage must match each priced WBS exactly once')
    conditions = projection['external_conditions']
    declared_actions = owner_actions.get('execution_actions', {})
    release_actions = {key: action for key, action in declared_actions.items()
                       if 'blocks_release_exits' in action}
    if set(conditions) != set(release_actions):
        raise ValueError('local condition coverage differs from independent owner release actions')
    resolved_conditions = {}
    for key, condition in conditions.items():
        ref = condition['action_ref']
        if ref != '/execution_actions/' + key:
            raise ValueError('local condition action reference mismatch: ' + key)
        action = release_actions.get(key)
        if not action or not action.get('state') or not action.get('evidence_state') or not isinstance(action.get('evidence_refs'), list):
            raise ValueError('local external condition action missing: ' + key)
        gates = action['blocks_release_exits']
        required = condition['required_exits']
        if (not isinstance(gates, list) or not gates or len(gates) != len(set(gates))
                or not isinstance(required, list) or not required or len(required) != len(set(required))
                or set(required) != set(gates)):
            raise ValueError('local condition gate binding differs from independent owner action: ' + key)
        resolved_conditions[key] = dict(source_ref='docs/planning/owner-decision-agenda.json#' + ref,
                                        state=action['state'], evidence_state=action['evidence_state'],
                                        evidence_refs=action['evidence_refs'], blocks_release_exits=gates)
    nodes = {}
    for position, binding in enumerate(bindings):
        key = binding['id']; parents = binding['parent_packages']
        if not parents or len(parents) != len(set(parents)) or not set(parents) <= set(parent_ids):
            raise ValueError('local node has unknown/duplicate parent mapping: ' + key)
        required = binding.get('external_condition_keys', [])
        if len(required) != len(set(required)) or not set(required) <= set(conditions):
            raise ValueError('local node has unknown external condition: ' + key)
        nodes[key] = dict(wbs_id=key, group=rows[key]['group'], days=pair(rows[key]['row']['days']),
                          parent_packages=parents, dependencies=set(binding['dependencies']),
                          priority=binding.get('priority', position), external_condition_keys=required)
    validate_dag({key: value['dependencies'] for key, value in nodes.items()})

    def closure(targets):
        selected = set(targets)
        if not selected or not selected <= set(nodes):
            raise ValueError('local journey/exit coverage references unknown or empty WBS')
        pending = list(selected)
        while pending:
            for dependency in nodes[pending.pop()]['dependencies']:
                if dependency not in selected:
                    selected.add(dependency); pending.append(dependency)
        return selected

    requirements = target['must_have']
    expected_refs = {'/must_have/' + str(i) for i in range(len(requirements))}
    journeys = projection['required_journeys']
    refs = [value['requirement_ref'] for value in journeys]
    if len(refs) != len(set(refs)) or set(refs) != expected_refs:
        raise ValueError('local required journey coverage drift')
    coverage, covered = [], set()
    for journey in journeys:
        index = int(journey['requirement_ref'].rsplit('/', 1)[1])
        text = requirements[index]
        digest = hashlib.sha256(text.encode('utf-8')).hexdigest()
        if journey['requirement_sha256'] != digest:
            raise ValueError('local requirement meaning changed: ' + journey['requirement_ref'])
        selected = closure(journey['wbs_ids']); covered |= selected
        coverage.append(dict(requirement_ref='docs/planning/first-usable-release-target.json#' + journey['requirement_ref'],
                             text=text, requirement_sha256=digest, targets=journey['wbs_ids'],
                             dependency_closure=sorted(selected),
                             overlapping_effort_do_not_add=list(summed(nodes[key]['days'] for key in selected))))
    if covered != set(nodes):
        raise ValueError('local WBS has no required journey coverage: ' + ', '.join(sorted(set(nodes) - covered)))

    exits = projection['delivery_exits']
    if 'formal_local' not in exits:
        raise ValueError('local formal gate missing')
    if closure(exits['formal_local']['targets']) != set(nodes):
        raise ValueError('local formal WBS coverage must include every selected required journey')
    for key, condition in conditions.items():
        for exit_id in condition['required_exits']:
            if exit_id not in exits or key not in exits[exit_id]['condition_keys']:
                raise ValueError('local condition missing from required gate: ' + key)
    exit_rows = {}
    for key, value in exits.items():
        required = value['condition_keys']
        if len(required) != len(set(required)) or not set(required) <= set(conditions) or not value.get('evidence_ref'):
            raise ValueError('local release gate condition/evidence binding invalid: ' + key)
        selected = closure(value['targets'])
        exit_rows[key] = dict(targets=value['targets'], dependency_closure=sorted(selected),
                              overlapping_effort_do_not_add=list(summed(nodes[item]['days'] for item in selected)),
                              condition_keys=required, evidence_ref=value['evidence_ref'],
                              readiness='not_verified_by_schedule', actual_activation_date=None)
    blocked = sorted(key for key in nodes if any(nodes[ancestor]['external_condition_keys'] for ancestor in closure([key])))
    resources = {}
    for seats in (1, 2):
        runs = [run_scheduler(nodes, seats, endpoint) for endpoint in (0, 1)]
        resources[str(seats)] = dict(
            engineering_makespan_if_conditions_ready=[run['makespan_workdays'] for run in runs],
            assumption='finite engineering ordering only; real input availability/waiting is not scheduled as zero',
            resource_records={label: run['records'] for label, run in zip(('low', 'high'), runs)},
        )
        for exit_id, value in exit_rows.items():
            value.setdefault('engineering_finish_if_conditions_ready', {})[str(seats)] = [
                max(run['records'][key]['finish'] for key in value['dependency_closure']) for run in runs]
    return dict(status='bounded_local_core_candidate_not_implemented', raw_engineering_days=list(total),
                rounded_display_days=list(rounded), conservative_without_shared_transfer_raw_days=list(conservative),
                conservative_without_shared_transfer_rounded_days=[math.ceil(x) for x in conservative],
                groups=groups, nodes={key: dict(value, days=list(value['days']), dependencies=sorted(value['dependencies'])) for key, value in sorted(nodes.items())},
                required_journeys=coverage, external_conditions=resolved_conditions, delivery_exits=exit_rows,
                resources=resources, externally_blocked_wbs=blocked,
                unblocked_public_wbs=sorted(set(nodes) - set(blocked)),
                external_waiting_days=None, calendar=None, formal_activation_approved=False,
                added_to_long_horizon_total=False,
                price_rule='Each local WBS row is charged once; mapped 76-parent prices/ancestors are never added. Journey/exit efforts overlap and must not be added.')


def current_core_summary(estimate):
    raw = pair(estimate['totals']['raw_days'])
    rounded = pair(estimate['totals']['rounded_display_days'])
    shared = summed(part.get('shared_allocation_to_S2', [0, 0])
                    for group in estimate['groups'] for row in group['wbs']
                    for part in row.get('suballocations', []))
    conservative = [math.ceil(raw[i] + shared[i]) for i in (0, 1)]
    return (f'2026-11-16 首版净估对账底稿：**{format_pair(rounded)} 个有效工程日**'
            f'（未取整 {format_pair(raw)}；S2 共用假设不成立时 {format_pair(conservative)}）。'
            '这是单席工程量候选，不是日历承诺。')


def validate_current_view_docs(repo, model, estimate):
    paths = model['current_view_docs']
    if not paths or len(paths) != len(set(paths)):
        raise ValueError('current view document registry empty or duplicated')
    summary = current_core_summary(estimate)
    result = {}
    for path in paths:
        relative = Path(path)
        if relative.is_absolute() or '..' in relative.parts or (repo / relative).is_symlink():
            raise ValueError('invalid current view document path: ' + path)
        text = (repo / relative).read_text()
        start, end = '<!-- local-core-current:start -->', '<!-- local-core-current:end -->'
        if text.count(start) != 1 or text.count(end) != 1:
            raise ValueError('current view marker missing or duplicated: ' + path)
        match = re.search(re.escape(start) + r'\s*(.*?)\s*' + re.escape(end), text, re.S)
        if not match or match.group(1).strip() != summary:
            raise ValueError('current local core view drift: ' + path)
        result[path] = dict(summary_sha256=hashlib.sha256(summary.encode('utf-8')).hexdigest(), check='matches_net_estimate')
    return result


def validate_current_source_links(repo, model):
    """Five non-authoritative pages carry a checked link, not copied prices."""
    paths = model['current_source_link_docs']
    if not paths or len(paths) != len(set(paths)):
        raise ValueError('current source view registry empty or duplicated')
    source = Path(model['local_core_net_estimate_ref'])
    if (source.as_posix() != 'docs/planning/local-core-release-estimate.json'
            or (repo / source).is_symlink() or not (repo / source).is_file()):
        raise ValueError('current source view authority missing or invalid')
    result = {}
    for path in paths:
        relative = Path(path)
        if relative.is_absolute() or '..' in relative.parts or (repo / relative).is_symlink():
            raise ValueError('invalid current source view path: ' + path)
        href = posixpath.relpath(source.as_posix(), relative.parent.as_posix())
        summary = f'当前首版本地核心净估与条件以[唯一净估册]({href})为准；本文不另立总价。'
        text = (repo / relative).read_text()
        start, end = '<!-- local-core-source:start -->', '<!-- local-core-source:end -->'
        if text.count(start) != 1 or text.count(end) != 1:
            raise ValueError('current source view marker missing or duplicated: ' + path)
        match = re.search(re.escape(start) + r'\s*(.*?)\s*' + re.escape(end), text, re.S)
        if not match or match.group(1).strip() != summary:
            raise ValueError('current source view drift: ' + path)
        result[path] = dict(source_ref=source.as_posix(),
                            summary_sha256=hashlib.sha256(summary.encode('utf-8')).hexdigest(),
                            check='matches_source_only_summary')
    return result


def uaa_capacity_binding(model):
    binding = model.get('uaa_capacity_gate_binding', {})
    if (binding.get('local_core_exit_id') != 'uaa_capacity_transition'
            or binding.get('owner_action_key') != 'Q12_core_stability'):
        raise ValueError('UAA main capacity must bind the existing core stability exit/action')
    return binding


def validate_uaa_capacity_gate(model, core):
    binding = uaa_capacity_binding(model)
    exit_id, action_key = binding['local_core_exit_id'], binding['owner_action_key']
    gate = core['delivery_exits'].get(exit_id)
    condition = core['external_conditions'].get(action_key)
    if (not gate or not condition or action_key not in gate['condition_keys']
            or exit_id not in condition['blocks_release_exits']
            or not set(core['delivery_exits']['formal_local']['dependency_closure'])
            <= set(gate['dependency_closure'])):
        raise ValueError('UAA main capacity core stability evidence/coverage binding missing')
    return dict(local_core_exit_id=exit_id, owner_action_key=action_key,
                gate_ref='local_core.delivery_exits.' + exit_id,
                evidence_ref=gate['evidence_ref'], readiness=gate['readiness'],
                preparation_contact_is_blocked=False, added_engineering_days=[0, 0],
                rule=binding['scope'])


def run_scheduler(nodes, seats, endpoint):
    if seats < 1 or endpoint not in (0, 1):
        raise ValueError('invalid capacity or endpoint')
    graph = {k: set(v['dependencies']) for k, v in nodes.items()}
    validate_dag(graph)
    successors = {k: [] for k in nodes}
    for k, deps in graph.items():
        for d in deps:
            successors[d].append(k)
    tails = {}
    def duration(k):
        return pair(nodes[k]['days'])[endpoint]
    def tail(k):
        if k not in tails:
            tails[k] = duration(k) + max((tail(x) for x in successors[k]), default=0)
        return tails[k]
    for k in nodes:
        tail(k)
    pending, done, running, records = set(nodes), set(), [], {}
    free, now = list(range(1, seats + 1)), 0
    while pending or running:
        while True:
            zero = [k for k in sorted(pending) if duration(k) == 0 and graph[k] <= done]
            if not zero:
                break
            for k in zero:
                pending.remove(k); done.add(k)
                records[k] = dict(start=now, finish=now, slot=None, engineering_days=0)
        ready = sorted((k for k in pending if graph[k] <= done), key=lambda k: (nodes[k].get('priority', 99), -tails[k], k))
        while free and ready:
            k = ready.pop(0); pending.remove(k); slot = heapq.heappop(free)
            records[k] = dict(start=now, finish=now + duration(k), slot=slot, engineering_days=duration(k))
            heapq.heappush(running, (now + duration(k), slot, k))
        if not running:
            if pending:
                raise ValueError('unschedulable tasks')
            break
        now = running[0][0]
        while running and running[0][0] == now:
            _, slot, k = heapq.heappop(running); done.add(k); heapq.heappush(free, slot)
    for k in nodes:
        if any(records[d]['finish'] > records[k]['start'] for d in graph[k]):
            raise ValueError('precedence violation')
    for slot in range(1, seats + 1):
        seq = sorted((v['start'], v['finish']) for v in records.values() if v['slot'] == slot)
        if any(a[1] > b[0] for a, b in zip(seq, seq[1:])):
            raise ValueError('resource overlap')
    effort = sum(duration(k) for k in nodes)
    if seats == 1 and now != effort:
        raise ValueError('single seat lost capacity')
    return dict(makespan_workdays=now, effort_engineering_days=effort, critical_path_lower_bound=max(tails.values(), default=0), records=records)


def apply_scope(backlog, estimates):
    result = copy.deepcopy(backlog)
    items = {x['id']: x for x in result['work_items']}
    if len(items) != len(result['work_items']):
        raise ValueError('duplicate parent ID')
    rows = estimates['items']
    if len({x['id'] for x in rows}) != len(rows):
        raise ValueError('duplicate scope ID')
    if summed(x['net_days'] for x in rows) != pair(estimates['net_increment_total']):
        raise ValueError('scope increment total mismatch')
    extras = []
    for row in rows:
        if summed(w['days'] for w in row['wbs']) != pair(row['net_days']):
            raise ValueError('WBS sum mismatch: ' + row['id'])
        owner, kind = row['owner_package'], row['kind']
        if kind == 'new_capability_scope':
            if owner not in items:
                item = dict(id=owner, title=row['title'], wave=row['wave'], dependencies=row['dependencies'], estimate_engineering_days=list(row['net_days']), state='planned_not_implemented')
                result['work_items'].append(item); items[owner] = item
            elif pair(items[owner]['estimate_engineering_days']) != pair(row['net_days']):
                raise ValueError('new capability price drift: ' + owner)
        elif owner not in items:
            raise ValueError('unknown owner: ' + owner)
        elif kind == 'reprice_existing_base':
            item = items[owner]
            if tuple(b-a for a,b in zip(pair(row['baseline_days']),pair(row['replacement_days']))) != pair(row['net_days']):
                raise ValueError('replacement increment mismatch')
            current = pair(item['estimate_engineering_days'])
            if current == pair(row['baseline_days']):
                item['estimate_engineering_days'] = list(row['replacement_days'])
                item['estimate_adjustment_ref'] = row['id']
            elif current != pair(row['replacement_days']) or item.get('estimate_adjustment_ref') != row['id']:
                raise ValueError('baseline price drift: ' + owner)
        elif kind == 'additional_existing_scope':
            extras.append(copy.deepcopy(row))
        else:
            raise ValueError('unknown scope kind: ' + kind)
    return result, extras


def source_graph(backlog):
    graph, milestones = {}, {}
    for item in backlog['work_items']:
        external = item['dependencies'] + item.get('milestone_dependencies', [])
        scoped = [x['milestone'] for x in item.get('scoped_milestone_dependencies', [])]
        graph[item['id']] = set(external + scoped + [x['id'] for x in item.get('milestones', [])])
        for m in item.get('milestones', []):
            if m['id'] in milestones or m['id'] in graph:
                raise ValueError('duplicate milestone: ' + m['id'])
            milestones[m['id']] = m
            graph[m['id']] = set((external if m.get('inherit_parent_dependencies', True) else []) + m['dependencies'] + [x['milestone'] for x in m.get('scoped_milestone_dependencies', [])])
    validate_dag(graph)
    return graph, milestones


def build_model(backlog, model, scope, uaa, include_r=False, reserve=True):
    capacity_gate = uaa_capacity_binding(model)
    backlog, extras = apply_scope(backlog, scope)
    items = {x['id']:x for x in backlog['work_items']}
    if not set(model['original_parent_ids']) <= set(items):
        raise ValueError('original scope missing')
    graph, milestones = source_graph(backlog)
    if not set(model['original_child_ids']) <= set(milestones):
        raise ValueError('original milestone missing')
    nodes = {}
    def add(k, owner, days, deps, category, wave, **extra):
        if k in nodes:
            raise ValueError('duplicate task: ' + k)
        nodes[k] = dict(id=k, owner_package=owner, days=pair(days), dependencies=set(deps), effort_category=category, wave=wave, **extra)
    completed = set(model['completed_parent_ids'])
    for item in items.values():
        k, wave = item['id'], item['wave']
        if k in completed:
            if 'locally_validated' not in item['state'] and 'complete' not in item['state']:
                raise ValueError('historical item not completed: ' + k)
            add(k,k,[0,0],[],'historical_completed',wave,historical_days=pair(item['estimate_engineering_days']))
        elif k == 'E01':
            add('E01-BASE',k,item['estimate_engineering_days'],item['dependencies']+item.get('milestone_dependencies',[]),'existing_remaining','P7')
            add(k,k,[0,0],['E01-BASE','E01-UAA-C','E01-UAA-P']+(['E01-UAA-R'] if include_r else []),'aggregation','P7')
        elif item.get('milestones'):
            if summed(x['estimate_engineering_days'] for x in item['milestones']) != pair(item['estimate_engineering_days']):
                raise ValueError('parent/child decomposition mismatch: '+k)
            add(k,k,[0,0],graph[k],'aggregation',wave)
            for child in item['milestones']:
                cid = child['id']
                if cid.startswith('E01-UAA-'):
                    continue
                if cid == 'CE02-L':
                    parts = model['ce02_migration_parts']
                    if summed(x['estimate'] for x in parts) != pair(child['estimate_engineering_days']):
                        raise ValueError('CE02-L decomposition mismatch')
                    for part in parts:
                        add(part['id'],k,part['estimate'],part['dependencies'],'new_capability',part['wave'],scope=part['scope'],official_child=cid)
                    add(cid,k,[0,0],graph[cid] | {p['id'] for p in parts},'aggregation','P9')
                else:
                    add(cid,k,child['estimate_engineering_days'],graph[cid],'new_capability' if k.startswith('CE') else 'existing_remaining',child.get('wave',wave))
        else:
            add(k,k,item['estimate_engineering_days'],graph[k],'new_capability' if k.startswith('CE') else 'existing_remaining',wave)
    for extra in extras:
        add(extra['id'],extra['owner_package'],extra['net_days'],extra['dependencies'],'legacy_additional_scope',extra['wave'],scope_ref=extra['scope_ref'])
    c = milestones['E01-UAA-C']
    if pair(c['estimate_engineering_days']) != pair(uaa['C']['engineering_days']):
        raise ValueError('UAA C price mismatch')
    add(c['id'],'E01',c['estimate_engineering_days'],graph[c['id']],'uaa_contract','P6.0')
    wbs = {x['id']:x for x in uaa['P']['wbs']}
    if pair(milestones['E01-UAA-P']['estimate_engineering_days']) != pair(uaa['P']['engineering_days']) or pair(milestones['E01-UAA-R']['estimate_engineering_days']) != pair(uaa['R']['engineering_days']):
        raise ValueError('UAA milestone price mismatch')
    if not {'E01-UAA-C','C04','X02-F'} <= graph['E01-UAA-P']:
        raise ValueError('UAA public shared dependency missing')
    if set(wbs) != set(model['uaa_p_dependencies']) or summed(x['engineering_days'] for x in wbs.values()) != pair(uaa['P']['engineering_days']):
        raise ValueError('UAA public WBS mismatch')
    pparts = []
    for wid,row in wbs.items():
        k='UAA-P-'+wid;pparts.append(k)
        deps=['UAA-P-'+x[1:] if x.startswith('@') else x for x in model['uaa_p_dependencies'][wid]]
        add(k,'E01',row['engineering_days'],deps,'uaa_public','P6.1',wbs_id=wid)
    add('E01-UAA-P','E01',[0,0],graph['E01-UAA-P']|set(pparts),'aggregation','P6.1')
    if include_r:
        add('UAA-R-ENGINEERING','E01',uaa['R']['engineering_days'],graph['E01-UAA-R'],'uaa_real','conditional_real',external_ready='UAA_TEST_ENTRY_INPUT_READY')
        add('E01-UAA-R','E01',[0,0],['UAA-R-ENGINEERING'],'aggregation','conditional_real')
    if reserve:
        add('MAINTENANCE-CLOSEOUT','maintenance',scope['baseline']['maintenance_reserve'],[],'maintenance_capacity_reserve','pre_business')
        for k,node in nodes.items():
            if k not in completed and k != 'MAINTENANCE-CLOSEOUT':
                node['dependencies'].add('MAINTENANCE-CLOSEOUT')
    validate_dag({k:n['dependencies'] for k,n in nodes.items()})
    # Every positive task is covered once by a release slice or the explicit UAA/reserve lane.
    direct={k:s['id'] for s in model['release_slices'] for k in s['items']}
    if sum(len(s['items']) for s in model['release_slices']) != len(direct):
        raise ValueError('duplicate slice membership')
    ranks={s['id']:i for i,s in enumerate(model['release_slices'])}
    for k,node in nodes.items():
        category=node['effort_category']
        if category in ('uaa_public', 'uaa_real'):
            node['capacity_gate_ref'] = 'local_core.delivery_exits.' + capacity_gate['local_core_exit_id']
        sid=direct.get(k)
        if not sid and category == 'existing_remaining':
            sid=direct.get(node['owner_package'])
        if category in ['existing_remaining','new_capability','legacy_additional_scope'] and not sid:
            raise ValueError('unsliced task: '+k)
        node['slice']=sid if category in ['existing_remaining','new_capability','legacy_additional_scope'] else None
        node['priority']=ranks.get(sid,99)+1
    def ancestors(k):
        seen=set()
        def walk(x):
            for dep in nodes[x]['dependencies']:
                if dep not in seen:seen.add(dep);walk(dep)
        walk(k);return seen
    priorities=model['delivery_priority_groups']
    for node in nodes.values():
        if node['effort_category']=='uaa_public':
            node['priority']=priorities['public_uaa_positive_tasks']['priority']
    def promote(group):
        for target in group['targets']:
            if target not in nodes:
                raise ValueError('unknown delivery priority target: '+target)
            for k in ancestors(target)|{target}:
                nodes[k]['priority']=min(nodes[k]['priority'],group['priority'])
    promote(priorities['foundation'])
    promote(priorities['pre_risk_investigation'])
    promote(priorities['risk_first_chain'])
    if include_r:
        # Ready enterprise engineering is a distinct, selected capacity scenario.
        # Priority does not satisfy any real pre-apply or production evidence gate.
        promote(priorities['enterprise_when_selected_ready'])
    if reserve:
        nodes['MAINTENANCE-CLOSEOUT']['priority']=-1
    categories={cat:summed(n['days'] for n in nodes.values() if n['effort_category']==cat) for cat in sorted({n['effort_category'] for n in nodes.values()})}
    expected=summed([scope['baseline']['with_conditional_uaa_R' if include_r else 'public_remaining'],scope['net_increment_total']]+([scope['baseline']['maintenance_reserve']] if reserve else []))
    total=summed(n['days'] for n in nodes.values())
    if total != expected:
        raise ValueError('effort sum mismatch: '+str((total,expected,categories)))
    return nodes,categories,total


def project_phase6_audit(estimate, core, long_reference, uaa):
    audit = estimate['phase6_design_audit_projection']
    mainline = estimate['mainline_release_scenario']
    residuals = mainline['residual_core_allocations']
    baseline = core['nodes']
    s1 = next(row for group in estimate['groups'] for row in group['wbs'] if row['id'] == 'S1')
    original_parts = {row['id']: pair(row['days']) for row in s1['suballocations']}
    if set(residuals) != {'S1', 'S3'} or set(residuals['S1']) != set(original_parts):
        raise ValueError('mainline residual allocations must cover every S1 component and S3 exactly')
    for key, original in original_parts.items():
        if any(remaining > initial for remaining, initial in zip(pair(residuals['S1'][key]), original)):
            raise ValueError('mainline residual exceeds original component: ' + key)
    if any(remaining > initial for remaining, initial in zip(pair(residuals['S3']), baseline['S3']['days'])):
        raise ValueError('mainline residual exceeds original component: S3')
    savings = tuple(old - remaining for old, remaining in zip(
        summed(baseline[key]['days'] for key in residuals),
        summed([summed(residuals['S1'].values()), residuals['S3']]),
    ))
    trial = core['delivery_exits']['risk_ticket_trial']['dependency_closure']
    formal = core['delivery_exits']['formal_local']['dependency_closure']
    later_nodes = set(formal) - set(trial)
    later_transfer_keys = later_nodes | {baseline[node]['group'] for node in later_nodes}
    additions = audit['first_trial_additions'].values()
    frontloaded = summed(row['days'] for row in additions)
    transfers = {}
    for row in additions:
        row_transfers = row.get('transfer_from_later', {})
        if any(days > addition for days, addition in zip(summed(row_transfers.values()), pair(row['days']))):
            raise ValueError('first-trial transfer exceeds its additional estimate')
        for key, days in row_transfers.items():
            if key not in later_transfer_keys:
                raise ValueError('first-trial transfer has no later component: ' + key)
            if key in transfers:
                raise ValueError('duplicate first-trial transfer: ' + key)
            transfers[key] = pair(days)
    combination = set(trial) | set(audit['combination_targets'])
    pending = list(combination)
    while pending:
        for dependency in baseline[pending.pop()]['dependencies']:
            if dependency not in combination:
                combination.add(dependency)
                pending.append(dependency)

    def included_transfer(keys):
        # A transfer can refer to one priced component of a later group.
        return summed(days for key, days in transfers.items()
                      if any(node == key or baseline[node]['group'] == key for node in keys))

    if included_transfer(trial) != (0, 0) or not set(trial) < combination < set(formal):
        raise ValueError('invalid first-trial transfer or dispatch closure')

    stages = []
    core_reserve = core['conservative_without_shared_transfer_raw_days'][1] - core['raw_engineering_days'][1]
    for key, nodes in [('risk_ticket_trial', trial), ('combination', combination), ('formal_local', formal)]:
        original = summed(baseline[node]['days'] for node in nodes)
        brought_forward = included_transfer(nodes)
        revised = tuple(original[i] - savings[i] + frontloaded[i] - brought_forward[i] for i in (0, 1))
        conservative = original[1] + frontloaded[1] - brought_forward[1] + core_reserve
        stages.append(dict(id=key, baseline_days=list(original), transferred_once_days=list(brought_forward),
                           engineering_candidate_days=list(revised), conservative_days=conservative))

    later = {row['id']: row['estimate'] for row in long_reference['ordered_tasks']}
    shared = summed([later['X01-C'], *mainline['uaa_shared']['local_reuse_if_verified'].values()])
    shared_before_reuse = summed(later[key] for key in ('X01-C', 'D06', 'X02-F'))
    public_days = pair(uaa['P']['engineering_days'])
    real_days = pair(uaa['R']['engineering_days'])
    for key, added, reserve_added in [
        ('uaa_public', summed([shared, public_days]), shared_before_reuse[1] + public_days[1]),
        ('uaa_managed', real_days, real_days[1]),
    ]:
        previous = stages[-1]
        stages.append(dict(id=key, conditional_full_governance=True,
                           engineering_candidate_days=list(summed([previous['engineering_candidate_days'], added])),
                           conservative_days=previous['conservative_days'] + reserve_added))

    full = audit['full_phase6_exposure']
    phase6_slices = {row['id']: row['effort'] for row in long_reference['release_slices']
                     if row['wave'].startswith('P6.')}
    if len(full['slices']) != len(phase6_slices) or set(full['slices']) != set(phase6_slices):
        raise ValueError('full Phase 6 projection must include each Phase 6 slice exactly once')
    base = summed([later['E01-UAA-C'], *phase6_slices.values()])
    standalone = tuple(base[i] - pair(later['C05'])[i] + pair(full['C05_replacement'])[i] for i in (0, 1))
    adjusted_iam = tuple(pair(full['iam_full_replacement'])[i] - summed([later['C04'], later['C05']])[i]
                         for i in (0, 1))
    integrated = summed([base, public_days, adjusted_iam, (0, core_reserve), full['obs_local_first_two_net'],
                         full['preselected_local_release_forward_slice'], full['dispositions_and_F004_bounded_allowance']])
    return dict(ref=audit['ref'], state=audit['state'], actual_T0=audit['actual_T0'],
                external_wait_days=audit['external_wait_days'], stages=stages, gates=audit['gates'],
                baseline_credits=list(savings), trial_additions=list(frontloaded),
                shared_uaa_days=list(shared), full_phase6_baseline_excluding_UAA_product=list(base),
                full_phase6_standalone_candidate_days=list(standalone),
                full_phase6_with_public_and_local_release_days=list(integrated),
                full_phase6_through_real_UAA_days=list(summed([integrated, real_days])))


def calculate(repo):
    paths=['docs/planning/planning-backlog.json','docs/planning/execution-schedule-model.json','docs/planning/planning-scope-estimates.json','docs/planning/uaa-implementation-estimate.json','docs/planning/uaa-real-integration-conditions.json','docs/planning/local-core-release-estimate.json','docs/planning/first-usable-release-target.json','docs/planning/owner-decision-agenda.json']
    blobs={p:(repo/p).read_bytes() for p in paths}
    backlog,model,scope,uaa,conditions,local_estimate,local_target,owner_actions=[json.loads(blobs[p]) for p in paths]
    sso = backlog['release_gates']['company_sso_local_authorization']
    if (sso['estimate_status'] != 'pending_net_estimate'
            or sso['engineering_days'] is not None
            or sso['included_in_priced_schedule_baseline'] is not False):
        raise ValueError('SSO-only delivery must stay outside the priced baseline until its estimate is integrated')
    graph,milestones=source_graph(backlog)
    result=dict(date=model['date'],status='priced_baseline_with_unpriced_sso_successor_not_implemented',calendar=None,estimate_source=paths,source_input_sha256={p:hashlib.sha256(blobs[p]).hexdigest() for p in paths},resource_assumptions=model['resource_assumptions'],source_structure=dict(parents=len(backlog['work_items']),official_child_milestones=len(milestones),additional_scope_tasks=sum(x['kind']=='additional_existing_scope' for x in scope['items']),release_slices=len(model['release_slices']),merged_dag=True,source_nodes=len(graph)),scenarios={},release_slices=model['release_slices'],pre_business_maintenance=dict(work_items=model['completed_parent_ids'],status='historical_version_maintenance_delivered; retained_reserve_profiles_are_conditional_comparisons_not_current_rebilling',do_not_repeat_future_coding_estimate=True,actual_record_ref='docs/planning/core-release-accounting-reconciliation-2026-11-16.md',reserve_profile_rule='Keep original with/without reserve arithmetic for comparison; completed version-maintenance work is not dispatched or charged again in local_core'),release_gates_ref='docs/planning/planning-backlog.json#/release_gates',external_waiting=dict(ref='docs/planning/uaa-real-integration-conditions.json',entry_and_activation_stages=conditions['entry_and_activation_stages'],additional_refs=model['external_waiting_refs'],rule='Waiting, approval, provider fixes and formal activation dates are outside engineering effort. Public mock/contract delivery is not blocked by unavailable real providers.'),endpoint_semantics=model['endpoint_semantics'],reference_algorithm=dict(kind='deterministic list scheduling; priority then remaining critical chain then ID',reproduction='python3 scripts/calculate-planning-schedule.py --check',runtime_acceptance_claimed=False),active_work_boundary='Design/schedule only; T0 not started; no business development or activation authorized')
    result['delivery_scope_boundaries']=dict(foundation_targets=model['delivery_priority_groups']['foundation']['targets'],risk_first_chain_targets=model['delivery_priority_groups']['risk_first_chain']['targets'],effort_rule='Each delivery exit reports the union of its named targets and actual dependencies. Shared ancestors overlap; do not add the exit efforts.',long_horizon_totals_are_not_foundation_duration=True,baseline_scope_only=True,post_baseline_supplements_not_merged=['F004 newly selected details pending net estimate','OBS01–03 separately priced candidate supplement'],supplement_ref='docs/planning/post-baseline-scope-estimates.json',production_or_enterprise_activation_approved=False)
    for include_r in (False,True):
        key='with_conditional_R_ready' if include_r else 'public_R_pending'
        scenario=dict(conditional_R_selected=include_r,profiles={})
        for reserve in (False,True):
            nodes,categories,total=build_model(backlog,model,scope,uaa,include_r,reserve)
            def closure(targets):
                selected=set(targets)
                pending=list(targets)
                while pending:
                    for dependency in nodes[pending.pop()]['dependencies']:
                        if dependency not in selected:
                            selected.add(dependency);pending.append(dependency)
                return selected
            profile=dict(total_effort_range=list(total),category_effort={k:list(v) for k,v in categories.items()},model_nodes=len(nodes),positive_tasks=sum(n['days']!=(0,0) for n in nodes.values()),resources={})
            for seats in (1,2):
                runs=[run_scheduler(nodes,seats,i) for i in (0,1)]
                def window(k,field='finish'):return [x['records'][k][field] for x in runs]
                def group(ids):
                    return dict(effort=list(summed(nodes[k]['days'] for k in ids)),start_endpoint_days=[min((r['records'][k]['start'] for k in ids),default=0) for r in runs],finish_endpoint_days=[max((r['records'][k]['finish'] for k in ids),default=0) for r in runs])
                slices=[dict(id=s['id'],name=s['name'],wave=s['wave'],**group([k for k,n in nodes.items() if n['slice']==s['id'] and n['days']!=(0,0)])) for s in model['release_slices']]
                phases={wave:group([k for k,n in nodes.items() if n['wave']==wave and n['days']!=(0,0)]) for wave in list(backlog['waves'])+['conditional_real','pre_business'] if any(n['wave']==wave and n['days']!=(0,0) for n in nodes.values())}
                ordered=[dict(id=k,owner_package=n['owner_package'],wave=n['wave'],slice=n['slice'],effort_category=n['effort_category'],estimate=list(n['days']),dependencies=sorted(n['dependencies']),start_endpoint_days=window(k,'start'),finish_endpoint_days=window(k),slot_low=runs[0]['records'][k]['slot'],slot_high=runs[1]['records'][k]['slot'],**({'capacity_gate_ref': n['capacity_gate_ref']} if 'capacity_gate_ref' in n else {})) for k,n in nodes.items() if n['days']!=(0,0)]
                ordered.sort(key=lambda x:(x['start_endpoint_days'][0],x['finish_endpoint_days'][0],x['id']))
                ms={k:window(k) for k in ['E01-UAA-C','E01-UAA-P','X02-F','CE01','CE02-S','CE02-D','D12','CE02-L']+(['E01-UAA-R'] if include_r else [])}
                profile['resources'][str(seats)]=dict(endpoint_makespans=[r['makespan_workdays'] for r in runs],critical_path_lower_bounds=[r['critical_path_lower_bound'] for r in runs],release_slices=slices,phases=phases,milestones=ms,ordered_tasks=ordered,resource_records={k:r['records'] for k,r in zip(['low','high'],runs)},formal_production_or_enterprise_activation=None,checks=dict(known_dependencies=True,dag=True,precedence=True,capacity_no_overlap=True,effort_sum=True,parent_child_deduplicated=True,single_seat_sum_equals_makespan=seats==1))
                profile['resources'][str(seats)]['delivery_exits']={name:dict(targets=model['delivery_priority_groups'][name]['targets'],**group(closure(model['delivery_priority_groups'][name]['targets']))) for name in ['foundation','risk_first_chain']}
            scenario['profiles']['with_maintenance_reserve' if reserve else 'without_maintenance_reserve']=profile
        result['scenarios'][key]=scenario
    result['bounded_effort_totals']={key:scenario['profiles']['without_maintenance_reserve']['total_effort_range'] for key,scenario in result['scenarios'].items()}
    result['source_input_sha256']['scripts/calculate-planning-schedule.py']=hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    result['scope_estimate_registry']='docs/planning/planning-scope-estimates.json'
    result['current_uaa_delivery'] = dict(
        source_ref='docs/planning/planning-backlog.json#/release_gates/company_sso_local_authorization',
        development_priority=sso['development_priority'],
        plan_ref=sso['plan_ref'],
        estimate_status=sso['estimate_status'],
        engineering_days=sso['engineering_days'],
        included_in_priced_schedule_baseline=sso['included_in_priced_schedule_baseline'],
    )
    result['no_unpriced_required_increment'] = (
        scope['all_required_supplements_priced'] and sso['included_in_priced_schedule_baseline']
    )
    result['no_unpriced_required_increment_scope'] = (
        'current_delivery_includes_unpriced_sso_successor; priced_76_parent_baseline_excludes_F004_new_detail_and_OBS'
    )
    result['dated_core_release_target_ref']=model['dated_core_release_target_ref']
    result['local_core_net_estimate_ref']=model['local_core_net_estimate_ref']
    result['local_core_view_note']=model['local_core_view_note']
    result['local_core']=build_local_core_projection(local_estimate,model['local_core_projection'],local_target,{item['id'] for item in backlog['work_items']},owner_actions)
    long_reference = result['scenarios']['public_R_pending']['profiles']['without_maintenance_reserve']['resources']['1']
    result['phase6_design_audit_projection'] = project_phase6_audit(local_estimate, result['local_core'], long_reference, uaa)
    result['uaa_main_capacity_gate']=validate_uaa_capacity_gate(model,result['local_core'])
    result['long_horizon_accounting']=dict(baseline_public_days=scope['baseline']['public_remaining'],
                                           baseline_with_R_days=scope['baseline']['with_conditional_uaa_R'],
                                           net_increment_days=scope['net_increment_total'])
    result['current_local_core_summary']=current_core_summary(local_estimate)
    result['current_view_checks']=validate_current_view_docs(repo,model,local_estimate)
    result['current_source_link_checks']=validate_current_source_links(repo,model)
    return result


def format_pair(value):
    return '\u2013'.join(str(x) for x in pair(value))


def dump_schedule(value, depth=0):
    """Valid JSON with compact pairs/leaf records; avoid 30k-line repeated receipts."""
    compact = isinstance(value, list) and all(not isinstance(x,(dict,list)) for x in value)
    compact = compact or isinstance(value,dict) and ('engineering_days' in value and 'slot' in value or 'owner_package' in value and 'start_endpoint_days' in value)
    if compact or not isinstance(value,(dict,list)) or not value:
        return json.dumps(value,ensure_ascii=False)
    pad='  '*depth;child='  '*(depth+1)
    if isinstance(value,dict):
        return '{\n'+',\n'.join(child+json.dumps(k,ensure_ascii=False)+': '+dump_schedule(v,depth+1) for k,v in value.items())+'\n'+pad+'}'
    return '[\n'+',\n'.join(child+dump_schedule(v,depth+1) for v in value)+'\n'+pad+']'


def day_window(value):
    starts,finishes=value['start_endpoint_days'],value['finish_endpoint_days']
    return f'累计 {starts[0]}→{finishes[0]} 日 / {starts[1]}→{finishes[1]} 日'


def render_local_core(result):
    core = result['local_core']
    lines = ['', '## 首版本地核心的机器投影（原对账底稿）', '', result['current_local_core_summary'], '',
             '同一生成器读取[首版净估册](local-core-release-estimate.json)和[目标](first-usable-release-target.json)，复用既有WBS、来源父包及必交旅程；净价不追加到76包总价。',
             'B0既有制品/mock技术预演见[首版阶梯](../phase6/first-usable-release-plan.md#4-按质量出口交付)，不反向等待全部新增能力。真实业务试用及正式启用仍须具体P1/能力/环境证据门。',
             '以下只演算所需输入就绪时的有效工程日；真实消息窗口、试用环境和稳定验收仍待证据，未知等待不是0，正式日期和激活批准均未给出。场景/出口的依赖闭包重叠，其工日不能相加。',
             '', '### 净额与必交旅程', '']
    for row in core['groups']:
        lines.append(f"- `{row['id']}`：{format_pair(row['days'])}工程日。")
    for row in core['required_journeys']:
        pointer = row['requirement_ref'].rsplit('#', 1)[1]
        lines.append(f"- [{pointer}](first-usable-release-target.json#{pointer}) {row['text']}；目标`{', '.join(row['targets'])}`。")
    lines += ['', '### 同一WBS的依赖编排', '', '父包仅作来源追踪，不追加父价或完整长期祖先；每行净额计一次。']
    one = core['resources']['1']['resource_records']['low']
    ordered = sorted(core['nodes'].values(), key=lambda row: (one[row['wbs_id']]['start'], one[row['wbs_id']]['finish'], row['wbs_id']))
    for row in ordered:
        lines.append(f"- `{row['wbs_id']}`（来源`{', '.join(row['parent_packages'])}`）：{format_pair(row['days'])}日；直接依赖`{', '.join(row['dependencies']) or '无'}`。")
    lines += ['', '### 条件与出口', '']
    for name, value in core['resources'].items():
        lines.append(f"- {name}个有效研发席位条件工程量端点：{format_pair(value['engineering_makespan_if_conditions_ready'])}有效工作日；不含未知外部等待，不是日历承诺。")
    for key, condition in core['external_conditions'].items():
        pointer = condition['source_ref'].rsplit('#', 1)[1]
        lines.append(f"- [{key}](owner-decision-agenda.json#{pointer})：{condition['state']}；证据状态{condition['evidence_state']}，不自动标为已就绪。")
    for key, value in core['delivery_exits'].items():
        evidence = value['evidence_ref'].replace('docs/phase6/', '../phase6/')
        lines.append(f"- **{key}**：目标`{', '.join(value['targets'])}`；条件`{', '.join(value['condition_keys']) or '无外部输入门；仍需产品验收'}`；[证据要求]({evidence})。计算结论仅为`not_verified_by_schedule`。")
    lines += ['', '机器检查净价/子分配、逐行单计、父包映射、缺依赖/环、必交原文hash与覆盖、外部动作/出口绑定及现势摘要；不证明映射语义、产品可用、安全缺陷修复或生产验收。',
              '受控摘要在[控制台](../review/11-project-control.md)和[路线图](../roadmap.md)共同核验，历史区保留原时点；净价改变须同步两区间及目标JSON。']
    return lines


def render_schedule(result):
    structure=result['source_structure']
    accounting=result['long_horizon_accounting']
    accounting_line=(f"补充前公共{format_pair(accounting['baseline_public_days'])}/含R{format_pair(accounting['baseline_with_R_days'])}，"
                     f"净增{format_pair(accounting['net_increment_days'])}，"
                     f"当前公共{format_pair(result['bounded_effort_totals']['public_R_pending'])}/含R{format_pair(result['bounded_effort_totals']['with_conditional_R_ready'])}。"
                     '基线、净增及当前总量分别由净增册与实际WBS求和派生，不与首版本地净价叠加。')
    lines=[
        '# 后续完整排程 — 既有能力、互通与遗漏范围的有界重排',
        '', f"日期：{result['date']}。本文件由仓内计算器生成；设计/计价候选，T0未启动，不是开发或部署指令。",
        '', '当前实施顺序、验收及日期请先读[按交付结果排列的排期](intranet-uaa-release-schedule.md)。本页用于复算历史工作量和工作包依赖。',
        '', '## 范围、来源与复现', '',
        f"原71父包完整承接，正式纳入CE五项后为{structure['parents']}父包、{structure['official_child_milestones']}正式子出口、{structure['additional_scope_tasks']}项LS额外范围任务、{structure['release_slices']}发布切片。",
        'LS不新增父业务包，普通子出口与CE02内部迁移配额只分解父价；UAA C/P/R仍为独立增量。',
        '本页长程总量只覆盖76包既有计价基线，不是基础版B0/B1工期，也未合并F004新增未净估细项及OBS候选补充；后继范围见[补充册](post-baseline-scope-estimates.json)。',
        'B4 以本地账号密码首发，核心稳定后优先仅 SSO、本地授权。仅 SSO 工日待独立净估，未计入下列总量；完整 UAA P/R 工日与日窗仅供按条件立项时对照。见[首版阶梯](../phase6/first-usable-release-plan.md)与[仅 SSO 计划](uaa-sso-local-authorization-plan.md)。',
        '六项RP替换原基价，不能替换后又把净差额再加一次。',
        '数字源为[净增册](planning-scope-estimates.json)、[UAA估算](uaa-implementation-estimate.json)和[总账](planning-backlog.json)。',
        '切片、UAA接线及分阶段迁移配额见[排程模型](execution-schedule-model.json)；逐任务/资源槽/输入哈希见[JSON](execution-schedule.json)。',
        '复现：`python3 scripts/calculate-planning-schedule.py --write`；检查：`python3 scripts/calculate-planning-schedule.py --check`。',
        '测试：`python3 scripts/test-planning-schedule.py`。无需本机隐藏目录、旧ignored生成器或Git冻结版本才能计算。',
        '', '## 工程量与相较上一版的变化', '',
        accounting_line,
        'G07A/G07B历史开发8–14日不再排；已完成的版本维护/合main不再派发或重复计价。原with/without reserve保留1–2日历史/条件容量对照，不是今日剩余交付承诺；本地Q2已扣除该维护信用。', '',
    ]
    audit = result['phase6_design_audit_projection']
    labels = ['人工风险整改', '自动任务、清单和公共消息组合', '本地核心正式和稳定', '公共 UAA 治理', '真实 UAA 纳管']
    lines += ['', '## 既有审计画像的累计工程量', '']
    for label, stage in zip(labels, audit['stages']):
        if stage.get('conditional_full_governance'):
            label += '（完整纳管条件对照）'
        lines.append(f"- {label}，{format_pair(stage['engineering_candidate_days'])} 日，保守 {stage['conservative_days']:g} 日。")
    lines += ['', '日期、目标环境和外部等待另列。累计值有重叠，按[交付排期](intranet-uaa-release-schedule.md#2-最早与保守排期)阅读。']
    lines += render_local_core(result)
    lines += ['', '## 长期76父包工程量对照', '']
    for key,label in [('public_R_pending','公共范围，R待外部条件'),('with_conditional_R_ready','选定R且各阶段外部等待为0的条件对照')]:
        sc=result['scenarios'][key]['profiles']
        lines.append(f"- {label}：不含条件储备{format_pair(sc['without_maintenance_reserve']['total_effort_range'])}日；保留历史/条件维护储备对照{format_pair(sc['with_maintenance_reserve']['total_effort_range'])}日。")
        for seats,r in sc['with_maintenance_reserve']['resources'].items():
            days=r['endpoint_makespans'];weeks=[round(x/5,1) for x in days]
            lines.append(f"  - {seats}席参考：累计 {days[0]} / {days[1]} 个有效工作日结束，约 {weeks[0]} / {weeks[1]} 个有效研发周。")
    lines += ['', 'UAA P的后台、四页最小UI、运维、CI与质量仍只算25–39；R真实UI/运维及正常切换观察仍只算13–22。',
        'U02/U06/UI新功能、CE与LS各按净增册扣重，不把完整前端或新增调用层误算作旧价免费附送。',
        '', '## 资源与时间含义', '',
        '主参考1个有效研发席位，每周5工程日；2席仅独立交付能力具备时对比，不等于两个agent。',
        '实际开工日记为 T0，所有相对日窗从开工起按累计有效工作日表示。本页用于长期相对估算，首版以[交付排期](intranet-uaa-release-schedule.md)为准。',
        '每项低/高输入分别跑一次排程；下文“低 / 高”不是置信区间或任意混合耗时的保证上界。',
        '条件对照保留原维护先行排序；已完成维护不重新派发。优先基础U01/U02/C02/X01-B/X02-Q及真实祖先，保留S01/S02安全工作与E10有限调查，再交D10/D15风险首链。公开UAA P正任务优先级7.5，选定且外部就绪的企业对照在基础后优先P/R；这些是原长期容量对照，不代表首版本地顺序或激活获批。',
        '阶段不是全串行大门；日窗可交错。单席总工期等于所选工日和；双席逐槽验证不重叠。',
        '', '## 全部13发布切片', '',
        '以下含CE/LS及RP替换后的净范围，剔除已做G07；UAA独立增量与条件维护容量单列，不塞进S09的E01旧基价。', '']
    public=result['scenarios']['public_R_pending']['profiles']['with_maintenance_reserve']['resources']
    alt={x['id']:x for x in public['2']['release_slices']}
    for row in public['1']['release_slices']:
        lines += [f"- **{row['id']} {row['wave']} {row['name']}**：{format_pair(row['effort'])}工程日；1席{day_window(row)}；2席{day_window(alt[row['id']])}。"]
    lines += ['', '## 基础目标与风险首链的独立出口', '',
        '以下只按具名目标及真实祖先计算联合范围；两组共享依赖，工日不能相加，日窗也不代表批准生产或企业模式激活。基础目标不是等待全部远期76包完成。']
    for name,label in [('foundation','基础接缝/首页与附件正确性'),('risk_first_chain','风险首链')]:
        row=public['1']['delivery_exits'][name];other=public['2']['delivery_exits'][name]
        lines.append(f"- **{label}**（{', '.join(row['targets'])}）：含真实祖先{format_pair(row['effort'])}工程日；1席{day_window(row)}；2席{day_window(other)}。")
    lines += ['', '## 按Phase汇总的工程窗', '',
        '本节按任务实际所属Phase汇总，含跨阶段UAA C/P与CE02-L配额；不把CE02父包全部20–30日塞进P6.1。', '']
    for phase,row in public['1']['phases'].items():
        other=public['2']['phases'][phase]
        lines.append(f"- {phase}：{format_pair(row['effort'])}工程日；1席{day_window(row)}；2席{day_window(other)}。")
    lines += ['', '## 关键出口与分阶段迁移', '']
    for k,value in public['1']['milestones'].items():
        other=public['2']['milestones'][k]
        lines.append(f'- {k}：1席累计 {value[0]} / {value[1]} 日；2席累计 {other[0]} / {other[1]} 日。')
    lines += [
        '', 'CE01因C04实际在P6.1；CE02-S6–9日/P6.1，CE02-D5–8日/P6.3，CE02-L9–13日最终在P9收口。',
        'L内部配额为P6.1 2–3、P6.3 3–4、P7 2–3、P9 2–3日，随IAM/组织、工单/通知/情报/AL、Action/MCP/适配、持久闭环分批迁移。',
        '新增或触及模块当批接入，P9只是存量最终库存/旁路验收；当前候选能力runtime_verified均不能因已排程变为true。',
        'D12只依CE02-D可靠入口，不反依I01；I01/I02/I03消费D12。CE03/04/05在P7，LS01–04在P6.3，LS05/06在P7。',
        'X01-C2–3/X01-B3–4/X01-H3–5仍合计原8–12日；B为配置后重启的受信接缝，H在S09/P7交完整管理增强。X02-Q独立依X01-C，不等D06/feed；X03只依U02、X01-B和X02-Q，资源授权不因此削弱。',
        '', '<a id="uaa在现有13切片中的跨阶段硬出口"></a>', '',
        '## 后续 UAA 主线与完整纳管条件对照', '',
        f"核心稳定后优先仅 SSO；实际主研发容量须先满足既有`{result['uaa_main_capacity_gate']['gate_ref']}`，"
        f"对应`{result['uaa_main_capacity_gate']['owner_action_key']}`业务/工程证据，当前仍为`{result['uaa_main_capacity_gate']['readiness']}`。"
        '此门也适用于另行立项的完整纳管。证据门本身无新增工日，协议联系与合成准备可以先行。仅 SSO 净估独立于下方完整治理日窗。',
        '公共范围保留mock/contract，不因公司资源未到而阻断独立IAM和公共开发；真实成功不能由合成用例代验。',
        '仅 SSO 的真实测试按认证计划验收。完整纳管联调须 P、测试应用、协议、网络和专属测试权限；R 内产出影子差异与私有制品。',
        '正式apply前另需映射/影子差异批准、正式资源/窗口、应急退出与适用公共生产门；apply后观察才是R整体收口。',
        '不把R整体收口倒置为自己apply前提，也不编造pre_apply具体天数或正式激活日。',
        '真实专业数据源/第二类adapter、标准OAuth AS/主体桥、SOAR/知识/Agent等依赖各自提供方合同、身份与权限。',
        '外部排队、修复、审批和窗口等待不在工程日内；LS05/06已计有限公共实现，真实条件未具备时保持未激活。',
        '未选Office/供应商门户/草稿等整套产品、获准后三类导入/merge建设仍按净增册条件另估，不由E05/E10小包包干。',
        '各形态正式启用均验证 G05/E09 适用硬项。仅 SSO 另验真实认证、会话和回退；完整纳管追加 R.pre_apply。',
        '详细条件和阶段见[真实接入条件](uaa-real-integration-conditions.json)；各提供方激活窗口由对应Owner确认。',
        '', '## 全部正工时任务派发顺序', '',
        '下列保留公共含历史/条件维护储备的单席对照，不是今日剩余派单；当前首版净额和顺序见上方本地核心投影。双席及含R记录保留JSON，聚合节点零工日不另计价。', '']
    for n,row in enumerate(public['1']['ordered_tasks'],1):
        gate = f"；实际主产能准入`{row['capacity_gate_ref']}`" if 'capacity_gate_ref' in row else ''
        lines.append(f"- {n}. `{row['id']}`（{row['wave']}，{row['owner_package']}）：{format_pair(row['estimate'])}日，{day_window(row)}；依赖`{', '.join(row['dependencies']) or '无'}`{gate}。")
    lines += ['', '## 验证与后续详设', '',
        '仓内测试覆盖价格替换/去重、原范围保留、缺依赖、环、资源不重叠、端点、确定性及R待条件公共路线。',
        '本轮先用旧范围应用行为跑红，再补实现到绿；红绿原记录留本机ignored，正式代码和测试在仓内。',
        '每包开工仍冻结准确DTO/schema、迁移和正负边界/授权用例，测试先行；远期设计允许分批深化。',
        '排程计算/文档绿不代替原完整acceptance或UI六件门禁，真实运行与部署没有因本页生成而完成。', '']
    return '\n'.join(lines)


def render_backlog(backlog):
    parents=len(backlog['work_items'])
    children=sum(len(x.get('milestones',[])) for x in backlog['work_items'])
    lines=['# 全项目排期总账 — 既有能力与新增范围的唯一任务入口','',
        f"日期：{backlog['date']}。{parents}父包、{children}正式子出口、6项LS额外子范围，13发布切片；原71包完整保留，CE五项正式纳入。",
        '唯一净增数字源为[估算注册表](planning-scope-estimates.json)，相对日窗见[完整排程](execution-schedule.md)，算法/分解见[模型](execution-schedule-model.json)。',
        'B4 先以本地账号密码正式运行。核心稳定后优先[仅 SSO、本地授权](uaa-sso-local-authorization-plan.md)，工日待独立净估。完整 P/R 保留有条件立项的设计和计价对照。首版按[业务与放行条件](../phase6/first-usable-release-plan.md)排期。',
        '本总账长程工日不是基础版B0/B1工期；基础目标与风险首链在完整排程单列。F004新增细项/OBS后继补充尚未合并，不把旧总额冒称完整新路线总价。',
        '', '## 读取规则与质量继承','',
        '尽可能承接现有功能、渐进优化扩展，专业平台能力优先取数与受权调用，保留大量合成mock。',
        '当前批继续standards §11测试先行、正负/边界/权限先红后绿，独立审查和完整门禁；不以规划存在当作准确设计已冻结。',
        '普通子出口分解父价；E01旧其他provider基价3–6不含UAA C/P/R2–3/25–39/13–22。',
        'RP六项替换旧价，不再加差额；LS六项追加但不造父包；CE02-L内部配额不新增正式出口。',
        'G07历史开发不再计未来；1–2日维护仅保留历史/条件容量对照，已完成维护不再派发或重复计价，首版本地Q2已扣除该信用。来源/真实等待/接线/运行验收分列。',
        '', '## 波次与工作包','']
    for wave,info in backlog['waves'].items():
        lines += [f"### {wave} {info['name']}",'',info['exit'],'']
        for row in backlog['work_items']:
            if row['wave'] != wave:
                continue
            dep=row['dependencies']+row.get('milestone_dependencies',[])
            lines += [f"- **{row['id']} {row['title']}**：{format_pair(row['estimate_engineering_days'])}日；依赖`{', '.join(dep) or '无'}`。{row['acceptance']}"]
    lines += ['', '### 跨阶段父包与全部具名子出口','']
    for row in backlog['work_items']:
        if row['wave'] not in backlog['waves']:
            lines += [f"- **{row['id']} {row['title']}**（{row['wave']}）：{format_pair(row['estimate_engineering_days'])}日，按下列子出口分解。"]
        for child in row.get('milestones',[]):
            lines.append(f"- `{child['id']}`（{child.get('wave',row['wave'])}）：{format_pair(child['estimate_engineering_days'])}日；显式依赖`{', '.join(child['dependencies']) or '无'}`；{child['acceptance']}")
    lines += ['', '## 已计价的LS额外子范围','']
    for row in backlog['additional_scope_tasks']:
        lines.append(f"- `{row['id']}` {row['title']}，归{row['owner_package']}，{row['wave']}，{format_pair(row['net_days'])}净增日；依赖`{', '.join(row['dependencies'])}`。")
    active_work_links = backlog.get('active_work_links')
    if active_work_links:
        lines += ['', '## 当前活跃包的合同、交接与准入', '', active_work_links['scope']]
        for group in active_work_links['groups']:
            lines += ['', f"### {group['title']}", '']
            for entry in group['entries']:
                lines.append(f"- **{entry['label']}** — 有效合同：{entry['contract']}；实施交接：{entry['handoff']}；验收／发布准入：{entry['admission']}")
    lines += ['', '## 依赖、发布与确认窗口','',
        '完整包依赖与具名子出口依赖分开；子出口默认继承父外部依赖，E01-UAA、CE02及X02-Q出口显式按自身依赖准入。',
        'X01-C/B交基础合同与重启生效接缝，X01-H在P7交完整管理增强，仍分解原8–12日；X02-Q只依X01-C，X02-F继续验真实feed安全前提；X03不等H/feed，仍验私有资源权限。',
        'D12链A依CE02-D，不依I01；CE02-D不依D12/I01，避免调用统一形成反向环。',
        'CE02-L存量迁移按P6.1/P6.3/P7/P9逐模块推进，最终P9库存收口；本轮候选映射不等于已迁移。',
        '普通公开/独立部署不等公司UAA或任何真实provider；首次生产仍过G05/E09适用硬证据。',
        '公司UAA纳管正式apply前需R.pre_apply；切换后观察是R整体收口，不倒置为apply前提。',
        '测试进入与正式窗口/映射批准分开；真实资料由所有者/提供方在对应联调窗口确认，不要求披露私有地址/身份入公共仓。',
        'E10在P6.1出口对三项导入/merge分别裁决；E05在P7裁决更远供给与价值。未选整套产品不是已计无限范围。',
        '每个包开工前按[设计成熟度](planning-design-status.json)冻结准确契约/数据和负例；所有者确认仅绑定具体前文政策，不默认为全面开工。',
        '', '## 按运行模式检查的发布证据门', '',
        '所有模式首次生产前均验证G05/E09适用硬项：已选拓扑的TLS/网络/会话/秘密、固定制品/配置/迁移回执、备份恢复和故障接管；允许单副本，不要求未选HA或全部P7/I03先完。',
        '公司仅 SSO 另验真实认证、撤权、试点和回退。完整纳管先过 E01-UAA-R 的 pre_apply，切换后再做观察收口。',
        '这些是按能力启用的证据门而非新包或重复预算；详细checklist以本总账JSON的release_gates及UAA条件册为准。',
        '', '## 本轮校准与去重','',
        '当前长期基线、净增与总量由[净增册](planning-scope-estimates.json)和[派生排程](execution-schedule.md)给出，不在本页另写一组价格；已完成维护不再次收费，首版本地净价不与长期总量相加。',
        '旧包、UAA、CE、LS及UI复用扣重详见净增册；所有真实条件与未选范围仍单列，不因已估价宣称实现或正式激活。',
        '历史快照（2026-11-13，3f79a76）：当时76父包/16子出口/LS六项及17行范围计价，未实现未来业务；这是真实旧结构，不作现势计数。当前正式结构由本页顶部按源JSON派生。','']
    return '\n'.join(lines)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo',type=Path,default=Path(__file__).resolve().parents[1])
    mode=parser.add_mutually_exclusive_group()
    mode.add_argument('--write',action='store_true')
    mode.add_argument('--check',action='store_true')
    args=parser.parse_args();repo=args.repo.resolve();result=calculate(repo)
    backlog=json.loads((repo/'docs/planning/planning-backlog.json').read_text())
    outputs={'docs/planning/execution-schedule.json':dump_schedule(result)+'\n','docs/planning/execution-schedule.md':render_schedule(result),'docs/planning/planning-backlog.md':render_backlog(backlog)}
    if args.write:
        for path,content in outputs.items():
            (repo/path).write_text(content)
    elif args.check:
        changed=[p for p,s in outputs.items() if (repo/p).read_text()!=s]
        if changed:
            parser.exit(1,'stale derived schedule: '+', '.join(changed)+'\n')
    print(json.dumps(dict(status='PASS',source_structure=result['source_structure'],bounded_effort_totals=result['bounded_effort_totals']),ensure_ascii=False))


if __name__=='__main__':
    main()
