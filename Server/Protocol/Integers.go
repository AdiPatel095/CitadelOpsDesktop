package Protocol

// plainInt64 parses a plain JSON integer literal, -?(0|[1-9][0-9]*) with at most
// 19 digits, straight from its bytes. It reports false for everything else
// (exponents, decimals, leading zeros, a bare sign, out-of-range values, any
// other byte) and rawInt64 then applies its exact-rational path, so results for
// those forms are unchanged. Every input it accepts is a valid JSON number that
// the exact path would convert to the same int64.
func PlainInt64(raw []byte) (int64, bool) {
	digits := raw
	negative := len(raw) > 0 && raw[0] == '-'
	if negative {
		digits = raw[1:]
	}
	if len(digits) == 0 || len(digits) > 19 || (digits[0] == '0' && len(digits) > 1) {
		return 0, false
	}
	var magnitude uint64
	for _, character := range digits {
		if character < '0' || character > '9' {
			return 0, false
		}
		magnitude = magnitude*10 + uint64(character-'0')
	}
	// Nineteen digits cannot overflow uint64; only int64's range remains to check.
	if negative {
		if magnitude > 1<<63 {
			return 0, false
		}
		return int64(-magnitude), true
	}
	if magnitude > 1<<63-1 {
		return 0, false
	}
	return int64(magnitude), true
}
