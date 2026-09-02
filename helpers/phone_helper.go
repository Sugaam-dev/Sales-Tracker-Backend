package helpers

import (
	"errors"
	"strconv"
	"strings"

	"github.com/nyaruka/phonenumbers"
)

// ValidatePhoneNumber validates an international phone number according to country code.
// Strictly rejects national numbers beginning with 0 ("Phone number must not start with 0.").
func ValidatePhoneNumber(phone string, countryCode string) error {
	trimmedPhone := strings.TrimSpace(phone)
	if trimmedPhone == "" {
		return errors.New("Phone number is required.")
	}

	// 1. Strict Leading Zero Check
	// If national number starts with 0 (e.g. 0987654321, 012345), reject immediately.
	// Strip leading '+' if user typed full international number directly into phone field to inspect national digits.
	nationalDigits := trimmedPhone
	if strings.HasPrefix(nationalDigits, "+") {
		// If starts with +, parse to inspect national number
		parsed, err := phonenumbers.Parse(nationalDigits, "")
		if err != nil {
			return errors.New("Invalid phone number.")
		}
		rawNational := strconv.FormatUint(parsed.GetNationalNumber(), 10)
		if strings.HasPrefix(rawNational, "0") {
			return errors.New("Phone number must not start with 0.")
		}
		if !phonenumbers.IsValidNumber(parsed) {
			return errors.New("Invalid phone number for the selected country.")
		}
		return nil
	}

	if strings.HasPrefix(nationalDigits, "0") {
		return errors.New("Phone number must not start with 0.")
	}

	// 2. Resolve Country / Region
	trimmedCC := strings.TrimSpace(countryCode)
	region := "IN" // Default region if empty
	var expectedCountryCode int32

	if trimmedCC != "" {
		cleanCC := strings.TrimPrefix(trimmedCC, "+")
		if codeNum, err := strconv.Atoi(cleanCC); err == nil {
			expectedCountryCode = int32(codeNum)
			r := phonenumbers.GetRegionCodeForCountryCode(codeNum)
			if r != "" && r != "ZZ" {
				region = r
			}
		} else if len(trimmedCC) == 2 {
			region = strings.ToUpper(trimmedCC)
			expectedCountryCode = int32(phonenumbers.GetCountryCodeForRegion(region))
		}
	}

	// 3. Parse and Validate with libphonenumber
	num, err := phonenumbers.Parse(trimmedPhone, region)
	if err != nil {
		return errors.New("Invalid phone number for the selected country.")
	}

	if !phonenumbers.IsValidNumber(num) {
		return errors.New("Invalid phone number for the selected country.")
	}

	// 4. Verify that parsed calling code matches specified country code if given
	if expectedCountryCode > 0 && num.GetCountryCode() != expectedCountryCode {
		return errors.New("Invalid phone number for the selected country.")
	}

	return nil
}
