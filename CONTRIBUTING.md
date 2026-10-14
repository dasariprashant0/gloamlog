# Contributing to Gloamlog

Thanks for helping. Gloamlog is a small, local-first macOS app and the aim is to keep it small. Bug reports, reproductions and focused pull requests are all welcome.

## Build

You need macOS 13+ and the Xcode Command Line Tools (`xcode-select --install`). You do not need Xcode.

```bash
bash app/build.sh
```

This compiles `app/Core` and `app/UI` with plain `swiftc` and produces `app/Gloamlog.app` (ad-hoc signed) and `app/Gloamlog.zip`. The page editor ships prebuilt in `app/Resources/editor/index.html`; you only need Node if you change `editor-web/` (`cd editor-web && npm ci --ignore-scripts && npm run build`).

## Test

```bash
bash tests/run-tests.sh
```

Run it before every PR. It compiles `app/Core` with `tests/main.swift` and runs assert-based tests. New logic in `app/Core/` should come with a test.

More harnesses live in `app/Tools/` and `tests/`. None of them ship in the app:

| Command | What it checks |
|---|---|
| `bash tests/run-kill-tests.sh` | Crash safety: a child process saves pages while the parent `kill -9`s it, then checks no day file is truncated or lost. |
| `bash app/Tools/editor-check/run.sh` | The real bundled editor running in a real WebKit view, through the Swift bridge. Opens windows, so run it from a normal terminal. |
| `bash app/Tools/snapshot/run.sh` | Renders the SwiftUI views with sample data to PNG (the editor area is a stand-in). |
| `app/Tools/make-icon.sh` | Regenerates the app icon. |

## Project layout

See [docs/FILE_STRUCTURE.md](docs/FILE_STRUCTURE.md). `app/Core` is pure Swift (Foundation only, no UI) and is covered by tests. `app/UI` is SwiftUI and AppKit. The editor contract is in [docs/EDITOR_CONTRACT.md](docs/EDITOR_CONTRACT.md) and the architecture notes are in [docs/v1/ARCHITECTURE.md](docs/v1/ARCHITECTURE.md).

## Constraints, and why

- **Command Line Tools only: no Xcode project, no Swift Package Manager, no Swift macros.** The build is one `swiftc` call so anyone with the Command Line Tools can build and read it. Some SwiftUI features are macros in current SDKs (for example `@State`), and macros need a compiler plugin that ships with Xcode, not with the Command Line Tools. Code using them compiles on a machine with full Xcode and breaks the build for everyone else, and CI runs on a runner with Xcode, so CI will not catch it. Use `ObservableObject` with `@Published` and `@StateObject` instead; `app/UI/Theme.swift` has a small observable box that stands in for `@State`. Check your change with `bash app/build.sh` on a plain Command Line Tools setup if you can.
- **macOS 13+.** Do not use newer APIs without an availability check. The build targets `<arch>-apple-macos13.0`.
- **No network, no telemetry, no crash reporting.** The only network request is the update check the user clicks (`app/UI/Updater.swift`). Pull requests that add other network calls will not be merged.
- **No third-party dependencies** in the app: Apple frameworks and the Swift standard library only. The editor bundle is the one exception and its licences are in `THIRD_PARTY_LICENSES.md`.
- **Notes stay plain markdown**, one file per day, in the user's folder. Do not add a database.
- **Match the surrounding code.** Follow the naming, spacing and comment style of the file you are in, keep changes small, and do not reformat code you are not changing.

## Proposing a feature

Open a [feature request](https://github.com/dasariprashant0/gloamlog/issues/new/choose) before you write code. Describe the problem you have, not only the solution, and say how it fits the constraints above. A small, local, optional feature is much more likely to be accepted than a large one. If it is accepted, say you are working on it so two people do not do the same thing. Small fixes and doc improvements do not need an issue first.

## Pull request checklist

- [ ] `bash app/build.sh` succeeds
- [ ] `bash tests/run-tests.sh` passes
- [ ] New behaviour has a test, or the PR says why not
- [ ] No macros, no new dependencies, no network access
- [ ] Docs updated if behaviour or structure changed
- [ ] `CHANGELOG.md` has an entry for user-visible changes

## Commit style

Short imperative subject, 72 characters or fewer, for example `Fix streak count across skipped days`. [Conventional Commits](https://www.conventionalcommits.org/) prefixes (`feat:`, `fix:`, `docs:`, `test:`, `ci:`, `chore:`) are welcome but not required. Explain the why in the body when it is not obvious. One logical change per commit.

## Reporting bugs

Use the issue templates, and do not paste your personal notes into an issue. For security problems, see [SECURITY.md](SECURITY.md) and do not open a public issue.

By contributing you agree your work is licensed under the [MIT License](LICENSE). Please follow the [Code of Conduct](CODE_OF_CONDUCT.md).
