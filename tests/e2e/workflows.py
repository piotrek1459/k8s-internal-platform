"""Full build/install/API/browser workflows on an owned real K3s cluster."""
import base64
import contextlib
import json
import secrets
import signal
import time
import unittest
import xml.etree.ElementTree as XML

from support import Client, Environment, ROOT
from acceptance import acceptance_successful

ENVIRONMENT = None


class Workflows(unittest.TestCase):
    def setUp(self):
        self.e = ENVIRONMENT
        if self._testMethodName != "test_00_browser_bootstrap":
            # Keep API diagnostics available after a browser bootstrap failure;
            # that original browser failure remains a failing test.
            if self.e.client.get("/setup/status")["setupAvailable"]:
                code, _ = self.e.client.request("POST", "/setup", {"password": self.e.password}, {"Authorization": "Bearer " + self.e.token})
                self.assertEqual(code, 201)
            self.e.client.login()

    @contextlib.contextmanager
    def page(self, name):
        from playwright.sync_api import sync_playwright
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch()
            context = browser.new_context()
            page = context.new_page()
            page.set_default_timeout(20000)
            try:
                yield page
            except Exception:
                # Traces/cookies/network bodies are deliberately not exported.
                # A closed browser must not replace the original failure with a
                # screenshot error. Keep only redacted application/operation state.
                with contextlib.suppress(Exception):
                    page.screenshot(path=str(self.e.artifacts / (name + ".png")), full_page=True, mask=[page.locator("input, textarea, pre, [aria-label='Workload events']")])
                with contextlib.suppress(Exception):
                    applications = self.e.client.get("/apps")["items"]
                    diagnostics = []
                    for app in applications:
                        item = {key: app.get(key) for key in ("id", "name", "activeOperationId", "lastSuccessfulRevision")}
                        if item["activeOperationId"]:
                            item["operation"] = self.e.client.get("/operations/" + item["activeOperationId"])
                        diagnostics.append(item)
                    (self.e.artifacts / (name + ".json")).write_text(self.e.redact(json.dumps(diagnostics, indent=2)))
                raise
            finally:
                context.close(); browser.close()

    def browser_login(self, page):
        page.goto(self.e.url + "/login")
        page.get_by_label("Password", exact=True).fill(self.e.password)
        with page.expect_response(lambda r: r.url.endswith("/api/v1/auth/session") and r.request.method == "POST") as response:
            page.get_by_role("button", name="Sign in", exact=True).click()
        self.assertEqual(response.value.status, 204)
        page.wait_for_url("**/apps")

    def click_mutation(self, page, button, method, path):
        with page.expect_response(lambda r: r.url.endswith("/api/v1" + path) and r.request.method == method) as response:
            button.click()
        self.assertEqual(response.value.status, 202)
        return response.value.json()

    def browser_success(self, page, accepted, navigates=True):
        from playwright.sync_api import expect
        if navigates:
            page.wait_for_url("**/operation?op=" + accepted["operationId"])
        # History badges also contain "succeeded". Only the new operation's
        # state indicator can establish that the submitted mutation completed.
        state = page.get_by_text("State:", exact=True).locator("..")
        expect(state.get_by_text("succeeded", exact=True)).to_be_visible(timeout=180000)
        self.assertEqual(self.e.client.get("/operations/" + accepted["operationId"])["state"], "succeeded")

    def test_00_browser_bootstrap(self):
        with self.page("bootstrap") as page:
            page.goto(self.e.url + "/")
            page.wait_for_url("**/setup")
            page.get_by_label("Bootstrap token").fill("invalid-bootstrap-token")
            page.get_by_label("Administrator password", exact=True).fill(self.e.password)
            page.get_by_label("Confirm password").fill(self.e.password)
            with page.expect_response(lambda r: r.url.endswith("/api/v1/setup") and r.request.method == "POST") as response:
                page.get_by_role("button", name="Create account").click()
            self.assertEqual(response.value.status, 401)
            page.get_by_text(response.value.json()["error"]["message"], exact=True).wait_for()
            self.assertTrue(self.e.client.get("/setup/status")["setupAvailable"])
            page.get_by_label("Bootstrap token").fill(self.e.token)
            with page.expect_response(lambda r: r.url.endswith("/api/v1/setup") and r.request.method == "POST") as response:
                page.get_by_role("button", name="Create account").click()
            self.assertEqual(response.value.status, 201)
            page.wait_for_url("**/login")
            self.browser_login(page)
            self.assertFalse(self.e.client.get("/setup/status")["setupAvailable"])
        self.e.record("Browser rejected an invalid token, created the administrator and logged in")

    def test_01_authentication_and_csrf(self):
        anonymous = Client(self.e)
        self.assertEqual(anonymous.request("GET", "/apps")[0], 401)
        self.assertEqual(anonymous.request("POST", "/auth/session", {"password": self.e.password}, csrf=False)[0], 403)
        self.assertEqual(anonymous.request("POST", "/auth/session", {"password": "incorrect-password"})[0], 401)
        anonymous.login()
        self.assertIn("administratorId", anonymous.get("/auth/me"))
        self.assertEqual(anonymous.request("DELETE", "/auth/session")[0], 204)
        self.assertEqual(anonymous.request("GET", "/auth/me")[0], 401)
        token = self.e.run(ROOT / "scripts/platform.sh", "token", private=True, check=False)
        self.assertNotEqual(token.returncode, 0); self.assertEqual(token.stdout, "")
        self.e.record("Authentication, CSRF, logout and consumed-token rejection")

    def test_02_invalid_requests_do_not_create_workloads(self):
        body = {"name": "bad-name-" + secrets.token_hex(3), "image": self.e.image("v1"), "containerPort": 8080, "profile": "http", "exposure": {"mode": "internal"}}
        before = len(self.e.client.get("/apps")["items"])
        self.assertEqual(self.e.client.request("POST", "/apps", body)[0], 400)
        for override, expected in (({"name": "INVALID"}, 400), ({"containerPort": 0}, 400), ({"storage": {"size": "1Gi"}}, 422), ({"exposure": {"mode": "public"}}, 422)):
            code, response = self.e.client.request("POST", "/apps", {**body, **override}, {"Idempotency-Key": secrets.token_hex(16)})
            self.assertEqual(code, expected); self.assertIn("code", response["error"])
        self.assertEqual(len(self.e.client.get("/apps")["items"]), before)
        self.e.record("Unsupported and malformed API requests leave application count unchanged")

    def test_03_workload_management_boundaries(self):
        import urllib.request
        from resilience import fixture_images
        image = fixture_images(self.e)['v1']
        ca = json.loads(self.e.kubectl('-n','platform-apps','get','configmap','kube-root-ca.crt','-o','json').stdout)['data']['ca.crt']
        app = self.e.create('boundary', image=image, environment={
            'FIXTURE_START_READY':'true', 'FIXTURE_PLATFORM_URL':'http://platform.platform-system.svc.cluster.local',
            'FIXTURE_KUBE_URL':'https://kubernetes.default.svc', 'FIXTURE_KUBE_CA':ca})
        try:
            pod = self.e.wait_replicas(app,1)[0]
            self.assertFalse(pod['spec']['automountServiceAccountToken'])
            self.assertEqual(pod['spec']['serviceAccountName'], 'default')
            self.assertFalse(any(source.get('serviceAccountToken') for volume in pod['spec'].get('volumes',[])
                for source in volume.get('projected',{}).get('sources',[])))
            before = len(self.e.client.get('/apps')['items'])
            secrets_before = len(self.e.client.get('/secrets')['items'])
            with self.e.forward('forward',app,port=self.e.app_port):
                request=urllib.request.Request(f'http://127.0.0.1:{self.e.app_port}/probe',method='POST',data=b'')
                with urllib.request.urlopen(request,timeout=35) as response:
                    result=json.load(response)
            self.assertTrue(result['serviceAccountTokenAbsent'])
            checks={item['name']:item for item in result['requests']}
            self.assertEqual(len(checks),6)
            self.assertTrue(all(not item['transportError'] for item in checks.values()), 'A network/TLS failure cannot establish an authorization boundary')
            self.assertEqual(checks['platform-health']['status'],200)
            for name in ('platform-read','platform-create','platform-secret'):
                self.assertEqual(checks[name]['status'],401)
            for name in ('kube-read','kube-create'):
                self.assertIn(checks[name]['status'],(401,403))
                self.assertEqual(checks[name]['kind'],'Status')
                self.assertIn(checks[name]['reason'],('Unauthorized','Forbidden'))
            self.assertEqual(len(self.e.client.get('/apps')['items']),before)
            self.assertEqual(len(self.e.client.get('/secrets')['items']),secrets_before)
            namespaces=json.loads(self.e.kubectl('get','namespaces','-o','json').stdout)['items']
            self.assertNotIn('workload-boundary-attempt',[item['metadata']['name'] for item in namespaces])
            self.e.http_version(app,'fixture-v1')
        finally:
            self.e.delete(app)
        self.e.record('Actual managed workload reached platform health, received protected API refusals and TLS-verified Kubernetes administration refusals; no mounted service-account token or unintended resources',
            checks=result)

    def test_05_api_lifecycle_and_idempotency(self):
        body = {"name": "api-" + secrets.token_hex(4), "image": self.e.image("v1"), "containerPort": 8080, "profile": "http", "replicas": 1, "health": {"readinessPath": "/readyz"}, "exposure": {"mode": "internal"}}
        key = secrets.token_hex(16)
        accepted, _ = self.e.client.mutate("POST", "/apps", body, key)
        app = accepted["applicationId"]
        try:
            first = self.e.client.get("/apps/" + app)
            revision = first["lastSuccessfulRevision"]
            old_uid = self.e.wait_replicas(app, 1)[0]["metadata"]["uid"]
            self.e.http_version(app, "v1")
            code, replay = self.e.client.request("POST", "/apps", body, {"Idempotency-Key": key})
            self.assertEqual(code, 202); self.assertEqual(replay["operationId"], accepted["operationId"])
            self.assertEqual(len(self.e.client.get("/apps/" + app + "/revisions")["items"]), 1)
            self.assertEqual(self.e.client.request("POST", "/apps", {**body, "replicas": 2}, {"Idempotency-Key": key})[0], 409)
            self.e.client.mutate("PATCH", "/apps/" + app, {"version": first["version"], "image": self.e.image("v2")})
            self.e.http_version(app, "v2")
            self.assertNotEqual(self.e.wait_replicas(app, 1)[0]["metadata"]["uid"], old_uid)
            code, _ = self.e.client.request("PATCH", "/apps/" + app, {"version": first["version"], "replicas": 2}, {"Idempotency-Key": secrets.token_hex(16)})
            self.assertEqual(code, 412)
            for replicas in (2, 0, 1):
                current = self.e.client.get("/apps/" + app)
                self.e.client.mutate("PATCH", "/apps/" + app, {"version": current["version"], "replicas": replicas})
                self.e.wait_replicas(app, replicas)
                frozen = self.e.client.get("/apps/" + app + "/revisions")["items"][0]["frozenImage"]
                self.assertEqual(frozen, self.e.image("v2"))
            current = self.e.client.get("/apps/" + app)
            self.e.client.mutate("POST", "/apps/" + app + "/rollback", {"version": current["version"], "revisionId": revision})
            self.e.http_version(app, "v1")
            self.e.record("API deploy/update/scale 2-0-1/rollback with immutable images and idempotent retries")
        finally:
            self.e.delete(app)
        self.assertNotIn(app, [a["id"] for a in self.e.client.get("/apps")["items"]])
        self.assertGreaterEqual(len(self.e.client.get("/apps/" + app + "/revisions")["items"]), 6)
        self.e.record("Deletion removed workload and Service while retaining revision history")

    def test_07_lost_response_and_moved_tag_retries(self):
        from resilience import cleanup_fixture, control_secret, drop_accepted_response, fixture_images, move_tag, release_gate, release_identity, wait_pending
        fixture_images(self.e)
        image = move_tag(self.e, 'v1')
        code, original = self.e.client.request('POST', '/images/inspect', {'image': image})
        self.assertEqual(code, 200)
        token, secret = control_secret(self.e)
        key = secrets.token_hex(16)
        body = {'name': 'lost-response-'+secrets.token_hex(4), 'image': image,
            'containerPort': 8080, 'profile': 'http', 'health': {'readinessPath': '/readyz'},
            'secretRefs': [{'environmentVariable': 'FIXTURE_CONTROL_TOKEN', 'secretId': secret['id'], 'version': 1}],
            'exposure': {'mode': 'internal'}}
        before = len(self.e.client.get('/apps')['items'])
        accepted = drop_accepted_response(self.e.client, body, key)
        app = accepted['applicationId']; cleaned = False
        try:
            frozen = wait_pending(self.e, app)
            self.assertTrue(frozen['frozenImage'].endswith('@'+original['digest']))
            identity = release_identity(self.e, app)
            revision = self.e.client.get('/apps/'+app)['desiredRevision']
            move_tag(self.e, 'v2')
            code, moved = self.e.client.request('POST', '/images/inspect', {'image': image})
            self.assertEqual(code, 200); self.assertNotEqual(moved['digest'], original['digest'])
            code, replay = self.e.client.request('POST', '/apps', body, {'Idempotency-Key': key})
            self.assertEqual(code, 202); self.assertTrue(replay['deduplicated'])
            self.assertEqual(replay['operationId'], accepted['operationId'])
            self.assertEqual(replay['applicationId'], app)
            self.assertEqual(len(self.e.client.get('/apps')['items']), before+1)
            self.assertEqual(self.e.client.get('/apps/'+app)['desiredRevision'], revision)
            self.assertEqual(self.e.client.get('/apps/'+app+'/revisions')['items'], [frozen])
            self.assertEqual(release_identity(self.e, app), identity)
            release_gate(self.e, app, token)
            self.e.client.poll(accepted['operationId'])
            self.e.http_version(app, 'fixture-v1')
            pod = self.e.wait_replicas(app, 1)[0]
            self.assertEqual(pod['spec']['containers'][0]['image'], frozen['frozenImage'])
            self.e.restart_broker()
            code, replay = self.e.client.request('POST', '/apps', body, {'Idempotency-Key': key})
            self.assertEqual(code, 202); self.assertEqual(replay['operationId'], accepted['operationId'])
            self.assertEqual(replay['applicationId'], app)
            self.assertEqual(self.e.client.get('/apps/'+app+'/revisions')['items'][0]['frozenImage'], frozen['frozenImage'])
            current = self.e.client.get('/apps/'+app)
            self.e.client.mutate('POST', '/apps/'+app+'/reapply', {'version': current['version']})
            self.e.http_version(app, 'fixture-v1')
            self.assertEqual(release_identity(self.e, app), identity)
            cleanup_fixture(self.e, app, secret); cleaned = True
        finally:
            if not cleaned:
                # Preserve the original failure; owned-cluster teardown removes
                # unresolved fixtures without rewriting SQL or Helm evidence.
                with contextlib.suppress(Exception):
                    current = self.e.client.get('/apps/'+app)
                    self.e.client.request('DELETE', '/apps/'+app, {'version':current['version']}, {'Idempotency-Key':secrets.token_hex(16)})
        self.e.record('Lost 202 after backend acceptance; real registry tag moved; retries retained operation/app/revision/digest and chart/values identity across broker replacement and explicit Reapply',
            originalImage=frozen['frozenImage'], movedDigest=moved['digest'], identity=identity)

    def test_10_browser_application_lifecycle(self):
        from playwright.sync_api import expect
        value = secrets.token_urlsafe(24); self.e.secret_values.append(value)
        newer = 'newer-'+value; self.e.secret_values.append(newer)
        secret_name = 'browser-secret-'+secrets.token_hex(3)
        with self.page("browser-lifecycle") as page:
            self.browser_login(page)
            page.on("dialog", lambda dialog: dialog.accept())
            page.goto(self.e.url + '/secrets')
            page.get_by_placeholder('Name', exact=True).fill(secret_name)
            page.get_by_placeholder('Value (write-only)', exact=True).fill(value)
            with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets') and r.request.method == 'POST') as response:
                page.get_by_role('button', name='Create', exact=True).click()
            self.assertEqual(response.value.status, 201)
            secret = response.value.json()
            self.assertTrue(value not in json.dumps(secret), 'Secret creation returned sensitive data')
            expect(page.get_by_placeholder('Value (write-only)', exact=True)).to_have_value('')
            row = page.get_by_text(secret_name, exact=True).locator('../..')
            row.get_by_role('button', name='Add version', exact=True).click()
            page.get_by_placeholder('New value (write-only)', exact=True).fill(newer)
            with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets/'+secret['id']+'/versions') and r.request.method == 'POST') as response:
                page.get_by_role('button', name='Add', exact=True).click()
            self.assertEqual(response.value.status, 201)
            self.assertEqual(response.value.json()['version'], 2)
            page.get_by_text('Versions: 1, 2', exact=True).wait_for()
            self.assertTrue(value not in page.locator('body').inner_text() and newer not in page.locator('body').inner_text(), 'Secret management rendered a sensitive value')
            page.goto(self.e.url + "/apps/new")
            page.get_by_label("Image reference").fill(self.e.image("v1"))
            page.get_by_role("button", name="Inspect image", exact=True).click()
            page.get_by_role("button", name="Next").click()
            name = "browser-" + secrets.token_hex(4)
            page.get_by_label("Name", exact=True).fill(name)
            page.get_by_label("Readiness path").fill("/readyz")
            for _ in range(2): page.get_by_role("button", name="Next").click()
            page.get_by_label("Secret environment variable").fill("BROWSER_SECRET")
            page.get_by_label("Secret", exact=True).select_option(secret["id"])
            page.get_by_label("Secret version", exact=True).select_option("2")
            page.get_by_role("button", name="Next").click()
            accepted = self.click_mutation(page, page.get_by_role("button", name="Deploy", exact=True), "POST", "/apps")
            app = accepted["applicationId"]
            try:
                self.browser_success(page, accepted, navigates=False)
                deployment = self.e.resource("deployments", app)[0]
                env = deployment["spec"]["template"]["spec"]["containers"][0]["env"]
                self.assertIn("BROWSER_SECRET", [item["name"] for item in env if "secretKeyRef" in item.get("valueFrom",{})])
                reference = next(item['valueFrom']['secretKeyRef'] for item in env if item['name'] == 'BROWSER_SECRET')
                self.assertTrue(reference['name'].endswith('-v2'))
                stored = json.loads(self.e.kubectl('-n', 'platform-apps', 'get', 'secret', reference['name'], '-o', 'json').stdout)
                self.assertTrue(base64.b64decode(stored['data']['value']).decode() == newer, 'Browser selected the wrong immutable Secret version')
                self.e.http_version(app, "v1")
                page.goto(self.e.url+'/secrets')
                row = page.get_by_text(secret_name, exact=True).locator('../..')
                with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets/'+secret['id']) and r.request.method == 'DELETE') as response:
                    row.get_by_role('button', name='Delete', exact=True).click()
                self.assertEqual(response.value.status, 409)
                page.get_by_text(response.value.json()['error']['message'], exact=True).wait_for()
                page.goto(self.e.url + "/apps/" + app + "/update")
                page.get_by_label("Image reference").fill(self.e.image("v2"))
                accepted = self.click_mutation(page, page.get_by_role("button", name="Apply update"), "PATCH", "/apps/" + app)
                self.browser_success(page, accepted)
                self.e.http_version(app, "v2")
                page.reload()
                self.browser_success(page, accepted)
                page.goto(self.e.url + "/apps/" + app)
                page.get_by_role("button", name="History", exact=True).click()
                row = page.get_by_role("row").filter(has_text=self.e.image("v1"))
                accepted = self.click_mutation(page, row.get_by_role("button", name="Roll back"), "POST", "/apps/" + app + "/rollback")
                self.browser_success(page, accepted)
                self.e.http_version(app, "v1")
                page.goto(self.e.url + "/apps/" + app)
                accepted = self.click_mutation(page, page.get_by_role("button", name="Delete", exact=True), "DELETE", "/apps/" + app)
                self.browser_success(page, accepted)
                self.assertNotIn(app, [a["id"] for a in self.e.client.get("/apps")["items"]])
                self.assertEqual(self.e.resource("deployments,services", app), [])
                page.goto(self.e.url+'/secrets')
                row = page.get_by_text(secret_name, exact=True).locator('../..')
                with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets/'+secret['id']) and r.request.method == 'DELETE') as response:
                    row.get_by_role('button', name='Delete', exact=True).click()
                self.assertEqual(response.value.status, 204)
                expect(page.get_by_text(secret_name, exact=True)).to_have_count(0)
                versions = json.loads(self.e.kubectl('-n', 'platform-apps', 'get', 'secrets', '-l', 'platform.local/secret-id='+secret['id'].replace('_','-'), '-o', 'json').stdout)['items']
                self.assertEqual(versions, [], 'Successful Secret deletion left Kubernetes values behind')
                page.screenshot(path=str(self.e.artifacts / "browser-complete.png"), mask=[page.locator("input, textarea")])
            finally:
                current = next((a for a in self.e.client.get("/apps")["items"] if a["id"] == app), None)
                if current and not current.get("activeOperationId"): self.e.delete(app)
        self.e.record("Browser Secret creation/version selection, in-use deletion refusal and physical version cleanup; image inspection, deployment, update, progress refresh, history rollback and deletion")

    def test_12_browser_running_operation_resume(self):
        from playwright.sync_api import expect
        from resilience import cleanup_fixture, control_secret, fixture_images, release_gate, wait_pending
        image=fixture_images(self.e)['v1'];token,secret=control_secret(self.e)
        app=None;cleaned=False
        try:
            with self.page('running-operation-resume') as page:
                self.browser_login(page)
                submissions=[]
                page.on('request',lambda request:submissions.append(request.url) if request.method=='POST' and request.url.endswith('/api/v1/apps') else None)
                page.goto(self.e.url+'/apps/new')
                page.get_by_label('Image reference').fill(image)
                page.get_by_role('button',name='Inspect image',exact=True).click()
                page.get_by_role('button',name='Next').click()
                page.get_by_label('Name',exact=True).fill('running-resume-'+secrets.token_hex(4))
                page.get_by_label('Readiness path').fill('/readyz')
                for _ in range(2):page.get_by_role('button',name='Next').click()
                page.get_by_label('Secret environment variable').fill('FIXTURE_CONTROL_TOKEN')
                page.get_by_label('Secret',exact=True).select_option(secret['id'])
                page.get_by_label('Secret version',exact=True).select_option('1')
                page.get_by_role('button',name='Next').click()
                accepted=self.click_mutation(page,page.get_by_role('button',name='Deploy',exact=True),'POST','/apps')
                app=accepted['applicationId'];frozen=wait_pending(self.e,app)
                operation=accepted['operationId']
                url=self.e.url+'/apps/'+app+'/operation?op='+operation
                page.goto(url)
                state=page.get_by_text('State:',exact=True).locator('..')
                expect(state.get_by_text('running',exact=True)).to_be_visible()
                page.reload();expect(state.get_by_text('running',exact=True)).to_be_visible()
                page.goto(self.e.url+'/apps');page.goto(url)
                expect(state.get_by_text('running',exact=True)).to_be_visible()
                self.assertEqual(submissions,[self.e.url+'/api/v1/apps'])
                self.assertEqual(self.e.client.get('/apps/'+app+'/revisions')['items'],[frozen])
                release_gate(self.e,app,token)
                self.browser_success(page,accepted,navigates=False)
                page.reload();self.browser_success(page,accepted,navigates=False)
                self.assertEqual(len(submissions),1)
                self.assertEqual(len(self.e.client.get('/apps/'+app+'/revisions')['items']),1)
                self.e.http_version(app,'fixture-v1')
            cleanup_fixture(self.e,app,secret);cleaned=True
        finally:
            if app and not cleaned:
                with contextlib.suppress(Exception):
                    current=self.e.client.get('/apps/'+app)
                    self.e.client.request('DELETE','/apps/'+app,{'version':current['version']},{'Idempotency-Key':secrets.token_hex(16)})
        self.e.record('Browser reload/navigation during an observed running Helm operation resumed persisted progress without resubmission; explicit readiness release completed the same operation/revision',
            submissions=len(submissions), operationId=operation, revisionId=frozen['id'])

    def test_15_private_registry_inspection_and_pull(self):
        from private_registry import authenticated_registry
        from playwright.sync_api import expect
        with authenticated_registry(self.e) as private:
            self.assertEqual(self.e.client.request("POST", "/images/inspect", {"image":private["image"]})[0],503)
            with self.page('private-registry-browser') as page:
                self.browser_login(page)
                page.on('dialog', lambda dialog: dialog.accept())

                def add_credential(label, password):
                    page.goto(self.e.url+'/credentials')
                    page.get_by_role('button', name='Add credential', exact=True).click()
                    page.get_by_label('Label', exact=True).fill(label)
                    page.get_by_label('Registry hostname', exact=True).fill(private['registry'])
                    page.get_by_label('Username', exact=True).fill(private['username'])
                    page.get_by_label('Password / token', exact=True).fill(password)
                    with page.expect_response(lambda r:r.url.endswith('/api/v1/registry-credentials') and r.request.method == 'POST') as response:
                        page.locator('form').get_by_role('button', name='Add credential', exact=True).click()
                    self.assertEqual(response.value.status, 201)
                    metadata = response.value.json()
                    page.get_by_text(label, exact=True).wait_for()
                    expect(page.get_by_label('Password / token', exact=True)).to_have_count(0)
                    self.assertTrue(password not in json.dumps(metadata) and password not in page.locator('body').inner_text(), 'Credential management disclosed the submitted password')
                    return metadata

                credential = add_credential('private-browser-'+secrets.token_hex(3), private['password'])
                bad = add_credential('invalid-browser-'+secrets.token_hex(3), 'incorrect-password')
                page.goto(self.e.url+'/apps/new')
                page.get_by_label('Image reference', exact=True).fill(private['image'])
                page.get_by_label('Registry credential (optional)', exact=True).select_option(bad['id'])
                with page.expect_response(lambda r:r.url.endswith('/api/v1/images/inspect') and r.request.method == 'POST') as response:
                    page.get_by_role('button', name='Inspect image', exact=True).click()
                self.assertEqual(response.value.status, 503)
                page.get_by_text(response.value.json()['error']['message'], exact=True).wait_for()
                expect(page.get_by_role('button', name='Next')).to_have_count(0)
                page.get_by_label('Registry credential (optional)', exact=True).select_option(credential['id'])
                with page.expect_response(lambda r:r.url.endswith('/api/v1/images/inspect') and r.request.method == 'POST') as response:
                    page.get_by_role('button', name='Inspect image', exact=True).click()
                self.assertEqual(response.value.status, 200)
                self.assertEqual(response.value.request.post_data_json['registryCredentialId'], credential['id'])
                page.get_by_role('button', name='Next').click()
                page.get_by_label('Name', exact=True).fill('private-browser-'+secrets.token_hex(3))
                page.get_by_label('Readiness path', exact=True).fill('/readyz')
                for _ in range(3):
                    page.get_by_role('button', name='Next').click()
                with page.expect_response(lambda r:r.url.endswith('/api/v1/apps') and r.request.method == 'POST') as response:
                    page.get_by_role('button', name='Deploy', exact=True).click()
                self.assertEqual(response.value.status, 202)
                self.assertEqual(response.value.request.post_data_json['registryCredentialId'], credential['id'])
                accepted = response.value.json(); app = accepted['applicationId']
                try:
                    self.browser_success(page, accepted, navigates=False)
                    self.e.http_version(app,'v1')
                    deployment=self.e.resource('deployments',app)[0]
                    self.assertTrue(deployment['spec']['template']['spec']['imagePullSecrets'])
                    page.goto(self.e.url+'/credentials')
                    row=page.get_by_text(credential['label'], exact=True).locator('../..')
                    with page.expect_response(lambda r:r.url.endswith('/api/v1/registry-credentials/'+credential['id']) and r.request.method == 'DELETE') as response:
                        row.get_by_role('button', name='Remove', exact=True).click()
                    self.assertEqual(response.value.status, 409)
                    page.get_by_text(response.value.json()['error']['message'], exact=True).wait_for()
                finally:
                    self.e.delete(app)
                for item in (bad, credential):
                    page.goto(self.e.url+'/credentials')
                    row=page.get_by_text(item['label'], exact=True).locator('../..')
                    with page.expect_response(lambda r, identity=item['id']:r.url.endswith('/api/v1/registry-credentials/'+identity) and r.request.method == 'DELETE') as response:
                        row.get_by_role('button', name='Remove', exact=True).click()
                    self.assertEqual(response.value.status, 204)
                    expect(page.get_by_text(item['label'], exact=True)).to_have_count(0)
                    missing=self.e.kubectl('-n','platform-apps','get','secret','registry-'+item['id'].replace('_','-'),check=False)
                    self.assertNotEqual(missing.returncode,0)
        self.e.record('Browser credential creation/selection, invalid-password errors, private image inspection and real kubelet pull; in-use refusal and physical credential cleanup')

    def test_20_configuration_secrets_and_snapshot_rollback(self):
        value = secrets.token_urlsafe(24); self.e.secret_values.append(value)
        code, secret = self.e.client.request("POST", "/secrets", {"name": "secret-" + secrets.token_hex(4), "data": {"value": value}})
        self.assertEqual(code, 201); self.assertNotIn(value, json.dumps(secret))
        reference = {"environmentVariable": "APP_SECRET", "secretId": secret["id"], "version": 1}
        app = self.e.create("config", secretRefs=[reference], environment={"APP_MODE": "first"})
        try:
            first = self.e.client.get("/apps/" + app)
            code, newer = self.e.client.request("POST", "/secrets/" + secret["id"] + "/versions", {"data": {"value": "second-" + value}})
            self.e.secret_values.append("second-" + value)
            self.assertEqual(code, 201); self.assertEqual(newer["version"], 2)
            self.e.client.mutate("PATCH", "/apps/" + app, {"version": first["version"], "secretRefs": [{**reference, "version": 2}], "environment": {"APP_MODE": "second"}})
            current = self.e.client.get("/apps/" + app)
            self.e.client.mutate("POST", "/apps/" + app + "/rollback", {"version": current["version"], "revisionId": first["lastSuccessfulRevision"]})
            pod = self.e.wait_replicas(app, 1)[0]
            variables = {item["name"]: item for item in pod["spec"]["containers"][0]["env"]}
            self.assertEqual(variables["APP_MODE"]["value"], "first")
            secret_name = variables["APP_SECRET"]["valueFrom"]["secretKeyRef"]["name"]
            self.assertTrue(secret_name.endswith("-v1"))
            actual = json.loads(self.e.kubectl("-n", "platform-apps", "get", "secret", secret_name, "-o", "json", private=True).stdout)
            self.assertTrue(actual["immutable"])
            self.assertTrue(base64.b64decode(actual["data"]["value"]).decode() == value, "Rollback selected incorrect Secret contents")
            self.assertEqual(self.e.client.request("DELETE", "/secrets/" + secret["id"])[0], 409)
            self.assertNotIn(value, json.dumps(self.e.client.get("/secrets")))
        finally: self.e.delete(app)
        self.e.record("Immutable Secret versions, write-only responses, deletion guard and configuration rollback")

    def test_25_secret_deletion_failure_and_retry(self):
        from playwright.sync_api import expect
        value = secrets.token_urlsafe(24); self.e.secret_values.append(value)
        name = 'retry-secret-'+secrets.token_hex(3)
        code, secret = self.e.client.request('POST', '/secrets', {'name': name, 'data': {'value': value}})
        self.assertEqual(code, 201)
        self.assertEqual(self.e.client.request('POST', '/secrets/'+secret['id']+'/versions', {'data': {'value': value}})[0], 201)
        prefix = 'app-secret-'+secret['id'].replace('_','-')
        second = json.loads(self.e.kubectl('-n','platform-apps','get','secret',prefix+'-v2','-o','json').stdout)
        self.e.kubectl('-n','platform-apps','label','secret',prefix+'-v2','app.kubernetes.io/managed-by=foreign-owner','--overwrite')
        self.assertEqual(self.e.client.request('DELETE', '/secrets/'+secret['id'])[0], 409)
        self.assertNotEqual(self.e.kubectl('-n','platform-apps','get','secret',prefix+'-v1',check=False).returncode, 0)
        remaining = json.loads(self.e.kubectl('-n','platform-apps','get','secret',prefix+'-v2','-o','json').stdout)
        self.assertEqual(remaining['metadata']['uid'], second['metadata']['uid'])
        self.assertEqual(remaining['metadata']['labels']['app.kubernetes.io/managed-by'], 'foreign-owner')
        metadata = next(item for item in self.e.client.get('/secrets')['items'] if item['id'] == secret['id'])
        self.assertTrue(metadata['deleting']); self.assertEqual(len(metadata['versions']), 2)
        self.assertEqual(self.e.client.request('POST', '/secrets/'+secret['id']+'/versions', {'data': {'value': value}})[0], 409)
        before = len(self.e.client.get('/apps')['items'])
        code, _ = self.e.client.request('POST','/apps',{'name':'unavailable-secret-'+secrets.token_hex(3),
            'image':self.e.image('v1'),'containerPort':8080,'profile':'http','exposure':{'mode':'internal'},
            'secretRefs':[{'environmentVariable':'APP_SECRET','secretId':secret['id'],'version':2}]},
            {'Idempotency-Key':secrets.token_hex(16)})
        self.assertEqual(code, 400)
        self.assertEqual(len(self.e.client.get('/apps')['items']), before)
        self.e.restart_broker()
        self.assertIn('administratorId', self.e.client.get('/auth/me'))
        metadata = next(item for item in self.e.client.get('/secrets')['items'] if item['id'] == secret['id'])
        self.assertTrue(metadata['deleting'])
        with self.page('secret-deletion-retry') as page:
            self.browser_login(page); page.goto(self.e.url+'/secrets')
            row = page.get_by_text(name, exact=True).locator('../..')
            expect(row.get_by_role('button', name='Add version', exact=True)).to_be_disabled()
            page.get_by_text('Deletion is pending. Retry deletion after resolving the reported error.', exact=True).wait_for()
            # Restore only this controlled fixture's ownership, under the current
            # UID/resource version. The platform itself must never adopt it.
            patch=[{'op':'test','path':'/metadata/uid','value':remaining['metadata']['uid']},
                {'op':'test','path':'/metadata/resourceVersion','value':remaining['metadata']['resourceVersion']},
                {'op':'replace','path':'/metadata/labels/app.kubernetes.io~1managed-by','value':'k8s-internal-platform'}]
            self.e.kubectl('-n','platform-apps','patch','secret',prefix+'-v2','--type=json','-p',json.dumps(patch))
            page.on('dialog', lambda dialog:dialog.accept())
            with page.expect_response(lambda r:r.url.endswith('/api/v1/secrets/'+secret['id']) and r.request.method=='DELETE') as response:
                row.get_by_role('button', name='Retry deletion', exact=True).click()
            self.assertEqual(response.value.status, 204)
            expect(page.get_by_text(name, exact=True)).to_have_count(0)
        self.assertNotEqual(self.e.kubectl('-n','platform-apps','get','secret',prefix+'-v2',check=False).returncode, 0)
        self.e.record('Partial Secret cleanup refused foreign ownership; durable reservation survived broker replacement, rejected new references/versions and completed through guarded browser retry')

    def test_30_missing_registry_image_is_rejected(self):
        image = self.e.image("v1").split("/platform-demo@")[0] + "/missing-image:no-such-tag"
        code, body = self.e.client.request("POST", "/images/inspect", {"image": image})
        self.assertIn(code, (422, 503)); self.assertIn("code", body["error"])
        self.e.record("Image inspection reports unavailable/nonexistent registry content")

    def test_40_reapply_after_service_deletion(self):
        app = self.e.create("reapply")
        try:
            self.e.kubectl("-n", "platform-apps", "delete", "service", "-l", "platform.local/app-id=" + app)
            current = self.e.client.get("/apps/" + app)
            self.e.client.mutate("POST", "/apps/" + app + "/reapply", {"version": current["version"]})
            self.e.http_version(app, "v1")
        finally: self.e.delete(app)
        self.e.record("Explicit reapply restored an owned Service from recorded state")

    def test_45_application_diagnostics_and_drift(self):
        import datetime
        import tempfile
        from pathlib import Path
        import urllib.error
        import urllib.parse
        import urllib.request

        value = 'diagnostic-private-'+secrets.token_hex(16)
        encoded = base64.b64encode(value.encode()).decode()
        self.e.secret_values.append(value)
        code, secret = self.e.client.request('POST', '/secrets', {'name': 'diagnostic-'+secrets.token_hex(4), 'data': {'value': value}})
        self.assertEqual(code, 201)
        app = self.e.create('diagnostic', environment={'MODE': 'stable'}, secretRefs=[{'environmentVariable':'APP_SECRET','secretId':secret['id'],'version':1}])
        other = self.e.create('other-diagnostic')
        event_names = []
        copied_pod = 'copied-labels-'+secrets.token_hex(4)
        try:
            pod = self.e.wait_replicas(app, 1)[0]
            foreign = self.e.wait_replicas(other, 1)[0]
            with self.e.forward('forward', app, port=self.e.app_port):
                try:
                    urllib.request.urlopen(f'http://127.0.0.1:{self.e.app_port}/diagnostic/'+urllib.parse.quote(value+'/'+encoded, safe=''), timeout=10)
                except urllib.error.HTTPError as error:
                    self.assertEqual(error.code, 404); error.close()
            raw_logs = self.e.kubectl('-n', 'platform-apps', 'logs', pod['metadata']['name'], '-c', 'application', '--tail=100').stdout
            self.assertTrue(value in raw_logs and encoded in raw_logs, 'Controlled fixture was not present in actual workload logs')
            logs = self.e.client.get('/apps/'+app+'/logs')
            self.assertNotIn(value, logs['text']); self.assertNotIn(encoded, logs['text'])
            self.assertIn('[REDACTED]', logs['text']); self.assertLessEqual(len(logs['text'].encode()), 65536)
            self.assertEqual(self.e.client.request('GET', '/apps/'+app+'/logs?pod='+foreign['metadata']['name'])[0], 404)
            self.assertEqual(self.e.client.request('GET', '/apps/'+app+'/logs?namespace=kube-system')[0], 400)
            self.assertEqual(self.e.client.request('GET', '/apps/'+app+'/logs?tailLines=501')[0], 400)
            self.assertEqual(Client(self.e).request('GET', '/apps/'+app+'/logs')[0], 401)

            payload = '<img src=x onerror="window.__diagnosticXss=1">'
            now = datetime.datetime.now(datetime.timezone.utc).isoformat().replace('+00:00', 'Z')
            # Raw fixtures live outside uploaded artifacts and are deleted with
            # their temporary directory. Kubernetes reads remain private.
            with tempfile.TemporaryDirectory() as directory:
                for fixture_pod, message, reason in ((pod, payload+' '+value+' '+encoded, 'DiagnosticFixture'), (foreign, 'foreign-owned-event', 'ForeignFixture')):
                    name = 'diagnostic-'+secrets.token_hex(4); event_names.append(name)
                    fixture = {'apiVersion':'v1','kind':'Event','metadata':{'name':name,'namespace':'platform-apps'},'involvedObject':{'apiVersion':'v1','kind':'Pod','namespace':'platform-apps','name':fixture_pod['metadata']['name'],'uid':fixture_pod['metadata']['uid']},'type':'Warning','reason':reason,'message':message,'firstTimestamp':now,'lastTimestamp':now,'count':1}
                    path = Path(directory)/'event.json'; path.write_text(json.dumps(fixture))
                    self.e.kubectl('create', '-f', path)
                fake = {'apiVersion':'v1','kind':'Pod','metadata':{'name':copied_pod,'namespace':'platform-apps','labels':{'platform.local/app-id':app}},'spec':{'automountServiceAccountToken':False,'securityContext':{'runAsNonRoot':True,'seccompProfile':{'type':'RuntimeDefault'}},'containers':[{'name':'application','image':self.e.image('v1'),'resources':pod['spec']['containers'][0]['resources'],'securityContext':{'allowPrivilegeEscalation':False,'readOnlyRootFilesystem':True,'capabilities':{'drop':['ALL']}}}]}}
                path = Path(directory)/'pod.json'; path.write_text(json.dumps(fake)); self.e.kubectl('create', '-f', path)
            self.assertEqual(self.e.client.request('GET', '/apps/'+app+'/logs?pod='+copied_pod)[0], 404)
            events = self.e.client.get('/apps/'+app+'/events')
            body = json.dumps(events)
            self.assertNotIn(value, body); self.assertNotIn(encoded, body); self.assertNotIn('foreign-owned-event', body)
            self.assertIn('DiagnosticFixture', body)
            self.assertEqual(self.e.client.get('/apps/'+app+'/drift')['state'], 'in_sync')

            with self.page('application-diagnostics') as page:
                self.browser_login(page); page.goto(self.e.url+'/apps/'+app)
                page.get_by_role('button', name='Diagnostics', exact=True).click()
                page.get_by_role('button', name='Read logs', exact=True).click()
                page.locator('pre').wait_for()
                self.assertNotIn(value, page.locator('pre').inner_text())
                page.get_by_role('button', name='Read events', exact=True).click()
                page.get_by_text('DiagnosticFixture', exact=False).wait_for()
                section = page.get_by_role('region', name='Workload events')
                self.assertIn(payload, section.inner_text()); self.assertEqual(section.locator('img').count(), 0)
                self.assertIsNone(page.evaluate('window.__diagnosticXss'))
                page.get_by_role('button', name='Inspect drift', exact=True).click()
                page.get_by_text('No owned configuration differences detected.', exact=True).wait_for()

            deployment = self.e.resource('deployments', app)[0]
            service = self.e.resource('services', app)[0]
            changed_image = self.e.image('v2')
            patch = {'spec':{'replicas':2,'template':{'spec':{'containers':[{'name':'application','image':changed_image,'env':[{'name':'MODE','value':'operator-change'}],'resources':{'limits':{'memory':'256Mi'}},'readinessProbe':{'httpGet':{'path':'/version'}}}]}}}}
            self.e.kubectl('-n', 'platform-apps', 'patch', 'deployment', deployment['metadata']['name'], '--type=strategic', '-p', json.dumps(patch))
            self.e.kubectl('-n', 'platform-apps', 'patch', 'service', service['metadata']['name'], '--type=json', '-p', '[{"op":"replace","path":"/spec/ports/0/port","value":81}]')
            drift = self.e.client.get('/apps/'+app+'/drift')
            self.assertEqual(drift['state'], 'drifted')
            fields = {item['field'] for item in drift['differences']}
            for field in ('/spec/replicas','/spec/ports','/spec/template/spec/containers/application/image','/spec/template/spec/containers/application/env','/spec/template/spec/containers/application/resources','/spec/template/spec/containers/application/readinessProbe'):
                self.assertIn(field, fields)
            self.assertNotIn(changed_image, json.dumps(drift)); self.assertNotIn('operator-change', json.dumps(drift))
            self.assertEqual(self.e.resource('services', app)[0]['spec']['ports'][0]['port'], 81, 'Inspection overwrote operator changes')

            with self.page('configuration-drift') as page:
                self.browser_login(page); page.goto(self.e.url+'/apps/'+app)
                page.get_by_role('button', name='Diagnostics', exact=True).click()
                page.get_by_role('button', name='Inspect drift', exact=True).click()
                page.get_by_text('Configuration drift detected.', exact=True).wait_for()
            current = self.e.client.get('/apps/'+app)
            self.e.client.mutate('POST', '/apps/'+app+'/reapply', {'version':current['version']})
            self.e.http_version(app, 'v1')
            self.assertEqual(self.e.client.get('/apps/'+app+'/drift')['state'], 'in_sync')
        finally:
            self.e.kubectl('-n', 'platform-apps', 'delete', 'pod', copied_pod, '--ignore-not-found=true', check=False)
            for name in event_names: self.e.kubectl('-n', 'platform-apps', 'delete', 'event', name, '--ignore-not-found=true', check=False)
            self.e.delete(app); self.e.delete(other)
        self.e.record('Owned bounded logs/events masked real fixture values; browser escaped hostile text; drift reported owned changes without mutation and explicit reapply restored intent')

    def test_50_broker_replacement_preserves_session_and_app(self):
        app = self.e.create("restart-broker")
        try:
            before = self.e.client.get("/apps/" + app)
            administrator = self.e.client.get("/auth/me")["administratorId"]
            with self.e.forward("forward", app, port=self.e.app_port):
                self.e.restart_broker()
                import urllib.request
                with urllib.request.urlopen(f"http://127.0.0.1:{self.e.app_port}/version", timeout=5) as response:
                    self.assertEqual(json.load(response)["version"], "v1")
            self.assertEqual(self.e.client.get("/auth/me")["administratorId"], administrator)
            self.assertEqual(self.e.client.get("/apps/" + app)["lastSuccessfulRevision"], before["lastSuccessfulRevision"])
        finally: self.e.delete(app)
        self.e.record("Broker Pod replacement preserved sessions/history and application traffic")

    def test_70_unhealthy_update_restores_previous_snapshot(self):
        self.e.record("Testing unhealthy rollout: the intentional Helm timeout can take five minutes")
        app = self.e.create("failed", "v2")
        before = self.e.client.get("/apps/" + app)
        self.e.client.mutate("PATCH", "/apps/" + app, {"version": before["version"], "image": self.e.image("v3")}, terminal="failed")
        self.e.http_version(app, "v2")
        current = self.e.client.get("/apps/" + app)
        self.assertEqual(current["lastSuccessfulRevision"], before["lastSuccessfulRevision"])
        self.assertEqual(current["desiredRevision"], before["lastSuccessfulRevision"])
        self.assertFalse(current["activeOperationId"])
        self.e.restart_broker()
        self.assertFalse(self.e.client.get("/apps/" + app)["activeOperationId"])
        self.e.delete(app)
        self.e.record("Unhealthy candidate failed; previous snapshot restored and verified, mutation lock released")

    def test_80_release_bundle_deploys_on_a_fresh_cluster(self):
        from bundle import verify_bundle_installation
        verify_bundle_installation(self.e)

    def test_90_status_contract(self):
        app = self.e.create("contract")
        try:
            status = self.e.client.get("/apps/" + app)["status"]
            self.assertEqual(status["phase"], "ready")
            self.assertEqual(status["readyReplicas"], 1)
        finally: self.e.delete(app)

    def test_91_application_restart(self):
        app = self.e.create("restart")
        try:
            before = self.e.wait_replicas(app, 1)[0]["metadata"]["uid"]
            current = self.e.client.get("/apps/" + app)
            accepted, _ = self.e.client.mutate("POST", "/apps/" + app + "/restart", {"version": current["version"]})
            pod = self.e.wait_replicas(app, 1)[0]
            deployment = self.e.resource("deployments", app)[0]
            token = deployment["spec"]["template"]["metadata"].get("annotations", {}).get("platform.k8s.io/restart-token")
            self.assertEqual(token, accepted["operationId"])
            self.assertNotEqual(pod["metadata"]["uid"], before)
        finally: self.e.delete(app)

    def test_95_pending_first_install_and_queued_browser_resume(self):
        from playwright.sync_api import expect

        code, blocked = self.e.client.request('POST', '/apps', {
            'name': 'interrupted-'+secrets.token_hex(4), 'image': self.e.image('v3'),
            'containerPort': 8080, 'profile': 'http',
            'health': {'startupPath': '/healthz', 'readinessPath': '/readyz'},
            'exposure': {'mode': 'internal'},
        }, {'Idempotency-Key': secrets.token_hex(16)})
        self.assertEqual(code, 202)
        bad = blocked['applicationId']; operation_id = blocked['operationId']
        # The deliberately unready process keeps Helm pending. The observed
        # release and running Pod establish the interruption barrier; no sleep
        # guesses or production-only fault-injection switches are required.
        deadline = time.monotonic()+120
        pending = None
        while time.monotonic() < deadline:
            release = json.loads(self.e.kubectl('-n', 'platform-apps', 'get', 'secrets',
                '-l', 'owner=helm,name=app-'+bad.replace('_', '-'), '-o', 'json').stdout)['items']
            pods = self.e.resource('pods', bad)
            if len(release) == 1 and release[0]['metadata']['labels'].get('status') == 'pending-install' and any(
                s.get('state', {}).get('running') and not s.get('ready')
                for pod in pods for s in pod.get('status', {}).get('containerStatuses', [])
            ):
                pending = release[0]
                break
            time.sleep(1)
        self.assertIsNotNone(pending, 'No real pending Helm install/running unready Pod barrier was reached')
        frozen_image = self.e.client.get('/apps/'+bad+'/revisions')['items'][0]['frozenImage']

        with self.page('queued-recovery') as page:
            self.browser_login(page)
            submissions = []
            page.on('request', lambda r: submissions.append(r.url) if r.method == 'POST' and r.url.endswith('/api/v1/apps') else None)
            page.goto(self.e.url+'/apps/new')
            page.get_by_label('Image reference').fill(self.e.image('v1'))
            page.get_by_role('button', name='Inspect image', exact=True).click()
            page.get_by_role('button', name='Next').click()
            page.get_by_label('Name', exact=True).fill('queued-'+secrets.token_hex(4))
            page.get_by_label('Readiness path').fill('/readyz')
            for _ in range(3):
                page.get_by_role('button', name='Next').click()
            accepted = self.click_mutation(page, page.get_by_role('button', name='Deploy', exact=True), 'POST', '/apps')
            good = accepted['applicationId']
            state = page.get_by_text('State:', exact=True).locator('..')
            expect(state.get_by_text('queued', exact=True)).to_be_visible()
            # The create wizard displays progress inline. Follow the accepted
            # operation to its durable URL before testing reload/navigation.
            page.goto(self.e.url+'/apps/'+good+'/operation?op='+accepted['operationId'])
            page.reload()
            expect(state.get_by_text('queued', exact=True)).to_be_visible()
            page.goto(self.e.url+'/apps')
            page.goto(self.e.url+'/apps/'+good+'/operation?op='+accepted['operationId'])
            expect(state.get_by_text('queued', exact=True)).to_be_visible()
            self.assertEqual(len(submissions), 1)

            self.e.crash_broker()
            self.e.client.poll(operation_id, terminal='needs_attention')
            self.e.client.poll(accepted['operationId'])
            page.reload(); self.browser_success(page, accepted, navigates=False)
            self.assertEqual(len(submissions), 1, 'Refresh/navigation resubmitted a mutation')
            self.assertEqual(len(self.e.client.get('/apps/'+good+'/revisions')['items']), 1)
            self.e.http_version(good, 'v1')

            retained = json.loads(self.e.kubectl('-n', 'platform-apps', 'get', 'secret',
                pending['metadata']['name'], '-o', 'json').stdout)
            self.assertTrue(retained['metadata']['uid'] == pending['metadata']['uid'] and retained['data'] == pending['data'],
                'Recovery modified/deleted the pending Helm evidence')
            current = self.e.client.get('/apps/'+bad)
            self.assertEqual(current['activeOperationId'], operation_id)
            self.assertFalse(current['lastSuccessfulRevision'])
            self.assertEqual(self.e.client.get('/apps/'+bad+'/revisions')['items'][0]['frozenImage'], frozen_image)
            self.assertEqual(self.e.client.request('PATCH', '/apps/'+bad,
                {'version': current['version'], 'replicas': 2}, {'Idempotency-Key': secrets.token_hex(16)})[0], 409)
            self.e.restart_broker()
            self.assertEqual(self.e.client.get('/operations/'+operation_id)['state'], 'needs_attention')
            self.assertEqual(self.e.client.get('/apps/'+bad)['activeOperationId'], operation_id)

            page.goto(self.e.url+'/apps/'+bad)
            page.get_by_text('The first deployment has no successful revision to restore.', exact=False).wait_for()
            expect(page.get_by_role('button', name='Delete', exact=True)).to_be_enabled()
            page.on('dialog', lambda dialog: dialog.accept())
            removed = self.click_mutation(page, page.get_by_role('button', name='Delete', exact=True), 'DELETE', '/apps/'+bad)
            self.browser_success(page, removed)
            original = self.e.client.get('/operations/'+operation_id)
            self.assertEqual(original['state'], 'failed')
            self.assertEqual(original['stage'], 'removal_requested')
            self.assertEqual(self.e.client.get('/apps/'+bad+'/revisions')['items'][-1]['frozenImage'], frozen_image)
            self.assertFalse(self.e.resource('deployments,services', bad))
            self.e.delete(good)
        self.e.record('Runtime SIGKILL preserved pending Helm evidence and a durable lock; queued browser work resumed once; explicit first-deployment removal retained history')


class Result(unittest.TextTestResult):
    def _exc_info_to_string(self, error, test):
        return ENVIRONMENT.redact(super()._exc_info_to_string(error, test))

    def addError(self, test, error):
        super().addError(test, error)
        self.report_failure(test, self.errors[-1][1])

    def addFailure(self, test, error):
        super().addFailure(test, error)
        self.report_failure(test, self.failures[-1][1])

    def report_failure(self, test, message):
        (ENVIRONMENT.artifacts / (test._testMethodName + ".txt")).write_text(message)
        self.stream.writeln(message)

    def startTest(self, test):
        super().startTest(test)
        self.started = time.monotonic()

    def stopTest(self, test):
        failures = dict(self.failures + self.errors + self.expectedFailures)
        for unexpected in self.unexpectedSuccesses:
            failures[unexpected] = "An unexpected success is not accepted."
        element = XML.SubElement(self.xml, "testcase", name=test.id(), time=f"{time.monotonic()-self.started:.3f}")
        if test in failures: XML.SubElement(element, "failure").text = ENVIRONMENT.redact(failures[test])
        for skipped, reason in self.skipped:
            if skipped == test: XML.SubElement(element, "skipped", message=ENVIRONMENT.redact(reason))
        super().stopTest(test)

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.xml = XML.Element("testsuite", name="platform-real-workflows")


def main(artifacts):
    global ENVIRONMENT
    environment = ENVIRONMENT = Environment(artifacts)
    def interrupted(*_): raise KeyboardInterrupt()
    previous = signal.signal(signal.SIGTERM, interrupted)
    result = None
    try:
        environment.start()
        result = unittest.TextTestRunner(verbosity=2, resultclass=Result).run(unittest.defaultTestLoader.loadTestsFromTestCase(Workflows))
        result.xml.set("tests", str(result.testsRun)); result.xml.set("failures", str(len(result.failures) + len(result.errors) + len(result.expectedFailures) + len(result.unexpectedSuccesses)))
        result.xml.set("skipped", str(len(result.skipped)))
        XML.ElementTree(result.xml).write(artifacts / "junit.xml", encoding="unicode")
        return 0 if acceptance_successful(result) else 1
    finally:
        try: environment.diagnostics()
        finally:
            environment.close()
            signal.signal(signal.SIGTERM, previous)
