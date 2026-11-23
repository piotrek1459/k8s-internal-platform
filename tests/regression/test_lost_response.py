"""The loss boundary must follow backend acceptance, never a pre-commit failure."""
import contextlib
import http.cookiejar
import http.server
import io
import json
from pathlib import Path
import sys
import threading
import unittest
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parents[1]/'e2e'))
from resilience import drop_accepted_response
from support import Client


class LostResponseTests(unittest.TestCase):
    def origin(self, status):
        observed = []
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass
            def do_POST(self):
                observed.append({'path': self.path, 'cookie': self.headers.get('Cookie'),
                    'key': self.headers.get('Idempotency-Key'), 'body': json.loads(self.rfile.read(int(self.headers['Content-Length'])))})
                raw=json.dumps({'applicationId':'owned-app','operationId':'durable-operation'}).encode()
                self.send_response(status);self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw)
        server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Handler)
        thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        self.addCleanup(thread.join, 5)
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        class Environment:
            url=f'http://127.0.0.1:{server.server_port}'
        return Client(Environment()), observed

    def test_client_loses_response_only_after_backend_accepted_with_original_request(self):
        client, observed=self.origin(202)
        client.cookies.set_cookie(http.cookiejar.Cookie(0,'session','controlled-cookie',None,False,
            '127.0.0.1',False,False,'/api',True,False,None,True,None,None,{}))
        output=io.StringIO()
        with contextlib.redirect_stdout(output),contextlib.redirect_stderr(output):
            accepted=drop_accepted_response(client,{'name':'owned'},'stable-key')
        self.assertEqual(accepted,{'applicationId':'owned-app','operationId':'durable-operation'})
        self.assertEqual(observed,[{'path':'/api/v1/apps','cookie':'session=controlled-cookie','key':'stable-key','body':{'name':'owned'}}])
        self.assertNotIn('controlled-cookie',output.getvalue())

    def test_rejected_request_cannot_be_reported_as_post_commit_loss(self):
        client,_=self.origin(400)
        with self.assertRaisesRegex(AssertionError,'durably accepted 202'):
            drop_accepted_response(client,{'name':'rejected'},'key')

    def test_remote_origin_is_refused_before_any_forward(self):
        class Environment:
            url='http://unrelated.example:8080'
        with self.assertRaisesRegex(ValueError,'owned loopback'):
            drop_accepted_response(Client(Environment()),{'name':'refused'},'key')
