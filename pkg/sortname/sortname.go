// Package sortname provides functions for generating bibliographic sort names
// following ALA/Library of Congress conventions.
package sortname

import (
	"strings"
)

// TitleArticles are articles to strip from the beginning of titles.
// These are moved to the end (e.g., "The Hobbit" -> "Hobbit, The").
var TitleArticles = []string{
	"The",
	"A",
	"An",
}

// GenerationalSuffixes are preserved in the sort name as they distinguish different people.
var GenerationalSuffixes = []string{
	"Jr.",
	"Jr",
	"Sr.",
	"Sr",
	"Junior",
	"Senior",
	"I",
	"II",
	"III",
	"IV",
	"V",
}

// AcademicSuffixes are stripped from the sort name as they are credentials, not part of the name.
var AcademicSuffixes = []string{
	"PhD",
	"Ph.D",
	"Ph.D.",
	"PsyD",
	"Psy.D",
	"Psy.D.",
	"MD",
	"M.D",
	"M.D.",
	"DO",
	"D.O",
	"D.O.",
	"DDS",
	"D.D.S",
	"D.D.S.",
	"JD",
	"J.D",
	"J.D.",
	"EdD",
	"Ed.D",
	"Ed.D.",
	"LLD",
	"LL.D",
	"LL.D.",
	"MBA",
	"M.B.A",
	"M.B.A.",
	"MS",
	"M.S",
	"M.S.",
	"MA",
	"M.A",
	"M.A.",
	"BA",
	"B.A",
	"B.A.",
	"BS",
	"B.S",
	"B.S.",
	"RN",
	"R.N",
	"R.N.",
	"Esq",
	"Esq.",
}

// Prefixes are honorifics/titles that are stripped from the sort name.
var Prefixes = []string{
	"Dr.",
	"Dr",
	"Mr.",
	"Mr",
	"Mrs.",
	"Mrs",
	"Ms.",
	"Ms",
	"Prof.",
	"Prof",
	"Rev.",
	"Rev",
	"Fr.",
	"Fr",
	"Sir",
	"Dame",
	"Lord",
	"Lady",
}

// Particles are name particles that are moved to the end with the given name (library style).
// Example: "Ludwig van Beethoven" -> "Beethoven, Ludwig van".
var Particles = []string{
	"van",
	"von",
	"de",
	"da",
	"di",
	"du",
	"del",
	"della",
	"la",
	"le",
	"el",
	"al",
	"bin",
	"ibn",
}

// ForTitle generates a sort title from a display title.
// Leading articles are moved to the end.
// Examples:
//   - "The Hobbit" -> "Hobbit, The"
//   - "A Tale of Two Cities" -> "Tale of Two Cities, A"
//   - "An American Tragedy" -> "American Tragedy, An"
//   - "Lord of the Rings" -> "Lord of the Rings" (no change)
func ForTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}

	for _, article := range TitleArticles {
		prefix := article + " "
		if strings.EqualFold(title[:min(len(prefix), len(title))], prefix) && len(title) > len(prefix) {
			// Extract the actual article from the title (preserving original case)
			actualArticle := title[:len(article)]
			rest := strings.TrimSpace(title[len(prefix):])
			if rest != "" {
				return rest + ", " + actualArticle
			}
		}
	}

	return title
}

// ForPerson generates a sort name from a person's display name.
// The name is converted to "Last, First Middle" format with proper handling of:
//   - Prefixes (Dr., Mr., etc.) - stripped
//   - Academic suffixes (PhD, MD, etc.) - stripped
//   - Generational suffixes (Jr., III, etc.) - preserved
//   - Particles (van, von, de, etc.) - moved to end with given name
//
// Examples:
//   - "Stephen King" -> "King, Stephen"
//   - "Martin Luther King Jr." -> "King, Martin Luther, Jr."
//   - "Jane Doe PhD" -> "Doe, Jane"
//   - "Dr. Sarah Connor" -> "Connor, Sarah"
//   - "Ludwig van Beethoven" -> "Beethoven, Ludwig van"
func ForPerson(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}

	// Single word name - return as is
	if len(parts) == 1 {
		return name
	}

	// Strip prefixes from the beginning
	for len(parts) > 1 && isPrefix(parts[0]) {
		parts = parts[1:]
	}

	if len(parts) == 0 {
		return name // All parts were prefixes, return original
	}

	if len(parts) == 1 {
		return parts[0]
	}

	// Extract and strip academic suffixes from the end, preserve generational suffixes
	var generationalSuffixes []string
	for len(parts) > 1 {
		last := parts[len(parts)-1]
		if isGenerationalSuffix(last) {
			generationalSuffixes = append([]string{last}, generationalSuffixes...)
			parts = parts[:len(parts)-1]
		} else if isAcademicSuffix(last) {
			parts = parts[:len(parts)-1]
		} else {
			break
		}
	}

	if len(parts) == 0 {
		return name // All parts were suffixes, return original
	}

	if len(parts) == 1 {
		// Only one part left after stripping, add back generational suffixes if any
		if len(generationalSuffixes) > 0 {
			return parts[0] + ", " + strings.Join(generationalSuffixes, ", ")
		}
		return parts[0]
	}

	// Find particles and determine the surname
	// Particles are words like "van", "von", "de" that precede the surname
	// In library style, the particle moves to the end with the given name
	// "Ludwig van Beethoven" -> surname is "Beethoven", given is "Ludwig van"

	// Find the last word (surname)
	surname := parts[len(parts)-1]
	givenParts := parts[:len(parts)-1]

	// Check if there are particles before the surname
	// Collect consecutive particles at the end of givenParts
	var particleParts []string
	for len(givenParts) > 0 {
		last := givenParts[len(givenParts)-1]
		if isParticle(last) {
			particleParts = append([]string{last}, particleParts...)
			givenParts = givenParts[:len(givenParts)-1]
		} else {
			break
		}
	}

	// Build the sort name
	var result strings.Builder
	result.WriteString(surname)

	// Add given name parts
	if len(givenParts) > 0 || len(particleParts) > 0 {
		result.WriteString(", ")
		if len(givenParts) > 0 {
			result.WriteString(strings.Join(givenParts, " "))
		}
		if len(particleParts) > 0 {
			if len(givenParts) > 0 {
				result.WriteString(" ")
			}
			result.WriteString(strings.Join(particleParts, " "))
		}
	}

	// Add generational suffixes
	if len(generationalSuffixes) > 0 {
		result.WriteString(", ")
		result.WriteString(strings.Join(generationalSuffixes, ", "))
	}

	return result.String()
}

// isPrefix checks if a word is a name prefix (case-insensitive).
func isPrefix(word string) bool {
	for _, prefix := range Prefixes {
		if strings.EqualFold(word, prefix) {
			return true
		}
	}
	return false
}

// isGenerationalSuffix checks if a word is a generational suffix (case-insensitive).
func isGenerationalSuffix(word string) bool {
	// Remove trailing comma if present
	word = strings.TrimSuffix(word, ",")
	for _, suffix := range GenerationalSuffixes {
		if strings.EqualFold(word, suffix) {
			return true
		}
	}
	return false
}

// isAcademicSuffix checks if a word is an academic suffix (case-insensitive).
func isAcademicSuffix(word string) bool {
	// Remove trailing comma if present
	word = strings.TrimSuffix(word, ",")
	for _, suffix := range AcademicSuffixes {
		if strings.EqualFold(word, suffix) {
			return true
		}
	}
	return false
}

// isParticle checks if a word is a name particle (case-insensitive).
func isParticle(word string) bool {
	for _, particle := range Particles {
		if strings.EqualFold(word, particle) {
			return true
		}
	}
	return false
}
