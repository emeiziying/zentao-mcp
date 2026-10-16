#!/usr/bin/env python3
"""Exercise every enabled v1 operation against an isolated fake upstream via MCP.
Usage: python3 scripts/test-v1-contract.py /absolute/path/to/zentao-mcp
No real ZenTao URL or credentials are used.
"""
import json
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT=Path(__file__).resolve().parents[1]
seen=[]
class Upstream(BaseHTTPRequestHandler):
    def log_message(self,*args):pass
    def handle_request(self):
        body=self.rfile.read(int(self.headers.get('Content-Length','0')))
        seen.append({'method':self.command,'path':self.path,'token':self.headers.get('Token'),'body':json.loads(body) if body else None})
        self.send_response(200);self.send_header('Content-Type','application/json');self.end_headers()
        self.wfile.write(b'{"id":1,"profile":{},"products":[],"tasks":[]}')
    do_GET=do_POST=do_PUT=do_DELETE=handle_request

def example(schema):
    if 'const' in schema:return schema['const']
    if schema.get('enum'):return schema['enum'][0]
    for key in ('anyOf','oneOf'):
        if schema.get(key):return example(schema[key][0])
    typ=schema.get('type','object')
    if isinstance(typ,list):typ=next(x for x in typ if x!='null')
    if typ=='object':
        result={k:example(v) for k,v in schema.get('properties',{}).items() if k in schema.get('required',[])}
        # Include optional properties to verify parameter serialization too.
        for k,v in schema.get('properties',{}).items():result.setdefault(k,example(v))
        return result
    if typ=='array':return [example(schema.get('items',{'type':'string'}))]
    if typ in ('number','integer'):return max(1,schema.get('minimum',1))
    if typ=='boolean':return True
    if schema.get('format')=='date':return '2026-01-01'
    return 'fixture'

def main():
    spec=json.loads((ROOT/'docs/zentao-v1.json').read_text())
    operations={op['operationId']:(method.upper(),path,op) for path,entry in spec['paths'].items() for method,op in entry.items() if method in ('get','post','put','delete') and path!='/tokens'}
    upstream=ThreadingHTTPServer(('127.0.0.1',0),Upstream)
    thread=threading.Thread(target=upstream.serve_forever,daemon=True);thread.start()
    with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
    with tempfile.TemporaryDirectory() as temp:
        config={'port':f'127.0.0.1:{port}','servers':[{'name':'v1','schema_url':str(ROOT/'docs/zentao-v1.json'),'base_url':f'http://127.0.0.1:{upstream.server_port}/api.php/v1','allow':[{'methods':['GET','POST','PUT','DELETE'],'regex':'.*'}],'block':[{'methods':['POST'],'regex':'^/tokens$'}]}]}
        cfg=Path(temp)/'config.json';cfg.write_text(json.dumps(config))
        proc=subprocess.Popen([str(Path(sys.argv[1]).resolve()),'-config',str(cfg)],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
        session=None;counter=0
        def rpc(method,params={},notify=False):
            nonlocal session,counter
            counter+=1;msg={'jsonrpc':'2.0','method':method,'params':params}
            if not notify:msg['id']=counter
            headers={'Content-Type':'application/json','Accept':'application/json, text/event-stream','Token':'contract-fixture','MCP-Protocol-Version':'2025-06-18'}
            if session:headers['Mcp-Session-Id']=session
            with urllib.request.urlopen(urllib.request.Request(f'http://127.0.0.1:{port}/v1/mcp',data=json.dumps(msg).encode(),headers=headers),timeout=10) as response:
                session=response.headers.get('Mcp-Session-Id',session);raw=response.read().decode()
                if not raw:return {}
                if 'text/event-stream' in response.headers.get('Content-Type',''):raw=next(x[6:] for x in raw.splitlines() if x.startswith('data: '))
                data=json.loads(raw)
            assert 'error' not in data, (method,data.get('error'))
            return data['result']
        try:
            for _ in range(100):
                try:
                    with socket.create_connection(('127.0.0.1',port),timeout=.1):break
                except OSError:
                    if proc.poll() is not None:raise RuntimeError('MCP startup failed')
                    time.sleep(.1)
            rpc('initialize',{'protocolVersion':'2025-06-18','capabilities':{},'clientInfo':{'name':'contract','version':'1'}})
            rpc('notifications/initialized',notify=True)
            tools=rpc('tools/list')['tools']
            aggregates={'zentao_my_task_history','zentao_my_bug_history'}
            assert {t['name'] for t in tools}==set(operations)|aggregates,(len(tools),len(operations))
            assert all(t.get('annotations',{}).get('readOnlyHint') is True for t in tools if t['name'] in aggregates)
            tools=[t for t in tools if t['name'] in operations]
            for tool in tools:
                method,path,op=operations[tool['name']];args=example(tool['inputSchema'])
                before=len(seen)
                result=rpc('tools/call',{'name':tool['name'],'arguments':args})
                assert not result.get('isError'),tool['name']
                assert len(seen)==before+1,tool['name']
                call=seen[-1];assert call['method']==method and call['token']=='contract-fixture',tool['name']
                for param in op.get('parameters',[]):
                    if param['in']=='path':path=path.replace('{'+param['name']+'}',str(args[param['name']]))
                assert call['path'].split('?')[0]=='/api.php/v1'+path,(tool['name'],call['path'],path)
                assert call['body']==args.get('payload'),tool['name']
                assert bool(tool.get('annotations',{}).get('readOnlyHint'))==(method=='GET'),tool['name']
            print(json.dumps({'operations_passed':len(tools),'real_business_writes':0,'token_tool_excluded':True}))
        finally:
            proc.terminate();proc.wait(timeout=10);upstream.shutdown();upstream.server_close();thread.join()

if __name__=='__main__':main()
