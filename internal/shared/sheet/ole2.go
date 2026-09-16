package sheet

// The compound file an .xls lives inside.
//
// A legacy Excel workbook is not a file format, it is a filesystem: a
// Microsoft Compound File with a FAT, a directory and a stream called
// "Workbook" holding the BIFF records. Everything reader_biff_native.go does
// starts by reassembling that one stream in order, and this file is the whole
// of that job.
//
// It exists because both third-party decoders in this package mis-read real
// distributor files — see reader_biff_native.go for the measurement — and
// neither exposes the record stream, so there was no way to read the workbook
// correctly without reading the container correctly first.

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Compound-file constants, from the container specification.
const (
	cfHeaderSize   = 512
	cfDirEntrySize = 128
	// Sector chain terminators.
	cfEndOfChain = 0xFFFFFFFE
	cfFreeSector = 0xFFFFFFFF
	// The header holds this many FAT sector numbers before the DIFAT chain
	// takes over.
	cfHeaderFATSlots = 109
	// A directory entry that names a stream rather than a storage.
	cfStreamEntry = 2
	// Guards against a malformed or hostile file describing an unbounded
	// structure. A workbook stream past a quarter of a gigabyte is not one this
	// importer is going to read anyway.
	cfMaxSectors = 1 << 20
)

var errNotCompoundFile = errors.New("not a compound file")

// compoundFile is a parsed OLE2 container, able to return a named stream.
type compoundFile struct {
	raw        []byte
	sectorSize int
	fat        []uint32
	miniFAT    []uint32
	miniStream []byte
	miniCutoff uint32
	dir        []cfEntry
}

// cfEntry is one directory entry: a stream or a storage.
type cfEntry struct {
	name  string
	kind  byte
	start uint32
	size  uint64
}

// openCompoundFile parses the container header, FAT and directory.
func openCompoundFile(raw []byte) (*compoundFile, error) {
	if len(raw) < cfHeaderSize || !hasOLESignature(raw) {
		return nil, errNotCompoundFile
	}
	sectorShift := binary.LittleEndian.Uint16(raw[30:32])
	if sectorShift < 7 || sectorShift > 20 {
		return nil, fmt.Errorf("compound file: implausible sector shift %d", sectorShift)
	}
	c := &compoundFile{
		raw:        raw,
		sectorSize: 1 << sectorShift,
		miniCutoff: binary.LittleEndian.Uint32(raw[56:60]),
	}
	if err := c.readFAT(); err != nil {
		return nil, err
	}
	if err := c.readDirectory(binary.LittleEndian.Uint32(raw[48:52])); err != nil {
		return nil, err
	}
	c.readMiniStream(binary.LittleEndian.Uint32(raw[60:64]))
	return c, nil
}

// hasOLESignature reports the eight bytes every compound file begins with.
func hasOLESignature(raw []byte) bool {
	const sig = "\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1"
	return len(raw) >= len(sig) && string(raw[:len(sig)]) == sig
}

// sector returns one sector's bytes, or nil where the file does not reach it.
//
// A short final sector is returned as far as the file goes rather than refused:
// real files written by real tools are routinely a few bytes shy of a sector
// boundary, and the record walk stops on its own at the end of the data.
func (c *compoundFile) sector(n uint32) []byte {
	start := cfHeaderSize + int(n)*c.sectorSize
	if start < 0 || start >= len(c.raw) {
		return nil
	}
	end := min(start+c.sectorSize, len(c.raw))
	return c.raw[start:end]
}

// readFAT assembles the file allocation table from the header slots and the
// DIFAT chain that continues them.
func (c *compoundFile) readFAT() error {
	numFAT := binary.LittleEndian.Uint32(raw32(c.raw, 44))
	sectors := make([]uint32, 0, min(numFAT, cfMaxSectors))
	for i := 0; i < cfHeaderFATSlots && uint32(len(sectors)) < numFAT; i++ {
		s := binary.LittleEndian.Uint32(c.raw[76+i*4 : 80+i*4])
		if s == cfFreeSector || s == cfEndOfChain {
			break
		}
		sectors = append(sectors, s)
	}
	// The DIFAT: each sector holds (sectorSize/4 - 1) more FAT sector numbers
	// and ends with the next DIFAT sector.
	next := binary.LittleEndian.Uint32(c.raw[68:72])
	for guard := 0; next != cfEndOfChain && next != cfFreeSector &&
		uint32(len(sectors)) < numFAT && guard < cfMaxSectors; guard++ {
		body := c.sector(next)
		if len(body) < c.sectorSize {
			break
		}
		for j := 0; j+4 <= c.sectorSize-4; j += 4 {
			s := binary.LittleEndian.Uint32(body[j : j+4])
			if s == cfFreeSector || s == cfEndOfChain {
				continue
			}
			sectors = append(sectors, s)
		}
		next = binary.LittleEndian.Uint32(body[c.sectorSize-4:])
	}

	c.fat = make([]uint32, 0, len(sectors)*c.sectorSize/4)
	for _, s := range sectors {
		body := c.sector(s)
		for j := 0; j+4 <= len(body); j += 4 {
			c.fat = append(c.fat, binary.LittleEndian.Uint32(body[j:j+4]))
		}
	}
	if len(c.fat) == 0 {
		return errors.New("compound file: empty allocation table")
	}
	return nil
}

// chain walks the sector chain starting at start, refusing to loop.
func (c *compoundFile) chain(start uint32) []uint32 {
	var out []uint32
	seen := make(map[uint32]struct{}, 64)
	for s := start; s != cfEndOfChain && s != cfFreeSector && int(s) < len(c.fat); s = c.fat[s] {
		if _, loop := seen[s]; loop || len(out) > cfMaxSectors {
			break
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// readDirectory reads every entry of the container's directory.
func (c *compoundFile) readDirectory(start uint32) error {
	for _, sec := range c.chain(start) {
		body := c.sector(sec)
		for off := 0; off+cfDirEntrySize <= len(body); off += cfDirEntrySize {
			e := body[off : off+cfDirEntrySize]
			nameLen := int(binary.LittleEndian.Uint16(e[64:66]))
			if nameLen <= 2 || nameLen > 64 {
				continue
			}
			name := make([]rune, 0, nameLen/2)
			for k := 0; k+1 < nameLen-2; k += 2 {
				name = append(name, rune(binary.LittleEndian.Uint16(e[k:k+2])))
			}
			c.dir = append(c.dir, cfEntry{
				name:  string(name),
				kind:  e[66],
				start: binary.LittleEndian.Uint32(e[116:120]),
				size:  binary.LittleEndian.Uint64(e[120:128]),
			})
		}
	}
	if len(c.dir) == 0 {
		return errors.New("compound file: empty directory")
	}
	return nil
}

// readMiniStream loads the container's small-stream area, which is itself a
// stream hanging off the root entry.
func (c *compoundFile) readMiniStream(miniFATStart uint32) {
	for _, sec := range c.chain(miniFATStart) {
		body := c.sector(sec)
		for j := 0; j+4 <= len(body); j += 4 {
			c.miniFAT = append(c.miniFAT, binary.LittleEndian.Uint32(body[j:j+4]))
		}
	}
	if len(c.dir) == 0 {
		return
	}
	root := c.dir[0]
	for _, sec := range c.chain(root.start) {
		c.miniStream = append(c.miniStream, c.sector(sec)...)
	}
}

// stream returns the bytes of a named stream, truncated to the length the
// directory declares.
func (c *compoundFile) stream(name string) ([]byte, error) {
	for _, e := range c.dir {
		if e.name != name || e.kind != cfStreamEntry {
			continue
		}
		var out []byte
		if e.size < uint64(c.miniCutoff) {
			out = c.readMini(e.start, e.size)
		} else {
			for _, sec := range c.chain(e.start) {
				out = append(out, c.sector(sec)...)
			}
		}
		if uint64(len(out)) > e.size {
			out = out[:e.size]
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("compound file: stream %q is empty", name)
		}
		return out, nil
	}
	return nil, fmt.Errorf("compound file: no stream %q", name)
}

// readMini assembles a small stream out of the mini-FAT.
func (c *compoundFile) readMini(start uint32, size uint64) []byte {
	const miniSectorSize = 64
	var out []byte
	seen := make(map[uint32]struct{}, 32)
	for s := start; s != cfEndOfChain && s != cfFreeSector && int(s) < len(c.miniFAT); s = c.miniFAT[s] {
		if _, loop := seen[s]; loop || uint64(len(out)) >= size {
			break
		}
		seen[s] = struct{}{}
		off := int(s) * miniSectorSize
		if off >= len(c.miniStream) {
			break
		}
		out = append(out, c.miniStream[off:min(off+miniSectorSize, len(c.miniStream))]...)
	}
	return out
}

// raw32 returns a four-byte window, or four zero bytes past the end.
func raw32(b []byte, off int) []byte {
	if off+4 > len(b) {
		return []byte{0, 0, 0, 0}
	}
	return b[off : off+4]
}
