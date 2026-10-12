// Loads the BUILT editor (app/Resources/editor/index.html, untouched: same inline script, same CSP) into an iframe
// and plays the Swift host: a mock window.webkit.messageHandlers.dl that records every message.
// The mock is injected BEFORE the CSP <meta>, exactly like WebKit provides the handler before the page script runs.
export const BUNDLE_URL = '/app/Resources/editor/index.html'

export async function mountEditor(container, { onMessage = () => {}, upload = () => 'ok', debug = {}, onApi = null } = {}) {
  const html = await (await fetch(BUNDLE_URL, { cache: 'no-store' })).text()
  const marker = '<meta http-equiv="Content-Security-Policy"'
  if (!html.includes(marker)) throw new Error('CSP meta not found in built index.html')

  const host = {
    messages: [],
    violations: [],
    ready: null,
    iframe: document.createElement('iframe'),
    get win() {
      return this.iframe.contentWindow
    },
    get api() {
      return this.win.DailyLogEditor
    }
  }

  // Called from inside the iframe (same origin). JSON round trip = what WKWebView's structured clone would accept.
  const handle = (raw) => {
    const msg = JSON.parse(JSON.stringify(raw))
    msg.at = performance.now()
    host.messages.push(msg)
    onMessage(msg)
    if (msg.type === 'uploadImage') {
      const mode = upload(msg)
      if (mode === 'ok') setTimeout(() => host.api._resolveUpload(msg.id, 'assets/mock.png', null), 40)
      else if (mode === 'error') setTimeout(() => host.api._resolveUpload(msg.id, null, 'Mock host says: could not save the image'), 40)
      // 'never': no answer, the editor must time out by itself
    }
  }
  host.onApi = onApi // called synchronously the moment the bundle assigns window.DailyLogEditor, i.e. before the editor is ready
  host.handle = handle
  host.violate = (v) => host.violations.push(v)
  const idx = (window.__dlHosts = window.__dlHosts || []).push(host) - 1 // one mock bridge per editor iframe

  const mock =
    '<script>' +
    'window.__DL_DEBUG__=' + JSON.stringify(debug) + ';' + // test seams read by the bundle (view, uploadTimeoutMs)
    // simulate Safari < 16.4, where a regex with lookbehind throws when it is constructed
    (debug.noLookbehind ? '(function(){var B=/\\(\\?<[=!]/,fail=function(a){if(typeof a[0]==="string"&&B.test(a[0])&&a[0]!==' + JSON.stringify(debug.noLookbehind === 'blind' ? '(?<!a)b' : '') + ')throw new SyntaxError("Invalid regular expression (simulated old Safari)")};window.RegExp=new Proxy(RegExp,{construct:function(t,a,n){fail(a);return Reflect.construct(t,a,n)},apply:function(t,s,a){fail(a);return Reflect.apply(t,s,a)}})})();' : '') +
    // tell the harness the instant the bundle publishes its API (a poll would be throttled in a hidden tab and miss the pre-ready window)
    'var D;Object.defineProperty(window,"DailyLogEditor",{configurable:true,enumerable:true,get:function(){return D},set:function(v){D=v;var h=parent.__dlHosts[' + idx + '];if(h.onApi)h.onApi(v)}});' +
    (debug.noClipboard ? 'delete Navigator.prototype.clipboard;' : '') + // a non-secure page, like loadHTMLString may give
    // A hidden browser tab throttles requestAnimationFrame to ~1 Hz; a visible WKWebView runs it at 60 Hz. Crepe moves the
    // caret in a rAF after a list item mounts, so keep rAF timely here or fast scripted typing lands in the wrong place.
    'window.requestAnimationFrame=function(f){return setTimeout(function(){f(performance.now())},16)};' +
    'window.cancelAnimationFrame=function(i){clearTimeout(i)};' +
    'window.webkit={messageHandlers:{dl:{postMessage:function(m){parent.__dlHosts[' + idx + '].handle(m)}}}};' +
    'document.addEventListener("securitypolicyviolation",function(e){parent.__dlHosts[' + idx + '].violate(e.violatedDirective+" "+e.blockedURI)});' +
    '</script>\n'
  host.iframe.srcdoc = html.replace(marker, mock + marker)
  host.iframe.title = 'Gloamlog editor'
  host.ready = new Promise((resolve) => {
    const t = setInterval(() => {
      if (host.messages.some((m) => m.type === 'ready')) {
        clearInterval(t)
        resolve(host)
      }
    }, 20)
  })
  container.append(host.iframe)
  return host
}

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
