package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dop251/goja"
	"github.com/pkg/errors"
)

// errDomainNotAllowed is returned when a request targets a domain not in the allowed list.
var errDomainNotAllowed = errors.New("domain is not in the allowed domains list")

// injectHTTPNamespace sets up shisho.http.fetch on the given shisho object.
// It validates requests against the plugin's declared httpAccess.domains list.
func injectHTTPNamespace(vm *goja.Runtime, shishoObj *goja.Object, rt *Runtime) error {
	httpObj := vm.NewObject()
	if err := shishoObj.Set("http", httpObj); err != nil {
		return fmt.Errorf("failed to set shisho.http: %w", err)
	}

	httpObj.Set("fetch", func(call goja.FunctionCall) goja.Value { //nolint:errcheck
		// Parse arguments
		if len(call.Arguments) < 1 {
			panic(vm.ToValue("shisho.http.fetch: url argument is required"))
		}
		rawURL := call.Argument(0).String()

		// Parse options
		method := "GET"
		var headers map[string]string
		var body string
		var hasBody bool

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
			optsObj := call.Argument(1).ToObject(vm)
			if m := optsObj.Get("method"); m != nil && !goja.IsUndefined(m) {
				method = strings.ToUpper(m.String())
			}
			if h := optsObj.Get("headers"); h != nil && !goja.IsUndefined(h) {
				headers = make(map[string]string)
				hObj := h.ToObject(vm)
				for _, key := range hObj.Keys() {
					headers[key] = hObj.Get(key).String()
				}
			}
			if b := optsObj.Get("body"); b != nil && !goja.IsUndefined(b) {
				body = b.String()
				hasBody = true
			}
		}

		// Parse and validate the URL
		parsedURL, err := url.Parse(rawURL)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.http.fetch: invalid URL: %v", err)))
		}
		if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
			panic(vm.ToValue("shisho.http.fetch: only http and https schemes are supported"))
		}

		// Check httpAccess capability
		if rt.manifest.Capabilities.HTTPAccess == nil {
			panic(vm.ToValue("shisho.http.fetch: plugin does not declare httpAccess capability"))
		}
		allowedDomains := rt.manifest.Capabilities.HTTPAccess.Domains

		// Validate the request domain
		if err := validateDomain(parsedURL.Host, allowedDomains); err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.http.fetch: %v", err)))
		}

		// Build the request
		var bodyReader io.Reader
		if hasBody {
			bodyReader = strings.NewReader(body)
		}
		req, err := http.NewRequest(method, rawURL, bodyReader) //nolint:noctx
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.http.fetch: failed to create request: %v", err)))
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		// Create client with redirect validation
		client := &http.Client{
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				if err := validateDomain(req.URL.Host, allowedDomains); err != nil {
					return fmt.Errorf("redirect blocked: %w", err)
				}
				return nil
			},
		}

		// Execute request
		resp, err := client.Do(req)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.http.fetch: request failed: %v", err)))
		}
		defer resp.Body.Close()

		// Buffer the body
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("shisho.http.fetch: failed to read response body: %v", err)))
		}

		// Build response headers as a plain object
		respHeaders := vm.NewObject()
		for key := range resp.Header {
			respHeaders.Set(strings.ToLower(key), resp.Header.Get(key)) //nolint:errcheck
		}

		// Build the response object
		responseObj := vm.NewObject()
		responseObj.Set("ok", resp.StatusCode >= 200 && resp.StatusCode <= 299) //nolint:errcheck
		responseObj.Set("status", resp.StatusCode)                              //nolint:errcheck
		responseObj.Set("statusText", http.StatusText(resp.StatusCode))         //nolint:errcheck
		responseObj.Set("headers", respHeaders)                                 //nolint:errcheck
		responseObj.Set("text", func(_ goja.FunctionCall) goja.Value {          //nolint:errcheck
			return vm.ToValue(string(bodyBytes))
		})
		responseObj.Set("arrayBuffer", func(_ goja.FunctionCall) goja.Value { //nolint:errcheck
			return vm.ToValue(vm.NewArrayBuffer(bodyBytes))
		})
		responseObj.Set("json", func(_ goja.FunctionCall) goja.Value { //nolint:errcheck
			var result interface{}
			if err := json.Unmarshal(bodyBytes, &result); err != nil {
				panic(vm.ToValue(fmt.Sprintf("response.json(): failed to parse JSON: %v", err)))
			}
			return vm.ToValue(result)
		})

		return responseObj
	})

	return nil
}

// validateDomain checks whether the given host (with optional port) is allowed
// by the plugin's declared domains list.
//
// Rules:
//   - Exact match: "example.com" only allows "example.com"
//   - Wildcard match: "*.example.com" allows "example.com", "api.example.com", "a.b.example.com"
//   - Only standard ports (80 for http, 443 for https) are allowed unless the
//     domain is listed with an explicit port (e.g., example.com:8080)
//   - Host comparison is case-insensitive
func validateDomain(host string, allowedDomains []string) error {
	// Normalize host to lowercase
	host = strings.ToLower(host)

	// Split host into hostname and port
	hostname, port := splitHostPort(host)

	for _, allowed := range allowedDomains {
		allowed = strings.ToLower(allowed)
		allowedHostname, allowedPort := splitHostPort(allowed)

		if !matchDomainPattern(hostname, allowedHostname) {
			continue
		}

		// If the allowed domain has an explicit port, the request port must match
		if allowedPort != "" {
			if port == allowedPort {
				return nil
			}
			continue
		}

		// Allowed domain has no explicit port: only standard ports (or no port) are allowed
		if port == "" || port == "80" || port == "443" {
			return nil
		}
	}

	return errors.Wrapf(errDomainNotAllowed, "domain %q", host)
}

// matchDomainPattern checks if hostname matches the pattern.
//
// Patterns:
//   - "example.com" - exact match only
//   - "*.example.com" - matches "example.com", "api.example.com", "a.b.example.com"
func matchDomainPattern(hostname, pattern string) bool {
	// Check for wildcard pattern
	if strings.HasPrefix(pattern, "*.") {
		baseDomain := pattern[2:] // Remove "*." prefix

		// Match the naked domain itself
		if hostname == baseDomain {
			return true
		}

		// Match any subdomain: hostname must end with ".baseDomain"
		if strings.HasSuffix(hostname, "."+baseDomain) {
			return true
		}

		return false
	}

	// Exact match (no wildcard)
	return hostname == pattern
}

// splitHostPort splits a host string into hostname and port.
// Unlike net.SplitHostPort, this handles the case where no port is present.
func splitHostPort(host string) (hostname, port string) {
	// Handle IPv6 addresses in brackets
	if strings.HasPrefix(host, "[") {
		closeBracket := strings.LastIndex(host, "]")
		if closeBracket == -1 {
			return host, ""
		}
		if closeBracket+1 < len(host) && host[closeBracket+1] == ':' {
			return host[:closeBracket+1], host[closeBracket+2:]
		}
		return host[:closeBracket+1], ""
	}

	// For non-IPv6, check if there's a colon for port
	lastColon := strings.LastIndex(host, ":")
	if lastColon == -1 {
		return host, ""
	}
	return host[:lastColon], host[lastColon+1:]
}
