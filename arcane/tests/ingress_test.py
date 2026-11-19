#!/usr/bin/env python3
"""Open Arcane the way Home Assistant's sidebar does, in a real browser.

    ingress_test.py --ingress http://<app>:3554 --password <first password>
                    [--direct http://<app>:3552] [--container <name>]
                    [--tab] [--shots <dir>]

It needs python3 and Chrome or Chromium, nothing else. Three parts:

  HomeAssistant  a stand-in for Home Assistant ingress: it serves a page that
                 frames the app under /api/hassio_ingress/<token>/, forwards
                 that path to --ingress with the headers Home Assistant sends,
                 and records every request for any other path. Those are the
                 requests a broken panel sends to Home Assistant itself.
  Chrome         drives a headless browser over the DevTools pipe.
  the checks     sign in, move around, reload, sign out, lose the session.

With --direct it first runs a control: the same page with no proxy in
between must fail, or the checks prove nothing. It also checks that a session
on the direct port and one in the panel stay apart.
"""
import argparse
import asyncio
import base64
import fcntl
import json
import os
import select
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from urllib.parse import urlsplit

PREFIX = '/api/hassio_ingress/TESTTOKEN_abcdefgh'
PORT = 18123
ORIGIN = f'http://127.0.0.1:{PORT}'

FRAME_PAGE = '''<!doctype html><html><head><meta charset="utf-8"><title>Home Assistant stand-in</title>
<link rel="icon" href="data:,"><style>html,body{margin:0;height:100%%}iframe{border:0;width:100%%;height:100%%;display:block}</style>
</head><body><iframe id="panel" src="%s/"></iframe></body></html>'''


class HomeAssistant:
    """What Home Assistant does to a panel request.

    Follows homeassistant/components/hassio/ingress.py (Core 2026.11) and
    supervisor/api/ingress.py (Supervisor 2026.11): Accept-Encoding is dropped,
    Host and Cookie pass through, X-Ingress-Path and friends are added, and the
    answer's Content-Type loses its charset.
    """

    def __init__(self, upstream):
        self.point_at(upstream)
        self.loop = asyncio.new_event_loop()
        self.ready = threading.Event()
        threading.Thread(target=self._run, daemon=True).start()
        if not self.ready.wait(10):
            raise RuntimeError('the Home Assistant stand-in did not start')

    def point_at(self, upstream):
        """Forward to this address from now on, and start counting again."""
        target = urlsplit(upstream)
        self.upstream = (target.hostname, target.port or 80)
        self.inside = 0
        self.outside = []
        self.sockets = 0

    def _run(self):
        asyncio.set_event_loop(self.loop)
        self.loop.run_until_complete(asyncio.start_server(self._client, '127.0.0.1', PORT, limit=1 << 20))
        self.ready.set()
        self.loop.run_forever()

    async def _client(self, reader, writer):
        try:
            await self._serve(reader, writer)
        except (ConnectionError, OSError, asyncio.IncompleteReadError, asyncio.LimitOverrunError, ValueError):
            pass
        finally:
            writer.close()

    @staticmethod
    async def _answer(writer, status, kind, body=''):
        body = body.encode()
        writer.write(f'HTTP/1.1 {status} -\r\nContent-Type: {kind}\r\nContent-Length: {len(body)}\r\nConnection: close\r\n\r\n'.encode() + body)
        await writer.drain()

    @staticmethod
    async def _pipe(reader, writer):
        try:
            while data := await reader.read(65536):
                writer.write(data)
                await writer.drain()
        except (ConnectionError, OSError):
            pass

    async def _serve(self, reader, writer):
        head = (await reader.readuntil(b'\r\n\r\n')).decode('latin-1').split('\r\n')
        method, target, _ = head[0].split(' ', 2)
        headers = [(n.strip(), v.strip()) for n, v in (line.split(':', 1) for line in head[1:] if ':' in line)]
        path = target.split('?', 1)[0]
        if path == '/':
            return await self._answer(writer, 200, 'text/html', FRAME_PAGE % PREFIX)
        if path == '/favicon.ico':  # Home Assistant has one; a tab without an icon asks for it
            return await self._answer(writer, 204, 'image/x-icon')
        if not path.startswith(PREFIX + '/'):
            self.outside.append(f'{method} {path}')
            return await self._answer(writer, 404, 'text/plain', '404: Not Found')
        self.inside += 1

        def sent(name):
            return next((v for n, v in headers if n.lower() == name), '')

        upgrade = sent('upgrade').lower() == 'websocket'
        dropped = {'accept-encoding', 'connection', 'keep-alive', 'x-ingress-path', 'x-hass-source',
                   'x-forwarded-for', 'x-forwarded-host', 'x-forwarded-proto'}
        out = [(n, v) for n, v in headers if n.lower() not in dropped and not n.lower().startswith('x-remote-user-')]
        out += [('X-Hass-Source', 'core.ingress'), ('X-Ingress-Path', PREFIX),
                ('X-Forwarded-For', '127.0.0.1, 172.30.32.1'), ('X-Forwarded-Host', sent('host')), ('X-Forwarded-Proto', 'http'),
                ('X-Remote-User-Id', '0123456789abcdef0123456789abcdef'), ('X-Remote-User-Name', 'tester'),
                ('X-Remote-User-Display-Name', 'Tester'), ('Connection', 'Upgrade' if upgrade else 'close')]

        app_reader, app_writer = await asyncio.open_connection(*self.upstream, limit=1 << 20)
        to_app = asyncio.ensure_future(self._pipe(reader, app_writer))
        try:
            app_writer.write((f'{method} {target[len(PREFIX):]} HTTP/1.1\r\n' + ''.join(f'{n}: {v}\r\n' for n, v in out) + '\r\n').encode('latin-1'))
            await app_writer.drain()
            reply = (await app_reader.readuntil(b'\r\n\r\n')).decode('latin-1').split('\r\n')[:-2]
            status = reply[0].split(' ', 2)[1]
            lines = [reply[0]]
            for line in reply[1:]:
                name, _, value = line.partition(':')
                if status != '101' and name.lower() in ('connection', 'keep-alive'):
                    continue
                if name.lower() == 'content-type':
                    line = 'Content-Type: ' + value.split(';')[0].strip()
                lines.append(line)
            if status == '101':
                self.sockets += 1
            else:
                lines.append('Connection: close')
            writer.write(('\r\n'.join(lines) + '\r\n\r\n').encode('latin-1'))
            await writer.drain()
            await self._pipe(app_reader, writer)
        finally:
            to_app.cancel()
            app_writer.close()


class Chrome:
    """A headless browser, driven over --remote-debugging-pipe."""

    def __init__(self, binary):
        to_r, to_w = os.pipe()
        from_r, from_w = os.pipe()

        def child():
            a = fcntl.fcntl(to_r, fcntl.F_DUPFD, 10)
            b = fcntl.fcntl(from_w, fcntl.F_DUPFD, 10)
            os.dup2(a, 3)
            os.dup2(b, 4)

        self.profile = tempfile.mkdtemp(prefix='arcane-ingress-test-')
        flags = ['--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
                 '--disable-background-networking', '--disable-component-update', '--disable-sync',
                 '--window-size=1360,900', f'--user-data-dir={self.profile}', '--remote-debugging-pipe']
        if os.environ.get('CI') or os.geteuid() == 0:
            flags += ['--no-sandbox', '--disable-dev-shm-usage']
        self.proc = subprocess.Popen([binary, *flags, 'about:blank'], preexec_fn=child, pass_fds=(3, 4),
                                     stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        os.close(to_r)
        os.close(from_w)
        self.w, self.r = to_w, from_r
        self.buf = b''
        self.next_id = 0
        self.results = {}
        self.sockets = {}
        self.exceptions = []
        self.loads = []       # pages loaded, as opposed to screens changed in place
        self.downloads = []
        self.session = None

    def pump(self, timeout=0.05):
        if not select.select([self.r], [], [], timeout)[0]:
            return
        data = os.read(self.r, 1 << 20)
        if not data:
            raise RuntimeError('the browser went away')
        self.buf += data
        while b'\0' in self.buf:
            raw, self.buf = self.buf.split(b'\0', 1)
            msg = json.loads(raw)
            if 'id' in msg:
                self.results[msg['id']] = msg
            else:
                self.event(msg.get('method', ''), msg.get('params', {}))

    def event(self, method, p):
        if method == 'Network.webSocketCreated':
            self.sockets[p['requestId']] = {'url': p['url'], 'status': None, 'frames': 0}
        elif method == 'Network.webSocketHandshakeResponseReceived':
            self.sockets.get(p['requestId'], {})['status'] = p['response']['status']
        elif method == 'Network.webSocketFrameReceived':
            if p['requestId'] in self.sockets:
                self.sockets[p['requestId']]['frames'] += 1
        elif method == 'Runtime.exceptionThrown':
            d = p.get('exceptionDetails', {})
            self.exceptions.append((d.get('text', '') + ' ' + str(d.get('exception', {}).get('description', '')))[:300])
        elif method == 'Page.frameNavigated':
            self.loads.append(p.get('frame', {}).get('url'))
        elif method == 'Page.downloadWillBegin':
            self.downloads.append(p.get('url'))

    def call(self, method, params=None, timeout=30, page=True):
        self.next_id += 1
        msg = {'id': self.next_id, 'method': method, 'params': params or {}}
        if page:
            msg['sessionId'] = self.session
        os.write(self.w, json.dumps(msg).encode() + b'\0')
        end = time.time() + timeout
        while self.next_id not in self.results:
            if time.time() > end:
                raise TimeoutError(method)
            self.pump()
        msg = self.results.pop(self.next_id)
        if 'error' in msg:
            raise RuntimeError(f"{method}: {msg['error']}")
        return msg.get('result', {})

    def idle(self, seconds):
        end = time.time() + seconds
        while time.time() < end:
            self.pump()

    def open(self):
        target = self.call('Target.createTarget', {'url': 'about:blank'}, page=False)['targetId']
        self.session = self.call('Target.attachToTarget', {'targetId': target, 'flatten': True}, page=False)['sessionId']
        for domain in ('Page', 'Runtime', 'Network'):
            self.call(domain + '.enable')
        self.call('Browser.setDownloadBehavior', {'behavior': 'deny', 'eventsEnabled': True}, page=False)

    def goto(self, url):
        self.call('Page.navigate', {'url': url})

    def js(self, code):
        res = self.call('Runtime.evaluate', {'expression': code, 'awaitPromise': True, 'returnByValue': True, 'userGesture': True})
        if 'exceptionDetails' in res:
            d = res['exceptionDetails']
            raise RuntimeError('js: ' + d.get('text', '') + ' ' + str(d.get('exception', {}).get('description', ''))[:300])
        return res.get('result', {}).get('value')

    def cookies(self):
        return self.call('Storage.getCookies', page=False)['cookies']

    def shot(self, path):
        with open(path, 'wb') as f:
            f.write(base64.b64decode(self.call('Page.captureScreenshot', {'format': 'png'})['data']))

    def close(self):
        try:
            self.call('Browser.close', page=False, timeout=5)
            self.proc.wait(timeout=5)
        except Exception:
            self.proc.kill()
        shutil.rmtree(self.profile, ignore_errors=True)


class Page:
    """The app's own window: the tab itself, or the frame inside the stand-in's page."""

    def __init__(self, chrome, framed):
        self.c = chrome
        self.framed = framed

    def js(self, code):
        if self.framed:
            code = "document.getElementById('panel').contentWindow.eval(%s)" % json.dumps(code)
        return self.c.js(code)

    def wait(self, code, timeout=40):
        end = time.time() + timeout
        last = None
        while time.time() < end:
            try:
                last = self.js(code)
                if last:
                    return last
            except RuntimeError as e:
                last = str(e)
            self.c.idle(0.25)
        raise TimeoutError(f'gave up waiting for: {code[:120]} (last: {str(last)[:200]})')

    def type(self, selector, text):
        if not self.js("""(() => { const el = document.querySelector(%s); if (!el) return false;
                el.focus();
                Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set.call(el, %s);
                el.dispatchEvent(new Event('input', {bubbles: true}));
                return true; })()""" % (json.dumps(selector), json.dumps(text))):
            raise RuntimeError('no such field: ' + selector)

    def click(self, text, among='button, a, [role=tab], [role=menuitem]'):
        """Click the last visible element whose text is exactly `text`."""
        if not self.js("""(() => { const els = [...document.querySelectorAll(%s)].filter(e =>
                e.getClientRects().length && (e.innerText || e.getAttribute('aria-label') || '').trim() === %s);
                if (!els.length) return false; els[els.length - 1].click(); return true; })()""" % (json.dumps(among), json.dumps(text))):
            raise RuntimeError('nothing to click: ' + text)

    def text(self):
        return self.js("(document.body.innerText || '').replace(/\\s+/g, ' ')")

    def path(self):
        return self.js('location.pathname + location.search')


FORM = "!!document.querySelector('#username')"
APP = "!!document.querySelector('a[href=\"/containers\"]')"


class Checks:
    def __init__(self, args, chrome, ha):
        self.args = args
        self.c = chrome
        self.ha = ha
        self.p = Page(chrome, framed=not args.tab)
        self.start = ORIGIN + ('/' if not args.tab else PREFIX + '/')
        self.results = []
        self.shots = 0
        self.works = args.password   # the password that signs in

    def check(self, name, ok, detail=''):
        self.results.append((name, bool(ok)))
        print(('  PASS: ' if ok else '  FAIL: ') + name + (f' ({str(detail)[:240]})' if detail and not ok else ''))
        return ok

    def shot(self, name):
        if self.args.shots:
            self.shots += 1
            self.c.shot(os.path.join(self.args.shots, f'{self.shots:02d}-{name}.png'))

    def run(self, name, fn):
        try:
            fn()
        except Exception as e:  # later checks still say something
            self.check(name, False, f'{type(e).__name__}: {e}')
            try:
                self.shot('failed-' + name.replace(' ', '-'))
                print('        at', self.p.path(), '|', self.p.text()[:200])
            except Exception:
                pass

    def socket(self, part, frames=0, timeout=25):
        end = time.time() + timeout
        while time.time() < end:
            for s in self.c.sockets.values():
                if part in s['url'] and s['status'] == 101 and s['frames'] >= frames:
                    return s
            self.c.idle(0.25)
        return None

    def loaded_since(self, before, timeout=20):
        end = time.time() + timeout
        while time.time() < end and len(self.c.loads) <= before:
            self.c.idle(0.2)
        return len(self.c.loads) > before

    def sign_in(self, page=None):
        """Sign in. The password is the first one until a run has changed it."""
        p = page or self.p
        other = self.args.new_password if self.works == self.args.password else self.args.password
        for password in (self.works, other):
            p.wait(FORM)
            p.type('#username', 'arcane')
            p.type('#password', password)
            p.click('Sign in to Arcane')
            try:
                p.wait("!document.querySelector('#username')", 12)
                self.works = password
                break
            except TimeoutError:
                continue
        p.wait("location.pathname !== '/login'")
        self.c.idle(2)
        if 'Change Default Password' in p.text():
            p.type('#current-password', self.works)
            p.type('#new-password', self.args.new_password)
            p.type('#confirm-password', self.args.new_password)
            p.click('Change Password')
            p.wait("!(document.body.innerText || '').includes('Change Default Password')")
            self.works = self.args.new_password
        p.wait(APP)
        self.c.idle(1.5)

    # -- the checks --------------------------------------------------------

    def opens(self):
        self.c.goto(self.start)
        self.p.wait(FORM)
        self.check('the panel opens on the sign-in form', self.p.path().startswith('/login'), self.p.path())
        self.shot('sign-in')

    def signs_in(self):
        self.sign_in()
        self.check('signing in reaches the dashboard', self.p.path().startswith('/dashboard'), self.p.path())
        names = sorted((ck['name'], ck['path']) for ck in self.c.cookies() if ck['name'].split('.')[-1] in ('token', 'refresh'))
        self.check('the session cookies are the panel\'s own and scoped to its path',
                   len(names) == 2 and all(n.startswith('ingress.') and path.startswith(PREFIX + '/') for n, path in names), names)
        stats = self.socket('/ws/system/stats', frames=1)
        self.check('live statistics arrive over a WebSocket', stats and PREFIX in stats['url'], [s['url'] for s in self.c.sockets.values()])
        self.shot('dashboard')

    def moves_around(self):
        before = len(self.c.loads)
        self.p.js("document.querySelector('a[href=\"/containers\"]').click()")
        self.p.wait("location.pathname === '/containers' && !!document.querySelector('a[href^=\"/containers/\"]')")
        self.check('a sidebar link changes screen without loading a page', len(self.c.loads) == before, self.c.loads[before:])
        marked = self.p.js("(document.querySelector('a[href=\"/containers\"][aria-current=page]') || {}).innerText || ''")
        self.check('the sidebar marks the current screen', 'Containers' in marked, marked)
        self.shot('containers')

    def follows_logs(self):
        self.p.js("""(() => { const all = [...document.querySelectorAll('a[href^="/containers/"]')];
            (all.find(a => (a.innerText || '').replace(/\\s+/g, '') === %s) || all[0]).click(); })()""" % json.dumps(self.args.container))
        self.p.wait("location.pathname.startsWith('/containers/') && !!document.querySelector('[role=tab]')")
        self.c.idle(1)
        self.p.click('Logs', '[role=tab]')
        self.c.idle(1)
        self.p.click('Start', 'button')
        stream = self.socket('/logs', frames=1)
        self.check('a container\'s log stream arrives over a WebSocket', stream and PREFIX in stream['url'], [s['url'][-80:] for s in self.c.sockets.values()])
        self.shot('logs')
        self.p.click('Download', 'button')
        end = time.time() + 15
        while time.time() < end and not self.c.downloads:
            self.c.idle(0.25)
        self.check('downloading the log asks for the panel\'s address',
                   self.c.downloads and self.c.downloads[-1].startswith(ORIGIN + PREFIX + '/api/'), self.c.downloads[-1:])
        self.p.click('Shell', '[role=tab]')
        shell = self.socket('/terminal')
        self.check('a container shell opens a WebSocket', shell and PREFIX in shell['url'], [s['url'][-80:] for s in self.c.sockets.values()])

    def reloads(self):
        self.p.js("document.querySelector('a[href=\"/images\"]').click()")
        self.p.wait("location.pathname === '/images'")
        self.c.idle(2)
        before = len(self.c.loads)
        self.p.js("setTimeout(() => location.reload(), 0)")
        reloaded = self.loaded_since(before)
        self.p.wait("location.pathname === '/images' && " + APP)
        self.check('the app reloading itself returns to the same screen', reloaded and PREFIX + '/images' in self.c.loads[-1], self.c.loads[before:])
        if self.args.tab:
            return  # a tab of its own reloads the address without the prefix: see DOCS.md
        before = len(self.c.loads)
        self.c.call('Page.reload')
        self.loaded_since(before)
        self.p.wait(APP)
        self.check('the browser\'s reload button returns signed in', self.p.path().startswith('/dashboard'), self.p.path())
        self.c.idle(1)

    def leaves_by_link(self):
        seen = self.p.js("""(() => { const a = document.querySelector('a[href="/volumes"]'); const seen = {};
            document.addEventListener('click', (e) => { seen.during = a.getAttribute('href'); e.preventDefault(); e.stopPropagation(); }, {once: true});
            a.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true, ctrlKey: true, button: 0}));
            return new Promise(done => setTimeout(() => { seen.after = a.getAttribute('href'); done(seen); }, 60)); })()""")
        self.check('a link opened in a new tab carries the panel\'s address, and only then',
                   seen == {'during': PREFIX + '/volumes', 'after': '/volumes'}, seen)
        fonts = self.p.js("document.fonts.ready.then(() => [...new Set([...document.fonts].filter(f => f.status === 'loaded').map(f => f.family.replace(/[\"']/g, '')))])")
        self.check('the web fonts loaded', any('Mona' in f for f in fonts), fonts)
        broken = self.p.js("[...document.images].filter(i => i.complete && !i.naturalWidth && i.getAttribute('src')).map(i => i.getAttribute('src').slice(0, 80))")
        self.check('no image is broken', not broken, broken[:4])

    def signs_out(self):
        self.p.js("""(() => { const els = [...document.querySelectorAll('button[aria-haspopup=menu]')].filter(e => /Arcane Admin/.test(e.innerText || '')); els[els.length - 1].click(); })()""")
        self.p.wait("(document.body.innerText || '').includes('Log out')", 10)
        self.c.idle(0.5)
        self.p.js("""(() => { const els = [...document.querySelectorAll('body *')].filter(e => (e.innerText || '').trim() === 'Log out' && e.getClientRects().length);
            const el = els[els.length - 1];
            for (const type of ['pointerdown', 'mousedown', 'pointerup', 'mouseup', 'click'])
                el.dispatchEvent(new (type.startsWith('pointer') ? PointerEvent : MouseEvent)(type, {bubbles: true, cancelable: true, button: 0, pointerType: 'mouse'})); })()""")
        self.p.wait("location.pathname.startsWith('/login') && " + FORM)
        self.c.idle(1)
        left = [ck['name'] for ck in self.c.cookies() if ck['name'] in ('ingress.token', 'ingress.refresh') and ck['value']]
        self.check('signing out returns to the form and ends the session', not left, left)

    def loses_the_session(self):
        self.sign_in()
        self.p.js("document.querySelector('a[href=\"/networks\"]').click()")
        self.p.wait("location.pathname === '/networks'")
        self.c.idle(2)
        self.c.call('Storage.clearCookies', page=False)
        self.p.js("document.querySelector('a[href=\"/volumes\"]').click()")
        self.p.wait("location.pathname.startsWith('/login') && " + FORM, 60)
        self.check('a lost session ends at the sign-in form', 'redirect=' in self.p.path(), self.p.path())
        self.sign_in()
        self.check('signing in again returns to where the session was lost', self.p.path().split('?')[0] in ('/networks', '/volumes'), self.p.path())

    def reads_the_api_reference(self):
        self.c.goto(ORIGIN + PREFIX + '/api/docs')
        tab = Page(self.c, framed=False)
        tab.wait("(document.body.innerText || '').includes('/api/')")
        self.check('the API reference loads Arcane\'s description', 'Arcane' in tab.text(), tab.text()[:120])

    def keeps_sessions_apart(self):
        direct = Page(self.c, framed=False)
        self.c.call('Storage.clearCookies', page=False)
        self.c.goto(self.args.direct + '/')
        self.sign_in(direct)
        self.c.goto(self.start)
        self.p.wait(FORM + ' || ' + APP)
        self.c.idle(1.5)
        self.check('a session on the direct port does not sign the panel in', self.p.js(FORM), self.p.path())
        self.sign_in()
        self.signs_out()
        self.c.goto(self.args.direct + '/')
        direct.wait(FORM + ' || ' + APP)
        self.c.idle(2)
        self.check('signing out of the panel leaves the direct session alone', direct.js(APP), direct.path())

    def all(self):
        steps = [('open', self.opens), ('sign in', self.signs_in), ('move around', self.moves_around),
                 ('logs', self.follows_logs), ('reload', self.reloads), ('links', self.leaves_by_link),
                 ('sign out', self.signs_out), ('lost session', self.loses_the_session),
                 ('API reference', self.reads_the_api_reference)]
        if self.args.direct:
            steps.append(('separate sessions', self.keeps_sessions_apart))
        for name, fn in steps:
            self.run(name, fn)
        self.c.idle(1)
        self.check('no request went to Home Assistant itself', not self.ha.outside, sorted(set(self.ha.outside))[:6])
        self.check('no uncaught error in the page', not self.c.exceptions, self.c.exceptions[:2])


def control(ha, binary):
    """Without the proxy the panel must fail. If it does not, the checks cannot tell."""
    chrome = Chrome(binary)
    try:
        chrome.open()
        chrome.goto(ORIGIN + PREFIX + '/')
        chrome.idle(6)
        form = chrome.js(FORM)
    finally:
        chrome.close()
    ok = not form and len(ha.outside) > 0
    print(('  PASS: ' if ok else '  FAIL: ') + f'control: without the proxy the panel stays blank ({len(ha.outside)} requests went to Home Assistant itself)')
    return ok


def find_chrome():
    for name in (os.environ.get('CHROME'), 'chromium', 'chromium-browser', 'google-chrome', 'google-chrome-stable'):
        if name and shutil.which(name):
            return shutil.which(name)
    return None


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--ingress', required=True, help='the app\'s ingress port, e.g. http://172.17.0.3:3554')
    ap.add_argument('--direct', help='the app\'s own port, e.g. http://172.17.0.3:3552: adds the control and the session checks')
    ap.add_argument('--password', required=True, help='the first-login password from the app log')
    ap.add_argument('--new-password', default='Ingress-Test-2026!x')
    ap.add_argument('--container', default='', help='the container whose log to follow (default: the first listed)')
    ap.add_argument('--tab', action='store_true', help='open the panel in a tab of its own instead of a frame')
    ap.add_argument('--shots', help='a directory for screenshots')
    args = ap.parse_args()
    if args.direct:
        args.direct = args.direct.rstrip('/')
    if args.shots:
        os.makedirs(args.shots, exist_ok=True)

    binary = find_chrome()
    if not binary:
        print('no Chrome or Chromium found (set CHROME to its path)')
        return 2

    ok = True
    ha = HomeAssistant(args.direct or args.ingress)
    if args.direct:
        ok = control(ha, binary)
        ha.point_at(args.ingress)

    chrome = Chrome(binary)
    try:
        chrome.open()
        checks = Checks(args, chrome, ha)
        checks.all()
    finally:
        chrome.close()
    failed = [name for name, passed in checks.results if not passed]
    print(f'  {len(checks.results) - len(failed)} passed, {len(failed)} failed; '
          f'{ha.inside} requests through the panel, {ha.sockets} WebSockets, {len(ha.outside)} outside it')
    return 0 if ok and not failed else 1


if __name__ == '__main__':
    code = main()
    sys.stdout.flush()
    os._exit(code)  # the stand-in's thread still holds open connections
