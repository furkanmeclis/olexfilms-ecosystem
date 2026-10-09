package ubl

// go-ubltr does not check VKN/TCKN check digits, so the mapper does
// (otopoly-go billing/invoice/mapper.go pattern).

// FinalConsumerTCKN is the GİB placeholder TCKN for an e-Arşiv buyer whose
// identity number is unknown. It does not pass the TCKN checksum.
const FinalConsumerTCKN = "11111111111"

// ValidVKN reports whether s is a 10-digit tax number with a valid check digit.
func ValidVKN(s string) bool {
	if len(s) != 10 || !allDigits(s) {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		v := (int(s[i]-'0') + 9 - i) % 10
		if v != 9 {
			v = (v << (9 - i)) % 9
		}
		sum += v
	}
	return (10-sum%10)%10 == int(s[9]-'0')
}

// ValidTCKN reports whether s is an 11-digit Turkish identity number with
// valid check digits. FinalConsumerTCKN is accepted.
func ValidTCKN(s string) bool {
	if s == FinalConsumerTCKN {
		return true
	}
	if len(s) != 11 || !allDigits(s) || s[0] == '0' {
		return false
	}
	d := func(i int) int { return int(s[i] - '0') }
	odd := d(0) + d(2) + d(4) + d(6) + d(8)
	even := d(1) + d(3) + d(5) + d(7)
	if ((odd*7-even)%10+10)%10 != d(9) {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		sum += d(i)
	}
	return sum%10 == d(10)
}

// ValidateTaxID checks a 10-digit VKN or an 11-digit TCKN; the error is a
// 422 *Error with CodeInvalidTaxID.
func ValidateTaxID(field, id string) error {
	switch len(id) {
	case 10:
		if !ValidVKN(id) {
			return taxIDError(field, "VKN check digit does not match")
		}
	case 11:
		if !ValidTCKN(id) {
			return taxIDError(field, "TCKN check digits do not match")
		}
	default:
		return taxIDError(field, "must be a 10-digit VKN or an 11-digit TCKN")
	}
	return nil
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
