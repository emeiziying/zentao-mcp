import importlib.util
import json
from pathlib import Path
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

spec=importlib.util.spec_from_file_location('login',Path(__file__).with_name('login-v1.py'))
login=importlib.util.module_from_spec(spec);spec.loader.exec_module(login)

class Tests(unittest.TestCase):
 def test_login_and_save(self):
  calls=[]
  class Handler(BaseHTTPRequestHandler):
   def log_message(self,*args):pass
   def do_POST(self):
    calls.append(self.path)
    payload=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
    assert payload=={'account':'fixture','password':'fixture-password'}
    self.send_response(200);self.end_headers();self.wfile.write(b'{"token":"fixture-token"}')
   def do_GET(self):
    calls.append(self.path);assert self.headers['Token']=='fixture-token'
    self.send_response(200);self.end_headers();self.wfile.write(b'{"profile":{"account":"fixture"}}')
  server=ThreadingHTTPServer(('127.0.0.1',0),Handler)
  thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
  try:
   config=login.login(f'http://127.0.0.1:{server.server_port}/api.php/v1','fixture','fixture-password')
   self.assertEqual(calls,['/api.php/v1/tokens','/api.php/v1/user'])
   with tempfile.TemporaryDirectory() as d:
    path=Path(d)/'config.json';login.save_config(path,config)
    self.assertNotIn('password',path.read_text());self.assertEqual(path.stat().st_mode & 0o777,0o600)
  finally:server.shutdown();server.server_close();thread.join()
 def test_reject_embedded_credentials(self):
  with self.assertRaises(ValueError):login.login('https://a:b@example.com/api.php/v1','x','y')

if __name__=='__main__':unittest.main()
