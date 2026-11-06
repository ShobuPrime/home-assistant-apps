// Makes Lemonade's web app work under Home Assistant ingress.
// Runs first among the deferred scripts: after lemond's inline window.api mock,
// before the app's bundles. See lemonade/UPDATE_GUIDE.md.
(function () {
  var match = location.pathname.match(/^\/api\/hassio_ingress\/[^/]+/);
  if (!match) return; // Direct access has no prefix; upstream's defaults are right.

  var prefix = match[0];
  var base = location.origin + prefix;

  // The API client reads this before it has loaded its settings.
  try {
    localStorage.setItem("lemonade_base_url", base);
  } catch (e) {
    // Storage blocked: getSettings below still supplies the base.
  }

  // lemond's mock returns apiUrl: location.origin, which would replace the above.
  var api = window.api;
  if (api && api.getSettings) {
    var getSettings = api.getSettings;
    api.getSettings = function () {
      return getSettings.apply(api, arguments).then(function (settings) {
        return Object.assign({}, settings, { apiUrl: base, baseURL: { value: base } });
      });
    };
  }

  // The client builds socket URLs by assigning url.pathname, which drops the prefix.
  window.WebSocket = new Proxy(window.WebSocket, {
    construct: function (target, args) {
      var url = new URL(args[0], location.href);
      if (url.host === location.host && url.pathname.indexOf(prefix + "/") !== 0) {
        url.pathname = prefix + url.pathname;
        args = [url.href].concat(Array.prototype.slice.call(args, 1));
      }
      return Reflect.construct(target, args);
    },
  });
})();
