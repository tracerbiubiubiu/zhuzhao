#!/usr/bin/env python3
"""Behavioral tests for scope accounting, graph integrity and capacity scheduling."""
import copy
import importlib.util
import json
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
        b=json.loads((ROOT/'docs/review/planning-backlog.json').read_text());e=json.loads((ROOT/'docs/review/planning-scope-estimates.json').read_text());u,extra=m.apply_scope(b,e)
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


class RepositoryModelTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.backlog=json.loads((ROOT/'docs/review/planning-backlog.json').read_text())
        cls.model=json.loads((ROOT/'docs/review/execution-schedule-model.json').read_text())
        cls.scope=json.loads((ROOT/'docs/review/planning-scope-estimates.json').read_text())
        cls.uaa=json.loads((ROOT/'docs/review/uaa-implementation-estimate.json').read_text())

    def build(self,include_r=False,reserve=True,**overrides):
        args={k:copy.deepcopy(getattr(self,k)) for k in ['backlog','model','scope','uaa']};args.update(overrides)
        return m.build_model(**args,include_r=include_r,reserve=reserve)

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
