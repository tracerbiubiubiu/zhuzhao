#!/usr/bin/env python3
"""Black-box synthetic B1 checks; never prints tokens or response records.

Run with B1_CORE_ORIGIN, B1_TOKEN_FILE, B1_ALLOWED_ORG and B1_OTHER_ORG.
Fixtures must contain at least two distinct in-scope rows and one out-of-scope
row. Set B1_MODE=revoked or disabled after performing that operation through the
real management/configuration path; neither mode modifies server state.
"""
import json
import os
import sys
from urllib.error import HTTPError
from urllib.parse import urlencode
from urllib.request import Request, urlopen

BASE = os.environ.get('B1_CORE_ORIGIN', 'http://127.0.0.1:45333').rstrip('/')
PATH = '/api/v1/extensions/reference-assets'


def request(token, params=(), extra_headers=None):
    url = BASE + PATH
    if params:
        url += '?' + urlencode(params, doseq=True)
    headers = {'Authorization': 'Bearer ' + token} if token else {}
    headers.update(extra_headers or {})
    try:
        with urlopen(Request(url, headers=headers), timeout=8) as response:
            return response.status, json.load(response)
    except HTTPError as error:
        try:
            body = json.load(error)
        except (ValueError, UnicodeDecodeError):
            body = None
        return error.code, body


def require(condition, description):
    if not condition:
        raise AssertionError(description)
    print('PASS', description, flush=True)


def main():
    token_file = os.environ['B1_TOKEN_FILE']
    with open(token_file, encoding='utf-8') as file:
        token = file.read().strip()
    allowed, other = os.environ['B1_ALLOWED_ORG'], os.environ['B1_OTHER_ORG']
    require(bool(token) and allowed != other, 'dedicated subject and distinct fixture scopes supplied')
    mode = os.environ.get('B1_MODE', 'positive')
    status, body = request(token, [('limit', '1')])
    if mode in ('revoked', 'disabled'):
        require(status in ((401, 403) if mode == 'revoked' else (403, 404)), mode + ' refuses access')
        require(not isinstance(body, dict) or not body.get('items'), mode + ' returns no records')
        return
    require(mode == 'positive', 'recognized test mode')
    require(status == 200 and isinstance(body, dict), 'authenticated reference query succeeds')
    require(isinstance(body.get('items'), list) and len(body['items']) == 1, 'limit returns one item')
    require(body.get('source_id') == 'ref-assets-v1' and bool(body.get('fetched_at')) and bool(body.get('mapping_version')), 'source and freshness metadata present')
    cursor = body.get('next_cursor')
    require(isinstance(cursor, str) and bool(cursor), 'first page has continuation cursor')
    first = body['items'][0].get('device_id')
    require(bool(first), 'first page has stable device identity')
    status, page = request(token, [('limit', '1'), ('cursor', cursor)])
    require(status == 200 and isinstance(page, dict) and len(page.get('items', [])) == 1, 'next page succeeds')
    require(page['items'][0].get('device_id') not in (None, first), 'pagination yields distinct records')
    status, scoped = request(token, [('owner_org_refs', allowed), ('owner_org_refs', other), ('limit', '50')])
    require(status == 200 and isinstance(scoped, dict) and bool(scoped.get('items')), 'mixed organization filter retains authorized data')
    require(all(str(item.get('owner_org_ref')) == allowed for item in scoped['items']), 'cross-organization records remain hidden')
    status, forged = request(token, [('owner_org_refs', other), ('limit', '50')], {'X-Operator': 'E000001'})
    require(status == 200 and isinstance(forged, dict) and forged.get('items') == [], 'untrusted identity claim does not expand access')
    status, fields = request(token, [('requested_fields', 'device_id'), ('limit', '50')])
    require(status == 200 and isinstance(fields, dict) and bool(fields.get('items')), 'field projection query succeeds')
    require(all(set(item) <= {'device_id'} and item.get('device_id') for item in fields['items']), 'field projection excludes unrequested fields')
    status, empty = request(token, [('status', 'down'), ('limit', '50')])
    require(status == 200 and isinstance(empty, dict) and empty.get('items') == [] and empty.get('complete') is True, 'empty result remains distinguishable from failure')
    status, _ = request(token, [('requested_fields', 'not_a_field')])
    require(status in (400, 403, 422), 'unregistered field rejected')
    status, _ = request('', [('limit', '1')])
    require(status in (401, 403), 'anonymous access rejected')


if __name__ == '__main__':
    try:
        main()
    except (AssertionError, KeyError, OSError, ValueError) as error:
        print('FAIL', str(error), file=sys.stderr)
        sys.exit(1)
