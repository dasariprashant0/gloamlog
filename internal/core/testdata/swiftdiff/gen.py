# gen.py - writes ../chunkb_cases.json, the inputs of the chunk B differential test (see ../../swiftdiff_test.go). Deterministic.
import json, os
HERE = os.path.dirname(os.path.abspath(__file__))
cases = []
def add(kind, **kw):
    kw['kind'] = kind
    kw['id'] = '%s-%03d' % (kind, sum(1 for c in cases if c['kind']==kind))
    cases.append(kw)

md = [
 "", "plain text", "# Title\n\npara one\nline two\n\n## Sub\ntext", "###### six\n####### seven",
 "- a\n- b\n  - c\n    - d\n- e", "1. one\n2. two\n   - nested\n3. three", "- [ ] open\n- [x] done\n  - [ ] nested task",
 "- item with\n  wrapped line\n  and more\n- next", "* star\n+ plus\n- dash", "- a\n\npara after\n\n- b",
 "> quote one\n> quote two\n\nafter", "> quote\npara right after", "para\n> quote right after\n", "---\n***\n___\n- - -",
 "```\ncode <b>x</b>\n  indented\n```\n\nafter", "~~~~\nunclosed fence", "``` swift\nlet a = 1\n```", "text `code` and ``double `tick` code`` end",
 "`unclosed code", "**bold** *italic* ***both*** ~~strike~~ _under_ __dunder__", "snake_case_word and 2*3*4 and a * b * c",
 "**bold *nested italic* bold**", "*a **b** c*", "~~a **b** c~~", "**unclosed bold", "* not emphasis *", "_ x _ and __ y __", "a_b_c", "_a_b",
 "[link](https://example.com) and [mail](mailto:a@b.co) and [bad](javascript:alert(1)) and [rel](notes/a.md) and [file](file:///etc/passwd)",
 "[with title](https://example.com \"Title here\") and [angle](<https://example.com/a b>)", "[parens](https://example.com/a_(b)) after", "[nested [brackets]](https://x.example)",
 "[**bold link**](https://x.example) and [`code link`](http://y.example)", "[empty]() and [](https://z.example) and [x]", "![alt](assets/a.png) and ![](assets/b.png) and ![alt2](c.jpg)",
 "![img [with] brackets](assets/a.png)", "[![inner](assets/a.png)](https://x.example)", "escapes \\* \\_ \\` \\[ \\] \\\\ \\# \\q end", "raw <b>html</b> & <script>alert(1)</script> \"quotes\" 'single'",
 "| h1 | h2 |\n| --- | :-: |\n| a | b |\n| c \\| d | e |\n\nafter table", "| a | b |\n|---|---|", "a | b\n--- | ---\n1 | 2", "pipe | not table\nnext line",
 "# Heading with **bold** and [link](https://x.example)", "## Heading\n- list right after\npara", "- a\n# heading closes list\n- b", "text\n\n    indented four\n\n- item\n\n  continued para",
 "<!-- comment -->\nvisible\n<!-- multi\nline -->\nafter", "- 14:32 a jot\n- [ ] 15:00 task jot", "café 日本語 \U0001F389 emoji **テスト**", "tab\tseparated\tcells and nbsp",
 "1) paren one\n2) paren two", "10. ten\n11. eleven\n- bullet\n1. back", "- a\n  - b\n - c\n- d", "- a\n    - deep\n  - mid\n      - deeper\n- z",
 "- " + "x"*300 + "\n", "a "+"*"*5+" b", "```\ncode\n```\n```\nmore\n```", "- [ ]\n- [x]\n-", "Hello\\\nworld hard break",
 "• not a list\n* real", "a  \nb", "[link](https://example.com)text [a](b)(c)", "https://bare.example.com and <https://angle.example.com>",
]
long = "word " * 1100  # > 5000 chars
md.append(long)
md.append("*" + "x"*10 + "* " + "y " * 2600)
for m in md:
    add('mdbody', md=m)
    add('mdbody', md=m, images=True)
    add('mdplain', md=m)
    add('imagetargets', md=m)
for m in md[:12]:
    add('mdfragment', md=m)
add('mddoc', title='Week of 5 to 11 Oct 2026 <&>', md="# T\n\n- a\n![x](assets/a.png)\n![y](assets/b.jpg)", images=True)
add('mddoc', title='Plain', md="text", images=False)

# CapturePlacement
texts = ["call with Sam about pricing","# x","## y z","###### s","####### seven","#tag","- x","+ x","* x","-5 degrees","*bold* word","1. x","12) x","1.5x faster","1.","> x",">x",
 "a\n# b\n- c\n> d\n1. e","a\n---","a\n===","***","- - -","___","```","~~~x","see <!-- x","<!--","one\r\ntwo","one\n\n\ntwo","  padded  ","",
 "emoji \U0001F389 \"q\" \\ b","[] literal","a\\\nb","line one\n  line two\n\tline three","12345678901. ten digits","123456789. nine digits"]
for t in texts:
    for stamp, todo in [("14:32", False), (None, False), (None, True), ("14:32", True)]:
        add('render', text=t, stamp=stamp, todo=todo)
bodies = ["", "\n\n  \n", "Some text here.\n\n", "## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next", "## Jotsy\n\ntext", "```\n## Jots\n```", "<!--\n## Jots\n-->\ntext", "a\r\nb",
 "text\n\n## Jots\n\n- 14:30 a", "## Jots\n\n- 14:30 a\n\n## Finished\n\nstuff", "## Jots\n\n- 14:30 a\n\n\n", "## Jots\n\nremember this", "## Jots\n\n- 14:30 a\n  more", "## Jots\n\n- 14:30 a\nlazy",
 "## Jots\n\n- a\n\npara", "## Jots\n\n1. a", "## Jots\n- [ ] a", "## Jots\n\n```\n- code\n```", "## Jots", "## Jots\n\n", "## Jots\n\n## Next\n\nstuff", "## Jots\n## Next",
 "### Jots\n- a\n\n## Next\ntext", "### Jots\n- a\n#### Sub\nmore", "# Jots\n- a\n## Sub\ntext\n# Next\nx", "## \U0001F4DD JOTS\n- a", "## jots!\n- a",
 "## Jots\n- a\n\n## Other\n\n## Jots\n- z", "```\ncode", "~~~~\ncode", "## Jots\n- a\n```\ncode", "text <!-- open", "## Jots\n- a\ntext <!-- open\nmore", "~~~\n## Jots\n- a",
 "## What I did\n\nwrote\n\n### \U0001F5D2️ Jots\n\n- 08:55 a\n\n#### Later\n\n- 09:30 z\n\n### Finished\n\nshipped",
 "## Jots\n\n* 14:32 call with Sam about pricing", "## Jots\n\n- 14:32 call with Sam about pricing", "- 14:32 call with Sam about pricing", "## Jots\n- 14:32 a b\n  - nested\n- 14:33 c"]
for b in bodies:
    for t, stamp, todo in [("call with Sam about pricing","14:32",False), ("# x", "14:32", False), ("a\nb", None, True), ("zz", "09:00", False)]:
        add('apply', text=t, stamp=stamp, todo=todo, body=b)
    add('jotkeys', body=b)
    add('lastjotkey', body=b)
    add('splitjots', body=b)
for h in ["Notes", "\U0001F4DD Jots", "", "\U0001F642", "  Daily \n notes "]:
    for b in ["text", "## Notes\n- a", "## Jots\n- a", "## Daily notes\n- 14:32 b"]:
        add('apply', text="b", stamp="14:32", todo=False, body=b, heading=h)
        add('jotkeys', body=b, heading=h)
        add('splitjots', body=b, heading=h)
for m in ["- 14:32 call Sam", "* 14:32   call   Sam  ", "- [x] call Sam", "- 14:32 2 \\* 3 = 6", "- 14:32 a\n  b", "- 14:32 a\\\n  b", "- 14:32 a\\\\\n  b", "- 14:32 <https://x.example/p> for Sam", "plain", "", "1. numbered\n  cont", "- 14:32 \\\\ escaped backslash \\q"]:
    add('key', md=m)

# periods
zones = [("America/New_York", [("2026-03-08",), ("2026-11-01",), ("2026-10-07",), ("2026-12-31",), ("2027-01-01",)]),
         ("Australia/Sydney", [("2026-04-05",), ("2026-10-04",), ("2026-10-05",), ("2026-02-28",)]),
         ("Asia/Beirut", [("2026-03-28",), ("2026-03-29",), ("2026-10-25",), ("2026-10-24",)]),
         ("America/Havana", [("2026-03-08",), ("2026-11-01",)]),
         ("Pacific/Kiritimati", [("2026-10-07",)]),
         ("Asia/Kolkata", [("2026-10-07",), ("2026-12-28",)]),
         ("UTC", [("2024-02-29",), ("2026-01-01",)])]
for tz, days in zones:
    for (d,) in days:
        for tod in ("00:30", "12:00", "23:30"):
            h, mi = tod.split(":")
            anchor = "%sT%s:%s" % (d, h, mi)
            for fw in (1, 2, 7):
                for sc in ("week", "month", "year", "range"):
                    add('period', scale=sc, tz=tz, fw=fw, anchor=anchor)
# shifts, daycount, title, filename on a set of periods
per = [("week","2026-10-05","2026-10-11"),("week","2026-12-28","2027-01-03"),("week","2026-01-01","2026-01-07"),("week","2025-12-29","2026-01-04"),("week","2026-03-02","2026-03-08"),
       ("week","2026-09-28","2026-10-04"),("month","2026-09-01","2026-09-30"),("month","2026-02-01","2026-02-28"),("month","2024-02-01","2024-02-29"),("month","2026-12-01","2026-12-31"),
       ("month","2026-01-01","2026-01-31"),("year","2026-01-01","2026-12-31"),("year","2024-01-01","2024-12-31"),("range","2026-09-05","2026-10-12"),("range","2026-10-07","2026-10-07"),
       ("range","2025-12-20","2026-01-10"),("range","2026-10-01","2026-10-05"),("range","2020-01-01","2026-10-07"),("week","bad","bad"),("month","2026-13-01","2026-13-30"),("range","2026-10-12","2026-10-05")]
for sc, s, e in per:
    for tz, fw in [("America/New_York", 2), ("America/New_York", 1), ("Australia/Sydney", 7), ("Asia/Beirut", 2)]:
        for n in (-2, -1, 1, 2):
            add('shift', scale=sc, start=s, end=e, n=n, tz=tz, fw=fw)
        add('daycount', scale=sc, start=s, end=e, tz=tz, fw=fw)
        add('title', scale=sc, start=s, end=e, tz=tz, fw=fw, dash=False)
        add('title', scale=sc, start=s, end=e, tz=tz, fw=fw, dash=True)
        add('filename', scale=sc, start=s, end=e, tz=tz, fw=fw)
for tz in ("America/New_York", "Australia/Sydney", "Asia/Beirut"):
    for a, b in [("2026-10-01T09:00","2026-10-05T09:00"),("2026-10-05T09:00","2026-10-01T09:00"),("2026-10-07T00:10","2026-10-07T23:50"),("2026-03-07T12:00","2026-03-09T12:00"),
                 ("2010-01-01T12:00","2026-10-07T12:00"),("2026-10-07T12:00","2026-10-07T12:00"),("2026-11-01T00:30","2026-11-01T23:30"),("2026-10-25T00:30","2026-10-26T23:30")]:
        add('range', tz=tz, fw=2, **{"from": a, "to": b})
for c in [[4,1,2,0],[0,0,0,0],[1,0,0,0],[5,0,0,0],[0,2,0,0],[3,1,1,1],[0,0,2,3],[4,0,1,0],[1,1,0,0]]:
    add('countsline', counts=c)
    for fut in (False, True):
        add('rowline', counts=c, future=fut)
for d, o in [(0,0),(1,0),(0,1),(1,1),(2,0),(12,3),(0,12)]:
    add('tasksline', done=d, open=o)

# reminders
st = {"2026-10-07":"logged","2026-10-08":"skipped","2026-10-09":"partial","2026-10-12":"missed"}
for tz in ("America/New_York", "Australia/Sydney", "Asia/Beirut", "America/Havana"):
    for now in ("2026-10-07T10:00","2026-10-07T16:55","2026-10-07T17:00","2026-10-09T23:59","2026-10-10T08:00","2026-11-01T08:00","2026-03-07T08:00","2026-10-24T08:00","2026-10-03T08:00"):
        for minutes in (1015, 0, 1439, 150, 60):
            add('reminder', tz=tz, fw=2, now=now, weekdays=[2,3,4,5,6], minutes=minutes, states=st, logStart=None, locale="en_US")
    add('reminder', tz=tz, fw=2, now="2026-10-07T10:00", weekdays=[2,3,4,5,6], minutes=1015, states=st, logStart="2026-10-09", locale="en_US")
    add('reminder', tz=tz, fw=2, now="2026-10-07T10:00", weekdays=[1,7], minutes=600, states={}, logStart=None, locale="en_US")
    add('reminder', tz=tz, fw=2, now="2026-10-07T10:00", weekdays=[2,3,4,5,6], minutes=1015, states={}, logStart=None, closedApp=False, locale="en_US")
    add('reminder', tz=tz, fw=2, now="2026-10-07T10:00", weekdays=[2], minutes=1015, states={}, logStart=None, horizon=3, locale="en_US")
    add('reminder', tz=tz, fw=2, now="2026-10-07T10:00", weekdays=[2,3,4,5,6], minutes=1015, states={}, logStart=None, locale="en_GB")
    add('reminder', tz=tz, fw=2, now="2026-10-07T10:00", weekdays=[], minutes=1015, states={}, logStart=None, locale="en_US")
for at in ("2026-10-08T16:55","2026-10-08T00:05","2026-10-08T12:00","2026-10-08T09:07","2026-10-08T23:59"):
    for loc in ("en_US","en_GB"):
        add('reminderlabel', tz="America/New_York", fw=2, at=at, locale=loc)
def pl(i, fire, title="Today isn't logged yet", body="It's 4:55 pm. Open Gloamlog when you're ready."):
    return {"id":"gloamlog.reminder.2026-10-%02d"%i, "fire":fire, "title":title, "body":body}
planned = [pl(8,[2026,10,8,16,55]), pl(9,[2026,10,9,16,55]), pl(12,[2026,10,12,16,55])]
add('reminderdiff', tz="UTC", planned=planned, pending=[])
add('reminderdiff', tz="UTC", planned=planned, pending=[dict(p, category="reminder") for p in planned])
add('reminderdiff', tz="UTC", planned=planned, pending=[dict(planned[0],category="reminder"), dict(planned[1], fire=[2026,10,9,17,0], category="reminder"), dict(planned[2], body="x", category="reminder"), pl(13,[2026,10,13,16,55]), {"id":"other.thing","fire":None,"title":"t","body":"b","category":"reminder"}, {"id":"gloamlog.snooze.2026-10-08","fire":None,"title":"t","body":"b","category":"reminder"}])
add('reminderdiff', tz="UTC", planned=planned, pending=[dict(planned[0], fire=None, category="reminder"), dict(planned[1], category="other")])
add('reminderdiff', tz="UTC", planned=[], pending=[pl(1,[2026,10,1,16,55])])

# reviews
tpl = "## What I did\n\n## Finished\n\n## Started\n\n## Pending / blocked\n\n## To do next"
def w(n, p="w"): return " ".join("%s%d"%(p,i) for i in range(1,n+1))
d_full = "## What I did\n\n" + w(30) + "\n\n## Finished\n\n- shipped the review screen\n- [x] wrote tests for it\n\n## Started\n\n- began the export\n\n## Pending / blocked\n\n- [ ] waiting on design\n\n## To do next\n\n- [ ] finish export\n- [ ] call Sam"
d_part = "## What I did\n\nonly a few words"
d_jots = "## What I did\n\n" + w(25, "j") + "\n\n## Finished\n\n- [x] closed the loop\n- [X] second done\n\n## Jots\n\n- 09:12 call Sam\n- 14:40 idea: **bold** thing\n- [ ] 15:00 follow up"
d_other = "Just some free text before any heading, " + w(24, "f") + "\n\n## Retro\n\nretro thoughts\n\n### deeper\n\ndeep text\n\n## Finished\n\n- [x] wrote tests for it\n- [x] Another one"
d_jotsonly = "## Jots\n\n- 10:00 note one\n- 10:30 note two"
d_code = "## What I did\n\n" + w(22,"c") + "\n\n```\n- [x] not a task\n```\n\n## Finished\n\n- [x] real task ![img](assets/a.png)\n"
d_img = "## What I did\n\n" + w(21,"i") + "\n\n![shot](assets/a.png) and ![other](assets/b.jpg)\n\n## To do next\n\n- [ ] carry this\n- [ ] and this"
d_empty_sections = "## What I did\n\n## Finished\n\n## Started\n\n- began something real here\n"
d_carried = "## Carried over from Mon\n\n- [x] old thing\n\n## What I did\n\n" + w(26, "k")
docs_week = [
 {"day":"2026-10-05","body":d_full}, {"day":"2026-10-06","body":d_part}, {"day":"2026-10-07","skipped":True,"reason":"Dentist"}, {"day":"2026-10-08","body":d_jots},
 {"day":"2026-10-09","body":d_other}, {"day":"2026-10-10","body":d_code}, {"day":"2026-10-11","body":d_jotsonly}]
settings0 = {"template": tpl, "carry": ["To do next","Pending / blocked"], "minWords": 20}
opt_variants = [{}, {"grouping":"day"}, {"jots":True}, {"jots":True,"grouping":"day"}, {"skipped":False}, {"skipped":False,"grouping":"day","jots":True}]
for o in opt_variants:
    add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-12T09:00", docs=docs_week, autostates=True, settings=settings0, options=o)
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-08T09:00", docs=docs_week[:4], autostates=True, settings=settings0, options={})
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-07T09:00", docs=docs_week[:3], autostates=True, settings=dict(settings0, logStart="2026-10-06"), options={})
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-12T09:00", docs=[], autostates=True, settings=settings0, options={})
add('review', tz="America/New_York", fw=1, scale="week", anchor="2026-10-07T12:00", now="2026-10-12T09:00", docs=docs_week, autostates=True, settings=dict(settings0, weekdays=[1,2,3,4,5,6,7]), options={})
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-12T09:00", docs=docs_week, autostates=True, settings=dict(settings0, jotsCount=True), options={"jots":True})
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-12T09:00", docs=docs_week, autostates=True, settings=dict(settings0, jotsHeading="Notes"), options={"jots":True})
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-12T09:00", docs=docs_week, autostates=True, settings=dict(settings0, minWords=5), options={})
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-12T09:00", docs=docs_week, autostates=True, settings=dict(settings0, template="## Done\n\n## Next", carry=["Next"]), options={})
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-10-07T12:00", now="2026-10-12T09:00", docs=docs_week, autostates=False, settings=settings0, options={})
month_docs = [{"day":"2026-09-%02d"%d, "body":b} for d,b in [(1,d_full),(2,d_other),(3,d_part),(8,d_jots),(9,d_code),(15,d_img),(16,d_empty_sections),(22,d_carried),(29,d_full),(30,d_full)]] + [{"day":"2026-09-10","skipped":True,"reason":""},{"day":"2026-09-17","skipped":True,"reason":"Holiday"}]
for o in [{}, {"grouping":"day"}, {"jots":True,"skipped":False}]:
    add('review', tz="America/New_York", fw=2, scale="month", anchor="2026-09-15T12:00", now="2026-10-07T09:00", docs=month_docs, autostates=True, settings=settings0, options=o)
add('review', tz="America/New_York", fw=2, scale="month", anchor="2026-09-15T12:00", now="2026-09-15T09:00", docs=month_docs, autostates=True, settings=settings0, options={})
add('review', tz="Australia/Sydney", fw=7, scale="month", anchor="2026-10-15T12:00", now="2026-10-31T23:30", docs=[{"day":"2026-10-%02d"%d,"body":d_full} for d in (1,2,5,6,7,12,13)], autostates=True, settings=settings0, options={})
year_docs = [{"day":"2026-%02d-%02d"%(m,d), "body":d_full} for m in (1,2,3,5,6,9,10) for d in (3,4,5,10,11,12,17,18,19)] + [{"day":"2026-03-30","skipped":True,"reason":"Away"},{"day":"2026-05-20","body":d_part}]
for o in [{}, {"skipped":False}]:
    add('review', tz="America/New_York", fw=2, scale="year", anchor="2026-06-15T12:00", now="2026-10-07T09:00", docs=year_docs, autostates=True, settings=settings0, options=o)
add('review', tz="America/New_York", fw=2, scale="year", anchor="2026-06-15T12:00", now="2026-10-07T09:00", docs=[], autostates=True, settings=settings0, options={})
add('review', tz="America/New_York", fw=2, scale="year", anchor="2027-06-15T12:00", now="2026-10-07T09:00", docs=[], autostates=True, settings=settings0, options={})
add('review', tz="America/New_York", fw=2, scale="year", anchor="2026-06-15T12:00", now="2026-10-07T09:00", docs=year_docs, autostates=True, settings=dict(settings0, logStart="2026-05-01"), options={})
range_docs = [{"day":"2026-09-%02d"%d,"body":d_full} for d in (28,29,30)] + [{"day":"2026-10-%02d"%d,"body":b} for d,b in [(1,d_jots),(2,d_other),(5,d_img)]]
for o in [{}, {"grouping":"day"}]:
    add('review', tz="America/New_York", fw=2, scale="range", start="2026-09-25", end="2026-10-06", now="2026-10-07T09:00", docs=range_docs, autostates=True, settings=settings0, options=o)
add('review', tz="America/New_York", fw=2, scale="range", start="2026-10-03", end="2026-10-03", now="2026-10-07T09:00", docs=[], autostates=True, settings=settings0, options={})
# DST weeks
add('review', tz="America/New_York", fw=2, scale="week", anchor="2026-11-01T12:00", now="2026-11-10T09:00", docs=[{"day":"2026-10-30","body":d_full},{"day":"2026-11-01","body":d_part},{"day":"2026-11-02","body":d_full}], autostates=True, settings=settings0, options={})
add('review', tz="America/New_York", fw=1, scale="week", anchor="2026-03-08T12:00", now="2026-03-14T09:00", docs=[{"day":"2026-03-09","body":d_full}], autostates=True, settings=dict(settings0, weekdays=[1,2,3,4,5,6,7]), options={})
add('review', tz="Asia/Beirut", fw=2, scale="week", anchor="2026-03-29T12:00", now="2026-04-05T09:00", docs=[{"day":"2026-03-26","body":d_full},{"day":"2026-03-30","body":d_full}], autostates=True, settings=dict(settings0, weekdays=[1,2,3,4,5,6,7]), options={})
add('review', tz="Australia/Sydney", fw=2, scale="week", anchor="2026-10-04T12:00", now="2026-10-12T09:00", docs=[{"day":"2026-09-28","body":d_full},{"day":"2026-10-04","body":d_full}], autostates=True, settings=dict(settings0, weekdays=[1,2,3,4,5,6,7]), options={})
add('review', tz="America/New_York", fw=2, scale="month", anchor="2026-11-10T12:00", now="2026-12-02T09:00", docs=[{"day":"2026-11-01","body":d_full},{"day":"2026-11-02","body":d_part}], autostates=True, settings=dict(settings0, weekdays=[1,2,3,4,5,6,7]), options={})


# journal lines and normalise
jt = ["plain", "line one\nline \"two\" \\ back\tslash \u2028 sep \U0001F468\u200d\U0001F469 caf\u00e9 </script> {\"x\":1}", "ctl \u0001 \u001f \u007f \u0085 \u00a0 end", "slash / and \\/ and & < >", "[] literal", "\u0008 bs \u000c ff \r cr", "emoji \U0001F389\U0001F389", "", "  spaces  "]
for t in jt:
    for ts in (1790000000.0, 1790000000.123, 1790000000.123456, 0.0, -1.5, 1790000000.9996, 4102444800.001):
        add('journalline', id="id-1", ts=ts, day="2026-10-07", text=t, stamp="14:32", todo=False, source="hotkey")
    add('journalline', id="u/ü\"", ts=1790000000.5, day="2026-10-07", text=t, stamp=None, todo=True, source="menu")
raws = ["  hello  ", "a\r\nb", "a\rb", "a\r\n\r\nb", "a\n\nb", "a\u2028b\u2029c\u0085d", "a\tb\u000bc\u000cd", "a\u0000b\u0007c\u001bd\u007fe", "[] call Sam", "[ ] call Sam", "[]call Sam", "[]", "[ ]  \n ", "x []",
        "   \n\t ", "\u0000\u0001", "a"*5000, "a"*3999 + " " + "b"*10, "caf\u00e9 \U0001F389", "[ ][ ] x", "[] [] x", "\u00a0 nbsp \u00a0", "line1\n   \nline3", "\u200b zero width", "\u0080\u009f c1 controls"]
for r in raws:
    for sc in (True, False):
        add('normalise', raw=r, shortcut=sc)


# fuzz: random strings over markdown-ish tokens (fixed seed)
import random
rnd = random.Random(20261018)
toks = ["*","**","***","_","__","~","~~","`","``","[","]","(",")","![","](","](https://x.example)","](a.png)","\\","\n","\n\n","- ","  - ","1. ","> ","# ","## ","---","| ","|","|---|"," ","\t","word","two words","[ ] ","[x] ","<","&","\"","'","```\n","~~~\n","<!--","-->","caf\u00e9","\U0001F389","e\u0301","\u200b","https://a.example/b_c","!","#","+","=","1)","- [ ] ","- [x] "]
for n in range(700):
    m = "".join(rnd.choice(toks) for _ in range(rnd.randint(1, 30)))
    add('mdbody', md=m, images=(n % 2 == 0))
    add('mdplain', md=m)
    add('imagetargets', md=m)
    if n % 3 == 0:
        add('apply', text=m, stamp="14:32", todo=False, body=m[::-1] if n % 2 else "")
        add('render', text=m, stamp=None, todo=(n % 2 == 0))
        add('key', md=m)
        add('jotkeys', body=m)
        add('splitjots', body=m)

# trailing: ids must be unique; write
ids = [c['id'] for c in cases]
assert len(ids) == len(set(ids))
json.dump(cases, open(os.path.join(HERE, '..', 'chunkb_cases.json'), 'w'), ensure_ascii=False, separators=(',', ':'))
print(len(cases))
