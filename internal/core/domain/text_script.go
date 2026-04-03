package domain

import "unicode"

// ContainsJapaneseScript is true if s includes hiragana, katakana, or Han (CJK unified).
func ContainsJapaneseScript(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x3040 && r <= 0x309F: // Hiragana
			return true
		case r >= 0x30A0 && r <= 0x30FF: // Katakana
			return true
		case unicode.Is(unicode.Han, r):
			return true
		}
	}
	return false
}
