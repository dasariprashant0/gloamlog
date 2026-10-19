package main

// "From yesterday" on a list page: Keep and Done add the line to the page (the screen does that, like any edit); Drop is
// remembered here so the line is not offered again that day. The file holds item keys (core.ItemKey), never whole pages,
// and forgets days older than 30. A damaged file means everything is offered again: nothing on any page is affected.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dasariprashant0/gloamlog/internal/core"
)

const carryDismissedFile = "carry-dismissed.json"

// carryDismissed is day -> the keys dropped from that day's offer.
type carryDismissed map[string][]string

func (n *Notes) carryDismissedPath() string {
	return filepath.Join(filepath.Dir(n.settingsPath), carryDismissedFile)
}

func (n *Notes) loadCarryDismissed() carryDismissed {
	data, err := os.ReadFile(n.carryDismissedPath())
	if err != nil {
		return carryDismissed{} // none dropped yet, or unreadable: everything is offered
	}
	var d carryDismissed
	if json.Unmarshal(data, &d) != nil || d == nil {
		return carryDismissed{}
	}
	return d
}

// CarryDismiss drops one item from day's "From yesterday" offer.
func (n *Notes) CarryDismiss(day, text string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	d := n.loadCarryDismissed()
	key := core.ItemKey(text)
	for _, k := range d[day] {
		if k == key {
			return nil
		}
	}
	d[day] = append(d[day], key)
	cutoff := core.DayKeyString(n.now().AddDate(0, 0, -30), time.Local)
	for k := range d {
		if k < cutoff {
			delete(d, k)
		}
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	path := n.carryDismissedPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return core.AtomicWrite(append(data, '\n'), path, "")
}

// offerFor takes out of a list page's card what was dropped today or is already a line on body. A classic card (no marks)
// comes back as it is. nil when nothing is left.
func (n *Notes) offerFor(day, body string, c *core.CarryCard) *core.CarryCard {
	if c == nil || c.Marks == nil {
		return c
	}
	gone := map[string]bool{}
	for _, k := range n.loadCarryDismissed()[day] {
		gone[k] = true
	}
	if p, ok := core.ParseLog(body); ok {
		for _, l := range p.Lines {
			gone[core.ItemKey(l.Text)] = true
		}
	}
	out := *c
	out.Items, out.Marks, out.Lists = nil, nil, nil
	for i, it := range c.Items {
		if !gone[core.ItemKey(it)] {
			out.Items, out.Marks = append(out.Items, it), append(out.Marks, c.Marks[i])
			if i < len(c.Lists) {
				out.Lists = append(out.Lists, c.Lists[i])
			}
		}
	}
	if len(out.Items) == 0 {
		return nil
	}
	return &out
}
