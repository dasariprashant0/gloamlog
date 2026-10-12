// FolderWatcher.swift - "something in the log folder changed", delivered once per quiet moment instead of on a 30 s tick.
// FSEventsSource listens to the file system (events are coalesced for 0.5 s by FSEvents itself); PollingSource looks at the folder
// every few seconds, for network and removable volumes and for the day FSEvents refuses to start. Both ignore what is not ours to
// care about: hidden files (our own temp files, .DS_Store), assets/, and the echo of Gloamlog's own writes (OwnWrites).
// Event flags are never trusted: any relevant event only means "look again", and the caller's own stat-based check decides what changed.
import Foundation
import CoreServices

protocol FolderWatcher: AnyObject {
    /// Called on the main queue after a quiet moment in which something other than our own writes changed in the folder.
    var onChange: (() -> Void)? { get set }
    @discardableResult func start() -> Bool
    func stop()
}

enum FolderWatchers {
    /// File events for local and cloud folders; polling for network and removable volumes, and whenever file events will not start.
    static func start(folder: URL, kind: FolderKind, onChange: @escaping () -> Void) -> FolderWatcher {
        if kind != .network {
            let w = FSEventsSource(folder: folder)
            w.onChange = onChange
            if w.start() { return w }
        }
        let p = PollingSource(folder: folder)
        p.onChange = onChange
        p.start()
        return p
    }
}

final class FSEventsSource: FolderWatcher {
    var onChange: (() -> Void)?
    private let root: String
    private let latency: TimeInterval
    private var stream: FSEventStreamRef?
    private let queue = DispatchQueue(label: "gloamlog.fsevents", qos: .utility)

    init(folder: URL, latency: TimeInterval = 0.5) {
        root = CloudFile.canonicalPath(folder.path)
        self.latency = latency
    }

    @discardableResult
    func start() -> Bool {
        guard stream == nil else { return true }
        var context = FSEventStreamContext(version: 0, info: Unmanaged.passUnretained(self).toOpaque(), retain: nil, release: nil, copyDescription: nil)
        let flags = UInt32(kFSEventStreamCreateFlagFileEvents | kFSEventStreamCreateFlagUseCFTypes | kFSEventStreamCreateFlagWatchRoot)
        let callback: FSEventStreamCallback = { _, info, count, paths, eventFlags, _ in
            guard let info = info else { return }
            Unmanaged<FSEventsSource>.fromOpaque(info).takeUnretainedValue().receive(paths: paths, count: count, flags: eventFlags)
        }
        guard let s = FSEventStreamCreate(kCFAllocatorDefault, callback, &context, [root] as CFArray,
                                          FSEventStreamEventId(kFSEventStreamEventIdSinceNow), latency, flags) else { return false }
        FSEventStreamSetDispatchQueue(s, queue)
        guard FSEventStreamStart(s) else { FSEventStreamInvalidate(s); FSEventStreamRelease(s); return false }
        stream = s
        return true
    }

    func stop() {
        guard let s = stream else { return }
        stream = nil
        FSEventStreamStop(s)
        FSEventStreamInvalidate(s)
        FSEventStreamRelease(s)
        queue.sync { }                                  // a callback that is already running finishes before `self` can go away
    }
    deinit { stop() }

    private func receive(paths: UnsafeMutableRawPointer, count: Int, flags: UnsafePointer<FSEventStreamEventFlags>) {
        let list = unsafeBitCast(paths, to: NSArray.self) as? [String] ?? []
        let lost = UInt32(kFSEventStreamEventFlagMustScanSubDirs | kFSEventStreamEventFlagUserDropped
                          | kFSEventStreamEventFlagKernelDropped | kFSEventStreamEventFlagRootChanged)
        for i in 0..<min(count, list.count) where (flags[i] & lost) != 0 || Self.isRelevant(list[i]) {
            DispatchQueue.main.async { [weak self] in self?.onChange?() }
            return
        }
    }

    /// A day file, an iCloud placeholder or a conflict copy that is not the echo of our own write.
    static func isRelevant(_ path: String) -> Bool {
        FolderEntryClassifier.classify((path as NSString).lastPathComponent) != .ignored && !OwnWrites.isOwn(path)
    }
}

final class PollingSource: FolderWatcher {
    var onChange: (() -> Void)?
    private let folder: URL
    private let interval: TimeInterval
    private let queue = DispatchQueue(label: "gloamlog.polling", qos: .utility)
    private var timer: DispatchSourceTimer?
    private var last: String?                           // only touched on `queue`

    init(folder: URL, interval: TimeInterval = 5) { self.folder = folder; self.interval = interval }

    @discardableResult
    func start() -> Bool {
        guard timer == nil else { return true }
        let t = DispatchSource.makeTimerSource(queue: queue)
        t.schedule(deadline: .now(), repeating: interval)
        t.setEventHandler { [weak self] in self?.poll() }
        t.resume()
        timer = t
        return true
    }

    func stop() { timer?.cancel(); timer = nil }

    private func poll() {
        let now = Self.signature(of: folder)
        defer { last = now }
        guard let before = last, before != now else { return }            // the first look is only the baseline
        DispatchQueue.main.async { [weak self] in self?.onChange?() }
    }

    /// Name, mtime, size and the not-downloaded flag of every day file, placeholder and conflict copy.
    static func signature(of folder: URL) -> String {
        let names = ((try? FileManager.default.contentsOfDirectory(atPath: folder.path)) ?? []).sorted()
        return names.compactMap { name -> String? in
            guard FolderEntryClassifier.classify(name) != .ignored else { return nil }
            let path = folder.appendingPathComponent(name).path
            return "\(name) \(CloudFile.signature(atPath: path) ?? "-") \(CloudFile.isDataless(atPath: path) ? "d" : "")"
        }.joined(separator: "\n")
    }
}
