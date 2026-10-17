package main

// The Swift app kept its settings in UserDefaults. On the first run of this app we read them once with
// `defaults export <domain> -` (an XML plist on stdout) and hand the top-level values to core.ImportLegacyDefaults.
// Read only: the old domain is never written.

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func readLegacyDefaults(domain string) map[string]any {
	out, err := exec.Command("/usr/bin/defaults", "export", domain, "-").Output()
	if err != nil {
		return nil // no such domain: a fresh install
	}
	m, _ := parsePlist(out).(map[string]any)
	return m
}

// parsePlist decodes an XML property list into Go values: map[string]any, []any, string, []byte, int64, float64, bool,
// time.Time. Anything it cannot read becomes nil.
func parsePlist(data []byte) any {
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.Token()
		if err != nil {
			return nil
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local != "plist" {
			return plistValue(d, se)
		}
	}
}

func plistValue(d *xml.Decoder, se xml.StartElement) any {
	switch se.Name.Local {
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			tok, err := d.Token()
			if err != nil {
				return m
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if t.Name.Local == "key" {
					key = plistText(d)
				} else {
					m[key] = plistValue(d, t)
				}
			case xml.EndElement:
				return m
			}
		}
	case "array":
		var a []any
		for {
			tok, err := d.Token()
			if err != nil {
				return a
			}
			switch t := tok.(type) {
			case xml.StartElement:
				a = append(a, plistValue(d, t))
			case xml.EndElement:
				return a
			}
		}
	case "true", "false":
		_ = d.Skip()
		return se.Name.Local == "true"
	}
	s := plistText(d)
	switch se.Name.Local {
	case "data":
		b, _ := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
		return b
	case "integer":
		i, _ := strconv.ParseInt(s, 10, 64)
		return i
	case "real":
		f, _ := strconv.ParseFloat(s, 64)
		return f
	case "date":
		t, _ := time.Parse(time.RFC3339, s)
		return t
	}
	return s
}

// plistText reads the character data up to the element's end tag.
func plistText(d *xml.Decoder) string {
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return b.String()
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String()
		}
	}
}
