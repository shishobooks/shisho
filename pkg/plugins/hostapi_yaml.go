package plugins

import (
	"fmt"

	"github.com/dop251/goja"
	"gopkg.in/yaml.v3"
)

// injectYAMLNamespace sets up shisho.yaml with parse and stringify.
// No file access or capability required — operates on in-memory strings.
//
// Parsing uses gopkg.in/yaml.v3, which does not support arbitrary object
// instantiation via custom tags (unlike PyYAML's full loader or Ruby's
// Psych), so parsing untrusted YAML cannot execute code. Residual DoS risk
// from oversized or deeply-nested inputs is bounded by plugin hook
// timeouts — same risk profile as shisho.xml, shisho.html, and resp.json().
func injectYAMLNamespace(vm *goja.Runtime, shishoObj *goja.Object) error {
	yamlObj := vm.NewObject()
	if err := shishoObj.Set("yaml", yamlObj); err != nil {
		return fmt.Errorf("failed to set shisho.yaml: %w", err)
	}

	yamlObj.Set("parse", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.yaml.parse: content argument is required"))
		}
		content := call.Argument(0).String()

		var parsed interface{}
		if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
			panic(vm.ToValue("shisho.yaml.parse: " + err.Error()))
		}

		return vm.ToValue(normalizeYAMLValue(parsed))
	})

	yamlObj.Set("stringify", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.yaml.stringify: value argument is required"))
		}
		val := call.Argument(0).Export()

		out, err := yaml.Marshal(val)
		if err != nil {
			panic(vm.ToValue("shisho.yaml.stringify: " + err.Error()))
		}
		return vm.ToValue(string(out))
	})

	return nil
}

// normalizeYAMLValue recursively converts yaml.v3's map[interface{}]interface{}
// into map[string]interface{} so goja exposes plain JS objects. yaml.v3 actually
// decodes maps as map[string]interface{} by default when the target is
// interface{}, but non-string keys can still produce map[interface{}]interface{}
// — this walk handles both shapes and also recurses into slices.
func normalizeYAMLValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, inner := range val {
			out[fmt.Sprintf("%v", k)] = normalizeYAMLValue(inner)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, inner := range val {
			out[k] = normalizeYAMLValue(inner)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, inner := range val {
			out[i] = normalizeYAMLValue(inner)
		}
		return out
	default:
		return v
	}
}
