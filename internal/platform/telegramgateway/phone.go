package telegramgateway

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	egyptianNational = regexp.MustCompile(`^0(1[0125]\d{8})$`)
	egyptianNoPlus   = regexp.MustCompile(`^20(1[0125]\d{8})$`)
	egyptianPlus     = regexp.MustCompile(`^\+20(1[0125]\d{8})$`)
	e164Regex        = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)
)

// --- Phone Number Normalization ---

// NormalizePhone converts Egyptian and international numbers into canonical E.164 format (+201...).
func NormalizePhone(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("empty phone number")
	}

	// 1. Convert Arabic-Indic digits (٠١٢٣٤٥٦٧٨٩) to standard ASCII digits (0-9)
	var sb strings.Builder
	for _, r := range input {
		switch {
		case r >= '٠' && r <= '٩':
			sb.WriteRune('0' + (r - '٠'))
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r == '+':
			if sb.Len() == 0 {
				sb.WriteRune('+')
			}
		}
	}
	clean := sb.String()

	// 2. Remove leading "00" and replace with "+"
	if strings.HasPrefix(clean, "00") {
		clean = "+" + clean[2:]
	}

	// 3. Handle Egyptian national format: e.g. 01012345678, 011..., 012..., 015... (11 digits)
	if m := egyptianNational.FindStringSubmatch(clean); len(m) > 1 {
		return "+20" + m[1], nil
	}

	// 4. Handle Egyptian with country code without plus: 201012345678 (12 digits)
	if m := egyptianNoPlus.FindStringSubmatch(clean); len(m) > 1 {
		return "+20" + m[1], nil
	}

	// 5. Handle Egyptian with country code with plus: +201012345678 (13 chars)
	if m := egyptianPlus.FindStringSubmatch(clean); len(m) > 1 {
		return "+20" + m[1], nil
	}

	// 6. Generic international E.164: + followed by 8 to 15 digits
	if e164Regex.MatchString(clean) {
		return clean, nil
	}

	return "", fmt.Errorf("unsupported phone format: %q", input)
}
