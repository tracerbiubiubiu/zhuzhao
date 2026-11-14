#!/usr/bin/env python3
"""Check explicit planning-closeout invariants; never infer semantic coverage."""
import argparse
import hashlib
import json
import sys
from pathlib import Path

REQUIRED_ACTIONS = {'M%02d' % i for i in range(1, 7)}
REQUIRED_CHECKS = {'source-coverage', 'documents', 'contracts', 'schedule', 'independent-review',
                   'runtime-core', 'runtime-ui', 'runtime-activelist', 'runtime-taskrunner', 'runtime-utils'}
ACTION_CHECKS = {
    'M01': {'documents', 'contracts', 'independent-review'},
    'M02': {'contracts', 'independent-review'},
    'M03': {'contracts', 'independent-review'},
    'M04': {'schedule', 'independent-review'},
    'M05': {'documents', 'runtime-core', 'runtime-ui', 'runtime-activelist',
            'runtime-taskrunner', 'runtime-utils'},
    'M06': {'source-coverage', 'documents', 'independent-review'},
}


def resolve(root, name):
    path = (root / name).resolve()
    allowed = [root] + [root.parent / n for n in ('zhuzhao', 'zhuzhao-ui', 'activelist', 'taskrunner', 'zhuzhao-utils')]
    if not any(path.is_relative_to(p.resolve()) for p in allowed):
        raise ValueError('evidence path outside repository family')
    return path


def read(root, name):
    return json.loads(resolve(root, name).read_text())


def validate(root, data):
    errors = []
    def check(ok, message):
        if not ok:
            errors.append(message)

    expected_binding = {
        'source_count': data['source_count'], 'source_cutoff': data['source_cutoff'],
        'source_registry_sha256': hashlib.sha256(resolve(root, data['source_registry']).read_bytes()).hexdigest(),
        'estimate_registry_sha256': hashlib.sha256(resolve(root, data['estimate_registry']).read_bytes()).hexdigest(),
    }

    def evidence(row, expected_check=None):
        try:
            path = resolve(root, row['path'])
            content = path.read_bytes()
            check(hashlib.sha256(content).hexdigest() == row.get('sha256'), 'evidence hash: '+row['path'])
            capsule = json.loads(content)
            check(capsule.get('binding') == expected_binding, 'evidence binding: '+row['path'])
            identity = row.get('check_id')
            check(identity in REQUIRED_CHECKS and (expected_check is None or identity == expected_check),
                  'evidence check identity: '+row['path'])
            result = capsule
            for key in row.get('result_key', 'result').split('.'):
                result = result[key]
            check(isinstance(result, dict) and result.get('id') == identity,
                  'evidence check identity in content: '+row['path'])
            check(isinstance(result, dict) and result.get('result') == 'PASS',
                  'evidence result is not PASS: '+row['path'])
        except (OSError, ValueError, KeyError, TypeError) as exc:
            errors.append('missing/invalid evidence: '+str(exc))

    check(data.get('schema_version') == 1, 'unsupported closure schema')
    check(not data['claims'].get('business_implemented') and
          not data['claims'].get('all_future_details_frozen'),
          'unsupported implementation/future-freeze claim in planning evidence')
    sources = read(root, data['source_registry'])
    refs = [s['ref'] for s in sources['sources']]
    reqs = {r['id']: r for r in sources['requirements']}
    check(len(refs) == len(set(refs)), 'duplicate source refs')
    check(len(reqs) == len(sources['requirements']), 'duplicate requirement ids')
    check(bool(refs) and len(refs) == data['source_count'] and refs[-1] == data['source_cutoff'], 'source cutoff/count')
    check(all(r.get('source_refs') and set(r['source_refs']) <= set(refs) for r in reqs.values()), 'invalid requirement sources')
    covered_sources = {ref for r in sources['requirements'] for ref in r.get('source_refs', [])}
    check(set(refs) <= covered_sources, 'unmapped sources: '+','.join(sorted(set(refs)-covered_sources)))
    actions = data['actions']
    check({a['id'] for a in actions} == REQUIRED_ACTIONS and len(actions) == len(REQUIRED_ACTIONS), 'required actions')
    mapped = set()
    complete = data['claims'].get('audit_complete', False)
    for action in actions:
        mapped.update(action.get('requirement_ids', []))
        check(bool(action.get('owner')) and bool(action.get('exit')), 'action lacks owner/exit: '+action['id'])
        check(set(action.get('requirement_ids', [])) <= reqs.keys(), 'unknown requirements: '+action['id'])
        if complete:
            check(action.get('state') == 'verified', 'unfinished action '+action['id'])
            check(bool(action.get('evidence')), 'action has no evidence: '+action['id'])
            check(ACTION_CHECKS.get(action['id'], set()) <=
                  {p.get('check_id') for p in action.get('evidence', [])},
                  'action evidence coverage '+action['id'])
            for proof in action.get('evidence', []):
                evidence(proof)
    check(mapped >= reqs.keys(), 'unmapped requirements: '+','.join(sorted(reqs.keys()-mapped)))
    checks = {c['id']: c for c in data.get('checks', [])}
    check(len(checks) == len(data.get('checks', [])), 'duplicate check ids')
    check(REQUIRED_CHECKS <= checks.keys(), 'required checks missing')
    if complete:
        for name in sorted(REQUIRED_CHECKS):
            if name not in checks:
                continue
            check(checks[name].get('result') == 'PASS', 'required check '+name+' is not PASS')
            evidence(checks[name].get('evidence', {}), name)
    if data['claims'].get('full_latest_scope_priced'):
        prices = read(root, data['estimate_registry'])
        check(bool(prices.get('items')), 'empty estimate scope')
        price_ids = [i['id'] for i in prices.get('items', [])]
        check(len(price_ids) == len(set(price_ids)), 'duplicate priced items')
        for item in prices.get('items', []):
            days = item.get('net_days')
            check(isinstance(days, list) and len(days) == 2 and
                  all(isinstance(n, int) and not isinstance(n, bool) and n >= 0 for n in days) and
                  days[0] <= days[1], 'unpriced item '+item['id'])
    if data['claims'].get('real_integration_passed'):
        proofs = [p for a in actions for p in a.get('evidence', [])]
        check(bool(proofs) and all(p.get('scope') == 'real' for p in proofs),
              'synthetic evidence cannot prove real integration')
    return errors


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path(__file__).resolve().parents[1])
    parser.add_argument('--registry', type=Path, default=Path('docs/review/planning-closure.json'))
    args = parser.parse_args()
    try:
        root = args.root.resolve()
        errors = validate(root, read(root, str(args.registry)))
    except (OSError, ValueError, KeyError, TypeError) as exc:
        print('[ERROR] invalid planning registry: '+str(exc), file=sys.stderr)
        return 2
    for error in errors:
        print('[FAIL] '+error, file=sys.stderr)
    if errors:
        return 1
    print('[OK] planning source/action/check/estimate invariants; semantic review remains mandatory')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
