package sheet

import (
	"encoding/binary"
	"math"
	"strconv"
)

// decodeSheet reads one worksheet substream into a dense grid.
func decodeSheet(records []biffRecord, stream []byte, offset int, sst []string) [][]string {
	start := indexOfOffset(records, offset)
	if start < 0 {
		return nil
	}
	cells := map[int]map[int]string{}
	maxRow, maxCol := -1, -1
	put := func(row, col int, value string) {
		if row < 0 || col < 0 || row >= maxBiffRows || col >= maxBiffCols {
			return
		}
		line, ok := cells[row]
		if !ok {
			line = map[int]string{}
			cells[row] = line
		}
		line[col] = value
		maxRow = max(maxRow, row)
		maxCol = max(maxCol, col)
	}

	// A formula's string result arrives in the record after it.
	pendingRow, pendingCol := -1, -1

	for i := start + 1; i < len(records); i++ {
		rec := records[i]
		if rec.id == recBOF {
			break // the next substream
		}
		if rec.id == recEOF {
			break
		}
		body := rec.body
		switch rec.id {
		case recLabelSST:
			if len(body) >= 10 {
				idx := int(binary.LittleEndian.Uint32(body[6:10]))
				if idx >= 0 && idx < len(sst) {
					put(cellRow(body), cellCol(body), sst[idx])
				}
			}
		case recLabel, recRString:
			if len(body) >= 9 {
				put(cellRow(body), cellCol(body), readLongUnicode(body[6:]))
			}
		case recNumber:
			if len(body) >= 14 {
				put(cellRow(body), cellCol(body),
					formatBiffNumber(math.Float64frombits(binary.LittleEndian.Uint64(body[6:14]))))
			}
		case recRK:
			if len(body) >= 10 {
				put(cellRow(body), cellCol(body),
					formatBiffNumber(decodeRK(binary.LittleEndian.Uint32(body[6:10]))))
			}
		case recMulRK:
			readMulRK(body, put)
		case recBoolErr:
			if len(body) >= 8 && body[7] == 0 {
				put(cellRow(body), cellCol(body), strconv.FormatBool(body[6] != 0))
			}
		case recFormula:
			if len(body) >= 14 {
				row, col := cellRow(body), cellCol(body)
				if isStringFormula(body[6:14]) {
					pendingRow, pendingCol = row, col
					continue
				}
				if !isNonNumericFormula(body[6:14]) {
					put(row, col, formatBiffNumber(
						math.Float64frombits(binary.LittleEndian.Uint64(body[6:14]))))
				}
			}
		case recString:
			if pendingRow >= 0 {
				put(pendingRow, pendingCol, readLongUnicode(body))
			}
		}
		if rec.id != recFormula {
			pendingRow, pendingCol = -1, -1
		}
	}

	if maxRow < 0 {
		return nil
	}
	grid := make([][]string, maxRow+1)
	for r := range grid {
		line := cells[r]
		if len(line) == 0 {
			continue
		}
		row := make([]string, maxCol+1)
		for c, v := range line {
			row[c] = v
		}
		grid[r] = row
	}
	return grid
}

// indexOfOffset finds the record that begins at a byte offset in the stream.
func indexOfOffset(records []biffRecord, offset int) int {
	for i, rec := range records {
		if rec.offset == offset {
			return i
		}
	}
	return -1
}

func cellRow(body []byte) int { return int(binary.LittleEndian.Uint16(body[0:2])) }
func cellCol(body []byte) int { return int(binary.LittleEndian.Uint16(body[2:4])) }

// readMulRK expands the run of RK values one record can carry.
func readMulRK(body []byte, put func(row, col int, value string)) {
	if len(body) < 6 {
		return
	}
	row := cellRow(body)
	col := cellCol(body)
	for i := 4; i+6 <= len(body)-2; i += 6 {
		put(row, col, formatBiffNumber(decodeRK(binary.LittleEndian.Uint32(body[i+2:i+6]))))
		col++
	}
}

// decodeRK expands Excel's packed 30-bit number.
func decodeRK(rk uint32) float64 {
	var value float64
	if rk&0x02 != 0 {
		value = float64(int32(rk) >> 2)
	} else {
		value = math.Float64frombits(uint64(rk&0xFFFFFFFC) << 32)
	}
	if rk&0x01 != 0 {
		value /= 100
	}
	return value
}

// isStringFormula and isNonNumericFormula read the eight result bytes a FORMULA
// record carries. A result whose last two bytes are 0xFFFF is not a float: the
// first byte says which of string, boolean, error or blank it is.
func isStringFormula(result []byte) bool {
	return len(result) == 8 && result[6] == 0xFF && result[7] == 0xFF && result[0] == 0
}

func isNonNumericFormula(result []byte) bool {
	return len(result) == 8 && result[6] == 0xFF && result[7] == 0xFF
}

// formatBiffNumber renders a cell's number the way the rest of this package
// expects to receive it: the shortest exact decimal, with no thousands
// separator and no currency.
func formatBiffNumber(f float64) string {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}
