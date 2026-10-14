# Security Policy

## Supported versions

Gloamlog is alpha software. Only the latest release receives fixes.

| Version | Supported |
| ------- | --------- |
| Latest release | Yes |
| Older releases | No |

## Reporting a vulnerability

Please do not open a public issue or post details in a discussion. Report it privately through GitHub security advisories:

1. Go to the repository's **Security** tab and choose **Report a vulnerability**, or open <https://github.com/dasariprashant0/gloamlog/security/advisories/new>.
2. Include the Gloamlog version, your macOS version, steps to reproduce and the impact you expect. Do not include your personal notes.

This is a one-person project. I will reply as soon as I can, usually within a week.

## What is in scope

- The app, including the quick-capture journal, the Claude Code inbox and the `gloamlog://` URL handler.
- The Claude Code helper (`app/Resources/claude/gloamlog-mcp.py`) and the changes "Connect Claude Code" makes to `~/.claude/CLAUDE.md`.
- The update check and installer: `Check for Updates…` and `install.sh`, including checksum handling.
- Anything that could make the app read, write or send data it says it does not.

## Security model, briefly

- Gloamlog is local-only. Its one network request is the update check you click, to `api.github.com` and `github.com`. See [PRIVACY.md](PRIVACY.md).
- Updates and the installer verify the release zip against the SHA-256 published in the same GitHub release. That protects against corrupted downloads, not against a compromised release.
- Release builds are ad-hoc signed and **not notarised**, so macOS shows a warning on the first launch of a browser-downloaded copy. See [docs/INSTALL.md](docs/INSTALL.md). Reports that only say "it is not notarised" are a known, deliberate limitation and not a vulnerability. You can always build from source with `bash app/build.sh`.
- Sources (reading Claude Code sessions and Git) is off by default, read-only, and redacts secrets before showing anything. A redaction miss is in scope.
- The app can register as a login item so reminders can fire. It is opt-in; remove it in System Settings > General > Login Items.
