# File structure

```
gloamlog/
├── README.md  CHANGELOG.md  LICENSE  CONTRIBUTING.md  CODE_OF_CONDUCT.md  SECURITY.md  THIRD_PARTY_LICENSES.md
├── app/
│   ├── build.sh              swiftc over Core + UI -> Gloamlog.app (ad-hoc signed) + Gloamlog.zip; copies the icon and the editor bundle
│   ├── Core/                 pure Swift (Foundation only), no UI; every file here is covered by tests/
│   │   ├── Calendar/         MonthGrid, DateJump (the "Go to date" parser), CatchUp (+ batch skip/undo)
│   │   ├── Capture/          quick capture ("Jot"): types, placement, journal, DayWriter, Jots rules
│   │   ├── Review/ Export/ Schedule/   reviews and exports, and the closed-app reminder plan (as they land)
│   │   └── LogStore, Settings, MarkdownBody, Streak, WeeklyReview, CarryOver, AssetStore, AtomicFile, ReminderPlanner, ...
│   ├── UI/                   SwiftUI + AppKit: app entry, AppModel (+ extensions), sidebar, page, catch-up, review, Settings window,
│   │                         menu-bar item, editor host (WKWebView bridge), capture panel
│   ├── Resources/            AppIcon.icns, editor/index.html (the prebuilt Notion-style editor bundle)
│   └── Tools/                editor-check (real windows + real WebKit), kill-test (crash safety), snapshot, make-icon
├── editor-web/               source of the editor bundle (Milkdown Crepe, esbuild); `npm run build` writes app/Resources/editor/index.html
├── tests/                    main.swift (assert-based core tests), run-tests.sh, run-kill-tests.sh
├── docs/
│   ├── STRATEGY.md           product direction and launch plan
│   ├── EDITOR_CONTRACT.md    the bridge between the Swift app and the editor bundle
│   ├── v1/                   the current plans: PRD_V1, UX_FLOWS, DESIGN_V4, TRD, ARCHITECTURE, M1/M2 build plans, evidence/
│   ├── research/             market, growth, SEO and Claude-integration research
│   └── ABOUT.md  EXPLANATION.md  FILE_STRUCTURE.md  (older v0.1/v0.2 docs in this folder are superseded by v1/)
└── .github/                  CI (build + tests), release workflow, issue and PR templates
```

Generated, not committed: `app/Gloamlog.app/`, `app/Gloamlog.zip`, `editor-web/node_modules/`.
Runtime data, outside the repo: `~/Gloamlog/YYYY-MM-DD.md` and `~/Gloamlog/assets/`; safety copies under `~/Library/Application Support/Gloamlog/`.

Build and test: `bash app/build.sh`, `bash tests/run-tests.sh`, `bash tests/run-kill-tests.sh`, `bash app/Tools/editor-check/run.sh` (run from a normal terminal: it opens real windows).
