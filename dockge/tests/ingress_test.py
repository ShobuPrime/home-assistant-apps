#!/usr/bin/env python3
"""Open Dockge the way Home Assistant's sidebar does, in a real browser.

    ingress_test.py --ingress http://<app>:8099 [--direct http://<app>:5001]
                    [--listed <project>] [--hidden <project>]
                    [--deploy <stack> --image <image>] [--shots <dir>]

It needs python3 and Chrome or Chromium, nothing else. The stand-in for Home
Assistant and the browser driver are in .github/scripts/ingress_harness.py.

With --direct it first runs a control: the same page with nothing adapting it
must stay empty, or the checks prove nothing. --listed and --hidden name
Compose projects Dockge must and must not list. --deploy has Dockge deploy a
stack whose folder already exists in its stacks folder, with a ./conf folder
in it, the way its editor does.
"""
import argparse
import json
import os
import sys
import time

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', '.github', 'scripts'))
from ingress_harness import ORIGIN, PREFIX, Chrome, HomeAssistant, Page, find_chrome  # noqa: E402

USER = 'admin'
PASSWORD = 'Ingress-Test-2026'
# Dockge's root component, which holds the socket and the login state.
ROOT = "document.querySelector('#app')._vnode.component.proxy"
APP = "!!(document.querySelector('#app') && document.querySelector('#app')._vnode && %s.socketIO)" % ROOT

# Each starts a call and leaves its answer in window.__answer, which the test
# polls: a deploy can take longer than one evaluation may.
SIGN_IN = """window.__answer = null; (async () => {
    const root = %s;
    const call = (...args) => new Promise(resolve => root.getSocket().emit(...args, resolve));
    // A second run finds the account made by the first.
    const r = await call('setup', %s, %s);
    if (!r.ok && !/initialized/.test(r.msg || '')) { window.__answer = {ok: false, msg: 'setup: ' + r.msg}; return; }
    root.login(%s, %s, '', r => { window.__answer = r; });
})(); true""" % (ROOT, json.dumps(USER), json.dumps(PASSWORD), json.dumps(USER), json.dumps(PASSWORD))

DEPLOY = """window.__answer = null; %s.emitAgent('', 'deployStack', %%s, %%s, '', false, r => { window.__answer = r; }); true""" % ROOT

STACK_YAML = """services:
  web:
    image: %s
    command: sleep 600
    volumes:
      # Not created when missing: a wrong host path fails instead of leaving
      # a folder behind on the host.
      - type: bind
        source: ./conf
        target: /conf
        bind:
          create_host_path: false
"""


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
        self.check('the panel shows Dockge', True)
        self.check('the page believes it is at the site root', not self.p.path().startswith(PREFIX), self.p.path())
        self.p.wait(ROOT + '.socketIO.connected', 30)
        self.check('Dockge\'s socket connects through the panel', self.ha.sockets > 0, f'{self.ha.sockets} WebSockets')
        self.shot('opened')

    def signs_in(self):
        self.p.js(SIGN_IN)
        answer = self.p.wait('window.__answer', 40)
        self.check('setting up and signing in work through the panel', answer.get('ok') and self.p.wait(ROOT + '.loggedIn', 20), answer)
        # The setup page would now move on to the dashboard itself.
        before = len(self.c.loads)
        self.p.js(ROOT + ".$router.push('/')")
        self.p.wait("location.pathname === '/'")
        self.check('moving to the dashboard does not load a page', len(self.c.loads) == before, self.c.loads[before:])
        self.shot('signed-in')

    def lists(self):
        if not (self.args.listed or self.args.hidden):
            return
        text = "(document.body.innerText || '')"
        if self.args.listed:
            self.p.wait('%s.includes(%s)' % (text, json.dumps(self.args.listed)), 45)
            self.check('Dockge lists a Compose project that is somebody\'s own', True)
        if self.args.hidden:
            shown = self.p.js('%s.includes(%s)' % (text, json.dumps(self.args.hidden)))
            self.check('Dockge does not list one whose containers Home Assistant manages', not shown)
        self.shot('stack-list')

    def deploys(self):
        if not self.args.deploy:
            return
        self.p.js(DEPLOY % (json.dumps(self.args.deploy), json.dumps(STACK_YAML % self.args.image)))
        answer = self.p.wait('window.__answer', 180)
        self.check('Dockge deploys a stack through the panel', answer.get('ok'), answer)

    def reloads(self):
        before = len(self.c.loads)
        self.p.js("setTimeout(() => location.reload(), 0)")
        reloaded = self.loaded_since(before)
        self.p.wait(APP)
        self.p.wait(ROOT + '.loggedIn', 30)
        self.check('a reload returns to Dockge, still signed in', reloaded and PREFIX in self.c.loads[-1], self.c.loads[before:])
        self.shot('reloaded')

    def all(self):
        for name, fn in (('open', self.opens), ('sign in', self.signs_in), ('list', self.lists),
                         ('deploy', self.deploys), ('reload', self.reloads)):
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
    why = f'{len(ha.outside)} requests went to Home Assistant itself' if ha.outside else 'the page does not come up'
    print(('  PASS: ' if ok else '  FAIL: ') + f'control: without the adapter the panel does not work ({why})')
    return ok


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument('--ingress', required=True, help='the app\'s ingress port, e.g. http://172.17.0.3:8099')
    ap.add_argument('--direct', help='the app\'s own port, e.g. http://172.17.0.3:5001: adds the control')
    ap.add_argument('--listed', default='', help='a Compose project Dockge must list')
    ap.add_argument('--hidden', default='', help='a Compose project Dockge must not list')
    ap.add_argument('--deploy', default='', help='a stack folder that exists, with conf/ in it, for Dockge to deploy')
    ap.add_argument('--image', default='mirror.gcr.io/library/python:3-slim', help='the image of the deployed stack')
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
