package api

// ordinalSuffix is the ordinal mark that follows a day count in the page
// language: "st", "nd", "rd", "th" in English, "." in German and so on. Only the
// suffix is language-specific, and it is a fixed rule rather than copy, so it
// lives here instead of in the locale files. The number is printed by the
// template so the suffix can be set smaller beside it.
func ordinalSuffix(language string, n int) string {
	switch language {
	case "de":
		return "."
	case "es":
		return "º"
	case "it":
		return "°"
	case "fr":
		if n == 1 {
			return "er"
		}
		return "e"
	case "ru":
		return "-й"
	default:
		return englishOrdinalSuffix(n)
	}
}

func englishOrdinalSuffix(n int) string {
	if n%100 >= 11 && n%100 <= 13 {
		return "th"
	}
	switch n % 10 {
	case 1:
		return "st"
	case 2:
		return "nd"
	case 3:
		return "rd"
	default:
		return "th"
	}
}
