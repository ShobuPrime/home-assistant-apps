// Loaded ahead of every other script on the app's page, and only under Home
// Assistant ingress.
//
// The app expects to own the site root: it reads the page's path as its
// route, requests /api/..., and redirects to "/...". Home Assistant serves it
// under window.__haIngressPath instead. This script hides that:
//
//   1. the page's own address loses the prefix, so the app's routing and path
//      checks see what they expect
//   2. every address the app asks the server for gets the prefix back
//   3. a full page load (a reload, a redirect by the app) is sent to the
//      prefixed address, where the proxy serves this page again
(function () {
  'use strict';

  var prefix = window.__haIngressPath;
  if (!prefix) return;
  var openAt = window.__haIngressOpenAt;
  var origin = location.origin;
  var host = location.host;

  function outside(path) {
    return path !== prefix && path.indexOf(prefix + '/') !== 0;
  }

  // The address on the server for one the app wrote, or null to leave it alone.
  function prefixed(url) {
    if (!url) return null;
    var u;
    try {
      u = new URL(url, document.baseURI);
    } catch (e) {
      return null;
    }
    var web = (u.protocol === 'http:' || u.protocol === 'https:') && u.origin === origin;
    var socket = (u.protocol === 'ws:' || u.protocol === 'wss:') && u.host === host;
    if (!(web || socket) || !outside(u.pathname)) return null;
    u.pathname = prefix + u.pathname;
    return u;
  }
  function toServer(url) {
    var u = prefixed(String(url));
    return u ? u.href : url;
  }
  // The same as a path, for attributes.
  function toServerPath(url) {
    var u = prefixed(url);
    return u ? u.pathname + u.search + u.hash : url;
  }

  // 1. The page's own address.

  function hidePrefix(opening) {
    if (outside(location.pathname)) return;
    var path = location.pathname.slice(prefix.length) || '/';
    // An app whose "/" answers a signed-out visitor with a full page load is
    // opened at the page that shows its form instead (INGRESS_OPEN_AT).
    if (opening && path === '/' && openAt) path = openAt;
    history.replaceState(history.state, '', path + location.search + location.hash);
  }
  hidePrefix(true);

  // A page kept by the browser comes back with the address showPrefix left.
  addEventListener('pageshow', function (event) {
    if (event.persisted) hidePrefix(false);
  });
  // If the browser restores this frame later, it has to ask for an address
  // the proxy serves.
  addEventListener('pagehide', function showPrefix() {
    if (outside(location.pathname)) {
      history.replaceState(history.state, '', prefix + location.pathname + location.search + location.hash);
    }
  });

  // 2. What the app asks the server for.

  // fetch(url) and fetch(new Request(url)). A Request is given its address
  // when it is built: re-addressing one later would turn its body into a
  // stream, which browsers only upload over HTTP/2.
  var fetch0 = window.fetch;
  window.fetch = function (input, init) {
    if (typeof input === 'string' || input instanceof URL) input = toServer(input);
    return fetch0.call(this, input, init);
  };

  var open0 = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function (method, url) {
    arguments[1] = toServer(url);
    return open0.apply(this, arguments);
  };

  function wrapConstructor(name) {
    var Native = window[name];
    if (!Native) return;
    function Wrapped(url, arg) {
      if (typeof url === 'string' || url instanceof URL) url = toServer(url);
      return arguments.length > 1 ? new Native(url, arg) : new Native(url);
    }
    Wrapped.prototype = Native.prototype;
    Object.setPrototypeOf(Wrapped, Native); // WebSocket.OPEN and friends
    window[name] = Wrapped;
  }
  wrapConstructor('Request');
  wrapConstructor('WebSocket');
  wrapConstructor('EventSource');

  var windowOpen0 = window.open;
  window.open = function (url) {
    if (typeof url === 'string') arguments[0] = toServer(url);
    return windowOpen0.apply(this, arguments);
  };

  // Images, icons, style sheets and scripts that the app adds to the page.
  // Links are left alone: the app's router reads them (see 3).
  var RESOURCE = /^(IMG|LINK|SCRIPT|SOURCE|VIDEO|AUDIO|TRACK|IFRAME|EMBED)$/;

  var setAttribute0 = Element.prototype.setAttribute;
  Element.prototype.setAttribute = function (name, value) {
    if ((name === 'src' || name === 'href') && typeof value === 'string' && RESOURCE.test(this.tagName)) {
      value = toServerPath(value);
    }
    return setAttribute0.call(this, name, value);
  };

  function wrapSetter(type, name) {
    var d = type && Object.getOwnPropertyDescriptor(type.prototype, name);
    if (!d || !d.set) return;
    Object.defineProperty(type.prototype, name, {
      configurable: true,
      enumerable: d.enumerable,
      get: d.get,
      set: function (value) {
        d.set.call(this, typeof value === 'string' ? toServerPath(value) : value);
      }
    });
  }
  wrapSetter(window.HTMLImageElement, 'src');
  wrapSetter(window.HTMLLinkElement, 'href');
  wrapSetter(window.HTMLScriptElement, 'src');
  wrapSetter(window.HTMLSourceElement, 'src');
  wrapSetter(window.HTMLMediaElement, 'src');
  wrapSetter(window.HTMLIFrameElement, 'src');

  // Elements that arrive already built (cloned templates, innerHTML) never
  // call a setter, so their addresses are corrected as they are added.
  function correct(el) {
    if (!RESOURCE.test(el.tagName)) return;
    var name = el.tagName === 'LINK' ? 'href' : 'src';
    var value = el.getAttribute(name);
    var moved = value && toServerPath(value);
    if (moved && moved !== value) setAttribute0.call(el, name, moved);
  }
  new MutationObserver(function (records) {
    for (var i = 0; i < records.length; i++) {
      var added = records[i].addedNodes;
      for (var j = 0; j < added.length; j++) {
        var node = added[j];
        if (node.nodeType !== 1) continue;
        correct(node);
        var inner = node.querySelectorAll('img,link,script,source,video,audio,track,iframe,embed');
        for (var k = 0; k < inner.length; k++) correct(inner[k]);
      }
    }
  }).observe(document, { subtree: true, childList: true });

  // A service worker here would be /service-worker.js on Home Assistant's own
  // origin. Never settle, so a caller without a catch stays quiet.
  if (navigator.serviceWorker) {
    navigator.serviceWorker.register = function () {
      return new Promise(function () {});
    };
  }

  // 3. Leaving the page.

  // A link the app's router handles keeps its address. One the browser follows
  // by itself (a new tab, a download) gets the prefix for that one click.
  function browserFollows(event, a) {
    return event.type !== 'click' || event.button !== 0 ||
      event.ctrlKey || event.metaKey || event.shiftKey || event.altKey ||
      (a.target && a.target !== '_self') || a.hasAttribute('download') ||
      /(^|\s)external(\s|$)/.test(a.getAttribute('rel') || '');
  }
  function onLink(event) {
    var a = event.target && event.target.closest && event.target.closest('a[href]');
    if (!a || !browserFollows(event, a)) return;
    var written = a.getAttribute('href');
    var moved = toServerPath(written);
    if (moved === written) return;
    setAttribute0.call(a, 'href', moved);
    // Long enough for the browser to read it, short enough that the next
    // ordinary click finds the address the router expects.
    setTimeout(function () {
      if (a.getAttribute('href') === moved) setAttribute0.call(a, 'href', written);
    }, event.type === 'contextmenu' ? 300 : 0);
  }
  ['click', 'auxclick', 'contextmenu'].forEach(function (type) {
    document.addEventListener(type, onLink, true);
  });

  // A form the browser posts by itself, such as "Log out". The app's router
  // handles GET forms.
  document.addEventListener('submit', function (event) {
    var form = event.target;
    if (!form || form.method !== 'post') return;
    var written = form.getAttribute('action') || location.pathname + location.search;
    var moved = toServerPath(written);
    if (moved !== written) setAttribute0.call(form, 'action', moved);
  }, true);

  // location.reload(), location.replace('/login?...') and the like would load
  // an address that belongs to Home Assistant. Send them to the prefixed one.
  // Browsers without the Navigation API (Safari before 26.2, Firefox before
  // 147) cannot do this.
  if (window.navigation && navigation.addEventListener) {
    navigation.addEventListener('navigate', function (event) {
      if (event.hashChange || event.downloadRequest != null || event.destination.sameDocument || !event.cancelable) return;
      var moved = prefixed(event.destination.url);
      if (!moved) return;
      event.preventDefault();
      if (event.navigationType === 'push') location.assign(moved.href);
      else location.replace(moved.href);
    });
  }
})();
