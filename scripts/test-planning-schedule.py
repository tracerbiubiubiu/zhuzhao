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
        self.assertEqual(len(children)-len(self.model['original_child_ids']),3)
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

    def test_complete_calculation_and_serialization_are_deterministic(self):
        a=m.calculate(ROOT);b=m.calculate(ROOT);self.assertEqual(a,b)
        encoded=m.dump_schedule(a);self.assertEqual(json.loads(encoded),a)
        self.assertEqual(a['scenarios']['public_R_pending']['profiles']['without_maintenance_reserve']['total_effort_range'],[394,656])


if __name__=='__main__':unittest.main()
