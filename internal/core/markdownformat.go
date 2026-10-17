// markdownformat.go - the day file <-> DayDocument. Pure functions, no disk. Port of app/Core/MarkdownFormat.swift.
//
// File:   "# YYYY-MM-DD\n\n<body>\n"   (the app writes the title; on load a leading "# <date>" line is stripped)
// Skip:   "# YYYY-MM-DD\n\n> Skipped: reason\n"   or "> Skipped" (no reason). Recognised only when it is the ONLY content,
//
//	and the text after "> Skipped" is empty or starts with ":" ("> Skipped the meeting" in a longer note is just text).
//
// Old files (v0.1/v0.2 "## 📝 What I did" ... headings) load unchanged: they are simply markdown. The old "\## " escape
// (a backslash before a line-start "## ") is undone on load, outside fenced code. Nothing is escaped on save.
// BOM and CRLF are tolerated on load.
//
// API: ParseDay(day, raw) DayDocument, SerializeDay(day, body) string, SerializeSkip(day, reason) string
package core

import "strings"

func isDateTitle(line string) bool {
	t := trimmed(line)
	return strings.HasPrefix(t, "# ") && DayKeyIsWellFormed(trimmed(t[2:]))
}

// SkipReasonInBody reads "> Skipped" / "> Skipped: reason" as the whole body. ok is false when the body is anything else;
// the reason is "" when none was given.
func SkipReasonInBody(body string) (reason string, ok bool) {
	t := trimmed(body)
	if !strings.HasPrefix(t, "> Skipped") || strings.Contains(t, "\n") {
		return "", false
	}
	rest := t[len("> Skipped"):]
	if rest == "" {
		return "", true
	}
	if !strings.HasPrefix(rest, ":") {
		return "", false
	}
	return trimmed(rest[1:]), true
}

// unescapeLegacy undoes the old "\## " escape (one backslash removed), outside fenced code.
func unescapeLegacy(text string) string {
	var out []string
	var open fence
	inFence := false
	for _, line := range strings.Split(text, "\n") {
		if inFence {
			if isClosingFence(line, open) {
				inFence = false
			}
			out = append(out, line)
			continue
		}
		if f, ok := openingFence(line); ok {
			open, inFence = f, true
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(line, "\\") && strings.HasPrefix(strings.TrimLeft(line, "\\"), "## ") {
			out = append(out, line[1:])
		} else {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

// ParseDay turns the raw text of a day file into a DayDocument.
func ParseDay(day, raw string) DayDocument {
	text := strings.TrimPrefix(raw, utf8BOM)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for len(lines) > 0 && trimmed(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && isDateTitle(lines[0]) {
		lines = lines[1:]
	}
	body := TrimBody(strings.Join(lines, "\n"))
	if reason, ok := SkipReasonInBody(body); ok {
		return DayDocument{Day: day, IsSkipped: true, SkipReason: reason}
	}
	return DayDocument{Day: day, Body: TrimBody(unescapeLegacy(body))}
}

// SerializeDay is the text of a day file: "# <day>\n\n<body>\n".
func SerializeDay(day, body string) string {
	return "# " + day + "\n\n" + TrimBody(body) + "\n"
}

// SerializeSkip is the text of a skip marker file; the reason is put on one line.
func SerializeSkip(day, reason string) string {
	r := trimmed(strings.ReplaceAll(reason, "\n", " "))
	if r != "" {
		r = ": " + r
	}
	return "# " + day + "\n\n> Skipped" + r + "\n"
}
