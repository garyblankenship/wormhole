package server

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// customInputDecoder scans only newly arrived bytes of the provider's JSON
// wrapper. pending holds an incomplete UTF-8 rune or JSON escape (at most 12
// bytes for a surrogate pair); emitted characters are never parsed again.
type customInputDecoder struct {
	phase   byte
	keyPos  int
	pending []byte
	decoded strings.Builder
}

func (d *customInputDecoder) append(fragment string) string {
	var delta strings.Builder
	for i := 0; i < len(fragment) && d.phase < 5; i++ {
		b := fragment[i]
		switch d.phase {
		case 0: // Opening object.
			if jsonSpace(b) {
				continue
			}
			if b != '{' {
				d.phase = 6
				break
			}
			d.phase = 1
		case 1: // The first property must be "input" for live deltas.
			if d.keyPos == 0 && jsonSpace(b) {
				continue
			}
			const key = `"input"`
			if b != key[d.keyPos] {
				d.phase = 6
				break
			}
			d.keyPos++
			if d.keyPos == len(key) {
				d.phase = 2
			}
		case 2:
			if jsonSpace(b) {
				continue
			}
			if b != ':' {
				d.phase = 6
				break
			}
			d.phase = 3
		case 3:
			if jsonSpace(b) {
				continue
			}
			if b != '"' {
				d.phase = 6
				break
			}
			d.phase = 4
		case 4:
			if len(d.pending) != 0 {
				d.pending = append(d.pending, b)
				if d.pending[0] == '\\' {
					d.consumeEscape(&delta)
				} else if utf8.FullRune(d.pending) {
					d.consumeRune(&delta)
				}
				continue
			}
			switch {
			case b == '"':
				d.phase = 5
			case b == '\\' || b >= utf8.RuneSelf:
				d.pending = append(d.pending, b)
			case b < 0x20:
				d.phase = 6
			default:
				d.emit(&delta, string(b))
			}
		}
	}
	return delta.String()
}

func (d *customInputDecoder) consumeEscape(delta *strings.Builder) {
	if len(d.pending) < 2 {
		return
	}
	n := 2
	if d.pending[1] == 'u' {
		n = 6
		if len(d.pending) >= 6 {
			value, err := strconv.ParseUint(string(d.pending[2:6]), 16, 16)
			if err != nil {
				d.phase = 6
				return
			}
			if value >= 0xd800 && value <= 0xdbff {
				n = 12 // A high surrogate needs the next complete escape.
			}
		}
	}
	if len(d.pending) < n {
		return
	}
	if n == 12 {
		if string(d.pending[6:8]) != `\u` {
			d.phase = 6
			return
		}
		low, err := strconv.ParseUint(string(d.pending[8:12]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			d.phase = 6
			return
		}
		high, _ := strconv.ParseUint(string(d.pending[2:6]), 16, 16)
		d.emit(delta, string(utf16.DecodeRune(rune(high), rune(low))))
	} else {
		decoded, err := strconv.Unquote(`"` + string(d.pending) + `"`)
		if err != nil {
			d.phase = 6
			return
		}
		d.emit(delta, decoded)
	}
	d.pending = d.pending[:0]
}

func (d *customInputDecoder) consumeRune(delta *strings.Builder) {
	_, size := utf8.DecodeRune(d.pending)
	if size == 1 {
		d.phase = 6
		return
	}
	d.emit(delta, string(d.pending))
	d.pending = d.pending[:0]
}

func (d *customInputDecoder) emit(delta *strings.Builder, value string) {
	d.decoded.WriteString(value)
	delta.WriteString(value)
}

func jsonSpace(b byte) bool {
	return b == ' ' || b == '\n' || b == '\r' || b == '\t'
}

// A complete valid wrapper can contain properties before or after input, or
// repeated input keys. Decode it once at completion to retain JSON semantics.
func (d *customInputDecoder) final(raw string) string {
	var full struct {
		Input string `json:"input"`
	}
	if json.Unmarshal([]byte(raw), &full) == nil {
		return full.Input
	}
	return d.decoded.String()
}

// partialCustomToolInput is retained for callers requesting a single snapshot.
func partialCustomToolInput(raw string) string {
	var d customInputDecoder
	d.append(raw)
	return d.final(raw)
}
