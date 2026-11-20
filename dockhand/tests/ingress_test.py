#!/usr/bin/env python3
"""Open Dockhand the way Home Assistant's sidebar does, in a real browser.

    ingress_test.py --ingress http://<app>:8099 [--direct http://<app>:3000]
                    [--container <name>] [--shots <dir>]

It needs python3 and Chrome or Chromium, nothing else. The stand-in for Home
Assistant and the browser driver are in .github/scripts/ingress_harness.py.

With --direct it first runs a control: the same page with nothing adapting it
must stay empty, or the checks prove nothing. --container names a running
container that has /bin/sh, for the list and the shell.
"""
import argparse
import json
import os
import sys
import time

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', '.github', 'scripts'))
from ingress_harness import ORIGIN, PREFIX, Chrome, HomeAssistant, Page, find_chrome  # noqa: E402

# Dockhand's sidebar, which every screen has.
APP = "!!document.querySelector('a[href=\"/containers\"]')"

ADD_ENVIRONMENT = """(async () => {
    const list = await (await fetch('/api/environments')).json();
    if (list.length) return 'already there';
    const r = await fetch('/api/environments', {method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({name: 'local', connectionType: 'socket', socketPath: '/var/run/docker.sock'})});
    return r.status;
})()"""

# The Shell screen's own WebSocket, opened by hand: find the container, open
# /api/containers/<id>/exec, type a command, read its output.
SHELL = """(async () => {
    const env = (await (await fetch('/api/environments')).json())[0].id;
    const list = await (await fetch('/api/containers?env=' + env)).json();
    const named = c => [c.name, ...(c.names || [])].filter(Boolean).map(n => n.replace(/^\\//, ''));
    const c = list.find(c => named(c).includes(%s));
    if (!c) return 'not among the ' + list.length + ' containers Dockhand lists';
    return await new Promise(resolve => {
        const ws = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host +
            '/api/containers/' + c.id + '/exec?shell=/bin/sh&user=root&envId=' + env);
        let out = '', typed = false;
        const done = v => { try { ws.close(); } catch (e) {} resolve(v); };
        setTimeout(() => done('nothing came back: ' + out.slice(-200)), 15000);
        ws.onerror = () => done('the socket failed');
        ws.onmessage = e => {
            try {
                const m = JSON.parse(e.data);
                if (m.type === 'output') out += m.data;
                if (m.type === 'error') done('error: ' + m.message);
            } catch (x) {}
            // The shell is ready once it has printed its prompt.
            if (!typed && out) { typed = true; ws.send(JSON.stringify({type: 'input', data: 'echo shell-$((40+2))\\n'})); }
            if (out.includes('shell-42')) done('ok');
        };
    });
})()"""


class Checks:
    def __init__(self, args, chrome, ha):
        self.args = args
        self.c = chrome
        self.ha = ha
        self.p = Page(chrome, framed=True)
        self.results = []
        self.shots = 0

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

    def loaded_since(self, before, timeout=20):
        end = time.time() + timeout
        while time.time() < end and len(self.c.loads) <= before:
            self.c.idle(0.2)
        return len(self.c.loads) > before

    def opens(self):
        self.c.goto(ORIGIN + '/')
        self.p.wait(APP)
        self.check('the panel shows Dockhand', True)
        self.check('the page believes it is at the site root', not self.p.path().startswith(PREFIX), self.p.path())
        self.shot('opened')

    def changes_something(self):
        # A new install has no environment. Adding the local socket is a POST,
        # which is what a cross-site check would refuse.
        answer = self.p.js(ADD_ENVIRONMENT)
        self.check('a request that changes something is accepted through the panel', answer in ('already there', 200), answer)
        before = len(self.c.loads)
        self.p.js("setTimeout(() => location.reload(), 0)")
        self.loaded_since(before)
        self.p.wait(APP)

    def moves_around(self):
        before = len(self.c.loads)
        self.p.js("document.querySelector('a[href=\"/containers\"]').click()")
        self.p.wait("location.pathname === '/containers'")
        self.check('a sidebar link changes screen without loading a page', len(self.c.loads) == before, self.c.loads[before:])
        if self.args.container:
            self.p.wait("(document.body.innerText || '').includes(%s)" % json.dumps(self.args.container), 45)
            self.check('the container list shows what Docker runs', True)
        self.shot('containers')

    def opens_a_shell(self):
        if not self.args.container:
            return
        answer = self.p.js(SHELL % json.dumps(self.args.container))
        self.check('a container shell works over a WebSocket through the panel', answer == 'ok', answer)

    def reloads(self):
        self.p.js("document.querySelector('a[href=\"/images\"]').click()")
        self.p.wait("location.pathname === '/images'")
        self.c.idle(2)
        before = len(self.c.loads)
        self.p.js("setTimeout(() => location.reload(), 0)")
        reloaded = self.loaded_since(before)
        self.p.wait("location.pathname === '/images' && " + APP)
        self.check('the app reloading itself returns to the same screen', reloaded and PREFIX + '/images' in self.c.loads[-1], self.c.loads[before:])
        self.shot('reloaded')

    def all(self):
        for name, fn in (('open', self.opens), ('change', self.changes_something), ('move around', self.moves_around),
                         ('shell', self.opens_a_shell), ('reload', self.reloads)):
            self.run(name, fn)
        self.c.idle(1)
        self.check('no request went to Home Assistant itself', not self.ha.outside, sorted(set(self.ha.outside))[:6])
        self.check('no uncaught error in the page', not self.c.exceptions, self.c.exceptions[:2])


def control(ha, binary):
    """Without the adapter the panel must stay empty. If it does not, the checks cannot tell."""
    chrome = Chrome(binary)
    try:
        chrome.open()
        chrome.goto(ORIGIN + '/')
        chrome.idle(8)
        try:
            shown = Page(chrome, framed=True).js(APP)
        except RuntimeError:  # a frame the browser refused cannot be looked into
            shown = False
    finally:
        chrome.close()
    ok = not shown or len(ha.outside) > 0
    why = f'{len(ha.outside)} requests went to Home Assistant itself' if shown else 'the browser shows nothing in the frame'
    print(('  PASS: ' if ok else '  FAIL: ') + f'control: without the adapter the panel does not work ({why})')
    return ok


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--ingress', required=True, help='the app\'s ingress port, e.g. http://172.17.0.3:8099')
    ap.add_argument('--direct', help='the app\'s own port, e.g. http://172.17.0.3:3000: adds the control')
    ap.add_argument('--container', default='', help='a running container with /bin/sh, for the list and the shell')
    ap.add_argument('--shots', help='a directory for screenshots')
    args = ap.parse_args()
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
