package roles

import (
	"testing"

	"github.com/shishobooks/shisho/pkg/testutils/goconsts"
	"github.com/stretchr/testify/assert"
)

// A permission resource missing from ValidResources makes every role that
// grants it fail validation, so each models.Resource* constant must be listed.
func TestValidResources_CoversEveryResourceConstant(t *testing.T) {
	t.Parallel()

	resources := goconsts.StringsWithPrefix(t, "../models", "Resource")
	assert.Subset(t, ValidResources, resources,
		"add every models.Resource* constant to roles.ValidResources")
}
