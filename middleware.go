package main

import (
	"context"
	"crypto/subtle"
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
		// Constant-time comparison to avoid leaking credentials via timing
		userOK := subtle.ConstantTimeCompare([]byte(username), []byte(a.username)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(password), []byte(a.password)) == 1
		if !ok || !userOK || !passOK {
			// abort the request handling on failure
			w.Header().Add("WWW-Authenticate", `Basic realm="Please authenticate", charset="UTF-8"`)
			http.Error(w, "HTTP Basic auth is required", http.StatusUnauthorized)
			return
		}

		// user is authenticated: store this info in the context
		ctx := context.WithValue(r.Context(), CtxKey{}, CtxValue{username})

		// delegate the work to the CardDAV handle
		actualHandler.ServeHTTP(w, r.WithContext(ctx))
	})
}
