package middleware

import (
	"encoding/base64"
	"net/http"
	"strings"
)

type CredentialProvider interface {
	GetCredentials() (username, password string)
}

func BasicAuthDynamic(provider CredentialProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, password := provider.GetCredentials()
			if username == "" || password == "" {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				unauthorized(w)
				return
			}

			const prefix = "Basic "
			if !strings.HasPrefix(authHeader, prefix) {
				unauthorized(w)
				return
			}

			credentials, err := base64.StdEncoding.DecodeString(authHeader[len(prefix):])
			if err != nil {
				unauthorized(w)
				return
			}

			parts := strings.SplitN(string(credentials), ":", 2)
			if len(parts) != 2 || parts[0] != username || parts[1] != password {
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func BasicAuth(username, password string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if username == "" || password == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				unauthorized(w)
				return
			}

			const prefix = "Basic "
			if !strings.HasPrefix(authHeader, prefix) {
				unauthorized(w)
				return
			}

			credentials, err := base64.StdEncoding.DecodeString(authHeader[len(prefix):])
			if err != nil {
				unauthorized(w)
				return
			}

			parts := strings.SplitN(string(credentials), ":", 2)
			if len(parts) != 2 || parts[0] != username || parts[1] != password {
				unauthorized(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="ENCV WebDAV"`)
	w.WriteHeader(http.StatusUnauthorized)
}
