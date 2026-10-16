#!/usr/bin/env python3
"""Generate REST v1 OpenAPI from reviewed official field inventory (stdlib only)."""
import copy
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'docs/v1-coverage-source.json'

def scalar(kind, description='', example=None):
    kind = kind.lower().strip()
    if not kind: return {'description': description}
    if kind == 'int/string': return {'anyOf':[{'type':'integer'},{'type':'string'}], 'description':description}
    types = {'int':'integer','integer':'integer','float':'number','double':'number','number':'number','bool':'boolean','boolean':'boolean','array':'array','object':'object'}
    schema = {'type': types.get(kind, 'string')}
    if description: schema['description'] = description
    if kind == 'date': schema['format'] = 'date'
    # ZenTao accepts local datetime strings; do not enforce RFC3339 date-time.
    if kind == 'array':
        schema['items'] = infer(example[0]) if isinstance(example,list) and example else {}
    if kind == 'object': schema['additionalProperties'] = True
    return schema

def infer(value):
    if isinstance(value,bool): return {'type':'boolean'}
    if isinstance(value,int): return {'type':'integer'}
    if isinstance(value,float): return {'type':'number'}
    if isinstance(value,str): return {'type':'string'}
    if isinstance(value,list): return {'type':'array','items':infer(value[0]) if value else {}}
    if isinstance(value,dict): return {'type':'object','properties':{k:infer(v) for k,v in value.items()}}
    return {}

def body_schema(rows, example):
    schema={'type':'object','properties':{},'additionalProperties':True}
    parent=None
    for name,kind,required,description in rows:
        if name.startswith('∟'):
            if parent is None: raise ValueError('Nested field without parent')
            target=schema['properties'][parent]
            if target['type']=='array': target=target.setdefault('items',{})
            target.update({'type':'object'})
            target.setdefault('properties',{})
            name=name.lstrip('∟ ').strip()
        else:
            parent=name;target=schema
        target['properties'][name]=scalar(kind,description,example.get(name))
        if required=='是':target.setdefault('required',[]).append(name)
    return schema

def main():
    source=json.loads(SOURCE.read_text(encoding='utf-8'))
    result={'openapi':'3.0.3','info':{'title':'ZenTao REST v1 compatibility','version':'1.0.0','description':'97 documented REST v1 endpoints. Availability depends on ZenTao edition, version, permissions and modules. Token creation is for user-owned login, not a default model tool.'},'security':[{'Token':[]}],'components':{'securitySchemes':{'Token':{'type':'apiKey','in':'header','name':'Token'}}},'paths':{}}
    coverage=[]
    original={'GET /user':'zentao_current_user','GET /products':'zentao_products','GET /tasks/{id}':'zentao_task_detail'}
    for row in source['endpoints']:
        path=row['path'].replace('/id','/{id}')
        if path=='/productsplan/{id}':path='/productplans/{id}'
        if path=='/product/{id}':path='/products/{id}'
        method=row['method'].lower();key=row['method']+' '+path
        op={'operationId':original.get(key,'zentao_v1_'+method+'_'+path.strip('/').replace('/','_').replace('{','').replace('}','')),'summary':row['title'],'description':row['title']+'。来源：'+row['url'],'tags':[row['module']],'externalDocs':{'url':row['url']},'x-documentation-number':row['number'],'parameters':[],'responses':{'200':{'description':'ZenTao JSON response; shape varies by endpoint and version.','content':{'application/json':{'schema':{}}}}}}
        if key in original:
            op['responses']['200']['content']['application/json']['schema']={'type':'object','additionalProperties':True}
        if '{id}' in path:op['parameters'].append({'name':'id','in':'path','required':True,'schema':{'type':'integer','minimum':1}})
        # Only the first request-parameter table is request data: page 691 embeds a response table under the same heading.
        for fields in row['query'][:1]:
            for name,kind,required,desc in fields:
                op['parameters'].append({'name':name,'in':'query','required':required=='是','schema':scalar(kind,desc)})
        # Preserve the already verified adapter's optional pagination and user fields.
        extras={'GET /user':{'fields':'string','page':'integer','limit':'integer'},'GET /products':{'page':'integer','limit':'integer'}}
        for name,kind in extras.get(key,{}).items():
            if not any(p['name']==name for p in op['parameters']):op['parameters'].append({'name':name,'in':'query','required':False,'schema':{'type':kind},'description':'Verified by existing REST v1 adapter.'})
        bodies=row['body']
        if method=='get' and bodies:
            for fields in bodies:
                for name,kind,required,desc in fields:
                    op['parameters'].append({'name':name,'in':'query','required':required=='是','schema':scalar(kind,desc)})
            bodies=[]
        if row['url'].endswith('/708.html'):
            bodies=[[[n,t,'否','Official buildEntry::put accepted field'] for n,t in [('execution','int'),('product','int'),('branch','int'),('name','string'),('builder','string'),('date','date'),('scmPath','string'),('filePath','string'),('desc','string')]]]
            op['description']+=' 请求字段补自官方 api/v1/entries/build.php put()：'+source['buildEditSource']
        if bodies:
            variants=[body_schema(fields,row['examples'][min(i,len(row['examples'])-1)] if row['examples'] else {}) for i,fields in enumerate(bodies)]
            schema=variants[0] if len(variants)==1 else {'anyOf':variants}
            if len(variants)>1:op['description']+=' 日志格式：20.7之前使用id/objectID/dates/work/consumed/left/objectType；20.7及以后使用date/work/consumed/left。请选择对应版本，不自动重试写入。'
            op['requestBody']={'required':any(v.get('required') for v in variants),'content':{'application/json':{'schema':schema}}}
        if path=='/tokens':op['security']=[]
        op['x-readonly']=method=='get'
        result['paths'].setdefault(path,{})[method]=op
        coverage.append(f"| {row['number']} | {row['module']} | {row['title']} | `{row['method']} {path}` | {sum(len(b) for b in bodies)} | [official]({row['url']}) |")
    readonly=copy.deepcopy(result);readonly['info']['title']='ZenTao REST v1 read-only compatibility';readonly['paths']={p:{'get':ops['get']} for p,ops in readonly['paths'].items() if 'get' in ops}
    for name,data in [('zentao-v1.json',result),('zentao-v1-readonly.json',readonly)]:
        (ROOT/'docs'/name).write_text(json.dumps(data,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    counts={method:sum(method in ops for ops in result['paths'].values()) for method in ['get','post','put','delete']}
    report='''# REST v1 coverage\n\nGenerated by `python3 scripts/generate-v1.py` from `docs/v1-coverage-source.json`.\n\nScope: all 97 endpoint pages in the official REST v1 manual sections 2.2–2.18; not every undocumented endpoint in newer ZenTao source. Field names, documented types and required flags are recorded with per-page SHA256 and retrieval date. Array item shapes use official request examples or nested field rows; unspecified items remain unconstrained. No response-field validation is imposed because versions differ.\n\nDocumentation coverage is not live mutation acceptance. All writes require a disposable test instance and explicit fixture authorization before acceptance. Edition-specific feedback/ticket features can be unavailable. Server-side custom required fields remain authoritative.\n\nRoute review against official source: SOURCE_ROUTE\n\n- Product edit: `/product/{id}` is an official alias; normalize to `/products/{id}`.\n- Delete plan: manual `/productsplan/{id}` is a typo; source maps `/productplans/{id}`.\n- Activate bug: `/bugs/{id}/active` is correct.\n- Build edit: manual 708 omits request body; the nine optional fields come from the corresponding official `api/v1/entries/build.php` `put()` method.\n- Task log: retain both documented request formats separated at CE 20.7 (Enterprise 10.6 / Ultimate 5.6 / IPD 2.6). No mutating fallback or auto-retry.\n- Feedback list page 846 labels filters as request body despite GET; represent these as query parameters.
- Page 691 has a response table under request parameters; only its first table is used.\n- Existing `zentao_current_user`, `zentao_products`, `zentao_task_detail` operation IDs are unchanged.\n- `/tokens` exists for protocol coverage; runtime config must block it from model tools. Use user-owned interactive login.\n\nCounts: COUNTS.\n\n| Section | Module | Interface | Method and path | Body field rows | Source |\n|---|---|---|---|---:|---|\n'''.replace('SOURCE_ROUTE',source['routeSource']).replace('COUNTS',json.dumps(counts))
    (ROOT/'docs/v1-coverage.md').write_text(report+'\n'.join(coverage)+'\n',encoding='utf-8')
    print(json.dumps(counts))
if __name__=='__main__':main()
