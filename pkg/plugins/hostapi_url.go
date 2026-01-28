package plugins

import (
	"fmt"
	"net/url"
	"sort"

	"github.com/dop251/goja"
)

// injectURLNamespace sets up shisho.url with URL utility functions.
// These provide functionality similar to browser APIs that aren't available in Goja.
func injectURLNamespace(vm *goja.Runtime, shishoObj *goja.Object) error {
	urlObj := vm.NewObject()
	if err := shishoObj.Set("url", urlObj); err != nil {
		return fmt.Errorf("failed to set shisho.url: %w", err)
	}

	// encodeURIComponent - encode a string for use in URLs
	urlObj.Set("encodeURIComponent", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.url.encodeURIComponent: string argument is required"))
		}
		str := call.Argument(0).String()
		return vm.ToValue(url.QueryEscape(str))
	})

	// decodeURIComponent - decode a URL-encoded string
	urlObj.Set("decodeURIComponent", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.url.decodeURIComponent: string argument is required"))
		}
		str := call.Argument(0).String()
		decoded, err := url.QueryUnescape(str)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.url.decodeURIComponent: %v", err)))
		}
		return vm.ToValue(decoded)
	})

	// searchParams - convert an object to a query string
	urlObj.Set("searchParams", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 1 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
			panic(vm.ToValue("shisho.url.searchParams: object argument is required"))
		}

		obj := call.Argument(0).ToObject(vm)
		keys := obj.Keys()

		// Sort keys for deterministic output
		sort.Strings(keys)

		values := url.Values{}
		for _, key := range keys {
			val := obj.Get(key)
			if goja.IsUndefined(val) || goja.IsNull(val) {
				continue
			}

			// Handle arrays
			if arr, ok := val.Export().([]interface{}); ok {
				for _, item := range arr {
					values.Add(key, fmt.Sprintf("%v", item))
				}
			} else {
				values.Add(key, val.String())
			}
		}

		return vm.ToValue(values.Encode())
	})

	// parse - parse a URL string into its components
	urlObj.Set("parse", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.url.parse: url argument is required"))
		}

		rawURL := call.Argument(0).String()
		parsed, err := url.Parse(rawURL)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.url.parse: %v", err)))
		}

		result := vm.NewObject()
		result.Set("href", rawURL)                     //nolint:errcheck
		result.Set("protocol", parsed.Scheme)          //nolint:errcheck
		result.Set("host", parsed.Host)                //nolint:errcheck
		result.Set("hostname", parsed.Hostname())      //nolint:errcheck
		result.Set("port", parsed.Port())              //nolint:errcheck
		result.Set("pathname", parsed.Path)            //nolint:errcheck
		result.Set("search", formatSearch(parsed))     //nolint:errcheck
		result.Set("hash", formatHash(parsed))         //nolint:errcheck
		result.Set("username", parsed.User.Username()) //nolint:errcheck

		password, hasPassword := parsed.User.Password()
		if hasPassword {
			result.Set("password", password) //nolint:errcheck
		} else {
			result.Set("password", "") //nolint:errcheck
		}

		// Parse query string into an object
		queryObj := vm.NewObject()
		for key, values := range parsed.Query() {
			if len(values) == 1 {
				queryObj.Set(key, values[0]) //nolint:errcheck
			} else {
				queryObj.Set(key, values) //nolint:errcheck
			}
		}
		result.Set("query", queryObj) //nolint:errcheck

		return result
	})

	return nil
}

// formatSearch returns the query string with leading "?" or empty string.
func formatSearch(u *url.URL) string {
	if u.RawQuery == "" {
		return ""
	}
	return "?" + u.RawQuery
}

// formatHash returns the fragment with leading "#" or empty string.
func formatHash(u *url.URL) string {
	if u.Fragment == "" {
		return ""
	}
	return "#" + u.Fragment
}
