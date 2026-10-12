// Gloamlog editor: Milkdown Crepe (builder, no LaTeX/AI/top-bar) behind the bridge in docs/EDITOR_CONTRACT.md.
import { CrepeBuilder } from '@milkdown/crepe/builder'
import { blockEdit } from '@milkdown/crepe/feature/block-edit'
import { codeMirror } from '@milkdown/crepe/feature/code-mirror'
import { cursor } from '@milkdown/crepe/feature/cursor'
import { imageBlock } from '@milkdown/crepe/feature/image-block'
import { linkTooltip } from '@milkdown/crepe/feature/link-tooltip'
import { listItem } from '@milkdown/crepe/feature/list-item'
import { placeholder, placeholderConfig } from '@milkdown/crepe/feature/placeholder'
import { table } from '@milkdown/crepe/feature/table'
import { toolbar } from '@milkdown/crepe/feature/toolbar'
import { imageBlockSchema } from '@milkdown/kit/component/image-block'
import {
  editorViewCtx,
  editorViewOptionsCtx,
  parserCtx,
  remarkStringifyOptionsCtx,
  serializerCtx
} from '@milkdown/kit/core'
import { imageSchema, remarkPreserveEmptyLinePlugin } from '@milkdown/kit/preset/commonmark'
import { closeHistory } from '@milkdown/kit/prose/history'
import { InputRule } from '@milkdown/kit/prose/inputrules'
import { Fragment } from '@milkdown/kit/prose/model'
import { Plugin, PluginKey, Selection } from '@milkdown/kit/prose/state'
import { findWrapping } from '@milkdown/kit/prose/transform'
import { indentPlugin } from '@milkdown/kit/plugin/indent'
import { uploadConfig } from '@milkdown/kit/plugin/upload'
import { $inputRule, $prose, insert, replaceAll } from '@milkdown/kit/utils'

import '@milkdown/crepe/theme/common/prosemirror.css'
import '@milkdown/crepe/theme/common/reset.css'
import '@milkdown/crepe/theme/common/block-edit.css'
import '@milkdown/crepe/theme/common/code-mirror.css'
import '@milkdown/crepe/theme/common/cursor.css'
import '@milkdown/crepe/theme/common/image-block.css'
import '@milkdown/crepe/theme/common/link-tooltip.css'
import '@milkdown/crepe/theme/common/list-item.css'
import '@milkdown/crepe/theme/common/placeholder.css'
import '@milkdown/crepe/theme/common/toolbar.css'
import '@milkdown/crepe/theme/common/table.css'
import './theme.css'

// Safari before 16.4 (macOS 13.0 to 13.2) rejects regex lookbehind. Milkdown's "**bold**" and "~~strike~~" input rules and
// remark's e-mail autolinks use it only as a "not right after a word character" guard (esbuild turns those literals into
// new RegExp(string), so the failure would hit at editor start). Where it is missing, drop the guard instead of dying.
try {
  new RegExp('(?<!a)b')
} catch (_) {
  const lookbehind = /\(\?<[=!][^()]*\)/g
  const retry = (args, make) => {
    try {
      return make(args)
    } catch (e) {
      const plain = typeof args[0] === 'string' ? args[0].replace(lookbehind, '') : args[0]
      if (plain === args[0]) throw e
      return make([plain, args[1]])
    }
  }
  window.RegExp = new Proxy(RegExp, {
    construct: (t, args, nt) => retry(args, (a) => Reflect.construct(t, a, nt)),
    apply: (t, self, args) => retry(args, (a) => Reflect.apply(t, self, a))
  })
}

const DEFAULT_PLACEHOLDER = 'Write about your day, or press / for commands'
const CHANGE_DEBOUNCE_MS = 300
const UPLOAD_TIMEOUT_MS = (window.__DL_DEBUG__ && window.__DL_DEBUG__.uploadTimeoutMs) || 60000 // seam: dev harness only
const MAX_IMAGE_BYTES = 20 * 1024 * 1024
const IMAGE_MIME = /^image\/(png|jpe?g|gif|webp)$/i
const ASSET_ORIGIN = 'dlasset://local/'

// ---------------------------------------------------------------- bridge out
const handler = window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.dl
function post(msg) {
  if (handler) handler.postMessage(msg)
  else console.debug('[dl bridge: no host]', msg)
}
function log(level, message) {
  try {
    post({ type: 'log', level, message: String(message) })
  } catch (_) {}
}
let errorCount = 0 // Milkdown reports a node it could not parse with console.error and DROPS it: we watch for that
let firstError = '' // first such message since setMarkdown reset it: the diagnostic in the rawFallback message
for (const level of ['warn', 'error']) {
  const orig = console[level].bind(console)
  console[level] = (...a) => {
    orig(...a)
    const text = a.map((x) => (x instanceof Error ? x.stack || x.message : typeof x === 'string' ? x : safeJson(x))).join(' ')
    if (level === 'error') {
      errorCount++
      if (!firstError) firstError = (a.find((x) => x instanceof Error) || { message: text }).message
    }
    if (a.some((x) => x && x.dlHandled)) return // upload failures were already shown to the user and reported by the host
    log(level, text)
  }
}
function safeJson(x) {
  try {
    return JSON.stringify(x)
  } catch (_) {
    return String(x)
  }
}
window.addEventListener('error', (e) => log('error', `${e.message} @${e.filename}:${e.lineno}`))
window.addEventListener('unhandledrejection', (e) => {
  if (e.reason && e.reason.dlHandled) return e.preventDefault()
  log('error', `unhandledrejection: ${e.reason && (e.reason.stack || e.reason.message || e.reason)}`)
})
document.addEventListener('securitypolicyviolation', (e) =>
  log('warn', `CSP blocked ${e.violatedDirective}: ${e.blockedURI}`)
)

// -------------------------------------------------------------- small helpers
let toastEl
let toastTimer
function toast(text) {
  if (!toastEl) return
  toastEl.textContent = text
  toastEl.dataset.show = 'true'
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => (toastEl.dataset.show = 'false'), 5000)
}

function assetURL(src) {
  if (typeof src !== 'string' || !/^assets\//.test(src)) return src
  return ASSET_ORIGIN + src.split('/').map(encodeURIComponent).join('/')
}

// The link tooltip's "copy" button needs navigator.clipboard, which WebKit only offers in a secure context; a page loaded
// with loadHTMLString may not be one, and a button that does nothing is worse than none.
if (!navigator.clipboard) {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: {
      writeText: (text) =>
        new Promise((resolve, reject) => {
          const area = document.createElement('textarea')
          area.value = text
          area.style.cssText = 'position:fixed;top:0;left:0;opacity:0'
          document.body.append(area)
          area.select()
          const ok = document.execCommand('copy')
          area.remove()
          if (ok) resolve()
          else reject(new Error('copy failed'))
        })
    }
  })
}

// remark escapes every "&" before a letter ("R\&D", "Q\&A"). Only "&name;" / "&#123;" need it, to stay text on reload.
function unescapeAmpersands(md) {
  return md.replace(/(\\+)&/g, (m, slashes, at, all) =>
    slashes.length % 2 && !/^&#?[A-Za-z0-9]+;/.test(all.slice(at + slashes.length)) ? slashes.slice(1) + '&' : m
  )
}

function fileToBase64(file) {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onerror = () => reject(r.error)
    r.onload = () => resolve(String(r.result).slice(String(r.result).indexOf(',') + 1))
    r.readAsDataURL(file)
  })
}

// -------------------------------------------------------------------- uploads
const uploads = new Map()
let uploadSeq = 0
function handled(message) {
  const e = new Error(message)
  e.dlHandled = true
  return e
}
async function uploadImage(file) {
  if (!IMAGE_MIME.test(file.type)) {
    toast('Only PNG, JPEG, GIF and WebP images can be added.')
    throw handled('unsupported image type: ' + file.type)
  }
  if (file.size > MAX_IMAGE_BYTES) {
    toast('Images can be up to 20 MB.')
    throw handled('image too large: ' + file.size)
  }
  const dataBase64 = await fileToBase64(file)
  return new Promise((resolve, reject) => {
    const id = `u${++uploadSeq}-${Math.random().toString(36).slice(2, 8)}`
    const timer = setTimeout(() => {
      uploads.delete(id)
      toast('Adding the image took too long. Try again.')
      reject(handled('upload timed out'))
    }, UPLOAD_TIMEOUT_MS)
    uploads.set(id, { resolve, reject, timer })
    post({ type: 'uploadImage', id, name: file.name || 'image', mime: file.type, dataBase64 })
  })
}

// ------------------------------------------------------------- editor state
let builder
let view
let ready = false
let silent = false // true while WE change the doc (setMarkdown/insertMarkdown): never a "user edit"
let lastSent = ''
let changeTimer = 0
let readOnly = false // what the host asked for
let rawFallback = null // markdown we could not parse without loss: shown read-only, handed back untouched
const queue = []
let queuedMarkdown = ''

// Markdown for the doc, without what is only a placeholder at the top level: empty paragraphs (they would add blank lines,
// "a\n\n\n\nb", and Milkdown keeps one after a list/table/code block) and image blocks nobody filled in (they would be "![]()").
function serialize() {
  return builder.editor.action((ctx) => {
    const { doc } = ctx.get(editorViewCtx).state
    const kept = []
    doc.forEach((n) => {
      const empty = n.type.name === 'paragraph' ? n.content.size === 0 : n.type.name === 'image-block' && !n.attrs.src
      if (!empty) kept.push(n)
    })
    return ctx.get(serializerCtx)(kept.length === doc.childCount ? doc : doc.copy(Fragment.from(kept)))
  })
}
const current = () => (rawFallback !== null ? rawFallback : serialize())
function applyReadOnly() {
  const ro = readOnly || rawFallback !== null
  builder.setReadonly(ro)
  document.documentElement.toggleAttribute('data-readonly', ro)
}
function asCodeBlock(text) {
  const longest = (text.match(/`+/g) || []).reduce((n, run) => Math.max(n, run.length), 2)
  const fence = '`'.repeat(longest + 1)
  return `${fence}text\n${text}\n${fence}\n`
}

// ------------------------------------------------------------------ quick capture
// Same rule as MarkdownBody.normalizeHeading on the Swift side: lowercase; letters, numbers and combining marks stay (variation
// selectors do not), everything else (emoji, punctuation, spaces) is a single space between words.
function normHeading(text) {
  let out = ''
  let gap = false
  for (const ch of String(text)) {
    const keep =
      /[\p{L}\p{N}]/u.test(ch) || (/[\p{Mn}\p{Mc}]/u.test(ch) && !/[\uFE00-\uFE0F\u{E0100}-\u{E01EF}]/u.test(ch))
    if (keep) {
      if (gap && out) out += ' '
      gap = false
      out += ch
    } else gap = true
  }
  return out.toLowerCase()
}
const isEmptyParagraph = (n) => n.type.name === 'paragraph' && n.content.size === 0
const sameItem = (a, b) => a.attrs.checked === b.attrs.checked && a.content.eq(b.content) // text, marks and checkbox; not list attrs

// The first top-level block whose bottom is below the top of the viewport: where the reader is looking.
function firstVisibleBlock() {
  for (const el of view.dom.children) {
    if (el.classList.contains('ProseMirror-widget') || el.classList.contains('prosemirror-virtual-cursor')) continue
    const r = el.getBoundingClientRect()
    if (r.bottom > 0) return { el, top: r.top }
  }
  return null
}

function flushChange() {
  clearTimeout(changeTimer)
  changeTimer = 0
  if (!ready) return
  const markdown = current()
  if (markdown === lastSent) return
  lastSent = markdown
  post({ type: 'change', markdown })
}
function scheduleChange() {
  clearTimeout(changeTimer)
  changeTimer = setTimeout(flushChange, CHANGE_DEBOUNCE_MS)
}
function markSynced(md = current()) {
  clearTimeout(changeTimer)
  changeTimer = 0
  lastSent = md
}

// Notion types "[] " for a to-do; Milkdown only knows "[ ] " after "- ". Both end up as a "- [ ] " item.
const taskInputRule = $inputRule(
  () =>
    new InputRule(/^\[(?<checked>\s|x)?\]\s$/i, (state, match, start, end) => {
      const checked = (match.groups.checked || '').toLowerCase() === 'x'
      const $pos = state.doc.resolve(start)
      for (let d = $pos.depth; d > 0; d--) {
        if ($pos.node(d).type.name !== 'list_item') continue
        if ($pos.node(d).attrs.checked != null) return null
        return state.tr.deleteRange(start, end).setNodeMarkup($pos.before(d), undefined, { ...$pos.node(d).attrs, checked })
      }
      const tr = state.tr.deleteRange(start, end)
      const range = tr.selection.$from.blockRange()
      const wrapping = range && findWrapping(range, state.schema.nodes.list_item, { checked })
      return wrapping ? tr.wrap(range, wrapping) : null
    })
)

const changePlugin = $prose(
  () =>
    new Plugin({
      key: new PluginKey('DL_CHANGE'),
      state: {
        init: () => null,
        apply(tr) {
          if (tr.docChanged && !silent) scheduleChange()
          return null
        }
      }
    })
)

// ------------------------------------------------------------------ public API
const api = {
  setMarkdown(md, opts) {
    const { cursor: where = 'start', focus = false } = opts || {}
    const text = String(md == null ? '' : md)
    const prev = view.state.selection.from
    silent = true
    try {
      const before = errorCount
      let parsed = true
      rawFallback = null
      firstError = ''
      try {
        builder.editor.action(replaceAll(text, true))
      } catch (e) {
        console.error(e)
        parsed = false
      }
      if (!parsed || errorCount > before) {
        // Milkdown drops what it cannot parse. Editing that copy would delete the user's text on the next save.
        rawFallback = text
        const reason = `${parsed ? 'content-dropped' : 'parse-error'}: ${firstError}`.slice(0, 300)
        post({ type: 'rawFallback', reason })
        log('warn', 'markdown could not be parsed without loss; showing the raw text read-only')
        builder.editor.action(replaceAll(asCodeBlock(text), true))
        toast('This day could not be opened for editing, so its raw text is shown. The file is untouched.')
      }
      const { doc } = view.state
      let tr = view.state.tr
      if (where === 'end') tr = tr.setSelection(Selection.atEnd(doc)).scrollIntoView()
      else if (where === 'keep') tr = tr.setSelection(Selection.near(doc.resolve(Math.min(prev, doc.content.size))))
      else tr = tr.setSelection(Selection.atStart(doc)).scrollIntoView()
      view.dispatch(tr)
    } finally {
      silent = false
    }
    applyReadOnly()
    markSynced()
    document.documentElement.setAttribute('data-loaded', '') // lets the placeholder show (see theme.css)
    if (focus) view.focus()
  },
  getMarkdown() {
    return current()
  },
  isRawFallback() {
    return rawFallback !== null
  },
  setTheme(theme) {
    document.documentElement.dataset.theme = theme === 'dark' ? 'dark' : 'light'
  },
  setReadOnly(value) {
    readOnly = !!value
    applyReadOnly()
  },
  focus() {
    view.focus()
  },
  setPlaceholder(text) {
    builder.editor.action((ctx) => ctx.update(placeholderConfig.key, (p) => ({ ...p, text: String(text || '') })))
    silent = true
    try {
      view.dispatch(view.state.tr) // no-op transaction: redraws the decoration
    } finally {
      silent = false
    }
  },
  insertMarkdown(md, where) {
    if (rawFallback !== null) return
    const text = String(md == null ? '' : md)
    silent = true
    try {
      if (where === 'start' || where === 'end') {
        const doc = builder.editor.action((ctx) => ctx.get(parserCtx)(text))
        if (!doc) return
        const { state } = view
        const last = state.doc.lastChild
        const emptyTail = last && last.type.name === 'paragraph' && last.content.size === 0
        const pos = where === 'start' ? 0 : state.doc.content.size - (emptyTail ? last.nodeSize : 0)
        view.dispatch(state.tr.insert(pos, doc.content))
      } else {
        builder.editor.action(insert(text)) // at the cursor
      }
    } finally {
      silent = false
    }
    markSynced()
  },
  // Quick capture: ONE transaction on the live document, answered by an appendResult message (see docs/EDITOR_CONTRACT.md).
  appendToSection(callId, token, heading, markdown, opts) {
    const answer = (result, md) => post({ type: 'appendResult', id: callId, result, markdown: md })
    // The page must still hold the document the host tagged with `token` (the host sets window.__dlDoc in the script that
    // swaps the page), and it must be real text: the raw-text fallback is not a document we may add to.
    if (rawFallback !== null || typeof token !== 'string' || window.__dlDoc !== token) return answer('stale', null)
    const text = String(markdown == null ? '' : markdown)
    const { state } = view
    const { doc, schema } = state

    // the note as a bullet list node (the host sends one list item line); anything else becomes one plain item
    const parsed = builder.editor.action((ctx) => ctx.get(parserCtx)(text))
    let list = parsed && parsed.childCount === 1 && parsed.firstChild.type.name === 'bullet_list' ? parsed.firstChild : null
    if (!list) {
      const plain = text.replace(/^\s*[-*+]\s+/, '').trim()
      if (!plain) return answer('alreadyPresent', current())
      const item = schema.nodes.list_item.create(null, schema.nodes.paragraph.create(null, schema.text(plain)))
      list = schema.nodes.bullet_list.create(null, item)
    }
    const items = []
    list.forEach((n) => items.push(n))

    // the section: first top-level heading of any level that matches; it runs to the next heading of the same or a higher level
    const want = normHeading(heading)
    const pos = []
    doc.forEach((n, p) => pos.push(p))
    let at = -1
    if (want) for (let i = 0; i < doc.childCount && at < 0; i++) if (doc.child(i).type.name === 'heading' && normHeading(doc.child(i).textContent) === want) at = i
    let end = doc.childCount
    if (at >= 0) for (let i = at + 1; i < doc.childCount; i++) if (doc.child(i).type.name === 'heading' && doc.child(i).attrs.level <= doc.child(at).attrs.level) { end = i; break }

    let toAdd = items
    if (opts && opts.unlessPresent && at >= 0) {
      const existing = []
      for (let i = at + 1; i < end; i++) doc.child(i).descendants((n) => { if (n.type.name === 'list_item') existing.push(n) })
      toAdd = items.filter((it) => !existing.some((e) => sameItem(e, it)))
      if (!toAdd.length) return answer('alreadyPresent', current())
    }

    const tr = closeHistory(state.tr) // its own undo step: ⌘Z removes the note and nothing typed before it
    if (at < 0) {
      const title = schema.text(String(heading).replace(/\s+/g, ' ').trim() || 'Jots')
      tr.insert(doc.content.size, [schema.nodes.heading.create({ level: 2 }, title), schema.nodes.bullet_list.create(list.attrs, toAdd)])
    } else {
      // the last real block of the section; empty paragraphs (the editor's open lines) do not count
      let last = end - 1
      while (last > at && isEmptyParagraph(doc.child(last))) last--
      if (last === at) tr.insert(pos[at] + doc.child(at).nodeSize, schema.nodes.bullet_list.create(list.attrs, toAdd))
      else if (doc.child(last).type.name === 'bullet_list') tr.insert(pos[last] + doc.child(last).nodeSize - 1, toAdd) // join the list
      else tr.insert(pos[last] + doc.child(last).nodeSize, schema.nodes.bullet_list.create(list.attrs, toAdd))
    }
    // No selection change, no scrollIntoView: the transaction maps the caret and selection by itself. WebKit has no scroll
    // anchoring, so put the reader's block back where it was if the note was added above it.
    const anchor = firstVisibleBlock()
    silent = true
    try {
      view.dispatch(tr)
      view.dispatch(closeHistory(view.state.tr)) // empty: whatever the user types next starts a new undo step
    } finally {
      silent = false
    }
    if (anchor) {
      const shift = anchor.el.getBoundingClientRect().top - anchor.top
      if (Math.abs(shift) > 0.5) window.scrollBy(0, shift)
    }
    const md = current()
    markSynced(md) // the reply carries the whole text, typed-a-moment-ago included: the host saves it, no `change` for this
    answer('appended', md)
  },
  _resolveUpload(id, path, err) {
    const u = uploads.get(id)
    if (!u) return
    uploads.delete(id)
    clearTimeout(u.timer)
    if (path && !err) return u.resolve(String(path))
    toast(err ? String(err) : 'Could not add the image.')
    u.reject(handled(err || 'upload failed'))
  }
}

// Before `ready` the host may already call us: remember the latest markdown, replay the rest.
window.DailyLogEditor = Object.fromEntries(
  Object.keys(api).map((name) => [
    name,
    (...args) => {
      if (ready) return api[name](...args)
      if (name === 'getMarkdown') return queuedMarkdown
      if (name === 'isRawFallback') return false
      if (name === 'setMarkdown') queuedMarkdown = String(args[0] == null ? '' : args[0])
      queue.push([name, args])
    }
  ])
)

// ------------------------------------------------------- navigation / DnD guards
document.addEventListener(
  'click',
  (e) => {
    const a = e.target instanceof Element ? e.target.closest('a[href]') : null
    if (!a) return
    const inText = !!a.closest('.ProseMirror')
    if (inText && !readOnly && !(e.metaKey || e.ctrlKey)) return // plain click only places the caret
    e.preventDefault()
    e.stopPropagation()
    const url = a.getAttribute('href') // Milkdown already blanks javascript:, data: and file: links
    if (url && !url.startsWith('#')) post({ type: 'openLink', url })
  },
  true
)
document.addEventListener('auxclick', (e) => {
  if (e.target instanceof Element && e.target.closest('a[href]')) e.preventDefault()
}, true)
// A file dropped anywhere the editor does not take it would navigate the web view away.
for (const type of ['dragover', 'drop']) window.addEventListener(type, (e) => e.preventDefault())
// Images that fail to load (remote URLs are blocked by the CSP) get a neutral box instead of a broken icon.
document.addEventListener('error', (e) => e.target instanceof HTMLImageElement && e.target.classList.add('dl-broken'), true)
document.addEventListener('load', (e) => e.target instanceof HTMLImageElement && e.target.classList.remove('dl-broken'), true)
window.addEventListener('pagehide', flushChange)
window.addEventListener('blur', flushChange)

// ------------------------------------------------------------------------ boot
async function boot() {
  const app = document.getElementById('app')
  toastEl = document.createElement('div')
  toastEl.className = 'dl-toast'
  toastEl.setAttribute('role', 'status')
  toastEl.dataset.show = 'false'
  document.body.append(toastEl)

  api.setTheme(matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light')

  builder = new CrepeBuilder({ root: app, defaultValue: '' })
  // Plain markdown out: an empty paragraph must serialize as nothing, not as a lone "<br />".
  await builder.editor.remove([remarkPreserveEmptyLinePlugin.options, remarkPreserveEmptyLinePlugin.plugin])
  // Tab must not type four spaces into text (they would be saved as "&#x20;   text" or turn a paragraph into a code
  // block). List items still indent with Tab (commonmark keymap), code blocks and tables handle Tab themselves.
  await builder.editor.remove(indentPlugin)

  builder.editor
    .config((ctx) => {
      ctx.update(editorViewOptionsCtx, (prev) => ({
        ...prev,
        attributes: {
          ...(prev.attributes || {}),
          spellcheck: 'true',
          role: 'textbox',
          'aria-multiline': 'true',
          'aria-label': 'Daily log'
        },
        // Pasted web content brings <img src="https://...">: it could never load (CSP) and would leave a dead URL in the
        // file. A copied picture arrives as a file instead and goes through the upload bridge.
        transformPastedHTML: (html) => {
          const d = new DOMParser().parseFromString(html, 'text/html')
          d.querySelectorAll('img').forEach((i) => i.remove())
          return d.body.innerHTML
        }
      }))
      // A dropped or pasted image goes between blocks, never in the middle of a sentence.
      ctx.update(uploadConfig.key, (o) => ({
        ...o,
        getInsertPos: (_event, c, pos) => {
          const { doc } = c.get(editorViewCtx).state
          const $p = doc.resolve(pos)
          if ($p.depth === 0) return pos
          return pos - $p.before(1) <= $p.node(1).nodeSize / 2 ? $p.before(1) : $p.after(1)
        }
      }))
      // Plain, portable markdown: "- item" and "---" (Milkdown defaults to "* item" and "***"), and "R&D" instead of "R\&D".
      ctx.update(remarkStringifyOptionsCtx, (o) => {
        const text = o.handlers.text
        return { ...o, bullet: '-', rule: '-', handlers: { ...o.handlers, text: (...a) => unescapeAmpersands(text(...a)) } }
      })
      // Upstream passes `title: null` for an inline image without title and ProseMirror rejects it: the image vanished.
      ctx.update(imageSchema.key, (prev) => (c) => {
        const s = prev(c)
        return {
          ...s,
          parseMarkdown: {
            match: s.parseMarkdown.match,
            runner: (state, node, type) =>
              state.addNode(type, { src: node.url || '', alt: node.alt || '', title: node.title || '' })
          }
        }
      })
      // image-block would store its resize ratio in the alt text (`![1.00](...)`) and drop the real alt.
      ctx.update(imageBlockSchema.key, (prev) => (c) => {
        const s = prev(c)
        return {
          ...s,
          attrs: { ...s.attrs, alt: { default: '', validate: 'string' } },
          parseMarkdown: {
            match: s.parseMarkdown.match,
            runner: (state, node, type) =>
              state.addNode(type, { src: node.url, caption: node.title || '', alt: node.alt || '' })
          },
          toMarkdown: {
            match: s.toMarkdown.match,
            runner: (state, node) => {
              state.openNode('paragraph')
              state.addNode('image', undefined, undefined, {
                title: node.attrs.caption || null,
                url: node.attrs.src,
                alt: node.attrs.alt || ''
              })
              state.closeNode()
            }
          }
        }
      })
    })
    .use(changePlugin)
    .use(taskInputRule)

  builder
    .addFeature(listItem)
    .addFeature(linkTooltip, {
      inputPlaceholder: 'Paste or type a link',
      onCopyLink: () => toast('Link copied')
    })
    .addFeature(cursor)
    .addFeature(imageBlock, {
      onUpload: uploadImage,
      proxyDomURL: assetURL,
      blockUploadButton: 'Choose image',
      blockUploadPlaceholderText: 'or paste or drop an image',
      blockCaptionPlaceholderText: 'Caption',
      inlineUploadButton: 'Choose image',
      inlineUploadPlaceholderText: 'or paste or drop an image'
    })
    .addFeature(blockEdit, {
      blockHandle: { getOffset: () => 4 },
      textGroup: { h4: null, h5: null, h6: null } // the menu offers Text, H1-H3; # to ###### still work
    })
    .addFeature(toolbar)
    .addFeature(placeholder, { text: DEFAULT_PLACEHOLDER, mode: 'block' })
    .addFeature(table)
    .addFeature(codeMirror)

  await builder.create()
  builder.editor.action((ctx) => (view = ctx.get(editorViewCtx)))
  if (window.__DL_DEBUG__) window.__DL_DEBUG__.view = view // test seam: only the dev harness defines __DL_DEBUG__
  markSynced()
  ready = true
  post({ type: 'ready' })
  for (const [name, args] of queue.splice(0)) api[name](...args) // same task: the host reacts to `ready` only after this
}

boot().catch((e) => {
  console.error('editor failed to start', e)
})
