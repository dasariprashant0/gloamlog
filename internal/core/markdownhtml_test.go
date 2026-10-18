package core

// MarkdownHTML had no Swift unit tests (PARITY 5.3). These pin the behaviour with literal expectations; swiftdiff_test.go checks the
// same code against the Swift output on thousands of inputs.

import (
	"strings"
	"testing"
)

func TestMarkdownHTMLHeadingsParagraphsAndEscaping(t *testing.T) {
	cases := []struct{ md, want string }{
		{"", ""},
		{"# Title\n\nline one\nline two\n\n## Sub", "<h1>Title</h1>\n<p>line one<br>\nline two</p>\n<h2>Sub</h2>\n"},
		{"###### six", "<h6>six</h6>\n"},
		{"raw <b>html</b> & \"q\" 'a'", "<p>raw &lt;b&gt;html&lt;/b&gt; &amp; &quot;q&quot; &#39;a&#39;</p>\n"},
		{"<script>alert(1)</script>", "<p>&lt;script&gt;alert(1)&lt;/script&gt;</p>\n"},
		{"# a <img src=x onerror=y>", "<h1>a &lt;img src=x onerror=y&gt;</h1>\n"},
		{"---", "<hr>\n"},
		{"> one\n> two\n\nafter", "<blockquote><p>one<br>\ntwo</p></blockquote>\n<p>after</p>\n"},
		{"<!-- hidden -->\nshown", "<p>shown</p>\n"},
	}
	for _, c := range cases {
		expect(t, MDBody(c.md, nil) == c.want, "%q:\n got %q\nwant %q", c.md, MDBody(c.md, nil), c.want)
	}
}

func TestMarkdownHTMLListsNestingAndTasks(t *testing.T) {
	cases := []struct{ md, want string }{
		{"- a\n- b", "<ul><li>a</li><li>b</li></ul>"},
		{"1. one\n2. two", "<ol><li>one</li><li>two</li></ol>"},
		{"- a\n  - b\n    - c\n- d", "<ul><li>a<ul><li>b<ul><li>c</li></ul></li></ul></li><li>d</li></ul>"},
		{"- a\n1. back to ordered", "<ul><li>a</li></ul><ol><li>back to ordered</li></ol>"},
		{"- [ ] open\n- [x] done", "<ul><li class=\"task\">\u2610 open</li><li class=\"task\">\u2611 done</li></ul>"},
		{"- item\n  wrapped\n  again\n- next", "<ul><li>item<br>wrapped<br>again</li><li>next</li></ul>"},
		{"- a\n\npara\n\n- b", "<ul><li>a</li></ul><p>para</p>\n<ul><li>b</li></ul>"},
		{"- a\n# heading closes the list", "<ul><li>a</li></ul><h1>heading closes the list</h1>\n"},
		{"- 14:32 a jot", "<ul><li>14:32 a jot</li></ul>"},
	}
	for _, c := range cases {
		expect(t, MDBody(c.md, nil) == c.want, "%q:\n got %q\nwant %q", c.md, MDBody(c.md, nil), c.want)
	}
	// nesting stops at 8 levels (mdMaxListLevel 7): ten levels of indentation never open more than eight lists
	var deep strings.Builder
	for i := 0; i < 10; i++ {
		deep.WriteString(strings.Repeat("  ", i) + "- l" + itoa(i) + "\n")
	}
	html := MDBody(deep.String(), nil)
	expect(t, strings.Count(html, "<ul>") == 8, "%d lists open", strings.Count(html, "<ul>"))
}

func TestMarkdownHTMLCodeAndInline(t *testing.T) {
	cases := []struct{ md, want string }{
		{"```\ncode <b>x</b>\n  indented\n```\n\nafter", "<pre><code>code &lt;b&gt;x&lt;/b&gt;\n  indented</code></pre>\n<p>after</p>\n"},
		{"~~~~\nunclosed", "<pre><code>unclosed</code></pre>\n"},
		{"`code` and ``a `tick` b``", "<p><code>code</code> and <code>a `tick` b</code></p>\n"},
		{"`unclosed", "<p>`unclosed</p>\n"},
		{"**bold** *it* ***both*** ~~gone~~", "<p><strong>bold</strong> <em>it</em> <strong><em>both</em></strong> <s>gone</s></p>\n"},
		{"snake_case_word 2*3*4", "<p>snake_case_word 2<em>3</em>4</p>\n"},
		{"**unclosed", "<p>**unclosed</p>\n"},
		{"* not emphasis *", "<ul><li>not emphasis *</li></ul>"}, // a "* " line is a list item in Swift too (testdata mdbody-050)
		{"escaped \\* \\_ \\` \\[ and \\q", "<p>escaped * _ ` [ and \\q</p>\n"},
		{"inside `**not bold**` code", "<p>inside <code>**not bold**</code> code</p>\n"},
	}
	for _, c := range cases {
		expect(t, MDBody(c.md, nil) == c.want, "%q:\n got %q\nwant %q", c.md, MDBody(c.md, nil), c.want)
	}
}

func TestMarkdownHTMLLinksKeepTheirTargetOnlyForHTTPHTTPSAndMailto(t *testing.T) {
	cases := []struct{ md, want string }{
		{"[a](https://example.com)", "<p><a href=\"https://example.com\">a</a></p>\n"},
		{"[a](HTTP://example.com)", "<p><a href=\"HTTP://example.com\">a</a></p>\n"},
		{"[m](mailto:a@b.co)", "<p><a href=\"mailto:a@b.co\">m</a></p>\n"},
		{"[bad](javascript:alert(1))", "<p>bad</p>\n"},
		{"[file](file:///etc/passwd)", "<p>file</p>\n"},
		{"[rel](notes/a.md)", "<p>rel</p>\n"},
		{"[data](data:text/html;base64,AAAA)", "<p>data</p>\n"},
		{"[t](https://example.com \"Title\")", "<p><a href=\"https://example.com\">t</a></p>\n"},
		{"[angle](<https://example.com/a b>)", "<p><a href=\"https://example.com/a b\">angle</a></p>\n"},
		{"[p](https://example.com/a_(b)) after", "<p><a href=\"https://example.com/a_(b)\">p</a> after</p>\n"},
		{"[**b**](https://x.example)", "<p><a href=\"https://x.example\"><strong>b</strong></a></p>\n"},
		{"[q](https://x.example/?a=1&b=\"2\")", "<p><a href=\"https://x.example/?a=1&amp;b=&quot;2&quot;\">q</a></p>\n"},
		{"[no target]", "<p>[no target]</p>\n"},
	}
	for _, c := range cases {
		expect(t, MDBody(c.md, nil) == c.want, "%q:\n got %q\nwant %q", c.md, MDBody(c.md, nil), c.want)
	}
}

func TestMarkdownHTMLImages(t *testing.T) {
	md := "![shot](assets/a.png) and ![](assets/b.jpg) and ![x](c.gif)"
	expect(t, MDBody(md, nil) == "<p><span class=\"img\">[Image: shot]</span> and <span class=\"img\">[Image]</span> and <span class=\"img\">[Image: x]</span></p>\n", "alt text without a resolver: %s", MDBody(md, nil))
	withData := func(p string) (string, bool) {
		if p == "assets/a.png" {
			return "data:image/png;base64,AAA", true
		}
		return "", false
	}
	got := MDBody(md, withData)
	expect(t, strings.HasPrefix(got, "<p><img src=\"data:image/png;base64,AAA\" alt=\"shot\"> and <span class=\"img\">[Image]</span>"), "a data URI only where the resolver has one: %s", got)
	evil := MDBody("![a\"><script>](assets/a.png)", func(string) (string, bool) { return "data:\"><script>", true })
	expect(t, !strings.Contains(evil, "<script>"), "alt and src are escaped: %s", evil)
	expect(t, equal(MDImageTargets("![a](x.png) ![b](y.png) ![c](x.png)\n```\n![code](z.png)\n```\n# ![h](h.png)\n[![i](in.png)](https://x.example)"),
		[]string{"x.png", "y.png", "h.png", "in.png"}), "every image once, in order, none from code: %v", MDImageTargets("![a](x.png)"))
}

func TestMarkdownHTMLTables(t *testing.T) {
	got := MDBody("| h1 | h2 |\n| --- | :-: |\n| a | b |\n| c \\| d | <e> |\n\nafter", nil)
	want := "<table><thead><tr><th>h1</th><th>h2</th></tr></thead><tbody><tr><td>a</td><td>b</td></tr><tr><td>c | d</td><td>&lt;e&gt;</td></tr></tbody></table>\n<p>after</p>\n"
	expect(t, got == want, "got %q", got)
	expect(t, MDBody("a | b\nnot a table", nil) == "<p>a | b<br>\nnot a table</p>\n", "a pipe alone is not a table")
}

func TestMarkdownHTMLLimits(t *testing.T) {
	long := strings.Repeat("*a* ", 1300) // over 5,000 characters: shown as written
	expect(t, MDBody(long, nil) == "<p>"+strings.TrimSpace(long)+"</p>\n", "a line over 5,000 characters is shown as written")
	nested := strings.Repeat("*a ", 20) + "x" + strings.Repeat("* ", 20)
	_ = MDBody(nested, nil) // must return, whatever the depth
	huge := strings.Repeat("word\n\n", 400_000)
	expect(t, len(MDNodes(huge)) == mdMaxInput/6+1 || len(MDNodes(huge)) == mdMaxInput/6, "input is cut at 2,000,000 characters: %d paragraphs", len(MDNodes(huge)))
}

func TestMarkdownHTMLPlain(t *testing.T) {
	md := "# Title\n\n## Section\n- a\n  - **b** and `c`\n1. one\n- [ ] open\n- [x] done\n\n[link](https://x.example) [same](https://same.example) [bad](javascript:x) ![alt](a.png)\n\n> quote\n\n---\n\n```\ncode\n```\n\n| a | b |\n|---|---|\n| 1 | **2** |\n"
	want := "Title\n\nSection:\n\n- a\n  - b and c\n1. one\n- [ ] open\n- [x] done\n\nlink (https://x.example) same (https://same.example) bad [Image: alt]\n\nquote\n\n----\n\ncode\n\na | b\n1 | 2\n"
	expect(t, MDPlain(md) == want, "got %q\nwant %q", MDPlain(md), want)
	expect(t, MDPlain("[https://x.example](https://x.example)") == "https://x.example\n", "a link whose text is its address is written once")
	expect(t, MDPlain("") == "\n", "empty is one newline")
}

func TestMarkdownHTMLDocumentAndFragment(t *testing.T) {
	doc := MDDocument("Week <of> 5 & 6", "# T\n\n- a", nil)
	expect(t, strings.HasPrefix(doc, "<!DOCTYPE html>\n<html lang=\"en\"><head><meta charset=\"utf-8\">\n<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; img-src data:; style-src 'unsafe-inline'\">\n<title>Week &lt;of&gt; 5 &amp; 6</title>\n<style>"), "head: %q", doc[:300])
	expect(t, strings.HasSuffix(doc, "</style></head>\n<body><h1>T</h1>\n<ul><li>a</li></ul></body></html>"), "tail")
	expect(t, !strings.Contains(strings.ToLower(doc), "<script"), "no script in a printable page")
	frag := MDFragment("# T")
	expect(t, frag == "<html><head><meta charset=\"utf-8\"></head><body style=\"font-family: -apple-system, Helvetica, Arial, sans-serif; font-size: 13px;\"><h1>T</h1>\n</body></html>", frag)
}

func TestMarkdownHTMLEscapeAndSafeURL(t *testing.T) {
	expect(t, MDEscape("<a href=\"x\">&'</a>") == "&lt;a href=&quot;x&quot;&gt;&amp;&#39;&lt;/a&gt;")
	for _, u := range []string{"https://a.b", " HTTPS://a.b ", "http://a", "mailto:x@y"} {
		_, ok := MDSafeURL(u)
		expect(t, ok, u)
	}
	for _, u := range []string{"javascript:x", "ftp://a", "//a.b", "a/b", "", "data:text/html,x", "vbscript:x", "httpx://a"} {
		_, ok := MDSafeURL(u)
		expect(t, !ok, u)
	}
	u, _ := MDSafeURL("  https://a.b \n")
	expect(t, u == "https://a.b", "trimmed")
}
