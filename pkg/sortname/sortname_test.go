package sortname

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForTitle(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic article handling
		{
			name:     "The at beginning",
			input:    "The Hobbit",
			expected: "Hobbit, The",
		},
		{
			name:     "A at beginning",
			input:    "A Tale of Two Cities",
			expected: "Tale of Two Cities, A",
		},
		{
			name:     "An at beginning",
			input:    "An American Tragedy",
			expected: "American Tragedy, An",
		},

		// Case insensitivity
		{
			name:     "the lowercase",
			input:    "the hobbit",
			expected: "hobbit, the",
		},
		{
			name:     "THE uppercase",
			input:    "THE HOBBIT",
			expected: "HOBBIT, THE",
		},

		// No article
		{
			name:     "no article",
			input:    "Lord of the Rings",
			expected: "Lord of the Rings",
		},
		{
			name:     "article in middle only",
			input:    "Return of the King",
			expected: "Return of the King",
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "just The",
			input:    "The",
			expected: "The",
		},
		{
			name:     "The with whitespace",
			input:    "The ",
			expected: "The",
		},
		{
			name:     "single word no article",
			input:    "Dune",
			expected: "Dune",
		},

		// Real world examples
		{
			name:     "The Lord of the Rings",
			input:    "The Lord of the Rings",
			expected: "Lord of the Rings, The",
		},
		{
			name:     "A Game of Thrones",
			input:    "A Game of Thrones",
			expected: "Game of Thrones, A",
		},
		{
			name:     "The Great Gatsby",
			input:    "The Great Gatsby",
			expected: "Great Gatsby, The",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ForTitle(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestForPerson(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic name inversion
		{
			name:     "simple two-part name",
			input:    "Stephen King",
			expected: "King, Stephen",
		},
		{
			name:     "three-part name",
			input:    "Martin Luther King",
			expected: "King, Martin Luther",
		},

		// Generational suffixes (preserved)
		{
			name:     "Jr suffix",
			input:    "Robert Downey Jr.",
			expected: "Downey, Robert, Jr.",
		},
		{
			name:     "Jr without period",
			input:    "Robert Downey Jr",
			expected: "Downey, Robert, Jr",
		},
		{
			name:     "Sr suffix",
			input:    "John Smith Sr.",
			expected: "Smith, John, Sr.",
		},
		{
			name:     "III suffix",
			input:    "John Smith III",
			expected: "Smith, John, III",
		},
		{
			name:     "II suffix",
			input:    "Robert Kennedy II",
			expected: "Kennedy, Robert, II",
		},
		{
			name:     "Martin Luther King Jr",
			input:    "Martin Luther King Jr.",
			expected: "King, Martin Luther, Jr.",
		},

		// Academic suffixes (stripped)
		{
			name:     "PhD suffix",
			input:    "Jane Doe PhD",
			expected: "Doe, Jane",
		},
		{
			name:     "Ph.D. suffix",
			input:    "Jane Doe Ph.D.",
			expected: "Doe, Jane",
		},
		{
			name:     "MD suffix",
			input:    "John Smith MD",
			expected: "Smith, John",
		},
		{
			name:     "M.D. suffix",
			input:    "John Smith M.D.",
			expected: "Smith, John",
		},
		{
			name:     "PsyD suffix",
			input:    "Sarah Connor PsyD",
			expected: "Connor, Sarah",
		},
		{
			name:     "MBA suffix",
			input:    "Bob Jones MBA",
			expected: "Jones, Bob",
		},
		{
			name:     "multiple academic suffixes",
			input:    "John Doe MD PhD",
			expected: "Doe, John",
		},

		// Mixed generational and academic (generational preserved, academic stripped)
		{
			name:     "Jr and PhD",
			input:    "John Smith Jr. PhD",
			expected: "Smith, John, Jr.",
		},

		// Prefixes (stripped)
		{
			name:     "Dr prefix",
			input:    "Dr. Sarah Connor",
			expected: "Connor, Sarah",
		},
		{
			name:     "Dr without period",
			input:    "Dr Sarah Connor",
			expected: "Connor, Sarah",
		},
		{
			name:     "Mr prefix",
			input:    "Mr. John Smith",
			expected: "Smith, John",
		},
		{
			name:     "Mrs prefix",
			input:    "Mrs. Jane Doe",
			expected: "Doe, Jane",
		},
		{
			name:     "Prof prefix",
			input:    "Prof. Albert Einstein",
			expected: "Einstein, Albert",
		},
		{
			name:     "Sir prefix",
			input:    "Sir Isaac Newton",
			expected: "Newton, Isaac",
		},

		// Prefix and suffix combined
		{
			name:     "Dr with PhD",
			input:    "Dr. John Smith PhD",
			expected: "Smith, John",
		},
		{
			name:     "Prof with multiple degrees",
			input:    "Prof. Jane Doe MD PhD",
			expected: "Doe, Jane",
		},

		// Particles (moved to end)
		{
			name:     "van Beethoven",
			input:    "Ludwig van Beethoven",
			expected: "Beethoven, Ludwig van",
		},
		{
			name:     "von Neumann",
			input:    "John von Neumann",
			expected: "Neumann, John von",
		},
		{
			name:     "da Vinci",
			input:    "Leonardo da Vinci",
			expected: "Vinci, Leonardo da",
		},
		{
			name:     "de Gaulle",
			input:    "Charles de Gaulle",
			expected: "Gaulle, Charles de",
		},
		{
			name:     "del Toro",
			input:    "Guillermo del Toro",
			expected: "Toro, Guillermo del",
		},
		{
			name:     "van der Waals",
			input:    "Johannes van der Waals",
			expected: "Waals, Johannes van der",
		},

		// Particle with suffix
		{
			name:     "van with Jr",
			input:    "John van Smith Jr.",
			expected: "Smith, John van, Jr.",
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: "",
		},
		{
			name:     "single name",
			input:    "Madonna",
			expected: "Madonna",
		},
		{
			name:     "single name with whitespace",
			input:    "  Cher  ",
			expected: "Cher",
		},

		// Real world examples
		{
			name:     "J.R.R. Tolkien",
			input:    "J.R.R. Tolkien",
			expected: "Tolkien, J.R.R.",
		},
		{
			name:     "George R.R. Martin",
			input:    "George R.R. Martin",
			expected: "Martin, George R.R.",
		},
		{
			name:     "H.P. Lovecraft",
			input:    "H.P. Lovecraft",
			expected: "Lovecraft, H.P.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ForPerson(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsPrefix(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"Dr.", true},
		{"Dr", true},
		{"dr.", true},
		{"DR", true},
		{"Mr.", true},
		{"Mrs.", true},
		{"Prof.", true},
		{"Sir", true},
		{"John", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			assert.Equal(t, tt.expected, isPrefix(tt.word))
		})
	}
}

func TestIsGenerationalSuffix(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"Jr.", true},
		{"Jr", true},
		{"jr.", true},
		{"JR", true},
		{"Sr.", true},
		{"III", true},
		{"iii", true},
		{"II", true},
		{"IV", true},
		{"V", true},
		{"Junior", true},
		{"Senior", true},
		{"PhD", false},
		{"John", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			assert.Equal(t, tt.expected, isGenerationalSuffix(tt.word))
		})
	}
}

func TestIsAcademicSuffix(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"PhD", true},
		{"Ph.D.", true},
		{"phd", true},
		{"MD", true},
		{"M.D.", true},
		{"PsyD", true},
		{"Psy.D.", true},
		{"MBA", true},
		{"Esq.", true},
		{"Jr.", false},
		{"III", false},
		{"John", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			assert.Equal(t, tt.expected, isAcademicSuffix(tt.word))
		})
	}
}

func TestIsParticle(t *testing.T) {
	tests := []struct {
		word     string
		expected bool
	}{
		{"van", true},
		{"Van", true},
		{"VAN", true},
		{"von", true},
		{"de", true},
		{"da", true},
		{"del", true},
		{"della", true},
		{"la", true},
		{"le", true},
		{"bin", true},
		{"ibn", true},
		{"John", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			assert.Equal(t, tt.expected, isParticle(tt.word))
		})
	}
}
