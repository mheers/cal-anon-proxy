package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthMiddleware_RejectsMissingAndWrongCredentials(t *testing.T) {
	a := &auth{username: "u", password: "p"}
	var seenUser string
	handler := a.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := currentUsername(r.Context())
		require.NoError(t, err)
		seenUser = user
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name     string
		username string
		password string
		wantCode int
	}{
		{name: "no credentials", wantCode: http.StatusUnauthorized},
		{name: "wrong username", username: "wrong", password: "p", wantCode: http.StatusUnauthorized},
		{name: "wrong password", username: "u", password: "wrong", wantCode: http.StatusUnauthorized},
		{name: "valid credentials", username: "u", password: "p", wantCode: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.username != "" || tc.password != "" {
				req.SetBasicAuth(tc.username, tc.password)
			}
			handler.ServeHTTP(rec, req)

			require.Equal(t, tc.wantCode, rec.Code)
			if tc.wantCode == http.StatusUnauthorized {
				require.Equal(t, `Basic realm="Please authenticate", charset="UTF-8"`,
					rec.Header().Get("WWW-Authenticate"))
			} else {
				require.Equal(t, "u", seenUser)
			}
		})
	}
}

func TestCurrentUsername_UnauthenticatedContext(t *testing.T) {
	_, err := currentUsername(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "not authenticated")
}
