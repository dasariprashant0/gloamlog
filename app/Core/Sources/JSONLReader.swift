// JSONLReader.swift - streams a (possibly huge) JSON-lines file one line at a time without ever holding the file in memory (M6).
// Foundation only. Zero-copy for lines that sit inside one 4 MiB chunk (memchr on the chunk), a small carry buffer for lines that
// cross a chunk boundary, and lines longer than `maxLine` (images, file dumps) are skipped without being buffered. A last line
// with no newline is left unread: it may still be being written (`endOffset` says where to resume).
import Foundation

struct JSONLReader {
    let url: URL
    var maxLine = 8 << 20
    var chunkSize = 4 << 20

    struct Outcome { var endOffset: UInt64; var skippedLong: Int }

    /// Calls `body` for every complete, non-empty line in [offset, min(size at open, offset + limit)). The Data is a slice of the
    /// chunk: copy it if you keep it. `body` returns false to stop reading.
    @discardableResult
    func forEachLine(from offset: UInt64 = 0, limit: UInt64? = nil, _ body: (Data) -> Bool) throws -> Outcome {
        let fh = try FileHandle(forReadingFrom: url)
        defer { try? fh.close() }
        var size = try fh.seekToEnd()
        if let l = limit { size = min(size, offset + l) }
        guard offset < size else { return Outcome(endOffset: min(offset, size), skippedLong: 0) }
        try fh.seek(toOffset: offset)

        var lineStart = offset, bufBase = offset
        var carry = Data(), skipping = false, skipped = 0, stopped = false
        while bufBase < size && !stopped {
            // One pool per chunk: Foundation hands the chunk (and anything the caller decodes from it) back lazily, which would
            // otherwise pile up for the whole file.
            let advanced: Bool = try autoreleasepool {
                let want = Int(min(UInt64(chunkSize), size - bufBase))
                guard let buf = try fh.read(upToCount: want), !buf.isEmpty else { return false }
                buf.withUnsafeBytes { (raw: UnsafeRawBufferPointer) in
                    guard let base = raw.baseAddress else { return }
                    let n = raw.count
                    var i = 0
                    while i < n {
                        guard let p = memchr(base + i, 0x0A, n - i) else {              // no newline in the rest of this chunk
                            if !skipping {
                                carry.append(buf[(buf.startIndex + i)...])
                                if carry.count > maxLine { skipping = true; carry.removeAll(keepingCapacity: true) }
                            }
                            break
                        }
                        let nl = base.distance(to: UnsafeRawPointer(p))
                        let piece = buf[(buf.startIndex + i)..<(buf.startIndex + nl)]
                        if skipping { skipping = false; skipped += 1 }
                        else if carry.isEmpty {                                         // the whole line is inside this chunk: no copy
                            if piece.count > maxLine { skipped += 1 } else if !piece.isEmpty, !body(piece) { stopped = true }
                        } else {
                            carry.append(piece)
                            if carry.count > maxLine { skipped += 1 } else if !body(carry) { stopped = true }
                        }
                        carry.removeAll(keepingCapacity: true)
                        lineStart = bufBase + UInt64(nl) + 1
                        i = nl + 1
                        if stopped { return }
                    }
                }
                bufBase += UInt64(buf.count)
                return true
            }
            if !advanced { break }
        }
        return Outcome(endOffset: lineStart, skippedLong: skipped)
    }
}

extension Data {
    /// True when `needle` occurs in the bytes (memmem: no copy, no decoding).
    func containsBytes(_ needle: [UInt8]) -> Bool {
        guard !needle.isEmpty, count >= needle.count else { return false }
        return withUnsafeBytes { h in needle.withUnsafeBytes { n in memmem(h.baseAddress, h.count, n.baseAddress, n.count) != nil } }
    }
}
