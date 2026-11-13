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


def calculate(repo):
    paths=['docs/review/planning-backlog.json','docs/review/execution-schedule-model.json','docs/review/planning-scope-estimates.json','docs/review/uaa-implementation-estimate.json','docs/review/uaa-real-integration-conditions.json']
    blobs={p:(repo/p).read_bytes() for p in paths}
    backlog,model,scope,uaa,conditions=[json.loads(blobs[p]) for p in paths]
    graph,milestones=source_graph(backlog)
    result=dict(date=model['date'],status='fully_priced_bounded_candidate_not_implemented',calendar=None,estimate_source=paths,source_input_sha256={p:hashlib.sha256(blobs[p]).hexdigest() for p in paths},resource_assumptions=model['resource_assumptions'],source_structure=dict(parents=len(backlog['work_items']),official_child_milestones=len(milestones),additional_scope_tasks=sum(x['kind']=='additional_existing_scope' for x in scope['items']),release_slices=len(model['release_slices']),merged_dag=True,source_nodes=len(graph)),scenarios={},release_slices=model['release_slices'],pre_business_maintenance=dict(work_items=model['completed_parent_ids'],status='locally_validated_pending_pr_main',do_not_repeat_future_coding_estimate=True),release_gates_ref='docs/review/planning-backlog.json#/release_gates',external_waiting=dict(ref='docs/review/uaa-real-integration-conditions.json',entry_and_activation_stages=conditions['entry_and_activation_stages'],additional_refs=model['external_waiting_refs'],rule='Waiting, approval, provider fixes and formal activation dates are outside engineering effort. Public mock/contract delivery is not blocked by unavailable real providers.'),endpoint_semantics=model['endpoint_semantics'],reference_algorithm=dict(kind='deterministic list scheduling; priority then remaining critical chain then ID',reproduction='python3 scripts/calculate-planning-schedule.py --check',runtime_acceptance_claimed=False),active_work_boundary='Design/schedule only; T0 not started; no business development or activation authorized')
    result['delivery_scope_boundaries']=dict(foundation_targets=model['delivery_priority_groups']['foundation']['targets'],risk_first_chain_targets=model['delivery_priority_groups']['risk_first_chain']['targets'],effort_rule='Each delivery exit reports the union of its named targets and actual dependencies. Shared ancestors overlap; do not add the exit efforts.',long_horizon_totals_are_not_foundation_duration=True,baseline_scope_only=True,post_baseline_supplements_not_merged=['F004 newly selected details pending net estimate','OBS01–03 separately priced candidate supplement'],supplement_ref='docs/review/post-baseline-scope-estimates.json',production_or_enterprise_activation_approved=False)
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
                ordered=[dict(id=k,owner_package=n['owner_package'],wave=n['wave'],slice=n['slice'],effort_category=n['effort_category'],estimate=list(n['days']),dependencies=sorted(n['dependencies']),start_endpoint_days=window(k,'start'),finish_endpoint_days=window(k),slot_low=runs[0]['records'][k]['slot'],slot_high=runs[1]['records'][k]['slot']) for k,n in nodes.items() if n['days']!=(0,0)]
                ordered.sort(key=lambda x:(x['start_endpoint_days'][0],x['finish_endpoint_days'][0],x['id']))
                ms={k:window(k) for k in ['E01-UAA-C','E01-UAA-P','X02-F','CE01','CE02-S','CE02-D','D12','CE02-L']+(['E01-UAA-R'] if include_r else [])}
                profile['resources'][str(seats)]=dict(endpoint_makespans=[r['makespan_workdays'] for r in runs],critical_path_lower_bounds=[r['critical_path_lower_bound'] for r in runs],release_slices=slices,phases=phases,milestones=ms,ordered_tasks=ordered,resource_records={k:r['records'] for k,r in zip(['low','high'],runs)},formal_production_or_enterprise_activation=None,checks=dict(known_dependencies=True,dag=True,precedence=True,capacity_no_overlap=True,effort_sum=True,parent_child_deduplicated=True,single_seat_sum_equals_makespan=seats==1))
                profile['resources'][str(seats)]['delivery_exits']={name:dict(targets=model['delivery_priority_groups'][name]['targets'],**group(closure(model['delivery_priority_groups'][name]['targets']))) for name in ['foundation','risk_first_chain']}
            scenario['profiles']['with_maintenance_reserve' if reserve else 'without_maintenance_reserve']=profile
        result['scenarios'][key]=scenario
    result['bounded_effort_totals']={key:scenario['profiles']['without_maintenance_reserve']['total_effort_range'] for key,scenario in result['scenarios'].items()}
    result['source_input_sha256']['scripts/calculate-planning-schedule.py']=hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    result['scope_estimate_registry']='docs/review/planning-scope-estimates.json'
    result['no_unpriced_required_increment']=scope['all_required_supplements_priced']
    result['no_unpriced_required_increment_scope']='priced_76_parent_baseline_only_excludes_F004_new_detail_and_OBS_not_complete_B0_B1_price'
    result['dated_core_release_target_ref']=model['dated_core_release_target_ref']
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
    return f'D{starts[0]}→D{finishes[0]} / D{starts[1]}→D{finishes[1]}'


def render_schedule(result):
    structure=result['source_structure']
    lines=[
        '# 后续完整排程 — 既有能力、互通与遗漏范围的有界重排',
        '', f"日期：{result['date']}。本文件由仓内计算器生成；设计/计价候选，T0未启动，不是开发或部署指令。",
        '', '## 范围、来源与复现', '',
        f"原71父包完整承接，正式纳入CE五项后为{structure['parents']}父包、{structure['official_child_milestones']}正式子出口、{structure['additional_scope_tasks']}项LS额外范围任务、{structure['release_slices']}发布切片。",
        'LS不新增父业务包，普通子出口与CE02内部迁移配额只分解父价；UAA C/P/R仍为独立增量。',
        '本页长程总量只覆盖76包既有计价基线，不是基础版B0/B1工期，也未合并F004新增未净估细项及OBS候选补充；后继范围见[补充册](post-baseline-scope-estimates.json)。',
        '首个核心可用版已定1月试用/2027年2月1日前，见[首版阶梯](../phase6/first-usable-release-plan.md)和[变化对照](core-release-change-comparison.md)；本页旧基线日窗不证明该日期可达，核心链联合净估/实际产能与真实条件仍待冻结。',
        '六项RP替换原基价，不能替换后又把净差额再加一次。',
        '数字源为[净增册](planning-scope-estimates.json)、[UAA估算](uaa-implementation-estimate.json)和[总账](planning-backlog.json)。',
        '切片、UAA接线及分阶段迁移配额见[排程模型](execution-schedule-model.json)；逐任务/资源槽/输入哈希见[JSON](execution-schedule.json)。',
        '复现：`python3 scripts/calculate-planning-schedule.py --write`；检查：`python3 scripts/calculate-planning-schedule.py --check`。',
        '测试：`python3 scripts/test-planning-schedule.py`。无需本机隐藏目录、旧ignored生成器或Git冻结版本才能计算。',
        '', '## 工程量与相较上一版的变化', '',
        '上一版公共292–496日/含R305–518日是补充前基线；新增LS34–56、CE52–80、RP净差16–24，合计102–160日。',
        'G07A/G07B历史开发8–14日不再排；维护PR/CI/main收尾仍单独预留1–2日。', '',
    ]
    for key,label in [('public_R_pending','公共范围，R待外部条件'),('with_conditional_R_ready','选定R且各阶段外部等待为0的条件对照')]:
        sc=result['scenarios'][key]['profiles']
        lines.append(f"- {label}：不含维护{format_pair(sc['without_maintenance_reserve']['total_effort_range'])}日；含维护{format_pair(sc['with_maintenance_reserve']['total_effort_range'])}日。")
        for seats,r in sc['with_maintenance_reserve']['resources'].items():
            days=r['endpoint_makespans'];weeks=[round(x/5,1) for x in days]
            lines.append(f"  - {seats}席参考：D{days[0]} / D{days[1]}结束，约{weeks[0]} / {weeks[1]}个有效研发周。")
    lines += ['', 'UAA P的后台、四页最小UI、运维、CI与质量仍只算25–39；R真实UI/运维及正常切换观察仍只算13–22。',
        'U02/U06/UI新功能、CE与LS各按净增册扣重，不把完整前端或新增调用层误算作旧价免费附送。',
        '', '## 资源与时间含义', '',
        '主参考1个有效研发席位，每周5工程日；2席仅独立交付能力具备时对比，不等于两个agent。',
        '实际开工日记T0，D0是开工时点、DN是累计N个有效工作日结束；本页长期相对窗不证明首版固定目标可行，首版另见[阶梯与试用](../phase6/first-usable-release-plan.md)。',
        '每项低/高输入分别跑一次排程；下文“低 / 高”不是置信区间或任意混合耗时的保证上界。',
        '维护先行，优先基础U01/U02/C02/X01-B/X02-Q及真实祖先；保留S01/S02安全工作与E10有限调查窗口，再交D10/D15风险首链。公开参考的UAA P正任务优先级7.5，在P6.E优先工作后/P7正任务前仍有明确窗口；选定且外部就绪的企业对照在基础后优先P/R，不代表实际激活获批。其余ready任务按切片和剩余关键链稳定排序。',
        '阶段不是全串行大门；日窗可交错。单席总工期等于所选工日和；双席逐槽验证不重叠。',
        '', '## 全部13发布切片', '',
        '以下含CE/LS及RP替换后的净范围，剔除已做G07；UAA独立增量与维护单列，不塞进S09的E01旧基价。', '']
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
        lines.append(f'- {k}：1席D{value[0]} / D{value[1]}；2席D{other[0]} / D{other[1]}。')
    lines += [
        '', 'CE01因C04实际在P6.1；CE02-S6–9日/P6.1，CE02-D5–8日/P6.3，CE02-L9–13日最终在P9收口。',
        'L内部配额为P6.1 2–3、P6.3 3–4、P7 2–3、P9 2–3日，随IAM/组织、工单/通知/情报/AL、Action/MCP/适配、持久闭环分批迁移。',
        '新增或触及模块当批接入，P9只是存量最终库存/旁路验收；当前候选能力runtime_verified均不能因已排程变为true。',
        'D12只依CE02-D可靠入口，不反依I01；I01/I02/I03消费D12。CE03/04/05在P7，LS01–04在P6.3，LS05/06在P7。',
        'X01-C2–3/X01-B3–4/X01-H3–5仍合计原8–12日；B为配置后重启的受信接缝，H在S09/P7交完整管理增强。X02-Q独立依X01-C，不等D06/feed；X03只依U02、X01-B和X02-Q，资源授权不因此削弱。',
        '', '## UAA在现有13切片中的跨阶段硬出口', '',
        '公共范围保留mock/contract，不因公司资源未到而阻断独立IAM和公共开发；真实成功不能由合成用例代验。',
        'UAA测试进入须P、测试应用/协议、网络和专属测试权限；R内部才产出影子差异、私有制品/runbook。',
        '正式apply前另需映射/影子差异批准、正式资源/窗口、应急退出与适用公共生产门；apply后观察才是R整体收口。',
        '不把R整体收口倒置为自己apply前提，也不编造pre_apply具体天数或正式激活日。',
        '真实专业数据源/第二类adapter、标准OAuth AS/主体桥、SOAR/知识/Agent等依赖各自提供方合同、身份与权限。',
        '外部排队、修复、审批和窗口等待不在工程日内；LS05/06已计有限公共实现，真实条件未具备时保持未激活。',
        '未选Office/供应商门户/草稿等整套产品、获准后三类导入/merge建设仍按净增册条件另估，不由E05/E10小包包干。',
        '独立首次生产仍验G05/E09适用硬项；公司UAA纳管模式加R.pre_apply。早期首发只前移硬证据，不机械等全部P7/I03。',
        '详细条件和阶段见[真实接入条件](uaa-real-integration-conditions.json)；各提供方激活窗口由对应Owner确认。',
        '', '## 全部正工时任务派发顺序', '',
        '下列为公共含维护的单席主参考；完整双席及含R记录保留JSON。聚合节点零工日，不另作一次开发计价。', '']
    for n,row in enumerate(public['1']['ordered_tasks'],1):
        lines.append(f"- {n}. `{row['id']}`（{row['wave']}，{row['owner_package']}）：{format_pair(row['estimate'])}日，{day_window(row)}；依赖`{', '.join(row['dependencies']) or '无'}`。")
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
        '1有效席位×每周5工程日，T0未启动；首版已确认1月试用/2027年2月1日前，目标及可行性见[首版阶梯](../phase6/first-usable-release-plan.md)。本页是长期参考，不是该日期已可兑现的排程或开发授权。',
        '本总账长程工日不是基础版B0/B1工期；基础目标与风险首链在完整排程单列。F004新增细项/OBS后继补充尚未合并，不把旧总额冒称完整新路线总价。',
        '', '## 读取规则与质量继承','',
        '尽可能承接现有功能、渐进优化扩展，专业平台能力优先取数与受权调用，保留大量合成mock。',
        '当前批继续standards §11测试先行、正负/边界/权限先红后绿，独立审查和完整门禁；不以规划存在当作准确设计已冻结。',
        '普通子出口分解父价；E01旧其他provider基价3–6不含UAA C/P/R2–3/25–39/13–22。',
        'RP六项替换旧价，不再加差额；LS六项追加但不造父包；CE02-L内部配额不新增正式出口。',
        'G07历史开发不再计未来；维护收尾单列1–2。来源/真实等待/功能接线/运行验收分列。',
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
        '公司UAA纳管模式另过E01-UAA-R的pre_apply检查；R切换后观察属于后置收口，不倒置为apply前提。完整独立IAM不等公司R、E01父包或企业凭据。',
        '这些是按能力启用的证据门而非新包或重复预算；详细checklist以本总账JSON的release_gates及UAA条件册为准。',
        '', '## 本轮校准与去重','',
        '相较补充前公共292–496/含R305–518，净增102–160，得到公共394–656/含R407–678；维护预留另加1–2。',
        '旧包、UAA、CE、LS及UI复用扣重详见净增册；所有真实条件与未选范围仍单列，不因已估价宣称实现或正式激活。',
        '2026-11-13：补17行范围计价、76父包/16子出口/LS六项与仓内可复现完整排程；未实现未来业务。','']
    return '\n'.join(lines)


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo',type=Path,default=Path(__file__).resolve().parents[1])
    mode=parser.add_mutually_exclusive_group()
    mode.add_argument('--write',action='store_true')
    mode.add_argument('--check',action='store_true')
    args=parser.parse_args();repo=args.repo.resolve();result=calculate(repo)
    backlog=json.loads((repo/'docs/review/planning-backlog.json').read_text())
    outputs={'docs/review/execution-schedule.json':dump_schedule(result)+'\n','docs/review/execution-schedule.md':render_schedule(result),'docs/review/planning-backlog.md':render_backlog(backlog)}
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
