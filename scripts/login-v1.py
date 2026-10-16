#!/usr/bin/env python3
"""Interactive REST v1 token login; credentials never enter MCP tool arguments."""
import argparse
import getpass
import json
import os
from pathlib import Path
import sys
import tempfile
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise RuntimeError('Refusing authentication redirect')


def login(base_url, account, password):
    parsed = urllib.parse.urlsplit(base_url)
    if parsed.scheme not in ('http', 'https') or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError('Use an absolute HTTP(S) API URL without embedded credentials/query/fragment')
    base_url = base_url.rstrip('/')
    if not base_url.endswith('/api.php/v1'):
        raise ValueError('URL must end with /api.php/v1')
    opener = urllib.request.build_opener(NoRedirect())
    req = urllib.request.Request(base_url + '/tokens', data=json.dumps({'account': account, 'password': password}).encode(), headers={'Content-Type': 'application/json'})
    with opener.open(req, timeout=30) as response:
        data = json.load(response)
    token = data.get('token')
    if not isinstance(token, str) or not token:
        raise RuntimeError('No token returned')
    with opener.open(urllib.request.Request(base_url + '/user', headers={'Token': token}), timeout=30) as response:
        profile = json.load(response)
    if not isinstance(profile, dict) or not isinstance(profile.get('profile'), dict):
        raise RuntimeError('Token identity verification failed')
    return {'base_url': base_url, 'account': account, 'token': token}


def save_config(path, config):
    path = Path(path).expanduser()
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix='.zentao-', dir=path.parent)
    try:
        with os.fdopen(fd, 'w', encoding='utf-8') as output:
            json.dump(config, output, ensure_ascii=False)
        os.chmod(temporary, 0o600)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary): os.unlink(temporary)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True, help='Site REST v1 base URL ending in /api.php/v1')
    parser.add_argument('--config', default=str(Path.home()/'.config/zentao-rest-v1/config.json'))
    args = parser.parse_args()
    if not sys.stdin.isatty(): raise RuntimeError('Login requires your interactive terminal')
    if args.url.startswith('http://') and input('此地址使用明文 HTTP，确认在可信网络登录？[y/N] ').lower() != 'y': return 1
    if Path(args.config).expanduser().exists() and input('配置已存在，验证成功后覆盖？[y/N] ').lower() != 'y': return 1
    account = input('禅道账号：').strip()
    password = getpass.getpass('禅道密码（隐藏）：')
    config = login(args.url, account, password)
    save_config(args.config, config)
    print('登录和身份验证成功。仅保存 Token，不保存密码。')
    return 0


if __name__ == '__main__':
    try: sys.exit(main())
    except Exception as exc:
        print('Login failed: '+type(exc).__name__+'; previous configuration preserved.', file=sys.stderr)
        sys.exit(1)
