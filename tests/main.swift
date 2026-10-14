// Tiny assert-based runner (no XCTest). Run with tests/run-tests.sh (outside the Bash sandbox: atomic writes need the system temp dir).
import Foundation

var passed = 0, failed = 0
var currentTest = ""
func test(_ name: String, _ body: () throws -> Void) {
    currentTest = name
    do { try body() } catch { failed += 1; print("FAIL [\(name)] threw \(error)") }
}
func expect(_ cond: @autoclosure () throws -> Bool, _ msg: String = "", line: Int = #line) rethrows {
    if try cond() { passed += 1 } else { failed += 1; print("FAIL [\(currentTest)] line \(line) \(msg)") }
}

// MARK: helpers
var cal = Calendar(identifier: .gregorian)
cal.timeZone = TimeZone(identifier: "America/New_York")!
cal.firstWeekday = 2
func mk(_ y: Int, _ m: Int, _ d: Int, _ h: Int = 12, _ mi: Int = 0) -> Date {
    cal.date(from: DateComponents(year: y, month: m, day: d, hour: h, minute: mi))!
}
let tmpBase = URL(fileURLWithPath: ProcessInfo.processInfo.environment["TMPDIR"] ?? NSTemporaryDirectory(), isDirectory: true)
let root = tmpBase.appendingPathComponent("dl-tests-\(UUID().uuidString)")
try! FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
func freshDir(_ name: String) -> URL {
    let u = root.appendingPathComponent(name + "-" + UUID().uuidString.prefix(6))
    try! FileManager.default.createDirectory(at: u, withIntermediateDirectories: true); return u
}
func freshStore(_ name: String) -> LogStore { LogStore(dir: freshDir(name)) }
/// n words: "w1 w2 ... wn" ("" for 0)
func lorem(_ n: Int, _ prefix: String = "w") -> String { n <= 0 ? "" : (1...n).map { "\(prefix)\($0)" }.joined(separator: " ") }
func raw(_ s: String, in dir: URL, day: String) throws { try s.write(to: dir.appendingPathComponent(day + ".md"), atomically: true, encoding: .utf8) }
func fileText(_ dir: URL, _ day: String) throws -> String { try String(contentsOf: dir.appendingPathComponent(day + ".md"), encoding: .utf8) }
let workdays: Set<Int> = [2, 3, 4, 5, 6]
let pngBytes = Data([0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3, 4])
// 2026-10-05 is a Monday.
func W(_ s: String) -> Int { MarkdownBody.words(in: s) }

// MARK: word counting
test("words: basics, headings, list/task markers, quotes") {
    expect(W("") == 0 && W("  \n\n ") == 0)
    expect(W("Hello world") == 2)
    expect(W("# Title\n## What I did\nwrote code") == 2, "heading lines are ignored")
    expect(W("#tag stays") == 2, "#tag is not a heading")
    expect(W("####### seven") == 1, "7 hashes is not a heading; the hashes are not a word")
    expect(W("## Done ##\n###### six\nreal words") == 2, "closing hashes, level 6")
    expect(W("   ### three spaces\ntext") == 1, "up to 3 leading spaces is still a heading")
    expect(W("    ### four spaces is not a heading") == 6)
    expect(W("-\n- \n* [ ]\n- [x]\n1.\n- [ ] \n+ [X]\n2) [ ]\n[ ]") == 0, "empty list/task markers")
    expect(W("- [ ] Call Ana") == 2 && W("- [x] done thing") == 2 && W("  - [ ] sub task") == 2)
    expect(W("1. First thing\n2) second thing") == 4 && W("* star item") == 2)
    expect(W("**bold** text") == 2 && W("-5 degrees") == 2, "not list markers")
    expect(W("> quoted words here") == 3 && W("> - [ ] task") == 1 && W(">> nested quote") == 2)
    expect(W("a\u{00A0}b\tc") == 3 && W("one two\r\nthree") == 3, "NBSP, tab, CRLF")
    expect(W("don't can't") == 2 && W("https://example.com/a/b") == 1 && W("v1.2.3 shipped") == 2 && W("2026-10-06") == 1)
    expect(W("---\n***\n___\n— – -") == 0, "rules and dashes are not words")
    expect(W("| a | b |\n|---|---|\n| c | d |") == 4, "tables")
    expect(W("[two words](http://x.com)") == 2 && W("<https://example.com>") == 1)
}
test("words: code fences") {
    expect(W("```swift\nlet x = 1\n```") == 3, "fence lines ignored, code between them counts")
    expect(W("~~~\ncode here\n~~~\nafter") == 3)
    expect(W("```\n# not heading\n```") == 2, "headings do not exist inside code")
    expect(W("````\n```\ntext\n````") == 1, "a shorter fence does not close a longer one")
    expect(W("```\none two") == 2, "unclosed fence runs to the end")
    expect(W("`a b`") == 2 && W("```\n```") == 0)
}
test("words: images, html, comments") {
    expect(W("![diagram](assets/2026-10-06-1a2b3c4d.png)") == 0)
    expect(W("see ![a b](x.png) here") == 2 && W("![a](x.png \"title text\")") == 0)
    expect(W("![ref image][r1] words") == 1 && W("- ![](a.png)") == 0)
    expect(W("<img src=\"a.png\" alt=\"x y\">") == 0 && W("<br>") == 0 && W("text<br>more") == 2)
    expect(W("<!-- hidden words here -->") == 0 && W("visible <!-- hidden --> text") == 2)
    expect(W("start\n<!--\nsecret one two\n-->\nend") == 2, "multi-line comment")
    expect(W("a b <!-- c d") == 2, "unterminated comment hides the rest")
    expect(W("<!-- a\nb --> tail words") == 2 && W("## Title <!-- note -->\nbody") == 1)
    expect(W("```\n<!-- x y -->\n```") == 2, "comments are literal inside code")
}
test("words: emoji and non-Latin text") {
    expect(W("Done ✅ 🎉") == 1 && W("🎉🎉🎉") == 0 && W("✅") == 0 && W("Shipped🚀") == 1 && W("👍 great job 👍") == 2)
    expect(W("👨‍👩‍👧") == 0, "ZWJ sequence is not a word")
    expect(W("Сегодня я закончил отчёт") == 4, "Cyrillic")
    expect(W("اليوم أنهيت التقرير") == 3 && W("היום סיימתי את הדוח") == 4, "Arabic, Hebrew")
    expect(W("आज मैंने रिपोर्ट पूरी की") == 5, "Devanagari")
    expect(W("오늘 보고서를 완료했다") == 3, "Hangul uses spaces")
    expect(W("今天完成了报告") == 4, "CJK run of 7 chars = 4 words (ceil(n/2))")
    expect(W("完成 报告") == 2 && W("Q4完成") == 2 && W("hello世界") == 2 && W("完成，报告。") == 2)
    expect(W("ありがとう") == 3 && W("コーヒーを飲んだ") == 4)
    expect(W("วันนี้ทำงานเสร็จ") == 1, "Thai: one word per space-delimited chunk (documented limit)")
}
test("hasContent / DayDocument.status boundaries") {
    expect(!MarkdownBody.hasContent("") && !MarkdownBody.hasContent("## A\n\n## B") && !MarkdownBody.hasContent("- [ ]\n"))
    expect(!MarkdownBody.hasContent("<!-- c -->") && !MarkdownBody.hasContent("```\n```"))
    expect(MarkdownBody.hasContent("![](assets/x.png)"), "an image alone is content")
    expect(MarkdownBody.hasContent("- [ ] x") && MarkdownBody.hasContent("---"))
    expect(DayDocument(day: "2026-10-05", body: lorem(19)).status(minWords: 20) == .partial)
    expect(DayDocument(day: "2026-10-05", body: lorem(20)).status(minWords: 20) == .logged)
    expect(DayDocument(day: "2026-10-05", body: Settings.defaultTemplate).status(minWords: 20) == .missed, "template alone is no log")
    expect(DayDocument(day: "2026-10-05", body: Settings.defaultTemplate + "\n\n" + lorem(3)).words == 3)
    expect(DayDocument(day: "2026-10-05", body: "![](a.png)").status(minWords: 1) == .missed)
    expect(DayDocument(day: "2026-10-05", isSkipped: true, skipReason: "x").status(minWords: 1) == .skipped)
    expect(DayDocument(day: "2026-10-05", body: lorem(1)).status(minWords: 0) == .logged, "minWords below 1 behaves as 1")
}

// MARK: headings and sections
test("normalizeHeading") {
    let n = MarkdownBody.normalizeHeading
    expect(n("📝 What I did") == "what i did" && n("⏳ Pending / blocked") == "pending blocked")
    expect(n("Pending / blocked") == n("⏳ Pending / blocked") && n("📌 To do next") == n("To do next"))
    expect(n("  To-Do   Next!! ") == "to do next" && n("Pending/blocked") == "pending blocked")
    expect(n("Q&A") == "q a" && n("Café ☕") == "café" && n("Привет, мир!") == "привет мир")
    expect(n("1️⃣ First") == "1 first" && n("हिंदी") == "हिंदी" && n("🙂") == "" && n("") == "")
}
test("sections(of:) levels, nesting, fences, edge headings") {
    let body = "intro line\n\n## 📝 What I did\ntext A\n#tag line\n\n### Sub\nsub text\n\n```\n## not heading\n```\n\n## Finished\n"
    let s = MarkdownBody.sections(of: body)
    expect(s.map { $0.level } == [0, 2, 3, 2])
    expect(s.map { $0.title } == ["", "📝 What I did", "Sub", "Finished"])
    expect(s.map { $0.normalizedTitle } == ["", "what i did", "sub", "finished"])
    expect(s[0].text == "intro line" && s[1].text == "text A\n#tag line")
    expect(s[2].text == "sub text\n\n```\n## not heading\n```" && s[3].text == "")
    let e = MarkdownBody.sections(of: "## Title ##\n##\n   ## three\n    ## four spaces\n## C#\n####### x\nSetext\n======")
    expect(e.map { $0.title } == ["Title", "", "three", "C#"], "\(e.map { $0.title })")
    expect(e[2].text == "## four spaces", "4 leading spaces is text, not a heading")
    expect(e[3].text == "####### x\nSetext\n======", "7 hashes and setext underlines are plain text")
    expect(MarkdownBody.sections(of: "").isEmpty && MarkdownBody.sections(of: "\n\n").isEmpty)
    expect(MarkdownBody.headings(inTemplate: Settings.defaultTemplate) == ["What I did", "Finished", "Started", "Pending / blocked", "To do next"])
    expect(MarkdownBody.headings(inTemplate: "## A\n### a\n# 🅰 A\n## B") == ["A", "B"], "de-duplicated by normalised title")
}
test("trimBody, shiftHeadings, pruneEmptySections, partition") {
    expect(MarkdownBody.trimBody("\n\n  indented\nline  \n\n") == "  indented\nline" && MarkdownBody.trimBody("a\r\nb\r\n") == "a\nb")
    expect(MarkdownBody.shiftHeadings(in: "# Big\n## Small\ntext", minLevel: 3) == "### Big\n#### Small\ntext")
    expect(MarkdownBody.shiftHeadings(in: "### a\n#### b", minLevel: 3) == "### a\n#### b")
    expect(MarkdownBody.shiftHeadings(in: "# a\n###### b", minLevel: 3) == "### a\n###### b", "capped at 6")
    expect(MarkdownBody.shiftHeadings(in: "no headings <!-- c -->", minLevel: 3) == "no headings")
    expect(MarkdownBody.pruneEmptySections("## A\n\n## B\ntext\n\n## C\n- [ ]\n\n## D\n### E\n### F\nstuff") == "## B\ntext\n\n## D\n### F\nstuff")
    expect(MarkdownBody.pruneEmptySections(Settings.defaultTemplate) == "")
    let body = "pre text\n## Finished\nA\n### Sub\nB\n## Misc\nC\n## finished\nD\n#### Deep\nE"
    let p = MarkdownBody.partition(body, headings: ["finished"])
    expect(p.matched.map { $0.index } == [0, 0] && p.matched[0].text == "A\n\n**Sub**\nB" && p.matched[1].text == "D\n\n**Deep**\nE")
    expect(p.other == "pre text\n\n**Misc**\nC", p.other)
    expect(MarkdownBody.partition(body, headings: ["finished"], subheadings: .drop).matched[0].text == "A\n\nB")
}

// MARK: day file format and old files
test("v0.1/v0.2 files load as plain markdown; re-save is stable") {
    let store = freshStore("old")
    var v02 = "# 2026-10-05\n\n"
    for (t, b) in [("📝 What I did", "Wrote code"), ("✅ Finished", "Shipped it"), ("🚀 Started", "Docs"),
                   ("⏳ Pending / blocked", "Waiting on Ana"), ("📌 To do next", "Review PR")] { v02 += "## \(t)\n\(b)\n\n" }
    try raw(v02, in: store.dir, day: "2026-10-05")
    let d = try store.load("2026-10-05")!
    expect(d.body.hasPrefix("## 📝 What I did\nWrote code\n\n## ✅ Finished") && d.body.hasSuffix("## 📌 To do next\nReview PR"))
    expect(!d.body.contains("# 2026-10-05") && !d.isSkipped)
    expect(d.words == 10 && d.status(minWords: 20) == .partial && d.status(minWords: 10) == .logged)
    try store.save(day: "2026-10-05", body: d.body)
    let once = try fileText(store.dir, "2026-10-05")
    expect(once.hasPrefix("# 2026-10-05\n\n## 📝 What I did\nWrote code") && once.hasSuffix("Review PR\n"))
    try store.save(day: "2026-10-05", body: try store.load("2026-10-05")!.body)
    try expect(try fileText(store.dir, "2026-10-05") == once, "save(load(x)) is a fixed point")
    expect(MarkdownBody.sections(of: d.body).map { $0.normalizedTitle } == ["what i did", "finished", "started", "pending blocked", "to do next"])
}
test("old '\\## ' escapes unescape on load, outside code") {
    let store = freshStore("esc")
    try raw("# 2026-10-05\n\n## 📝 What I did\nline\n\\## fake\n\\\\## two\n  \\## indented stays\n\n```\n\\## code stays\n```\n", in: store.dir, day: "2026-10-05")
    let d = try store.load("2026-10-05")!
    let lines = d.body.components(separatedBy: "\n")
    expect(lines.contains("## fake") && lines.contains("\\## two") && lines.contains("  \\## indented stays") && lines.contains("\\## code stays"), "\(lines)")
}
test("file format: title, BOM, CRLF, skip markers, serialize") {
    let store = freshStore("fmt")
    func load(_ s: String) throws -> DayDocument { try raw(s, in: store.dir, day: "2026-10-05"); return try store.load("2026-10-05")! }
    try expect(try load("# 2026-10-05\n\nbody").body == "body")
    try expect(try load("body only").body == "body only" && (try load("\n\n# 2026-10-05\n\n\nbody\n\n\n").body == "body"))
    try expect(try load("# 2026-01-01\n\nbody").body == "body", "any date title is stripped")
    try expect(try load("# 2026-10-05 retro\n\nbody").body == "# 2026-10-05 retro\n\nbody", "a title with text is content")
    try expect(try load("# My plan\n\nbody").body == "# My plan\n\nbody")
    try expect(try load("\u{FEFF}# 2026-10-05\r\n\r\nline one\r\nline two\r\n").body == "line one\nline two")
    try expect(try load("# 2026-10-05\n\n> Skipped: Holiday\n").isSkipped && (try load("# 2026-10-05\n\n> Skipped: Holiday\n").skipReason == "Holiday"))
    try expect(try load("# 2026-10-05\n\n> Skipped\n\n\n").skipReason == "" && (try load("# 2026-10-05\n\n> Skipped\n").isSkipped))
    try expect(try load("# 2026-10-05\n\n> Skipped the meeting\n").isSkipped == false, "no colon: ordinary quote")
    try expect(try load("# 2026-10-05\n\n> Skipped: x\nmore text\n").isSkipped == false, "marker must be the only content")
    try expect(try load("# 2026-10-05\n\n> Skipped: Holiday\n").body == "", "skipped day has no body")
    expect(MarkdownFormat.serialize(day: "2026-10-05", body: "\n\nhello\n\n") == "# 2026-10-05\n\nhello\n")
    expect(MarkdownFormat.serializeSkip(day: "2026-10-05", reason: " a\nb ") == "# 2026-10-05\n\n> Skipped: a b\n")
    expect(MarkdownFormat.serializeSkip(day: "2026-10-05", reason: "") == "# 2026-10-05\n\n> Skipped\n")
}

// MARK: store
test("save/load: no escaping, unicode, images, idempotent, date-like first line") {
    let store = freshStore("rt")
    let body = "intro\n## Real heading\ntext\n\\## literal slash stays\n\n![](assets/2026-10-05-1a2b3c4d.png)\n\n- [ ] 今天完成 ✅ café"
    try store.save(day: "2026-10-05", body: body)
    try expect(try fileText(store.dir, "2026-10-05") == "# 2026-10-05\n\n" + body + "\n", "written verbatim, nothing is escaped")
    try expect(try store.load("2026-10-05")!.body.contains("\n## literal slash stays\n"), "documented trade-off: old-style escapes are undone on load")
    let b2 = "intro\n## Real heading\ntext\n\n![](assets/2026-10-05-1a2b3c4d.png)\n\n- [ ] 今天完成 ✅ café"
    try store.save(day: "2026-10-06", body: b2)
    try expect(try store.load("2026-10-06")!.body == b2)
    try expect(try fileText(store.dir, "2026-10-06").contains("\n## Real heading\n"), "headings are not escaped")
    try store.save(day: "2026-10-07", body: "# 2026-10-07\ntext under a date heading")
    try expect(try store.load("2026-10-07")!.body == "# 2026-10-07\ntext under a date heading", "only the first title line is stripped")
    for b in ["\n\nlead blank", "trail  \n\n", "  indented first", "a\r\nb"] {
        try store.save(day: "2026-10-08", body: b)
        let l = try store.load("2026-10-08")!.body
        expect(l == MarkdownBody.trimBody(b))
        try store.save(day: "2026-10-08", body: l)
        try expect(try store.load("2026-10-08")!.body == l)
    }
}
test("an emptied page is no log; empty save never creates a file or touches a skip marker") {
    let store = freshStore("empty")
    try store.save(day: "2026-10-05", body: "  \n \n")
    try expect(try store.listDays().isEmpty && (try store.load("2026-10-05")) == nil, "no file for an empty page")
    try store.save(day: "2026-10-05", body: "hello there")
    try expect(try store.listDays() == ["2026-10-05"])
    try store.save(day: "2026-10-05", body: "\n")
    try expect(try store.listDays().isEmpty, "emptied page removes the file")
    try store.skip("2026-10-06", reason: "Holiday")
    try store.save(day: "2026-10-06", body: "")
    try expect(try store.load("2026-10-06")!.isSkipped, "empty save keeps the skip marker")
    try store.save(day: "2026-10-06", body: "I worked anyway today")
    let d = try store.load("2026-10-06")!
    expect(!d.isSkipped && d.body == "I worked anyway today", "write a log anyway replaces the marker")
}
test("skip / unskip / skip range") {
    let store = freshStore("skip"); let dir = store.dir
    try store.skip("2026-10-06", reason: "Holiday")
    try expect(try fileText(dir, "2026-10-06").contains("> Skipped: Holiday"))
    let e = try store.load("2026-10-06")!
    expect(e.isSkipped && e.skipReason == "Holiday" && e.status(minWords: 20) == .skipped)
    try store.skip("2026-10-07")
    try expect(try fileText(dir, "2026-10-07").contains("> Skipped\n") && (try store.load("2026-10-07")!.skipReason == ""))
    try store.skip("2026-10-06", reason: "Leave")
    try expect(try store.load("2026-10-06")!.skipReason == "Leave", "re-skipping updates the reason")
    try expect(try store.unskip("2026-10-06") && (try store.load("2026-10-06")) == nil)
    try expect(try store.unskip("2026-10-06") == false)
    try store.save(day: "2026-10-08", body: lorem(30))
    do { try store.skip("2026-10-08"); expect(false, "should throw") } catch { expect(error as? LogError == .hasContent("2026-10-08")) }
    try expect(try store.unskip("2026-10-08") == false, "unskip never deletes a log")
    try store.save(day: "2026-10-09", body: "## What I did\n\n## Finished")
    try store.skip("2026-10-09", reason: "headings only")
    try expect(try store.load("2026-10-09")!.isSkipped, "a heading-only page may be skipped")
    try store.save(day: "2026-10-10", body: "![](assets/x.png)")
    do { try store.skip("2026-10-10"); expect(false, "an image is content") } catch { expect(error as? LogError == .hasContent("2026-10-10")) }
    let w = try store.skipRange(from: "2026-10-12", to: "2026-10-18", reason: "Leave", weekdays: workdays, calendar: cal)
    expect(w == ["2026-10-12", "2026-10-13", "2026-10-14", "2026-10-15", "2026-10-16"], "\(w)")
    try expect(try store.load("2026-10-17") == nil)
    try store.save(day: "2026-10-21", body: lorem(30))
    let w2 = try store.skipRange(from: "2026-10-20", to: "2026-10-22", weekdays: workdays, calendar: cal)
    expect(w2 == ["2026-10-20", "2026-10-22"], "days with writing are left alone")
    try store.save(day: "2026-10-20", body: lorem(25))
    try expect(try store.load("2026-10-20")!.status(minWords: 20) == .logged)
}
test("fileStates(minWords:) follows the words rule; listDays/folderSummary ignore assets and foreign files") {
    let store = freshStore("states"); let dir = store.dir
    try store.save(day: "2026-10-05", body: lorem(25))
    try store.save(day: "2026-10-06", body: lorem(5))
    try store.save(day: "2026-10-07", body: Settings.defaultTemplate)
    try store.skip("2026-10-08", reason: "x")
    try expect(try store.listDays() == ["2026-10-08", "2026-10-07", "2026-10-06", "2026-10-05"])
    try expect(try store.fileStates(minWords: 20) == ["2026-10-05": .logged, "2026-10-06": .partial, "2026-10-08": .skipped], "template-only page is omitted")
    try expect(try store.fileStates(minWords: 5) == ["2026-10-05": .logged, "2026-10-06": .logged, "2026-10-08": .skipped])
    try expect(try store.fileStates(minWords: 30) == ["2026-10-05": .partial, "2026-10-06": .partial, "2026-10-08": .skipped])
    try FileManager.default.createDirectory(at: dir.appendingPathComponent("assets"), withIntermediateDirectories: true)
    try "hi".write(to: dir.appendingPathComponent("notes.md"), atomically: true, encoding: .utf8)
    try "hi".write(to: dir.appendingPathComponent("2026-10-0x.md"), atomically: true, encoding: .utf8)
    try "hi".write(to: dir.appendingPathComponent(".hidden"), atomically: true, encoding: .utf8)
    try expect(try store.listDays().count == 4)
    try expect(try store.folderSummary() == FolderSummary(logs: 2, skipped: 1, unrecognized: 2))
    let docs = try store.documents(from: "2026-10-06", to: "2026-10-07")
    try expect(Set(docs.keys) == ["2026-10-06", "2026-10-07"] && (try store.allDocuments().count) == 4)
}
test("missing and unwritable folders give typed errors; atomic writes leave no temp files; bad days") {
    let missing = LogStore(dir: root.appendingPathComponent("nope-\(UUID().uuidString)"))
    do { try missing.save(day: "2026-10-05", body: "hello"); expect(false) } catch { expect(error as? LogError == .folderMissing(missing.dir.path)) }
    do { _ = try missing.listDays(); expect(false) } catch { expect(error as? LogError == .folderMissing(missing.dir.path)) }
    do { _ = try missing.load("2026-10-05"); expect(false) } catch { expect(error as? LogError == .folderMissing(missing.dir.path)) }
    do { try missing.skip("2026-10-05"); expect(false) } catch { expect(error as? LogError == .folderMissing(missing.dir.path)) }
    try missing.ensureFolder()
    try missing.save(day: "2026-10-05", body: "hello")
    try expect(try FileManager.default.contentsOfDirectory(atPath: missing.dir.path) == ["2026-10-05.md"])
    let rs = freshStore("ro"); let ro = rs.dir
    try rs.save(day: "2026-10-05", body: "hello there")
    try FileManager.default.setAttributes([.posixPermissions: 0o555], ofItemAtPath: ro.path)
    do { try rs.save(day: "2026-10-06", body: "x y z"); expect(false, "should throw") } catch { expect(error as? LogError == .folderNotWritable(ro.path), "\(error)") }
    do { try rs.skip("2026-10-07"); expect(false) } catch { expect(error as? LogError == .folderNotWritable(ro.path)) }
    do { _ = try rs.assets.save(data: pngBytes, ext: "png", day: "2026-10-05"); expect(false) } catch { expect(error as? LogError == .folderNotWritable(ro.path), "\(error)") }
    try expect(try rs.load("2026-10-05") != nil, "still readable")
    try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: ro.path)
    for bad in ["../evil", "2026-10-5", "x", "2026-10-05.md", ""] {
        do { try rs.save(day: bad, body: "x"); expect(false, bad) } catch { expect(error as? LogError == .badDay(bad), bad) }
    }
    do { _ = try rs.load("../x"); expect(false) } catch { expect(error as? LogError == .badDay("../x")) }
}

// MARK: backups
var fakeNow = mk(2026, 10, 6, 12, 0)   // 16:00:00 UTC (New York is on EDT)
func backupStore(_ name: String, interval: TimeInterval = 0) -> (store: LogStore, bdir: URL) {
    let bdir = freshDir(name + "-backups")
    return (LogStore(dir: freshDir(name), backupDir: bdir, minBackupInterval: interval, clock: { fakeNow }), bdir)
}
func bkNames(_ bdir: URL, _ day: String) -> [String] {
    ((try? FileManager.default.contentsOfDirectory(atPath: bdir.appendingPathComponent(day).path)) ?? []).sorted()
}
func bkText(_ b: BackupInfo) throws -> String { try String(contentsOf: b.url, encoding: .utf8) }
func isIO(_ e: Error) -> Bool { if case LogError.io = e { return true } else { return false } }
/// Safe accessor: a missing backup becomes a placeholder that makes the next call throw instead of trapping the test run.
func bk(_ l: [BackupInfo], _ i: Int) -> BackupInfo { (i >= 0 && i < l.count) ? l[i] : fakeBackup("00000000-000000.md") }
func fakeBackup(_ name: String = "20260101-000000.md") -> BackupInfo {
    BackupInfo(date: Date(), url: URL(fileURLWithPath: "/tmp/" + name), words: 0)
}

test("backup: overwrite copies the previous file first") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    let (store, bdir) = backupStore("bk-over"); let day = "2026-10-06"
    var o = try store.save(day: day, body: "first version " + lorem(25))
    expect(o == SaveOutcome() && store.listBackups(day: day).isEmpty, "nothing existed, nothing to back up")
    expect(!FileManager.default.fileExists(atPath: bdir.appendingPathComponent(day).path), "no backup folder before the first backup")
    let v1 = try fileText(store.dir, day)
    fakeNow = fakeNow.addingTimeInterval(30)
    o = try store.save(day: day, body: "second version " + lorem(30))
    expect(o.backedUp && o.backupWarning == nil && store.lastBackupWarning == nil)
    var list = store.listBackups(day: day)
    expect(list.count == 1 && bk(list, 0).words == 27 && bk(list, 0).date == fakeNow)
    try expect(try bkText(bk(list, 0)) == v1, "byte-for-byte copy of the replaced file")
    expect(bkNames(bdir, day) == ["20261006-160030.md"], "UTC time stamp: \(bkNames(bdir, day))")
    try expect(try fileText(store.dir, day).contains("second version"))
    fakeNow = fakeNow.addingTimeInterval(60)
    let v2 = try fileText(store.dir, day)
    try store.save(day: day, body: "third " + lorem(5))
    list = store.listBackups(day: day)
    try expect(list.map { $0.words } == [32, 27] && (try bkText(bk(list, 0))) == v2 && bk(list, 0).date > bk(list, 1).date, "newest first")
    try store.save(day: "2026-10-07", body: "a b c"); try store.save(day: "2026-10-07", body: "a b c d")
    expect(store.listBackups(day: "2026-10-07").count == 1 && store.listBackups(day: day).count == 2, "one folder per day")
    try expect(Set(try FileManager.default.contentsOfDirectory(atPath: bdir.path)) == ["2026-10-06", "2026-10-07"])
}
test("backup: deleting a page (empty body) keeps a copy; nothing worth keeping means no copy") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    let (store, bdir) = backupStore("bk-del"); let day = "2026-10-06"
    try store.save(day: day, body: "keep me " + lorem(25))
    let before = try fileText(store.dir, day)
    var o = try store.save(day: day, body: " \n\n ")
    try expect(o.backedUp && o.backupWarning == nil && (try store.load(day)) == nil, "file deleted")
    let list = store.listBackups(day: day)
    try expect(list.count == 1 && (try bkText(bk(list, 0))) == before && bk(list, 0).words == 27)
    try raw("# 2026-10-08\n", in: store.dir, day: "2026-10-08")
    o = try store.save(day: "2026-10-08", body: "")
    try expect(!o.backedUp && (try store.load("2026-10-08")) == nil && store.listBackups(day: "2026-10-08").isEmpty, "title-only file: nothing to keep")
    try store.skip("2026-10-09", reason: "x")
    o = try store.save(day: "2026-10-09", body: "")
    try expect(!o.backedUp && (try store.load("2026-10-09"))!.isSkipped && store.listBackups(day: "2026-10-09").isEmpty, "skip marker is neither copied nor deleted")
    o = try store.save(day: "2026-10-10", body: "")
    expect(!o.backedUp && store.listBackups(day: "2026-10-10").isEmpty, "no file at all")
    try expect(Set(try FileManager.default.contentsOfDirectory(atPath: bdir.path)) == ["2026-10-06"])
}
test("backup: only the newest 10 per day are kept (names keep increasing on a frozen clock)") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    let (store, bdir) = backupStore("bk-prune"); let day = "2026-10-06"
    expect(LogStore.maxBackupsPerDay == 10)
    try store.save(day: "2026-10-07", body: "other day v1"); try store.save(day: "2026-10-07", body: "other day v2")
    for i in 1...15 { try store.save(day: day, body: "version \(i) " + lorem(25)) }   // the clock never moves
    let list = store.listBackups(day: day)
    expect(list.count == 10 && bkNames(bdir, day).count == 10, "\(list.count)")
    try expect(try bkText(bk(list, 0)).contains("version 14 ") && (try bkText(bk(list, list.count - 1))).contains("version 5 "), "versions 5...14 survive")
    expect(zip(list, list.dropFirst()).allSatisfy { $0.date > $1.date }, "strictly newest first")
    try expect(try fileText(store.dir, day).contains("version 15 "))
    expect(store.listBackups(day: "2026-10-07").count == 1, "pruning is per day")
}
test("backup: identical content is a no-op; skip markers and title-only files are not copied") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    let (store, _) = backupStore("bk-same"); let day = "2026-10-06"
    try store.save(day: day, body: "same text " + lorem(25))
    func mtime() throws -> Date { try FileManager.default.attributesOfItem(atPath: store.url(for: day).path)[.modificationDate] as! Date }
    let m0 = try mtime()
    Thread.sleep(forTimeInterval: 0.05)
    for b in ["same text " + lorem(25), "same text " + lorem(25) + "\n\n  ", "\n\nsame text " + lorem(25)] {
        let o = try store.save(day: day, body: b)
        expect(o == SaveOutcome(), "identical after normalisation: \(o)")
    }
    try expect(store.listBackups(day: day).isEmpty && (try mtime()) == m0, "no copy and the file was not rewritten")
    try store.save(day: day, body: "changed " + lorem(25))
    expect(store.listBackups(day: day).count == 1)
    try store.save(day: day, body: "changed " + lorem(25))
    expect(store.listBackups(day: day).count == 1, "saving the same thing again adds nothing")
    try store.skip("2026-10-07", reason: "Holiday")
    let o = try store.save(day: "2026-10-07", body: "worked anyway " + lorem(25))
    try expect(!o.backedUp && store.listBackups(day: "2026-10-07").isEmpty && !(try store.load("2026-10-07")!.isSkipped), "a skip marker is not worth a copy")
    try raw("# 2026-10-08\n\n", in: store.dir, day: "2026-10-08")
    try expect(!(try store.save(day: "2026-10-08", body: "now text")).backedUp && store.listBackups(day: "2026-10-08").isEmpty, "title-only file")
    let legacy = "# 2026-10-09\n\n## 📝 What I did\nWrote code\n\n## ✅ Finished\nShipped it\n\n"
    try raw(legacy, in: store.dir, day: "2026-10-09")
    let o2 = try store.save(day: "2026-10-09", body: try store.load("2026-10-09")!.body)
    try expect(o2.backedUp && (try bkText(bk(store.listBackups(day: "2026-10-09"), 0))) == legacy, "re-normalising an old file keeps its original bytes")
}
test("backup: failures never fail the save; unwritable backup dir; restore refuses without its safety copy") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    let day = "2026-10-06"
    let blocker = root.appendingPathComponent("blocker-\(UUID().uuidString)")
    try "i am a file".write(to: blocker, atomically: true, encoding: .utf8)
    let a = LogStore(dir: freshDir("bk-a"), backupDir: blocker, clock: { fakeNow })
    try a.save(day: day, body: "v1 " + lorem(25))
    let oa = try a.save(day: day, body: "v2 " + lorem(25))
    expect(!oa.backedUp && oa.backupWarning != nil && a.lastBackupWarning == oa.backupWarning, "\(oa)")
    try expect(try fileText(a.dir, day).contains("v2 ") && a.listBackups(day: day).isEmpty, "backup dir that is a file: the save still happened")

    let (b, bdir) = backupStore("bk-b")
    try b.save(day: day, body: "v1 " + lorem(25))
    let fm = FileManager.default
    try fm.setAttributes([.posixPermissions: 0o555], ofItemAtPath: bdir.path)
    let o2 = try b.save(day: day, body: "v2 " + lorem(25))
    expect(!o2.backedUp && o2.backupWarning != nil && b.lastBackupWarning == o2.backupWarning)
    try expect(try fileText(b.dir, day).contains("v2 "), "unwritable backup dir: the save itself went through")
    let o3 = try b.save(day: day, body: "")
    try expect(!o3.backedUp && o3.backupWarning != nil && (try b.load(day)) == nil, "deleting is also never blocked by a failed backup")
    try fm.setAttributes([.posixPermissions: 0o755], ofItemAtPath: bdir.path)
    try b.save(day: day, body: "v3 " + lorem(25))
    let o4 = try b.save(day: day, body: "v4 " + lorem(25))
    expect(o4.backedUp && o4.backupWarning == nil && b.lastBackupWarning == nil && b.listBackups(day: day).count == 1, "recovers on its own and clears the warning")
    let v4 = try fileText(b.dir, day)
    let old = bk(b.listBackups(day: day), 0)
    let dayDir = bdir.appendingPathComponent(day)
    try fm.setAttributes([.posixPermissions: 0o555], ofItemAtPath: dayDir.path)
    do { _ = try b.restoreBackup(day: day, backup: old); expect(false, "restore needs its safety copy") }
    catch { if case LogError.backupFailed = error { expect(true) } else { expect(false, "\(error)") } }
    try expect(try fileText(b.dir, day) == v4 && b.lastBackupWarning != nil, "nothing was changed")
    try fm.setAttributes([.posixPermissions: 0o755], ofItemAtPath: dayDir.path)
    try b.restoreBackup(day: day, backup: old)
    try expect(try fileText(b.dir, day).contains("v3 ") && b.lastBackupWarning == nil, "works again once the folder is writable")
}
test("backup: restore round trip (restore backs up the current file first)") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    let (store, _) = backupStore("bk-restore"); let day = "2026-10-06"
    var raws = [String]()
    for i in 1...3 {
        fakeNow = fakeNow.addingTimeInterval(10)
        try store.save(day: day, body: "v\(i) " + lorem(25)); raws.append(try fileText(store.dir, day))
    }
    var list = store.listBackups(day: day)   // [v2, v1]
    try expect(list.count == 2 && (try bkText(bk(list, 0))) == raws[1] && (try bkText(bk(list, 1))) == raws[0])
    fakeNow = fakeNow.addingTimeInterval(10)
    let doc = try store.restoreBackup(day: day, backup: bk(list, 1))   // back to v1
    try expect(doc.day == day && doc.body.hasPrefix("v1 ") && (try fileText(store.dir, day)) == raws[0] && (try store.load(day)) == doc)
    list = store.listBackups(day: day)   // [v3 (the safety copy), v2, v1]
    try expect(list.count == 3 && (try bkText(bk(list, 0))) == raws[2], "the current file was copied first")
    fakeNow = fakeNow.addingTimeInterval(10)
    try store.restoreBackup(day: day, backup: bk(list, 0))   // undo the restore
    try expect(try fileText(store.dir, day) == raws[2])
    list = store.listBackups(day: day)   // [v1 (safety copy), v3, v2, v1]
    try expect(list.count == 4 && (try bkText(bk(list, 0))) == raws[0])
    try store.restoreBackup(day: day, backup: bk(list, 1))   // identical to the current file
    try expect(store.listBackups(day: day).count == 4 && (try fileText(store.dir, day)) == raws[2], "identical: no write, no copy")
    // over a deleted page and over a skip marker
    try store.save(day: day, body: "")
    try expect(try store.load(day) == nil)
    list = store.listBackups(day: day)
    try store.restoreBackup(day: day, backup: bk(list, 0))
    try expect(try fileText(store.dir, day) == raws[2], "a deleted page comes back")
    try store.save(day: "2026-10-07", body: "s1 " + lorem(25)); try store.save(day: "2026-10-07", body: "s2 " + lorem(25))
    let s2 = try fileText(store.dir, "2026-10-07")
    try store.save(day: "2026-10-07", body: ""); try store.skip("2026-10-07", reason: "x")
    let sl = store.listBackups(day: "2026-10-07")   // [s2, s1]
    try store.restoreBackup(day: "2026-10-07", backup: bk(sl, 0))
    try expect(try fileText(store.dir, "2026-10-07") == s2 && store.listBackups(day: "2026-10-07").count == 2, "a skip marker is replaced without a copy")
    // forged or foreign backups
    for bad in [fakeBackup("passwd"), fakeBackup(), fakeBackup("../x.md"), BackupInfo(date: Date(), url: bk(list, 0).url.deletingLastPathComponent(), words: 0)] {
        do { _ = try store.restoreBackup(day: day, backup: bad); expect(false, bad.url.path) } catch { expect(isIO(error), "\(error)") }
    }
    do { _ = try store.restoreBackup(day: "2026-10-09", backup: bk(list, 0)); expect(false) } catch { expect(isIO(error), "a name is only looked up inside the given day's folder") }
    do { _ = try store.restoreBackup(day: "../x", backup: bk(list, 0)); expect(false) } catch { expect(error as? LogError == .badDay("../x")) }
    let plain = freshStore("bk-plain")
    do { _ = try plain.restoreBackup(day: day, backup: bk(list, 0)); expect(false) } catch { expect(error as? LogError == .backupsDisabled) }
}
test("backup: backups are never listed, searched or counted") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    for nested in [false, true] {
        let dir = freshDir("bk-ex")
        let bdir = nested ? dir.appendingPathComponent("backups") : freshDir("bk-ex-out")
        let store = LogStore(dir: dir, backupDir: bdir, clock: { fakeNow })
        try store.save(day: "2026-10-06", body: "zebra " + lorem(25))
        try store.save(day: "2026-10-06", body: "giraffe " + lorem(25))
        try store.save(day: "2026-10-07", body: "zebra again " + lorem(25))
        try store.save(day: "2026-10-07", body: "okapi " + lorem(25))
        expect(store.listBackups(day: "2026-10-06").count == 1 && FileManager.default.fileExists(atPath: bdir.path))
        try expect(try store.listDays() == ["2026-10-07", "2026-10-06"], "nested=\(nested)")
        try expect(try store.fileStates(minWords: 20) == ["2026-10-06": .logged, "2026-10-07": .logged])
        try expect(try store.search("zebra").isEmpty && (try store.search("giraffe").map { $0.day }) == ["2026-10-06"] && (try store.search("okapi")).count == 1, "old text lives only in backups")
        try expect(try store.allDocuments().count == 2 && (try store.documents(from: "2000-01-01", to: "2100-01-01").count) == 2)
        try expect(try store.folderSummary() == FolderSummary(logs: 2, skipped: 0, unrecognized: 0), "nested=\(nested)")
        try expect(try store.load("2026-10-06")!.body.hasPrefix("giraffe"))
    }
}
test("backup: UTC names, junk ignored, clock set back, throttle, off by default") {
    var utc = Calendar(identifier: .gregorian); utc.timeZone = TimeZone(identifier: "GMT")!
    let fm = FileManager.default
    // DST fall-back: 01:30 happens twice in New York; the UTC names stay distinct and ordered
    let t1 = utc.date(from: DateComponents(year: 2026, month: 11, day: 1, hour: 5, minute: 30))!
    let t2 = t1.addingTimeInterval(3600)
    expect(cal.component(.hour, from: t1) == 1 && cal.component(.hour, from: t2) == 1 && cal.component(.minute, from: t2) == 30, "same wall-clock time twice")
    let (store, bdir) = backupStore("bk-names"); let day = "2026-11-01"
    fakeNow = t1.addingTimeInterval(-60)
    try store.save(day: day, body: "a " + lorem(25))
    fakeNow = t1; try store.save(day: day, body: "b " + lorem(25))
    fakeNow = t2; try store.save(day: day, body: "c " + lorem(25))
    expect(bkNames(bdir, day) == ["20261101-053000.md", "20261101-063000.md"], "\(bkNames(bdir, day))")
    expect(store.listBackups(day: day).map { $0.date } == [t2, t1])
    // junk next to real backups; a valid name holding non-UTF-8 bytes lists with 0 words
    let dayDir = bdir.appendingPathComponent(day)
    for junk in ["notes.txt", ".DS_Store", "20261101-053000.md.tmp", "bad-name.md", "20261301-160000.md", "2026110-0530000.md", "20261101_053000.md"] {
        try "x".write(to: dayDir.appendingPathComponent(junk), atomically: true, encoding: .utf8)
    }
    try Data([0xFF, 0xFE, 0xFD]).write(to: dayDir.appendingPathComponent("20260101-000000.md"))
    let l0 = store.listBackups(day: day)
    expect(l0.count == 3 && bk(l0, 2).words == 0 && bk(l0, 2).date == utc.date(from: DateComponents(year: 2026, month: 1, day: 1)), "\(l0.count)")
    expect(store.listBackups(day: "../x").isEmpty && store.listBackups(day: "nope").isEmpty && store.listBackups(day: "2026-12-31").isEmpty)
    do { _ = try store.restoreBackup(day: day, backup: bk(l0, 2)); expect(false) } catch { expect(isIO(error), "non-UTF-8 backup") }
    // a clock set back never makes a new copy sort older (and so prunable)
    fakeNow = t2.addingTimeInterval(-86400 * 3)
    try store.save(day: day, body: "d " + lorem(25))
    let l1 = store.listBackups(day: day)
    try expect(l1.count == 4 && bk(l1, 0).date == t2.addingTimeInterval(1) && (try bkText(bk(l1, 0))).contains("c "), "newest copy is still first: \(l1.map { $0.date })")
    // throttle: overwrites skip the copy inside the interval; deletes and restores always copy
    fakeNow = mk(2026, 10, 6, 12, 0)
    let (ts, _) = backupStore("bk-throttle", interval: 60); let d2 = "2026-10-06"
    try ts.save(day: d2, body: "v1 " + lorem(25))
    fakeNow = fakeNow.addingTimeInterval(5); try ts.save(day: d2, body: "v2 " + lorem(25))
    fakeNow = fakeNow.addingTimeInterval(5); let skipped = try ts.save(day: d2, body: "v3 " + lorem(25))
    try expect(!skipped.backedUp && skipped.backupWarning == nil && ts.listBackups(day: d2).count == 1 && (try bkText(bk(ts.listBackups(day: d2), 0))).contains("v1 "), "inside the interval")
    fakeNow = fakeNow.addingTimeInterval(120); try ts.save(day: d2, body: "v4 " + lorem(25))
    try expect(ts.listBackups(day: d2).count == 2 && (try bkText(bk(ts.listBackups(day: d2), 0))).contains("v3 "), "interval over: copy of v3")
    fakeNow = fakeNow.addingTimeInterval(1); try ts.save(day: d2, body: "")
    try expect(ts.listBackups(day: d2).count == 3 && (try bkText(bk(ts.listBackups(day: d2), 0))).contains("v4 "), "delete ignores the interval")
    fakeNow = fakeNow.addingTimeInterval(1); try ts.restoreBackup(day: d2, backup: bk(ts.listBackups(day: d2), 0))
    fakeNow = fakeNow.addingTimeInterval(1); try ts.restoreBackup(day: d2, backup: bk(ts.listBackups(day: d2), 2))
    expect(ts.listBackups(day: d2).count == 4, "restore ignores the interval: the current page was copied before the oldest backup came back")
    // off by default: nothing is created, nothing breaks
    let off = freshStore("bk-off")
    try off.save(day: "2026-10-06", body: "a " + lorem(25))
    try expect(try off.save(day: "2026-10-06", body: "b " + lorem(25)) == SaveOutcome() && off.listBackups(day: "2026-10-06").isEmpty && off.lastBackupWarning == nil && off.backupDir == nil)
    try off.save(day: "2026-10-06", body: "")
    try expect(try off.load("2026-10-06") == nil && (try fm.contentsOfDirectory(atPath: off.dir.path)).isEmpty)
    do { _ = try off.restoreBackup(day: "2026-10-06", backup: fakeBackup()); expect(false) } catch { expect(error as? LogError == .backupsDisabled) }
}

// MARK: settings
test("settings defaults, round trip, tolerant decode, normalize") {
    let d = Settings()
    expect(d.mode == .strict && d.minWords == 20 && d.carryOverHeadings == ["To do next", "Pending / blocked"] && d.reminderMinutes == 16 * 60 + 55)
    expect(d.template == "## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next")
    let ms = MemoryStore()
    expect(Settings.load(from: ms) == Settings(), "defaults when empty")
    var s = Settings()
    s.mode = .gentle; s.reminderMinutes = 17 * 60; s.weekdays = [2, 4]; s.snoozeMinutes = 30
    s.storageFolder = URL(fileURLWithPath: "/tmp/x"); s.onboarded = true; s.launchAtLogin = false
    s.template = "## Wins\n\n## Next"; s.carryOverHeadings = ["Next"]; s.minWords = 50
    s.save(to: ms)
    expect(Settings.load(from: ms) == s)
    ms.setData("{\"mode\":\"gentle\"}".data(using: .utf8), forKey: Settings.storageKey)
    let t = Settings.load(from: ms)
    expect(t.mode == .gentle && t.reminderMinutes == 16 * 60 + 55 && t.weekdays == workdays && t.template == Settings.defaultTemplate && t.minWords == 20)
    ms.setData("garbage".data(using: .utf8), forKey: Settings.storageKey)
    expect(Settings.load(from: ms) == Settings())
    var bad = Settings(); bad.weekdays = []; bad.snoozeMinutes = 7; bad.reminderMinutes = 5000; bad.minWords = 0
    bad.carryOverHeadings = ["  Next  ", "", "   "]
    let n = bad.normalized()
    expect(n.weekdays == workdays && n.snoozeMinutes == 15 && n.reminderMinutes == 1439 && n.minWords == 1 && n.carryOverHeadings == ["Next"])
    bad.minWords = 9999; expect(bad.normalized().minWords == 500)
    bad.minWords = -3; expect(bad.normalized().minWords == 1)
    var empty = Settings(); empty.template = ""; empty.carryOverHeadings = []
    empty.save(to: ms); let e = Settings.load(from: ms)
    expect(e.template == "" && e.carryOverHeadings.isEmpty, "an empty template / no carry-over headings are allowed")
    let ud = UserDefaults(suiteName: "dailylog.tests.\(UUID().uuidString)")!
    s.save(to: ud); expect(Settings.load(from: ud) == s, "UserDefaults conforms")
}
test("settings migration from v0.2 JSON") {
    let ms = MemoryStore()
    let old = """
    {"mode":"gentle","reminderMinutes":1020,"weekdays":[2,3],"snoozeMinutes":20,"launchAtLogin":false,"onboarded":true,
     "storageFolder":"file:///tmp/old-logs/","carryOverSectionID":"pending",
     "sections":[{"id":"did","title":"📝 What I did","hint":"h","required":true},{"id":"x","title":"  Wins  ","hint":"","required":false},
                 {"id":"y","title":"   ","hint":"","required":true},{"id":"todo","title":"📌 To do next","hint":"","required":true}]}
    """
    ms.setData(old.data(using: .utf8), forKey: Settings.storageKey)
    let s = Settings.load(from: ms)
    expect(s.template == "## 📝 What I did\n\n## Wins\n\n## 📌 To do next", "\(s.template)")
    expect(s.mode == .gentle && s.reminderMinutes == 1020 && s.weekdays == [2, 3] && s.snoozeMinutes == 20 && s.onboarded && !s.launchAtLogin)
    expect(s.storageFolder.path == "/tmp/old-logs" && s.minWords == 20 && s.carryOverHeadings == Settings.defaultCarryOverHeadings)
    expect(MarkdownBody.headings(inTemplate: s.template).map { MarkdownBody.normalizeHeading($0) } == ["what i did", "wins", "to do next"])
    s.save(to: ms)
    let json = String(data: ms.data(forKey: Settings.storageKey)!, encoding: .utf8)!
    expect(!json.contains("\"sections\"") && !json.contains("carryOverSectionID") && json.contains("\"template\""), "re-saved in the new shape")
    expect(Settings.load(from: ms) == s)
    // an explicit template wins over old sections; empty/blank sections fall back to the default
    ms.setData("{\"template\":\"## Mine\",\"sections\":[{\"id\":\"a\",\"title\":\"Old\"}]}".data(using: .utf8), forKey: Settings.storageKey)
    expect(Settings.load(from: ms).template == "## Mine")
    ms.setData("{\"sections\":[]}".data(using: .utf8), forKey: Settings.storageKey)
    expect(Settings.load(from: ms).template == Settings.defaultTemplate)
    ms.setData("{\"sections\":[{\"id\":\"a\",\"title\":\"  \"}]}".data(using: .utf8), forKey: Settings.storageKey)
    expect(Settings.load(from: ms).template == Settings.defaultTemplate)
    ms.setData("{\"sections\":\"nonsense\"}".data(using: .utf8), forKey: Settings.storageKey)
    expect(Settings.load(from: ms).template == Settings.defaultTemplate, "unreadable old sections are ignored")
}

// MARK: assets
test("AssetStore.save: names, dedupe, ext allowlist, size cap") {
    let store = freshStore("assets-save"); let a = store.assets
    let p = try a.save(data: Data("hello".utf8), ext: "png", day: "2026-10-06")
    expect(p == "assets/2026-10-06-2cf24dba.png", "SHA-256 prefix of the bytes: \(p)")
    expect(FileManager.default.fileExists(atPath: store.dir.appendingPathComponent(p).path))
    try expect(try a.save(data: Data("hello".utf8), ext: "png", day: "2026-10-06") == p, "same bytes, same day, same path")
    try expect(try FileManager.default.contentsOfDirectory(atPath: store.dir.appendingPathComponent("assets").path).count == 1)
    try expect(try a.save(data: Data("hello".utf8), ext: "png", day: "2026-10-07") == "assets/2026-10-07-2cf24dba.png")
    let q = try a.save(data: pngBytes, ext: "png", day: "2026-10-06")
    expect(q != p && q.range(of: "^assets/2026-10-06-[0-9a-f]{8}\\.png$", options: .regularExpression) != nil, q)
    for (ext, want) in [("PNG", "png"), (".jpg", "jpg"), ("jpeg", "jpeg"), ("GIF", "gif"), (" webp ", "webp")] {
        let r = try a.save(data: Data("x-\(ext)".utf8), ext: ext, day: "2026-10-06")
        expect(r.hasSuffix("." + want) && r.hasPrefix("assets/2026-10-06-"), r)
    }
    for ext in ["svg", "exe", "png/../x", "", "md", "heic", "png.exe", "../png"] {
        do { _ = try a.save(data: pngBytes, ext: ext, day: "2026-10-06"); expect(false, ext) } catch { expect(error as? LogError == .unsupportedAsset(ext), "\(ext) \(error)") }
    }
    do { _ = try a.save(data: Data(), ext: "png", day: "2026-10-06"); expect(false) } catch { expect(error as? LogError == .emptyAsset) }
    for bad in ["../x", "2026-10-6", "assets"] {
        do { _ = try a.save(data: pngBytes, ext: "png", day: bad); expect(false, bad) } catch { expect(error as? LogError == .badDay(bad)) }
    }
    let big = Data(count: AssetStore.maxBytes + 1)
    do { _ = try a.save(data: big, ext: "png", day: "2026-10-06"); expect(false) } catch { expect(error as? LogError == .assetTooLarge(AssetStore.maxBytes + 1)) }
    let exact = Data(count: AssetStore.maxBytes)
    try expect(AssetStore.maxBytes == 20 * 1024 * 1024 && (try a.save(data: exact, ext: "png", day: "2026-10-06")).hasPrefix("assets/"), "exactly 20 MB is accepted")
    let nofolder = AssetStore(dir: root.appendingPathComponent("no-such-\(UUID().uuidString)"))
    do { _ = try nofolder.save(data: pngBytes, ext: "png", day: "2026-10-06"); expect(false) } catch { expect(error as? LogError == .folderMissing(nofolder.dir.path)) }
}
test("AssetStore.fileExtension(mime:name:)") {
    let f = AssetStore.fileExtension
    expect(f("image/png", nil) == "png" && f("IMAGE/JPEG; charset=binary", "x.png") == "jpg" && f("image/jpg", nil) == "jpg")
    expect(f("image/gif", "a.png") == "gif" && f("image/webp", nil) == "webp" && f(" image/png ", "") == "png")
    expect(f("application/octet-stream", "Photo.JPEG") == "jpeg" && f(nil, "pic.webp") == "webp" && f("", "pic.gif") == "gif")
    expect(f("image/svg+xml", "a.svg") == nil && f(nil, nil) == nil && f("text/plain", "notes.md") == nil && f("image/heic", "a.heic") == nil)
}
test("AssetStore.resolve: inside the folder only") {
    let store = freshStore("assets-resolve"); let a = store.assets; let dir = store.dir
    let p = try a.save(data: pngBytes, ext: "png", day: "2026-10-06")
    let real = AssetStore.realPath(dir.appendingPathComponent(p))!
    expect(a.resolve(p)?.path == real && FileManager.default.fileExists(atPath: real))
    expect(a.resolve("./" + p)?.path == real && a.resolve("assets//" + String(p.dropFirst(7)))?.path == real, "normalised")
    expect(a.resolve("") == nil && a.resolve("assets/") == nil && a.resolve("assets") == nil)
    for bad in ["../outside.png", "assets/../../outside.png", "assets/../" + String(p.dropFirst(7)), "assets/..", "..", "a/b/../../../x.png"] {
        expect(a.resolve(bad) == nil, bad)
    }
    expect(a.resolve("/etc/passwd.png") == nil && a.resolve("/" + p) == nil && a.resolve(real) == nil, "absolute paths")
    expect(a.resolve("file:///etc/hosts.png") == nil && a.resolve("https://example.com/a.png") == nil)
    expect(a.resolve("%2e%2e/x.png") == nil && a.resolve("assets/nope.png") == nil, "missing file")
    try "notes".write(to: dir.appendingPathComponent("notes.md"), atomically: true, encoding: .utf8)
    try "x".write(to: dir.appendingPathComponent("assets/a.svg"), atomically: true, encoding: .utf8)
    try "x".write(to: dir.appendingPathComponent("assets/noext"), atomically: true, encoding: .utf8)
    expect(a.resolve("notes.md") == nil && a.resolve("assets/a.svg") == nil && a.resolve("assets/noext") == nil, "non-image extensions")
    try FileManager.default.createDirectory(at: dir.appendingPathComponent("assets/dir.png"), withIntermediateDirectories: true)
    expect(a.resolve("assets/dir.png") == nil, "a directory named like an image")
    try pngBytes.write(to: dir.appendingPathComponent("loose.JPG"))
    expect(a.resolve("loose.JPG") != nil, "images elsewhere in the folder are fine, ext case-insensitive")
    // dlasset:// URLs
    expect(a.resolve(assetURL: URL(string: "dlasset://local/" + p)!)?.path == real)
    expect(a.resolve(assetURL: URL(string: "https://local/" + p)!) == nil && a.resolve(assetURL: URL(string: "dlasset://local/assets/%2e%2e/%2e%2e/x.png")!) == nil)
    expect(a.resolve(assetURL: URL(string: "dlasset://local/")!) == nil)
}
test("AssetStore: symlinks cannot escape the storage folder") {
    let store = freshStore("assets-link"); let dir = store.dir; let a = store.assets
    let outside = freshDir("outside")
    try pngBytes.write(to: outside.appendingPathComponent("secret.png"))
    try "top secret".write(to: outside.appendingPathComponent("passwords.txt"), atomically: true, encoding: .utf8)
    let assets = dir.appendingPathComponent("assets")
    try FileManager.default.createDirectory(at: assets, withIntermediateDirectories: true)
    let fm = FileManager.default
    try fm.createSymbolicLink(at: assets.appendingPathComponent("evil.png"), withDestinationURL: outside.appendingPathComponent("secret.png"))
    try fm.createSymbolicLink(at: assets.appendingPathComponent("linkdir"), withDestinationURL: outside)
    try fm.createSymbolicLink(at: assets.appendingPathComponent("txt.png"), withDestinationURL: outside.appendingPathComponent("passwords.txt"))
    try fm.createSymbolicLink(atPath: assets.appendingPathComponent("rel.png").path, withDestinationPath: "../../" + outside.lastPathComponent + "/secret.png")
    expect(a.resolve("assets/evil.png") == nil, "file symlink leaving the folder")
    expect(a.resolve("assets/linkdir/secret.png") == nil, "directory symlink leaving the folder")
    expect(a.resolve("assets/txt.png") == nil && a.resolve("assets/rel.png") == nil, "png-named links to other files")
    // links that stay inside are fine, but only to images
    let p = try a.save(data: pngBytes, ext: "png", day: "2026-10-06")
    try fm.createSymbolicLink(at: assets.appendingPathComponent("alias.png"), withDestinationURL: dir.appendingPathComponent(p))
    try "notes".write(to: dir.appendingPathComponent("notes.md"), atomically: true, encoding: .utf8)
    try fm.createSymbolicLink(at: assets.appendingPathComponent("md.png"), withDestinationURL: dir.appendingPathComponent("notes.md"))
    expect(a.resolve("assets/alias.png")?.path == AssetStore.realPath(dir.appendingPathComponent(p)), "alias inside the folder resolves to the real file")
    expect(a.resolve("assets/md.png") == nil, "a link to a non-image inside the folder")
    // the storage folder itself reached through a symlink still works
    let linkToStore = root.appendingPathComponent("storage-link-\(UUID().uuidString)")
    try fm.createSymbolicLink(at: linkToStore, withDestinationURL: dir)
    expect(AssetStore(dir: linkToStore).resolve(p) != nil, "root given as a symlink")
    // a symlinked assets/ directory pointing outside: no saves, no resolves
    let store2 = freshStore("assets-link2"); let outside2 = freshDir("outside2")
    try pngBytes.write(to: outside2.appendingPathComponent("a.png"))
    try fm.createSymbolicLink(at: store2.dir.appendingPathComponent("assets"), withDestinationURL: outside2)
    do { _ = try store2.assets.save(data: pngBytes, ext: "png", day: "2026-10-06"); expect(false) } catch { expect(error as? LogError == .badPath("assets"), "\(error)") }
    expect(store2.assets.resolve("assets/a.png") == nil, "everything behind an escaping assets/ link is refused")
    try expect(try fm.contentsOfDirectory(atPath: outside2.path) == ["a.png"], "nothing was written outside")
}
// MARK: streak + heatmap
func states(_ pairs: [(String, DayStatus)]) -> [String: DayStatus] { Dictionary(uniqueKeysWithValues: pairs) }
test("streak across weekend, skip and miss") {
    let mon12 = mk(2026, 10, 12, 9)  // Monday, nothing logged yet today
    let l: DayStatus = .logged
    var s = states([("2026-10-05", l), ("2026-10-06", l), ("2026-10-07", l), ("2026-10-08", l), ("2026-10-09", l)])
    expect(Streak.compute(states: s, now: mon12, calendar: cal, weekdays: workdays) == StreakResult(current: 5, best: 5), "weekend neutral, today not yet")
    s["2026-10-12"] = l
    expect(Streak.compute(states: s, now: mon12, calendar: cal, weekdays: workdays).current == 6, "today logged +1")
    s["2026-10-12"] = nil; s["2026-10-07"] = .skipped
    expect(Streak.compute(states: s, now: mon12, calendar: cal, weekdays: workdays).current == 4, "skip neutral")
    s["2026-10-07"] = nil
    expect(Streak.compute(states: s, now: mon12, calendar: cal, weekdays: workdays) == StreakResult(current: 2, best: 2), "miss breaks")
    s["2026-10-07"] = .partial
    expect(Streak.compute(states: s, now: mon12, calendar: cal, weekdays: workdays).current == 2, "partial past day breaks")
    s["2026-10-07"] = l; s["2026-10-10"] = l  // Saturday extra effort
    expect(Streak.compute(states: s, now: mon12, calendar: cal, weekdays: workdays).current == 6, "non-scheduled log +1")
    // today is Tuesday unlogged, Monday missed -> broken
    s["2026-10-12"] = nil
    expect(Streak.compute(states: s, now: mk(2026, 10, 13, 9), calendar: cal, weekdays: workdays).current == 0, "yesterday missed")
    expect(Streak.compute(states: [:], now: mon12, calendar: cal, weekdays: workdays) == StreakResult(current: 0, best: 0))
    // schedule change re-evaluates: Friday off
    let noFri: Set<Int> = [2, 3, 4, 5]
    let t = states([("2026-10-05", l), ("2026-10-06", l), ("2026-10-07", l), ("2026-10-08", l)])
    expect(Streak.compute(states: t, now: mon12, calendar: cal, weekdays: noFri).current == 4)
    expect(Streak.compute(states: t, now: mon12, calendar: cal, weekdays: workdays).current == 0, "Fri 9 missed under Mon-Fri")
    // best survives a later break
    let b = states([("2026-10-05", l), ("2026-10-06", l), ("2026-10-07", l), ("2026-10-09", l)])
    expect(Streak.compute(states: b, now: mon12, calendar: cal, weekdays: workdays) == StreakResult(current: 1, best: 3))
}
test("heatmap: 12 weeks, statuses, today ring") {
    let now = mk(2026, 10, 7, 10)  // Wed
    let st = states([("2026-10-05", .logged), ("2026-10-06", .skipped), ("2026-09-30", .partial)])
    let hm = Heatmap.weeks(states: st, now: now, calendar: cal, weekdays: workdays)
    expect(hm.count == 12 && hm.allSatisfy { $0.count == 7 })
    let last = hm.last!
    expect(last.map { $0.day } == ["2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09", "2026-10-10", "2026-10-11"], "week starts Monday")
    expect(last.map { $0.status } == [.logged, .skipped, .missed, .future, .future, .future, .future])
    expect(last.map { $0.isToday } == [false, false, true, false, false, false, false])
    expect(hm[10][2].status == .partial && hm[10][5].status == .off, "Saturday off")
    expect(hm[0][0].status == .off, "before first entry is not 'missed'")
    expect(hm[10][3].status == .missed || hm[10][3].day == "2026-10-01", "after first entry, empty workday is missed")
    expect(hm[10][3].status == .missed)
}
test("DST: day keys, heatmap and streak across fall-back and spring-forward") {
    // Fall back 2026-11-01 (25h day), spring forward 2026-03-08 (23h day) in New York.
    expect(DayKey.string(mk(2026, 11, 1, 23, 30), cal) == "2026-11-01")
    expect(DayKey.adding("2026-11-01", 1, cal) == "2026-11-02" && DayKey.adding("2026-03-08", 1, cal) == "2026-03-09")
    expect(DayKey.adding("2026-11-02", -1, cal) == "2026-11-01" && DayKey.adding("2026-03-09", -1, cal) == "2026-03-08")
    expect(DayKey.date("2026-02-30", cal) == nil && DayKey.date("2026-03-08", cal) != nil)
    for now in [mk(2026, 11, 3, 10), mk(2026, 3, 10, 10)] {
        let hm = Heatmap.weeks(states: ["2026-10-01": .logged], now: now, calendar: cal, weekdays: workdays).flatMap { $0 }
        expect(hm.count == 84 && Set(hm.map { $0.day }).count == 84, "unique days")
        var ok = true
        for i in 1..<hm.count where DayKey.adding(hm[i - 1].day, 1, cal) != hm[i].day { ok = false }
        expect(ok, "consecutive across DST")
    }
    let every: Set<Int> = Set(1...7)
    let s = states([("2026-10-30", .logged), ("2026-10-31", .logged), ("2026-11-01", .logged), ("2026-11-02", .logged)])
    expect(Streak.compute(states: s, now: mk(2026, 11, 2, 18), calendar: cal, weekdays: every).current == 4, "streak over the 25h day")
    let s2 = states([("2026-03-07", .logged), ("2026-03-08", .logged), ("2026-03-09", .logged)])
    expect(Streak.compute(states: s2, now: mk(2026, 3, 9, 18), calendar: cal, weekdays: every).current == 3, "streak over the 23h day")
    // reminder uses wall-clock minutes on the DST day
    var st = Settings(); st.weekdays = every
    let r = ReminderPlanner.decide(now: mk(2026, 11, 1, 17, 0), calendar: cal, settings: st, todayStatus: .missed,
                                   state: PlannerState(day: "2026-11-01"), windowVisible: true)
    expect(r.0 == .bringToFront(notify: true), "17:00 wall clock on fall-back day")
}

// MARK: reminder planner
let monDay = "2026-10-05"
func at(_ h: Int, _ m: Int, day: Int = 5) -> Date { mk(2026, 10, day, h, m) }
func D(_ now: Date, _ s: Settings, _ st: DayStatus, _ state: PlannerState, win: Bool = true) -> (ReminderAction, PlannerState) {
    ReminderPlanner.decide(now: now, calendar: cal, settings: s, todayStatus: st, state: state, windowVisible: win)
}
test("planner: not due / ineligible days") {
    let s = Settings()
    let fresh = PlannerState(day: monDay)
    expect(D(at(16, 54), s, .missed, fresh).0 == .none, "before reminder")
    expect(D(at(17, 0), s, .logged, fresh).0 == .none, "already logged")
    expect(D(at(17, 0), s, .skipped, fresh).0 == .none, "skipped today")
    expect(D(at(17, 0, day: 10), s, .off, PlannerState(day: "2026-10-10")).0 == .none, "Saturday")
    expect(D(at(17, 0, day: 10), s, .missed, PlannerState(day: "2026-10-10")).0 == .none, "Saturday even if status says unlogged")
    expect(D(at(17, 0), s, .future, fresh).0 == .none)
    expect(D(at(17, 0), s, .partial, fresh).0 == .bringToFront(notify: true), "partial still nags")
    var tue = s; tue.weekdays = [3]
    expect(D(at(17, 0), tue, .missed, fresh).0 == .none, "Monday not scheduled")
}
test("planner: strict first fire, re-open every 5 min, skip hint, 23:00 cutoff") {
    let s = Settings()
    var (a, st) = D(at(16, 55), s, .missed, PlannerState(day: monDay))
    expect(a == .bringToFront(notify: true) && st.firstFired)
    (a, st) = D(at(16, 56), s, .missed, st, win: true); expect(a == .none, "visible: leave alone")
    (a, st) = D(at(17, 0), s, .missed, st, win: false); expect(a == .none, "just closed: start 5 min wait")
    (a, st) = D(at(17, 4), s, .missed, st, win: false); expect(a == .none)
    (a, st) = D(at(17, 5), s, .missed, st, win: false); expect(a == .reopen && st.reopenCount == 1)
    (a, st) = D(at(17, 6), s, .missed, st, win: false); expect(a == .none, "closed again, wait")
    (a, st) = D(at(17, 11), s, .missed, st, win: false); expect(a == .reopen && st.reopenCount == 2 && !st.showSkipHint)
    (a, st) = D(at(17, 12), s, .missed, st, win: false)
    (a, st) = D(at(17, 17), s, .missed, st, win: false); expect(a == .reopen && st.reopenCount == 3 && st.showSkipHint)
    (a, st) = D(at(22, 59), s, .missed, st, win: false); (a, st) = D(at(23, 0), s, .missed, st, win: false)
    expect(a == .none, "23:00 cutoff")
    (a, _) = D(at(23, 30), s, .missed, st, win: false); expect(a == .none)
    expect(D(at(23, 5), s, .missed, PlannerState(day: monDay)).0 == .none, "woke after cutoff: no first fire")
    expect(D(at(22, 59), s, .missed, PlannerState(day: monDay)).0 == .bringToFront(notify: true), "woke at 22:59 fires")
}
test("planner: strict snooze (max 2) then re-fire without second notification") {
    let s = Settings()
    var (_, st) = D(at(16, 55), s, .missed, PlannerState(day: monDay))
    expect(ReminderPlanner.snoozesLeft(state: st, settings: s) == 2)
    st = ReminderPlanner.snooze(state: st, now: at(17, 0), settings: s, calendar: cal)!
    expect(st.snoozedUntil == at(17, 15) && ReminderPlanner.snoozesLeft(state: st, settings: s) == 1)
    var a = D(at(17, 10), s, .missed, st, win: false).0; expect(a == .none, "silent during snooze, even with window closed")
    (a, st) = D(at(17, 15), s, .missed, st, win: false); expect(a == .bringToFront(notify: false), "re-fire quietly")
    st = ReminderPlanner.snooze(state: st, now: at(17, 16), settings: s, calendar: cal)!
    expect(ReminderPlanner.snoozesLeft(state: st, settings: s) == 0)
    expect(ReminderPlanner.snooze(state: st, now: at(17, 40), settings: s, calendar: cal) == nil, "third snooze refused")
    (a, st) = D(at(17, 31), s, .missed, st, win: true); expect(a == .bringToFront(notify: false))
    // snooze runs past 23:00 -> nothing
    var late = PlannerState(day: monDay); late.firstFired = true; late.snoozedUntil = at(23, 10)
    expect(D(at(23, 11), s, .missed, late).0 == .none)
    // snooze length follows settings
    var s30 = s; s30.snoozeMinutes = 30
    expect(ReminderPlanner.snooze(state: PlannerState(day: monDay), now: at(17, 0), settings: s30, calendar: cal)!.snoozedUntil == at(17, 30))
}
test("planner: gentle never forces, notifies once, unlimited snooze") {
    var g = Settings(); g.mode = .gentle
    var (a, st) = D(at(16, 55), g, .missed, PlannerState(day: monDay), win: false)
    expect(a == .notify)
    for m in [0, 5, 30, 59] { expect(D(at(17, m), g, .missed, st, win: false).0 == .none, "no repeats / re-opens") }
    expect(ReminderPlanner.snoozesLeft(state: st, settings: g) == nil)
    for i in 0..<5 { st = ReminderPlanner.snooze(state: st, now: at(17, i), settings: g, calendar: cal)! }
    expect(st.snoozesUsed == 0, "no counter in gentle")
    (a, st) = D(at(17, 20), g, .missed, st); expect(a == .notify, "notification again after snooze")
    expect(D(at(17, 21), g, .missed, st).0 == .none)
}
test("planner: launched after reminder time, laptop asleep, settings/undo grace, day rollover") {
    let s = Settings()
    var st = PlannerState.atLaunch(previous: nil, now: at(18, 0), calendar: cal)
    var a = D(at(18, 0), s, .missed, st).0; expect(a == .none, "2 min grace after launch")
    a = D(at(18, 1), s, .missed, st).0; expect(a == .none)
    (a, st) = D(at(18, 2), s, .missed, st); expect(a == .bringToFront(notify: true), "fires once grace is over")
    // asleep through 16:55, wakes 20:10
    expect(D(at(20, 10), s, .missed, PlannerState(day: monDay)).0 == .bringToFront(notify: true))
    // wakes after a snooze elapsed during sleep
    var sn = PlannerState(day: monDay); sn.firstFired = true; sn.snoozedUntil = at(17, 15)
    expect(D(at(19, 0), s, .missed, sn).0 == .bringToFront(notify: false))
    // settings change / undo skip suppress
    let sup = PlannerState(day: monDay).suppressed(until: at(17, 1).addingTimeInterval(ReminderPlanner.graceAfterSettingsChange - 60 + 60))
    expect(D(at(17, 0), s, .missed, sup).0 == .none && D(at(17, 2), s, .missed, sup).0 != .none)
    // snooze count survives relaunch the same day, not the next
    var used = PlannerState(day: monDay); used.snoozesUsed = 2
    expect(PlannerState.atLaunch(previous: used, now: at(18, 0), calendar: cal).snoozesUsed == 2)
    expect(PlannerState.atLaunch(previous: used, now: at(9, 0, day: 6), calendar: cal).snoozesUsed == 0)
    // yesterday's state resets on a new day
    var old = PlannerState(day: monDay); old.firstFired = true; old.snoozesUsed = 2; old.reopenCount = 5
    let (a2, n) = D(at(17, 0, day: 6), s, .missed, old)
    expect(a2 == .bringToFront(notify: true) && n.snoozesUsed == 0 && n.reopenCount == 0 && n.day == "2026-10-06")
}
test("planner: isPending and state is Codable") {
    let s = Settings()
    expect(ReminderPlanner.isPending(now: at(17, 0), calendar: cal, settings: s, todayStatus: .missed))
    expect(!ReminderPlanner.isPending(now: at(10, 0), calendar: cal, settings: s, todayStatus: .missed))
    expect(!ReminderPlanner.isPending(now: at(17, 0), calendar: cal, settings: s, todayStatus: .logged))
    expect(!ReminderPlanner.isPending(now: at(17, 0, day: 10), calendar: cal, settings: s, todayStatus: .off))
    var st = PlannerState(day: monDay); st.snoozedUntil = at(17, 15); st.reopenCount = 2
    try expect(try JSONDecoder().decode(PlannerState.self, from: JSONEncoder().encode(st)) == st)
}

// MARK: weekly review
func weekFixture(minWords: Int = 20, template: String? = nil) throws -> (WeekSummary, LogStore) {
    let store = freshStore("week")
    let mon = "## What I did\nPlanned the week and reviewed pricing page copy with Ana.\nLong standup then deep work on the editor bridge.\n\n## Finished\nShipped A\nand B\n\n## Started\nDocs\n\n## Pending / blocked\n\n## To do next\n- [ ] Plan Q4"
    let tue = "## Standup notes\nQuick sync about launch dates and owners for the week ahead plans.\n\n## Finished\nShipped C\n### Details\nten words here to describe what was shipped for the customer demo\n\n## Pending / blocked\nWaiting on legal"
    let thu = "Spent the whole day on the editor bridge and fixed three bugs in the autosave debounce logic, then wrote notes for tomorrow."
    try store.save(day: "2026-10-05", body: mon); try store.save(day: "2026-10-06", body: tue)
    try store.skip("2026-10-07", reason: "Holiday")
    try store.save(day: "2026-10-08", body: thu); try store.save(day: "2026-10-09", body: "Short note only")
    try store.save(day: "2026-09-30", body: lorem(30, "outside"))
    var s = Settings(); s.minWords = minWords
    if let t = template { s.template = t }
    let w = WeeklyReview.summary(weekContaining: mk(2026, 10, 7), documents: try store.documents(from: "2026-10-05", to: "2026-10-11"),
                                 states: try store.fileStates(minWords: minWords), settings: s, calendar: cal, now: mk(2026, 10, 11, 20))
    return (w, store)
}
test("weekly review aggregate: by template heading plus Other notes") {
    let (w, _) = try weekFixture()
    expect(w.weekStart == "2026-10-05" && w.weekEnd == "2026-10-11" && w.days.count == 7)
    expect(w.loggedCount == 3 && w.workdayCount == 5 && w.skippedCount == 1)
    expect(w.days.map { $0.status } == [.logged, .logged, .skipped, .logged, .partial, .off, .off], "\(w.days.map { $0.status })")
    expect(w.sections.map { $0.title } == ["What I did", "Finished", "Started", "Pending / blocked", "To do next", "Other notes"], "\(w.sections.map { $0.title })")
    expect(w.sections.map { $0.isOther } == [false, false, false, false, false, true])
    let fin = w.section(titled: "finished")!
    expect(fin.items.map { $0.day } == ["2026-10-05", "2026-10-06"] && fin.items[0].text == "Shipped A\nand B")
    expect(fin.items[1].text == "Shipped C\n\n**Details**\nten words here to describe what was shipped for the customer demo", "nested heading stays with its owner")
    expect(w.section(titled: "⏳ Pending / blocked")!.items.map { $0.text } == ["Waiting on legal"], "empty Monday section is left out")
    expect(w.section(titled: "to do next")!.items.map { $0.text } == ["- [ ] Plan Q4"] && w.section(titled: "Started")!.items[0].text == "Docs")
    let other = w.section(titled: "Other notes")!
    expect(other.isOther && other.items.map { $0.day } == ["2026-10-06", "2026-10-08", "2026-10-09"])
    expect(other.items[0].text == "**Standup notes**\nQuick sync about launch dates and owners for the week ahead plans.")
    expect(other.items[1].text.hasPrefix("Spent the whole day") && other.items[2].text == "Short note only")
    expect(w.section(titled: "Nope") == nil)
    // a smaller template: everything else goes to Other notes (with its heading kept as a bold label)
    let (w2, _) = try weekFixture(template: "## Finished")
    expect(w2.sections.map { $0.title } == ["Finished", "Other notes"])
    expect(w2.section(titled: "other notes")!.items[0].text.hasPrefix("**What I did**\nPlanned the week"))
    let (w3, _) = try weekFixture(template: "")
    expect(w3.sections.map { $0.title } == ["Other notes"] && w3.sections[0].items.count == 4, "no template: one group, one item per page with content")
    let (w4, _) = try weekFixture(minWords: 3)
    expect(w4.loggedCount == 4, "minWords drives logged: Friday's 3 words now count")
    expect(WeeklyReview.shift(mk(2026, 10, 7), weeks: -1, calendar: cal) == mk(2026, 9, 30))
}
test("weekly review markdown (day + section grouping)") {
    let (w, _) = try weekFixture()
    let day = WeeklyReview.markdown(for: w, grouping: .day, calendar: cal)
    expect(day.hasPrefix("# Week of 5 Oct – 11 Oct 2026\n\nLogged 3 of 5 workdays · 1 skipped\n"), day)
    expect(day.contains("## Mon 5 Oct\n\n### What I did\nPlanned the week"))
    let monBlock = String(day[day.range(of: "## Mon 5 Oct")!.lowerBound..<day.range(of: "## Tue 6 Oct")!.lowerBound])
    expect(monBlock.contains("### Started\nDocs\n\n### To do next\n- [ ] Plan Q4") && !monBlock.contains("Pending / blocked"), "empty sections are pruned")
    expect(day.contains("## Tue 6 Oct\n\n### Standup notes\nQuick sync") && day.contains("#### Details\nten words here"))
    expect(day.contains("## Wed 7 Oct\n_Skipped: Holiday_"))
    expect(day.contains("## Thu 8 Oct\n\nSpent the whole day") && day.contains("## Fri 9 Oct\n\nShort note only"))
    expect(!day.contains("Sat 10 Oct") && !day.contains("outside") && !day.contains("30 Sep"))
    let sec = WeeklyReview.markdown(for: w, grouping: .section, calendar: cal)
    expect(sec.contains("## Finished\n\n- **Mon 5 Oct**: Shipped A\n  and B\n- **Tue 6 Oct**: Shipped C\n\n  **Details**\n  ten words here"), sec)
    expect(sec.contains("## To do next\n\n- **Mon 5 Oct**\n  - [ ] Plan Q4"), "list items get the label on its own line")
    expect(sec.contains("## Other notes\n\n- **Tue 6 Oct**\n  **Standup notes**\n  Quick sync"))
    expect(sec.contains("- **Thu 8 Oct**: Spent the whole day") && sec.contains("- **Fri 9 Oct**: Short note only"))
    expect(!sec.contains("Skipped") && sec.hasSuffix("\n") && !sec.hasSuffix("\n\n"))
    let empty = WeeklyReview.summary(weekContaining: mk(2026, 10, 7), documents: [:], states: [:], settings: Settings(), calendar: cal, now: mk(2026, 10, 7))
    for g in [WeekGrouping.day, .section] {
        let em = WeeklyReview.markdown(for: empty, grouping: g, calendar: cal)
        expect(em == "# Week of 5 Oct – 11 Oct 2026\n\nLogged 0 of 3 workdays\n", em)
    }
}

// MARK: carry-over
test("carry-over: card, items, apply, idempotent") {
    let store = freshStore("carry")
    let fri = "## What I did\n" + lorem(25) + "\n\n## To do next\n- [ ] Call Ana\n- [x] Sent invoice\nFix build\n1. Plan Q4\n\n## Pending / blocked\n- Legal review\n- [ ] Call Ana\n![](assets/x.png)"
    try store.save(day: "2026-10-02", body: fri)
    try store.skip("2026-10-05", reason: "Holiday")
    let load: (String) -> DayDocument? = { (try? store.load($0)) ?? nil }
    let s = Settings()
    let card = CarryOver.card(before: "2026-10-06", calendar: cal, settings: s, load: load)!
    expect(card.sourceDay == "2026-10-02" && card.sourceLabel == "Fri 2 Oct", "skipped Monday is ignored")
    expect(card.items == ["Call Ana", "Fix build", "Plan Q4", "Legal review"], "\(card.items)")
    expect(card.headingText == "Carried over from Fri 2 Oct")
    expect(CarryOver.heading(for: card, today: "2026-10-06", calendar: cal) == "Friday")
    expect(CarryOver.heading(for: card, today: "2026-10-03", calendar: cal) == "Yesterday")
    expect(CarryOver.heading(for: card, today: "2026-10-12", calendar: cal) == "2 Oct 2026")
    let block = "## Carried over from Fri 2 Oct\n\n- [ ] Call Ana\n- [ ] Fix build\n- [ ] Plan Q4\n- [ ] Legal review"
    expect(CarryOver.apply(card, to: "") == block)
    let once = CarryOver.apply(card, to: "## What I did\nstarted work")
    expect(once == block + "\n\n## What I did\nstarted work", "inserted at the TOP")
    expect(CarryOver.apply(card, to: once) == once && CarryOver.isApplied(card, in: once) && !CarryOver.isApplied(card, in: "x"), "idempotent")
    expect(CarryOver.apply(card, to: once.replacingOccurrences(of: "## Carried", with: "### Carried")) == once.replacingOccurrences(of: "## Carried", with: "### Carried"), "heading level does not matter")
    expect(CarryOver.apply(card, to: "- [x] Call Ana\n- [ ] Fix build\n  - [ ] Plan Q4\nlegal  review") == "- [x] Call Ana\n- [ ] Fix build\n  - [ ] Plan Q4\nlegal  review", "items already present (any marker, case, spacing) are not added again")
    expect(CarryOver.apply(card, to: "Call Ana\nfoo") == "## Carried over from Fri 2 Oct\n\n- [ ] Fix build\n- [ ] Plan Q4\n- [ ] Legal review\n\nCall Ana\nfoo", "only the missing items")
    let withTemplate = CarryOver.apply(card, to: Settings.defaultTemplate)
    expect(withTemplate.hasPrefix(block + "\n\n## What I did") && withTemplate.hasSuffix("## To do next"))
    expect(MarkdownBody.hasContent(block) && MarkdownBody.words(in: block) == 0, "carried tasks are content but not words (was 8 before the carried-over exclusion)")
}
test("carry-over: previous LOGGED day only, lookback, minWords, headings") {
    let store = freshStore("carry2")
    let load: (String) -> DayDocument? = { (try? store.load($0)) ?? nil }
    let s = Settings()
    try store.save(day: "2026-10-02", body: "## What I did\n" + lorem(25) + "\n\n## To do next\n- [ ] From Friday")
    try store.save(day: "2026-10-05", body: "## To do next\n- [ ] Should not carry")   // 3 words: partial
    try store.skip("2026-10-06", reason: "Leave")
    expect(CarryOver.card(before: "2026-10-07", calendar: cal, settings: s, load: load)?.items == ["From Friday"], "partial and skipped days are walked past")
    expect(CarryOver.card(before: "2026-10-02", calendar: cal, settings: s, load: load) == nil, "nothing before the first log")
    expect(CarryOver.card(before: "2026-10-16", calendar: cal, settings: s, load: load)?.sourceDay == "2026-10-02", "14 days back is still found")
    expect(CarryOver.card(before: "2026-10-17", calendar: cal, settings: s, load: load) == nil, "15 days back is not")
    var big = s; big.minWords = 40
    expect(CarryOver.card(before: "2026-10-07", calendar: cal, settings: big, load: load) == nil, "minWords decides what 'logged' means")
    try store.save(day: "2026-10-06", body: "## What I did\n" + lorem(25) + "\n\n## Notes\n- [ ] not a carry heading")
    expect(CarryOver.card(before: "2026-10-07", calendar: cal, settings: s, load: load) == nil, "the latest logged day has nothing to carry: stop, do not dig further back")
    try store.save(day: "2026-10-06", body: "## What I did\n" + lorem(25) + "\n\n## To do next\n- [x] done already\n\n## Pending / blocked\n")
    expect(CarryOver.card(before: "2026-10-07", calendar: cal, settings: s, load: load) == nil, "ticked tasks are not carried")
    var none = s; none.carryOverHeadings = []
    try store.save(day: "2026-10-06", body: "## To do next\n- [ ] x\n" + lorem(25))
    expect(CarryOver.card(before: "2026-10-07", calendar: cal, settings: none, load: load) == nil)
    expect(CarryOver.items(in: "## 🔜 Next\n- [ ] a\n### Sub\n- b\n## Other\n- c", headings: ["next"]) == ["a", "b"], "emoji-insensitive, sub-headings included")
    expect(CarryOver.items(in: "## 📌 TO DO NEXT\n- x\n## to-do next\n- y\n## To do next\n- x", headings: ["To do next"]) == ["x", "y"], "all matching headings, de-duplicated")
    expect(CarryOver.items(in: "## To do next\n![](assets/x.png)\n```\ncode\n```\n<!-- hidden -->\n- real\n\n   \n- [ ]   spaced   out  ", headings: ["To do next"]) == ["real", "spaced out"])
    expect(CarryOver.items(in: "- [ ] outside any heading\n## Other\n- c", headings: ["To do next"]).isEmpty && CarryOver.items(in: "x", headings: [" ", "🙂"]).isEmpty)
    expect(CarryOver.items(in: "## To do next\nFix build and test\n\nCall [Ana](http://x.com) today", headings: ["to do next"]) == ["Fix build and test", "Call [Ana](http://x.com) today"])
    expect(MarkdownBody.insertCarryOver(into: "body", heading: "Carried over from X", items: ["", "  ", "\n"]) == "body", "nothing to add")
    expect(MarkdownBody.insertCarryOver(into: "", heading: "H", items: ["a\nb"]) == "## H\n\n- [ ] a b", "items are single lines")
}

test("carried-over tasks never count toward the logged rule (one Carry over click cannot log a day)") {
    func carried(_ label: String = "Fri 2 Oct", items: Int = 4, per: Int = 8) -> String {
        "## Carried over from \(label)\n\n" + (1...items).map { "- [ ] " + lorem(per, "i\($0)w") }.joined(separator: "\n")
    }
    let block = carried()
    expect(MarkdownBody.hasContent(block) && W(block) == 0, "32 task words, none counted")
    expect(DayDocument(day: "2026-10-06", body: block).status(minWords: 20) == .missed && DayDocument(day: "2026-10-06", body: block).status(minWords: 1) == .missed)
    // the real flow: card from a logged day -> apply -> save
    let store = freshStore("carry-words"); let day = "2026-10-06"
    try store.save(day: "2026-10-02", body: "## What I did\n" + lorem(25) + "\n\n## To do next\n- [ ] " + lorem(10, "t") + "\n- [ ] " + lorem(10, "u"))
    let load: (String) -> DayDocument? = { (try? store.load($0)) ?? nil }
    let card = CarryOver.card(before: day, calendar: cal, settings: Settings(), load: load)!
    expect(card.items.count == 2 && card.items.joined(separator: " ").split(separator: " ").count == 20, "20 carried words, as many as minWords")
    let onTemplate = CarryOver.apply(card, to: Settings.defaultTemplate)
    expect(W(onTemplate) == 0 && MarkdownBody.hasContent(onTemplate) && W(CarryOver.apply(card, to: "")) == 0)
    try store.save(day: day, body: onTemplate)
    try expect(try store.load(day) != nil, "a page holding only carried tasks is kept, never treated as empty")
    try expect(try store.fileStates(minWords: 20)[day] == nil && (try store.fileStates(minWords: 1))[day] == nil, "neither logged nor partial")
    try expect(try store.fileStates(minWords: 20)["2026-10-02"] == .logged)
    try expect(try store.folderSummary().logs == 2, "it still has content")
    do { try store.skip(day); expect(false, "skip rules are unchanged: the page has content") } catch { expect(error as? LogError == .hasContent(day)) }
    // still unlogged for status, planner, streak and carry sources
    let st = try store.fileStates(minWords: 20)
    let now = mk(2026, 10, 6, 18)
    let status = Status.resolve(day: day, fileState: st[day], now: now, calendar: cal, weekdays: workdays, since: st.keys.min())
    expect(status == .missed)
    expect(ReminderPlanner.decide(now: now, calendar: cal, settings: Settings(), todayStatus: status, state: PlannerState(day: day), windowVisible: true).0 == .bringToFront(notify: true), "the reminder still fires")
    expect(CarryOver.card(before: "2026-10-07", calendar: cal, settings: Settings(), load: load)?.sourceDay == "2026-10-02", "a day with only carried tasks is not a carry source")
    expect(Streak.compute(states: st, now: now, calendar: cal, weekdays: workdays) == StreakResult(current: 0, best: 1), "Friday logged, Monday missed, Tuesday not yet")
    // partial / logged only through real words, wherever they are written
    for (n, want) in [(0, DayStatus.missed), (1, .partial), (19, .partial), (20, .logged), (50, .logged)] {
        let real = lorem(n, "real")
        let bodies = [onTemplate.replacingOccurrences(of: "## What I did", with: "## What I did\n" + real),   // under another heading
                      block + "\n\n" + real,                                                                  // headingless paragraph right below the block
                      block + "\n\n" + real + "\n\n## Finished\n",
                      block + "\n\n" + real.split(separator: " ").map { "- " + $0 }.joined(separator: "\n")]  // plain bullets
        for b in bodies {
            let d = DayDocument(day: day, body: b)
            expect(d.words == n && d.status(minWords: 20) == want, "n=\(n) got \(d.words) \(d.status(minWords: 20)) in: \(b.prefix(60))")
        }
    }
    // the reverse trap: a carry-over click above existing headingless writing must not swallow it
    let free = lorem(25)
    let bullets = (1...6).map { "- " + lorem(5, "b\($0)x") }.joined(separator: "\n")
    for body in [free, bullets, "## What I did\n" + lorem(30), "intro words here\n\n" + Settings.defaultTemplate] {
        let once = CarryOver.apply(card, to: body)
        expect(once != body && W(once) == W(body) && DayDocument(day: day, body: once).status(minWords: 20) == DayDocument(day: day, body: body).status(minWords: 20), "words unchanged by carrying: \(W(body)) vs \(W(once))")
    }
}
test("carried-over exclusion: which headings, which lines, nested and multiple blocks") {
    let t = "- [ ] " + lorem(4, "t")   // 4 words
    for h in ["## Carried over from Mon 5 Oct", "# Carried over from Mon", "### carried OVER FROM friday", "#### **Carried over from Mon**", "## 🔁 Carried over from Mon", "## Carried over from"] {
        expect(W("\(h)\n\(t)") == 0 && MarkdownBody.hasContent("\(h)\n\(t)"), h)
    }
    // look-alike headings are the user's own and count like any other
    for h in ["## Carried over", "## Carried over today", "## Not carried over from Mon", "## Carried overfrom Mon", "## Carry over from Mon", "## Carried from Mon"] {
        expect(W("\(h)\n\(t)") == 4, h)
    }
    expect(W("Carried over from Mon\n\(t)") == 8, "the phrase as plain text is not a heading (4 + 4)")
    expect(W("```\n## Carried over from Mon\n```\n\(t)") == 8, "a heading inside a code fence is not a heading (4 code words + 4 task words)")
    // extent: up to the next heading of the same or a higher level
    expect(W("## Carried over from Mon\n\(t)\n\n## What I did\nreal words here") == 3)
    expect(W("### Carried over from Mon\n\(t)\n## Next\n\(t)") == 4, "a higher-level heading ends the block")
    expect(W("## Carried over from Mon\n\(t)\n### Details\n\(t)\n## Next\nreal") == 1, "deeper sub-headings stay inside")
    expect(W("# Carried over from Mon\n\(t)\n## Notes\n\(t)\n# Real\nyes words") == 2)
    expect(W("## What I did\nmine here\n### Carried over from Mon\n\(t)\n## Next\nafter text") == 4, "a block nested in a normal section: 2 + 2")
    // multiple and nested blocks
    expect(W("## Carried over from Fri 2 Oct\n\(t)\n\n## Carried over from Mon 5 Oct\n\(t)\n\n## What I did\nreal words here") == 3)
    expect(W("## Carried over from Fri\n\(t)\n### Carried over from Mon\n\(t)\n## Next\nreal one") == 2)
    expect(W("## Carried over from Fri\n\(t)\n## Carried over from Mon\n\(t)\n### Carried over from Sun\n\(t)\n# Top\nreal one") == 2)
    // every kind of task line is skipped, ticked or rewritten, nested or not; other text under the block is the user's own
    let b = "## Carried over from Mon\n- [x] done thing here\n- [ ] rewritten into a much longer sentence than before\n* [ ] star task words\n1. [ ] numbered task words\n  - [ ] nested sub task words\n+ [X] plus task words"
    expect(W(b) == 0 && MarkdownBody.hasContent(b))
    expect(W(b + "\n\nmy own note under the block") == 6, "a paragraph typed under the block counts")
    expect(W(b + "\n- plain bullet two\n- plain bullet words") == 6, "so do plain bullets")
    expect(W(b + "\n\n```\ncode words\n```") == 2, "and code")
    expect(W("## Carried over from Mon\n<!-- c -->\n![](a.png)\n- [ ]\n\(t)") == 0 && MarkdownBody.hasContent("## Carried over from Mon\n![](a.png)"))
    // outside a carried block nothing changed
    expect(W("## To do next\n- [ ] one two three\n- [x] four five") == 5 && W("- [ ] loose task words") == 3)
    expect(W("## Carried over from Mon\n\n## Real\n\(t)") == 4, "an empty block ends at the next heading")
}
test("carry-over: apply is idempotent and never changes a page's word count") {
    let store = freshStore("carry-idem")
    try store.save(day: "2026-10-02", body: "## What I did\n" + lorem(25) + "\n\n## To do next\n- [ ] alpha beta\n- [ ] gamma delta\n\n## Pending / blocked\n- epsilon zeta")
    let card = CarryOver.card(before: "2026-10-06", calendar: cal, settings: Settings(), load: { (try? store.load($0)) ?? nil })!
    expect(card.items == ["alpha beta", "gamma delta", "epsilon zeta"])
    for body in ["", Settings.defaultTemplate, lorem(25), "## What I did\n" + lorem(30), "x y z\n\n" + Settings.defaultTemplate] {
        let once = CarryOver.apply(card, to: body)
        expect(CarryOver.apply(card, to: once) == once && CarryOver.isApplied(card, in: once) && !CarryOver.isApplied(card, in: body), "idempotent")
        expect(W(once) == W(body) && MarkdownBody.hasContent(once) && once.hasPrefix("## Carried over from Fri 2 Oct\n\n- [ ] alpha beta"))
        // ticking or editing the carried items afterwards changes nothing about the count or the idempotence
        let edited = once.replacingOccurrences(of: "- [ ] alpha beta", with: "- [x] alpha beta and a long rewrite of it")
        expect(W(edited) == W(body) && CarryOver.apply(card, to: edited) == edited)
    }
}

// MARK: search
test("search: headings, snippets, matching rules") {
    let store = freshStore("search")
    try store.save(day: "2026-10-05", body: "## 📝 What I did\nDrafted the Pricing page copy at the Café with Ana\n- [ ] review PRICING page with Ana\n\n## Finished\nPricing only")
    try store.save(day: "2026-10-06", body: "no headings here: notes on pricing and more")
    try store.skip("2026-10-07", reason: "pricing holiday")
    var hits = try store.search("pricing")
    expect(hits.map { $0.day } == ["2026-10-06", "2026-10-05", "2026-10-05"], "newest day first, one hit per section: \(hits.map { $0.day })")
    expect(hits.map { $0.heading } == [nil, "📝 What I did", "Finished"])
    let h = hits[1]
    expect(h.matchRange.map { String(h.snippet[$0]) } == "Pricing", "matchRange points at the match in its original case")
    expect(!h.snippet.contains("- [ ]") && !h.snippet.contains("##") && !h.snippet.contains("\n") && h.snippet.contains("review PRICING page"), h.snippet)
    try expect(try store.search("CAFE ana").map { $0.heading } == ["📝 What I did"], "case/diacritic-insensitive, words AND-ed")
    try expect(try store.search("pricing zebra").isEmpty && (try store.search("   ").isEmpty) && (try store.search("").isEmpty))
    try expect(try store.search("drafted finished").isEmpty, "words must co-occur inside one section (the heading text counts)")
    try expect(try store.search("pricing", heading: "to-do next").isEmpty && (try store.search("pricing", heading: "what I did").count == 1))
    try expect(try store.search("pricing", heading: "Finished").map { $0.day } == ["2026-10-05"])
    try expect(try store.search("holiday").isEmpty, "skipped days are not searched")
    try store.save(day: "2026-10-08", body: "## Pricing review\n\n## Notes\nnothing\n\n## Finished\n### Details\nneedle in a sub section\n\nintro without heading goes first")
    hits = try store.search("pricing review")
    expect(hits.map { $0.day } == ["2026-10-08", "2026-10-05"] && hits[0].heading == "Pricing review" && hits[0].snippet == "Pricing review", "heading-only match: \(hits)")
    try expect(try store.search("needle").map { $0.heading } == ["Details"], "nearest preceding heading")
    try store.save(day: "2026-10-09", body: "## Pricing\nreview notes")
    try expect(try store.search("pricing review").map { $0.day } == ["2026-10-09", "2026-10-08", "2026-10-05"], "words split between heading and text still match")
    try store.save(day: "2026-10-10", body: "```\nlet needle = 1\n```\n\n![needle alt](assets/x.png)\n<!-- needle -->")
    try expect(try store.search("needle").map { $0.day } == ["2026-10-10", "2026-10-08"].sorted(by: >), "code is searchable; image alt text and comments are not")
    let long = lorem(60) + " needle " + lorem(60, "t")
    try store.save(day: "2026-10-11", body: long)
    let lh = try store.search("needle").first { $0.day == "2026-10-11" }!
    expect(lh.heading == nil && lh.snippet.hasPrefix("…") && lh.snippet.hasSuffix("…") && lh.snippet.count < 140 && lh.matchRange.map { String(lh.snippet[$0]) } == "needle")
    // cutting a snippet next to combining marks / emoji sequences must neither crash nor shift the match
    let tricky = String(repeating: "e\u{0301}\u{0308} 👨‍👩‍👧 ", count: 30) + "needle" + String(repeating: " \u{0301}a👩🏽‍💻", count: 40)
    try store.save(day: "2026-10-13", body: tricky)
    let th = try store.search("needle").first { $0.day == "2026-10-13" }!
    expect(th.matchRange.map { String(th.snippet[$0]) } == "needle" && th.snippet.hasPrefix("…") && th.snippet.hasSuffix("…"))
    // an old v0.2 file is searchable too
    try raw("# 2026-10-12\n\n## 🚀 Started\nKicked off the zebra project\n\n", in: store.dir, day: "2026-10-12")
    try expect(try store.search("zebra").map { $0.heading } == ["🚀 Started"])
    expect(Search.hits(in: DayDocument(day: "2026-10-12", isSkipped: true, skipReason: "zebra"), query: "zebra").isEmpty)
}

// MARK: end to end through real files
test("store -> fileStates -> streak/heatmap/status/planner agree under the words rule") {
    let store = freshStore("e2e")
    for d in ["2026-10-05", "2026-10-06", "2026-10-08", "2026-10-09"] { try store.save(day: d, body: "## What I did\n" + lorem(25)) }
    try store.skip("2026-10-07", reason: "x")
    try store.save(day: "2026-10-12", body: Settings.defaultTemplate + "\n\n" + lorem(5))
    let now = mk(2026, 10, 12, 18)
    let st = try store.fileStates(minWords: 20)
    expect(st["2026-10-12"] == .partial && st["2026-10-05"] == .logged && st["2026-10-07"] == .skipped)
    expect(Streak.compute(states: st, now: now, calendar: cal, weekdays: workdays) == StreakResult(current: 4, best: 4), "partial today is 'not yet'")
    func status(_ day: String, _ s: [String: DayStatus]) -> DayStatus {
        Status.resolve(day: day, fileState: s[day], now: now, calendar: cal, weekdays: workdays, since: s.keys.min())
    }
    expect(status("2026-10-12", st) == .partial && status("2026-10-13", st) == .future && status("2026-10-10", st) == .off && status("2026-10-02", st) == .off)
    let hm = Heatmap.weeks(states: st, now: now, calendar: cal, weekdays: workdays)
    expect(hm.last!.map { $0.status } == [.partial, .future, .future, .future, .future, .future, .future] && hm.last![0].isToday)
    expect(hm[10].map { $0.status } == [.logged, .logged, .skipped, .logged, .logged, .off, .off])
    // planner: a partial page still nags; reaching minWords stops it
    var s = Settings(); s.minWords = 20
    let fresh = PlannerState(day: "2026-10-12")
    expect(ReminderPlanner.decide(now: now, calendar: cal, settings: s, todayStatus: status("2026-10-12", st), state: fresh, windowVisible: true).0 == .bringToFront(notify: true))
    try store.save(day: "2026-10-12", body: Settings.defaultTemplate + "\n\n" + lorem(25))
    let st2 = try store.fileStates(minWords: 20)
    expect(status("2026-10-12", st2) == .logged)
    expect(ReminderPlanner.decide(now: now, calendar: cal, settings: s, todayStatus: status("2026-10-12", st2), state: fresh, windowVisible: true).0 == .none)
    expect(Streak.compute(states: st2, now: now, calendar: cal, weekdays: workdays) == StreakResult(current: 5, best: 5))
    // changing the threshold re-evaluates history
    let st3 = try store.fileStates(minWords: 30)
    expect(st3["2026-10-05"] == .partial && st3["2026-10-07"] == .skipped)
    expect(Streak.compute(states: st3, now: now, calendar: cal, weekdays: workdays) == StreakResult(current: 0, best: 0))
    let st4 = try store.fileStates(minWords: 5)
    expect(st4["2026-10-12"] == .logged && Streak.compute(states: st4, now: now, calendar: cal, weekdays: workdays).current == 5)
    // a day that was emptied again is no log at all
    try store.save(day: "2026-10-09", body: "")
    try expect(try store.fileStates(minWords: 20)["2026-10-09"] == nil)
    try expect(Streak.compute(states: try store.fileStates(minWords: 20), now: now, calendar: cal, weekdays: workdays).current == 1, "Friday missed -> only Monday counts")
}

// MARK: log start date (M1)
test("log start: backfilling an older day creates no wall of missed days and leaves the streak alone") {
    let now = mk(2026, 10, 7)                      // Wednesday
    var st: [String: DayStatus] = ["2026-10-05": .logged, "2026-10-06": .logged]
    let before = Streak.compute(states: st, now: now, calendar: cal, weekdays: workdays, since: "2026-10-01")
    expect(before == StreakResult(current: 2, best: 2), "\(before)")
    st["2026-09-14"] = .logged                     // backfill a Monday weeks earlier
    // without a pinned start the earliest file moves back and 15 Sep - 2 Oct become "missed"
    let unpinned = Status.effectiveSince(setting: nil, states: st)
    expect(unpinned == "2026-09-14")
    expect(Status.resolve(day: "2026-09-15", fileState: nil, now: now, calendar: cal, weekdays: workdays, since: unpinned) == .missed)
    // with the start pinned they are off, and the streak is exactly what it was
    let since = Status.effectiveSince(setting: "2026-10-01", states: st)
    expect(since == "2026-10-01")
    expect(Status.resolve(day: "2026-09-15", fileState: nil, now: now, calendar: cal, weekdays: workdays, since: since) == .off)
    expect(Status.resolve(day: "2026-09-14", fileState: .logged, now: now, calendar: cal, weekdays: workdays, since: since) == .logged,
           "the backfilled page itself still shows as logged")
    expect(Streak.compute(states: st, now: now, calendar: cal, weekdays: workdays, since: since) == before)
    let flat = Heatmap.weeks(states: st, now: now, calendar: cal, weekdays: workdays, since: "2026-10-01").flatMap { $0 }
    expect(flat.first { $0.day == "2026-09-15" }?.status == .off && flat.first { $0.day == "2026-09-14" }?.status == .logged)
}
test("log start: nil keeps the old rule; a start after today counts nothing") {
    let now = mk(2026, 10, 7)
    let st: [String: DayStatus] = ["2026-10-05": .logged, "2026-10-06": .logged]
    expect(Streak.compute(states: st, now: now, calendar: cal, weekdays: workdays)
           == Streak.compute(states: st, now: now, calendar: cal, weekdays: workdays, since: nil))
    expect(Streak.compute(states: st, now: now, calendar: cal, weekdays: workdays, since: "2026-10-20") == StreakResult(current: 0, best: 0))
    expect(Status.effectiveSince(setting: nil, states: st) == "2026-10-05" && Status.effectiveSince(setting: nil, states: [:]) == nil)
}

test("fresh install: with no pages and no log start, past days are not missed") {
    let now = mk(2026, 10, 7)                      // Wednesday
    for day in ["2026-09-30", "2026-10-05", "2026-10-06"] {
        expect(Status.resolve(day: day, fileState: nil, now: now, calendar: cal, weekdays: workdays, since: nil) == .off, day)
    }
    expect(Status.resolve(day: "2026-10-07", fileState: nil, now: now, calendar: cal, weekdays: workdays, since: nil) == .missed, "today is still due")
    expect(Status.resolve(day: "2026-10-08", fileState: nil, now: now, calendar: cal, weekdays: workdays, since: nil) == .future)
    let hm = Heatmap.weeks(states: [:], now: now, calendar: cal, weekdays: workdays).flatMap { $0 }.filter { $0.status == .missed }.map { $0.day }
    expect(hm == ["2026-10-07"], "\(hm)")
    let grid = MonthGrid.rows(year: 2026, month: 10, states: [:], now: now, calendar: cal, weekdays: workdays, logStart: nil)
        .flatMap { $0 }.filter { $0.status == .missed }.map { $0.day }
    expect(grid == ["2026-10-07"], "\(grid)")
}

// MARK: settings added for real-world navigation (M1)
test("settings M1 fields: defaults, round trip, tolerant decode, clamping") {
    let d = Settings()
    expect(d.logStartDate == nil && d.appearance == .system && d.weekStart == nil && d.catchUpWindowDays == 30)
    let mem = MemoryStore()
    var s = Settings(); s.logStartDate = "2026-10-01"; s.appearance = .dark; s.weekStart = 2; s.catchUpWindowDays = 90
    s.save(to: mem)
    let back = Settings.load(from: mem)
    expect(back.logStartDate == "2026-10-01" && back.appearance == .dark && back.weekStart == 2 && back.catchUpWindowDays == 90)
    // a v0.3 save (none of the new keys) decodes to the defaults
    let old = #"{"reminderMinutes":1020,"minWords":25,"onboarded":true}"#.data(using: .utf8)!
    let o = try JSONDecoder().decode(Settings.self, from: old).normalized()
    expect(o.minWords == 25 && o.onboarded && o.logStartDate == nil && o.appearance == .system && o.catchUpWindowDays == 30)
    // clamping and validation
    var bad = Settings(); bad.catchUpWindowDays = 3; bad.logStartDate = "garbage"; bad.weekStart = 9
    let n = bad.normalized()
    expect(n.catchUpWindowDays == 7 && n.logStartDate == nil && n.weekStart == nil)
    bad.catchUpWindowDays = 99999; bad.weekStart = 2; bad.logStartDate = "2026-09-30"
    let m = bad.normalized()
    expect(m.catchUpWindowDays == 365 && m.weekStart == 2 && m.logStartDate == "2026-09-30")
}

// MARK: legacy migration (rename Daily Log -> Gloamlog)
test("legacy migration: settings keys are copied once, never overwritten, system keys ignored") {
    let oldName = "dl-test-old-" + UUID().uuidString, newName = "dl-test-new-" + UUID().uuidString
    let old = UserDefaults(suiteName: oldName)!, new = UserDefaults(suiteName: newName)!
    defer { old.removePersistentDomain(forName: oldName); new.removePersistentDomain(forName: newName) }
    old.set(Data([1, 2, 3]), forKey: "dailylog.settings.v2"); old.set(true, forKey: "showMenuBar"); old.set("x", forKey: "unrelated")
    let n = LegacyMigration.migrateDefaults(from: old, to: new)
    expect(n == 2, "copied \(n)")
    expect(new.data(forKey: "dailylog.settings.v2") == Data([1, 2, 3]) && new.bool(forKey: "showMenuBar"))
    expect(new.object(forKey: "unrelated") == nil, "keys outside our namespace are not copied")
    new.set(Data([9]), forKey: "dailylog.settings.v2")
    expect(LegacyMigration.migrateDefaults(from: old, to: new) == 0 && new.data(forKey: "dailylog.settings.v2") == Data([9]), "second run changes nothing, existing values win")
    expect(LegacyMigration.migrateDefaults(from: nil, to: new) == 0)
}
test("legacy migration: support folder moves once and only when the new one is absent") {
    let fm = FileManager.default
    let support = freshDir("support")
    let old = support.appendingPathComponent("Daily Log/backups/2026-10-06", isDirectory: true)
    try fm.createDirectory(at: old, withIntermediateDirectories: true)
    try "v1".write(to: old.appendingPathComponent("20261006-171500.md"), atomically: true, encoding: .utf8)
    expect(LegacyMigration.migrateSupportFolder(in: support) == true)
    expect(!fm.fileExists(atPath: support.appendingPathComponent("Daily Log").path))
    expect(fm.fileExists(atPath: support.appendingPathComponent("Gloamlog/backups/2026-10-06/20261006-171500.md").path), "backup survives the move")
    expect(LegacyMigration.migrateSupportFolder(in: support) == false, "nothing left to move")
    // both exist: leave both alone, never merge or overwrite
    try fm.createDirectory(at: support.appendingPathComponent("Daily Log"), withIntermediateDirectories: true)
    expect(LegacyMigration.migrateSupportFolder(in: support) == false)
    expect(fm.fileExists(atPath: support.appendingPathComponent("Daily Log").path) && fm.fileExists(atPath: support.appendingPathComponent("Gloamlog").path))
    // nothing at all
    expect(LegacyMigration.migrateSupportFolder(in: freshDir("empty-support")) == false)
}
test("default storage folder is Gloamlog for new installs") {
    expect(Settings.defaultFolder.lastPathComponent == "Gloamlog" && Settings().storageFolder.lastPathComponent == "Gloamlog")
}

// MARK: M1 core helpers (C2 to C6)
func calIn(_ tz: String = "America/New_York", firstWeekday: Int = 2, locale: String? = nil) -> Calendar {
    var c = Calendar(identifier: .gregorian)
    c.timeZone = TimeZone(identifier: tz)!; c.firstWeekday = firstWeekday
    if let l = locale { c.locale = Locale(identifier: l) }
    return c
}
/// Independent oracle for day arithmetic: proleptic Gregorian in UTC (uses neither DayKey nor the zone under test).
let utcCal = calIn("UTC", firstWeekday: 1)
func utcKey(_ d: Date) -> String { let c = utcCal.dateComponents([.year, .month, .day], from: d); return String(format: "%04ld-%02ld-%02ld", c.year!, c.month!, c.day!) }
func utcDate(_ y: Int, _ m: Int, _ d: Int) -> Date { utcCal.date(from: DateComponents(year: y, month: m, day: d, hour: 12))! }

// MARK: C2 MonthGrid
func grid(_ y: Int, _ m: Int, _ st: [String: DayStatus] = [:], now: Date = mk(2026, 10, 7), cal c: Calendar = cal,
          weekdays wd: Set<Int> = workdays, logStart: String? = nil) -> [[MonthCell]] {
    MonthGrid.rows(year: y, month: m, states: st, now: now, calendar: c, weekdays: wd, logStart: logStart)
}
func gridCell(_ g: [[MonthCell]], _ day: String) -> MonthCell? { g.flatMap { $0 }.first { $0.day == day } }

test("month grid: shape and position, Monday-first October 2026") {
    let g = grid(2026, 10)
    expect(g.count == 6 && g.allSatisfy { $0.count == 7 }, "6 rows x 7 columns")
    expect(g.first?.first?.day == "2026-09-28" && g.first?.first?.inMonth == false, "first cell is the Monday on or before the 1st")
    expect(g.count == 6 && g[0].count == 7 && g[0][3].day == "2026-10-01" && g[0][3].inMonth, "1 Oct is a Thursday: row 0, column 3")
    expect(g.count == 6 && g[5].count == 7 && g[5][6].day == "2026-11-08" && !g[5][6].inMonth, "last cell, 41 days after the first")
    let inMonth = g.flatMap { $0 }.filter { $0.inMonth }.map { $0.day }
    expect(inMonth.count == 31 && inMonth.first == "2026-10-01" && inMonth.last == "2026-10-31", "\(inMonth.count) in-month cells")
    let all = g.flatMap { $0 }
    expect(all.filter { !$0.inMonth }.allSatisfy { $0.day < "2026-10-01" || $0.day > "2026-10-31" }, "inMonth is false exactly outside the month")
    for row in g { for (c, x) in row.enumerated() {
        expect(DayKey.weekday(x.day, cal) == ((cal.firstWeekday - 1 + c) % 7) + 1, "column \(c) of \(x.day) has the wrong weekday")
    } }
}
test("month grid: Sunday-first calendar, year boundaries, short months still give 6 rows") {
    let sun = calIn(firstWeekday: 1)
    let g = grid(2026, 10, cal: sun)
    expect(g.count == 6 && g[0].count == 7 && g[0][0].day == "2026-09-27" && !g[0][0].inMonth, "Sunday-first: first cell is Sunday 27 Sep")
    expect(g.count == 6 && g[0].count == 7 && g[0][4].day == "2026-10-01", "1 Oct is column 4 when weeks start on Sunday")
    for row in g { for (c, x) in row.enumerated() { expect(DayKey.weekday(x.day, sun) == c + 1, "Sunday-first column \(c) = \(x.day)") } }
    // a month that begins on the first column: 1 Feb 2027 is a Monday, only 4 rows are needed and 6 are still returned
    let feb = grid(2027, 2)
    expect(feb.count == 6 && feb[0][0].day == "2027-02-01" && feb[0][0].inMonth, "Feb 2027 starts in column 0")
    expect(feb.count == 6 && feb[4][0].day == "2027-03-01" && !feb[4][0].inMonth && feb[5][6].day == "2027-03-14", "rows 5 and 6 are March")
    // year boundaries
    let dec = grid(2026, 12)
    expect(dec.count == 6 && dec[0][0].day == "2026-11-30" && dec[5][6].day == "2027-01-10", "December 2026 runs into January 2027")
    let jan = grid(2027, 1)
    expect(jan.count == 6 && jan[0][0].day == "2026-12-28" && jan[0][4].day == "2027-01-01" && jan[0][4].inMonth, "January 2027 starts in December 2026")
}
test("month grid: leap February has 29 in-month cells, century and ordinary years 28") {
    func count(_ y: Int, _ c: Calendar = cal) -> Int { grid(y, 2, cal: c).flatMap { $0 }.filter { $0.inMonth }.count }
    expect(count(2028) == 29 && count(2024) == 29 && count(2000) == 29, "leap years")
    expect(count(2026) == 28 && count(2027) == 28 && count(2100) == 28, "ordinary years, and 2100 is not a leap year")
    expect(count(2028, calIn(firstWeekday: 1)) == 29)
    let g = grid(2028, 2)
    expect(gridCell(g, "2028-02-29")?.inMonth == true && gridCell(g, "2028-03-01")?.inMonth == false, "29 Feb is in, 1 Mar is out")
    expect(g.flatMap { $0 }.map { $0.day }.contains("2028-02-29"))
    let g26 = grid(2026, 2)
    expect(gridCell(g26, "2026-02-28")?.inMonth == true && gridCell(g26, "2026-03-01")?.inMonth == false)
}
test("month grid: 42 unique consecutive days in DST months, Sunday- and Monday-first, in zones with midnight DST") {
    // New York DST months explicitly: no duplicate and no missing key (the 23 h and 25 h days).
    for (y, m) in [(2026, 3), (2026, 11), (2027, 3), (2027, 11), (2028, 3)] {
        let keys = grid(y, m).flatMap { $0 }.map { $0.day }
        expect(keys.count == 42 && Set(keys).count == 42, "\(y)-\(m): 42 unique keys, got \(Set(keys).count)")
        var consecutive = true
        for i in stride(from: 1, to: keys.count, by: 1) where DayKey.adding(keys[i - 1], 1, cal) != keys[i] { consecutive = false }
        expect(consecutive && !keys.isEmpty, "\(y)-\(m): consecutive")
    }
    // Every month, both week starts, zones whose DST changes at midnight (Beirut, Havana, Santiago, Sao Paulo 2018) or by half an hour.
    let cases: [(String, Int)] = [("America/New_York", 2026), ("America/Sao_Paulo", 2018), ("Europe/London", 2026), ("Asia/Kolkata", 2026),
                                  ("Pacific/Auckland", 2026), ("Australia/Lord_Howe", 2026), ("America/Santiago", 2026), ("Asia/Beirut", 2026),
                                  ("America/Havana", 2026)]
    var bad = [String](), checked = 0
    for (tz, year) in cases { for fw in [1, 2] {
        let c = calIn(tz, firstWeekday: fw)
        for m in 1...12 {
            checked += 1
            let g = grid(year, m, now: mk(year, 6, 15), cal: c)
            let first = utcDate(year, m, 1)
            let lead = (utcCal.component(.weekday, from: first) - fw + 7) % 7
            let want = (0..<42).map { utcKey(utcCal.date(byAdding: .day, value: $0 - lead, to: first)!) }
            let got = g.flatMap { $0 }.map { $0.day }
            if g.count != 6 || !g.allSatisfy({ $0.count == 7 }) || got != want { bad.append("\(tz) fw\(fw) \(year)-\(m)") }
            let inMonth = g.flatMap { $0 }.filter { $0.inMonth }.count
            if inMonth != utcCal.range(of: .day, in: .month, for: first)!.count { bad.append("\(tz) fw\(fw) \(year)-\(m) in-month=\(inMonth)") }
        }
    } }
    expect(bad.isEmpty && checked == cases.count * 2 * 12, "wrong grids: \(bad.prefix(6)), checked \(checked)")
}
test("month grid: statuses come from Status.resolve (logged, partial, skipped, missed, future), today is flagged once") {
    let st: [String: DayStatus] = ["2026-09-30": .logged, "2026-10-02": .skipped, "2026-10-05": .logged, "2026-10-06": .partial]
    let g = grid(2026, 10, st)          // now = Wed 7 Oct 2026
    func s(_ d: String) -> DayStatus? { gridCell(g, d)?.status }
    expect(s("2026-10-05") == .logged && s("2026-10-06") == .partial && s("2026-10-02") == .skipped, "file states map through")
    expect(s("2026-09-30") == .logged, "a leading out-of-month cell keeps its state")
    expect(s("2026-10-01") == .missed, "scheduled, unwritten, after the first page = missed")
    expect(s("2026-10-07") == .missed && gridCell(g, "2026-10-07")?.isToday == true, "today unlogged is .missed with the today flag")
    expect(s("2026-10-08") == .future && s("2026-10-31") == .future && s("2026-11-08") == .future, "after today is .future")
    expect(s("2026-10-10") == .future, "a future Saturday is .future, not .off")
    expect(s("2026-10-03") == .off && s("2026-10-04") == .off, "past weekend, nothing written = .off")
    expect(g.flatMap { $0 }.filter { $0.isToday }.map { $0.day } == ["2026-10-07"], "exactly one today cell")
    expect(g.flatMap { $0 }.filter { $0.status == .skipped }.count == 1 && g.flatMap { $0 }.filter { $0.status == .partial }.count == 1)
    // today can be an out-of-month cell
    let sep = grid(2026, 9, now: mk(2026, 10, 1, 9))
    expect(sep.flatMap { $0 }.filter { $0.isToday }.map { $0.day } == ["2026-10-01"] && gridCell(sep, "2026-10-01")?.inMonth == false, "today as a trailing cell")
    // viewing other months than today's
    let later = grid(2026, 11, now: mk(2026, 10, 7))
    expect(later.flatMap { $0 }.allSatisfy { $0.status == .future && !$0.isToday }, "a month entirely after today: all .future, no today")
    let earlier = grid(2026, 10, st, now: mk(2026, 11, 20))
    expect(earlier.flatMap { $0 }.allSatisfy { $0.status != .future && !$0.isToday }, "a month entirely before today: nothing future, no today")
    expect(gridCell(earlier, "2026-10-30")?.status == .missed && gridCell(earlier, "2026-10-31")?.status == .off)
}
test("month grid: isScheduled follows the working weekdays; off days stay .off unless written") {
    let g = grid(2026, 10, ["2026-09-30": .logged, "2026-10-03": .logged])
    expect(gridCell(g, "2026-10-05")?.isScheduled == true && gridCell(g, "2026-10-09")?.isScheduled == true, "Mon and Fri are scheduled")
    expect(gridCell(g, "2026-10-03")?.isScheduled == false && gridCell(g, "2026-10-04")?.isScheduled == false, "Sat and Sun are not")
    expect(gridCell(g, "2026-10-03")?.status == .logged, "a log on a day off still shows as logged")
    expect(gridCell(g, "2026-10-04")?.status == .off, "an unwritten day off is .off, never missed")
    expect(gridCell(g, "2026-10-31")?.isScheduled == false, "isScheduled does not depend on the future")
    let weekend = grid(2026, 10, ["2026-09-30": .logged], weekdays: [1, 7])
    expect(gridCell(weekend, "2026-10-03")?.isScheduled == true && gridCell(weekend, "2026-10-05")?.isScheduled == false)
    expect(gridCell(weekend, "2026-10-03")?.status == .missed && gridCell(weekend, "2026-10-05")?.status == .off, "a weekend schedule flips missed and off")
}
test("month grid: the log start cuts missed days to .off; the backfilled page itself still shows") {
    let st: [String: DayStatus] = ["2026-09-14": .logged, "2026-10-05": .logged]
    let g = grid(2026, 9, st, logStart: "2026-10-01")
    expect(gridCell(g, "2026-09-14")?.status == .logged, "the backfilled page is logged")
    expect(gridCell(g, "2026-09-15")?.status == .off && gridCell(g, "2026-09-30")?.status == .off, "13 to 30 Sep are .off, not missed")
    expect(gridCell(g, "2026-09-15")?.isScheduled == true, "off because of the start date, not because it is a day off")
    expect(gridCell(g, "2026-10-01")?.status == .missed, "the log start day itself counts again (trailing cell of the Sep grid)")
    // without a pinned start the earliest file is the start, which creates the wall of missed days
    let wall = grid(2026, 9, st, logStart: nil)
    expect(gridCell(wall, "2026-09-15")?.status == .missed && gridCell(wall, "2026-09-14")?.status == .logged, "nil start = earliest file")
    // a file before the start keeps its state; an empty day before it is off even if scheduled
    let late = grid(2026, 10, ["2026-10-05": .logged], logStart: "2026-10-07")
    expect(gridCell(late, "2026-10-06")?.status == .off && gridCell(late, "2026-10-05")?.status == .logged && gridCell(late, "2026-10-07")?.status == .missed)
    // a start in the future: nothing before it is missed
    let fut = grid(2026, 10, [:], logStart: "2026-10-20")
    expect(fut.flatMap { $0 }.filter { $0.day <= "2026-10-07" }.allSatisfy { $0.status == .off }, "start after today: everything up to today is off")
    // no files and no start (a brand-new install): nothing before today is due, so no wall of missed days
    expect(gridCell(grid(2026, 10), "2026-10-06")?.status == .off)
}
test("month grid: invalid month gives no rows, never a crash") {
    expect(grid(2026, 0).isEmpty && grid(2026, 13).isEmpty && grid(2026, -1).isEmpty)
    expect(!grid(2026, 1).isEmpty && !grid(2026, 12).isEmpty)
}

// MARK: C3 DateJump
let wed7 = mk(2026, 10, 7, 12)       // Wednesday 7 Oct 2026, noon, New York
func jump(_ s: String, _ now: Date = wed7, _ c: Calendar = cal) -> String? { DateJump.parse(s, now: now, calendar: c) }
/// One assertion per row; the message names the input.
func jumpTable(_ rows: [(String, String?)], now: Date = wed7, cal c: Calendar = cal) {
    for (input, want) in rows {
        let got = jump(input, now, c)
        expect(got == want, "'\(input)' -> \(got ?? "nil"), want \(want ?? "nil")")
    }
}

test("date jump: keywords and relative days") {
    jumpTable([("yesterday", "2026-10-06"), ("today", "2026-10-07"), ("-1", "2026-10-06"), ("-3", "2026-10-04"), ("-0", "2026-10-07"),
               ("3 days ago", "2026-10-04"), ("1 day ago", "2026-10-06"), ("0 days ago", "2026-10-07"),
               ("1 week ago", "2026-09-30"), ("2 weeks ago", "2026-09-23"), ("-30", "2026-09-07"),
               ("-279", "2026-01-01"), ("-280", "2025-12-31")])
}
test("date jump: weekday names (fri = most recent on or before today, last fri = strictly before)") {
    jumpTable([("last fri", "2026-10-02"), ("fri", "2026-10-02"), ("friday", "2026-10-02"), ("last friday", "2026-10-02"),
               ("wed", "2026-10-07"), ("wednesday", "2026-10-07"), ("weds", "2026-10-07"), ("last wed", "2026-09-30"), ("last wednesday", "2026-09-30"),
               ("thu", "2026-10-01"), ("thur", "2026-10-01"), ("thurs", "2026-10-01"), ("last thu", "2026-10-01"),
               ("tue", "2026-10-06"), ("tues", "2026-10-06"), ("tuesday", "2026-10-06"), ("last tue", "2026-10-06"),
               ("mon", "2026-10-05"), ("last mon", "2026-10-05"), ("sat", "2026-10-03"), ("sun", "2026-10-04"), ("last sun", "2026-10-04"),
               ("saturday", "2026-10-03"), ("last saturday", "2026-10-03"), ("sunday", "2026-10-04"), ("monday", "2026-10-05")])
}
test("date jump: month names; without a year the most recent past occurrence, never a future day") {
    jumpTable([("2 oct", "2026-10-02"), ("oct 2", "2026-10-02"), ("2 October 2026", "2026-10-02"), ("2026-10-02", "2026-10-02"),
               ("2nd oct", "2026-10-02"), ("oct 2nd", "2026-10-02"), ("october 2, 2026", "2026-10-02"), ("Oct. 2", "2026-10-02"),
               ("2 oct 2026", "2026-10-02"), ("oct 2 2026", "2026-10-02"), ("3rd oct", "2026-10-03"), ("1st oct", "2026-10-01"),
               ("7 oct", "2026-10-07"), ("1 sep", "2026-09-01"), ("1 sept", "2026-09-01"), ("1 september", "2026-09-01"),
               ("15 jan", "2026-01-15"), ("31 dec", "2025-12-31"), ("2 dec", "2025-12-02"), ("8 oct", "2025-10-08"),
               ("29 feb", "2024-02-29"), ("29 feb 2024", "2024-02-29"), ("1 jan 2000", "2000-01-01"), ("mar 3", "2026-03-03"),
               ("3 may", "2026-05-03"), ("8 oct 2025", "2025-10-08"), ("8 oct 2026", nil)])
}
test("date jump: numeric dates follow the calendar's locale; ISO never does") {
    // No locale on the calendar behaves like en_US (deterministic in tests).
    jumpTable([("10/2", "2026-10-02"), ("10/7", "2026-10-07"), ("10/8", "2025-10-08"), ("10/2/2026", "2026-10-02"), ("10/2/26", "2026-10-02"),
               ("1/5", "2026-01-05"), ("12/31", "2025-12-31"), ("10.2", "2026-10-02"), ("10-2", "2026-10-02"),
               ("2026/10/02", "2026-10-02"), ("2026-10-2", "2026-10-02"), ("2026-1-5", "2026-01-05"), ("2026.10.02", "2026-10-02"),
               ("13/2", nil), ("2/30", nil), ("0/5", nil), ("1/1/00", "2000-01-01"), ("10/2/27", nil)])
    let us = calIn(locale: "en_US"), gb = calIn(locale: "en_GB"), de = calIn(locale: "de_DE"), ja = calIn(locale: "ja_JP")
    jumpTable([("10/2", "2026-10-02"), ("10/2/2026", "2026-10-02"), ("2/10", "2026-02-10"), ("13/2", nil)], cal: us)
    jumpTable([("10/2", "2026-02-10"), ("2/10", "2026-10-02"), ("10/2/2026", "2026-02-10"), ("2/10/26", "2026-10-02"),
               ("31/12", "2025-12-31"), ("12/31", nil), ("2026-10-02", "2026-10-02"), ("8/10", "2025-10-08")], cal: gb)
    let inn = calIn(locale: "en_IN")      // the owner's Mac: day first, like en_GB
    jumpTable([("10/2", "2026-02-10"), ("2/10", "2026-10-02"), ("10/2/2026", "2026-02-10"), ("2026-10-02", "2026-10-02")], cal: inn)
    expect(cal.locale?.identifier.isEmpty ?? true, "fixture: the harness calendar has no real locale (root), so it must read like en_US")
    jumpTable([("10.2", "2026-02-10"), ("10.2.", "2026-02-10"), ("2.10.2026", "2026-10-02"), ("2026-10-02", "2026-10-02")], cal: de)
    jumpTable([("10/2", "2026-10-02"), ("26/10/2", "2026-10-02"), ("2026/10/2", "2026-10-02"), ("2026-10-02", "2026-10-02")], cal: ja)
    // a locale changes only numeric order, never the words
    jumpTable([("2 oct", "2026-10-02"), ("oct 2", "2026-10-02"), ("yesterday", "2026-10-06"), ("last fri", "2026-10-02")], cal: gb)
}
test("date jump: unreadable, impossible, future and pre-2000 input gives nil") {
    jumpTable([("tomorrow", nil), ("2027-01-01", nil), ("2026-10-08", nil), ("garbage", nil), ("", nil), ("   ", nil), ("\n\t", nil),
               ("31 feb", nil), ("30 feb", nil), ("29 feb 2026", nil), ("30 feb 2024", nil), ("2026-02-30", nil), ("2026-02-29", nil),
               ("2026-13-01", nil), ("2026-00-10", nil), ("2026-10-00", nil), ("2026-10-32", nil), ("32 oct", nil), ("0 oct", nil),
               ("31 sep", nil), ("31 apr", nil), ("oct", nil), ("2", nil), ("3", nil), ("last", nil), ("last week", nil), ("last garbage", nil),
               ("fri fri", nil), ("last last fri", nil), ("2 oct oct", nil), ("oct oct 2", nil), ("2 3 oct", nil), ("2 oct 3", nil), ("2 oct 26", nil),
               ("oct 2026", nil), ("2026 oct 2", nil), ("+3", nil), ("-", nil), ("--3", nil), ("3 days", nil), ("days ago", nil),
               ("-3 days ago", nil), ("3 days from now", nil), ("in 3 days", nil), ("next fri", nil), ("this fri", nil), ("now", nil),
               ("1999-12-31", nil), ("31 dec 1999", nil), ("1 jan 1999", nil), ("1900-01-01", nil), ("0000-01-01", nil), ("-99999", nil),
               ("12/31/99", nil), ("99999999999999999999 days ago", nil), ("-99999999999999999999", nil), ("9999999 weeks ago", nil),
               ("10000-01-01", nil), ("2026-10-02-1", nil), ("1/2/3/4", nil), ("//", nil), ("1//2", nil), ("10/2/", nil), ("/10/2", nil),
               ("10/2-2026", nil), ("2026-10", nil), ("٣", nil), ("١٢/٣", nil), ("🎉", nil), ("2 🎉", nil)])
}
test("date jump: case, whitespace and punctuation are ignored") {
    jumpTable([("  YESTERDAY  ", "2026-10-06"), ("LAST FRI", "2026-10-02"), ("Last   Fri", "2026-10-02"), ("\t2 Oct\n", "2026-10-02"),
               ("OCT 2", "2026-10-02"), ("2 OCTOBER 2026", "2026-10-02"), ("  2026-10-02  ", "2026-10-02"), ("2\u{00A0}oct", "2026-10-02"),
               ("Oct 2,", "2026-10-02"), ("2 oct,", "2026-10-02"), ("OCTOBER 2ND, 2026", "2026-10-02"), (" -3 ", "2026-10-04"),
               ("3  DAYS   AGO", "2026-10-04"), ("Fri.", "2026-10-02"), ("last fri.", "2026-10-02")])
}
test("date jump: a whole year round-trips through ISO, 'd MMM yyyy', 'd MMM' and -n; weekday names match an independent oracle") {
    for (now, label) in [(wed7, "2026-10-07"), (mk(2028, 3, 10, 9), "2028-03-10"), (mk(2026, 1, 2, 22), "2026-01-02")] {
        let today = DayKey.string(now, cal)
        expect(today == label, "fixture \(label)")
        var failures = [String](), checked = 0
        for n in 0..<360 {
            guard let day = DayKey.adding(today, -n, cal) else { failures.append("adding -\(n)"); continue }
            let oracle = utcKey(utcCal.date(byAdding: .day, value: -n, to: utcDate(Int(today.prefix(4))!, Int(today.dropFirst(5).prefix(2))!, Int(today.suffix(2))!))!)
            if day != oracle { failures.append("oracle \(n): \(day) vs \(oracle)") }
            for input in [day, DayKey.format(day, "d MMM yyyy", cal), DayKey.format(day, "MMMM d, yyyy", cal), DayKey.format(day, "d MMM", cal),
                          DayKey.format(day, "MMM d", cal), "-\(n)", "\(n) days ago"] {
                checked += 1
                if jump(input, now) != day { failures.append("'\(input)' -> \(jump(input, now) ?? "nil"), want \(day)") }
            }
        }
        expect(failures.isEmpty && checked == 360 * 7, "\(label): \(failures.prefix(4)) checked \(checked)")
    }
    // weekday names vs a scan with the UTC calendar
    let names = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"]
    var bad = [String](), checked = 0
    for dayOfMonth in 1...31 {
        let now = mk(2026, 10, dayOfMonth, 12)
        let todayUTC = utcDate(2026, 10, dayOfMonth)
        for (i, name) in names.enumerated() {
            for (input, strict) in [(name, false), ("last " + name, true)] {
                var k = strict ? 1 : 0
                while utcCal.component(.weekday, from: utcCal.date(byAdding: .day, value: -k, to: todayUTC)!) != i + 1 { k += 1 }
                let want = utcKey(utcCal.date(byAdding: .day, value: -k, to: todayUTC)!)
                checked += 1
                if jump(input, now) != want { bad.append("\(input) on 2026-10-\(dayOfMonth)") }
            }
        }
    }
    expect(bad.isEmpty && checked == 31 * 14, "\(bad.prefix(4)) checked \(checked)")
}
test("date jump: boundaries (year 2000, new year, DST, leap day, the time zone of `now`)") {
    jumpTable([("yesterday", "2000-01-02"), ("-2", "2000-01-01"), ("-3", nil), ("last mon", nil), ("mon", "2000-01-03"), ("1 jan", "2000-01-01"),
               ("31 dec", nil), ("sun", "2000-01-02"), ("2000-01-01", "2000-01-01"), ("1999-12-31", nil)], now: mk(2000, 1, 3, 12))
    jumpTable([("2 dec", "2025-12-02"), ("last fri", "2025-12-26"), ("-3", "2025-12-30"), ("1 jan", "2026-01-01"), ("3 jan", "2025-01-03"),
               ("12/31", "2025-12-31")], now: mk(2026, 1, 2, 12))
    jumpTable([("-1", "2026-11-01"), ("-2", "2026-10-31"), ("last sun", "2026-11-01"), ("3 days ago", "2026-10-30"), ("1 week ago", "2026-10-26"),
               ("sun", "2026-11-01")], now: mk(2026, 11, 2, 12))                         // day after the 25 h fall-back day
    jumpTable([("yesterday", "2026-03-08"), ("last sun", "2026-03-08"), ("-2", "2026-03-07"), ("-7", "2026-03-02")], now: mk(2026, 3, 9, 12))   // after the 23 h day
    jumpTable([("yesterday", "2028-02-29"), ("29 feb", "2028-02-29"), ("-366", "2027-03-01"), ("2028-02-29", "2028-02-29"), ("29 feb 2027", nil),
               ("1 mar", "2028-03-01"), ("2 mar", "2027-03-02")], now: mk(2028, 3, 1, 12))
    jumpTable([("29 feb", "2000-02-29"), ("29 feb 2000", "2000-02-29"), ("29 feb 2001", nil)], now: mk(2002, 1, 5, 12))      // most recent leap day, two years back
    jumpTable([("today", "2026-10-07"), ("yesterday", "2026-10-06"), ("-1", "2026-10-06"), ("tomorrow", nil)], now: mk(2026, 10, 7, 23, 59))
    jumpTable([("today", "2026-10-07"), ("yesterday", "2026-10-06"), ("wed", "2026-10-07")], now: mk(2026, 10, 7, 0, 1))
    // the same instant is a different day in another zone: `today` follows the calendar's time zone
    let instant = utcCal.date(from: DateComponents(year: 2026, month: 10, day: 7, hour: 20))!
    expect(jump("today", instant, calIn("America/New_York")) == "2026-10-07", "20:00 UTC is still the 7th in New York")
    expect(jump("today", instant, calIn("Pacific/Auckland")) == "2026-10-08", "and already the 8th in Auckland")
    expect(jump("yesterday", instant, calIn("Pacific/Auckland")) == "2026-10-07" && jump("2026-10-08", instant, calIn("Pacific/Auckland")) == "2026-10-08")
    expect(jump("2026-10-08", instant, calIn("America/New_York")) == nil, "the 8th is the future in New York")
}
test("date jump: junk and extreme input never crash and never return an unusable day") {
    let junk = [String(repeating: "9", count: 5000), String(repeating: "a ", count: 3000), String(repeating: "1/", count: 2000), "-" + String(repeating: "9", count: 40),
                "\u{0000}", "oct\u{0000}2", "2\u{202E}oct", "e\u{0301} 2", "1e3", "0x10", "1_000", "٢٠٢٦-١٠-٠٢", "２０２６-１０-０２", "２ oct", "ⅹ", "½",
                "last fri fri fri fri", "-9223372036854775808", "9223372036854775807 days ago", "-0 days ago", "00000000000000000002 oct"]
    for s in junk {
        let r = jump(s)
        expect(r == nil || (r! >= "2000-01-01" && r! <= "2026-10-07" && DayKey.date(r!, cal) != nil), "junk '\(s.prefix(20))' -> \(r ?? "nil")")
    }
    expect(jump("00000000000000000002 oct") == nil || jump("00000000000000000002 oct") == "2026-10-02")
}

// MARK: C4 CatchUp and batch skip
let thu8 = mk(2026, 10, 8, 12)      // Thursday 8 Oct 2026
func missing(_ st: [String: DayStatus] = [:], now: Date = wed7, weekdays wd: Set<Int> = workdays, logStart: String? = "2026-01-01",
             window: Int = 30, cal c: Calendar = cal) -> [String] {
    CatchUp.missing(states: st, now: now, calendar: c, weekdays: wd, logStart: logStart, windowDays: window)
}
/// Every entry of a folder (hidden files too) with its bytes: a byte-for-byte comparison of two states of the folder.
func folderSnapshot(_ dir: URL) throws -> [String: Data] {
    var out = [String: Data]()
    for n in try FileManager.default.contentsOfDirectory(atPath: dir.path) {
        var isDir: ObjCBool = false
        FileManager.default.fileExists(atPath: dir.appendingPathComponent(n).path, isDirectory: &isDir)
        out[n] = isDir.boolValue ? Data("<dir>".utf8) : try Data(contentsOf: dir.appendingPathComponent(n))
    }
    return out
}

test("catch up: lists scheduled .missed and .partial days before today, oldest first") {
    // Wed 7 Oct 2026; Mon-Fri; log start Mon 28 Sep
    let list = missing(["2026-10-05": .logged], logStart: "2026-09-28")
    expect(list == ["2026-09-28", "2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02", "2026-10-06"], "\(list)")
    expect(!list.contains("2026-10-07"), "never today")
    expect(!list.contains("2026-10-03") && !list.contains("2026-10-04"), "weekend is not scheduled")
    expect(list == list.sorted(), "oldest first")
    // states: partial listed; skipped and logged not; a run of skips leaves the list
    let st: [String: DayStatus] = ["2026-09-29": .partial, "2026-09-30": .skipped, "2026-10-01": .logged, "2026-10-05": .skipped, "2026-10-06": .skipped]
    expect(missing(st, logStart: "2026-09-28") == ["2026-09-28", "2026-09-29", "2026-10-02"], "\(missing(st, logStart: "2026-09-28"))")
    // a partial or even an explicit .missed on a day off is never listed; a logged day off neither
    let off: [String: DayStatus] = ["2026-10-03": .partial, "2026-10-04": .missed, "2026-09-26": .logged, "2026-09-27": .partial]
    expect(missing(off, logStart: "2026-09-21") == ["2026-09-21", "2026-09-22", "2026-09-23", "2026-09-24", "2026-09-25", "2026-09-28", "2026-09-29",
                                                    "2026-09-30", "2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"], "\(missing(off, logStart: "2026-09-21"))")
    // future days and today's own state never matter
    expect(!missing(["2026-10-07": .partial, "2026-10-09": .logged, "2026-10-08": .partial], logStart: "2026-10-05").contains("2026-10-07"))
    expect(missing(["2026-10-07": .partial, "2026-10-08": .partial], logStart: "2026-10-05") == ["2026-10-05", "2026-10-06"])
    // every day written: nothing to catch up
    expect(missing(["2026-10-05": .logged, "2026-10-06": .logged, "2026-10-02": .logged], logStart: "2026-10-02").isEmpty)
    // other schedules
    expect(missing([:], weekdays: [1, 7], logStart: "2026-09-28") == ["2026-10-03", "2026-10-04"], "weekend schedule")
    expect(missing([:], weekdays: Set(1...7), logStart: "2026-10-03") == ["2026-10-03", "2026-10-04", "2026-10-05", "2026-10-06"], "every day")
    // on a Monday yesterday is a Sunday: the list ends on Friday
    expect(missing([:], now: mk(2026, 10, 5, 9), logStart: "2026-09-28") == ["2026-09-28", "2026-09-29", "2026-09-30", "2026-10-01", "2026-10-02"])
}
test("catch up: window edges are exact and windowDays is clamped to 7...365") {
    // Thu 8 Oct: 30 days back = Tue 8 Sep (included), 31 back = Mon 7 Sep (excluded)
    let l30 = missing([:], now: thu8, window: 30)
    expect(l30.first == "2026-09-08" && !l30.contains("2026-09-07") && l30.last == "2026-10-07", "\(l30.first ?? "nil") ... \(l30.last ?? "nil")")
    expect(l30.count == 22, "22 workdays between 8 Sep and 7 Oct, got \(l30.count)")
    // 7 days back from Wed 7 Oct = Wed 30 Sep (included), Tue 29 Sep (excluded)
    expect(missing([:], window: 7) == ["2026-09-30", "2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"], "\(missing([:], window: 7))")
    // clamping: below 7 acts as 7, above 365 acts as 365
    for w in [6, 3, 1, 0, -5, Int.min] { expect(missing([:], window: w) == missing([:], window: 7), "window \(w) acts as 7") }
    let l365 = missing([:], now: thu8, logStart: "2025-01-01", window: 365)
    expect(l365.first == "2025-10-08" && !l365.contains("2025-10-07") && l365.last == "2026-10-07", "\(l365.first ?? "nil")")
    for w in [366, 1000, 100_000, Int.max] { expect(missing([:], now: thu8, logStart: "2025-01-01", window: w) == l365, "window \(w) acts as 365") }
    expect(missing([:], now: thu8, logStart: "2025-01-01", window: 364).first == "2025-10-09")
}
test("catch up: the log start cuts the list; no start and no pages means nothing to catch up") {
    expect(missing([:], logStart: "2026-10-02") == ["2026-10-02", "2026-10-05", "2026-10-06"], "start day itself is listed")
    expect(missing(["2026-09-30": .partial], logStart: "2026-10-02") == ["2026-10-02", "2026-10-05", "2026-10-06"], "a partial page before the start is not listed")
    expect(missing([:], logStart: "2026-10-20").isEmpty && missing([:], logStart: "2026-10-07").isEmpty, "a start today or later: nothing before it")
    // nil start = the earliest page
    expect(missing(["2026-09-30": .logged], logStart: nil) == ["2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"])
    expect(missing(["2026-09-30": .skipped], logStart: nil) == ["2026-10-01", "2026-10-02", "2026-10-05", "2026-10-06"], "a skip marker also starts the log")
    expect(missing([:], logStart: nil).isEmpty, "a new user has nothing to catch up on: days before the first page do not count")
    expect(missing([:], logStart: nil, window: 365).isEmpty)
    // M1-A9: log starts 1 Oct, 12 Sep is backfilled: 13-30 Sep are not listed
    let m = missing(["2026-09-12": .logged, "2026-10-05": .logged], logStart: "2026-10-01")
    expect(m == ["2026-10-01", "2026-10-02", "2026-10-06"], "\(m)")
    // the same backfill with no pinned start: the earliest page is the start and the wall of missed days appears (why the app pins it)
    let wall = missing(["2026-09-12": .logged, "2026-10-05": .logged], logStart: nil)
    expect(wall.first == "2026-09-14" && wall.count == 16, "\(wall.count) days without a pinned start")
    // an explicit start wins over an earlier page; an earlier explicit start wins over later pages
    expect(missing(["2026-10-06": .logged], logStart: "2026-10-05") == ["2026-10-05"])
}
test("catch up: a window across DST has no duplicate or missing day (every day scheduled)") {
    let every = Set(1...7)
    for now in [mk(2026, 11, 4, 12), mk(2026, 3, 10, 12), mk(2026, 11, 2, 0, 30), mk(2026, 3, 9, 23, 59)] {
        for w in [7, 30, 365] {
            let l = missing([:], now: now, weekdays: every, logStart: "2000-01-01", window: w)
            let today = DayKey.string(now, cal)
            expect(l.count == w && Set(l).count == w, "\(today) window \(w): \(l.count) unique days")
            expect(l.last == DayKey.adding(today, -1, cal) && l.first == DayKey.adding(today, -w, cal), "\(today) window \(w) edges")
            var consecutive = true
            for i in stride(from: 1, to: l.count, by: 1) where DayKey.adding(l[i - 1], 1, cal) != l[i] { consecutive = false }
            expect(consecutive, "\(today) window \(w) consecutive")
        }
    }
}
test("catch up: a full year window is fast") {
    let t0 = Date()
    var total = 0
    for _ in 0..<5 { total += missing([:], now: thu8, logStart: "2000-01-01", window: 365).count }
    let per = Date().timeIntervalSince(t0) / 5
    expect(total == 5 * 261 || total > 0, "")
    expect(per < 0.05, "365-day window took \(Int(per * 1000)) ms")
}
test("catch up: next(after:in:) is the first entry strictly after the day") {
    let l = ["2026-09-28", "2026-09-29", "2026-10-02", "2026-10-05"]
    expect(CatchUp.next(after: nil, in: l) == "2026-09-28", "nil -> first")
    expect(CatchUp.next(after: "2026-09-28", in: l) == "2026-09-29" && CatchUp.next(after: "2026-09-29", in: l) == "2026-10-02")
    expect(CatchUp.next(after: "2026-09-30", in: l) == "2026-10-02", "a day that is not in the list")
    expect(CatchUp.next(after: "2026-10-05", in: l) == nil && CatchUp.next(after: "2026-12-01", in: l) == nil, "none after the last")
    expect(CatchUp.next(after: "2000-01-01", in: l) == "2026-09-28")
    expect(CatchUp.next(after: nil, in: []) == nil && CatchUp.next(after: "2026-10-01", in: []) == nil, "empty list")
    expect(CatchUp.next(after: nil, in: ["2026-10-02"]) == "2026-10-02" && CatchUp.next(after: "2026-10-01", in: ["2026-10-02"]) == "2026-10-02")
    expect(CatchUp.next(after: "2026-10-02", in: ["2026-10-02"]) == nil && CatchUp.next(after: "2026-10-03", in: ["2026-10-02"]) == nil, "strictly after")
}
test("skipDays: one marker per day, returns only the days it changed, leaves writing alone") {
    let store = freshStore("skipdays"); let dir = store.dir
    try store.save(day: "2026-10-05", body: lorem(30))                       // a written day in the middle of the selection
    try store.save(day: "2026-10-02", body: "## What I did\n\n## Finished")  // headings only: no writing
    try store.save(day: "2026-10-01", body: "![](assets/x.png)")             // an image alone is content
    let before = try folderSnapshot(dir)
    let r = try store.skipDays(["2026-09-29", "2026-09-30", "2026-10-05", "2026-10-02", "2026-10-01"], reason: "Leave")
    expect(r == ["2026-09-29", "2026-09-30", "2026-10-02"], "\(r)")
    for d in r { let doc = try store.load(d); expect(doc?.isSkipped == true && doc?.skipReason == "Leave", d) }
    try expect(try fileText(dir, "2026-09-29") == "# 2026-09-29\n\n> Skipped: Leave\n", "the marker format of skip(_:reason:)")
    let after = try folderSnapshot(dir)
    expect(after["2026-10-05.md"] == before["2026-10-05.md"] && after["2026-10-01.md"] == before["2026-10-01.md"], "pages with writing are byte-identical")
    expect(Set(after.keys) == Set(before.keys).union(["2026-09-29.md", "2026-09-30.md"]), "only markers were added: \(after.keys.sorted())")
    // empty list, empty reason, duplicates and order
    try expect(try store.skipDays([], reason: "Leave").isEmpty)
    let r2 = try store.skipDays(["2026-10-08", "2026-10-07", "2026-10-08"], reason: "")
    expect(r2 == ["2026-10-08", "2026-10-07"], "input order, duplicates once: \(r2)")
    try expect(try fileText(dir, "2026-10-07") == "# 2026-10-07\n\n> Skipped\n", "no reason = plain marker")
    let r3 = try store.skipDays(["2026-10-09"], reason: "Sick\nday  ")
    try expect(r3 == ["2026-10-09"] && (try store.load("2026-10-09")!.skipReason) == "Sick day", "the reason stays on one line")
    // an already skipped day is not ours to rewrite or to undo: left alone, not returned (any reason)
    let r4 = try store.skipDays(["2026-09-29", "2026-10-07"], reason: "Holiday")
    expect(r4.isEmpty, "\(r4)")
    try expect(try store.load("2026-09-29")!.skipReason == "Leave" && (try store.load("2026-10-07")!.skipReason) == "", "reasons are kept")
    // the Catch up list and the file states agree afterwards
    let st = try store.fileStates(minWords: 20)
    expect(st["2026-09-29"] == .skipped && st["2026-10-05"] == .logged && st["2026-10-02"] == .skipped)
    // typed errors, and nothing is written when one entry is malformed
    let snap = try folderSnapshot(dir)
    do { _ = try store.skipDays(["2026-11-02", "../evil", "2026-11-03"], reason: "x"); expect(false, "bad day must throw") }
    catch { expect(error as? LogError == .badDay("../evil"), "\(error)") }
    try expect(try folderSnapshot(dir) == snap, "a malformed entry writes nothing at all")
    do { _ = try store.skipDays(["2026-11-04", "x"], reason: "x"); expect(false) } catch { expect(error as? LogError == .badDay("x")) }
    let missingFolder = LogStore(dir: root.appendingPathComponent("nope-skip-\(UUID().uuidString)"))
    do { _ = try missingFolder.skipDays(["2026-10-05"], reason: "x"); expect(false) } catch { expect(error as? LogError == .folderMissing(missingFolder.dir.path), "\(error)") }
}
test("skipDays: a failing write takes back what the call wrote (all or nothing)") {
    let store = freshStore("skipfail"); let dir = store.dir
    try store.save(day: "2026-10-05", body: lorem(30))
    try FileManager.default.setAttributes([.posixPermissions: 0o555], ofItemAtPath: dir.path)
    do { _ = try store.skipDays(["2026-10-01", "2026-10-02"], reason: "Leave"); expect(false, "unwritable folder must throw") }
    catch { expect(error as? LogError == .folderNotWritable(dir.path), "\(error)") }
    try FileManager.default.setAttributes([.posixPermissions: 0o755], ofItemAtPath: dir.path)
    try expect(try store.listDays() == ["2026-10-05"], "no markers were left behind")
    // a failure in the MIDDLE of a batch (a directory squats on the day file name): the marker written before it is taken back
    let mid = freshStore("skipfail2")
    try FileManager.default.createDirectory(at: mid.dir.appendingPathComponent("2026-10-02.md"), withIntermediateDirectories: false)
    do { _ = try mid.skipDays(["2026-10-01", "2026-10-02", "2026-10-05"], reason: "Leave"); expect(false, "must throw") }
    catch { expect(error as? LogError != nil, "typed error, got \(error)") }
    try expect(try FileManager.default.contentsOfDirectory(atPath: mid.dir.path) == ["2026-10-02.md"], "no marker left behind")
}
test("unskipDays: restores pure markers only; a skip then undo leaves the folder byte-identical") {
    let store = freshStore("unskipdays"); let dir = store.dir
    try store.save(day: "2026-10-05", body: lorem(30))
    try FileManager.default.createDirectory(at: dir.appendingPathComponent("assets"), withIntermediateDirectories: true)
    try pngBytes.write(to: dir.appendingPathComponent("assets/2026-10-05-1a2b3c4d.png"))
    try store.save(day: "2026-10-06", body: lorem(5))                         // a partial day stays untouched by the batch
    let before = try folderSnapshot(dir)
    let skipped = try store.skipDays(["2026-09-29", "2026-09-30", "2026-10-01", "2026-10-05", "2026-10-06"], reason: "Leave")
    expect(skipped == ["2026-09-29", "2026-09-30", "2026-10-01"])
    let restored = try store.unskipDays(skipped)
    expect(restored == skipped, "\(restored)")
    try expect(try folderSnapshot(dir) == before, "folder is byte-identical after skip then undo")
    try expect(try store.unskipDays(skipped).isEmpty, "undo twice does nothing")
    // undo after the user typed into one skipped day: that day is left alone
    let again = try store.skipDays(["2026-09-29", "2026-09-30", "2026-10-01"], reason: "Leave")
    try store.save(day: "2026-09-30", body: "I worked after all today, wrote this by hand.")
    let r = try store.unskipDays(again)
    expect(r == ["2026-09-29", "2026-10-01"], "\(r)")
    try expect(try store.load("2026-09-30")!.body == "I worked after all today, wrote this by hand." && !(try store.load("2026-09-30")!.isSkipped))
    try expect(try store.load("2026-09-29") == nil && (try store.load("2026-10-01")) == nil)
    // a marker someone added notes to is no longer a pure marker: left alone
    try raw("# 2026-10-02\n\n> Skipped: Leave\n\nand some notes I added\n", in: dir, day: "2026-10-02")
    try raw("# 2026-10-07\n\n> Skipped\n", in: dir, day: "2026-10-07")
    try expect(try store.unskipDays(["2026-10-02", "2026-10-07", "2026-10-05", "2026-10-06", "2026-10-30"]) == ["2026-10-07"], "logs, partial pages, notes and missing files are not touched")
    try expect(try fileText(dir, "2026-10-02").contains("and some notes I added"))
    try expect(try store.load("2026-10-05")?.words == 30 && (try store.load("2026-10-06"))?.words == 5)
    // empty list, duplicates, bad day
    try expect(try store.unskipDays([]).isEmpty)
    let two = try store.skipDays(["2026-10-12", "2026-10-13"], reason: "Sick")
    try expect(try store.unskipDays(["2026-10-13", "2026-10-12", "2026-10-13"]) == ["2026-10-13", "2026-10-12"] && two.count == 2, "input order, duplicates once")
    _ = try store.skipDays(["2026-10-14"], reason: "x")
    do { _ = try store.unskipDays(["2026-10-14", "nope"]); expect(false) } catch { expect(error as? LogError == .badDay("nope")) }
    try expect(try store.load("2026-10-14")?.isSkipped == true, "a malformed entry stops the call before anything is deleted")
    // a headings-only page that was skipped has no file after undo (it carried no writing; the template shows it again): documented, not byte-identical
    try store.save(day: "2026-10-19", body: "## What I did\n\n## Finished")
    try expect(try store.skipDays(["2026-10-19"], reason: "Leave") == ["2026-10-19"] && (try store.unskipDays(["2026-10-19"])) == ["2026-10-19"])
    try expect(try store.load("2026-10-19") == nil)
}
test("skip then undo: the streak never falls on skip and is restored exactly on undo; Catch up follows") {
    let store = freshStore("skipstreak")
    let now = mk(2026, 10, 8, 12)                                            // Thu 8 Oct, nothing written yet today
    for d in ["2026-09-28", "2026-09-29", "2026-09-30", "2026-10-06", "2026-10-07"] { try store.save(day: d, body: lorem(25)) }
    // missed: Thu 1 Oct, Fri 2 Oct, Mon 5 Oct
    func state() throws -> (StreakResult, [String]) {
        let st = try store.fileStates(minWords: 20)
        return (Streak.compute(states: st, now: now, calendar: cal, weekdays: workdays, since: "2026-09-28"),
                CatchUp.missing(states: st, now: now, calendar: cal, weekdays: workdays, logStart: "2026-09-28", windowDays: 30))
    }
    let (s0, l0) = try state()
    expect(l0 == ["2026-10-01", "2026-10-02", "2026-10-05"] && s0 == StreakResult(current: 2, best: 3), "before: \(s0) \(l0)")
    let skipped = try store.skipDays(l0, reason: "Leave")
    let (s1, l1) = try state()
    expect(skipped == l0 && l1.isEmpty, "the days leave the list")
    expect(s1.current >= s0.current && s1.best >= s0.best, "skipping never lowers the streak: \(s0) -> \(s1)")
    expect(s1 == StreakResult(current: 5, best: 5), "skipped days are neutral, so the two runs join: \(s1)")
    _ = try store.unskipDays(skipped)
    let (s2, l2) = try state()
    expect(s2 == s0 && l2 == l0, "undo brings the streak and the list back exactly: \(s2)")
}
test("catch up end to end through real files: write the minimum in each day, the list shrinks, next walks on, ends empty") {
    let store = freshStore("catchup-e2e")
    for d in ["2026-09-28", "2026-09-29", "2026-09-30", "2026-10-06", "2026-10-07"] { try store.save(day: d, body: lorem(25)) }
    let now = mk(2026, 10, 8, 12)
    func list() throws -> [String] {
        CatchUp.missing(states: try store.fileStates(minWords: 20), now: now, calendar: cal, weekdays: workdays, logStart: nil, windowDays: 30)
    }
    var l = try list()
    expect(l == ["2026-10-01", "2026-10-02", "2026-10-05"], "exactly the 3 unwritten workdays: \(l)")
    var cur = CatchUp.next(after: nil, in: l)
    expect(cur == "2026-10-01")
    try store.save(day: "2026-10-01", body: lorem(19))
    l = try list()
    expect(l == ["2026-10-01", "2026-10-02", "2026-10-05"], "19 of 20 words stays on the list (partial)")
    try store.save(day: "2026-10-01", body: lorem(20))
    l = try list()
    expect(l == ["2026-10-02", "2026-10-05"], "20 words closes the day: \(l)")
    cur = CatchUp.next(after: cur, in: l)
    expect(cur == "2026-10-02")
    try store.save(day: "2026-10-02", body: lorem(20))
    cur = CatchUp.next(after: cur, in: try list())
    expect(cur == "2026-10-05")
    try store.save(day: "2026-10-05", body: lorem(20))
    try expect(try list().isEmpty && CatchUp.next(after: cur, in: try list()) == nil, "All caught up")
    // a template-only page is not writing: still listed, and a skip marker replaces it
    try store.save(day: "2026-10-05", body: Settings.defaultTemplate)
    try expect(try list() == ["2026-10-05"], "an emptied day returns to the list")
}
test("skipDays: 30 days are fast") {
    let store = freshStore("skipperf")
    let days = (0..<30).map { DayKey.adding("2026-09-01", $0, cal)! }
    let t0 = Date()
    let r = try store.skipDays(days, reason: "Leave")
    let skipT = Date().timeIntervalSince(t0)
    expect(r == days && skipT < 0.25, "skipDays(30) took \(Int(skipT * 1000)) ms")
    let t1 = Date()
    let u = try store.unskipDays(days)
    let unT = Date().timeIntervalSince(t1)
    expect(u == days && unT < 0.25, "unskipDays(30) took \(Int(unT * 1000)) ms")
    print("  note: skipDays(30) \(Int(skipT * 1000)) ms, unskipDays(30) \(Int(unT * 1000)) ms")
}

// MARK: C5 week review completeness
func weekSummary(_ date: Date, states st: [String: DayStatus] = [:], docs: [String: DayDocument] = [:], now: Date, cal c: Calendar = cal,
                 logStart: String? = nil, weekdays wd: Set<Int> = workdays) -> WeekSummary {
    var s = Settings(); s.logStartDate = logStart; s.weekdays = wd
    return WeeklyReview.summary(weekContaining: date, documents: docs, states: st, settings: s, calendar: c, now: now)
}
test("week review: a week with no files lists every scheduled day, all .missed, and offers them as missingDays") {
    let history: [String: DayStatus] = ["2026-09-28": .logged, "2026-09-29": .logged]          // pages exist, but none in the week under review
    let w = weekSummary(mk(2026, 10, 7), states: history, now: mk(2026, 10, 14, 12))
    expect(w.weekStart == "2026-10-05" && w.weekEnd == "2026-10-11" && w.days.count == 7, "\(w.weekStart) \(w.weekEnd) \(w.days.count)")
    let scheduled = w.days.filter { workdays.contains(DayKey.weekday($0.day, cal)) }
    expect(scheduled.map { $0.day } == ["2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09"], "5 entries Mon-Fri")
    expect(scheduled.allSatisfy { $0.status == .missed && $0.doc == nil }, "all .missed, no page")
    expect(w.days.filter { !workdays.contains(DayKey.weekday($0.day, cal)) }.allSatisfy { $0.status == .off }, "weekend .off")
    expect(w.missingDays == ["2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09"], "\(w.missingDays)")
    expect(w.loggedCount == 0 && w.workdayCount == 5 && w.skippedCount == 0 && w.sections.isEmpty)
    // with no page anywhere and no log start nothing is "missing" (a new user), although the statuses keep Status.resolve's rule
    let fresh = weekSummary(mk(2026, 10, 7), now: mk(2026, 10, 14, 12))
    expect(fresh.missingDays.isEmpty && fresh.days.count == 7, "no history, no start: nothing to catch up")
    // a week before the first page: nothing is missed, nothing is missing
    let early = weekSummary(mk(2026, 9, 9), states: history, now: mk(2026, 10, 14, 12))
    expect(early.days.count == 7 && early.days.allSatisfy { $0.status == .off } && early.missingDays.isEmpty, "\(early.days.map { $0.status })")
    // the log start date from Settings mutes the days before it (it used to be ignored here)
    let cut = weekSummary(mk(2026, 10, 7), states: history, now: mk(2026, 10, 14, 12), logStart: "2026-10-07")
    expect(cut.days.map { $0.status } == [.off, .off, .missed, .missed, .missed, .off, .off], "\(cut.days.map { $0.status })")
    expect(cut.missingDays == ["2026-10-07", "2026-10-08", "2026-10-09"], "\(cut.missingDays)")
    // a written page and a skip marker are not missing; a started page is
    let mixed = weekSummary(mk(2026, 10, 7), states: history.merging(["2026-10-05": .logged, "2026-10-06": .skipped, "2026-10-07": .partial], uniquingKeysWith: { a, _ in a }),
                            now: mk(2026, 10, 14, 12))
    expect(mixed.missingDays == ["2026-10-07", "2026-10-08", "2026-10-09"] && mixed.loggedCount == 1 && mixed.skippedCount == 1, "\(mixed.missingDays)")
}
test("week review: weeks start on calendar.firstWeekday") {
    let sun = calIn(firstWeekday: 1)
    let w = weekSummary(mk(2026, 10, 7), states: ["2026-09-28": .logged], now: mk(2026, 10, 14, 12), cal: sun)
    expect(w.weekStart == "2026-10-04" && w.weekEnd == "2026-10-10" && w.days.count == 7, "Sunday-first: \(w.weekStart)...\(w.weekEnd)")
    expect(w.days.map { $0.status } == [.off, .missed, .missed, .missed, .missed, .missed, .off], "\(w.days.map { $0.status })")
    expect(w.missingDays == ["2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09"] && w.workdayCount == 5)
    // a Sunday belongs to the week before it on Monday-first calendars and starts its own week on Sunday-first ones
    let monday = weekSummary(mk(2026, 10, 11), now: mk(2026, 10, 14, 12))
    let sunday = weekSummary(mk(2026, 10, 11), now: mk(2026, 10, 14, 12), cal: sun)
    expect(monday.weekStart == "2026-10-05" && monday.weekEnd == "2026-10-11", "\(monday.weekStart)")
    expect(sunday.weekStart == "2026-10-11" && sunday.weekEnd == "2026-10-17", "\(sunday.weekStart)")
    // Saturday-first, to show it is the calendar and not a Monday/Sunday special case
    let sat = weekSummary(mk(2026, 10, 7), now: mk(2026, 10, 14, 12), cal: calIn(firstWeekday: 7))
    expect(sat.weekStart == "2026-10-03" && sat.weekEnd == "2026-10-09", "\(sat.weekStart)")
}
test("week review: the current week lists future days as .future; missingDays never holds today or the future") {
    let st: [String: DayStatus] = ["2026-09-30": .logged, "2026-10-05": .logged]
    let w = weekSummary(mk(2026, 10, 7), states: st, now: wed7)                    // Wed 7 Oct is today, nothing written yet
    expect(w.days.map { $0.status } == [.logged, .missed, .missed, .future, .future, .future, .future], "\(w.days.map { $0.status })")
    expect(w.days.count == 7, "every day of the week is present")
    expect(w.missingDays == ["2026-10-06"], "Tuesday only: \(w.missingDays)")
    expect(w.workdayCount == 3 && w.loggedCount == 1, "scheduled days not in the future: \(w.workdayCount)")
    // once today has a page it is logged, still not missing; a started page today is not listed either
    let w2 = weekSummary(mk(2026, 10, 7), states: st.merging(["2026-10-07": .partial], uniquingKeysWith: { a, _ in a }), now: wed7)
    expect(w2.missingDays == ["2026-10-06"] && w2.days[2].status == .partial)
    // next week: all future
    let next = weekSummary(mk(2026, 10, 14), states: st, now: wed7)
    expect(next.days.count == 7 && next.days.allSatisfy { $0.status == .future } && next.missingDays.isEmpty && next.workdayCount == 0)
    // Monday morning: the week is only today
    let mon = weekSummary(mk(2026, 10, 5), states: ["2026-09-30": .logged], now: mk(2026, 10, 5, 9))
    expect(mon.days.map { $0.status } == [.missed, .future, .future, .future, .future, .future, .future] && mon.missingDays.isEmpty)
}
test("week review: missingDays is CatchUp.missing restricted to the week") {
    let now = mk(2026, 10, 21, 12)                                                  // Wed 21 Oct
    let st: [String: DayStatus] = ["2026-09-21": .logged, "2026-09-22": .partial, "2026-09-23": .skipped, "2026-09-30": .logged, "2026-10-01": .partial,
                                   "2026-10-02": .skipped, "2026-10-06": .logged, "2026-10-07": .logged, "2026-10-12": .partial, "2026-10-17": .partial,
                                   "2026-10-20": .skipped]
    for logStart in [nil, "2026-09-21", "2026-09-30", "2026-10-07", "2026-10-15"] as [String?] {
        let all = CatchUp.missing(states: st, now: now, calendar: cal, weekdays: workdays, logStart: logStart, windowDays: 365)
        for weekOffset in -5...0 {
            let date = WeeklyReview.shift(now, weeks: weekOffset, calendar: cal)
            let w = weekSummary(date, states: st, now: now, logStart: logStart)
            let days = Set(w.days.map { $0.day })
            expect(w.missingDays == all.filter { days.contains($0) }, "logStart \(logStart ?? "nil") week \(w.weekStart): \(w.missingDays) vs \(all.filter { days.contains($0) })")
        }
    }
}

// MARK: C6 folderStamp
test("folderStamp: stable over (name, mtime, size) of the day files; add, edit, delete, rename and touch change it") {
    let store = freshStore("stamp"); let dir = store.dir; let fm = FileManager.default
    func setMTime(_ day: String, _ d: Date) throws { try fm.setAttributes([.modificationDate: d], ofItemAtPath: store.url(for: day).path) }
    let old = Date(timeIntervalSince1970: 1_700_000_000)
    try store.save(day: "2026-10-05", body: lorem(30)); try store.save(day: "2026-10-06", body: lorem(5)); try store.skip("2026-10-07", reason: "x")
    for d in ["2026-10-05", "2026-10-06", "2026-10-07"] { try setMTime(d, old) }       // pin mtimes so "edit" can only be seen through the stamp
    let a = try store.folderStamp()
    expect(!a.isEmpty, "a folder with days has a stamp")
    try expect(try store.folderStamp() == a && (try store.folderStamp()) == a, "unchanged folder -> identical stamp")
    try expect(try LogStore(dir: dir).folderStamp() == a, "another store over the same folder agrees")
    try expect(try store.listDays().count == 3 && (try store.load("2026-10-05")) != nil && (try store.fileStates(minWords: 20)).count == 3, "reading changes nothing")
    try expect(try store.folderStamp() == a, "reading (listDays, load, fileStates) does not change the stamp")
    try store.save(day: "2026-10-05", body: lorem(30))
    try expect(try store.folderStamp() == a, "saving identical content rewrites nothing")
    // add
    try store.save(day: "2026-10-08", body: "a new page with words")
    let added = try store.folderStamp()
    expect(added != a, "add")
    // delete (an emptied page removes the file): back to exactly the first stamp
    try store.save(day: "2026-10-08", body: "")
    try expect(try store.folderStamp() == a, "delete returns to the earlier stamp")
    // edit with the same size and a new mtime
    try store.save(day: "2026-10-05", body: lorem(30, "x"))
    let edited = try store.folderStamp()
    expect(edited != a, "edit (same size, new mtime)")
    // edit that changes the size but keeps the mtime
    try setMTime("2026-10-05", old)
    let sameMTime = try store.folderStamp()
    expect(sameMTime == a, "same name, size and mtime, different bytes: the same stamp (contents are not read)")
    try store.save(day: "2026-10-05", body: lorem(31)); try setMTime("2026-10-05", old)
    try expect(try store.folderStamp() != a, "edit that changes only the size")
    try store.save(day: "2026-10-05", body: lorem(30)); try setMTime("2026-10-05", old)
    try expect(try store.folderStamp() == a, "same name, size and mtime: same stamp again")
    // touch: only the mtime changes
    try setMTime("2026-10-06", old.addingTimeInterval(60))
    let touched = try store.folderStamp()
    expect(touched != a, "touch")
    try setMTime("2026-10-06", old)
    try expect(try store.folderStamp() == a, "touch undone")
    try setMTime("2026-10-06", old.addingTimeInterval(0.5))
    try expect(try store.folderStamp() != a, "a sub-second mtime change is seen")
    try setMTime("2026-10-06", old)
    // rename: same size and mtime, different name
    try fm.moveItem(at: store.url(for: "2026-10-06"), to: store.url(for: "2026-10-09"))
    try expect(try store.folderStamp() != a, "rename")
    try fm.moveItem(at: store.url(for: "2026-10-09"), to: store.url(for: "2026-10-06"))
    try expect(try store.folderStamp() == a, "renamed back")
    // skip and unskip are file changes too
    try store.skip("2026-10-12", reason: "Leave")
    try expect(try store.folderStamp() != a, "a skip marker is a file")
    try store.unskip("2026-10-12")
    try expect(try store.folderStamp() == a)
    _ = dir
}
test("folderStamp: assets, non-markdown, hidden files, folders and backups are ignored") {
    let store = freshStore("stampignore"); let dir = store.dir; let fm = FileManager.default
    try store.save(day: "2026-10-05", body: lorem(30))
    let a = try store.folderStamp()
    try fm.createDirectory(at: dir.appendingPathComponent("assets"), withIntermediateDirectories: true)
    try pngBytes.write(to: dir.appendingPathComponent("assets/2026-10-05-1a2b3c4d.png"))
    try fm.createDirectory(at: dir.appendingPathComponent("backups/2026-10-05"), withIntermediateDirectories: true)
    try "old".write(to: dir.appendingPathComponent("backups/2026-10-05/20261005-120000.md"), atomically: true, encoding: .utf8)
    for n in ["notes.md", "2026-10-0x.md", "2026-10-5.md", "readme.txt", "2026-10-05.md.bak", ".hidden", ".2026-10-06.md.tmp", "2026-10-05.txt"] {      // not ".MD": on a case-insensitive volume that IS the day file
        try "hi".write(to: dir.appendingPathComponent(n), atomically: true, encoding: .utf8)
    }
    try fm.createDirectory(at: dir.appendingPathComponent("2026-10-11.md"), withIntermediateDirectories: true)     // a folder that looks like a day file
    try expect(try store.folderStamp() == a, "none of those is a day file")
    try pngBytes.write(to: dir.appendingPathComponent("assets/another.png"))
    try "changed".write(to: dir.appendingPathComponent("notes.md"), atomically: true, encoding: .utf8)
    try expect(try store.folderStamp() == a, "editing them changes nothing")
    // a symlink to a day file counts, through its target
    let target = freshDir("stamp-target").appendingPathComponent("page.md")
    try "# 2026-10-13\n\nlinked page".write(to: target, atomically: true, encoding: .utf8)
    try fm.createSymbolicLink(at: dir.appendingPathComponent("2026-10-13.md"), withDestinationURL: target)
    let linked = try store.folderStamp()
    expect(linked != a, "a linked day file is a day file")
    try fm.removeItem(at: target)
    _ = try store.folderStamp()      // a dangling link must not throw
    try fm.removeItem(at: dir.appendingPathComponent("2026-10-13.md"))
    try expect(try store.folderStamp() == a)
    // a missing folder throws the same typed error as the other reads
    let missingFolder = LogStore(dir: root.appendingPathComponent("nope-stamp-\(UUID().uuidString)"))
    do { _ = try missingFolder.folderStamp(); expect(false) } catch { expect(error as? LogError == .folderMissing(missingFolder.dir.path), "\(error)") }
    try expect(try freshStore("stampempty").folderStamp() == (try freshStore("stampempty2").folderStamp()), "empty folders agree")
}
test("folderStamp: never reads file contents (an unreadable page is still stamped)") {
    let store = freshStore("stampread"); let fm = FileManager.default
    try store.save(day: "2026-10-05", body: lorem(30)); try store.save(day: "2026-10-06", body: lorem(30))
    let a = try store.folderStamp()
    try fm.setAttributes([.posixPermissions: 0o000], ofItemAtPath: store.url(for: "2026-10-05").path)
    defer { try? fm.setAttributes([.posixPermissions: 0o644], ofItemAtPath: store.url(for: "2026-10-05").path) }
    let b = try store.folderStamp()
    expect(b.contains("2026-10-05.md") && b.contains("2026-10-06.md"), "both files are in the stamp: \(b)")
    _ = a
}
test("folderStamp: 3,650 day files are stamped in well under 50 ms") {
    let store = freshStore("stampbig"); let fm = FileManager.default
    var d = utcDate(2016, 10, 8)
    for _ in 0..<3650 {
        fm.createFile(atPath: store.dir.appendingPathComponent(utcKey(d) + ".md").path, contents: Data("# \(utcKey(d))\n\nsome words for this day\n".utf8))
        d = utcCal.date(byAdding: .day, value: 1, to: d)!
    }
    let first = try store.folderStamp()
    var best = Double.infinity, total = 0.0
    for _ in 0..<5 {
        let t0 = Date()
        let s = try store.folderStamp()
        let dt = Date().timeIntervalSince(t0)
        best = min(best, dt); total += dt
        expect(s == first, "stable across calls")
    }
    expect(best < 0.05 && total / 5 < 0.05, "3,650 files: best \(Int(best * 1000)) ms, mean \(Int(total / 5 * 1000)) ms")
    expect(first.split(separator: "\n").count == 3650, "one entry per file")
    print("  note: folderStamp over 3,650 files: best \(Int(best * 1000)) ms, mean \(Int(total / 5 * 1000)) ms")
}

// MARK: C6 regressions for what tests/run-kill-tests.sh found
/// A pid that is certainly not running: a child that already exited and was reaped.
func deadPid() throws -> Int32 {
    let p = Process(); p.executableURL = URL(fileURLWithPath: "/usr/bin/true"); try p.run(); p.waitUntilExit(); return p.processIdentifier
}
test("kill regression: a killed write leaves only a hidden temp file; the next store's first save removes it from the log folder and the backups") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    let (s0, bdir) = backupStore("kill-reg"); let dir = s0.dir; let fm = FileManager.default
    try s0.save(day: "2026-10-05", body: lorem(30))
    let pid = try deadPid()
    let strayA = dir.appendingPathComponent(".gloamlog-tmp-\(pid)-AAAA"), strayB = bdir.appendingPathComponent(".gloamlog-tmp-\(pid)-BBBB")
    let strayC = dir.appendingPathComponent(".gloamlog-tmp-garbage")                                  // our prefix, no readable pid: stale
    let live = dir.appendingPathComponent(".gloamlog-tmp-\(getpid())-LIVE")                           // this process is alive: not a leftover
    let foreign = ["notes.md", ".hidden", ".2026-10-05.md.tmp", "2026-10-05.md.sb-9d433aed-Oy56Tb"]   // never ours: never touched
    for u in [strayA, strayB, strayC, live] { try "half a page".write(to: u, atomically: false, encoding: .utf8) }
    for n in foreign { try "keep".write(to: dir.appendingPathComponent(n), atomically: false, encoding: .utf8) }
    // invisible meanwhile: not a day, not an unrecognised file, not part of the folder stamp
    try expect(try s0.listDays() == ["2026-10-05"] && (try s0.folderSummary()) == FolderSummary(logs: 1, skipped: 0, unrecognized: 2), "\(try s0.folderSummary())")
    try expect(!(try s0.folderStamp()).contains("gloamlog"))
    // a fresh store (= the next launch): nothing is swept before it writes, everything of ours that is stale goes with its first save
    let s1 = LogStore(dir: dir, backupDir: bdir, clock: { fakeNow })
    _ = try s1.load("2026-10-05"); _ = try s1.listDays()
    expect(fm.fileExists(atPath: strayA.path) && fm.fileExists(atPath: strayB.path), "reading sweeps nothing")
    fakeNow = fakeNow.addingTimeInterval(30)
    try s1.save(day: "2026-10-05", body: lorem(31))
    expect(!fm.fileExists(atPath: strayA.path), "stale temp in the log folder is removed")
    expect(!fm.fileExists(atPath: strayB.path), "stale temp in the backups folder is removed")
    expect(!fm.fileExists(atPath: strayC.path), "a temp with an unreadable pid is stale")
    expect(fm.fileExists(atPath: live.path), "a temp whose writer is alive is left alone")
    for n in foreign { expect(fm.fileExists(atPath: dir.appendingPathComponent(n).path), "\(n) is not ours and stays") }
    try expect(Set(try fm.contentsOfDirectory(atPath: bdir.path)) == ["2026-10-05"], "backups folder holds only day folders: \(try fm.contentsOfDirectory(atPath: bdir.path))")
    expect(s1.listBackups(day: "2026-10-05").count == 1)
    try fm.removeItem(at: live)
    // every later save by the same store is a plain save: nothing else is deleted, nothing is left behind
    try "later".write(to: dir.appendingPathComponent(".gloamlog-tmp-\(pid)-LATER"), atomically: false, encoding: .utf8)
    try s1.save(day: "2026-10-05", body: lorem(32))
    expect(fm.fileExists(atPath: dir.appendingPathComponent(".gloamlog-tmp-\(pid)-LATER").path), "one sweep per store, at its first write")
    try fm.removeItem(at: dir.appendingPathComponent(".gloamlog-tmp-\(pid)-LATER"))
    for n in foreign { try fm.removeItem(at: dir.appendingPathComponent(n)) }
    try expect(Set(try fm.contentsOfDirectory(atPath: dir.path)) == ["2026-10-05.md"], "a save leaves no temp file")
}
test("kill regression: skip also sweeps; a failed save leaves no temp file behind") {
    fakeNow = mk(2026, 10, 6, 12, 0)
    let (s0, bdir) = backupStore("kill-reg2"); let dir = s0.dir; let fm = FileManager.default
    let pid = try deadPid()
    let stray = dir.appendingPathComponent(".gloamlog-tmp-\(pid)-SKIP")
    try "x".write(to: stray, atomically: false, encoding: .utf8)
    try LogStore(dir: dir, backupDir: bdir).skip("2026-10-07", reason: "Leave")
    expect(!fm.fileExists(atPath: stray.path), "skip is a write: the first one sweeps")
    // a directory squats on a day file name: the save fails with a typed error and leaves nothing behind
    try fm.createDirectory(at: dir.appendingPathComponent("2026-10-08.md"), withIntermediateDirectories: false)
    do { try s0.save(day: "2026-10-08", body: "some words here"); expect(false, "must throw") } catch { expect(isIO(error), "\(error)") }
    try expect(!(try fm.contentsOfDirectory(atPath: dir.path)).contains { $0.hasPrefix(".gloamlog-tmp-") }, "no temp file after a failed write")
    try expect(!(try fm.contentsOfDirectory(atPath: bdir.path)).contains { $0.hasPrefix(".gloamlog-tmp-") })
}
test("kill regression: saving keeps the day file's permissions, as Foundation's atomic write did") {
    let store = freshStore("perm"); let fm = FileManager.default
    func mode(_ d: String) -> Int { ((try? fm.attributesOfItem(atPath: store.url(for: d).path))?[.posixPermissions] as? Int) ?? -1 }
    try store.save(day: "2026-10-05", body: lorem(30))
    try fm.setAttributes([.posixPermissions: 0o600], ofItemAtPath: store.url(for: "2026-10-05").path)
    try store.save(day: "2026-10-05", body: lorem(31))
    expect(mode("2026-10-05") == 0o600, "0600 stays 0600, got \(String(mode("2026-10-05"), radix: 8))")
    try fm.setAttributes([.posixPermissions: 0o444], ofItemAtPath: store.url(for: "2026-10-05").path)
    try store.save(day: "2026-10-05", body: lorem(32))
    try expect(mode("2026-10-05") == 0o444 && (try store.load("2026-10-05"))?.words == 32, "a read-only page is still replaced (the folder is writable), mode kept")
    try fm.setAttributes([.posixPermissions: 0o644], ofItemAtPath: store.url(for: "2026-10-05").path)
}

// MARK: M2 quick capture, stream K (K1 CapturePlacement + Jots rules, K2 CaptureJournal, K3 DayWriter, K4 logged rule)
let jotT0 = mk(2026, 10, 7, 14, 32)   // Wed 7 Oct 2026, 14:32 New York
func jot(_ text: String, stamp: String? = "14:32", todo: Bool = false, day: String = "2026-10-07", source: String = "hotkey") -> CaptureItem {
    CaptureItem(id: UUID().uuidString, ts: jotT0, day: day, text: text, stamp: stamp, todo: todo, source: source)
}
func jr(_ s: String, stamp: String? = "14:32", todo: Bool = false) -> String { CapturePlacement.render(jot(s, stamp: stamp, todo: todo)) }
func ja(_ s: String, to body: String, heading: String = "Jots", stamp: String? = "14:32", todo: Bool = false) -> String {
    CapturePlacement.apply(jot(s, stamp: stamp, todo: todo), to: body, jotsHeading: heading)
}
func jhas(_ item: CaptureItem, _ body: String, heading: String = "Jots") -> Bool { CapturePlacement.contains(item, in: body, jotsHeading: heading) }
func jcounts(_ body: String, _ heading: String = "Jots") -> MarkdownBody.Counts { MarkdownBody.counts(in: body, jotsHeading: heading) }

// MARK: K1 CapturePlacement
test("capture render: time stamp, no stamp, to-do, multi-line") {
    expect(jr("call with Sam about pricing") == "- 14:32 call with Sam about pricing")
    expect(jr("x", stamp: nil) == "- x")
    expect(jr("call Sam", stamp: nil, todo: true) == "- [ ] call Sam")
    expect(jr("call Sam", stamp: "14:32", todo: true) == "- [ ] call Sam", "a to-do never carries a time")
    expect(jr("one\ntwo\n  three  ") == "- 14:32 one\n  two\n  three", "continuation lines are trimmed and indented two spaces")
    expect(jr("one\r\ntwo") == "- 14:32 one\n  two" && jr("one\n\n\ntwo") == "- 14:32 one\n  two", "CRLF, and blank lines inside a note are dropped")
    expect(jr("one\ntwo", stamp: nil, todo: true) == "- [ ] one\n  two")
    expect(jr("  padded  ") == "- 14:32 padded")
    expect(jr("emoji 🎉 \"quotes\" and \\ backslash") == "- 14:32 emoji 🎉 \"quotes\" and \\ backslash", "only the start of a line is ever escaped")
}
test("capture render: a leading # > - + * or 1. is backslash-escaped so a note never becomes a heading or a nested list") {
    expect(jr("# x") == "- 14:32 \\# x" && jr("## x") == "- 14:32 \\## x" && jr("###### x") == "- 14:32 \\###### x" && jr("#") == "- 14:32 \\#")
    expect(jr("- x") == "- 14:32 \\- x" && jr("+ x") == "- 14:32 \\+ x" && jr("* x") == "- 14:32 \\* x")
    expect(jr("1. x") == "- 14:32 1\\. x" && jr("12) x") == "- 14:32 12\\) x" && jr("1.") == "- 14:32 1\\.")
    expect(jr("> x") == "- 14:32 \\> x" && jr(">x") == "- 14:32 \\>x")
    expect(jr("# x", stamp: nil) == "- \\# x" && jr("1. x", stamp: nil, todo: true) == "- [ ] 1\\. x", "also without a time, where it matters most")
    // things that only look like markers stay as typed
    expect(jr("#tag x") == "- 14:32 #tag x" && jr("####### x") == "- 14:32 ####### x")
    expect(jr("-5 degrees") == "- 14:32 -5 degrees" && jr("*bold* word") == "- 14:32 *bold* word" && jr("+1 yes") == "- 14:32 +1 yes")
    expect(jr("1.5x faster") == "- 14:32 1.5x faster" && jr("2026-10-07 plan") == "- 14:32 2026-10-07 plan")
    expect(jr("call # x") == "- 14:32 call # x", "only the start of a line matters")
    // every line of a multi-line note
    expect(jr("a\n# b\n- c\n> d\n1. e") == "- 14:32 a\n  \\# b\n  \\- c\n  \\> d\n  1\\. e")
    // setext underlines, thematic breaks, fences and HTML comments would also rewrite the page structure
    expect(jr("a\n---") == "- 14:32 a\n  \\---" && jr("a\n===") == "- 14:32 a\n  \\===" && jr("***") == "- 14:32 \\***" && jr("- - -") == "- 14:32 \\- - -")
    expect(jr("```") == "- 14:32 \\```" && jr("~~~x") == "- 14:32 \\~~~x")
    expect(jr("see <!-- x") == "- 14:32 see <\\!-- x" && jr("<!--") == "- 14:32 <\\!--")
    // the escaped text still reads as the same words and can never produce a heading
    for t in ["# x", "## y z", "- a", "1. b", "> c", "a\n# b", "```", "<!-- x"] {
        let page = ja(t, to: "")
        expect(MarkdownBody.sections(of: page).filter { $0.level > 0 }.map { $0.normalizedTitle } == ["jots"], "no extra heading for \(t.debugDescription): \(page.debugDescription)")
    }
}
test("capture apply: no Jots block creates one at the end and keeps the page") {
    expect(ja("call Sam", to: "") == "## Jots\n\n- 14:32 call Sam")
    expect(ja("call Sam", to: "\n\n  \n") == "## Jots\n\n- 14:32 call Sam")
    expect(ja("call Sam", to: "Some text here.\n\n") == "Some text here.\n\n## Jots\n\n- 14:32 call Sam")
    expect(ja("x", to: Settings.defaultTemplate) == Settings.defaultTemplate + "\n\n## Jots\n\n- 14:32 x", "a template page keeps its headings")
    expect(ja("x", to: "## Jotsy\n\ntext") == "## Jotsy\n\ntext\n\n## Jots\n\n- 14:32 x", "a similar heading is not the Jots heading")
    expect(ja("x", to: "```\n## Jots\n```") == "```\n## Jots\n```\n\n## Jots\n\n- 14:32 x", "a heading inside a code fence is not a heading")
    expect(ja("x", to: "<!--\n## Jots\n-->\ntext") == "<!--\n## Jots\n-->\ntext\n\n## Jots\n\n- 14:32 x", "nor one inside a comment")
    expect(ja("x", to: "a\r\nb") == "a\nb\n\n## Jots\n\n- 14:32 x", "CRLF becomes LF")
    expect(ja("x", to: "text", stamp: nil, todo: true) == "text\n\n## Jots\n\n- [ ] x")
}
test("capture apply: an existing Jots block gets the line at its end, joined to the list") {
    expect(ja("b", to: "text\n\n## Jots\n\n- 14:30 a") == "text\n\n## Jots\n\n- 14:30 a\n- 14:32 b", "last block, ends in a list")
    expect(ja("b", to: "## Jots\n\n- 14:30 a\n\n## Finished\n\nstuff") == "## Jots\n\n- 14:30 a\n- 14:32 b\n\n## Finished\n\nstuff", "block in the middle")
    expect(ja("b", to: "## Jots\n\n- 14:30 a\n\n\n") == "## Jots\n\n- 14:30 a\n- 14:32 b", "trailing blank lines are not content")
    expect(ja("b", to: "## Jots\n\nremember this") == "## Jots\n\nremember this\n\n- 14:32 b", "last content line a paragraph: one blank line")
    expect(ja("b", to: "## Jots\n\n- 14:30 a\n  more") == "## Jots\n\n- 14:30 a\n  more\n- 14:32 b", "a multi-line item above is still a list")
    expect(ja("b", to: "## Jots\n\n- 14:30 a\nlazy") == "## Jots\n\n- 14:30 a\nlazy\n- 14:32 b", "so is a lazy continuation")
    expect(ja("b", to: "## Jots\n\n- a\n\npara") == "## Jots\n\n- a\n\npara\n\n- 14:32 b", "a paragraph after the list needs the blank line again")
    expect(ja("b", to: "## Jots\n\n1. a") == "## Jots\n\n1. a\n- 14:32 b" && ja("b", to: "## Jots\n- [ ] a") == "## Jots\n- [ ] a\n- 14:32 b", "ordered and task items are list items")
    expect(ja("b", to: "## Jots\n\n```\n- code\n```") == "## Jots\n\n```\n- code\n```\n\n- 14:32 b", "a list-looking line inside code is not a list")
    // empty blocks
    expect(ja("b", to: "## Jots") == "## Jots\n\n- 14:32 b" && ja("b", to: "## Jots\n\n") == "## Jots\n\n- 14:32 b")
    expect(ja("b", to: "## Jots\n\n## Next\n\nstuff") == "## Jots\n\n- 14:32 b\n\n## Next\n\nstuff")
    expect(ja("b", to: "## Jots\n## Next") == "## Jots\n\n- 14:32 b\n\n## Next")
    // multi-line note
    expect(ja("b\nc", to: "## Jots\n\n- 14:30 a") == "## Jots\n\n- 14:30 a\n- 14:32 b\n  c")
    // two applies in a row build one list in order
    let twice = CapturePlacement.apply(jot("c", stamp: "14:33"), to: ja("b", to: "## Jots\n\n- 14:30 a"), jotsHeading: "Jots")
    expect(twice == "## Jots\n\n- 14:30 a\n- 14:32 b\n- 14:33 c")
}
test("capture apply: block boundaries by heading level; case, emoji and the configured heading") {
    expect(ja("b", to: "### Jots\n- a\n\n## Next\ntext") == "### Jots\n- a\n- 14:32 b\n\n## Next\ntext", "a higher heading ends the block")
    expect(ja("b", to: "### Jots\n- a\n#### Sub\nmore") == "### Jots\n- a\n#### Sub\nmore\n\n- 14:32 b", "a deeper heading stays inside: the line goes after it")
    expect(ja("b", to: "# Jots\n- a\n## Sub\ntext\n# Next\nx") == "# Jots\n- a\n## Sub\ntext\n\n- 14:32 b\n\n# Next\nx")
    expect(ja("b", to: "## 📝 JOTS\n- a") == "## 📝 JOTS\n- a\n- 14:32 b" && ja("b", to: "## jots!\n- a") == "## jots!\n- a\n- 14:32 b", "case, emoji, punctuation")
    expect(ja("b", to: "## Jots\n- a", heading: "📝 Jots") == "## Jots\n- a\n- 14:32 b", "matching is by normalised text")
    expect(ja("b", to: "text", heading: "📝 Jots") == "text\n\n## 📝 Jots\n\n- 14:32 b", "a new heading uses the configured text")
    expect(ja("b", to: "text", heading: "") == "text\n\n## Jots\n\n- 14:32 b" && ja("b", to: "text", heading: "🙂") == "text\n\n## Jots\n\n- 14:32 b", "an unusable heading falls back to Jots")
    expect(ja("b", to: "text", heading: "  Daily \n notes ") == "text\n\n## Daily notes\n\n- 14:32 b")
    expect(ja("b", to: "## Notes\n- a", heading: "Notes") == "## Notes\n- a\n- 14:32 b")
    expect(ja("b", to: "## Jots\n- a\n\n## Other\n\n## Jots\n- z") == "## Jots\n- a\n- 14:32 b\n\n## Other\n\n## Jots\n- z", "the first Jots block is the target, like the editor")
    expect(ja("b", to: "## What I did\n\nwrote\n\n### 🗒️ Jots\n\n- 08:55 a\n\n#### Later\n\n- 09:30 z\n\n### Finished\n\nshipped")
           == "## What I did\n\nwrote\n\n### 🗒️ Jots\n\n- 08:55 a\n\n#### Later\n\n- 09:30 z\n- 14:32 b\n\n### Finished\n\nshipped", "the editor fixture shape")
}
test("capture contains: true after apply; ignores list-marker style, escapes, a ticked box and soft breaks; false elsewhere") {
    let bodies = ["", "text", "## Jots\n\n- 14:30 a", "## Jots\n\n- 14:30 a\n\n## Finished\n\nstuff", Settings.defaultTemplate, "## Jots", "### Jots\n- a\n## N\nz", "## Jots\n\nremember"]
    for text in ["call Sam", "# x", "- y", "a\nb", "1. n", "caf\u{e9} \u{1F389}"] {
        for b in bodies {
            let it = jot(text)
            expect(!jhas(it, b), "not yet in \(b.debugDescription)")
            let applied = CapturePlacement.apply(it, to: b, jotsHeading: "Jots")
            expect(jhas(it, applied), "contains after apply: \(text.debugDescription) into \(b.debugDescription) -> \(applied.debugDescription)")
            expect(!jhas(jot(text + " more"), applied) && !jhas(jot(text, stamp: "09:00"), applied), "other text or time is another note")
        }
    }
    // what the editor may write back
    expect(jhas(jot("call Sam"), "## Jots\n\n* 14:32 call Sam") && jhas(jot("call Sam"), "## Jots\n\n+ 14:32   call   Sam  "), "marker style, spaces")
    expect(jhas(jot("# x"), "## Jots\n\n- 14:32 # x") && jhas(jot("2 * 3 = 6"), "## Jots\n\n- 14:32 2 \\* 3 = 6") && jhas(jot("a_b"), "## Jots\n\n- 14:32 a\\_b"), "escape differences")
    expect(jhas(jot("call Sam", stamp: nil, todo: true), "## Jots\n\n- [x] call Sam"), "a to-do ticked since is still the same note")
    expect(jhas(jot("a\nb"), "## Jots\n\n- 14:32 a\n  b") && jhas(jot("a\nb"), "## Jots\n\n- 14:32 a\\\n  b") && jhas(jot("a\nb"), "## Jots\n\n- 14:32 a b"), "soft and hard breaks")
    // not the same note
    expect(!jhas(jot("call Sam"), "## Jots\n\n- 14:33 call Sam") && !jhas(jot("call Sam"), "## Jots\n\n- 14:32 call Sam now") && !jhas(jot("call Sam"), "## Jots\n\n- 14:32 Call Sam"))
    expect(!jhas(jot("call Sam"), "## Finished\n\n- 14:32 call Sam"), "outside the Jots block")
    expect(!jhas(jot("call Sam"), "## Jots\n\ntext\n\n## Finished\n\n- 14:32 call Sam"), "after the block ended")
    expect(jhas(jot("call Sam"), "## Jots\n\n- a\n\n## X\n\n## Jots\n\n- 14:32 call Sam"), "in any Jots block")
    expect(!jhas(jot("call Sam"), "## Jots\n\n```\n- 14:32 call Sam\n```"), "code is not a note")
    expect(!jhas(jot("call Sam", stamp: nil, todo: true), "## Jots\n\n- 14:32 call Sam") && !jhas(jot("call Sam"), "## Jots\n\n- [ ] call Sam"), "a to-do is not a stamped note")
    expect(jhas(jot("x", stamp: nil), "## Jots\n- x") && !jhas(jot("x", stamp: nil), "## Jots\n- 14:32 x"), "no stamp")
    expect(jhas(jot("b"), "## Notes\n- 14:32 b", heading: "Notes") && !jhas(jot("b"), "## Notes\n- 14:32 b"), "the configured heading")
}

test("capture apply: a page that ends inside an open fence or comment still gets a visible note (no swallowed note, no endless replay)") {
    expect(ja("b", to: "```\ncode") == "```\ncode\n```\n\n## Jots\n\n- 14:32 b", "no block, open fence")
    expect(ja("b", to: "~~~~\ncode") == "~~~~\ncode\n~~~~\n\n## Jots\n\n- 14:32 b", "the closing fence copies the opener")
    expect(ja("b", to: "## Jots\n- a\n```\ncode") == "## Jots\n- a\n```\ncode\n```\n\n- 14:32 b", "block, open fence")
    expect(ja("b", to: "text <!-- open") == "text <!-- open\n-->\n\n## Jots\n\n- 14:32 b", "open comment")
    expect(ja("b", to: "## Jots\n- a\ntext <!-- open\nmore") == "## Jots\n- a\ntext <!-- open\n-->\n\n- 14:32 b\n\nmore")
    for body in ["```\ncode", "## Jots\n- a\n```\ncode", "text <!-- open", "~~~\n## Jots\n- a"] {
        let it = jot("zz")
        let out = CapturePlacement.apply(it, to: body, jotsHeading: "Jots")
        expect(jhas(it, out) && MarkdownBody.counts(in: out).jotCount >= 1, body.debugDescription)
        expect(CapturePlacement.apply(it, to: out, jotsHeading: "Jots") != out, "apply is not its own guard: contains is")
    }
}
struct LCG {
    var s: UInt64
    mutating func next() -> Int { s = s &* 6364136223846793005 &+ 1442695040888963407; return Int(s >> 33) }
    mutating func pick<T>(_ a: [T]) -> T { a[next() % a.count] }
}
test("capture apply: fuzz - the page's own lines are never lost, reordered or edited; the note is always recognised afterwards") {
    var g = LCG(s: 20261007)
    let safe = ["", "text words here", "## Jots", "### Jots", "# Top", "## Other", "- item", "- 14:30 note", "1. one", "  more", "> quote", "---",
                "- [ ] task", "para line", "## Carried over from Mon", "#### deep", "- 08:00 [x] looks like a task", "# Jots"]
    let hazard = safe + ["```", "~~~", "code line", "<!-- c", "-->", "<!-- all -->", "```swift"]
    for n in 0..<600 {
        let hazardous = n % 2 == 1
        let lines = (0..<(g.next() % 14)).map { _ in g.pick(hazardous ? hazard : safe) }
        let body = lines.joined(separator: "\n")
        let it = jot("zz\(n) note")
        expect(!jhas(it, body), "fresh note is not in \(body.debugDescription)")
        let out = CapturePlacement.apply(it, to: body, jotsHeading: "Jots")
        expect(jhas(it, out), "recognised after apply: \(body.debugDescription) -> \(out.debugDescription)")
        var rest = out.components(separatedBy: "\n")[...]
        var kept = true
        for l in lines where !l.dlTrimmed.isEmpty {
            if let i = rest.firstIndex(of: l) { rest = rest[(i + 1)...] } else { kept = false; break }
        }
        expect(kept, "own lines kept in order: \(body.debugDescription) -> \(out.debugDescription)")
        if !hazardous {
            expect(MarkdownBody.loggedWords(in: out) == MarkdownBody.loggedWords(in: body), "jots never change the logged words: \(body.debugDescription)")
            expect(MarkdownBody.counts(in: out).jotCount == MarkdownBody.counts(in: body).jotCount + 1, "one more jot: \(body.debugDescription) -> \(out.debugDescription)")
            func headingCount(_ s: String) -> Int { MarkdownBody.scan(s).filter { if case .heading = $0.kind { return true }; return false }.count }
            let hadBlock = !MarkdownBody.jotsBlocks(in: MarkdownBody.scan(body), jotsHeading: "Jots").isEmpty
            expect(headingCount(out) == headingCount(body) + (hadBlock ? 0 : 1), "only a missing Jots heading is ever added: \(body.debugDescription) -> \(out.debugDescription)")
        }
        // a second, different note joins the same block and the first is still recognised
        let it2 = jot("yy\(n) other", stamp: "14:40")
        let out2 = CapturePlacement.apply(it2, to: out, jotsHeading: "Jots")
        expect(jhas(it, out2) && jhas(it2, out2), "two notes: \(out2.debugDescription)")
    }
}

// MARK: K1 Jots rules in the words rule
test("jots rules: counts split the page at the Jots block; a leading time is not a word") {
    expect(jcounts("") == MarkdownBody.Counts(words: 0, jotWords: 0, jotCount: 0))
    expect(jcounts(lorem(5)) == MarkdownBody.Counts(words: 5, jotWords: 0, jotCount: 0), "no Jots block: everything is words")
    expect(jcounts("intro words here\n\n## Jots\n\n- 14:32 call Sam\n- 14:40 email Ana now") == MarkdownBody.Counts(words: 3, jotWords: 5, jotCount: 2))
    expect(jcounts("## Jots\n- 14:32 meet at 15:00") == MarkdownBody.Counts(words: 0, jotWords: 3, jotCount: 1), "only a LEADING time is dropped")
    expect(jcounts("## Jots\n- [ ] call Sam\n- plain note here") == MarkdownBody.Counts(words: 0, jotWords: 5, jotCount: 2), "a to-do and a note without a time")
    expect(jcounts("## Jots\n- 9:05 a\n- 23:59 b\n- 24:00 c\n- 12:75 d\n- 7:5 e") == MarkdownBody.Counts(words: 0, jotWords: 1 + 1 + 2 + 2 + 2, jotCount: 5), "only a real HH:mm is dropped")
    expect(jcounts("## Jots\n- 14:32\n- 14:33 ") == MarkdownBody.Counts(words: 0, jotWords: 0, jotCount: 2), "a bare time is a jot with no words")
    expect(jcounts("## Jots\n14:32 typed by hand\n- 14:33 x") == MarkdownBody.Counts(words: 0, jotWords: 4 + 1, jotCount: 1), "a time that starts a paragraph is the user's own text")
    expect(jcounts("## Jots\n- 14:32 a b\n  c d\n  - nested e\n- 14:33 f") == MarkdownBody.Counts(words: 0, jotWords: 7, jotCount: 2), "continuation and nested lines count as words, nested items are not jots")
    expect(jcounts("## Jots\n- a b\n## Finished\nfour words right here") == MarkdownBody.Counts(words: 4, jotWords: 2, jotCount: 1), "the next heading of the same level ends the block")
    expect(jcounts("## Notes\nn1 n2\n### Jots\n- j1 j2\n## Other\no1") == MarkdownBody.Counts(words: 3, jotWords: 2, jotCount: 1), "### Jots under ## Notes")
    expect(jcounts("## Jots\n- a\n### sub\nmore words") == MarkdownBody.Counts(words: 0, jotWords: 3, jotCount: 1), "a deeper heading stays inside the block")
    expect(jcounts("# Jots\n- a\n## sub\nmore\n# Next\nx y") == MarkdownBody.Counts(words: 2, jotWords: 2, jotCount: 1))
    expect(jcounts("## Carried over from Mon\n- [ ] x y\n## Jots\n- j") == MarkdownBody.Counts(words: 0, jotWords: 1, jotCount: 1), "carried tasks still never count")
    expect(jcounts("## Jots\n- a\n```\ncode here\n```") == MarkdownBody.Counts(words: 0, jotWords: 3, jotCount: 1), "code words count, fence lines do not")
    expect(jcounts("## Jots\n<!-- hidden words -->\n- a") == MarkdownBody.Counts(words: 0, jotWords: 1, jotCount: 1), "comments are not words")
    expect(jcounts("## Jots\n- a\n## X\nw\n## Jots\n- b c") == MarkdownBody.Counts(words: 1, jotWords: 3, jotCount: 2), "every Jots block counts as jots")
    expect(jcounts("## JOTS!\n- a") == MarkdownBody.Counts(words: 0, jotWords: 1, jotCount: 1) && jcounts("## 📝 Jots\n- a").jotCount == 1)
    expect(jcounts("## Jotsy\n- a b") == MarkdownBody.Counts(words: 2, jotWords: 0, jotCount: 0) && jcounts("##\n- a b").jotCount == 0, "a similar or empty heading is not a Jots heading")
    expect(jcounts("```\n## Jots\n- a\n```").jotCount == 0, "a heading inside a fence is no heading")
    expect(jcounts("## Notes\n- a b", "Notes") == MarkdownBody.Counts(words: 0, jotWords: 2, jotCount: 1) && jcounts("## Jots\n- a b", "") == MarkdownBody.Counts(words: 0, jotWords: 2, jotCount: 1), "configured heading; an unusable one falls back to Jots")
    // a page without a Jots block counts exactly like words(in:)
    for b in [Settings.defaultTemplate, "## A\ntext here\n## Carried over from Mon\n- [ ] x\n- y", "![i](a.png) word", "```\ncode\n```", lorem(30)] {
        expect(jcounts(b) == MarkdownBody.Counts(words: W(b), jotWords: 0, jotCount: 0), b)
    }
}
test("jots rules: loggedWords is words, plus jot words only when asked") {
    let body = "intro words here\n\n## Jots\n\n- 14:32 call Sam\n- 14:40 email Ana now"
    expect(MarkdownBody.loggedWords(in: body) == 3 && MarkdownBody.loggedWords(in: body, jotsCount: false) == 3)
    expect(MarkdownBody.loggedWords(in: body, jotsCount: true) == 8, "the two times are still not words")
    expect(MarkdownBody.loggedWords(in: body, jotsHeading: "Notes", jotsCount: false) == 10, "no such block: everything counts")
    expect(MarkdownBody.loggedWords(in: lorem(7)) == 7 && MarkdownBody.loggedWords(in: "") == 0)
    expect(MarkdownBody.loggedWords(in: "## Jots\n- a b c") == 0 && MarkdownBody.loggedWords(in: "## Jots\n- a b c", jotsCount: true) == 3)
}
test("jots rules: isJotsOnly") {
    expect(!MarkdownBody.isJotsOnly("") && !MarkdownBody.isJotsOnly("\n \n"), "an empty page is not jots-only")
    for b in ["## Jots\n\n- 14:32 a", "\n\n## Jots\n- a\n\n", "## Jots", "### Jots\n- a", "# Jots\n- a\n## Sub\nx", "## Jots\n- a\n### sub\nx",
              "## Jots\n- a\n## Jots\n- b", "<!-- c -->\n## Jots\n- a", "## 📝 jots\n- a", "  \n## Jots\n- 14:32 a\n- 14:40 b\n"] {
        expect(MarkdownBody.isJotsOnly(b), b.debugDescription)
    }
    for b in ["text\n## Jots\n- a", "## Jots\n- a\n## Other\nx", Settings.defaultTemplate + "\n\n## Jots\n- a", "- a", "```\n## Jots\n```", "## Jotsy\n- a", "## What I did\n\n## Jots\n- a"] {
        expect(!MarkdownBody.isJotsOnly(b), b.debugDescription)
    }
    expect(MarkdownBody.isJotsOnly("## Notes\n- a", jotsHeading: "Notes") && !MarkdownBody.isJotsOnly("## Jots\n- a", jotsHeading: "Notes"))
}
test("jots rules: stripUntouchedTemplate (the template above a jots-only page is never written back)") {
    let t = Settings.defaultTemplate
    let jots = "## Jots\n\n- 14:32 call Sam\n- 14:40 email Ana"
    func strip(_ md: String, _ template: String = t, _ heading: String = "Jots") -> String { MarkdownBody.stripUntouchedTemplate(md, template: template, jotsHeading: heading) }
    expect(strip(t + "\n\n" + jots) == jots, "template + jots round trip")
    expect(MarkdownBody.isJotsOnly(strip(t + "\n\n" + jots)))
    expect(strip(t) == "" && strip("\n" + t + "\n\n") == "", "the template alone has nothing to write")
    expect(strip(t + "\n" + jots) == jots && strip("\n\n" + t + "\n\n\n" + jots + "\n\n") == jots, "blank lines around do not matter")
    expect(strip("## What I did\n## Finished\n## Started\n## Pending / blocked\n## To do next\n\n" + jots) == jots, "nor between template headings")
    expect(strip(t + "\n\n### Jots\n- a") == "### Jots\n- a" && strip(t + "\n\n## Jots\n- a\n### sub\ntext") == "## Jots\n- a\n### sub\ntext")
    // anything the user wrote is left alone
    for md in [t + "\n\nmy own text", t + "\n\n" + jots + "\n\n## Finished\nreal text", "## What I did\n\nwrote code\n\n## Finished\n\n" + jots,
               jots, "", "my own text", t.replacingOccurrences(of: "## Started", with: "## Begun") + "\n\n" + jots, "## What I did\n\n## Finished"] {
        expect(strip(md) == md, md.debugDescription)
    }
    expect(strip(t + "\n\n" + jots, "") == t + "\n\n" + jots, "no template, nothing to strip")
    expect(strip("## A\n\n## B\n\n## Jots\n- a", "## A\n\n## B") == "## Jots\n- a" && strip("## AB\n\n## Jots\n- a", "## A") == "## AB\n\n## Jots\n- a", "a prefix inside a word is not the template")
    expect(strip(t + "\n\n## Notes\n- a", t, "Notes") == "## Notes\n- a" && strip(t + "\n\n## Jots\n- a", t, "Notes") == t + "\n\n## Jots\n- a")
}

// MARK: K2 CaptureJournal
func jdir(_ name: String) -> URL { root.appendingPathComponent("jr-" + name + "-" + UUID().uuidString.prefix(6)) }   // not created: the journal makes it
func jfile(_ dir: URL) -> URL { dir.appendingPathComponent("journal.jsonl") }
func fmode(_ u: URL) -> Int { ((try? FileManager.default.attributesOfItem(atPath: u.path))?[.posixPermissions] as? Int) ?? -1 }
func appendRaw(_ s: String, to u: URL) throws {
    try FileManager.default.createDirectory(at: u.deletingLastPathComponent(), withIntermediateDirectories: true)
    if !FileManager.default.fileExists(atPath: u.path) { FileManager.default.createFile(atPath: u.path, contents: nil, attributes: [.posixPermissions: 0o600]) }
    let h = try FileHandle(forWritingTo: u); defer { try? h.close() }
    _ = try h.seekToEnd(); try h.write(contentsOf: Data(s.utf8))
}
func jlines(_ u: URL) -> [String] { ((try? String(contentsOf: u, encoding: .utf8)) ?? "").components(separatedBy: "\n").filter { !$0.isEmpty } }
func jitem(_ n: Int, text: String? = nil, day: String = "2026-10-07") -> CaptureItem {
    CaptureItem(id: "id-\(n)", ts: jotT0.addingTimeInterval(Double(n)), day: day, text: text ?? "note \(n)", stamp: "14:32", todo: false, source: "hotkey")
}

test("journal: append, pending and ack in order; the folder is created; the file is 0600") {
    let d = jdir("basic"); let j = CaptureJournal(dir: d)
    let (a, b, c) = (jitem(1), jitem(2), jitem(3))
    expect(j.pending().isEmpty, "no file yet")
    try j.append(a); try j.append(b); try j.append(c)
    expect(j.pending() == [a, b, c])
    try j.ack(b.id)
    expect(j.pending() == [a, c])
    try j.ack("never-seen")
    expect(j.pending() == [a, c], "an unknown ack changes nothing")
    try j.ack(a.id); try j.ack(c.id)
    expect(j.pending().isEmpty)
    expect(jfile(d).path == j.fileURL.path && fmode(jfile(d)) == 0o600, "journal.jsonl, mode \(String(fmode(jfile(d)), radix: 8))")
    expect(fmode(d) == 0o700, "the folder is private too: \(String(fmode(d), radix: 8))")
    expect(CaptureJournal(dir: d).pending().isEmpty && CaptureJournal(dir: d).fileURL.path == j.fileURL.path)
}
test("journal: one JSON line per record, and any text survives") {
    let d = jdir("format"); let j = CaptureJournal(dir: d)
    let wild = "line one\nline \"two\" \\ back\tslash \u{2028} sep \u{1F468}\u{200D}\u{1F469}\u{200D}\u{1F467} caf\u{e9} </script> {\"x\":1}"
    let items = [jitem(1, text: wild), jitem(2, text: String(repeating: "word ", count: 800)), jitem(3, text: "[] literal")]
    for it in items { try j.append(it) }
    try j.ack("id-9")
    let lines = jlines(jfile(d))
    expect(lines.count == 4, "three notes and one ack, one physical line each: \(lines.count)")
    for l in lines.dropLast() {
        let o = try JSONSerialization.jsonObject(with: Data(l.utf8)) as? [String: Any]
        expect(o?["v"] as? Int == 1 && o?["id"] is String && o?["ts"] is String && o?["day"] as? String == "2026-10-07"
               && o?["text"] is String && o?["source"] as? String == "hotkey" && o?["todo"] as? Bool == false, l)
    }
    expect(lines[0].contains("\"ts\":\"2026-10-07T18:32:01") && lines[0].contains("\"stamp\":\"14:32\""), lines[0])
    expect(lines.last == "{\"ack\":\"id-9\"}", lines.last ?? "")
    expect(j.pending() == items, "exact round trip")
    let withMs = CaptureItem(id: "ms", ts: Date(timeIntervalSince1970: 1_790_000_000.123), day: "2026-10-07", text: "x", stamp: nil, todo: true, source: "menu")
    try j.append(withMs)
    let back = j.pending().last
    expect(back?.stamp == nil && back?.todo == true && back?.source == "menu" && abs((back?.ts.timeIntervalSince1970 ?? 0) - 1_790_000_000.123) < 0.001, "milliseconds, nil stamp, to-do")
}
test("journal: a torn last line never loses earlier notes, and the next append is not glued onto it") {
    let d = jdir("torn"); let j = CaptureJournal(dir: d)
    let (a, b, c) = (jitem(1), jitem(2), jitem(3))
    try j.append(a); try j.append(b)
    try appendRaw("{\"v\":1,\"id\":\"torn\",\"ts\":\"2026-10-07T14:3", to: jfile(d))      // power cut mid-write: no newline
    expect(j.pending() == [a, b], "the torn line is ignored, earlier notes intact")
    try j.append(c)
    expect(j.pending() == [a, b, c], "a new note after a torn line is a note of its own")
    expect(jlines(jfile(d)).count == 4, "the fragment sits on its own line")
    try appendRaw("{\"ack\":\"id-", to: jfile(d))                                           // a torn ack
    expect(j.pending() == [a, b, c], "a torn ack acknowledges nothing")
    try j.ack(a.id)
    expect(j.pending() == [b, c])
}
test("journal: unknown, garbled and future lines are skipped; old records and duplicates are tolerated") {
    let d = jdir("garbage"); let j = CaptureJournal(dir: d)
    let good = jitem(1)
    var text = ["", "not json at all", "{\"foo\":1}", "[1,2]", "42", "\"str\"", "{\"ack\":42}", "{\"v\":9,\"kind\":\"future\"}", "{\"ack\":null}",
                "{\"id\":\"bad-day\",\"ts\":\"2026-10-07T14:32:11Z\",\"day\":\"../../etc\",\"text\":\"x\",\"source\":\"hotkey\"}",
                "{\"id\":\"no-text\",\"ts\":\"2026-10-07T14:32:11Z\",\"day\":\"2026-10-07\",\"text\":\"\",\"source\":\"hotkey\"}",
                "{\"id\":\"no-ts\",\"day\":\"2026-10-07\",\"text\":\"x\",\"source\":\"hotkey\"}",
                CaptureJournal.line(for: good), CaptureJournal.line(for: good)]            // the same id twice
    text += ["{\"id\":\"old\",\"ts\":\"2026-10-07T14:32:11Z\",\"day\":\"2026-10-07\",\"text\":\"hi\",\"stamp\":\"14:32\",\"source\":\"hotkey\"}",   // no v, no todo
             "{\"v\":2,\"id\":\"newer\",\"ts\":\"2026-10-07T14:33:00.250Z\",\"day\":\"2026-10-07\",\"text\":\"from the future\",\"stamp\":null,\"todo\":true,\"source\":\"x\",\"extra\":[1]}",
             "{\"ack\":\"never\",\"also\":1}"]
    try appendRaw(text.joined(separator: "\n") + "\n", to: jfile(d))
    let p = j.pending()
    expect(p.map { $0.id } == ["id-1", "old", "newer"], "\(p.map { $0.id })")
    expect(p[1].todo == false && p[1].stamp == "14:32" && p[1].ts == ISO8601DateFormatter().date(from: "2026-10-07T14:32:11Z") && p[1].text == "hi", "a record without v and todo reads")
    expect(p[2].todo == true && p[2].stamp == nil && abs(p[2].ts.timeIntervalSince(ISO8601DateFormatter().date(from: "2026-10-07T14:33:00Z")!) - 0.25) < 0.001, "unknown fields are ignored")
    try appendRaw("{\"ack\":\"old\",\"x\":1}\n", to: jfile(d))
    expect(j.pending().map { $0.id } == ["id-1", "newer"], "an ack with extra keys still acks")
}
test("journal: an unwritable folder throws a typed error; nothing is pending; it works again when the folder does") {
    let fm = FileManager.default
    // the folder's parent is a file
    let blockerDir = jdir("blocker"); try fm.createDirectory(at: blockerDir, withIntermediateDirectories: true)
    let blocker = blockerDir.appendingPathComponent("file"); try Data("x".utf8).write(to: blocker)
    let j1 = CaptureJournal(dir: blocker.appendingPathComponent("capture"))
    do { try j1.append(jitem(1)); expect(false, "append must throw") } catch { expect(error is JournalError, "\(error)") }
    do { try j1.ack("id-1"); expect(false, "ack must throw") } catch { expect(error is JournalError, "\(error)") }
    expect(j1.pending().isEmpty)
    // a folder without write permission
    let d = jdir("readonly"); let j2 = CaptureJournal(dir: d)
    try j2.append(jitem(1))
    try fm.setAttributes([.posixPermissions: 0o500], ofItemAtPath: d.path)
    try fm.setAttributes([.posixPermissions: 0o400], ofItemAtPath: jfile(d).path)
    do { try j2.append(jitem(2)); expect(false, "append must throw") } catch { expect(error is JournalError, "\(error)") }
    expect(j2.pending() == [jitem(1)], "what is already there is still readable")
    try fm.setAttributes([.posixPermissions: 0o700], ofItemAtPath: d.path)
    try fm.setAttributes([.posixPermissions: 0o600], ofItemAtPath: jfile(d).path)
    try j2.append(jitem(2))
    expect(j2.pending() == [jitem(1), jitem(2)], "writable again")
}
test("journal: a file that existed with wider permissions is tightened to 0600") {
    let d = jdir("mode"); try FileManager.default.createDirectory(at: d, withIntermediateDirectories: true)
    FileManager.default.createFile(atPath: jfile(d).path, contents: nil, attributes: [.posixPermissions: 0o644])
    try CaptureJournal(dir: d).append(jitem(1))
    expect(fmode(jfile(d)) == 0o600, String(fmode(jfile(d)), radix: 8))
}
test("journal: compact drops acknowledged notes (2,000 items), keeps order and mode, leaves no temp file, and is a no-op when clean") {
    let d = jdir("compact"); let j = CaptureJournal(dir: d)
    var body = "", keep = [CaptureItem]()
    for n in 0..<2000 {
        let it = jitem(n); body += CaptureJournal.line(for: it) + "\n"
        if n % 200 == 7 { keep.append(it) } else { body += CaptureJournal.ackLine(it.id) + "\n" }
    }
    body += "garbage line\n{\"v\":1,\"id\":\"torn"                                   // and a torn last line
    try FileManager.default.createDirectory(at: d, withIntermediateDirectories: true)
    try body.write(to: jfile(d), atomically: true, encoding: .utf8)
    try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: jfile(d).path)
    let t0 = Date(); let before = j.pending(); let readTime = Date().timeIntervalSince(t0)
    expect(before == keep && keep.count == 10, "10 of 2,000 are still pending")
    expect(readTime < 5.0, "reading a 4,000-line journal: \(Int(readTime * 1000)) ms")
    try j.compact()
    expect(j.pending() == keep, "the same notes in the same order")
    expect(jlines(jfile(d)).count == 10 && !(jlines(jfile(d)).joined().contains("garbage")), "10 lines: acked notes, acks, garbage and the torn line are gone")
    expect(fmode(jfile(d)) == 0o600)
    try expect(!(try FileManager.default.contentsOfDirectory(atPath: d.path)).contains { $0.hasPrefix(".gloamlog-tmp-") }, "no temp file")
    let bytes = try Data(contentsOf: jfile(d))
    try j.compact()
    try expect(try Data(contentsOf: jfile(d)) == bytes, "compacting a clean journal changes nothing")
    try j.append(jitem(5000))
    expect(j.pending() == keep + [jitem(5000)], "appends go on after a compaction")
    for it in keep + [jitem(5000)] { try j.ack(it.id) }
    try j.compact()
    expect(j.pending().isEmpty && ((try? Data(contentsOf: jfile(d)))?.isEmpty ?? false) && fmode(jfile(d)) == 0o600, "everything acknowledged: an empty file")
    try j.append(jitem(1))
    expect(j.pending() == [jitem(1)])
    try CaptureJournal(dir: jdir("nothing")).compact()                              // no file at all: not an error
}
test("journal: dropping the writer without any ack loses nothing (in-process kill)") {
    let d = jdir("drop")
    var j: CaptureJournal? = CaptureJournal(dir: d)
    let items = (1...5).map { jitem($0) }
    for it in items { try j?.append(it) }
    try j?.ack(items[1].id); try j?.ack(items[3].id)
    j = nil                                                                          // the process dies here: no flush, no compact, no goodbye
    expect(CaptureJournal(dir: d).pending() == [items[0], items[2], items[4]], "pending equals exactly the unacknowledged notes")
}

test("Sources: all-projects mode reads every project except the skipped ones") {
    var p = SourcesPrefs()
    expect(p.policy.isEmpty && p.policy.root(for: "/work/a") == nil, "nothing is read until a project is ticked")
    p.allProjects = true
    expect(!p.policy.isEmpty && p.policy.root(for: "/work/a") == "/work/a", "every project is read")
    expect(p.policy.root(for: "/work/a/.claude/worktrees/x") == "/work/a", "worktrees fold into their project")
    p.setAllowed("/work/b", false)
    expect(p.policy.root(for: "/work/b") == nil && p.policy.root(for: "/work/a") != nil, "a skipped project is not read")
    p.setAllowed("/work/b", true)
    expect(p.policy.root(for: "/work/b") != nil && p.allowed.isEmpty, "switching it back on removes the skip")
    expect(ClaudeText.promptLine("Below is a conversation log from a Claude Code coding session.") == nil, "a summariser prompt is not work")
}

try? FileManager.default.removeItem(at: root)
print("\(passed) passed, \(failed) failed")
exit(failed == 0 ? 0 : 1)
