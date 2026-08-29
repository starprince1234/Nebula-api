package domain

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const MaxCreditMilli int64 = 1_000_000_000_000

var creditPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,9})(\.[0-9]{1,3})?$`)

func ParseCredits(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if !creditPattern.MatchString(value) {
		return 0, NewError(CodeValidation, "credits must be a decimal value with at most three fractional digits")
	}
	parts := strings.SplitN(value, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, NewError(CodeValidation, "credits are out of range")
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	fraction += strings.Repeat("0", 3-len(fraction))
	fractionValue, _ := strconv.ParseInt(fraction, 10, 64)
	if whole > MaxCreditMilli/1000 {
		return 0, NewError(CodeValidation, "credits are out of range")
	}
	milli := whole*1000 + fractionValue
	if milli > MaxCreditMilli {
		return 0, NewError(CodeValidation, "credits are out of range")
	}
	return milli, nil
}

func FormatCredits(milli int64) string {
	if milli < 0 {
		return "-" + FormatCredits(-milli)
	}
	return fmt.Sprintf("%d.%03d", milli/1000, milli%1000)
}
