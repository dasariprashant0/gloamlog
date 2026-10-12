// ReviewExporter.swift - the AppKit side of copying and exporting a review (M3): pasteboard flavours, images for the PDF, the PDF itself,
// and the short-lived files behind Share. Nothing here touches the network; WebKit only lays out and prints a page we built.
//
//   ReviewPasteboard.put(_ text)                      one plain string
//   ReviewPasteboard.putRich(markdown:)               .rtf (when under 200 KB) + .html + .string (the Markdown, so chat boxes and plain editors get Markdown)
//   ReviewImages.dataURI(_ path, assets:)             a log-folder image as a data: URI, scaled to 1,600 px (JPEG, or PNG when it has transparency)
//   PDFMaker.make(html:, to:) { ok in }               WKWebView + the print operation (jobDisposition .save), checked with PDFKit; falls back to
//                                                     WKWebView.createPDF (one long page) when printing fails; a 30 s watchdog ends it either way
//   ReviewShare.fileForSharing(plan:, layout name)    writes the Markdown (or a zip when images are used) under ~/Library/Caches/Gloamlog/share/<uuid>/;
//                                                     old ones go after an hour and everything goes at quit
import AppKit
import WebKit
import PDFKit
import ImageIO
import UniformTypeIdentifiers

enum ReviewPasteboard {
    static func put(_ text: String) {
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(text, forType: .string)
    }

    static func putRich(markdown: String) {
        let html = MarkdownHTML.fragment(markdown)
        let rtf = html.utf8.count <= 200_000 ? rtf(fromHTML: html) : nil
        let pb = NSPasteboard.general
        pb.declareTypes((rtf == nil ? [] : [.rtf]) + [.html, .string], owner: nil)      // richest first
        if let r = rtf { pb.setData(r, forType: .rtf) }
        pb.setString(html, forType: .html)
        pb.setString(markdown, forType: .string)
    }

    /// HTML to RTF through the system importer. Main thread only.
    static func rtf(fromHTML html: String) -> Data? {
        let opts: [NSAttributedString.DocumentReadingOptionKey: Any] = [.documentType: NSAttributedString.DocumentType.html,
                                                                         .characterEncoding: String.Encoding.utf8.rawValue]
        guard let a = try? NSAttributedString(data: Data(html.utf8), options: opts, documentAttributes: nil) else { return nil }
        return try? a.data(from: NSRange(location: 0, length: a.length), documentAttributes: [.documentType: NSAttributedString.DocumentType.rtf])
    }
}

enum ReviewImages {
    static let maxPixel = 1_600

    static func dataURI(_ path: String, assets: AssetStore) -> String? {
        guard let file = assets.resolve(path), let src = CGImageSourceCreateWithURL(file as CFURL, nil) else { return nil }
        let opts: [CFString: Any] = [kCGImageSourceCreateThumbnailFromImageAlways: true, kCGImageSourceCreateThumbnailWithTransform: true,
                                     kCGImageSourceThumbnailMaxPixelSize: maxPixel]
        guard let cg = CGImageSourceCreateThumbnailAtIndex(src, 0, opts as CFDictionary) else { return nil }
        let opaque: Bool
        switch cg.alphaInfo {
        case .none, .noneSkipFirst, .noneSkipLast: opaque = true
        default: opaque = false
        }
        let out = NSMutableData()
        guard let dest = CGImageDestinationCreateWithData(out, (opaque ? UTType.jpeg : UTType.png).identifier as CFString, 1, nil) else { return nil }
        CGImageDestinationAddImage(dest, cg, opaque ? [kCGImageDestinationLossyCompressionQuality: 0.8] as CFDictionary : nil)
        guard CGImageDestinationFinalize(dest) else { return nil }
        return "data:image/\(opaque ? "jpeg" : "png");base64,\((out as Data).base64EncodedString())"
    }
}

/// Makes a PDF of an HTML page. The web view lives in an off-screen borderless window (the print operation needs one) with JavaScript
/// off and a private data store; the page itself carries a CSP that allows nothing but inline styles and data: images.
final class PDFMaker: NSObject, WKNavigationDelegate {
    private static var running = [PDFMaker]()
    static let margin: CGFloat = 54

    private let dest: URL
    private let tmp = URL(fileURLWithPath: NSTemporaryDirectory()).appendingPathComponent("gloamlog-\(UUID().uuidString).pdf")
    private var completion: ((Bool) -> Void)?
    private var web: WKWebView?
    private var window: NSWindow?

    static func make(html: String, to dest: URL, completion: @escaping (Bool) -> Void) {
        let m = PDFMaker(dest: dest, completion: completion)
        running.append(m)
        m.start(html)
    }

    private init(dest: URL, completion: @escaping (Bool) -> Void) {
        self.dest = dest; self.completion = completion
        super.init()
    }

    private func start(_ html: String) {
        let cfg = WKWebViewConfiguration()
        cfg.websiteDataStore = .nonPersistent()
        cfg.defaultWebpagePreferences.allowsContentJavaScript = false
        let paper = NSPrintInfo.shared.paperSize
        let frame = NSRect(x: 0, y: 0, width: paper.width - 2 * Self.margin, height: paper.height - 2 * Self.margin)
        let w = WKWebView(frame: frame, configuration: cfg)
        w.navigationDelegate = self
        let win = NSWindow(contentRect: frame, styleMask: .borderless, backing: .buffered, defer: false)
        win.isReleasedWhenClosed = false
        win.contentView = w
        win.setFrameOrigin(NSPoint(x: -20_000, y: -20_000))
        win.orderBack(nil)
        web = w; window = win
        w.loadHTMLString(html, baseURL: nil)
        DispatchQueue.main.asyncAfter(deadline: .now() + 30) { [weak self] in self?.finish(false) }
    }

    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        DispatchQueue.main.async { [weak self] in self?.printToPDF() }      // one turn of the run loop: layout settles before pagination
    }
    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) { finish(false) }
    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) { finish(false) }

    private func printToPDF() {
        guard let web = web, let win = window, completion != nil else { return }
        let info = (NSPrintInfo.shared.copy() as? NSPrintInfo) ?? NSPrintInfo()
        info.topMargin = Self.margin; info.bottomMargin = Self.margin; info.leftMargin = Self.margin; info.rightMargin = Self.margin
        info.isHorizontallyCentered = false; info.isVerticallyCentered = false
        info.jobDisposition = .save
        info.dictionary()[NSPrintInfo.AttributeKey.jobSavingURL] = tmp
        let op = web.printOperation(with: info)
        op.showsPrintPanel = false
        op.showsProgressPanel = false
        op.view?.frame = NSRect(x: 0, y: 0, width: info.paperSize.width - 2 * Self.margin, height: info.paperSize.height - 2 * Self.margin)
        op.runModal(for: win, delegate: self, didRun: #selector(printOperationDidRun(_:success:contextInfo:)), contextInfo: nil)
    }

    @objc private func printOperationDidRun(_ op: NSPrintOperation, success: Bool, contextInfo: UnsafeMutableRawPointer?) {
        if success, Self.isReadablePDF(tmp) { place(tmp) } else { fallback() }
    }

    /// One tall page, as WebKit lays it out: used only when the print operation did not give a PDF with text in it.
    private func fallback() {
        guard let web = web, completion != nil else { return }
        web.createPDF(configuration: WKPDFConfiguration()) { [weak self] result in
            guard let self = self else { return }
            if case .success(let data) = result, (try? data.write(to: self.tmp, options: .atomic)) != nil, Self.isReadablePDF(self.tmp) { self.place(self.tmp) }
            else { self.finish(false) }
        }
    }

    private static func isReadablePDF(_ url: URL) -> Bool {
        guard let doc = PDFDocument(url: url), doc.pageCount >= 1 else { return false }
        return !(doc.string ?? "").dlTrimmed.isEmpty
    }

    private func place(_ pdf: URL) {
        let fm = FileManager.default
        do {
            if fm.fileExists(atPath: dest.path) { try fm.removeItem(at: dest) }
            try fm.moveItem(at: pdf, to: dest)
            finish(true)
        } catch { finish(false) }
    }

    private func finish(_ ok: Bool) {
        guard let done = completion else { return }
        completion = nil
        window?.orderOut(nil)
        window?.contentView = nil
        window = nil
        web?.navigationDelegate = nil
        web = nil
        try? FileManager.default.removeItem(at: tmp)
        PDFMaker.running.removeAll { $0 === self }
        done(ok)
    }
}

/// The file behind Share…: written only when the person chooses Share, under the user's Caches folder, never in the log folder.
enum ReviewShare {
    static var root: URL {
        (FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first ?? URL(fileURLWithPath: NSTemporaryDirectory()))
            .appendingPathComponent("Gloamlog/share", isDirectory: true)
    }

    /// The Markdown file, or a zip of the Markdown and its images when it uses any. Throws ExportError.
    static func fileForSharing(plan: ExportPlan) throws -> URL {
        pruneOld()
        let dir = root.appendingPathComponent(UUID().uuidString, isDirectory: true)
        do { try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true) } catch { throw ExportError.cannotWrite(error.localizedDescription) }
        let layout: ExportBundle.Layout = plan.assets.isEmpty ? .file(dir.appendingPathComponent(plan.name + ".md")) : .zip(dir.appendingPathComponent(plan.name + ".zip"))
        return try ExportBundle.write(plan, layout: layout)
    }

    static func pruneOld(olderThan age: TimeInterval = 3_600) {
        let fm = FileManager.default
        for u in (try? fm.contentsOfDirectory(at: root, includingPropertiesForKeys: [.contentModificationDateKey])) ?? [] {
            let modified = (try? u.resourceValues(forKeys: [.contentModificationDateKey]))?.contentModificationDate ?? .distantPast
            if Date().timeIntervalSince(modified) > age { try? fm.removeItem(at: u) }
        }
    }

    static func removeAll() { try? FileManager.default.removeItem(at: root) }
}
