package security

import (
	"net/http"
	"regexp"
	"strings"
)

// SecurityHeaders returns middleware that adds security headers to responses.
func SecurityHeaders() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Prevent MIME type sniffing
			w.Header().Set("X-Content-Type-Options", "nosniff")

			// Enable XSS protection in browsers
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			// Prevent clickjacking
			w.Header().Set("X-Frame-Options", "DENY")

			// Control referrer information
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

			// Content Security Policy for API responses
			w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")

			// Strict Transport Security (for HTTPS)
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

			// Permissions Policy
			w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

			// Remove server header
			w.Header().Del("Server")

			next.ServeHTTP(w, r)
		})
	}
}

// InputValidation returns middleware that validates and sanitizes common input.
func InputValidation() func(http.Handler) http.Handler {
	// Compile regex patterns once
	sqlInjectionPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(union\s+select|select\s+\*|insert\s+into|delete\s+from|drop\s+table|update\s+.*\s+set)`),
		regexp.MustCompile(`(?i)(;\s*--|;\s*#|'\s*or\s*'|"\s*or\s*"|1\s*=\s*1|'\s*=\s*')`),
		regexp.MustCompile(`(?i)(exec\s*\(|execute\s*\(|xp_cmdshell|sp_executesql)`),
	}

	xssPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?i)javascript:`),
		regexp.MustCompile(`(?i)on\w+\s*=`),
		regexp.MustCompile(`(?i)<iframe[^>]*>`),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check query parameters
			for key, values := range r.URL.Query() {
				for _, value := range values {
					if containsMaliciousPattern(value, sqlInjectionPatterns) {
						http.Error(w, `{"error":"invalid input","message":"potentially malicious input detected"}`, http.StatusBadRequest)
						return
					}
					if containsMaliciousPattern(value, xssPatterns) {
						http.Error(w, `{"error":"invalid input","message":"potentially malicious input detected"}`, http.StatusBadRequest)
						return
					}
				}

				// Validate key names
				if containsMaliciousPattern(key, sqlInjectionPatterns) || containsMaliciousPattern(key, xssPatterns) {
					http.Error(w, `{"error":"invalid input","message":"invalid parameter name"}`, http.StatusBadRequest)
					return
				}
			}

			// Check common headers for injection attempts
			headersToCheck := []string{"X-Tenant-ID", "X-Request-ID", "Authorization"}
			for _, header := range headersToCheck {
				value := r.Header.Get(header)
				if value != "" {
					if containsMaliciousPattern(value, sqlInjectionPatterns) {
						http.Error(w, `{"error":"invalid input","message":"invalid header value"}`, http.StatusBadRequest)
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// containsMaliciousPattern checks if the input matches any of the given patterns.
func containsMaliciousPattern(input string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(input) {
			return true
		}
	}
	return false
}

// RequestSizeLimit returns middleware that limits request body size.
func RequestSizeLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBytes {
				http.Error(w, `{"error":"request too large","message":"request body exceeds maximum allowed size"}`, http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// ContentTypeValidation returns middleware that validates Content-Type for POST/PUT/PATCH requests.
// Paths listed in excludePaths are exempt from validation (e.g. file-upload endpoints that use multipart/form-data).
func ContentTypeValidation(allowedTypes ...string) func(http.Handler) http.Handler {
	return ContentTypeValidationWithExclusions(nil, allowedTypes...)
}

// ContentTypeValidationWithExclusions is like ContentTypeValidation but skips validation for the given path prefixes.
func ContentTypeValidationWithExclusions(excludePaths []string, allowedTypes ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool)
	for _, t := range allowedTypes {
		allowed[strings.ToLower(t)] = true
	}
	if len(allowed) == 0 {
		allowed["application/json"] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only check for methods that typically have a body
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
				// Skip validation for excluded paths (e.g. media upload)
				for _, prefix := range excludePaths {
					if strings.HasPrefix(r.URL.Path, prefix) {
						next.ServeHTTP(w, r)
						return
					}
				}

				contentType := r.Header.Get("Content-Type")
				if contentType == "" {
					http.Error(w, `{"error":"missing content type","message":"Content-Type header is required"}`, http.StatusBadRequest)
					return
				}

				// Extract media type (ignore parameters like charset)
				mediaType := strings.ToLower(strings.Split(contentType, ";")[0])
				mediaType = strings.TrimSpace(mediaType)

				if !allowed[mediaType] {
					http.Error(w, `{"error":"unsupported content type","message":"Content-Type must be application/json"}`, http.StatusUnsupportedMediaType)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// APIVersionValidation returns middleware that validates API version in path.
func APIVersionValidation(supportedVersions ...string) func(http.Handler) http.Handler {
	versions := make(map[string]bool)
	for _, v := range supportedVersions {
		versions[v] = true
	}
	if len(versions) == 0 {
		versions["v1"] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract version from path (e.g., /api/v1/...)
			path := r.URL.Path
			if strings.HasPrefix(path, "/api/") {
				parts := strings.SplitN(path[5:], "/", 2)
				if len(parts) > 0 {
					version := parts[0]
					if !versions[version] {
						http.Error(w, `{"error":"unsupported api version","message":"API version not supported"}`, http.StatusBadRequest)
						return
					}
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TenantValidation returns middleware that validates tenant ID format.
func TenantValidation() func(http.Handler) http.Handler {
	// UUID regex pattern
	uuidPattern := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	// Slug pattern (alphanumeric with hyphens)
	slugPattern := regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := r.Header.Get("X-Tenant-ID")
			if tenantID != "" {
				// Validate format (UUID or slug)
				if !uuidPattern.MatchString(tenantID) && !slugPattern.MatchString(tenantID) {
					http.Error(w, `{"error":"invalid tenant","message":"invalid tenant ID format"}`, http.StatusBadRequest)
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
