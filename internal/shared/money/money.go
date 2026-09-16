// Package money represents monetary amounts exactly, as integer minor units.
//
// The legacy database stores every monetary value as DECIMAL(p,2) (106 columns;
// the only float columns are geographic coordinates). That precision is not
// negotiable: a marketplace that loses a piastre per line item loses vendor
// trust. Money therefore never touches float64 anywhere in this codebase.
//
// Amounts are int64 minor units (piastres for EGP). int64 covers ~92 quadrillion
// minor units, which is nine orders of magnitude beyond anything this platform
// will transact.
package money

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Scale is the number of decimal places. Every legacy money column is
// DECIMAL(p,2), so this is 2 and changing it is a migration, not a config flag.
const Scale = 2

var scaleFactor = int64(math.Pow10(Scale))

var (
	ErrInvalidFormat = errors.New("money: invalid decimal format")
	ErrOverflow      = errors.New("money: value out of range")
	ErrCurrency      = errors.New("money: currency mismatch")
)

// Amount is a monetary value in minor units. The zero value is a valid zero
// amount, which makes it safe to use as a struct field without initialisation.
type Amount struct {
	minor int64
}

// FromMinor builds an Amount from minor units (piastres).
func FromMinor(m int64) Amount { return Amount{minor: m} }

// FromMajor builds an Amount from whole major units (pounds).
func FromMajor(m int64) Amount { return Amount{minor: m * scaleFactor} }

// Zero is the additive identity.
var Zero = Amount{}

// Parse reads a decimal string such as "1234.56" or "-0.05".
//
// It deliberately does not accept scientific notation, thousands separators, or
// more than Scale decimal places: every one of those would represent a value the
// database cannot store, and silently rounding it is how money goes missing.
func Parse(s string) (Amount, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Zero, ErrInvalidFormat
	}

	neg := false
	switch s[0] {
	case '-':
		neg, s = true, s[1:]
	case '+':
		s = s[1:]
	}
	if s == "" {
		return Zero, ErrInvalidFormat
	}

	intPart, fracPart, hasFrac := strings.Cut(s, ".")
	if intPart == "" && !hasFrac {
		return Zero, ErrInvalidFormat
	}
	// Only digits are permitted from here. strconv.ParseInt would otherwise
	// accept a second sign, so "--1" would parse as -1 and then be negated back
	// to a positive 1 — a silent sign flip on a refund or an adjustment.
	if !isDigits(intPart) || (hasFrac && !isDigits(fracPart)) {
		return Zero, ErrInvalidFormat
	}
	if hasFrac && len(fracPart) > Scale {
		// Refuse rather than round. A caller with more precision than the
		// database can hold has a bug we want to surface here, not at 3am in a
		// reconciliation report.
		return Zero, fmt.Errorf("%w: more than %d decimal places", ErrInvalidFormat, Scale)
	}

	var units int64
	if intPart != "" {
		var err error
		if units, err = strconv.ParseInt(intPart, 10, 64); err != nil {
			return Zero, ErrInvalidFormat
		}
	}

	var frac int64
	if hasFrac && fracPart != "" {
		padded := fracPart + strings.Repeat("0", Scale-len(fracPart))
		var err error
		if frac, err = strconv.ParseInt(padded, 10, 64); err != nil {
			return Zero, ErrInvalidFormat
		}
	}

	if units > (math.MaxInt64-frac)/scaleFactor {
		return Zero, ErrOverflow
	}

	minor := units*scaleFactor + frac
	if neg {
		minor = -minor
	}
	return Amount{minor: minor}, nil
}

// MustParse is Parse for compile-time constants and tests. It panics on error.
func MustParse(s string) Amount {
	a, err := Parse(s)
	if err != nil {
		panic(fmt.Sprintf("money.MustParse(%q): %v", s, err))
	}
	return a
}

// Minor returns the raw minor units, for storage and arithmetic in callers that
// need it (for example proportional allocation).
func (a Amount) Minor() int64 { return a.minor }

// String renders the canonical decimal form, always with Scale decimal places.
// This is what gets written to NUMERIC columns and shown to users.
func (a Amount) String() string {
	minor := a.minor
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	return fmt.Sprintf("%s%d.%0*d", sign, minor/scaleFactor, Scale, minor%scaleFactor)
}

// FormatWithCommas renders the decimal form with thousands separators, e.g. "1,234,567.89".
func (a Amount) FormatWithCommas() string {
	minor := a.minor
	sign := ""
	if minor < 0 {
		sign = "-"
		minor = -minor
	}
	units := minor / scaleFactor
	cents := minor % scaleFactor

	unitsStr := strconv.FormatInt(units, 10)
	n := len(unitsStr)
	if n <= 3 {
		return fmt.Sprintf("%s%s.%0*d", sign, unitsStr, Scale, cents)
	}

	var sb strings.Builder
	sb.WriteString(sign)
	remainder := n % 3
	if remainder > 0 {
		sb.WriteString(unitsStr[:remainder])
		if n > remainder {
			sb.WriteByte(',')
		}
	}
	for i := remainder; i < n; i += 3 {
		sb.WriteString(unitsStr[i : i+3])
		if i+3 < n {
			sb.WriteByte(',')
		}
	}
	sb.WriteByte('.')
	sb.WriteString(fmt.Sprintf("%0*d", Scale, cents))
	return sb.String()
}

// FormatIntWithCommas formats an integer with thousands commas, e.g. "1,234".
func FormatIntWithCommas(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	l := len(s)
	if l <= 3 {
		return sign + s
	}
	var sb strings.Builder
	sb.WriteString(sign)
	rem := l % 3
	if rem > 0 {
		sb.WriteString(s[:rem])
		if l > rem {
			sb.WriteByte(',')
		}
	}
	for i := rem; i < l; i += 3 {
		sb.WriteString(s[i : i+3])
		if i+3 < l {
			sb.WriteByte(',')
		}
	}
	return sb.String()
}

func (a Amount) IsZero() bool     { return a.minor == 0 }
func (a Amount) IsNegative() bool { return a.minor < 0 }
func (a Amount) IsPositive() bool { return a.minor > 0 }

// Abs returns the absolute value of the amount.
func (a Amount) Abs() Amount {
	if a.minor < 0 {
		return Amount{minor: -a.minor}
	}
	return a
}

// Add returns a+b, reporting overflow rather than wrapping.
func (a Amount) Add(b Amount) (Amount, error) {
	sum := a.minor + b.minor
	// Overflow iff the operands share a sign that the result does not.
	if (a.minor > 0 && b.minor > 0 && sum < 0) || (a.minor < 0 && b.minor < 0 && sum > 0) {
		return Zero, ErrOverflow
	}
	return Amount{minor: sum}, nil
}

// Sub returns a-b.
func (a Amount) Sub(b Amount) (Amount, error) {
	if b.minor == math.MinInt64 {
		return Zero, ErrOverflow
	}
	return a.Add(Amount{minor: -b.minor})
}

// MulInt scales by a whole number, typically a line quantity.
func (a Amount) MulInt(n int64) (Amount, error) {
	if a.minor != 0 && (n > math.MaxInt64/abs(a.minor) || n < math.MinInt64/abs(a.minor)) {
		return Zero, ErrOverflow
	}
	return Amount{minor: a.minor * n}, nil
}

// ApplyPercent returns the portion of a given by percent, using banker-free
// half-up rounding away from zero, which is what the legacy PHP code produced.
//
// percentBasisPoints is in hundredths of a percent: 1250 means 12.50%. Using
// basis points keeps the discount itself exact, so "12.5% off" does not become
// a float somewhere upstream.
func (a Amount) ApplyPercent(percentBasisPoints int64) Amount {
	const bpsPerWhole = 10000
	num := a.minor * percentBasisPoints
	q := num / bpsPerWhole
	r := num % bpsPerWhole
	if r >= bpsPerWhole/2 {
		q++
	} else if r <= -bpsPerWhole/2 {
		q--
	}
	return Amount{minor: q}
}

// Allocate splits an amount across n parts by integer ratios without losing or
// inventing a single minor unit. Remainders go to the earliest parts, so the sum
// of the result always equals the original exactly.
//
// This is how order totals get distributed across vendor shipments. Doing it by
// percentage and rounding each part independently is the classic way to end up a
// piastre short on the invoice.
func (a Amount) Allocate(ratios []int64) ([]Amount, error) {
	if len(ratios) == 0 {
		return nil, errors.New("money: Allocate requires at least one ratio")
	}
	var total int64
	for _, r := range ratios {
		if r < 0 {
			return nil, errors.New("money: Allocate ratios must be non-negative")
		}
		total += r
	}
	if total == 0 {
		return nil, errors.New("money: Allocate ratios must not sum to zero")
	}

	out := make([]Amount, len(ratios))
	var assigned int64
	for i, r := range ratios {
		share := a.minor * r / total
		out[i] = Amount{minor: share}
		assigned += share
	}

	// Distribute the rounding remainder one unit at a time.
	remainder := a.minor - assigned
	step := int64(1)
	if remainder < 0 {
		step = -1
	}
	for i := 0; remainder != 0; i = (i + 1) % len(out) {
		out[i].minor += step
		remainder -= step
	}
	return out, nil
}

// isDigits reports whether s is non-empty and entirely ASCII digits. An empty
// string is permitted only where the caller has already handled it (".5" has an
// empty integer part, "5." an empty fraction).
func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// Value implements driver.Valuer, writing the canonical decimal string so pgx
// hands PostgreSQL a NUMERIC literal with no float in the path.
func (a Amount) Value() (driver.Value, error) { return a.String(), nil }

// Scan implements sql.Scanner for NUMERIC columns.
func (a *Amount) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*a = Zero
		return nil
	case string:
		parsed, err := Parse(v)
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	case []byte:
		parsed, err := Parse(string(v))
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	case int64:
		*a = FromMajor(v)
		return nil
	default:
		return fmt.Errorf("money: cannot scan %T", src)
	}
}

// MarshalJSON emits a decimal string, never a JSON number. JSON numbers are
// IEEE-754 doubles in most parsers, which would reintroduce the float we spent
// this whole package avoiding.
func (a Amount) MarshalJSON() ([]byte, error) {
	return []byte(`"` + a.String() + `"`), nil
}

func (a *Amount) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "null" {
		*a = Zero
		return nil
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}
