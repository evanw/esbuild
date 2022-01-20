package helpers

import "unicode/utf8"

type TypoDetector struct {
	oneCharTypos map[string]string
}

func MakeTypoDetector(valid []string) TypoDetector {
	detector := TypoDetector{oneCharTypos: make(map[string]string)}

	// Add all combinations of each valid word with one character missing
	for _, correct := range valid {
		if len(correct) > 3 {
			for i, ch := range correct {
				detector.oneCharTypos[correct[:i]+correct[i+utf8.RuneLen(ch):]] = correct
			}
		}
	}

	return detector
}

func (detector TypoDetector) MaybeCorrectTypo(typo string) (string, bool) {
	// Check for a single deleted character
	if corrected, ok := detector.oneCharTypos[typo]; ok {
		return corrected, true
	}

	// Check for a single misplaced character
	for i, ch := range typo {
		if corrected, ok := detector.oneCharTypos[typo[:i]+typo[i+utf8.RuneLen(ch):]]; ok {
			return corrected, true
		}
	}

	return "", false
}
