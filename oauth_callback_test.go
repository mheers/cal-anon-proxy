package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOAuthCallbackHandler_Success(t *testing.T) {
	ch := make(chan callbackResult, 1)
	h := newOAuthCallbackHandler("expected-state", ch)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth2/callback?state=expected-state&code=auth-code-123", nil)
	h(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	res := <-ch
	require.NoError(t, res.err)
	require.Equal(t, "auth-code-123", res.code)
}

func TestOAuthCallbackHandler_StateMismatch(t *testing.T) {
	ch := make(chan callbackResult, 1)
	h := newOAuthCallbackHandler("expected-state", ch)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth2/callback?state=tampered&code=x", nil)
	h(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	res := <-ch
	require.Error(t, res.err)
	require.Contains(t, res.err.Error(), "invalid oauth state")
}

func TestOAuthCallbackHandler_ErrorParam(t *testing.T) {
	ch := make(chan callbackResult, 1)
	h := newOAuthCallbackHandler("expected-state", ch)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth2/callback?state=expected-state&error=access_denied", nil)
	h(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	res := <-ch
	require.Error(t, res.err)
	require.Contains(t, res.err.Error(), "access_denied")
}

func TestOAuthCallbackHandler_MissingCode(t *testing.T) {
	ch := make(chan callbackResult, 1)
	h := newOAuthCallbackHandler("expected-state", ch)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth2/callback?state=expected-state", nil)
	h(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	res := <-ch
	require.Error(t, res.err)
	require.Contains(t, res.err.Error(), "missing authorization code")
}

func TestOAuthCallbackHandler_WrongPath(t *testing.T) {
	ch := make(chan callbackResult, 1)
	h := newOAuthCallbackHandler("expected-state", ch)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/some/other/path", nil)
	h(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	select {
	case <-ch:
		t.Fatal("unrelated paths must not produce a result")
	default:
	}
}
