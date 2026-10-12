# Editor contract (v0.3)

Single source of truth shared by the web-editor bundle (`editor-web/`), the Swift core (`app/Core/`) and the Swift UI (`app/UI/`). Change it here first.

## Why
v0.2's five fixed boxes felt cluttered and could not hold headings, sizes, lists or images. v0.3 replaces them with one free-form, Notion-style page per day. The five prompts become an optional, editable **template** of headings.

## Day file (`<storage>/YYYY-MM-DD.md`)
- Plain markdown. The app writes `# YYYY-MM-DD`, a blank line, then the body. On load, a leading `# <date>` line is stripped; the editor never shows it.
- v0.1/v0.2 files (`## 📝 What I did` ... ) load unchanged: they are just markdown headings.
- Skipped day: unchanged (`> Skipped: reason` marker file, no body).
- Images: `<storage>/assets/<YYYY-MM-DD>-<8hex>.<ext>`; markdown references them **relative to the storage folder**: `![](assets/2026-10-06-1a2b3c4d.png)`. Allowed ext: png, jpg, jpeg, gif, webp. Max 20 MB each.
- Autosave: debounced ~600 ms after an editor `change`. Never create a file just because a day was opened; write only after a real edit. No Save button, no drafts folder.
- Logged rule: `words(body) >= settings.minWords` (default 20). Word count ignores heading lines, empty list/task markers, code-fence lines, image syntax, HTML comments. `partial` = 1...minWords-1 words. Streak/heatmap/reminder planner use this rule.

## Settings changes
Replace `sections` with: `template: String` (markdown, default below), `carryOverHeadings: [String]` (default `["To do next", "Pending / blocked"]`), `minWords: Int` (default 20, range 1...500). Decode old settings tolerantly: if an old `sections` array exists and no `template`, build the template from the old section titles as `## <title>` blocks.

Default template:
```
## What I did

## Finished

## Started

## Pending / blocked

## To do next
```
A new day is pre-filled with the template in the editor only; it is not written until the user edits.

## Core API (Swift, Foundation only)
- `DayDocument { day, body, isSkipped, skipReason, words, status(minWords:) }`.
- `LogStore`: `load`, `save(day, body)`, `skip/unskip/skipRange`, `listDays`, `fileStates`, `search(query)` -> hits with `day`, nearest preceding `heading?`, `snippet`, `matchRange`. Typed `LogError` as before.
- `AssetStore(dir:)`: `save(data:, ext:, day:) throws -> String` (relative path, content-hash name, validates ext/size) and `resolve(relativePath) -> URL?` (must stay inside the storage dir; reject `..`, absolute paths, non-image ext).
- `MarkdownBody`: `words(in:)`, `sections(of:)` (heading level, normalised title, text), `normalizeHeading` (lowercase, strip emoji/punctuation), `insertCarryOver`.
- Carry-over: take non-empty lines from the previous *logged* day's `carryOverHeadings` sections, strip list/checkbox markers, insert at the TOP of today's body as `## Carried over from <Mon 5 Oct>` + `- [ ] item` lines; idempotent (do not insert twice).
- Weekly review: per day = the body; by section = gather text under headings matching the template headings across the week, plus 'Other notes'. `markdown(for:grouping:)` stays.
- Remove DraftStore and the `## ` escaping. Keep ReminderPlanner, Streak, Heatmap behaviour, now driven by the logged rule.

## Bridge: JS -> Swift
`window.webkit.messageHandlers.dl.postMessage(obj)`:
- `{type:"ready"}` once the editor is mounted and the API below is callable.
- `{type:"change", markdown}` debounced 300 ms after USER edits only (never after programmatic `setMarkdown`).
- `{type:"uploadImage", id, name, mime, dataBase64}`; Swift answers with `DailyLogEditor._resolveUpload(id, relativePathOrNull, errorMessageOrNull)`.
- `{type:"openLink", url}`; Swift opens only http/https/mailto in the default browser.
- `{type:"log", level, message}` for debugging.
- `{type:"rawFallback", reason}` once per `setMarkdown` call whose text could not be parsed without loss: the page then shows the raw text read-only (see "Safety net" below); `reason` is diagnostic text starting `content-dropped` or `parse-error`.
- `{type:"appendResult", id, result, markdown}`: the one answer to each `appendToSection` call. `id` is the caller's `callId`, `result` is `"appended"`, `"alreadyPresent"` or `"stale"`, and `markdown` is the editor's normalised markdown of the whole page (for `appended` and `alreadyPresent`), or `null` for `stale` (never take text from a stale answer).

## Bridge: Swift -> JS (`window.DailyLogEditor`)
`setMarkdown(md, {cursor: "start"|"end"|"keep", focus})`, `getMarkdown()`, `isRawFallback()` (true while the raw-text fallback is showing), `setTheme("light"|"dark")`, `setReadOnly(bool)`, `focus()`, `setPlaceholder(text)`, `insertMarkdown(md, where)` (`"cursor"|"end"|"start"`), `appendToSection(callId, token, heading, markdown, {unlessPresent})`, `_resolveUpload(id, path, err)`.
Programmatic calls must not emit `change`. After first normalisation the markdown round trip must be idempotent (`serialize(parse(x)) == x`) so Swift never loops on its own output.

**`appendToSection`** (quick capture; the only call that edits the live page and keeps what the user is doing). It returns nothing; the answer is one `appendResult` message with the same `callId`. A call made before `ready` is queued and answered right after `ready`.
- `token` is compared with the string `window.__dlDoc`, which Swift sets in the same script that swaps the page (`EditorBridge.swapDocument`). If it differs, is not a string, the page was never tagged, or the raw-text fallback is showing, the answer is `stale` (`markdown` null) and nothing changes.
- `heading` is matched like `MarkdownBody.normalizeHeading` (lowercase; letters, numbers and combining marks only, everything else, emoji and punctuation included, is a single space). The first top-level heading of ANY level that matches is the section; it runs to the next heading of the same or a higher level, so deeper sub-headings stay inside it.
- `markdown` is one list item as `CapturePlacement.render` makes it (`- 14:32 text`, `- [ ] text`, continuation lines indented 2 spaces). It goes at the end of the section as the last item of the section's last block when that block is a bullet list (the editor's empty open lines do not count), otherwise as a new bullet list after the last block (right under the heading when the section is empty). When no heading matches, `## <heading>` (level 2, the text as given) and the list are added at the very end of the page; an empty page gets exactly those.
- `unlessPresent: true` answers `alreadyPresent`, changing nothing, when an item with the same text, formatting and checkbox state is already in the section; items elsewhere do not count. A to-do that was ticked meanwhile is a different item.
- It is ONE ProseMirror transaction on the live document (plus an empty one that closes the undo step). The caret and any selection are mapped, never set; the page does not scroll (the reader's block is kept in place when the note lands above it); the note is its own undo step, so ⌘Z removes only the note, and text typed just before or just after is separate steps; a note that created the heading is removed together with it in one undo.
- It never sends `change`. The reply's `markdown` is the whole normalised page, including text typed a moment ago, so Swift adopts it and schedules the save itself. On `appended` the editor drops its pending 300 ms change timer (that text is in the reply); on `alreadyPresent` and `stale` it leaves it alone, and the pending `change` is still sent.

## Assets and privacy
- Swift serves images with a `WKURLSchemeHandler` for `dlasset://local/<relative path>` (path segments are percent-encoded; decode, resolve through `AssetStore.resolve`, answer with the bytes and the right `Content-Type`). The editor maps `assets/...` to that scheme for display only (image-block `proxyDomURL`); the markdown keeps the relative path.
- **One self-contained page**: `app/Resources/editor/index.html` with the CSS and the JS inline (no `editor.js`, `editor.css` or fonts). Decision: a CSP `'self'` is unreliable for `loadHTMLString` and custom schemes, so the build computes the sha256 of the inline script and writes it into the CSP `<meta>`: `default-src 'none'; script-src 'sha256-<hash, set by the build>'; style-src 'unsafe-inline'; img-src dlasset: data: blob:; font-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'` (replaces the earlier `'self'` rules; `'unsafe-inline'` covers the inline `<style>` and the style attributes ProseMirror sets). Swift loads it with `loadHTMLString(html, baseURL:)`; the base URL has no effect on the CSP (nil or `dlasset://local/` both work).
- Remote images do not load: the editor shows a neutral "Image unavailable" box, and HTML pasted from the web has its `<img>` removed. Links never navigate the web view: the editor posts `openLink` (⌘-click in the text, any click when read-only, the URL in the link tooltip).
- No telemetry, no fonts or scripts from a CDN, everything bundled. The built page contains no `fetch`, `XMLHttpRequest`, `WebSocket`, `sendBeacon`, worker, `eval` or dynamic `import()`; the only URL strings in it are XML namespaces and two documentation links inside error messages.
- The host must: register the `dlasset` handler; set `drawsBackground = false` so the transparent page shows the window; treat **every** `ready` as "(re)load state" (`setMarkdown`, `setTheme`, `setPlaceholder`, `setReadOnly`), because a reload of the page sends `ready` again; make the web view first responder before `focus()`; call `getMarkdown()` before switching day or quitting (`change` trails the last key by up to 300 ms; it is also flushed on blur); open only http/https/mailto from `openLink`; offer a file picker for "Choose image" (`webView(_:runOpenPanelWith:...)`); remove "Reload" from the web view's context menu.

## Editor bundle (`editor-web/` -> `app/Resources/editor/index.html`)
- Milkdown Crepe 7.22.2 (MIT) through `CrepeBuilder`, with: slash menu and block drag handle, selection toolbar, lists + task lists, headings, quote, divider, code block (CodeMirror, no language packs, so no highlighting), table, link tooltip, image block (upload via the bridge, paste and drop), placeholder, cursor. Not bundled: LaTeX/KaTeX, the AI feature, the top bar. Real size about 1.1 MB (limit ~3.5 MB).
- Build: `cd editor-web && npm ci && npm run build` (esbuild only; `.npmrc` sets `ignore-scripts=true`). Exact versions are pinned and `package-lock.json` is committed. The single built file is committed too, so contributors do not need Node to build the Mac app. `npm run licenses` regenerates `THIRD_PARTY_LICENSES.md` (repo root) from what is really in the bundle. JS is built for Safari 16 (macOS 13); a small shim covers the missing regex lookbehind of Safari before 16.4 (macOS 13.0 to 13.2; only simulated in the test page with `?oldsafari`), and a few Crepe decorations use `color-mix()` (Safari 16.2).
- Dev and tests: `node editor-web/dev/serve.mjs`, then `/editor-web/dev/` (harness with a mock `window.webkit.messageHandlers.dl`) and `/editor-web/test/` (fixture round trips, bridge and behaviour checks, prints PASS/FAIL).
- Style: system font (SF Pro via `-apple-system`), 16 px body, 1.65 line height, accent from `docs/DESIGN_SYSTEM.md` (light `#0E7A5F`, dark `#45C29C`), transparent page background, no chrome of its own (the app provides title/date). Light and dark through `setTheme`. The text column matches the host layout: 720 pt wide, 48 pt side gutter (the block handle lives in the left gutter), so give the web view the full detail-pane width.
- Markdown it writes (Swift can rely on this): ATX headings, `-` bullets, `1.` lists, `- [ ]`/`- [x]` tasks, `---` rules, fenced code, `**bold**` `*italic*` `~~strike~~`, `![alt](assets/x.png "caption")` with no size data, GFM tables with padded columns, a hard break is `\` + newline. Empty paragraphs and unfilled image blocks are never written (no blank-line runs, no `<br />`, no `![]()`). Markdown punctuation in prose is backslash-escaped where CommonMark needs it (`2 \* 3`, `\_private`, `\[bug]`, `\~`, a literal `\&amp;`, and `?a=1\&b=2` inside a link target; plain `R&D` is left alone): search must ignore backslash escapes. A leading space is written as `&#x20;`. Two adjacent lists alternate the bullet (`-` then `*`) so they stay two lists. Setext headings, `*`/`+` bullets, `~~~` fences, indented code, reference links and bare URLs are normalised once on first load, then stable.
- Safety net: if Milkdown drops anything while parsing a day (or throws), the editor shows the raw text read-only and `getMarkdown()` returns the original text unchanged. It posts `{type:"rawFallback", reason}` once per `setMarkdown` call (`reason` is diagnostic text starting `content-dropped` or `parse-error`; a `log` warn goes with it), and `DailyLogEditor.isRawFallback()` returns `true` until the next `setMarkdown`; `setReadOnly(false)` cannot unlock it. A `setMarkdown` made before `ready` is replayed right after `ready`, so its `rawFallback` also arrives after `ready`. It never edits a lossy copy.

## Acceptance
- Typing `# `, `## `, `- `, `1. `, `[] `, `> `, ``` and `/` behaves like Notion; images can be pasted and dropped; the saved markdown is plain and portable.
- Old v0.2 files open correctly. Round trip is idempotent on the fixtures in `editor-web/test/`.
- Nothing in the editor makes a network request.
