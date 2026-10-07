package plugins

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/dop251/goja"
	"github.com/shishobooks/shisho/pkg/testutils/goconsts"
	"github.com/stretchr/testify/assert"
)

// reconcileHookOrder builds hook order rows from HookTypes, so a
// models.PluginHook* constant that HookTypes never reports gets no order row
// and its plugins never run. Every goja.Value field on Runtime is a hook
// slot; filling them all must report every hook type.
func TestRuntimeHookTypes_CoversEveryPluginHookConstant(t *testing.T) {
	t.Parallel()

	rt := &Runtime{}
	v := reflect.ValueOf(rt).Elem()
	hookValue := reflect.TypeOf((*goja.Value)(nil)).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Type() != hookValue {
			continue
		}
		reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(goja.Undefined()))
	}

	hooks := goconsts.StringsWithPrefix(t, "../models", "PluginHook")
	assert.Subset(t, rt.HookTypes(), hooks,
		"Runtime.HookTypes must report every models.PluginHook* constant")
}
