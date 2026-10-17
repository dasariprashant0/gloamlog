// claudemd.go - the short instruction Gloamlog adds to ~/.claude/CLAUDE.md so Claude Code knows when to call `log_work`.
// Pure text work. Port of app/Core/ClaudeMd.swift. The section is wrapped in markers so it can be replaced or removed without touching
// anything else in the file. A file that already tells Claude to use `log_work` (written by hand) counts as set up and is left alone.
package core

import (
	"regexp"
	"strings"
	"unicode"
)

const (
	ClaudeMdBegin = "<!-- gloamlog:begin -->"
	ClaudeMdEnd   = "<!-- gloamlog:end -->"

	ClaudeMdInstruction = "## Gloamlog work log\n" +
		"When a meaningful piece of work is finished in a session (a problem solved, a deliverable done, a decision made, or I say I'm done), call the gloamlog `log_work` tool once with 1-3 plain sentences: the problem I was solving, what was done, and what is still open. Not after every step. No secrets, customer data or code."

	// ClaudeMdBlock is the instruction between its markers.
	ClaudeMdBlock = ClaudeMdBegin + "\n" + ClaudeMdInstruction + "\n" + ClaudeMdEnd
)

var blankRunRE = regexp.MustCompile(`\n{3,}`)

// ClaudeMdIsPresent is true when the text already carries our block, or an older hand-written section that mentions `log_work`.
func ClaudeMdIsPresent(text string) bool {
	return strings.Contains(text, ClaudeMdBegin) || strings.Contains(text, "log_work")
}

// trimTrailingSpace is the Swift replacingOccurrences("\\s+$", ...): ICU's \s is [\t\n\f\r\p{Z}].
func trimTrailingSpace(s string) string {
	return strings.TrimRightFunc(s, func(r rune) bool {
		return r == '\t' || r == '\n' || r == '\f' || r == '\r' || unicode.Is(unicode.Z, r)
	})
}

// ClaudeMdApply is text with the block added at the end, or replaced when ours is already there. A hand-written section is left as it is.
func ClaudeMdApply(text string) string {
	if lo, hi, ok := claudeMdMarkedRange(text); ok {
		return text[:lo] + ClaudeMdBlock + text[hi:]
	}
	if strings.Contains(text, "log_work") {
		return text
	}
	trimmedText := trimTrailingSpace(text)
	if trimmedText != "" {
		trimmedText += "\n\n"
	}
	return trimmedText + ClaudeMdBlock + "\n"
}

// ClaudeMdRemove is text without our block (and the blank line before it). Anything else, including a hand-written section, stays.
func ClaudeMdRemove(text string) string {
	lo, hi, ok := claudeMdMarkedRange(text)
	if !ok {
		return text
	}
	out := text[:lo] + text[hi:]
	out = blankRunRE.ReplaceAllLiteralString(out, "\n\n")
	out = trimTrailingSpace(out)
	if out == "" {
		return ""
	}
	return out + "\n"
}

func claudeMdMarkedRange(text string) (lo, hi int, ok bool) {
	b := strings.Index(text, ClaudeMdBegin)
	if b < 0 {
		return 0, 0, false
	}
	after := b + len(ClaudeMdBegin)
	e := strings.Index(text[after:], ClaudeMdEnd)
	if e < 0 {
		return 0, 0, false
	}
	return b, after + e + len(ClaudeMdEnd), true
}
