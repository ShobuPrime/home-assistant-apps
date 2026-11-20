#!/usr/bin/env python3
"""What a browser test of a sidebar panel needs, shared by the apps' tests.

It needs python3 and Chrome or Chromium, nothing else.

  HomeAssistant  a stand-in for Home Assistant ingress: it serves a page that
                 frames the app under /api/hassio_ingress/<token>/, forwards
                 that path to the app with the headers Home Assistant sends,
                 and records every request for any other path. Those are the
                 requests a broken panel sends to Home Assistant itself.
  Chrome         drives a headless browser over the DevTools pipe.
  Page           the app's own window: the tab, or the frame in the stand-in.

An app's test lives in <app>/tests/ingress_test.py and brings its own checks.
"""
import asyncio
import base64
import fcntl
import json
import os
import select
import shutil
import subprocess
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


def find_chrome():
    for name in (os.environ.get('CHROME'), 'chromium', 'chromium-browser', 'google-chrome', 'google-chrome-stable'):
        if name and shutil.which(name):
            return shutil.which(name)
    return None
