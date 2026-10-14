// SearchIndex.swift - the in-memory cache behind LogStore.search.
//
// Every page as search sees it (Search.Section: the plain text of each heading-delimited section), parsed once per run and re-read
// only when the file's "mtime size" signature changed: the app writes files itself, and folder sync and outside editors change them
// underneath it. Which days exist is still listDays()'s call, so a deleted file drops out of the results (and of the cache). A day
// that is not on this Mac yet (LogStore.loadIfLocal returns nil: an iCloud placeholder or a dataless file) is left out and never
// cached as empty, exactly as the uncached search skipped it. Memory: the plain text plus the titled haystack of every section, as
// NSStrings (Search.Section): about 2-3x the text, so a decade of logs is ~20-30 MB.
// Thread-safe: one lock around each call (the app searches and warms on a utility queue, see AppModel+Browse.swift).
// ponytail: one global lock, so a search typed while the launch warm-up is still reading waits for it (once per run, the cost of one
// cold search); every search stats every file (~30 ms per 3,650 on APFS) and then runs NSString's case/diacritic-insensitive search over
// all cached text (~50-90 ms per 5 MB). Ceiling: a cold start over tens of thousands of files, or a folder whose stat sweep is slow
// (network volume). Upgrade path then, in order: a byte prefilter on case/diacritic-folded text ahead of the exact search; one
// opendir/fstatat sweep instead of per-file stats (as folderStamp does); a persistent SQLite FTS5 index through the system `import SQLite3`.
import Foundation

final class SearchIndex {
    typealias Page = (day: String, sections: [Search.Section])
    private struct Entry { var signature: String; var sections: [Search.Section] }
    private var entries = [String: Entry]()
    private let lock = NSLock()
    /// Files read and parsed so far (tests: a file is parsed once, and again only after it changed).
    private(set) var fileReads = 0

    /// The pages of the store's days, newest first, from the cache wherever the file is unchanged. `load` is LogStore.loadIfLocal:
    /// nil = not here yet, left out and not cached; a throwing load fails the whole call, as it did for the uncached search.
    func pages(of store: LogStore, load: (String) throws -> DayDocument?) throws -> [Page] {
        lock.lock(); defer { lock.unlock() }
        let days = try store.listDays()
        var out = [Page](); out.reserveCapacity(days.count)
        var seen = Set<String>()
        let dirPath = store.dir.path                   // paths by concatenation: URL(...).path per file costs more than the stat calls
        for day in days {
            // Signature before read, so a cached text is never older than its signature.
            let path = dirPath + "/" + day + ".md"
            guard let sig = CloudFile.signature(atPath: path) else { continue }               // gone since the listing: load() says nil too
            // A hit is served only while the page is here: the same two checks as LogStore.isNotLocal (a miss goes through load).
            if let e = entries[day], e.signature == sig,
               !CloudFile.exists(atPath: dirPath + "/" + CloudFile.placeholderName(for: day)), !CloudFile.isDataless(atPath: path) {
                out.append((day: day, sections: e.sections)); seen.insert(day); continue
            }
            guard let doc = try load(day) else { entries[day] = nil; continue }               // not on this Mac yet: skipped, never cached
            fileReads += 1
            let sections = Search.sections(of: doc)
            entries[day] = Entry(signature: sig, sections: sections)
            out.append((day: day, sections: sections)); seen.insert(day)
        }
        if entries.count > seen.count { entries = entries.filter { seen.contains($0.key) } }   // deleted days leave the cache
        return out
    }
}
