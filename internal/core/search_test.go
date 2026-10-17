package core

// Ported from tests/main.swift: "search" and the "search index" tests (cache equals uncached, invalidation, warm-up).

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func hitDays(h []SearchHit) []string {
	var out []string
	for _, x := range h {
		out = append(out, x.Day)
	}
	return out
}

func hitHeadings(h []SearchHit) []string {
	var out []string
	for _, x := range h {
		if x.Heading == nil {
			out = append(out, "<nil>")
		} else {
			out = append(out, *x.Heading)
		}
	}
	return out
}

func str(s string) *string { return &s }

func TestSearchHeadingsSnippetsMatchingRules(t *testing.T) {
	store := newStore(t)
	save(t, store, "2026-10-05", "## 📝 What I did\nDrafted the Pricing page copy at the Café with Ana\n- [ ] review PRICING page with Ana\n\n## Finished\nPricing only")
	save(t, store, "2026-10-06", "no headings here: notes on pricing and more")
	must(t, store.Skip("2026-10-07", "pricing holiday"))
	hits := val(store.Search("pricing", nil))
	expect(t, equal(hitDays(hits), []string{"2026-10-06", "2026-10-05", "2026-10-05"}), "newest day first, one hit per section: %v", hitDays(hits))
	expect(t, equal(hitHeadings(hits), []string{"<nil>", "📝 What I did", "Finished"}), "%v", hitHeadings(hits))
	h := hits[1]
	expect(t, h.MatchText() == "Pricing", "matchRange points at the match in its original case: %q", h.MatchText())
	expect(t, !strings.Contains(h.Snippet, "- [ ]") && !strings.Contains(h.Snippet, "##") && !strings.Contains(h.Snippet, "\n") && strings.Contains(h.Snippet, "review PRICING page"), h.Snippet)
	expect(t, equal(hitHeadings(val(store.Search("CAFE ana", nil))), []string{"📝 What I did"}), "case/diacritic-insensitive, words AND-ed")
	expect(t, len(val(store.Search("pricing zebra", nil))) == 0 && len(val(store.Search("   ", nil))) == 0 && len(val(store.Search("", nil))) == 0, "no match / empty query")
	expect(t, len(val(store.Search("drafted finished", nil))) == 0, "words must co-occur inside one section (the heading text counts)")
	expect(t, len(val(store.Search("pricing", str("to-do next")))) == 0 && len(val(store.Search("pricing", str("what I did")))) == 1, "heading filter")
	expect(t, equal(hitDays(val(store.Search("pricing", str("Finished")))), []string{"2026-10-05"}), "heading filter 2")
	expect(t, len(val(store.Search("holiday", nil))) == 0, "skipped days are not searched")
	save(t, store, "2026-10-08", "## Pricing review\n\n## Notes\nnothing\n\n## Finished\n### Details\nneedle in a sub section\n\nintro without heading goes first")
	hits = val(store.Search("pricing review", nil))
	expect(t, equal(hitDays(hits), []string{"2026-10-08", "2026-10-05"}) && *hits[0].Heading == "Pricing review" && hits[0].Snippet == "Pricing review", "heading-only match: %+v", hits)
	expect(t, equal(hitHeadings(val(store.Search("needle", nil))), []string{"Details"}), "nearest preceding heading")
	save(t, store, "2026-10-09", "## Pricing\nreview notes")
	expect(t, equal(hitDays(val(store.Search("pricing review", nil))), []string{"2026-10-09", "2026-10-08", "2026-10-05"}), "words split between heading and text still match")
	save(t, store, "2026-10-10", "```\nlet needle = 1\n```\n\n![needle alt](assets/x.png)\n<!-- needle -->")
	expect(t, equal(hitDays(val(store.Search("needle", nil))), []string{"2026-10-10", "2026-10-08"}), "code is searchable; image alt text and comments are not")
	save(t, store, "2026-10-11", lorem(60, "w")+" needle "+lorem(60, "t"))
	var lh SearchHit
	for _, x := range val(store.Search("needle", nil)) {
		if x.Day == "2026-10-11" {
			lh = x
		}
	}
	expect(t, lh.Heading == nil && strings.HasPrefix(lh.Snippet, "…") && strings.HasSuffix(lh.Snippet, "…") && len([]rune(lh.Snippet)) < 140 && lh.MatchText() == "needle", "long page snippet")
	// cutting a snippet next to combining marks / emoji sequences must neither crash nor shift the match
	tricky := strings.Repeat("é̈ 👨‍👩‍👧 ", 30) + "needle" + strings.Repeat(" ́a👩🏽‍💻", 40)
	save(t, store, "2026-10-13", tricky)
	for _, x := range val(store.Search("needle", nil)) {
		if x.Day == "2026-10-13" {
			expect(t, x.MatchText() == "needle" && strings.HasPrefix(x.Snippet, "…") && strings.HasSuffix(x.Snippet, "…"), "tricky snippet: %q", x.Snippet)
		}
	}
	// an old v0.2 file is searchable too
	writeRaw(t, store.Dir, "2026-10-12", "# 2026-10-12\n\n## 🚀 Started\nKicked off the zebra project\n\n")
	expect(t, equal(hitHeadings(val(store.Search("zebra", nil))), []string{"🚀 Started"}), "old file")
	expect(t, len(SearchDocument(DayDocument{Day: "2026-10-12", IsSkipped: true, SkipReason: "zebra"}, "zebra", nil)) == 0, "skipped document")
}

func TestSearchFoldsCaseAndDiacriticsLikeFoundation(t *testing.T) {
	doc := DayDocument{Day: "2026-10-05", Body: "Café STRASSE straße øl ǖber е́ Ёж"}
	for _, q := range []string{"cafe", "CAFÉ", "strasse", "straße", "uber", "e", "еж", "Ёж"} {
		expect(t, len(SearchDocument(doc, q, nil)) == 1, "%q should match", q)
	}
	for _, q := range []string{"ol", "oel"} {
		expect(t, len(SearchDocument(doc, q, nil)) == 0, "%q must not match (ø does not decompose)", q)
	}
	h := SearchDocument(doc, "cafe", nil)[0]
	expect(t, h.MatchText() == "Café", "match keeps the original text: %q", h.MatchText())
	a, b := h.MatchUTF16()
	expect(t, a == 0 && b == 4, "UTF-16 offsets %d %d", a, b)
}

// ---- search index ----

func uncachedSearch(t *testing.T, store *LogStore, q string, heading *string) []SearchHit {
	t.Helper()
	var hits []SearchHit
	for _, d := range val(store.ListDays()) {
		doc, err := store.Load(d)
		if errors.Is(err, ErrNotLocal) {
			continue
		}
		must(t, err)
		if doc != nil {
			hits = append(hits, SearchDocument(*doc, q, heading)...)
		}
	}
	return hits
}

func TestSearchIndexCachedEqualsUncachedAndAFileIsParsedOnce(t *testing.T) {
	store := newStore(t)
	save(t, store, "2026-10-05", "## 📝 What I did\nDrafted the Pricing page copy at the Café with Ana\n- [ ] review PRICING page with Ana\n\n## Finished\nPricing only")
	save(t, store, "2026-10-06", "no headings here: notes on pricing and more")
	must(t, store.Skip("2026-10-07", "pricing holiday"))
	save(t, store, "2026-10-08", "## Pricing review\n\n## Notes\nnothing\n\n## Finished\n### Details\nneedle in a sub section\n\nintro without heading goes first")
	writeRaw(t, store.Dir, "2026-10-09", "# 2026-10-09\n\n## 🚀 Started\nKicked off the zebra project\n\n")
	for _, c := range []struct {
		q string
		h *string
	}{{"pricing", nil}, {"CAFE ana", nil}, {"pricing review", nil}, {"needle", nil}, {"zebra", nil}, {"holiday", nil}, {"pricing zebra", nil}, {"", nil},
		{"pricing", str("finished")}, {"pricing", str("what I did")}, {"pricing", str("to-do next")}} {
		cached, plain := val(store.Search(c.q, c.h)), uncachedSearch(t, store, c.q, c.h)
		expect(t, len(cached) == len(plain) && (len(cached) == 0 || equal(cached, plain)), "%q: %v vs %v", c.q, cached, plain)
	}
	expect(t, equal(hitDays(val(store.Search("pricing", nil))), []string{"2026-10-08", "2026-10-06", "2026-10-05", "2026-10-05"}), "newest day first, then document order")
	expect(t, equal(val(store.Search("pricing", str("finished")))[0].Snippet, "Pricing only"), "heading snippet")
	expect(t, store.searchIndex.FileReads() == 5, "every file (the skip marker too) parsed once: %d", store.searchIndex.FileReads())
	val(store.Search("anything", nil))
	expect(t, store.searchIndex.FileReads() == 5, "a later search reads nothing")
}

func TestSearchIndexChangedFileIsReReadDeletedDropsOutPlaceholderNeverCached(t *testing.T) {
	store := newStore(t)
	reads := func() int { return store.searchIndex.FileReads() }
	days := func(q string) []string { return hitDays(val(store.Search(q, nil))) }
	save(t, store, "2026-10-05", "alpha one")
	save(t, store, "2026-10-06", "alpha two")
	expect(t, equal(days("alpha"), []string{"2026-10-06", "2026-10-05"}) && reads() == 2, "first search")
	save(t, store, "2026-10-05", "bravo one again") // the app's own save (atomic replace)
	expect(t, equal(days("alpha"), []string{"2026-10-06"}) && equal(days("bravo"), []string{"2026-10-05"}) && reads() == 3, "own save")
	// an outside write of the same size, its mtime pushed forward explicitly
	writeRaw(t, store.Dir, "2026-10-06", "# 2026-10-06\n\ndelta two\n")
	setMTime(t, store.Path("2026-10-06"), time.Now().Add(5*time.Second))
	expect(t, len(days("alpha")) == 0 && equal(days("delta"), []string{"2026-10-06"}) && reads() == 4, "outside write")
	// touched only (same bytes, newer mtime): read again, nothing visible changes
	setMTime(t, store.Path("2026-10-06"), time.Now().Add(10*time.Second))
	expect(t, equal(days("delta"), []string{"2026-10-06"}) && reads() == 5, "touch")
	// deleted outside the app
	must(t, os.Remove(store.Path("2026-10-06")))
	expect(t, len(days("delta")) == 0 && equal(days("bravo"), []string{"2026-10-05"}) && reads() == 5, "deleted")
	// an iCloud placeholder next to the page: the day is not here yet, so it is neither found nor cached as empty
	save(t, store, "2026-10-07", "bravo three")
	placeholder := filepath.Join(store.Dir, placeholderName("2026-10-07"))
	must(t, os.WriteFile(placeholder, nil, 0o644))
	expect(t, equal(days("bravo"), []string{"2026-10-05"}) && reads() == 5, "placeholder: skipped, not read")
	must(t, os.Remove(placeholder))
	expect(t, equal(days("bravo"), []string{"2026-10-07", "2026-10-05"}) && reads() == 6, "downloaded: found, read once")
	// a cached page that goes back to the cloud is left out again until it is here, then read afresh
	must(t, os.WriteFile(placeholder, nil, 0o644))
	expect(t, equal(days("bravo"), []string{"2026-10-05"}), "evicted again")
	must(t, os.Remove(placeholder))
	expect(t, equal(days("bravo"), []string{"2026-10-07", "2026-10-05"}) && reads() == 7, "back: re-read, not the stale entry (%d)", reads())
}

func TestSearchIndexWarmFillsTheCacheReplacedStoreStartsEmptyOverlapIsSafe(t *testing.T) {
	a := newStore(t)
	save(t, a, "2026-10-05", "echo in a")
	must(t, a.WarmSearch())
	expect(t, a.searchIndex.FileReads() == 1, "warm reads every page")
	expect(t, equal(hitDays(val(a.Search("echo", nil))), []string{"2026-10-05"}) && a.searchIndex.FileReads() == 1, "the first search after a warm reads nothing")
	b := newStore(t)
	save(t, b, "2026-10-06", "echo in b")
	expect(t, b.searchIndex.FileReads() == 0, "new store, empty index")
	expect(t, equal(hitDays(val(b.Search("echo", nil))), []string{"2026-10-06"}) && b.searchIndex.FileReads() == 1, "nothing carried over from the old folder")
	expect(t, equal(hitDays(val(a.Search("echo", nil))), []string{"2026-10-05"}) && a.searchIndex.FileReads() == 1, "the old store's index is untouched")
	// the launch warm-up and the first keystroke can overlap: both complete, every file is still read once
	c := newStore(t)
	for i := 1; i <= 30; i++ {
		save(t, c, "2026-09-"+pad2(i), "foxtrot "+itoa(i))
	}
	var wg sync.WaitGroup
	var hits []SearchHit
	wg.Add(2)
	go func() { defer wg.Done(); _ = c.WarmSearch() }()
	go func() { defer wg.Done(); hits, _ = c.Search("foxtrot", nil) }()
	wg.Wait()
	expect(t, len(hits) == 30 && c.searchIndex.FileReads() == 30, "%d hits, %d reads", len(hits), c.searchIndex.FileReads())
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}
