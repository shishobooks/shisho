package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCompatible(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		version    string // current Version
		minVersion string
		want       bool
	}{
		{"empty minVersion", "1.0.0", "", true},
		{"empty Version", "", "1.0.0", true},
		{"dev Version", "dev", "1.0.0", true},
		{"equal versions", "1.0.0", "1.0.0", true},
		{"current newer major", "2.0.0", "1.0.0", true},
		{"current newer minor", "1.2.0", "1.1.0", true},
		{"current newer patch", "1.0.2", "1.0.1", true},
		{"current older major", "1.0.0", "2.0.0", false},
		{"current older minor", "1.0.0", "1.1.0", false},
		{"current older patch", "1.0.0", "1.0.1", false},
		{"with v prefix", "v1.2.0", "1.1.0", true},
		{"both v prefix", "v1.2.0", "v1.3.0", false},
		{"with prerelease", "1.2.0-beta.1", "1.1.0", true},
		{"unparseable current", "abc", "1.0.0", true},
		{"unparseable min", "1.0.0", "abc", true},
		{"partial version", "1.2", "1.1.0", true},
		{"partial min version", "1.2.0", "1.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isVersionCompatible(tt.version, tt.minVersion)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCompare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"equal", "1.2.3", "1.2.3", 0},
		{"major greater", "2.0.0", "1.0.0", 1},
		{"major less", "1.0.0", "2.0.0", -1},
		{"minor greater", "1.2.0", "1.1.0", 1},
		{"minor less", "1.1.0", "1.2.0", -1},
		{"patch greater", "1.0.2", "1.0.1", 1},
		{"patch less", "1.0.1", "1.0.2", -1},
		{"double-digit minor greater than single-digit", "0.10.0", "0.2.0", 1},
		{"v prefix on one side", "v1.2.3", "1.2.3", 0},
		{"v prefix on both", "v2.0.0", "v1.0.0", 1},
		{"prerelease stripped — compares equal to release", "1.0.0-rc1", "1.0.0", 0},
		{"partial version treated as zero", "1.2", "1.2.0", 0},
		{"unparseable a returns 0", "abc", "1.0.0", 0},
		{"unparseable b returns 0", "1.0.0", "abc", 0},
		{"both unparseable returns 0", "abc", "def", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Compare(tt.a, tt.b)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSemver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    string
		want []int
	}{
		{"basic", "1.2.3", []int{1, 2, 3}},
		{"with v prefix", "v1.2.3", []int{1, 2, 3}},
		{"with prerelease", "1.2.3-beta.1", []int{1, 2, 3}},
		{"partial two", "1.2", []int{1, 2, 0}},
		{"partial one", "1", []int{1, 0, 0}},
		{"invalid", "abc", nil},
		{"empty", "", nil},
		{"too many parts", "1.2.3.4", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := parseSemver(tt.s)
			assert.Equal(t, tt.want, got)
		})
	}
}
