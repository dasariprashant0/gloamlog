// CaptureJournal.swift - write-ahead journal: a note is durable here (fsynced) before anything else happens. Owned by the core stream (task K2).
// CONTRACT (signatures frozen):
//   File: <dir>/journal.jsonl, mode 0600, one JSON object per line: a CaptureItem, or {"ack":"<id>"}.
//   append   open(O_WRONLY|O_APPEND|O_CREAT, 0600), write ONE line, F_FULLFSYNC: the durability point. Throws on failure.
//   ack      appends {"ack": id}.
//   pending  items without an ack, in order; tolerant of a torn last line (a crash mid-write) and of unknown/garbled lines.
//   compact  rewrites the file without acked items (atomically); call at launch when everything is acked or the file has > 1,000 lines.
// Details:
//   * A record is {"v":1,"id","ts" (ISO 8601 with milliseconds),"day","text","stamp"?,"todo","source"}. Reading is lenient: "v" and "todo" may be
//     missing, "ts" may lack the milliseconds, unknown keys are ignored, a line that is not an object, has no usable id/day/text/ts or a
//     malformed day is skipped (a day like "../x" can never reach a file name). The same id twice is one note. An ack is any object whose
//     "ack" is a string, wherever it sits in the file (acks are not order dependent).
//   * A torn last line (power cut mid-write) is ignored by pending(). The next append never glues onto it: when the file does not end in a
//     newline the new record starts on a fresh line, so a fragment can only ever cost the one record that was being written.
//   * A write that fails half way is cut back to the size the file had before it, so a failed append leaves no fragment either.
//   * append and ack both fsync (F_FULLFSYNC, plain fsync where that is unsupported); the first record also syncs the folder entry.
//     The folder is created on demand (0700). The file is created 0600 and tightened to 0600 if it existed wider.
//   * compact writes through AtomicFile (old complete content or new complete content, never a mix), keeps the raw lines of the pending notes
//     in order, and does nothing when the file would not change. A missing file is not an error.
import Foundation

enum JournalError: Error, Equatable { case io(String) }

final class CaptureJournal {
    let dir: URL
    init(dir: URL) { self.dir = dir }

    var fileURL: URL { dir.appendingPathComponent("journal.jsonl") }

    static var defaultDir: URL {                          // ~/Library/Application Support/Gloamlog/capture
        FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/Gloamlog/capture")
    }

    // MARK: write
    /// Durable when this returns: the line is written and flushed to the disk. Throws JournalError.io when it cannot be.
    func append(_ item: CaptureItem) throws { try writeLine(Data(Self.line(for: item).utf8)) }

    func ack(_ id: String) throws { try writeLine(Data(Self.ackLine(id).utf8)) }

    private func writeLine(_ line: Data) throws {
        let fm = FileManager.default
        var isDir: ObjCBool = false
        if !(fm.fileExists(atPath: dir.path, isDirectory: &isDir) && isDir.boolValue) {
            do { try fm.createDirectory(at: dir, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700]) }
            catch { throw JournalError.io("cannot create \(dir.path): \(error.localizedDescription)") }
        }
        let fd = open(fileURL.path, O_RDWR | O_APPEND | O_CREAT, 0o600)
        guard fd >= 0 else { throw JournalError.io("cannot open \(fileURL.path): \(String(cString: strerror(errno)))") }
        defer { close(fd) }

        var st = stat()
        var sizeBefore: off_t = 0
        var data = Data()
        if fstat(fd, &st) == 0 {
            sizeBefore = st.st_size
            if (st.st_mode & 0o777) != 0o600 { _ = fchmod(fd, 0o600) }
            if st.st_size > 0 {                           // a torn last line: start this record on a line of its own
                var last: UInt8 = 0x0A
                if pread(fd, &last, 1, st.st_size - 1) == 1, last != 0x0A { data.append(0x0A) }
            }
        }
        data.append(line)
        data.append(0x0A)

        var failure: String?
        data.withUnsafeBytes { raw in
            var offset = 0
            while offset < raw.count {
                let n = write(fd, raw.baseAddress! + offset, raw.count - offset)
                if n < 0 {
                    if errno == EINTR { continue }
                    failure = "cannot write \(fileURL.path): \(String(cString: strerror(errno)))"
                    return
                }
                offset += n
            }
        }
        if let message = failure {
            _ = ftruncate(fd, sizeBefore)                 // best effort: do not leave half a line behind
            throw JournalError.io(message)
        }
        if fcntl(fd, F_FULLFSYNC) == -1 && fsync(fd) == -1 {
            throw JournalError.io("cannot flush \(fileURL.path): \(String(cString: strerror(errno)))")
        }
        if sizeBefore == 0 {                              // a new file: make its directory entry durable too
            let dfd = open(dir.path, O_RDONLY)
            if dfd >= 0 { if fcntl(dfd, F_FULLFSYNC) == -1 { _ = fsync(dfd) }; close(dfd) }
        }
    }

    // MARK: read
    /// Notes without an ack, in the order they were written. Never throws: an unreadable or missing file has nothing pending.
    func pending() -> [CaptureItem] {
        guard let data = FileManager.default.contents(atPath: fileURL.path) else { return [] }
        let (live, acked) = Self.scan(data)
        return live.filter { !acked.contains($0.item.id) }.map { $0.item }
    }

    /// Rewrites the file with the raw lines of the pending notes only (acknowledged notes, acks, garbage and a torn last line are dropped).
    func compact() throws {
        guard FileManager.default.fileExists(atPath: fileURL.path) else { return }
        AtomicFile.sweep(in: dir)
        let data: Data
        do { data = try Data(contentsOf: fileURL) } catch { throw JournalError.io("cannot read \(fileURL.path): \(error.localizedDescription)") }
        let (live, acked) = Self.scan(data)
        var out = Data()
        for entry in live where !acked.contains(entry.item.id) { out.append(entry.raw); out.append(0x0A) }
        if out == data { return }
        do { try AtomicFile.write(out, to: fileURL) } catch { throw JournalError.io("cannot compact \(fileURL.path): \(error.localizedDescription)") }
    }

    // MARK: records
    private struct Record: Encodable {
        var v = 1
        var id: String
        var ts: String
        var day: String
        var text: String
        var stamp: String?
        var todo: Bool
        var source: String
    }
    private struct AckRecord: Encodable { var ack: String }

    private static func encode<T: Encodable>(_ value: T) -> String {
        let e = JSONEncoder()
        e.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        return (try? e.encode(value)).flatMap { String(data: $0, encoding: .utf8) } ?? ""
    }
    /// The journal line (without the newline) for a note. Internal so tests can build journals of any size without 2,000 fsyncs.
    static func line(for item: CaptureItem) -> String {
        encode(Record(id: item.id, ts: iso(item.ts), day: item.day, text: item.text, stamp: item.stamp, todo: item.todo, source: item.source))
    }
    static func ackLine(_ id: String) -> String { encode(AckRecord(ack: id)) }

    private static let isoPlain: ISO8601DateFormatter = ISO8601DateFormatter()
    private static let isoFraction: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter(); f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]; return f
    }()
    /// "2026-10-07T18:32:01.250Z": whole milliseconds, formatted by hand so rounding can never move a note by a millisecond.
    private static func iso(_ d: Date) -> String {
        let ms = Int64((d.timeIntervalSince1970 * 1000).rounded())
        let secs = ms >= 0 ? ms / 1000 : -((-ms + 999) / 1000)
        let base = isoPlain.string(from: Date(timeIntervalSince1970: Double(secs)))
        return String(base.dropLast()) + String(format: ".%03ldZ", Int(ms - secs * 1000))
    }
    private static func date(_ v: Any?) -> Date? {
        if let s = v as? String { return isoFraction.date(from: s) ?? isoPlain.date(from: s) }
        if let n = v as? Double { return Date(timeIntervalSince1970: n) }
        return nil
    }

    private enum Entry { case item(CaptureItem), ack(String) }
    private static func parse(_ line: Data) -> Entry? {
        guard let obj = (try? JSONSerialization.jsonObject(with: line)) as? [String: Any] else { return nil }
        if let id = obj["ack"] as? String { return .ack(id) }
        guard let id = obj["id"] as? String, !id.isEmpty,
              let day = obj["day"] as? String, DayKey.isWellFormed(day),
              let text = obj["text"] as? String, !text.isEmpty,
              let ts = date(obj["ts"]) else { return nil }
        return .item(CaptureItem(id: id, ts: ts, day: day, text: text, stamp: obj["stamp"] as? String,
                                 todo: obj["todo"] as? Bool ?? false, source: obj["source"] as? String ?? "unknown"))
    }

    /// Every readable note (first occurrence of an id wins) with its raw line, and every acknowledged id.
    private static func scan(_ data: Data) -> (live: [(item: CaptureItem, raw: Data)], acked: Set<String>) {
        var live = [(item: CaptureItem, raw: Data)](), acked = Set<String>(), seen = Set<String>()
        for raw in data.split(separator: 0x0A, omittingEmptySubsequences: true) {
            switch parse(Data(raw)) {
            case .ack(let id)?: acked.insert(id)
            case .item(let item)?: if seen.insert(item.id).inserted { live.append((item, Data(raw))) }
            case nil: break
            }
        }
        return (live, acked)
    }
}
