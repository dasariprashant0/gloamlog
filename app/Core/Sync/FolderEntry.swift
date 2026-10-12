// FolderEntry.swift - what a name in the log folder is, as far as sync is concerned: a day file, an iCloud placeholder, a
// conflict copy, or nothing of ours. Pure (names in, a verdict out, no disk). Precision over recall: only the known
// providers' conflict names count; a name that merely starts like a day ("2026-10-08 standup.md") is left alone.
//
//   2026-10-08.md                                              .day
//   .2026-10-08.md.icloud   .2026-10-08 2.md.icloud            .placeholder (iCloud has the file, not its bytes yet; `name` is the real file's)
//   2026-10-08 2.md   2026-10-08 3.md                          .conflict, iCloud / Finder numbering (2 or more)
//   2026-10-08 (conflicted copy).md                            .conflict, Dropbox
//   2026-10-08 (Alex's MacBook's conflicted copy 2026-10-09).md  .conflict, Dropbox
//   2026-10-08.sync-conflict-20261009-101530-ABCDEFG.md        .conflict, Syncthing
//   notes.md, 2026-13-40.md, .DS_Store, .gloamlog-tmp-*, assets   .ignored
import Foundation

enum ConflictFamily: Equatable { case iCloudNumbered, dropbox, syncthing }

enum FolderEntry: Equatable {
    case day(String)
    case conflict(day: String, family: ConflictFamily)
    case placeholder(name: String, day: String, isConflictCopy: Bool)
    case ignored
}

enum FolderEntryClassifier {
    static func classify(_ name: String) -> FolderEntry {
        if name.hasPrefix(".") {
            guard name.hasSuffix(".icloud"), name.utf8.count > 8 else { return .ignored }
            let real = String(name.dropFirst().dropLast(".icloud".count))
            switch visible(real) {
            case .day(let d): return .placeholder(name: real, day: d, isConflictCopy: false)
            case .conflict(let d, _): return .placeholder(name: real, day: d, isConflictCopy: true)
            default: return .ignored
            }
        }
        return visible(name)
    }

    private static func visible(_ name: String) -> FolderEntry {
        guard name.hasSuffix(".md"), name.utf8.count >= 13 else { return .ignored }
        let bytes = Array(name.utf8.dropLast(3))
        let day = String(decoding: bytes[0..<10], as: UTF8.self)
        guard isPlausibleDay(day) else { return .ignored }
        let rest = String(decoding: bytes[10...], as: UTF8.self)
        if rest.isEmpty { return .day(day) }
        if let family = conflictFamily(rest) { return .conflict(day: day, family: family) }
        return .ignored
    }

    /// A well-formed key with a month 01...12 and a day 01...31 (the calendar itself is not consulted).
    private static func isPlausibleDay(_ day: String) -> Bool {
        guard DayKey.isWellFormed(day) else { return false }
        let m = Int(day.dropFirst(5).prefix(2)) ?? 0, d = Int(day.dropFirst(8)) ?? 0
        return (1...12).contains(m) && (1...31).contains(d)
    }

    /// `rest` is what follows the 10-character day in the name, without ".md".
    private static func conflictFamily(_ rest: String) -> ConflictFamily? {
        let syncthing = ".sync-conflict-"
        if rest.hasPrefix(syncthing) { return rest.utf8.count > syncthing.utf8.count ? .syncthing : nil }
        guard rest.hasPrefix(" ") else { return nil }
        let t = rest.dropFirst()
        if !t.isEmpty, t.utf8.count <= 3, t.utf8.allSatisfy({ $0 >= 48 && $0 <= 57 }), let n = Int(t), n >= 2 { return .iCloudNumbered }
        if t.hasPrefix("("), t.hasSuffix(")"), t.lowercased().contains("conflicted copy") { return .dropbox }
        return nil
    }
}
