// bothversions.go - "Keep both": two versions of one day's page become one without losing a line. Pure and deterministic.
// Port of app/Core/Sync/BothVersions.swift.
//
// The base text (this Mac's, or the open page's) is kept exactly; the lines that exist only in the other copy go under a
// "From another copy" heading at the end. A line is compared by its trimmed text against the trimmed lines of the base; the extra
// lines keep their order and their own indentation, and a blank line stays between two extra lines only where the other copy had
// one. When the other copy has nothing new (the same text, a subset, or empty) the base comes back unchanged.
//
// The rule is deliberately dumb: a sentence edited on both Macs shows up once from each side and a ticked task sits next to its
// unticked twin. Nothing is ever lost, and every version also stays in the safety copies. Jots from two Macs are separate stamped
// lines, so they simply add up.
package core

import "strings"

// BothHeading is the heading "Keep both" puts above the other copy's extra lines.
const BothHeading = "From another copy"

func bothLines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

func bothKeys(lines []string) map[string]bool {
	keys := make(map[string]bool, len(lines))
	for _, l := range lines {
		if t := trimmed(l); t != "" {
			keys[t] = true
		}
	}
	return keys
}

// BothExtraLines are the lines of theirs that mine does not have, as they will be appended (blank lines only between two of them).
func BothExtraLines(mine, theirs string) []string {
	have := bothKeys(bothLines(mine))
	var out []string
	previousWasExtra, blanks := false, 0
	for _, line := range bothLines(theirs) {
		t := trimmed(line)
		if t == "" {
			if previousWasExtra {
				blanks++
			}
			continue
		}
		if have[t] {
			previousWasExtra, blanks = false, 0
			continue
		}
		if previousWasExtra && blanks > 0 {
			out = append(out, "")
		}
		out = append(out, line)
		previousWasExtra, blanks = true, 0
	}
	return out
}

// bothTrailing is what Swift's Character.isNewline plus space and tab cover: the end of the base that is dropped before the heading.
const bothTrailing = "\n\v\f\r\u0085   \t"

// MergeBoth is the text of the page after "Keep both": mine, then the other copy's extra lines under BothHeading (mine unchanged
// when there are none).
func MergeBoth(mine, theirs string) string {
	extra := BothExtraLines(mine, theirs)
	if len(extra) == 0 {
		return mine
	}
	base := strings.TrimRight(mine, bothTrailing)
	if base == "" { // nothing of ours to keep apart from
		return strings.Join(extra, "\n")
	}
	return base + "\n\n## " + BothHeading + "\n\n" + strings.Join(extra, "\n")
}

// MarkedLine is one line of a version for the conflict dialog: whether the other version lacks it.
type MarkedLine struct {
	Text    string `json:"text"`
	Differs bool   `json:"differs"`
}

// MarkLines is every line of text, and whether other lacks it (blank lines never differ).
func MarkLines(text, other string) []MarkedLine {
	have := bothKeys(bothLines(other))
	lines := bothLines(text)
	out := make([]MarkedLine, len(lines))
	for i, line := range lines {
		k := trimmed(line)
		out[i] = MarkedLine{Text: line, Differs: k != "" && !have[k]}
	}
	return out
}
