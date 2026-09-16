package sheet

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// Numeric coercion for spreadsheet cells.
//
// A supplier price list is not a form. The same column legitimately arrives as
// "1,234.50", "1.234,50", "٢٥٫٥٠", "115.00 ج.م", "42 EGP", "(12.50)" and "35",
// and a parser strict enough for an order total rejects all but the last. So
// this layer decides what the human meant, produces a canonical decimal string
// the strict money parser will accept, and reports every assumption it had to
// make. What it cannot read confidently becomes an error the caller surfaces,
// never a silent zero.

var (
	// ErrNoValue marks an empty or explicitly-absent cell. That is a normal
	// absence, not a malformed value, and must not be reported as an error.
	ErrNoValue = errors.New("sheet: empty value")
	// ErrNotNumeric marks a cell that carries no readable number.
	ErrNotNumeric = errors.New("sheet: value is not a number")
)

// currencyNoise is stripped before parsing. Longer tokens come first so "ج.م"
// is removed before a bare "م" could be.
var currencyNoise = []string{
	"ج.م.", "ج.م", "جنيه مصري", "جنيه", "جم", "egp", "l.e.", "l.e", "le",
	"pound", "pounds", "ر.س", "sar", "usd", "$", "€", "£", "ريال", "دولار",
	"pcs", "قطعة", "قطع", "علبة", "vat", "incl", "excl",
}

// blankTokens are what Excel and its exporters write instead of leaving a cell
// empty. They mean "no value", not "bad value".
var blankTokens = map[string]bool{
	"-": true, "--": true, "n/a": true, "na": true, "#n/a": true,
	"null": true, "nil": true, "none": true, "#value!": true,
	"#div/0!": true, "#ref!": true, "#name?": true, "#num!": true,
	"لا يوجد": true, "غير متاح": true, "لايوجد": true,
}

// Decimal is what Coerce made of a cell.
type Decimal struct {
	// Canonical is the value as a bare decimal string with at most two
	// fraction digits, ready for money.Parse.
	Canonical string
	// Float is the same value as a float64. It is for statistics — deciding
	// whether a column looks like a percentage — and must never reach a price.
	Float float64
	// Rounded is true when the source carried more precision than a
	// NUMERIC(12,2) column holds and the value was rounded half away from zero.
	Rounded bool
	// Percent is true when the source was written as a percentage ("20%").
	Percent bool
	// Scale is how many fraction digits the source actually carried, before
	// rounding. A column whose values are all scale 0 is a count, not a price.
	Scale int
}

// Coerce reads a cell as a decimal number, tolerating the formatting a human or
// an export tool is likely to have applied.
//
// It returns ErrNoValue for an empty cell so the caller can distinguish "the
// supplier left the price out" from "the supplier typed something unreadable".
// Those need different messages.
func Coerce(raw string) (Decimal, error) {
	s := strings.ToLower(CleanCell(NormalizeDigits(raw)))
	if s == "" {
		return Decimal{}, ErrNoValue
	}
	if blankTokens[s] {
		return Decimal{}, ErrNoValue
	}

	for _, tok := range currencyNoise {
		s = strings.ReplaceAll(s, tok, " ")
	}

	percent := strings.Contains(s, "%") || strings.Contains(s, "٪")
	s = strings.NewReplacer(
		"%", "", "٪", "", " ", "", "٬", "", "’", "", "_", "", "'", "",
	).Replace(s)

	// U+066B is the Arabic decimal separator; it always means a decimal point.
	s = strings.ReplaceAll(s, "٫", ".")

	neg := false
	// Accounting exports write a negative as "(12.50)".
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg = true
		s = strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	}
	switch {
	case strings.HasPrefix(s, "-"):
		neg = true
		s = s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}

	s = disambiguateSeparators(s)
	if !isDigits(strings.ReplaceAll(s, ".", "")) {
		return Decimal{}, ErrNotNumeric
	}

	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if intPart == "" {
		intPart = "0"
	}
	if hasFrac && fracPart == "" {
		hasFrac = false
	}
	if !isDigits(intPart) || (hasFrac && !isDigits(fracPart)) {
		return Decimal{}, ErrNotNumeric
	}

	res := Decimal{Percent: percent}
	if hasFrac {
		res.Scale = len(fracPart)
	}
	if hasFrac && len(fracPart) > 2 {
		intPart, fracPart = roundToTwo(intPart, fracPart)
		res.Rounded = true
	}

	// Trim leading zeros so a long run cannot overflow the later parse.
	intPart = strings.TrimLeft(intPart, "0")
	if intPart == "" {
		intPart = "0"
	}

	canonical := intPart
	if hasFrac {
		canonical += "." + fracPart
	}
	if neg && canonical != "0" && canonical != "0.00" {
		canonical = "-" + canonical
	}
	res.Canonical = canonical
	res.Float, _ = strconv.ParseFloat(canonical, 64)
	return res, nil
}

// CoerceInt reads a cell as a whole number, truncating any fraction. Quantities
// arrive as "45", "45.00" and "45 علبة".
func CoerceInt(raw string) (int64, error) {
	d, err := Coerce(raw)
	if err != nil {
		return 0, err
	}
	intPart, _, _ := strings.Cut(d.Canonical, ".")
	n, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, ErrNotNumeric
	}
	return n, nil
}

// disambiguateSeparators decides whether "." and "," are decimal points or
// thousands groupings.
//
// The rule follows how the two conventions differ: whichever separator appears
// last is the decimal one, except that a repeated separator, or a trailing
// group of exactly three digits behind a short head, only ever happens in
// grouping.
func disambiguateSeparators(s string) string {
	lastDot := strings.LastIndex(s, ".")
	lastComma := strings.LastIndex(s, ",")

	switch {
	case lastDot < 0 && lastComma < 0:
		return s

	case lastDot >= 0 && lastComma >= 0:
		// Both present: the rightmost is the decimal separator and the other is
		// grouping. "1.234,50" and "1,234.50" both mean 1234.50.
		if lastComma > lastDot {
			return strings.ReplaceAll(s[:lastComma], ".", "") + "." + s[lastComma+1:]
		}
		return strings.ReplaceAll(s[:lastDot], ",", "") + "." + s[lastDot+1:]

	case lastComma >= 0:
		return resolveSingleSeparator(s, ',')

	default:
		return resolveSingleSeparator(s, '.')
	}
}

// resolveSingleSeparator handles a value using only one separator character.
//
// A repeated separator is always grouping — "1.234.567" is never a decimal. A
// single one is ambiguous at three digits, and the two characters break the tie
// differently:
//
//   - A comma with a three-digit tail is grouping. "1,234" means one thousand
//     two hundred and thirty-four in every file this has been run against; a
//     price of one and a bit written with a decimal comma would be "1,23".
//   - A dot is always a decimal point. Reading "25.005" as twenty-five thousand
//     overstates a price a thousandfold, while misreading a European "1.234" as
//     1.23 is both rarer and far less harmful.
func resolveSingleSeparator(s string, sep byte) string {
	parts := strings.Split(s, string(sep))
	tail := parts[len(parts)-1]

	grouping := len(parts) > 2 ||
		(sep == ',' && len(parts) == 2 && len(tail) == 3 && parts[0] != "")
	if grouping && isDigits(tail) {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], "") + "." + tail
}

// roundToTwo rounds a decimal given as separate integer and fraction strings to
// two fraction digits, half away from zero, carrying into the integer part.
func roundToTwo(intPart, fracPart string) (string, string) {
	keep := fracPart[:2]
	if fracPart[2] < '5' {
		return intPart, keep
	}

	frac, err := strconv.ParseInt(keep, 10, 64)
	if err != nil {
		return intPart, keep
	}
	frac++
	if frac < 100 {
		return intPart, pad2(frac)
	}
	// Carry: .995 rounds to 1.00. Done as a string addition so an integer part
	// longer than int64 does not overflow on the way through.
	return addOne(intPart), "00"
}

func pad2(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}
	return strconv.FormatInt(n, 10)
}

func addOne(s string) string {
	if s == "" {
		return "1"
	}
	b := []byte(s)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < '9' {
			b[i]++
			return string(b)
		}
		b[i] = '0'
	}
	return "1" + string(b)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// DigitsOnly returns the ASCII digits of s, which is how a barcode typed with
// spaces or a leading apostrophe is recovered.
func DigitsOnly(s string) string {
	s = undoSpreadsheetNumber(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range NormalizeDigits(s) {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// scientificCode matches a code Excel has rendered in scientific notation.
var scientificCode = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[eE]\+?[0-9]+$`)

// undoSpreadsheetNumber reverses what a spreadsheet does to a long code.
//
// Excel decides a column of digits is numeric and then renders it as a number:
// a thirteen-digit barcode comes back as "6.22123E+12", and an item code comes
// back as "20203380.0". Both are the cell's rendering rather than its content.
//
// Left alone, the fraction's digits are read as part of the code — "20203380.0"
// yields 202033800, a nine-digit number matching nothing — and the exponent form
// yields a six-digit fragment. Neither fails loudly; both simply never match,
// which is indistinguishable from a product the catalogue does not carry.
func undoSpreadsheetNumber(s string) string {
	s = strings.TrimSpace(NormalizeDigits(s))
	if s == "" {
		return s
	}
	if scientificCode.MatchString(s) {
		if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 && f < 1e18 {
			return strconv.FormatFloat(f, 'f', 0, 64)
		}
	}
	// A trailing ".0" (or ".00") is the float rendering of a whole number. A
	// fraction with any non-zero digit is a real decimal and is left alone.
	if i := strings.IndexByte(s, '.'); i >= 0 {
		if frac := s[i+1:]; frac != "" && strings.Trim(frac, "0") == "" {
			return s[:i]
		}
	}
	return s
}

// ValidGTIN reports whether digits is a GTIN-8/12/13/14 whose check digit is
// correct.
//
// This is the single most decisive signal in column detection. A column of
// thirteen-digit numbers might be a barcode or might be a national ID; a column
// of thirteen-digit numbers that all satisfy the GS1 check digit is a barcode,
// whatever its header says.
func ValidGTIN(digits string) bool {
	switch len(digits) {
	case 8, 12, 13, 14:
	default:
		return false
	}
	sum := 0
	// The check digit is last; weights alternate 3,1 leftwards from it.
	for i := len(digits) - 2; i >= 0; i-- {
		d := int(digits[i] - '0')
		if (len(digits)-2-i)%2 == 0 {
			sum += d * 3
		} else {
			sum += d
		}
	}
	check := (10 - sum%10) % 10
	return check == int(digits[len(digits)-1]-'0')
}

// NormalizeCode folds an identifier for comparison: spreadsheet rendering undone
// first, then case, separators and leading zeros.
//
// Separate from NormalizeKey, which folds a *header* and must keep Arabic
// letters intact. A code is compared against another code, and the ways one
// system writes "PAN-24" and another writes "pan 24" are noise on both sides.
func NormalizeCode(s string) string {
	s = strings.ToLower(strings.TrimSpace(undoSpreadsheetNumber(s)))
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '-', '_', '/', ' ', '.', '\\':
			continue
		}
		b.WriteRune(r)
	}
	out := b.String()
	// Leading zeros are padding an ERP added to align a column, not part of the
	// code — but only where the code is entirely digits, since "0X12" is a code
	// whose first character happens to be a zero.
	if isDigits(out) {
		if trimmed := strings.TrimLeft(out, "0"); trimmed != "" {
			return trimmed
		}
	}
	return out
}
