package sheet

import (
	"strconv"
	"strings"
	"time"
)

// Date coercion for spreadsheet cells.
//
// An expiry date is the one imported value whose misreading is a safety
// problem, not an accounting one: a batch read as 2026 instead of 2028 is
// pulled from sale, and one read as 2028 instead of 2026 is sold expired. So
// this parser refuses far more than it guesses, and every ambiguity it does
// resolve is reported to the caller.

// DateResult is what CoerceDate made of a cell.
type DateResult struct {
	Time time.Time
	// MonthOnly is true when the source gave a month and year but no day, which
	// pharmaceutical packaging usually does. The day is then set to the last of
	// the month, because a product marked "EXP 11/2027" is good through
	// November.
	MonthOnly bool
	// FromSerial is true when the cell held an Excel serial number rather than
	// text. Those are unambiguous and need no day/month guess.
	FromSerial bool
	// DayMonthSwapped is true when the value was ambiguous between d/m/y and
	// m/d/y and was resolved as day-first, which is the Egyptian convention.
	// The caller warns on these so an admin can check a sample.
	DayMonthSwapped bool
}

// excelEpoch is 1899-12-30: Excel numbers days from 1900-01-01 as serial 1 and
// wrongly believes 1900 was a leap year, so the usable epoch sits two days back.
var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// serialRange bounds a plausible Excel date serial: 1990-01-01 to 2099-12-31.
// Outside it a bare number is a quantity or a code, not a date.
const (
	serialMin = 32874
	serialMax = 73050
)

// monthNames maps the month words that appear in Egyptian supplier files —
// English, abbreviated English, and both Arabic naming systems — onto a number.
var monthNames = map[string]int{
	"jan": 1, "january": 1, "يناير": 1, "كانونالثاني": 1,
	"feb": 2, "february": 2, "فبراير": 2, "شباط": 2,
	"mar": 3, "march": 3, "مارس": 3, "اذار": 3,
	"apr": 4, "april": 4, "ابريل": 4, "نيسان": 4,
	"may": 5, "مايو": 5, "ايار": 5,
	"jun": 6, "june": 6, "يونيو": 6, "حزيران": 6,
	"jul": 7, "july": 7, "يوليو": 7, "تموز": 7,
	"aug": 8, "august": 8, "اغسطس": 8, "اب": 8,
	"sep": 9, "sept": 9, "september": 9, "سبتمبر": 9, "ايلول": 9,
	"oct": 10, "october": 10, "اكتوبر": 10, "تشرينالاول": 10,
	"nov": 11, "november": 11, "نوفمبر": 11, "تشرينالثاني": 11,
	"dec": 12, "december": 12, "ديسمبر": 12, "كانونالاول": 12,
}

// CoerceDate reads a cell as a calendar date.
//
// It accepts, in order of confidence: an Excel serial number; an ISO date; a
// numeric d/m/y or m/d/y in any of the four common separators; a month/year
// pair; and a written month name with a year.
func CoerceDate(raw string) (DateResult, error) {
	s := CleanCell(NormalizeDigits(raw))
	if s == "" {
		return DateResult{}, ErrNoValue
	}
	if blankTokens[strings.ToLower(s)] {
		return DateResult{}, ErrNoValue
	}

	if res, ok := dateFromSerial(s); ok {
		return res, nil
	}
	if res, ok := dateFromMonthName(s); ok {
		return res, nil
	}
	return dateFromParts(s)
}

// dateFromSerial reads the number Excel stores behind a formatted date cell.
func dateFromSerial(s string) (DateResult, bool) {
	// A serial may carry a time fraction; only the whole days matter here.
	whole, _, _ := strings.Cut(s, ".")
	n, err := strconv.Atoi(whole)
	if err != nil || n < serialMin || n > serialMax {
		return DateResult{}, false
	}
	return DateResult{Time: excelEpoch.AddDate(0, 0, n), FromSerial: true}, true
}

// dateFromMonthName reads "Nov 2027", "نوفمبر-27" and "12-Dec-2026".
func dateFromMonthName(s string) (DateResult, bool) {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r == '/' || r == '-' || r == '.' || r == ' ' || r == ','
	})
	month, year, day := 0, 0, 0
	for _, f := range fields {
		if m, ok := monthNames[NormalizeKey(f)]; ok && month == 0 {
			month = m
			continue
		}
		n, err := strconv.Atoi(f)
		if err != nil {
			continue
		}
		switch {
		case n >= 1000:
			if !plausibleYear(n) {
				return DateResult{}, false
			}
			year = n
		case n > 31:
			year = 2000 + n%100
		case n >= 1 && day == 0 && month != 0:
			day = n
		case n >= 1 && year == 0:
			// A bare two-digit number beside a month name is the year far more
			// often than the day in an expiry column ("EXP 11/27").
			year = 2000 + n
		}
	}
	if month == 0 || year == 0 {
		return DateResult{}, false
	}
	if day == 0 {
		return DateResult{Time: endOfMonth(year, month), MonthOnly: true}, true
	}
	if !validDay(year, month, day) {
		return DateResult{}, false
	}
	return DateResult{Time: date(year, month, day)}, true
}

// dateFromParts reads a purely numeric date, deciding day-first versus
// month-first from the values themselves.
func dateFromParts(s string) (DateResult, error) {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == '-' || r == '.' || r == ' ' || r == '\\'
	})
	nums := make([]int, 0, 3)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return DateResult{}, ErrNotNumeric
		}
		nums = append(nums, n)
	}

	switch len(nums) {
	case 2:
		return twoPartDate(nums[0], nums[1])
	case 3:
		return threePartDate(nums[0], nums[1], nums[2], len(parts[0]) == 4)
	default:
		return DateResult{}, ErrNotNumeric
	}
}

// twoPartDate reads "11/2027" and "2027/11" as the end of that month.
//
// The year bound is not cosmetic. A product code written "1785345031-1" splits
// into two numbers of which the second is a legal month, and without a bound on
// the first it parses as January of the year 1,785,345,031 — which is how a
// column of item codes was read as a column of expiry dates.
func twoPartDate(a, b int) (DateResult, error) {
	switch {
	case plausibleYear(a) && b >= 1 && b <= 12:
		return DateResult{Time: endOfMonth(a, b), MonthOnly: true}, nil
	case plausibleYear(b) && a >= 1 && a <= 12:
		return DateResult{Time: endOfMonth(b, a), MonthOnly: true}, nil
	case a >= 1 && a <= 12 && b >= 20 && b <= 99:
		return DateResult{Time: endOfMonth(2000+b, a), MonthOnly: true}, nil
	}
	return DateResult{}, ErrNotNumeric
}

// threePartDate resolves a three-number date. isoFirst says the first field was
// written with four digits, which settles the order without any guessing.
func threePartDate(a, b, c int, isoFirst bool) (DateResult, error) {
	if isoFirst || a >= 1000 {
		if !validDay(a, b, c) {
			return DateResult{}, ErrNotNumeric
		}
		return DateResult{Time: date(a, b, c)}, nil
	}

	year := c
	if year < 100 {
		year += 2000
	}
	if year < 1990 || year > 2099 {
		return DateResult{}, ErrNotNumeric
	}

	switch {
	case a > 12 && b <= 12:
		// Unambiguous day-first.
		if !validDay(year, b, a) {
			return DateResult{}, ErrNotNumeric
		}
		return DateResult{Time: date(year, b, a)}, nil
	case b > 12 && a <= 12:
		// Unambiguous month-first, which is how a US-locale export writes it.
		if !validDay(year, a, b) {
			return DateResult{}, ErrNotNumeric
		}
		return DateResult{Time: date(year, a, b)}, nil
	case a <= 12 && b <= 12:
		// Genuinely ambiguous. Egypt writes day first, so that is the reading,
		// and the flag tells the caller to say so.
		if !validDay(year, b, a) {
			return DateResult{}, ErrNotNumeric
		}
		return DateResult{Time: date(year, b, a), DayMonthSwapped: true}, nil
	}
	return DateResult{}, ErrNotNumeric
}

// plausibleYear bounds a four-digit year to the range a pharmaceutical expiry
// can occupy.
func plausibleYear(y int) bool { return y >= 1990 && y <= 2099 }

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func endOfMonth(y, m int) time.Time {
	return date(y, m, 1).AddDate(0, 1, -1)
}

func validDay(y, m, d int) bool {
	if y < 1990 || y > 2099 || m < 1 || m > 12 || d < 1 || d > 31 {
		return false
	}
	// Reject 31 April and 30 February rather than letting time.Date roll them
	// forward into the next month, which would silently extend an expiry.
	t := date(y, m, d)
	return t.Day() == d && int(t.Month()) == m
}
