package plugins

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestRuntimeWithHTTPAccess creates a Runtime with a manifest that includes httpAccess.
func newTestRuntimeWithHTTPAccess(domains []string) *Runtime {
	rt := &Runtime{
		vm:       goja.New(),
		scope:    "official",
		pluginID: "test-http-plugin",
		manifest: &Manifest{
			ManifestVersion: 1,
			ID:              "test-http-plugin",
			Name:            "Test HTTP Plugin",
			Version:         "1.0.0",
			Capabilities: Capabilities{
				HTTPAccess: &HTTPAccessCap{
					Description: "Test HTTP access",
					Domains:     domains,
				},
			},
		},
	}
	return rt
}

// newTestRuntimeWithoutHTTPAccess creates a Runtime with no httpAccess capability.
func newTestRuntimeWithoutHTTPAccess() *Runtime {
	rt := &Runtime{
		vm:       goja.New(),
		scope:    "official",
		pluginID: "test-no-http-plugin",
		manifest: &Manifest{
			ManifestVersion: 1,
			ID:              "test-no-http-plugin",
			Name:            "Test No HTTP Plugin",
			Version:         "1.0.0",
			Capabilities:    Capabilities{},
		},
	}
	return rt
}

// setupHTTPNamespace injects the HTTP namespace into the runtime.
func setupHTTPNamespace(t *testing.T, rt *Runtime) {
	t.Helper()
	shishoObj := rt.vm.NewObject()
	err := rt.vm.Set("shisho", shishoObj)
	require.NoError(t, err)
	err = injectHTTPNamespace(rt.vm, shishoObj, rt)
	require.NoError(t, err)
}

func TestHTTPFetch_AllowedDomain(t *testing.T) {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world")) //nolint:errcheck
	}))
	defer ts.Close()

	// Extract host from test server URL (e.g., "127.0.0.1:PORT")
	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	// Call fetch with the test server URL
	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/test");
		JSON.stringify({ok: resp.ok, status: resp.status, text: resp.text()});
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &result)
	require.NoError(t, err)
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, 200, int(result["status"].(float64)))
	assert.Equal(t, "hello world", result["text"])
}

func TestHTTPFetch_BlockedDomain(t *testing.T) {
	rt := newTestRuntimeWithHTTPAccess([]string{"allowed.example.com"})
	setupHTTPNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.http.fetch("https://blocked.example.com/api")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not in the allowed domains list")
}

func TestHTTPFetch_RedirectToBlockedDomain(t *testing.T) {
	// Create a target server (blocked domain)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("should not reach here")) //nolint:errcheck
	}))
	defer target.Close()

	// Create a redirecting server (allowed domain) that redirects to the blocked domain
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/secret", http.StatusFound)
	}))
	defer redirector.Close()

	// Only allow the redirector's domain
	redirectorHost := redirector.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{redirectorHost})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`shisho.http.fetch("%s/start")`, redirector.URL)
	_, err := rt.vm.RunString(script)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirect blocked")
}

func TestHTTPFetch_ResponseJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{ //nolint:errcheck
			"title":  "Test Book",
			"author": "Jane Doe",
			"pages":  300,
		})
	}))
	defer ts.Close()

	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/book");
		var data = resp.json();
		JSON.stringify({title: data.title, author: data.author, pages: data.pages});
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &result)
	require.NoError(t, err)
	assert.Equal(t, "Test Book", result["title"])
	assert.Equal(t, "Jane Doe", result["author"])
	assert.Equal(t, 300, int(result["pages"].(float64)))
}

func TestHTTPFetch_ResponseText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("plain text response")) //nolint:errcheck
	}))
	defer ts.Close()

	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/text");
		resp.text();
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)
	assert.Equal(t, "plain text response", val.String())
}

func TestHTTPFetch_ResponseBytes(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte{0x48, 0x65, 0x6C, 0x6C, 0x6F}) //nolint:errcheck // "Hello"
	}))
	defer ts.Close()

	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/bytes");
		var buf = resp.arrayBuffer();
		// Convert ArrayBuffer to string by reading bytes
		var arr = new Uint8Array(buf);
		var str = "";
		for (var i = 0; i < arr.length; i++) {
			str += String.fromCharCode(arr[i]);
		}
		str;
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)
	assert.Equal(t, "Hello", val.String())
}

func TestHTTPFetch_NoHTTPAccessCapability(t *testing.T) {
	rt := newTestRuntimeWithoutHTTPAccess()
	setupHTTPNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.http.fetch("https://example.com/api")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin does not declare httpAccess capability")
}

func TestHTTPFetch_NonStandardPortBlocked(t *testing.T) {
	// A domain listed without port should block non-standard ports
	rt := newTestRuntimeWithHTTPAccess([]string{"example.com"})
	setupHTTPNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.http.fetch("https://example.com:8080/api")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not in the allowed domains list")
}

func TestHTTPFetch_ExplicitPortAllowed(t *testing.T) {
	// Create a test server on a non-standard port
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("port ok")) //nolint:errcheck
	}))
	defer ts.Close()

	// The test server uses a random non-standard port, allow it explicitly
	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/test");
		resp.text();
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)
	assert.Equal(t, "port ok", val.String())
}

func TestHTTPFetch_StandardPortAllowed(t *testing.T) {
	// A domain listed without port should allow standard ports (80, 443)
	// We can't easily test real 80/443 connections, so test the validateDomain function directly
	err := validateDomain("example.com:443", []string{"example.com"})
	require.NoError(t, err)

	err = validateDomain("example.com:80", []string{"example.com"})
	require.NoError(t, err)

	err = validateDomain("example.com", []string{"example.com"})
	require.NoError(t, err)
}

func TestHTTPFetch_SubdomainNotAllowed(t *testing.T) {
	// "goodreads.com" should NOT allow "api.goodreads.com"
	rt := newTestRuntimeWithHTTPAccess([]string{"goodreads.com"})
	setupHTTPNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.http.fetch("https://api.goodreads.com/search")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not in the allowed domains list")
}

func TestHTTPFetch_PostWithHeaders(t *testing.T) {
	var receivedMethod string
	var receivedContentType string
	var receivedBody string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedContentType = r.Header.Get("Content-Type")
		bodyBytes := make([]byte, r.ContentLength)
		r.Body.Read(bodyBytes) //nolint:errcheck
		receivedBody = string(bodyBytes)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 42}`)) //nolint:errcheck
	}))
	defer ts.Close()

	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/create", {
			method: "POST",
			headers: {"Content-Type": "application/json"},
			body: JSON.stringify({name: "Test"})
		});
		JSON.stringify({ok: resp.ok, status: resp.status});
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)

	assert.Equal(t, "POST", receivedMethod)
	assert.Equal(t, "application/json", receivedContentType)
	assert.Contains(t, receivedBody, `"name"`)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &result)
	require.NoError(t, err)
	assert.Equal(t, true, result["ok"])
	assert.Equal(t, 201, int(result["status"].(float64)))
}

func TestHTTPFetch_ResponseHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Custom-Header", "custom-value")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	defer ts.Close()

	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/headers");
		JSON.stringify({
			customHeader: resp.headers["x-custom-header"],
			contentType: resp.headers["content-type"]
		});
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &result)
	require.NoError(t, err)
	assert.Equal(t, "custom-value", result["customHeader"])
	assert.Equal(t, "text/plain", result["contentType"])
}

func TestHTTPFetch_StatusText(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found")) //nolint:errcheck
	}))
	defer ts.Close()

	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/missing");
		JSON.stringify({ok: resp.ok, status: resp.status, statusText: resp.statusText});
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &result)
	require.NoError(t, err)
	assert.Equal(t, false, result["ok"])
	assert.Equal(t, 404, int(result["status"].(float64)))
	assert.Equal(t, "Not Found", result["statusText"])
}

func TestHTTPFetch_InvalidURL(t *testing.T) {
	rt := newTestRuntimeWithHTTPAccess([]string{"example.com"})
	setupHTTPNamespace(t, rt)

	_, err := rt.vm.RunString(`shisho.http.fetch("ftp://example.com/file")`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only http and https schemes are supported")
}

func TestHTTPFetch_ResponseBodyMultipleReads(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"value"}`)) //nolint:errcheck
	}))
	defer ts.Close()

	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	// Call text() and json() multiple times on the same response to verify buffered body
	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/data");
		var text1 = resp.text();
		var text2 = resp.text();
		var jsonData = resp.json();
		JSON.stringify({text1: text1, text2: text2, jsonKey: jsonData.key});
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal([]byte(val.String()), &result)
	require.NoError(t, err)
	assert.JSONEq(t, `{"key":"value"}`, result["text1"].(string))
	assert.JSONEq(t, `{"key":"value"}`, result["text2"].(string))
	assert.Equal(t, "value", result["jsonKey"])
}

func TestValidateDomain_CaseInsensitive(t *testing.T) {
	err := validateDomain("Example.COM", []string{"example.com"})
	require.NoError(t, err)

	err = validateDomain("example.com", []string{"EXAMPLE.COM"})
	require.NoError(t, err)
}

func TestHTTPFetch_RedirectToAllowedDomain(t *testing.T) {
	// Final target (allowed)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("redirected ok")) //nolint:errcheck
	}))
	defer target.Close()

	// Redirector (allowed) redirects to target (also allowed)
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/final", http.StatusFound)
	}))
	defer redirector.Close()

	// Allow both domains
	redirectorHost := redirector.Listener.Addr().String()
	targetHost := target.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{redirectorHost, targetHost})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/start");
		resp.text();
	`, redirector.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)
	assert.Equal(t, "redirected ok", val.String())
}

// --- Wildcard Domain Matching Tests ---

func TestMatchDomainPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hostname string
		pattern  string
		expected bool
	}{
		// Exact match (no wildcard)
		{
			name:     "exact match",
			hostname: "example.com",
			pattern:  "example.com",
			expected: true,
		},
		{
			name:     "exact match fails for subdomain",
			hostname: "api.example.com",
			pattern:  "example.com",
			expected: false,
		},
		{
			name:     "exact match different domain",
			hostname: "other.com",
			pattern:  "example.com",
			expected: false,
		},

		// Wildcard matches naked domain
		{
			name:     "wildcard matches naked domain",
			hostname: "example.com",
			pattern:  "*.example.com",
			expected: true,
		},

		// Wildcard matches single subdomain
		{
			name:     "wildcard matches single subdomain",
			hostname: "api.example.com",
			pattern:  "*.example.com",
			expected: true,
		},
		{
			name:     "wildcard matches www subdomain",
			hostname: "www.example.com",
			pattern:  "*.example.com",
			expected: true,
		},

		// Wildcard matches nested subdomains
		{
			name:     "wildcard matches double nested subdomain",
			hostname: "a.b.example.com",
			pattern:  "*.example.com",
			expected: true,
		},
		{
			name:     "wildcard matches triple nested subdomain",
			hostname: "a.b.c.example.com",
			pattern:  "*.example.com",
			expected: true,
		},
		{
			name:     "wildcard matches cdn subdomain",
			hostname: "cdn1.images.example.com",
			pattern:  "*.example.com",
			expected: true,
		},

		// Wildcard does NOT match different domains
		{
			name:     "wildcard does not match different domain",
			hostname: "other.com",
			pattern:  "*.example.com",
			expected: false,
		},
		{
			name:     "wildcard does not match suffix attack",
			hostname: "malicious-example.com",
			pattern:  "*.example.com",
			expected: false,
		},
		{
			name:     "wildcard does not match prefix attack",
			hostname: "example.com.evil.com",
			pattern:  "*.example.com",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty hostname",
			hostname: "",
			pattern:  "*.example.com",
			expected: false,
		},
		{
			name:     "single label hostname",
			hostname: "localhost",
			pattern:  "*.localhost",
			expected: true,
		},
		{
			name:     "wildcard for single label",
			hostname: "sub.localhost",
			pattern:  "*.localhost",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := matchDomainPattern(tt.hostname, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateDomain_WildcardNakedDomain(t *testing.T) {
	t.Parallel()
	// *.example.com should match example.com
	err := validateDomain("example.com", []string{"*.example.com"})
	require.NoError(t, err)
}

func TestValidateDomain_WildcardSubdomain(t *testing.T) {
	t.Parallel()
	// *.example.com should match api.example.com
	err := validateDomain("api.example.com", []string{"*.example.com"})
	require.NoError(t, err)
}

func TestValidateDomain_WildcardNestedSubdomain(t *testing.T) {
	t.Parallel()
	// *.example.com should match a.b.c.example.com
	err := validateDomain("a.b.c.example.com", []string{"*.example.com"})
	require.NoError(t, err)
}

func TestValidateDomain_WildcardWithPort(t *testing.T) {
	t.Parallel()
	// *.example.com should work with standard ports
	err := validateDomain("api.example.com:443", []string{"*.example.com"})
	require.NoError(t, err)

	err = validateDomain("api.example.com:80", []string{"*.example.com"})
	require.NoError(t, err)
}

func TestValidateDomain_WildcardWithExplicitPort(t *testing.T) {
	t.Parallel()
	// *.example.com:8080 should only match that specific port
	err := validateDomain("api.example.com:8080", []string{"*.example.com:8080"})
	require.NoError(t, err)

	// Should not match different port
	err = validateDomain("api.example.com:9000", []string{"*.example.com:8080"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not in the allowed domains list")
}

func TestValidateDomain_WildcardBlocksNonStandardPort(t *testing.T) {
	t.Parallel()
	// *.example.com (no port) should block non-standard ports
	err := validateDomain("api.example.com:8080", []string{"*.example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not in the allowed domains list")
}

func TestValidateDomain_WildcardDoesNotMatchDifferentDomain(t *testing.T) {
	t.Parallel()
	// *.example.com should NOT match other.com
	err := validateDomain("other.com", []string{"*.example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not in the allowed domains list")
}

func TestValidateDomain_WildcardDoesNotMatchSuffixAttack(t *testing.T) {
	t.Parallel()
	// *.example.com should NOT match malicious-example.com
	err := validateDomain("malicious-example.com", []string{"*.example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not in the allowed domains list")
}

func TestValidateDomain_MixedExactAndWildcard(t *testing.T) {
	t.Parallel()
	// Should support both exact and wildcard in the same list
	allowedDomains := []string{"specific.other.com", "*.example.com"}

	// Exact match
	err := validateDomain("specific.other.com", allowedDomains)
	require.NoError(t, err)

	// Wildcard match
	err = validateDomain("api.example.com", allowedDomains)
	require.NoError(t, err)

	// Neither
	err = validateDomain("random.other.com", allowedDomains)
	require.Error(t, err)
}

func TestValidateDomain_WildcardCaseInsensitive(t *testing.T) {
	t.Parallel()
	// Case insensitive matching with wildcards
	err := validateDomain("API.EXAMPLE.COM", []string{"*.example.com"})
	require.NoError(t, err)

	err = validateDomain("api.example.com", []string{"*.EXAMPLE.COM"})
	require.NoError(t, err)
}

func TestHTTPFetch_WildcardDomainAllowed(t *testing.T) {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("wildcard ok")) //nolint:errcheck
	}))
	defer ts.Close()

	// Test server uses 127.0.0.1:PORT, so we need to allow *.0.0.1 or the exact host
	// For this test, we'll use the exact host but demonstrate wildcard in validateDomain tests
	host := ts.Listener.Addr().String()
	rt := newTestRuntimeWithHTTPAccess([]string{host})
	setupHTTPNamespace(t, rt)

	script := fmt.Sprintf(`
		var resp = shisho.http.fetch("%s/test");
		resp.text();
	`, ts.URL)

	val, err := rt.vm.RunString(script)
	require.NoError(t, err)
	assert.Equal(t, "wildcard ok", val.String())
}

func TestHTTPFetch_WildcardRedirectAllowed(t *testing.T) {
	// This test validates that wildcard domains work with redirects
	// Since we can't easily create test servers with custom hostnames,
	// we verify the validateDomain function behavior instead

	// Simulate: redirect from api.example.com to cdn.example.com
	// With wildcard *.example.com, both should be allowed
	allowedDomains := []string{"*.example.com"}

	err := validateDomain("api.example.com", allowedDomains)
	require.NoError(t, err)

	err = validateDomain("cdn.example.com", allowedDomains)
	require.NoError(t, err)

	err = validateDomain("images.cdn.example.com", allowedDomains)
	require.NoError(t, err)
}
