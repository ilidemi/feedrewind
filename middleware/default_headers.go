package middleware

import "net/http"

func DefaultHeaders(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")                      // Disallow embedding by <iframe> and such
		w.Header().Set("X-Content-Type-Options", "nosniff")                  // Disable guessing mime types
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin") // Safe referrers
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
