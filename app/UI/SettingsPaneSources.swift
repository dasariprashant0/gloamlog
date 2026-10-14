// SettingsPaneSources.swift - Settings > Sources (M6): the two switches (both off), the heading Insert uses, the folders Git looks in,
// and the project list where each project is ticked by hand. Nothing is read for a project until it is ticked here, and nothing at all
// is read before the first switch is turned on and the consent sheet accepted.
import SwiftUI

struct SourcesSettings: View {
    @ObservedObject var model: AppModel
    @ObservedObject var sources: SourcesController

    init(model: AppModel) {
        self.model = model
        _sources = ObservedObject(wrappedValue: model.sources)
    }

    private var prefs: SourcesPrefs { model.settings.sources }
    private func display(_ s: String) -> String { Redactor.standard.display(s) }

    private func switchBinding(_ kind: SourceKind) -> Binding<Bool> {
        Binding(get: { kind == .claude ? model.settings.sources.claude : model.settings.sources.git }, set: { sources.setSource(kind, on: $0) })
    }
    private var heading: Binding<String> {
        Binding(get: { model.settings.sources.heading }, set: { model.settings.sources.heading = $0 })
    }
    private var consent: Binding<SourceKind?> {
        Binding(get: { sources.consentFor }, set: { if $0 == nil { sources.declineConsent() } })
    }

    var body: some View {
        SettingsPaneFrame(model: model, pane: .sources) {
            Section {
                SettingsStatus(symbol: "lock.shield", "Read-only, on this Mac only, nothing is sent anywhere, secrets are redacted.")
                SettingsRow("Claude Code", help: "Reads the session files Claude Code keeps in ~/.claude, only for the projects you tick below: the first line of your prompts, the files it edited, commits and pull-request links.") {
                    Toggle("Claude Code", isOn: switchBinding(.claude)).labelsHidden().toggleStyle(.switch)
                }
                SettingsRow("Git", help: "Runs git log in the repositories you tick and lists only commits made with your own git email. It never fetches, pushes or changes anything.") {
                    Toggle("Git", isOn: switchBinding(.git)).labelsHidden().toggleStyle(.switch)
                }
                SettingsRow("Every session and tab", help: "Reads all your Claude Code sessions, in every folder and tab, without ticking projects. Use the list below to skip a project.") {
                    Toggle("Every session and tab", isOn: Binding(get: { model.settings.sources.allProjects }, set: { model.settings.sources.allProjects = $0; model.settings.sources.allowed = [] })).labelsHidden().toggleStyle(.switch)
                }
                SettingsRow("Claude writes my log", help: "Lets Claude Code write a short entry (the problem, what it did, what is left) into today's page when a piece of work is done. Step 1: copy the setup command and run it once in a terminal. Step 2: copy the instructions into ~/.claude/CLAUDE.md. Entries land under the Jots heading (Settings > Shortcuts > Jots go under); type \"What I did\" there to keep them with your day.") {
                    HStack { Button("1 Copy setup command") { sources.copySetupCommand() }; Button("2 Copy instructions") { sources.copyClaudeInstructions() } }
                }
                SettingsRow("Add commits for me", help: "Writes today's commits and pull requests into your page by themselves as you work, about every 2 minutes, and right away when a Claude Code session ends (see the hook below). Edit or delete them like any other line.") {
                    Toggle("Add them for me", isOn: Binding(get: { model.settings.sources.autoAdd }, set: { model.settings.sources.autoAdd = $0 })).labelsHidden().toggleStyle(.switch)
                }
                SettingsRow("Instant update", help: "Optional. Add this hook to ~/.claude/settings.json so Gloamlog updates the moment a Claude Code session ends, instead of waiting up to 2 minutes.") {
                    Button("Copy hook") { sources.copyHook() }
                }
                SettingsRow("Insert under", help: "The heading the lines you choose are added under. It is added at the end of the page when the page has none.") {
                    TextField("Worked on", text: heading).textFieldStyle(.roundedBorder).frame(width: 180).multilineTextAlignment(.trailing)
                }
            } header: { Text("Fill in my log from my work").font(Theme.font(13, .semibold)) }

            if prefs.git {
                Section {
                    ForEach(prefs.gitRoots, id: \.self) { r in
                        HStack(spacing: Theme.s3) {
                            Text(display(r)).font(.system(size: 12, design: .monospaced)).foregroundColor(Theme.textPrimary)
                                .lineLimit(1).truncationMode(.middle)
                            Spacer(minLength: Theme.s3)
                            Button("Remove") { sources.removeGitRoot(r) }
                        }.frame(minHeight: SettingsTheme.rowMin)
                    }
                    HStack { Spacer(minLength: 0); Button("Add folder…") { sources.addGitFolder() } }
                    SettingsHelp("Add a repository, or a folder that holds your repositories (for example ~/Projects). Gloamlog looks up to three levels down; you tick the ones to include below.")
                } header: { Text("Git folders").font(Theme.font(13, .semibold)) }
            }

            Section {
                if !prefs.anyOn {
                    SettingsHelp("Turn on Claude Code or Git to choose projects.")
                } else if sources.projectsLoading && sources.projects.isEmpty {
                    HStack(spacing: Theme.s2) { ProgressView().controlSize(.small); Text("Looking for projects…").font(Theme.font(13)).foregroundColor(Theme.textSecondary) }
                } else if sources.projects.isEmpty {
                    SettingsHelp(prefs.git && prefs.gitRoots.isEmpty && !prefs.claude
                                 ? "Add a folder above to see its repositories."
                                 : "No projects found yet. Claude Code projects appear once you have used it on this Mac; for Git, add a folder above.")
                }
                ForEach(sources.projects) { p in projectRow(p) }
                SettingsHelp(prefs.allProjects ? "Every project is read except the ones you switch off here. Gloamlog stores nothing from what it reads: it looks again when you open a day." : "Hidden projects are skipped before any text is read. Nothing is included until you tick it. Gloamlog stores nothing from what it reads: it looks again when you open a day.")
                if let l = sources.lastScanLine, prefs.anyOn { SettingsHelp(l) }
            } header: { Text("Projects").font(Theme.font(13, .semibold)) }
        }
        .onAppear { sources.paneAppeared() }
        .onDisappear { sources.paneDisappeared() }
        .sheet(item: consent) { _ in
            SourcesConsentSheet(onAccept: { sources.acceptConsent() }, onDecline: { sources.declineConsent() })
        }
    }

    private func projectRow(_ p: SourceProject) -> some View {
        var tags = [String]()
        if p.sources.contains("claude") { tags.append("Claude Code") }
        if p.sources.contains("git") { tags.append("Git") }
        let line = display(p.root) + " · " + (tags.isEmpty ? "not found" : tags.joined(separator: ", "))
        return HStack(spacing: Theme.s3) {
            VStack(alignment: .leading, spacing: 1) {
                Text(display(p.name)).font(Theme.font(13)).foregroundColor(Theme.textPrimary).lineLimit(1)
                Text(line).font(Theme.font(12)).foregroundColor(Theme.textSecondary).lineLimit(1).truncationMode(.middle)
            }
            Spacer(minLength: Theme.s3)
            Toggle("Include \(display(p.name))", isOn: Binding(get: { model.settings.sources.isAllowed(p.root) },
                                                              set: { sources.setAllowed(p.root, $0) }))
                .labelsHidden().toggleStyle(.switch)
        }
        .frame(minHeight: SettingsTheme.rowMin)
        .accessibilityElement(children: .combine)
    }
}

/// The first-enable sheet: exactly what is read, and what never is.
struct SourcesConsentSheet: View {
    let onAccept: () -> Void
    let onDecline: () -> Void

    private func point(_ title: String, _ text: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title).font(Theme.font(13, .semibold)).foregroundColor(Theme.textPrimary)
            Text(text).font(Theme.font(13)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: Theme.s4) {
            Text("Let Gloamlog read your Claude Code and Git activity?").font(Theme.font(17, .semibold)).foregroundColor(Theme.textPrimary)
            Text("Read-only. It stays on this Mac and nothing is sent anywhere. Unless you switch on \"Every session and tab\", projects you have not ticked are skipped before any text is read.")
                .font(Theme.font(13)).foregroundColor(Theme.textSecondary).fixedSize(horizontal: false, vertical: true)
            VStack(alignment: .leading, spacing: Theme.s3) {
                point("Claude Code", "The session files in ~/.claude/projects (and $CLAUDE_CONFIG_DIR when it is set). To list your projects it reads the working folder named at the start of each file. For a project you tick it reads the whole file and keeps only: the first line of the prompts you typed, the names of the files Claude edited, git commit messages from its commands, pull-request links from a pull-request command, and when each record was written. Claude's replies, other tool output and file contents are not kept.")
                point("Git", "In the folders you add it looks for repositories, up to three levels down. For the repositories you tick it runs git log and git config user.email to find your own commits: hash, subject and time. It never fetches, pushes or changes a repository.")
                point("Before anything is shown", "Tokens, keys, emails and long secrets are hidden, and your home folder is shown as ~. Nothing is saved: Gloamlog reads again when you open a day. The only thing it writes is lines into your page: the ones you choose to insert, or all of today's when you switch on \"Add them for me\".")
            }
            HStack(spacing: Theme.s3) {
                Spacer()
                Button("Not now") { onDecline() }.keyboardShortcut(.cancelAction)
                Button("Turn on") { onAccept() }.keyboardShortcut(.defaultAction).buttonStyle(PrimaryButtonStyle())
            }
        }
        .padding(Theme.s6)
        .frame(width: 500)
    }
}
