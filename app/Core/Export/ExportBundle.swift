// ExportBundle.swift - what "Save as Markdown" writes: the review as one .md file plus an assets/ folder holding ONLY the images it uses (M3).
//
// API:
//   ExportBundle.plan(name:, markdown:, dayCount:, assets: AssetStore) -> ExportPlan
//       Finds every image the markdown links (outside code), resolves each through AssetStore.resolve (so nothing outside the log folder is
//       ever read), and rewrites the links to "assets/<file>". A link that points at no file stays as written and is listed in `missing`.
//       Two different files with one name get a "2-" prefix. The plan is computed first, so a test (or a preview) can see exactly what a bundle holds.
//   ExportBundle.write(plan, layout:) throws -> URL
//       .file(url)     <dir>/<name>.md with <dir>/assets/ beside it (assets/ only when an image is used; a file already there with the same bytes is kept)
//       .folder(dir)   a new folder <dir>/<name>/ ("<name> 2" when taken) holding the .md and assets/
//       .zip(url)      the same folder, zipped with /usr/bin/ditto (argument array, no shell), the folder being the single top-level entry
//       Images are copied before the .md is written (atomically), so a failure never leaves a .md without its pictures. Errors: ExportError.
import Foundation

struct ExportAsset: Equatable {
    var path: String            // "assets/<file>" inside the export
    var source: URL             // the real file in the log folder
}

struct ExportPlan: Equatable {
    var name: String            // "Gloamlog 2026-W41": the file or folder name without ".md"
    var markdown: String
    var assets: [ExportAsset]
    var missing: [String]
    var dayCount: Int

    /// "Saved 5 days and 2 images."
    var resultLine: String {
        let days = "\(dayCount) day\(dayCount == 1 ? "" : "s")"
        return assets.isEmpty ? "Saved \(days)." : "Saved \(days) and \(assets.count) image\(assets.count == 1 ? "" : "s")."
    }
}

enum ExportError: Error, Equatable {
    case cannotWrite(String)
    case zipFailed(String)
}

enum ExportBundle {
    enum Layout { case file(URL), folder(URL), zip(URL) }

    static func plan(name: String, markdown: String, dayCount: Int, assets store: AssetStore) -> ExportPlan {
        var assets = [ExportAsset](), missing = [String](), taken = [String: URL](), md = markdown
        for target in MarkdownHTML.imageTargets(in: markdown) {
            guard let src = store.resolve(target) else { missing.append(target); continue }
            let base = src.lastPathComponent
            var file = base, n = 2
            while let other = taken[file], other != src { file = "\(n)-\(base)"; n += 1 }
            if taken[file] == nil { taken[file] = src; assets.append(ExportAsset(path: "\(AssetStore.folderName)/\(file)", source: src)) }
            let rel = "\(AssetStore.folderName)/\(file)"
            if target != rel { md = md.replacingOccurrences(of: "](\(target)", with: "](\(rel)") }
        }
        return ExportPlan(name: name, markdown: md, assets: assets, missing: missing, dayCount: dayCount)
    }

    @discardableResult
    static func write(_ plan: ExportPlan, layout: Layout) throws -> URL {
        let fm = FileManager.default
        do {
            switch layout {
            case .file(let md):
                try tree(plan, mdName: md.lastPathComponent, in: md.deletingLastPathComponent())
                return md
            case .folder(let parent):
                var dir = parent.appendingPathComponent(plan.name, isDirectory: true), n = 2
                while fm.fileExists(atPath: dir.path) { dir = parent.appendingPathComponent("\(plan.name) \(n)", isDirectory: true); n += 1 }
                try fm.createDirectory(at: dir, withIntermediateDirectories: true)
                try tree(plan, mdName: plan.name + ".md", in: dir)
                return dir
            case .zip(let dest):
                let work = URL(fileURLWithPath: ProcessInfo.processInfo.environment["TMPDIR"] ?? NSTemporaryDirectory()).appendingPathComponent("gloamlog-export-\(UUID().uuidString)", isDirectory: true)
                defer { try? fm.removeItem(at: work) }
                let root = work.appendingPathComponent(plan.name, isDirectory: true)
                try fm.createDirectory(at: root, withIntermediateDirectories: true)
                try tree(plan, mdName: plan.name + ".md", in: root)
                let zip = work.appendingPathComponent("out.zip")
                try ditto(root, to: zip)
                if fm.fileExists(atPath: dest.path) { try fm.removeItem(at: dest) }
                try fm.moveItem(at: zip, to: dest)
                return dest
            }
        } catch let e as ExportError { throw e }
        catch { throw ExportError.cannotWrite(error.localizedDescription) }
    }

    private static func tree(_ plan: ExportPlan, mdName: String, in dir: URL) throws {
        let fm = FileManager.default
        if !plan.assets.isEmpty {
            try fm.createDirectory(at: dir.appendingPathComponent(AssetStore.folderName, isDirectory: true), withIntermediateDirectories: true)
            for a in plan.assets {
                let to = dir.appendingPathComponent(a.path)
                if fm.fileExists(atPath: to.path) {
                    if fm.contentsEqual(atPath: to.path, andPath: a.source.path) { continue }
                    try fm.removeItem(at: to)
                }
                try fm.copyItem(at: a.source, to: to)
            }
        }
        try AtomicFile.write(Data(plan.markdown.utf8), to: dir.appendingPathComponent(mdName))
    }

    private static func ditto(_ dir: URL, to zip: URL) throws {
        let p = Process(), err = Pipe()
        p.executableURL = URL(fileURLWithPath: "/usr/bin/ditto")
        p.arguments = ["-c", "-k", "--keepParent", "--norsrc", "--noextattr", "--noqtn", "--noacl", dir.path, zip.path]
        p.standardError = err
        p.standardOutput = FileHandle.nullDevice
        do { try p.run() } catch { throw ExportError.zipFailed(error.localizedDescription) }
        let text = err.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        guard p.terminationStatus == 0 else { throw ExportError.zipFailed(String(decoding: text, as: UTF8.self).dlTrimmed) }
    }
}
