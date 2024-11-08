package main

import (
	"context"
	"net/http"

	"github.com/sirupsen/logrus"
)

type (
	CtxKey   struct{}
	CtxValue struct {
		Username string
	}
)

type auth struct {
	username string
	password string
}

func (a *auth) middleware(actualHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// log the request
		logrus.Infof("%s %s", r.Method, r.URL.Path)

		username, password, ok := r.BasicAuth()
		// check username and password: adjust the logic to your system (do NOT store passwords in plaintext)
		if !ok || username != a.username || password != a.password {
			// abort the request handling on failure
			w.Header().Add("WWW-Authenticate", `Basic realm="Please authenticate", charset="UTF-8"`)
			http.Error(w, "HTTP Basic auth is required", http.StatusUnauthorized)
			return
		}

		// user is authenticated: store this info in the context
		ctx := context.WithValue(r.Context(), CtxKey{}, CtxValue{username})

		// logrus.Infof("authenticated as %s", username)

		// delegate the work to the CardDAV handle
		actualHandler.ServeHTTP(w, r.WithContext(ctx))
	})
}

// tracingMiddleware logs incoming requests
func tracingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// start := time.Now()

		// Log the incoming request details
		logrus.Infof("Request Method: %s, URL: %s, RemoteAddr: %s, User-Agent: %s",
			r.Method, r.URL.Path, r.RemoteAddr, r.Header.Get("User-Agent"))

		// Call the next handler
		next.ServeHTTP(w, r)

		// Log the duration it took to handle the request
		// logrus.Infof("Completed in %s", time.Since(start))
	})
}
