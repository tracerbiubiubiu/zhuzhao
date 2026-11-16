#!/usr/bin/env python3
"""Behavioral tests for scope accounting, graph integrity and capacity scheduling."""
import copy
import importlib.util
import json
import hashlib
import tempfile
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location('planning_schedule', ROOT / 'scripts/calculate-planning-schedule.py')
m = importlib.util.module_from_spec(SPEC); SPEC.loader.exec_module(m)


def fixtures():
    backlog = {'work_items': [{'id':'A', 'estimate_engineering_days':'2–3', 'dependencies':[], 'state':'planned_not_implemented'}]}
    estimates = {'items': [
        {'id':'RP-A','kind':'reprice_existing_base','owner_package':'A','baseline_days':[2,3],'replacement_days':[4,6],'net_days':[2,3],'wbs':[{'days':[2,3]}]},
        {'id':'CE01','kind':'new_capability_scope','owner_package':'CE01','title':'new','wave':'P6.1','dependencies':['A'],'net_days':[3,5],'wbs':[{'days':[3,5]}]},
        {'id':'LS01','kind':'additional_existing_scope','owner_package':'A','title':'old omitted','wave':'P6.3','dependencies':['A'],'net_days':[1,2],'wbs':[{'days':[1,2]}]}
    ],'net_increment_total':[6,10]}
    return backlog, estimates


class ScopeTests(unittest.TestCase):
    def test_replacement_replaces_base_instead_of_adding_twice(self):
        b,e=fixtures(); updated,extra=m.apply_scope(b,e)
        self.assertEqual(m.pair(updated['work_items'][0]['estimate_engineering_days']),(4,6))
        self.assertEqual(m.summed([m.pair(x['estimate_engineering_days']) for x in updated['work_items']]+[x['net_days'] for x in extra]),(8,13))
        self.assertEqual(b['work_items'][0]['estimate_engineering_days'],'2–3')

    def test_scope_application_is_idempotent(self):
        b,e=fixtures(); one,extra=m.apply_scope(b,e); two,again=m.apply_scope(one,e)
        self.assertEqual(one,two);self.assertEqual(extra,again)
        self.assertEqual({x['id'] for x in two['work_items']},{'A','CE01'})
        self.assertEqual([x['id'] for x in extra],['LS01'])

    def test_price_drift_rejected(self):
        b,e=fixtures();b['work_items'][0]['estimate_engineering_days']='8–9'
        with self.assertRaisesRegex(ValueError,'baseline|price'):m.apply_scope(b,e)

    def test_duplicate_and_missing_owner_rejected(self):
        b,e=fixtures();e['items'].append(copy.deepcopy(e['items'][0]))
        with self.assertRaisesRegex(ValueError,'duplicate'):m.apply_scope(b,e)
        b,e=fixtures();e['items'][0]['owner_package']='absent'
        with self.assertRaisesRegex(ValueError,'owner'):m.apply_scope(b,e)

    def test_wbs_and_declared_increment_are_checked(self):
        b,e=fixtures();e['items'][1]['wbs'][0]['days']=[1,1]
        with self.assertRaisesRegex(ValueError,'WBS'):m.apply_scope(b,e)
        b,e=fixtures();e['net_increment_total']=[99,100]
        with self.assertRaisesRegex(ValueError,'increment'):m.apply_scope(b,e)

    def test_existing_scope_never_disappears(self):
        b=json.loads((ROOT/'docs/planning/planning-backlog.json').read_text());e=json.loads((ROOT/'docs/planning/planning-scope-estimates.json').read_text());u,extra=m.apply_scope(b,e)
        original={x['id'] for x in b['work_items']};self.assertTrue(original <= {x['id'] for x in u['work_items']})
        self.assertEqual(len(u['work_items']),76);self.assertEqual(len(extra),6)


class GraphAndCapacityTests(unittest.TestCase):
    def test_missing_dependency_is_not_silently_ignored(self):
        with self.assertRaisesRegex(ValueError,'unknown'):m.validate_dag({'a':{'missing'}})

    def test_cycle_is_rejected(self):
        with self.assertRaisesRegex(ValueError,'cycle'):m.validate_dag({'a':{'b'},'b':{'a'}})

    def test_endpoints_zero_milestones_and_parallel_capacity(self):
        nodes={'a':{'days':[2,4],'dependencies':[]},'b':{'days':[3,5],'dependencies':[]},'join':{'days':[0,0],'dependencies':['a','b']},'c':{'days':[1,2],'dependencies':['join']}}
        for endpoint,total,parallel in [(0,6,4),(1,11,7)]:
            one=m.run_scheduler(nodes,1,endpoint);two=m.run_scheduler(nodes,2,endpoint)
            self.assertEqual(one['makespan_workdays'],total);self.assertEqual(two['makespan_workdays'],parallel)
            self.assertEqual(two['records']['join']['start'],max(two['records'][x]['finish'] for x in ['a','b']))

    def test_determinism_independent_of_input_map_order(self):
        nodes={k:{'days':[2,3],'dependencies':[]} for k in ['c','a','b']}
        self.assertEqual(m.run_scheduler(nodes,2,0),m.run_scheduler(dict(reversed(list(nodes.items()))),2,0))

    def test_early_investigation_can_overlap_mock_implementation_without_phase_barrier(self):
        nodes={
            'definition':{'days':[2,2],'dependencies':[]},
            'investigation':{'days':[2,4],'dependencies':['definition'],'priority':1},
            'mock_risk':{'days':[3,5],'dependencies':['definition'],'priority':2},
            'risk_closure':{'days':[1,2],'dependencies':['mock_risk'],'priority':2},
        }
        for endpoint in [0,1]:
            records=m.run_scheduler(nodes,2,endpoint)['records']
            self.assertLess(records['mock_risk']['start'],records['investigation']['finish'])
            self.assertLessEqual(records['investigation']['finish'],records['mock_risk']['finish'])
            self.assertLessEqual(records['investigation']['finish'],records['risk_closure']['start'])
        self.assertNotIn('investigation',nodes['mock_risk']['dependencies'])
        self.assertNotIn('mock_risk',nodes['investigation']['dependencies'])

    def test_invalid_estimates_and_capacity_rejected(self):
        for value in [[-1,2],[3,2],[True,2],'unpriced',None,{},1]:
            with self.assertRaises(ValueError):m.pair(value)
        with self.assertRaises(ValueError):m.run_scheduler({},0,0)


def local_core_fixture():
    estimate = {
        'groups': [
            {'id': 'S', 'days': [2, 3], 'wbs': [
                {'id': 'S1', 'days': [2, 3], 'suballocations': [
                    {'id': 'UAA-C', 'days': [1, 2]}, {'id': 'IAM-A', 'days': [1, 1]}]}]},
            {'id': 'RELEASE', 'days': [2, 3], 'wbs': [
                {'id': 'Q1', 'days': [1, 2]}, {'id': 'Q2', 'days': [1, 1]}]},
        ],
        'totals': {'raw_days': [4, 6], 'rounded_display_days': [4, 6]},
        'capacity': {'effective_seats': 1, 'engineering_days_per_week': 5, 'actual_T0': None},
    }
    target = {'must_have': ['independent local IAM', 'verified local release'],
              'estimate_views': {'full_release_net_days': [4, 6]}}
    projection = {
        'node_bindings': [
            {'id': 'S1', 'parent_packages': ['C04'], 'dependencies': []},
            {'id': 'Q1', 'parent_packages': ['G04'], 'dependencies': ['S1']},
            {'id': 'Q2', 'parent_packages': ['G05'], 'dependencies': ['Q1'],
             'external_condition_keys': ['MH01_MR1']},
        ],
        'required_journeys': [
            {'requirement_ref': '/must_have/' + str(i),
             'requirement_sha256': hashlib.sha256(text.encode()).hexdigest(),
             'wbs_ids': ['S1'] if i == 0 else ['Q1', 'Q2']}
            for i, text in enumerate(target['must_have'])
        ],
        'external_conditions': {'MH01_MR1': {
            'action_ref': '/execution_actions/MH01_MR1',
            'required_exits': ['formal_local']}},
        'delivery_exits': {
            'public_contract': {'targets': ['Q1'], 'condition_keys': [],
                                'evidence_ref': 'docs/phase6/first-usable-release-plan.md#4'},
            'formal_local': {'targets': ['Q2'], 'condition_keys': ['MH01_MR1'],
                             'evidence_ref': 'docs/phase6/first-usable-release-plan.md#4'},
        },
    }
    actions = {'execution_actions': {'MH01_MR1': {
        'state': 'pending_real_inputs_then_binding', 'evidence_state': 'not_provided',
        'evidence_refs': [], 'blocks': 'real message verification and formal local release',
        'blocks_release_exits': ['formal_local']}}}
    return estimate, projection, target, {'C04', 'G04', 'G05'}, actions


class LocalCoreProjectionTests(unittest.TestCase):
    def build(self, *args):
        return m.build_local_core_projection(*(args or local_core_fixture()))

    def test_actual_wbs_is_charged_once_without_parent_ancestor_prices(self):
        result = self.build()
        self.assertEqual(result.get('raw_engineering_days'), [4, 6])
        self.assertEqual(result.get('rounded_display_days'), [4, 6])
        self.assertEqual(set(result.get('nodes', {})), {'S1', 'Q1', 'Q2'})
        self.assertFalse(result.get('added_to_long_horizon_total', True))
        self.assertEqual(result.get('resources', {}).get('1', {}).get('engineering_makespan_if_conditions_ready'), [4, 6])

    def test_group_row_total_and_suballocation_drift_are_rejected(self):
        for mutation in ('group', 'total', 'display', 'suballocation'):
            args = list(local_core_fixture()); estimate = args[0]
            if mutation == 'group': estimate['groups'][0]['days'] = [1, 1]
            elif mutation == 'total': estimate['totals']['raw_days'] = [99, 99]
            elif mutation == 'display': estimate['totals']['rounded_display_days'] = [5, 7]
            else: estimate['groups'][0]['wbs'][0]['suballocations'][0]['days'] = [8, 9]
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, 'sum|total|round|allocation'):
                self.build(*args)

    def test_every_priced_row_has_one_known_parent_mapped_node(self):
        for mutation in ('missing', 'duplicate', 'unknown', 'parent'):
            args = list(local_core_fixture()); bindings = args[1]['node_bindings']
            if mutation == 'missing': bindings.pop()
            elif mutation == 'duplicate': bindings.append(copy.deepcopy(bindings[0]))
            elif mutation == 'unknown': bindings.append({'id': 'new_free_scope', 'parent_packages': ['C04'], 'dependencies': []})
            else: bindings[0]['parent_packages'] = ['unknown_parent']
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, 'node|binding|parent'):
                self.build(*args)

    def test_local_dependency_missing_and_cycle_are_rejected(self):
        for dependency in ('missing', 'Q2'):
            args = list(local_core_fixture()); args[1]['node_bindings'][0]['dependencies'] = [dependency]
            with self.subTest(dependency=dependency), self.assertRaisesRegex(ValueError, 'dependency|cycle'):
                self.build(*args)

    def test_required_journeys_cannot_be_dropped_or_rebound_to_changed_text(self):
        for mutation in ('missing', 'duplicate', 'new_requirement', 'changed_text', 'unknown_wbs'):
            args = list(local_core_fixture()); journeys = args[1]['required_journeys']
            if mutation == 'missing': journeys.pop()
            elif mutation == 'duplicate': journeys.append(copy.deepcopy(journeys[0]))
            elif mutation == 'new_requirement': args[2]['must_have'].append('new mandatory scope')
            elif mutation == 'changed_text': args[2]['must_have'][0] = 'changed business meaning'
            else: journeys[0]['wbs_ids'] = ['not_priced']
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, 'journey|requirement|coverage'):
                self.build(*args)

    def test_missing_external_action_or_formal_gate_binding_is_rejected(self):
        for mutation in ('action', 'gate'):
            args = list(local_core_fixture())
            if mutation == 'action': args[4]['execution_actions'].clear()
            else: args[1]['delivery_exits']['formal_local']['condition_keys'] = []
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, 'condition|action|gate'):
                self.build(*args)

    def test_unknown_waiting_never_becomes_zero_or_formal_readiness(self):
        result = self.build()
        self.assertIn('external_waiting_days', result)
        self.assertIsNone(result['external_waiting_days'])
        self.assertIsNone(result.get('calendar'))
        self.assertFalse(result.get('formal_activation_approved', True))
        self.assertEqual(result.get('externally_blocked_wbs'), ['Q2'])
        self.assertEqual(result.get('unblocked_public_wbs'), ['Q1', 'S1'])
        formal = result.get('delivery_exits', {}).get('formal_local', {})
        self.assertEqual(formal.get('condition_keys'), ['MH01_MR1'])
        self.assertEqual(formal.get('readiness'), 'not_verified_by_schedule')

    def test_current_target_price_cannot_drift_from_net_estimate(self):
        args = list(local_core_fixture()); args[2]['estimate_views']['full_release_net_days'] = [3, 5]
        with self.assertRaisesRegex(ValueError, 'current.*view|price.*drift'):
            self.build(*args)

    def test_formal_local_exit_cannot_drop_selected_wbs_or_required_journeys(self):
        args = list(local_core_fixture())
        args[1]['delivery_exits']['formal_local']['targets'] = ['S1']
        with self.assertRaisesRegex(ValueError, 'formal.*coverage|formal.*WBS'):
            self.build(*args)

    def test_model_cannot_self_remove_or_redirect_owner_external_release_gate(self):
        for mutation in ('double_remove', 'redirect', 'omit_condition'):
            args = list(local_core_fixture()); projection = args[1]
            projection['delivery_exits']['formal_local']['condition_keys'] = []
            if mutation == 'double_remove':
                projection['external_conditions']['MH01_MR1']['required_exits'] = []
            elif mutation == 'redirect':
                projection['external_conditions']['MH01_MR1']['required_exits'] = ['public_contract']
                projection['delivery_exits']['public_contract']['condition_keys'] = ['MH01_MR1']
            else:
                projection['external_conditions'].clear()
                projection['node_bindings'][-1]['external_condition_keys'] = []
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, 'condition|action|gate'):
                self.build(*args)

    def test_repository_generator_exposes_separate_local_core_and_hashes_its_sources(self):
        result = m.calculate(ROOT)
        self.assertIn('local_core', result)
        for path in ('docs/planning/local-core-release-estimate.json',
                     'docs/planning/first-usable-release-target.json',
                     'docs/planning/owner-decision-agenda.json'):
            self.assertIn(path, result['source_input_sha256'])
        core = result['local_core']
        estimate = json.loads((ROOT / 'docs/planning/local-core-release-estimate.json').read_text())
        self.assertEqual(core['raw_engineering_days'], estimate['totals']['raw_days'])
        self.assertEqual(core['rounded_display_days'], estimate['totals']['rounded_display_days'])
        self.assertEqual(len(core['required_journeys']), 9)
        self.assertEqual(set(result['current_view_checks']),
                         {'docs/review/11-project-control.md', 'docs/roadmap.md'})
        self.assertIn('首版本地核心的机器投影', m.render_schedule(result))
        self.assertIn(result['current_local_core_summary'], m.render_schedule(result))
        self.assertEqual(result['bounded_effort_totals']['public_R_pending'], [394, 656])

    def test_current_summary_uses_current_raw_rounded_and_shared_transfer(self):
        estimate = local_core_fixture()[0]
        estimate['totals']['raw_days'] = [4.5, 6.5]
        estimate['totals']['rounded_display_days'] = [5, 7]
        summary = m.current_core_summary(estimate)
        self.assertIn('**5–7 个有效工程日**', summary)
        self.assertIn('未取整 4.5–6.5', summary)
        self.assertIn('S2 共用假设不成立时 5–7', summary)

    def test_current_view_markers_ignore_history_but_reject_missing_duplicate_and_stale(self):
        estimate = local_core_fixture()[0]
        summary = m.current_core_summary(estimate)
        name = 'docs/roadmap.md'
        model = {'current_view_docs': [name]}
        good = '<!-- local-core-current:start -->\n' + summary + '\n<!-- local-core-current:end -->\n'
        with tempfile.TemporaryDirectory() as tmp:
            repo = Path(tmp); (repo / name).parent.mkdir(parents=True)
            (repo / name).write_text('Historical estimate 68–104 remains historical.\n' + good)
            checked = m.validate_current_view_docs(repo, model, estimate)
            self.assertEqual(set(checked), {name})
            for value in ('no markers', good + good, good.replace(summary, 'stale current price')):
                (repo / name).write_text(value)
                with self.subTest(value=value[:40]), self.assertRaisesRegex(ValueError, 'current.*view|marker'):
                    m.validate_current_view_docs(repo, model, estimate)


class RepositoryModelTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.backlog=json.loads((ROOT/'docs/planning/planning-backlog.json').read_text())
        cls.model=json.loads((ROOT/'docs/planning/execution-schedule-model.json').read_text())
        cls.scope=json.loads((ROOT/'docs/planning/planning-scope-estimates.json').read_text())
        cls.uaa=json.loads((ROOT/'docs/planning/uaa-implementation-estimate.json').read_text())

    def build(self,include_r=False,reserve=True,**overrides):
        args={k:copy.deepcopy(getattr(self,k)) for k in ['backlog','model','scope','uaa']};args.update(overrides)
        return m.build_model(**args,include_r=include_r,reserve=reserve)

    def source_link_fixture(self, root):
        names = ['docs/planning/planning-backlog.json', 'docs/planning/execution-schedule-model.json',
                 'docs/planning/planning-scope-estimates.json', 'docs/planning/uaa-implementation-estimate.json',
                 'docs/planning/uaa-real-integration-conditions.json', 'docs/planning/local-core-release-estimate.json',
                 'docs/planning/first-usable-release-target.json', 'docs/planning/owner-decision-agenda.json',
                 'docs/review/11-project-control.md', 'docs/roadmap.md']
        for name in names:
            path = root / name; path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes((ROOT / name).read_bytes())
        model_path = root / 'docs/planning/execution-schedule-model.json'
        model = json.loads(model_path.read_text())
        model['current_source_link_docs'] = ['docs/phase6/module-design-handoff.md']
        model_path.write_text(json.dumps(model))
        page = root / model['current_source_link_docs'][0]
        page.parent.mkdir(parents=True, exist_ok=True)
        text = ('<!-- local-core-source:start -->\n'
                '当前首版本地核心净估与条件以[唯一净估册](../planning/local-core-release-estimate.json)为准；本文不另立总价。\n'
                '<!-- local-core-source:end -->\n')
        page.write_text('历史估算68–104保持原时点。\n' + text)
        return page, text

    def test_non_authoritative_current_pages_are_checked_against_source_only_contract(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp); page, good = self.source_link_fixture(root)
            for bad in ('no markers', good + good,
                        good.replace('唯一净估册', '当前95–148日'),
                        good.replace('local-core-release-estimate.json', 'planning-backlog.json')):
                page.write_text(bad)
                with self.subTest(bad=bad[:40]), self.assertRaisesRegex(ValueError, 'source.*view|marker'):
                    m.calculate(root)

    def test_source_only_current_page_preserves_history_and_records_exact_authority(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp); page, good = self.source_link_fixture(root)
            result = m.calculate(root)
            self.assertIn('current_source_link_checks', result)
            self.assertEqual(set(result['current_source_link_checks']), {'docs/phase6/module-design-handoff.md'})
            self.assertEqual(result['current_source_link_checks']['docs/phase6/module-design-handoff.md']['source_ref'],
                             'docs/planning/local-core-release-estimate.json')

    def test_source_only_page_cannot_rebind_its_authority_to_another_existing_json(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp); page, good = self.source_link_fixture(root)
            path = root / 'docs/planning/execution-schedule-model.json'
            model = json.loads(path.read_text())
            model['local_core_net_estimate_ref'] = 'docs/planning/planning-backlog.json'
            path.write_text(json.dumps(model))
            page.write_text(good.replace('local-core-release-estimate.json', 'planning-backlog.json'))
            with self.assertRaisesRegex(ValueError, 'source.*authority'):
                m.calculate(root)

    def test_current_source_link_registry_keeps_each_selected_non_authoritative_page(self):
        self.assertEqual(set(self.model['current_source_link_docs']), {
            'docs/phase6/module-design-handoff.md', 'docs/phase6/task-activelist-message-detail.md',
            'docs/design/extension-compatibility.md', 'docs/design/authorization-model-and-engine-choice.md',
            'docs/design/rbac-inheritance-and-cascade.md'})

    def test_uaa_main_capacity_binding_cannot_drop_or_replace_core_stability_condition(self):
        for mutation in ('missing', 'formal_instead_of_stability', 'wrong_owner_action'):
            model = copy.deepcopy(self.model)
            model['uaa_capacity_gate_binding'] = dict(local_core_exit_id='uaa_capacity_transition',
                                                      owner_action_key='Q12_core_stability')
            if mutation == 'missing':
                del model['uaa_capacity_gate_binding']
            elif mutation == 'formal_instead_of_stability':
                model['uaa_capacity_gate_binding']['local_core_exit_id'] = 'formal_local'
            else:
                model['uaa_capacity_gate_binding']['owner_action_key'] = 'MH01_MR1'
            with self.subTest(mutation=mutation), self.assertRaisesRegex(ValueError, 'UAA.*capacity|stability'):
                self.build(model=model)

    def test_uaa_capacity_gate_covers_public_and_real_tasks_without_new_priced_node(self):
        result = m.calculate(ROOT)
        self.assertIn('uaa_main_capacity_gate', result)
        gate = result['uaa_main_capacity_gate']
        self.assertEqual(gate['local_core_exit_id'], 'uaa_capacity_transition')
        self.assertEqual(gate['owner_action_key'], 'Q12_core_stability')
        self.assertFalse(gate['preparation_contact_is_blocked'])
        for key, scenario in result['scenarios'].items():
            for profile in scenario['profiles'].values():
                rows = profile['resources']['1']['ordered_tasks']
                for row in rows:
                    if row['effort_category'] in ('uaa_public', 'uaa_real'):
                        self.assertEqual(row.get('capacity_gate_ref'), 'local_core.delivery_exits.uaa_capacity_transition')
                    else:
                        self.assertNotIn('capacity_gate_ref', row)
        self.assertEqual(result['bounded_effort_totals'],
                         {'public_R_pending': [394, 656], 'with_conditional_R_ready': [407, 678]})

    def test_historical_sixteen_remains_history_while_current_counts_follow_changed_source(self):
        rendered = m.render_backlog(self.backlog)
        self.assertIn('历史快照（2026-11-13，3f79a76）', rendered)
        self.assertIn('当时76父包/16子出口', rendered)
        self.assertIn('17正式子出口', rendered.splitlines()[2])
        changed = copy.deepcopy(self.backlog)
        item = next(item for item in changed['work_items'] if item.get('milestones'))
        child = copy.deepcopy(item['milestones'][0]); child['id'] = 'REFERENCE-NEW-CHILD'
        item['milestones'].append(child)
        header = m.render_backlog(changed).splitlines()[2]
        self.assertIn('18正式子出口', header)
        self.assertNotIn('17正式子出口', header)
        self.assertNotIn('16子出口', header)

    def test_long_range_current_accounting_text_uses_explicit_source_values(self):
        result = m.calculate(ROOT)
        self.assertEqual(result['long_horizon_accounting'], {
            'baseline_public_days': self.scope['baseline']['public_remaining'],
            'baseline_with_R_days': self.scope['baseline']['with_conditional_uaa_R'],
            'net_increment_days': self.scope['net_increment_total']})
        result['long_horizon_accounting'] = dict(baseline_public_days=[11, 17],
                                                baseline_with_R_days=[13, 23], net_increment_days=[7, 9])
        result['bounded_effort_totals'] = {'public_R_pending': [18, 26], 'with_conditional_R_ready': [20, 32]}
        rendered = m.render_schedule(result)
        self.assertIn('补充前公共11–17/含R13–23，净增7–9，当前公共18–26/含R20–32', rendered)

    def test_all_original_parents_and_child_scopes_survive(self):
        ids={x['id'] for x in self.backlog['work_items']};graph,children=m.source_graph(self.backlog)
        self.assertTrue(set(self.model['original_parent_ids']) <= ids)
        self.assertTrue(set(self.model['original_child_ids']) <= set(children))
        self.assertEqual(ids-set(self.model['original_parent_ids']),{'CE01','CE02','CE03','CE04','CE05'})
        self.assertEqual(len(children)-len(self.model['original_child_ids']),4)
        self.assertEqual(set(children)-set(self.model['original_child_ids']),{'CE02-S','CE02-D','CE02-L','X01-B'})
        self.assertEqual(len(set(x['id'] for x in self.backlog['additional_scope_tasks'])),6)

    def test_all_four_accounting_profiles_and_each_capacity_lane(self):
        for include_r in [False,True]:
            for reserve in [False,True]:
                nodes,categories,total=self.build(include_r,reserve)
                expected=m.summed([self.scope['baseline']['with_conditional_uaa_R' if include_r else 'public_remaining'],self.scope['net_increment_total']]+([self.scope['baseline']['maintenance_reserve']] if reserve else []))
                self.assertEqual(total,expected)
                for seats in [1,2]:
                    for endpoint in [0,1]:
                        run=m.run_scheduler(nodes,seats,endpoint)
                        self.assertEqual(run['effort_engineering_days'],total[endpoint])
                        for k,n in nodes.items():
                            self.assertTrue(all(run['records'][x]['finish']<=run['records'][k]['start'] for x in n['dependencies']))
                        for slot in range(1,seats+1):
                            seq=sorted((x['start'],x['finish']) for x in run['records'].values() if x['slot']==slot)
                            self.assertTrue(all(x[1]<=y[0] for x,y in zip(seq,seq[1:])))
                        if seats==1:self.assertEqual(run['makespan_workdays'],total[endpoint])

    def test_no_historical_or_parent_double_charge(self):
        nodes,_,_=self.build()
        for k in self.model['completed_parent_ids']:self.assertEqual(nodes[k]['days'],(0,0))
        for k in ['G03','D13','X01','X02','A03','CE02','E01']:self.assertEqual(nodes[k]['days'],(0,0))
        self.assertEqual(m.summed(n['days'] for n in nodes.values() if n.get('official_child')=='CE02-L'),(9,13))
        self.assertEqual(m.summed(n['days'] for n in nodes.values() if n['effort_category']=='uaa_public'),m.pair(self.uaa['P']['engineering_days']))

    def test_d12_uses_reliable_slice_without_reverse_i01_cycle(self):
        nodes,_,_=self.build();self.assertIn('CE02-D',nodes['D12']['dependencies']);self.assertNotIn('I01',nodes['D12']['dependencies'])
        b=copy.deepcopy(self.backlog);next(x for x in b['work_items'] if x['id']=='D12')['dependencies'].append('I01')
        with self.assertRaisesRegex(ValueError,'cycle'):self.build(backlog=b)

    def test_missing_new_dependency_and_unsliced_increment_fail(self):
        b=copy.deepcopy(self.backlog);b['work_items']=[x for x in b['work_items'] if x['id']!='CE02']
        with self.assertRaisesRegex(ValueError,'unknown'):m.source_graph(b)
        model=copy.deepcopy(self.model)
        for s in model['release_slices']:s['items']=[x for x in s['items'] if x!='LS01']
        with self.assertRaisesRegex(ValueError,'unsliced'):self.build(model=model)

    def test_child_price_cannot_drift_behind_zero_cost_parent(self):
        b=copy.deepcopy(self.backlog);next(x for x in b['work_items'] if x['id']=='CE02')['milestones'][0]['estimate_engineering_days']=[1,2]
        with self.assertRaisesRegex(ValueError,'decomposition'):self.build(backlog=b)

    def test_public_route_has_no_unresolved_real_node(self):
        nodes,_,_=self.build();self.assertNotIn('E01-UAA-R',nodes)
        self.assertNotIn('UAA-R-ENGINEERING',nodes)
        for n in nodes.values():self.assertNotIn('UAA_TEST_ENTRY_INPUT_READY',n['dependencies'])

    def ancestors(self,nodes,key):
        required=set()
        def visit(k):
            for dependency in nodes[k]['dependencies']:
                if dependency not in required:
                    required.add(dependency);visit(dependency)
        visit(key)
        return required

    def test_query_connector_does_not_wait_for_hr_lifecycle_or_feed(self):
        graph,_=m.source_graph(self.backlog)
        self.assertEqual(graph['X02-Q'],{'X01-C'})
        nodes,_,_=self.build()
        required=self.ancestors(nodes,'X02-Q')
        self.assertIn('X01-C',required)
        self.assertFalse({'D06','X02-F','E01-UAA-P'}&required)
        # The feed still has its actual source/identity safety prerequisites.
        self.assertTrue({'D06','X01-C','E01-UAA-C'}<=self.ancestors(nodes,'X02-F'))

    def test_device_slice_uses_basic_host_and_query_not_enhanced_host_or_feed(self):
        item=next(x for x in self.backlog['work_items'] if x['id']=='X03')
        self.assertEqual(set(item['dependencies']),{'U02','X01-B','X02-Q'})
        nodes,_,_=self.build()
        required=self.ancestors(nodes,'X03')
        self.assertFalse({'X01','X01-H','X02','X02-F','D06','E01-UAA-P'}&required)
        isolated={k:v for k,v in nodes.items() if k in required|{'X03'}}
        for endpoint in [0,1]:
            self.assertGreater(m.run_scheduler(isolated,1,endpoint)['records']['X03']['finish'],0)

    def test_host_basic_and_enhanced_slices_preserve_parent_price_and_old_id(self):
        item=next(x for x in self.backlog['work_items'] if x['id']=='X01')
        children={x['id']:x for x in item['milestones']}
        self.assertEqual(set(children),{'X01-C','X01-B','X01-H'})
        self.assertEqual(m.pair(item['estimate_engineering_days']),(8,12))
        self.assertEqual(m.pair(children['X01-C']['estimate_engineering_days']),(2,3))
        self.assertEqual(m.pair(children['X01-B']['estimate_engineering_days']),(3,4))
        self.assertEqual(m.pair(children['X01-H']['estimate_engineering_days']),(3,5))
        self.assertEqual(m.summed(x['estimate_engineering_days'] for x in children.values()),(8,12))
        nodes,_,_=self.build()
        self.assertEqual(nodes['X01-B']['slice'],'S04')
        self.assertEqual(nodes['X01-H']['slice'],'S09')
        self.assertEqual(nodes['X01-H']['wave'],'P7')
        self.assertIn('X01-B',nodes['X01-H']['dependencies'])

    def test_foundation_is_executable_without_full_uaa_or_future_host(self):
        nodes,_,_=self.build()
        targets=set(self.model['delivery_priority_groups']['foundation']['targets'])
        self.assertTrue(targets<=set(nodes))
        required=targets|set().union(*(self.ancestors(nodes,k) for k in targets))
        self.assertFalse({'E01-UAA-P','X01-H','D06','X02-F'}&required)
        isolated={k:v for k,v in nodes.items() if k in required}
        for seats in [1,2]:
            for endpoint in [0,1]:
                run=m.run_scheduler(isolated,seats,endpoint)
                self.assertGreater(max(run['records'][k]['finish'] for k in targets),0)

    def test_public_priority_delivers_foundation_then_risk_before_full_uaa(self):
        for reserve in [False,True]:
            nodes,_,_=self.build(reserve=reserve)
            self.assertEqual({n['priority'] for n in nodes.values() if n['effort_category']=='uaa_public'},{7.5})
            for endpoint in [0,1]:
                run=m.run_scheduler(nodes,1,endpoint);records=run['records']
                foundation=max(records[k]['finish'] for k in self.model['delivery_priority_groups']['foundation']['targets'])
                self.assertLessEqual(foundation,records['D08']['start'])
                self.assertLessEqual(records['E10']['finish'],records['D08']['finish'])
                self.assertLessEqual(records['E10']['finish'],records['D10']['start'])
                self.assertLessEqual(max(records[k]['finish'] for k in ['D10','D15']),records['E01-UAA-P']['finish'])
                self.assertLessEqual(foundation,min(records[k]['start'] for k,n in nodes.items() if n['effort_category']=='uaa_public'))

    def test_selected_ready_enterprise_lane_preserves_foundation_before_uaa_r(self):
        nodes,_,_=self.build(include_r=True)
        self.assertLess(nodes['UAA-R-ENGINEERING']['priority'],nodes['D10']['priority'])
        for endpoint in [0,1]:
            records=m.run_scheduler(nodes,1,endpoint)['records']
            foundation=max(records[k]['finish'] for k in self.model['delivery_priority_groups']['foundation']['targets'])
            self.assertLessEqual(foundation,min(records[k]['start'] for k,n in nodes.items() if n['effort_category']=='uaa_public'))
            self.assertLessEqual(records['E01-UAA-R']['finish'],records['D08']['start'])
        self.assertEqual(nodes['UAA-R-ENGINEERING']['external_ready'],'UAA_TEST_ENTRY_INPUT_READY')

    def test_schedule_render_counts_follow_structure_not_old_sixteen_constant(self):
        result=m.calculate(ROOT)
        self.assertEqual(result['source_structure']['official_child_milestones'],17)
        self.assertIn('17正式子出口',m.render_schedule(result))
        self.assertIn('17正式子出口',m.render_backlog(self.backlog))
        self.assertEqual(result['date'],self.model['date'])
        self.assertIn(self.model['date'],m.render_schedule(result))
        self.assertIn(self.backlog['date'],m.render_backlog(self.backlog))

    def test_foundation_exit_reports_actual_target_window_not_long_horizon_total(self):
        result=m.calculate(ROOT)
        self.assertTrue(result['delivery_scope_boundaries']['long_horizon_totals_are_not_foundation_duration'])
        self.assertFalse(result['delivery_scope_boundaries']['production_or_enterprise_activation_approved'])
        for scenario in result['scenarios'].values():
            for profile in scenario['profiles'].values():
                for resource in profile['resources'].values():
                    row=resource['delivery_exits']['foundation']
                    expected=[max(resource['resource_records'][endpoint][k]['finish'] for k in row['targets']) for endpoint in ['low','high']]
                    self.assertEqual(row['finish_endpoint_days'],expected)
                    self.assertTrue(all(a<b for a,b in zip(row['effort'],profile['total_effort_range'])))

    def test_foundation_requires_attachment_correctness_before_declaring_exit(self):
        with self.subTest(scope='declared foundation targets'):
            self.assertIn('C02',self.model['delivery_priority_groups']['foundation']['targets'])
        result=m.calculate(ROOT)
        for scenario_name,scenario in result['scenarios'].items():
            for profile_name,profile in scenario['profiles'].items():
                for seats,resource in profile['resources'].items():
                    row=resource['delivery_exits']['foundation']
                    with self.subTest(scenario=scenario_name,profile=profile_name,seats=seats,scope='output targets'):
                        self.assertIn('C02',row['targets'])
                    for index,endpoint in enumerate(['low','high']):
                        with self.subTest(scenario=scenario_name,profile=profile_name,seats=seats,endpoint=endpoint):
                            self.assertLessEqual(resource['resource_records'][endpoint]['C02']['finish'],row['finish_endpoint_days'][index])

    def test_e10_investigation_has_early_scope_and_no_late_risk_dependency(self):
        item=next(x for x in self.backlog['work_items'] if x['id']=='E10')
        self.assertEqual(item['wave'],'P6.1')
        self.assertEqual(m.pair(item['estimate_engineering_days']),(2,4))
        self.assertEqual(set(item['dependencies']),{'D01','D03','X02'})
        nodes,_,_=self.build()
        self.assertEqual(nodes['E10']['slice'],'S04')
        required=set()
        def ancestors(k):
            for dependency in nodes[k]['dependencies']:
                if dependency not in required:
                    required.add(dependency);ancestors(dependency)
        ancestors('E10')
        self.assertTrue({'D01','D03','X02'}<=required)
        self.assertFalse({'D08','D10'}&required)
        # The priced investigation can complete with later risk implementation absent.
        isolated={k:v for k,v in nodes.items() if k in required|{'E10'}}
        for endpoint in [0,1]:
            self.assertGreater(m.run_scheduler(isolated,1,endpoint)['records']['E10']['finish'],0)

    def test_e10_investigation_completes_within_p61_before_late_risk_implementation(self):
        for include_r in [False,True]:
            for reserve in [False,True]:
                nodes,_,_=self.build(include_r,reserve)
                for seats in [1,2]:
                    for endpoint in [0,1]:
                        run=m.run_scheduler(nodes,seats,endpoint)
                        records=run['records'];finish=records['E10']['finish']
                        other_p61_exit=max(records[k]['finish'] for k,n in nodes.items() if n['wave']=='P6.1' and k!='E10')
                        # E10 may be the final P6.1 task; it must enter that phase's
                        # active window, rather than waiting behind later risk work.
                        self.assertLessEqual(records['E10']['start'],other_p61_exit)
                        # Owner's incremental-boundaries direction permits this
                        # finite investigation to overlap mock risk implementation.
                        # Keep its early exit before risk closure, without adding a
                        # fictitious business dependency or whole-phase barrier.
                        self.assertLessEqual(finish,records['D08']['finish'])
                        self.assertLessEqual(finish,records['D10']['start'])

    def test_complete_calculation_and_serialization_are_deterministic(self):
        a=m.calculate(ROOT);b=m.calculate(ROOT);self.assertEqual(a,b)
        encoded=m.dump_schedule(a);self.assertEqual(json.loads(encoded),a)
        self.assertEqual(a['scenarios']['public_R_pending']['profiles']['without_maintenance_reserve']['total_effort_range'],[394,656])


if __name__=='__main__':unittest.main()
