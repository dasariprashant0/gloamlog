// captureplacement.go - where and how a note appears in a page. Pure. Port of app/Core/Capture/CapturePlacement.swift.
//
//	RenderJot   "- 14:32 text", or "- [ ] text" for a to-do; continuation lines indented 2 spaces; a leading # > - + * or "1." in the
//	            text is backslash-escaped so it can never become a heading or nested list.
//	ApplyJot    puts the rendered line at the END of the page's Jots block (a heading whose normalised text equals jotsHeading, any
//	            level, running to the next heading of the same or higher level), joined to the list without a blank line when the last
//	            content line is a list item, else after one blank line; with no Jots block: the trimmed page + blank line + "## Jots" +
//	            blank line + the line.
//	JotPresent  true when the rendered line (ignoring list-marker style and escape differences) is already in the Jots block.
//
// Details:
//   - render: every line of the note is trimmed, blank lines inside a note are dropped, then the START of each line is escaped when it
//     would rewrite the page: an ATX heading (#, up to six, then a space or the end), ">", a bullet (- + * then a space or the end),
//     an ordered item ("1." / "1)" gets the backslash before the dot), a setext underline or thematic break (---, ===, ***, - - -) and
//     a code fence (``` or ~~~). "<!--" anywhere becomes "<\!--": an open comment would hide the rest of the page from the scanners.
//     Text that only looks like a marker (#tag, -5, *bold*, 1.5x) is left alone. A to-do never carries a time stamp.
//   - apply: the first Jots block is the target (the same rule as the editor's appendToSection). The result is TrimBody'd (no trailing
//     newline). Lines after the insertion keep their spacing, except that a line directly after the new note (a heading, or text
//     hidden by an open comment) gets one blank line, so it can never become a continuation of the note.
//   - contains: the note's identity is jotKey: list marker, task box, backslash escapes and line breaks (soft break, hard break "\")
//     are flattened and whitespace collapsed, so the editor re-serialising the line ("* " for "- ", "\*" for "*", a ticked box) never
//     defeats it. It looks in every Jots block, at top-level list items only (a multi-line note is one item), never in code.
package core

import "strings"

// RenderJot is the markdown a note is written as (see the header).
func RenderJot(item CaptureItem) string {
	text := strings.ReplaceAll(strings.ReplaceAll(item.Text, "\r\n", "\n"), "\r", "\n")
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = trimmed(l); l != "" {
			lines = append(lines, escapeJotLine(l))
		}
	}
	head := "- "
	if item.Todo {
		head += "[ ] "
	} else if item.Stamp != "" {
		head += item.Stamp + " "
	}
	first := ""
	if len(lines) > 0 {
		first = lines[0]
	}
	firstLine := head + first
	if first == "" {
		firstLine = head[:len(head)-1] // "- " without its trailing space
	}
	out := []string{firstLine}
	for _, l := range lines[min(1, len(lines)):] {
		out = append(out, "  "+l)
	}
	return strings.Join(out, "\n")
}

// allBytes is Swift's allSatisfy: true for an empty string.
func allBytes(s string, ch byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ch {
			return false
		}
	}
	return true
}

// escapeJotLine backslash-escapes what would turn this line (the first line of the note or a continuation line) into page structure.
func escapeJotLine(line string) string {
	s := strings.ReplaceAll(line, "<!--", "<\\!--")
	if s == "" {
		return s
	}
	bare := strings.NewReplacer(" ", "", "\t", "").Replace(s)
	if allBytes(bare, '=') || allBytes(bare, '-') { // setext underline, "-", "---"
		return "\\" + s
	}
	if utf8Len(bare) >= 3 && strings.IndexByte("*_-", bare[0]) >= 0 && allBytes(bare, bare[0]) { // "***", "- - -"
		return "\\" + s
	}
	if strings.HasPrefix(s, "```") || strings.HasPrefix(s, "~~~") {
		return "\\" + s
	}
	marker := func(after string) bool { return after == "" || after[0] == ' ' || after[0] == '\t' }
	f := s[0]
	switch {
	case f == '#':
		n := 0
		for n < len(s) && s[n] == '#' {
			n++
		}
		if n <= 6 && marker(s[n:]) {
			return "\\" + s
		}
	case f == '>':
		return "\\" + s
	case strings.IndexByte("-+*", f) >= 0 && marker(s[1:]):
		return "\\" + s
	default:
		n := 0
		for n < len(s) && s[n] >= '0' && s[n] <= '9' {
			n++
		}
		if n > 0 && n <= 9 && n < len(s) && (s[n] == '.' || s[n] == ')') && marker(s[n+1:]) {
			return s[:n] + "\\" + s[n:]
		}
	}
	return s
}

// ApplyJot puts the note into body (see the header). jotsHeading is the configured Jots heading.
func ApplyJot(item CaptureItem, body, jotsHeading string) string {
	if out, ok := applyTimed(item, body); ok {
		return out
	}
	line := RenderJot(item)
	text := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	scanned := ScanBody(text) // splits exactly like lines, so the indexes match
	// A page that ends inside an open code fence or HTML comment would swallow the note (it would be saved but never seen as a note,
	// so a replay would add it again): in that case the fence / comment is closed first.
	attempts := [][]string{nil}
	fence, hasFence := unclosedFenceCloser(scanned)
	if hasFence {
		attempts = append(attempts, []string{fence})
	}
	attempts = append(attempts, []string{"-->"})
	if hasFence {
		attempts = append(attempts, []string{fence, "-->"})
	}
	first, haveFirst := "", false
	for _, closers := range attempts {
		result := placeJot(line, closers, text, lines, scanned, jotsHeading)
		if !haveFirst {
			first, haveFirst = result, true
		}
		if JotPresent(item, result, jotsHeading) {
			return result
		}
	}
	return first
}

// applyTimed puts a timed note (a commit) among its list's lines by its time (the list named item.List; a one-list page's Log).
// ok is false on a classic page, or when the line would not show as a note there (the page ends inside an open code fence):
// ApplyJot then places it as usual.
func applyTimed(item CaptureItem, body string) (string, bool) {
	if !item.Timed {
		return "", false
	}
	p, ok := ParseLog(body)
	if !ok {
		return "", false
	}
	l, ok := parseLogRows(splitLines(RenderJot(item)))
	if !ok {
		return "", false
	}
	list := item.List
	if list == "" {
		list = ListWork
	}
	if project := CommitsLineProject(l.Text); project != "" {
		p.Lines = WithoutCommitsLines(p.Lines, project) // a project's newer commit count replaces its line (commitsline.go)
	}
	target := p.Target(list)
	l.Section = target
	var mine, rest []LogLine
	for _, x := range p.Lines {
		if x.Section == target {
			mine = append(mine, x)
		} else {
			rest = append(rest, x)
		}
	}
	p.Lines = append(rest, InsertTimed(mine, l)...) // Markdown writes each list's lines in their own order
	out := TrimBody(p.Markdown())
	return out, JotPresent(item, out, p.Sections[target].Name)
}

func placeJot(line string, closers []string, text string, lines []string, scanned []ScannedLine, jotsHeading string) string {
	blocks := jotsBlocks(scanned, jotsHeading)
	if len(blocks) == 0 {
		page := TrimBody(text)
		var parts []string
		for _, p := range append([]string{page}, closers...) {
			if p != "" {
				parts = append(parts, p)
			}
		}
		head := strings.Join(parts, "\n")
		if head != "" {
			head += "\n\n"
		}
		return head + "## " + jotsHeadingText(jotsHeading) + "\n\n" + line
	}
	block := blocks[0]
	last := -1
	for i := block.end - 1; i > block.heading; i-- {
		if scanned[i].Kind != LineBlank {
			last = i
			break
		}
	}
	var insertAt int
	var blankFirst bool
	switch {
	case last >= 0:
		insertAt = last + 1
		blankFirst = len(closers) > 0 || !jotEndsInList(scanned, last, block.heading)
	case block.heading+1 < block.end: // an empty block: the blank line after the heading is the separator
		insertAt, blankFirst = block.heading+2, false
	default:
		insertAt, blankFirst = block.heading+1, true
	}
	out := append([]string{}, lines[:insertAt]...)
	out = append(out, closers...)
	if blankFirst {
		out = append(out, "")
	}
	out = append(out, strings.Split(line, "\n")...)
	if insertAt < len(lines) && trimmed(lines[insertAt]) != "" { // never glue the next heading or line onto the note
		out = append(out, "")
	}
	out = append(out, lines[insertAt:]...)
	return TrimBody(strings.Join(out, "\n"))
}

// jotEndsInList is true when the content line last belongs to a list: some line of the run of text lines ending there is a list item
// (the lines after an item without a blank line are its continuation). Code, headings and blank lines end the run.
func jotEndsInList(s []ScannedLine, last, floor int) bool {
	for i := last; i > floor && s[i].Kind == LineText; i-- {
		if _, ok := listItemIndent(s[i].Text); ok {
			return true
		}
	}
	return false
}

// JotPresent is true when the note's line is already in a Jots block of body (replay safety).
func JotPresent(item CaptureItem, body, jotsHeading string) bool {
	_, ok := JotKeys(body, jotsHeading)[jotKey(RenderJot(item))]
	return ok
}

// JotKeys are the keys of every top-level list item in every Jots block of body (one scan; the writer checks many notes against one page).
func JotKeys(body, jotsHeading string) map[string]bool {
	scanned := ScanBody(body)
	keys := map[string]bool{}
	for _, b := range jotsBlocks(scanned, jotsHeading) {
		for _, k := range jotItemKeys(scanned, b) {
			keys[k] = true
		}
	}
	return keys
}

// LastJotKey is the key of the last top-level list item of the FIRST Jots block: where the page editor puts the note it just
// appended, in the editor's own spelling. The editor does not write every note the way RenderJot did (a bare link becomes
// <https://...>, a www address a link, a doubled code span a single one, an entity its character), so the writer keeps this key next
// to the note and accepts it as proof that the page holds the note. ok is false when there is no Jots block or no item in it.
func LastJotKey(body, jotsHeading string) (key string, ok bool) {
	scanned := ScanBody(body)
	blocks := jotsBlocks(scanned, jotsHeading)
	if len(blocks) == 0 {
		return "", false
	}
	keys := jotItemKeys(scanned, blocks[0])
	if len(keys) == 0 {
		return "", false
	}
	return keys[len(keys)-1], true
}

func jotItemKeys(scanned []ScannedLine, b jotsBlock) []string {
	var keys []string
	var item []string
	inItem := false
	flush := func() {
		if inItem {
			keys = append(keys, jotKey(strings.Join(item, "\n")))
		}
		item, inItem = nil, false
	}
	for i := b.heading + 1; i < b.end; i++ {
		l := scanned[i]
		if l.Kind != LineText {
			flush()
			continue
		}
		if indent, ok := listItemIndent(l.Text); ok && indent < 2 {
			flush()
			item, inItem = []string{l.Text}, true
		} else if inItem {
			item = append(item, l.Text)
		}
	}
	flush()
	return keys
}

// jotKey is a note's identity: first line without its list marker and task box, every line without backslash escapes, line breaks
// turned into spaces ("\" at the end of a line is a hard break), whitespace collapsed. Case and time are part of it.
func jotKey(markdown string) string {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	lines[0], _ = StripMarkers(lines[0])
	for i := 0; i < len(lines)-1; i++ {
		trailing := len(lines[i]) - len(strings.TrimRight(lines[i], "\\"))
		if trailing%2 == 1 {
			lines[i] = lines[i][:len(lines[i])-1]
		}
	}
	return Collapse(unescapeJot(strings.Join(lines, " ")))
}

const jotEscapable = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

func unescapeJot(s string) string {
	rs := []rune(s)
	var out strings.Builder
	for i := 0; i < len(rs); i++ {
		if rs[i] == '\\' && i+1 < len(rs) && rs[i+1] < 128 && strings.ContainsRune(jotEscapable, rs[i+1]) {
			i++ // drop the backslash, keep the escaped character as it is
			out.WriteRune(rs[i])
			continue
		}
		out.WriteRune(rs[i])
	}
	return out.String()
}
