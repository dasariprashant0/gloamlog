// Redactor.swift - hides secrets and personal values in text that is about to be shown or inserted (M6). Foundation only.
//
// Transcripts are plain text and can hold anything a command printed, so every string that can reach the screen or a page goes
// through `apply` (ActivityDigest does it for all of them): API keys and tokens (ghp_ gho_ sk- AKIA ...), `Bearer` values,
// private key blocks, `password=` style assignments, emails and long hex or base64 strings become "[redacted]" / "[email]",
// and the home folder becomes "~". It is best effort, which is why only short first lines are ever read; it is idempotent
// (running it twice changes nothing) and leaves ordinary prose and 40-character git hashes alone.
import Foundation

struct Redactor {
    /// What the "N values hidden" line counts. Paths are shortened, not counted.
    enum Kind: String { case token, email }

    private struct Rule {
        let kind: Kind?
        let re: NSRegularExpression
        let template: String
        let accept: ((NSString, NSTextCheckingResult) -> Bool)?
    }
    private let rules: [Rule]
    private let homeRe: NSRegularExpression?
    private let otherHomesRe: NSRegularExpression?
    let ruleCount: Int

    static let standard = Redactor(home: NSHomeDirectory())

    private static let nameWords = #"(?:secret|token|password|passwd|api[_-]?key|apikey|private[_-]?key|credentials?)"#
    private static let name = #"\b((?:[A-Za-z0-9]+[_-])*"# + nameWords + #"(?:[_-][A-Za-z0-9]+)*)"#
    private static let value = #"("[^"\n]*"|'[^'\n]*'|[^\s"',;]+)"#

    init(home: String) {
        func rule(_ kind: Kind?, _ pattern: String, _ template: String, _ options: NSRegularExpression.Options = [],
                  accept: ((NSString, NSTextCheckingResult) -> Bool)? = nil) -> Rule? {
            guard let re = try? NSRegularExpression(pattern: pattern, options: options) else { return nil }
            return Rule(kind: kind, re: re, template: template, accept: accept)
        }
        let notAlready: (NSString, NSTextCheckingResult) -> Bool = { s, m in m.numberOfRanges > 3 ? s.substring(with: m.range(at: 3)) != "[redacted]" : true }
        // A "name: value" pair is prose as often as a secret ("Fix Token: handle expiry"), so the value must look like one.
        let looksSecret: (NSString, NSTextCheckingResult) -> Bool = { s, m in
            let v = s.substring(with: m.range(at: 3)).trimmingCharacters(in: CharacterSet(charactersIn: "\"'"))
            if v == "[redacted]" { return false }
            return v.count >= 20 || v.rangeOfCharacter(from: CharacterSet(charactersIn: "0123456789-_+/=@!$%^&*#")) != nil
        }
        let built = [
            rule(.token, #"-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z0-9 ]*PRIVATE KEY-----"#, "[redacted]"),
            rule(.token, #"-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----[\s\S]*"#, "[redacted]"),                    // header only, the body was cut off
            rule(.token, #"([A-Za-z][A-Za-z0-9+.-]*://)[^\s/@:]+:[^\s/@]+@"#, "$1[redacted]@"),             // scheme://user:password@host
            rule(.token, #"\b(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA|ANVA|AIPA)[A-Z0-9]{16}\b"#, "[redacted]"),    // AWS access key ids
            rule(.token, #"\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})"#, "[redacted]"),  // GitHub
            rule(.token, #"\bxox[abprs]-[A-Za-z0-9-]{10,}"#, "[redacted]"),                                 // Slack
            rule(.token, #"\bsk-[A-Za-z0-9_-]{16,}"#, "[redacted]"),                                        // sk-, sk-ant-, sk-proj-
            rule(.token, #"\b[sr]k_(?:live|test)_[A-Za-z0-9]{10,}"#, "[redacted]"),                         // Stripe
            rule(.token, #"\bAIza[0-9A-Za-z_-]{30,}"#, "[redacted]"),                                       // Google
            rule(.token, #"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}"#, "[redacted]"),   // JWT
            rule(.token, #"\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]{16,}"#, "$1 [redacted]", [.caseInsensitive]),
            rule(.token, Redactor.name + #"(\s*=\s*)"# + Redactor.value, "$1$2[redacted]", [.caseInsensitive], accept: { s, m in
                s.substring(with: m.range(at: 3)) != "[redacted]" }),                                       // PASSWORD=..., api_key = "..."
            rule(.token, Redactor.name + #"(\s*:\s*)"# + Redactor.value, "$1$2[redacted]", [.caseInsensitive], accept: looksSecret),
            rule(.token, #"(--?(?:password|passwd|token|secret|api[_-]?key))(\s+|=)([^\s"']+)"#, "$1$2[redacted]", [.caseInsensitive], accept: notAlready),
            rule(.email, #"[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}"#, "[email]"),
            rule(.token, #"\b[0-9A-Fa-f]{32,}\b"#, "[redacted]", accept: { s, m in m.range.length != 40 }),   // long hex; a 40-character git hash stays
            rule(.token, #"[A-Za-z0-9+/_-]{32,}={0,2}"#, "[redacted]", accept: { s, m in                     // long base64-ish
                let t = s.substring(with: m.range)
                return t.contains(where: { $0.isUppercase }) && t.contains(where: { $0.isLowercase }) && t.contains(where: { $0.isNumber })
                    && t.filter({ $0 == "/" }).count <= 1
            }),
        ].compactMap { $0 }
        rules = built
        ruleCount = built.count
        homeRe = home.count > 1 ? try? NSRegularExpression(pattern: NSRegularExpression.escapedPattern(for: home) + #"(?=$|[\s"'),:;/])"#) : nil
        otherHomesRe = try? NSRegularExpression(pattern: #"/(?:Users|home)/[^/\s"']+/"#)
    }

    /// `s` with every secret and personal value hidden, and how many of each kind were hidden.
    func apply(_ s: String) -> (text: String, hidden: [Kind: Int]) {
        var out = s, hidden = [Kind: Int]()
        for r in rules {
            let ns = out as NSString
            let ms = r.re.matches(in: out, range: NSRange(location: 0, length: ns.length))
            if ms.isEmpty { continue }
            let mutable = NSMutableString(string: out)
            var n = 0
            for m in ms.reversed() {
                if let ok = r.accept, !ok(ns, m) { continue }
                mutable.replaceCharacters(in: m.range, with: r.re.replacementString(for: m, in: out, offset: 0, template: r.template))
                n += 1
            }
            if n > 0 { out = mutable as String; if let k = r.kind { hidden[k, default: 0] += n } }
        }
        return (shortenPaths(out), hidden)
    }

    /// For text that is only displayed (paths and names in Settings): the same hiding, without the counts.
    func display(_ s: String) -> String { apply(s).text }

    /// The home folder becomes `~`; any other `/Users/<name>/` or `/home/<name>/` becomes `~/`.
    func shortenPaths(_ s: String) -> String {
        var out = s
        if let re = homeRe { out = re.stringByReplacingMatches(in: out, range: NSRange(location: 0, length: (out as NSString).length), withTemplate: "~") }
        if let re = otherHomesRe { out = re.stringByReplacingMatches(in: out, range: NSRange(location: 0, length: (out as NSString).length), withTemplate: "~/") }
        return out
    }

    /// "4 values hidden (2 emails, 2 tokens)", or nil when nothing was hidden.
    static func summary(_ hidden: [Kind: Int]) -> String? {
        let n = hidden.values.reduce(0, +)
        guard n > 0 else { return nil }
        var parts = [String]()
        if let e = hidden[.email], e > 0 { parts.append("\(e) email\(e == 1 ? "" : "s")") }
        if let t = hidden[.token], t > 0 { parts.append("\(t) token\(t == 1 ? "" : "s")") }
        return "\(n) value\(n == 1 ? "" : "s") hidden (\(parts.joined(separator: ", ")))"
    }
}
