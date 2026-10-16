#!/usr/bin/env python3
"""Read-only MCP smoke check. Credentials are read from environment, never printed."""
import json
import os
import urllib.request

url = os.environ.get('ZENTAO_MCP_URL', 'http://127.0.0.1:9090/zentao/mcp')
token = os.environ['ZENTAO_TOKEN']
session = None
counter = 0


def rpc(method, params=None, notification=False):
    global session, counter
    counter += 1
    message = {'jsonrpc': '2.0', 'method': method, 'params': params or {}}
    if not notification:
        message['id'] = counter
    headers = {'Content-Type': 'application/json', 'Accept': 'application/json, text/event-stream',
               'Token': token, 'MCP-Protocol-Version': '2025-06-18'}
    if session:
        headers['Mcp-Session-Id'] = session
    req = urllib.request.Request(url, data=json.dumps(message).encode(), headers=headers)
    with urllib.request.urlopen(req, timeout=30) as response:
        session = response.headers.get('Mcp-Session-Id', session)
        raw = response.read().decode()
        if not raw:
            return {}
        if 'text/event-stream' in response.headers.get('Content-Type', ''):
            raw = next(line[6:] for line in raw.splitlines() if line.startswith('data: '))
        data = json.loads(raw)
    if 'error' in data:
        raise RuntimeError('MCP JSON-RPC failure')
    return data['result']


def check(name, arguments, key):
    result = rpc('tools/call', {'name': name, 'arguments': arguments})
    if result.get('isError'):
        raise RuntimeError(name + ' failed')
    data = result.get('structuredContent')
    if not isinstance(data, dict) or key not in data:
        raise RuntimeError(name + ' returned unexpected data')
    print(name + ': passed')


rpc('initialize', {'protocolVersion': '2025-06-18', 'capabilities': {},
                   'clientInfo': {'name': 'zentao-v1-smoke', 'version': '1'}})
rpc('notifications/initialized', notification=True)
tools = rpc('tools/list')['tools']
expected = {'zentao_current_user', 'zentao_products', 'zentao_task_detail'}
assert expected <= {tool['name'] for tool in tools}
assert len(tools) == 38
assert all(tool.get('annotations', {}).get('readOnlyHint') for tool in tools)
print('readonly tool list: passed')
check('zentao_current_user', {'fields': 'task', 'limit': 1}, 'profile')
check('zentao_products', {'limit': 1}, 'products')
if os.environ.get('ZENTAO_TEST_TASK_ID'):
    check('zentao_task_detail', {'id': int(os.environ['ZENTAO_TEST_TASK_ID'])}, 'id')
