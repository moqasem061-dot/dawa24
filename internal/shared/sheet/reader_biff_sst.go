package sheet

// The shared-string table, and the rule at a CONTINUE boundary.
//
// Every piece of text in a BIFF8 worksheet is a four-byte index into one table
// held in the workbook globals. The table is usually far larger than the 8,224
// bytes a record may hold, so it continues into CONTINUE records — and a string
// may be cut anywhere, including between two of its own characters.
//
// When that happens the continuation record begins with one extra byte, an
// encoding flag for the REST of that string, and the rest may be encoded
// differently from the beginning: a name whose first half is plain ASCII and
// whose second half is Arabic is split as compressed bytes followed by UTF-16.
// A decoder that does not read that byte is one byte out of step for the whole
// remainder of the table, which is why a real file returns its first hundred
// and fifty strings correctly and nothing afterwards.
//
// That single rule is the reason this file exists. Everything else here is
// bookkeeping around it.

import "encoding/binary"

// String option bits, from the BIFF8 unicode string.
const (
	sstHighByte = 0x01 // characters are UTF-16LE rather than one byte each
	sstExtSt    = 0x04 // an Asian phonetic block follows
	sstRichSt   = 0x08 // formatting runs follow
)

// readSharedStrings decodes the SST record and its continuations.
//
// A table that runs out mid-string returns what it read: the strings already
// decoded are correct and are the ones the sheet's earlier rows point at, and
// returning none of them would turn a partial loss into a total one.
func readSharedStrings(rec biffRecord) []string {
	if rec.id != recSST || len(rec.body) < 8 {
		return nil
	}
	unique := int(int32(binary.LittleEndian.Uint32(rec.body[4:8])))
	if unique <= 0 {
		return nil
	}
	if unique > maxSSTStrings {
		unique = maxSSTStrings
	}

	c := &sstCursor{records: append([][]byte{rec.body}, rec.continues...), pos: 8}
	out := make([]string, 0, unique)
	for i := 0; i < unique; i++ {
		s, ok := c.readString()
		if !ok {
			break
		}
		out = append(out, s)
	}
	return out
}

// sstCursor reads across a record and its continuations, remembering where the
// boundaries are.
type sstCursor struct {
	records [][]byte
	rec     int
	pos     int
}

// atEnd reports whether every record has been consumed.
func (c *sstCursor) atEnd() bool {
	c.settle()
	return c.rec >= len(c.records)
}

// settle advances past any records the cursor has finished.
func (c *sstCursor) settle() {
	for c.rec < len(c.records) && c.pos >= len(c.records[c.rec]) {
		c.rec++
		c.pos = 0
	}
}

// crossed reports whether the cursor is exactly at the start of a continuation
// record, which is where the extra encoding byte lives.
func (c *sstCursor) crossed() bool {
	c.settle()
	return c.rec > 0 && c.pos == 0
}

// byteAt reads one byte, or reports the table has run out.
func (c *sstCursor) byteAt() (byte, bool) {
	if c.atEnd() {
		return 0, false
	}
	b := c.records[c.rec][c.pos]
	c.pos++
	return b, true
}

// take reads n bytes from the current record only.
//
// The fixed-width headers this is used for — the character count, the run
// count, the phonetic length — are not split across a record boundary by any
// writer, and reading them as if they might be would mean guessing whether a
// leading byte is data or an encoding flag. Where the current record is short,
// the cursor moves to the next one first.
func (c *sstCursor) take(n int) ([]byte, bool) {
	if c.atEnd() {
		return nil, false
	}
	body := c.records[c.rec]
	if c.pos+n > len(body) {
		// The header straddles the boundary. Skip the remainder rather than
		// misread it; the caller will fail and decoding stops cleanly.
		return nil, false
	}
	out := body[c.pos : c.pos+n]
	c.pos += n
	return out, true
}

// skip discards n bytes, following the record chain.
func (c *sstCursor) skip(n int) {
	for n > 0 {
		if c.atEnd() {
			return
		}
		body := c.records[c.rec]
		step := min(n, len(body)-c.pos)
		c.pos += step
		n -= step
	}
}

// readString reads one shared string, honouring the encoding byte that opens a
// continuation record.
func (c *sstCursor) readString() (string, bool) {
	head, ok := c.take(3)
	if !ok {
		return "", false
	}
	cch := int(binary.LittleEndian.Uint16(head[0:2]))
	flags := head[2]
	high := flags&sstHighByte != 0

	var runs, extra int
	if flags&sstRichSt != 0 {
		b, got := c.take(2)
		if !got {
			return "", false
		}
		runs = int(binary.LittleEndian.Uint16(b))
	}
	if flags&sstExtSt != 0 {
		b, got := c.take(4)
		if !got {
			return "", false
		}
		extra = int(int32(binary.LittleEndian.Uint32(b)))
	}

	text, ok := c.readChars(cch, high)
	if !ok {
		return "", false
	}
	// The formatting runs and the phonetic block belong to this string and are
	// not text. Four bytes per run, by the format's definition.
	c.skip(runs * 4)
	if extra > 0 {
		c.skip(extra)
	}
	return text, true
}

// readChars reads cch characters, re-reading the encoding flag every time the
// cursor steps into a continuation record.
//
// This is the rule the whole file is about. `high` is a variable rather than a
// parameter used once, because the second half of a split string may be encoded
// differently from the first.
func (c *sstCursor) readChars(cch int, high bool) (string, bool) {
	runes := make([]rune, 0, cch)
	for i := 0; i < cch; i++ {
		if c.atEnd() {
			return string(runes), false
		}
		if c.crossed() {
			flag, ok := c.byteAt()
			if !ok {
				return string(runes), false
			}
			high = flag&sstHighByte != 0
		}
		if high {
			b, ok := c.take(2)
			if !ok {
				return string(runes), false
			}
			runes = append(runes, rune(binary.LittleEndian.Uint16(b)))
			continue
		}
		b, ok := c.byteAt()
		if !ok {
			return string(runes), false
		}
		runes = append(runes, rune(b))
	}
	return string(runes), true
}
