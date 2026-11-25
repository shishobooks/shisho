package reading

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMovedInReorder(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		before, after []int
		moved         map[int]bool
	}{
		{"unchanged", []int{1, 2, 3}, []int{1, 2, 3}, map[int]bool{}},
		{"one dragged to the top", []int{1, 2, 3, 4}, []int{4, 1, 2, 3}, map[int]bool{4: true}},
		{"one dragged to the end", []int{1, 2, 3, 4}, []int{2, 3, 4, 1}, map[int]bool{1: true}},
		{"two moved", []int{1, 2, 3, 4, 5}, []int{5, 2, 3, 1, 4}, map[int]bool{5: true, 1: true}},
		{"ids only in after are ignored", []int{1, 2}, []int{9, 2, 1}, map[int]bool{2: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.moved, MovedInReorder(tc.before, tc.after))
		})
	}
}
