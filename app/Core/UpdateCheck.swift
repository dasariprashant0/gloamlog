// UpdateCheck.swift - reads GitHub's "latest release" answer and compares versions. Foundation only, no network here (the UI does that).
// The release must carry two assets: Gloamlog.zip and Gloamlog.zip.sha256 (`shasum -a 256` output). Without the checksum file the
// app refuses to install by itself and only opens the release page.
import Foundation

struct ReleaseInfo: Equatable {
    var version: String          // "0.6.1" (a leading "v" removed)
    var notes: String
    var pageURL: URL
    var zipURL: URL
    var shaURL: URL?
}

enum UpdateCheck {
    static let repo = "dasariprashant0/gloamlog"
    static var latestURL: URL { URL(string: "https://api.github.com/repos/\(repo)/releases/latest")! }

    static func parse(_ data: Data) -> ReleaseInfo? {
        guard let o = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let tag = o["tag_name"] as? String, !(o["draft"] as? Bool ?? false), !(o["prerelease"] as? Bool ?? false),
              let page = (o["html_url"] as? String).flatMap(URL.init(string:)) else { return nil }
        let assets = (o["assets"] as? [[String: Any]]) ?? []
        func url(_ name: String) -> URL? {
            assets.first { ($0["name"] as? String) == name }.flatMap { $0["browser_download_url"] as? String }.flatMap(URL.init(string:))
        }
        guard let zip = url("Gloamlog.zip") else { return nil }
        return ReleaseInfo(version: tag.hasPrefix("v") ? String(tag.dropFirst()) : tag, notes: (o["body"] as? String) ?? "",
                           pageURL: page, zipURL: zip, shaURL: url("Gloamlog.zip.sha256"))
    }

    /// Numeric dotted versions ("0.6.10" > "0.6.9"); anything that is not numbers counts as 0.
    static func isNewer(_ remote: String, than local: String) -> Bool {
        func parts(_ s: String) -> [Int] { s.split(separator: ".").map { Int($0.prefix(while: { $0.isNumber })) ?? 0 } }
        let a = parts(remote), b = parts(local)
        for i in 0..<max(a.count, b.count) {
            let x = i < a.count ? a[i] : 0, y = i < b.count ? b[i] : 0
            if x != y { return x > y }
        }
        return false
    }

    /// The hash in a `shasum -a 256` line ("<64 hex>  Gloamlog.zip"), lower-cased; nil when it is not one.
    static func checksum(from text: String) -> String? {
        guard let first = text.split(whereSeparator: { $0 == " " || $0 == "\n" || $0 == "\t" }).first, first.count == 64,
              first.allSatisfy({ $0.isHexDigit }) else { return nil }
        return first.lowercased()
    }

    /// Only GitHub may serve the download.
    static func isTrusted(_ url: URL) -> Bool {
        url.scheme == "https" && (url.host == "github.com" || url.host?.hasSuffix(".github.com") == true)
    }
}
