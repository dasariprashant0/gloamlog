// EditorBridge+Capture.swift - the one editor-contract call quick capture needs: DailyLogEditor.appendToSection (M2, task U3).
//
// Swift calls   DailyLogEditor.appendToSection(callId, token, heading, markdown, {unlessPresent: true})
// The page answers with a bridge message   {type: "appendResult", id: callId, result: "appended" | "alreadyPresent" | "stale", markdown}
// (docs/EDITOR_CONTRACT.md, Bridge). The call is ONE ProseMirror transaction on the live document: the user's caret, selection,
// scroll position and undo history are untouched and no `change` event is emitted, so the model schedules the save itself.
//
// Rules this file keeps:
//  * The token is checked here, in the same script that makes the call (window.__dlDoc), so a note can never land in another
//    day's page; a mismatch is reported as `.stale` and changes nothing.
//  * Every call has a deadline: when the page never answers (it died, or hangs), the call ends as `.failed` and the caller keeps
//    the note in its journal. A late answer is ignored; the next attempt is idempotent because of `unlessPresent`.
//  * The note's text only travels as a JSON literal (JS.literal), never concatenated into a script.
import WebKit
import AppKit

enum AppendOutcome: Equatable {
    case appended(String)        // the page's normalised markdown after the change
    case alreadyPresent
    case stale                   // the page holds another document now
    case failed                  // not ready, no such call in this bundle, no answer in time
}

extension EditorBridge {
    /// How long a call may wait for the page's answer.
    static let appendDeadline: TimeInterval = 5

    func appendToSection(token: String, heading: String, markdown: String, unlessPresent: Bool,
                         completion: @escaping (AppendOutcome) -> Void) {
        guard isReady else { completion(.failed); return }
        let callId = UUID().uuidString
        pendingAppends[callId] = completion
        let script = """
        (function () {
          var E = window.DailyLogEditor;
          if (window.__dlDoc !== \(JS.literal(token))) return 'stale';
          if (!E || typeof E.appendToSection !== 'function') return 'unsupported';
          E.appendToSection(\(JS.literal(callId)), \(JS.literal(token)), \(JS.literal(heading)), \(JS.literal(markdown)), {unlessPresent: \(unlessPresent ? "true" : "false")});
          return 'sent';
        })()
        """
        run(script) { [weak self] result in
            guard let self = self, self.pendingAppends[callId] != nil else { return }
            switch result as? String {
            case "sent": break                                  // the answer comes as an `appendResult` message
            case "stale": self.finishAppend(callId, .stale)
            default: self.finishAppend(callId, .failed)         // script error, or a bundle without the call
            }
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + Self.appendDeadline) { [weak self] in self?.finishAppend(callId, .failed) }
    }

    /// The `appendResult` bridge message.
    func handleAppendResult(_ m: [String: Any]) {
        guard let id = m["id"] as? String, pendingAppends[id] != nil else { return }
        switch m["result"] as? String {
        case "appended":
            if let md = m["markdown"] as? String { finishAppend(id, .appended(md)) }
            else { finishAppend(id, .failed) }                  // no text to adopt: the caller retries (idempotent)
        case "alreadyPresent": finishAppend(id, .alreadyPresent)
        case "stale": finishAppend(id, .stale)
        default: finishAppend(id, .failed)
        }
    }

    /// Completes a call exactly once.
    func finishAppend(_ id: String, _ outcome: AppendOutcome) {
        guard let done = pendingAppends.removeValue(forKey: id) else { return }
        done(outcome)
    }

    /// The page went away (the web process died, or the page reloaded): nothing in flight will be answered.
    func failPendingAppends() {
        for id in Array(pendingAppends.keys) { finishAppend(id, .failed) }
    }
}
