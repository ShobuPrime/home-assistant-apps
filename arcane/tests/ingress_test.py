#!/usr/bin/env python3
"""Open Arcane the way Home Assistant's sidebar does, in a real browser.

    ingress_test.py --ingress http://<app>:3554 --password <first password>
                    [--direct http://<app>:3552] [--container <name>]
                    [--tab] [--shots <dir>]

It needs python3 and Chrome or Chromium, nothing else. Three parts, the first
two shared with other apps' tests (.github/scripts/ingress_harness.py):

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
import json
import os
import sys
import time

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..', '.github', 'scripts'))
from ingress_harness import ORIGIN, PREFIX, Chrome, HomeAssistant, Page, find_chrome  # noqa: E402

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
