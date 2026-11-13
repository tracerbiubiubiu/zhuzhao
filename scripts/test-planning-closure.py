#!/usr/bin/env python3
"""Behavior oracles for false-completion, stale-source and missing-evidence paths."""
import copy
import hashlib
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

CHECKER = Path(__file__).with_name('check-planning-closure.py')
REQUIRED = ['source-coverage', 'documents', 'contracts', 'schedule', 'independent-review',
            'runtime-core', 'runtime-ui', 'runtime-activelist', 'runtime-taskrunner', 'runtime-utils']


class CloseoutTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.write('sources.json', {'sources': [{'ref': 'U01'}, {'ref': 'U02'}], 'requirements': [
            {'id': 'R01', 'source_refs': ['U01']}, {'id': 'R02', 'source_refs': ['U02']}]})
        self.write('prices.json', {'items': [{'id': 'CE01', 'net_days': [2, 3]}]})
        self.write('evidence.json', {
            'binding': {'source_count': 2, 'source_cutoff': 'U02',
                        'source_registry_sha256': hashlib.sha256((self.root/'sources.json').read_bytes()).hexdigest(),
                        'estimate_registry_sha256': hashlib.sha256((self.root/'prices.json').read_bytes()).hexdigest()},
            'checks': {key: {'id': key, 'result': 'PASS'} for key in REQUIRED},
        })
        digest = hashlib.sha256((self.root/'evidence.json').read_bytes()).hexdigest()
        proofs = [{'path': 'evidence.json', 'sha256': digest, 'result_key': 'checks.'+key,
                   'check_id': key, 'scope': 'synthetic'} for key in REQUIRED]
        self.registry = {
            'schema_version': 1, 'source_registry': 'sources.json', 'source_count': 2,
            'source_cutoff': 'U02', 'estimate_registry': 'prices.json',
            'claims': {'audit_complete': True, 'full_latest_scope_priced': True, 'real_integration_passed': False},
            'actions': [{'id': 'M%02d' % i, 'state': 'verified', 'requirement_ids': ['R01', 'R02'],
                         'owner': 'reviewer', 'exit': 'specified oracle checked', 'evidence': copy.deepcopy(proofs)}
                        for i in range(1, 7)],
            'checks': [{'id': key, 'result': 'PASS', 'evidence': copy.deepcopy(proof)}
                       for key, proof in zip(REQUIRED, proofs)]
        }

    def write(self, name, data):
        (self.root/name).write_text(json.dumps(data, ensure_ascii=False)+'\n')

    def run_case(self, expected, diagnostic=''):
        self.write('closure.json', self.registry)
        p = subprocess.run([sys.executable, str(CHECKER), '--root', str(self.root),
                            '--registry', 'closure.json'], text=True, capture_output=True)
        self.assertEqual(p.returncode, expected, p.stdout+p.stderr)
        if diagnostic:
            self.assertIn(diagnostic, p.stdout+p.stderr)

    def test_valid_complete_fixture(self):
        self.run_case(0)

    def test_unfinished_action_cannot_claim_complete(self):
        self.registry['actions'][2]['state'] = 'pending'
        self.run_case(1, 'unfinished action M03')

    def test_missing_action_not_hidden_by_count(self):
        self.registry['actions'][-1]['id'] = 'M99'
        self.run_case(1, 'required actions')

    def test_later_source_cannot_be_left_out(self):
        p = json.loads((self.root/'sources.json').read_text())
        p['sources'].append({'ref': 'U03'})
        self.write('sources.json', p)
        self.run_case(1, 'source cutoff/count')

    def test_requirement_needs_actual_disposition(self):
        for a in self.registry['actions']:
            a['requirement_ids'] = ['R01']
        self.run_case(1, 'unmapped requirements')

    def test_document_existence_is_not_passing_evidence(self):
        data = json.loads((self.root/'evidence.json').read_text())
        for check in data['checks'].values():
            check['result'] = 'NOT_RUN'
        self.write('evidence.json', data)
        digest = hashlib.sha256((self.root/'evidence.json').read_bytes()).hexdigest()
        for a in self.registry['actions']:
            for proof in a['evidence']:
                proof['sha256'] = digest
        for c in self.registry['checks']:
            c['evidence']['sha256'] = digest
        self.run_case(1, 'evidence result is not PASS')

    def test_stale_evidence_hash_rejected(self):
        self.write('evidence.json', {'result': 'PASS', 'changed': True})
        self.run_case(1, 'evidence hash')

    def test_runtime_not_run_is_not_exempted_by_docs_label(self):
        self.registry['checks'][5]['result'] = 'NOT_RUN'
        self.run_case(1, 'required check runtime-core')

    def test_null_increment_not_free_complete_schedule(self):
        self.write('prices.json', {'items': [{'id': 'CE01', 'net_days': None}]})
        self.run_case(1, 'unpriced item CE01')

    def test_mock_proof_cannot_be_claimed_as_real(self):
        self.registry['claims']['real_integration_passed'] = True
        self.run_case(1, 'synthetic evidence cannot prove real integration')

    def test_duplicate_requirement_id_rejected(self):
        p = json.loads((self.root/'sources.json').read_text())
        p['requirements'].append(copy.deepcopy(p['requirements'][0]))
        self.write('sources.json', p)
        self.run_case(1, 'duplicate requirement ids')

    def test_each_source_needs_requirement_mapping(self):
        p = json.loads((self.root/'sources.json').read_text())
        p['requirements'][1]['source_refs'] = ['U01']
        self.write('sources.json', p)
        self.run_case(1, 'unmapped sources')

    def test_proof_cannot_impersonate_another_check(self):
        self.registry['checks'][0]['evidence']['check_id'] = 'runtime-core'
        self.run_case(1, 'evidence check identity')

    def test_generic_PASS_document_not_a_complete_proof(self):
        self.write('evidence.json', {'result': 'PASS'})
        digest = hashlib.sha256((self.root/'evidence.json').read_bytes()).hexdigest()
        for a in self.registry['actions']:
            for proof in a['evidence']:
                proof['sha256'] = digest
                proof['result_key'] = 'result'
        for c in self.registry['checks']:
            c['evidence']['sha256'] = digest
            c['evidence']['result_key'] = 'result'
        self.run_case(1, 'evidence binding')

    def test_action_requires_relevant_checks(self):
        self.registry['actions'][0]['evidence'] = [copy.deepcopy(self.registry['checks'][5]['evidence'])]
        self.run_case(1, 'action evidence coverage M01')

    def test_planning_proof_cannot_claim_business_implementation(self):
        self.registry['claims']['business_implemented'] = True
        self.run_case(1, 'unsupported implementation/future-freeze claim')

    def test_planning_proof_cannot_claim_all_future_details(self):
        self.registry['claims']['all_future_details_frozen'] = True
        self.run_case(1, 'unsupported implementation/future-freeze claim')

    def test_proof_bound_to_source_content_not_just_count(self):
        data = json.loads((self.root/'sources.json').read_text())
        data['requirements'][0]['requirement'] = 'materially changed scope'
        self.write('sources.json', data)
        self.run_case(1, 'evidence binding')

    def test_proof_bound_to_estimate_content(self):
        self.write('prices.json', {'items': [{'id': 'CE01', 'net_days': [3, 4]}]})
        self.run_case(1, 'evidence binding')


if __name__ == '__main__':
    unittest.main()
