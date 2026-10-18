package core

// Ported from tests/main.swift: "carry-over" (card, items, apply, lookback) and the store-backed carried-over tests.
// The tests of the words rule alone are in markdownbody_test.go.

import (
	"strings"
	"testing"
)

func loader(t *testing.T, s *LogStore) func(day string) *DayDocument {
	return func(day string) *DayDocument {
		doc, _ := s.Load(day)
		return doc
	}
}

func TestCarryOverCardItemsApplyIdempotent(t *testing.T) {
	store := newStore(t)
	fri := "## What I did\n" + lorem(25, "w") + "\n\n## To do next\n- [ ] Call Ana\n- [x] Sent invoice\nFix build\n1. Plan Q4\n\n## Pending / blocked\n- Legal review\n- [ ] Call Ana\n![](assets/x.png)"
	save(t, store, "2026-10-02", fri)
	must(t, store.Skip("2026-10-05", "Holiday"))
	s := defaults()
	card := CarryCardBefore("2026-10-06", ny, s, loader(t, store))
	if card == nil {
		t.Fatal("expected a card")
	}
	expect(t, card.SourceDay == "2026-10-02" && card.SourceLabel == "Fri 2 Oct", "skipped Monday is ignored")
	expect(t, equal(card.Items, []string{"Call Ana", "Fix build", "Plan Q4", "Legal review"}), "items %v", card.Items)
	expect(t, card.HeadingText() == "Carried over from Fri 2 Oct", "heading text")
	expect(t, CarryHeading(*card, "2026-10-06", ny) == "Friday", "within a week: the weekday")
	expect(t, CarryHeading(*card, "2026-10-03", ny) == "Yesterday", "yesterday")
	expect(t, CarryHeading(*card, "2026-10-12", ny) == "2 Oct 2026", "older: the date")
	block := "## Carried over from Fri 2 Oct\n\n- [ ] Call Ana\n- [ ] Fix build\n- [ ] Plan Q4\n- [ ] Legal review"
	expect(t, ApplyCarryOver(*card, "") == block, "block on an empty page")
	once := ApplyCarryOver(*card, "## What I did\nstarted work")
	expect(t, once == block+"\n\n## What I did\nstarted work", "inserted at the TOP")
	expect(t, ApplyCarryOver(*card, once) == once && IsCarryApplied(*card, once) && !IsCarryApplied(*card, "x"), "idempotent")
	h3 := strings.Replace(once, "## Carried", "### Carried", 1)
	expect(t, ApplyCarryOver(*card, h3) == h3, "heading level does not matter")
	loose := "- [x] Call Ana\n- [ ] Fix build\n  - [ ] Plan Q4\nlegal  review"
	expect(t, ApplyCarryOver(*card, loose) == loose, "items already present (any marker, case, spacing) are not added again")
	expect(t, ApplyCarryOver(*card, "Call Ana\nfoo") == "## Carried over from Fri 2 Oct\n\n- [ ] Fix build\n- [ ] Plan Q4\n- [ ] Legal review\n\nCall Ana\nfoo", "only the missing items")
	withTemplate := ApplyCarryOver(*card, ClassicTemplate)
	expect(t, strings.HasPrefix(withTemplate, block+"\n\n## What I did") && strings.HasSuffix(withTemplate, "## To do next"), "on the template")
	expect(t, HasContent(block) && W(block) == 0, "carried tasks are content but not words (was 8 before the carried-over exclusion)")
}

func TestCarryOverPreviousLoggedDayOnlyLookbackMinWordsHeadings(t *testing.T) {
	store := newStore(t)
	load := loader(t, store)
	s := defaults()
	save(t, store, "2026-10-02", "## What I did\n"+lorem(25, "w")+"\n\n## To do next\n- [ ] From Friday")
	save(t, store, "2026-10-05", "## To do next\n- [ ] Should not carry") // 3 words: partial
	must(t, store.Skip("2026-10-06", "Leave"))
	c := CarryCardBefore("2026-10-07", ny, s, load)
	expect(t, c != nil && equal(c.Items, []string{"From Friday"}), "partial and skipped days are walked past")
	expect(t, CarryCardBefore("2026-10-02", ny, s, load) == nil, "nothing before the first log")
	c = CarryCardBefore("2026-10-16", ny, s, load)
	expect(t, c != nil && c.SourceDay == "2026-10-02", "14 days back is still found")
	expect(t, CarryCardBefore("2026-10-17", ny, s, load) == nil, "15 days back is not")
	big := s
	big.MinWords = 40
	expect(t, CarryCardBefore("2026-10-07", ny, big, load) == nil, "minWords decides what 'logged' means")
	save(t, store, "2026-10-06", "## What I did\n"+lorem(25, "w")+"\n\n## Notes\n- [ ] not a carry heading")
	expect(t, CarryCardBefore("2026-10-07", ny, s, load) == nil, "the latest logged day has nothing to carry: stop, do not dig further back")
	save(t, store, "2026-10-06", "## What I did\n"+lorem(25, "w")+"\n\n## To do next\n- [x] done already\n\n## Pending / blocked\n")
	expect(t, CarryCardBefore("2026-10-07", ny, s, load) == nil, "ticked tasks are not carried")
	none := s
	none.CarryOverHeadings = []string{}
	save(t, store, "2026-10-06", "## To do next\n- [ ] x\n"+lorem(25, "w"))
	expect(t, CarryCardBefore("2026-10-07", ny, none, load) == nil, "no carry headings")
	for _, c := range []struct {
		body     string
		headings []string
		want     []string
		why      string
	}{
		{"## 🔜 Next\n- [ ] a\n### Sub\n- b\n## Other\n- c", []string{"next"}, []string{"a", "b"}, "emoji-insensitive, sub-headings included"},
		{"## 📌 TO DO NEXT\n- x\n## to-do next\n- y\n## To do next\n- x", []string{"To do next"}, []string{"x", "y"}, "all matching headings, de-duplicated"},
		{"## To do next\n![](assets/x.png)\n```\ncode\n```\n<!-- hidden -->\n- real\n\n   \n- [ ]   spaced   out  ", []string{"To do next"}, []string{"real", "spaced out"}, ""},
		{"- [ ] outside any heading\n## Other\n- c", []string{"To do next"}, nil, ""},
		{"x", []string{" ", "🙂"}, nil, ""},
		{"## To do next\nFix build and test\n\nCall [Ana](http://x.com) today", []string{"to do next"}, []string{"Fix build and test", "Call [Ana](http://x.com) today"}, ""},
	} {
		got := CarryItems(c.body, c.headings)
		expect(t, strs(got, c.want), "CarryItems(%q) = %q, want %q %s", c.body, got, c.want, c.why)
	}
	expect(t, InsertCarryOver("body", "Carried over from X", []string{"", "  ", "\n"}) == "body", "nothing to add")
	expect(t, InsertCarryOver("", "H", []string{"a\nb"}) == "## H\n\n- [ ] a b", "items are single lines")
}

func TestCarriedOverTasksNeverCountTheRealFlow(t *testing.T) {
	// the real flow: card from a logged day -> apply -> save
	store := newStore(t)
	day := "2026-10-06"
	save(t, store, "2026-10-02", "## What I did\n"+lorem(25, "w")+"\n\n## To do next\n- [ ] "+lorem(10, "t")+"\n- [ ] "+lorem(10, "u"))
	load := loader(t, store)
	card := CarryCardBefore(day, ny, defaults(), load)
	if card == nil {
		t.Fatal("expected a card")
	}
	total := 0
	for _, it := range card.Items {
		total += len(strings.Fields(it))
	}
	expect(t, len(card.Items) == 2 && total == 20, "20 carried words, as many as minWords")
	onTemplate := ApplyCarryOver(*card, ClassicTemplate)
	expect(t, W(onTemplate) == 0 && HasContent(onTemplate) && W(ApplyCarryOver(*card, "")) == 0, "no words")
	save(t, store, day, onTemplate)
	expect(t, load(day) != nil, "a page holding only carried tasks is kept, never treated as empty")
	expect(t, val(store.FileStates(20))[day] == "" && val(store.FileStates(1))[day] == "", "neither logged nor partial")
	expect(t, val(store.FileStates(20))["2026-10-02"] == StatusLogged, "Friday is logged")
	expect(t, val(store.FolderSummary()).Logs == 2, "it still has content")
	expectErr(t, store.Skip(day, ""), ErrHasContent, day) // skip rules are unchanged: the page has content
	// still unlogged for status, planner, streak and carry sources
	st := val(store.FileStates(20))
	now := mk(2026, 10, 6, 18, 0)
	status := ResolveStatus(day, st[day], now, ny, workdays, EffectiveSince("", st))
	expect(t, status == StatusMissed, "status %s", status)
	a, _ := ReminderDecide(now, ny, defaults(), status, PlannerState{Day: day}, true)
	expect(t, a == ActionBringToFront(true), "the reminder still fires")
	c2 := CarryCardBefore("2026-10-07", ny, defaults(), load)
	expect(t, c2 != nil && c2.SourceDay == "2026-10-02", "a day with only carried tasks is not a carry source")
	expect(t, ComputeStreak(st, now, ny, workdays, "") == StreakResult{Current: 0, Best: 1}, "Friday logged, Monday missed, Tuesday not yet")
	// the reverse trap: a carry-over click above existing headingless writing must not swallow it
	free := lorem(25, "w")
	var bl []string
	for i := 1; i <= 6; i++ {
		bl = append(bl, "- "+lorem(5, "b"+itoa(i)+"x"))
	}
	bullets := strings.Join(bl, "\n")
	for _, body := range []string{free, bullets, "## What I did\n" + lorem(30, "w"), "intro words here\n\n" + ClassicTemplate} {
		once := ApplyCarryOver(*card, body)
		same := DayDocument{Day: day, Body: once}.Status(20) == DayDocument{Day: day, Body: body}.Status(20)
		expect(t, once != body && W(once) == W(body) && same, "words unchanged by carrying: %d vs %d", W(body), W(once))
	}
}

func TestCarryOverApplyIsIdempotentAndNeverChangesTheWordCount(t *testing.T) {
	store := newStore(t)
	save(t, store, "2026-10-02", "## What I did\n"+lorem(25, "w")+"\n\n## To do next\n- [ ] alpha beta\n- [ ] gamma delta\n\n## Pending / blocked\n- epsilon zeta")
	card := CarryCardBefore("2026-10-06", ny, defaults(), loader(t, store))
	if card == nil {
		t.Fatal("expected a card")
	}
	expect(t, equal(card.Items, []string{"alpha beta", "gamma delta", "epsilon zeta"}), "items %v", card.Items)
	for _, body := range []string{"", ClassicTemplate, lorem(25, "w"), "## What I did\n" + lorem(30, "w"), "x y z\n\n" + ClassicTemplate} {
		once := ApplyCarryOver(*card, body)
		expect(t, ApplyCarryOver(*card, once) == once && IsCarryApplied(*card, once) && !IsCarryApplied(*card, body), "idempotent")
		expect(t, W(once) == W(body) && HasContent(once) && strings.HasPrefix(once, "## Carried over from Fri 2 Oct\n\n- [ ] alpha beta"), "word count and shape")
		// ticking or editing the carried items afterwards changes nothing about the count or the idempotence
		edited := strings.Replace(once, "- [ ] alpha beta", "- [x] alpha beta and a long rewrite of it", 1)
		expect(t, W(edited) == W(body) && ApplyCarryOver(*card, edited) == edited, "edited carried item")
	}
}
