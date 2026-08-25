package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/googleapi"
)

func TestNormalizeGoogleAuthMode(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default empty", input: "", want: googleAuthModeOAuth},
		{name: "oauth", input: "oauth", want: googleAuthModeOAuth},
		{name: "sso", input: "sso", want: googleAuthModeSSO},
		{name: "upper-case sso", input: "SSO", want: googleAuthModeSSO},
		{name: "invalid", input: "token", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeGoogleAuthMode(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestValidateGoogleCloneConfig_OAuthRequiresCredentials(t *testing.T) {
	cfg := googleCloneConfig{
		AuthMode:         googleAuthModeOAuth,
		SourceCalendarID: "source@example.com",
		DestCalendarID:   "dest@example.com",
		DaysPast:         1,
		DaysFuture:       30,
	}

	err := validateGoogleCloneConfig(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "GOOGLE_CLIENT_ID")
	require.Contains(t, err.Error(), "GOOGLE_REFRESH_TOKEN")
}

func TestValidateGoogleCloneConfig_OAuthWithoutClientSecret(t *testing.T) {
	cfg := googleCloneConfig{
		AuthMode:         googleAuthModeOAuth,
		ClientID:         "client-id",
		RefreshToken:     "refresh-token",
		SourceCalendarID: "source@example.com",
		DestCalendarID:   "dest@example.com",
		DaysPast:         1,
		DaysFuture:       30,
	}

	err := validateGoogleCloneConfig(cfg)
	require.NoError(t, err)
}

func TestValidateGoogleCloneConfig_SSOWithoutCredentials(t *testing.T) {
	cfg := googleCloneConfig{
		AuthMode:         googleAuthModeSSO,
		SourceCalendarID: "source@example.com",
		DestCalendarID:   "dest@example.com",
		DaysPast:         1,
		DaysFuture:       30,
	}

	err := validateGoogleCloneConfig(cfg)
	require.NoError(t, err)
}

func TestValidateGoogleCloneConfig_InvalidAuthMode(t *testing.T) {
	cfg := googleCloneConfig{
		AuthMode:         "invalid",
		SourceCalendarID: "source@example.com",
		DestCalendarID:   "dest@example.com",
		DaysPast:         1,
		DaysFuture:       30,
	}

	err := validateGoogleCloneConfig(cfg)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid auth mode")
}

func TestNormalizeGoogleCalendarID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain ID",
			input: "calendar.example_ext@org.example",
			want:  "calendar.example_ext@org.example",
		},
		{
			name:  "base64url ID",
			input: "Y2FsZW5kYXIuZXhhbXBsZV9leHRAb3JnLmV4YW1wbGU",
			want:  "calendar.example_ext@org.example",
		},
		{
			name:  "base64 std ID",
			input: "Y2FsZW5kYXIuZXhhbXBsZV9leHRAb3JnLmV4YW1wbGU=",
			want:  "calendar.example_ext@org.example",
		},
		{
			name:  "non-decodable value",
			input: "not-a-calendar-id",
			want:  "not-a-calendar-id",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeGoogleCalendarID(tc.input)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestWrapGoogleCloneError_SSOInsufficientScope(t *testing.T) {
	cfg := googleCloneConfig{AuthMode: googleAuthModeSSO}
	err := &googleapi.Error{Code: 403, Message: "Insufficient Permission"}

	wrapped := wrapGoogleCloneError(err, cfg)
	require.Error(t, wrapped)
	require.Contains(t, wrapped.Error(), "SSO token lacks required scopes")
	require.Contains(t, wrapped.Error(), "https://www.googleapis.com/auth/cloud-platform")
}

func TestWrapGoogleCloneError_OAuthNoHint(t *testing.T) {
	cfg := googleCloneConfig{AuthMode: googleAuthModeOAuth}
	err := fmt.Errorf("some error")

	wrapped := wrapGoogleCloneError(err, cfg)
	require.Equal(t, err, wrapped)
}

func TestPKCEChallenge(t *testing.T) {
	challenge := pkceChallenge("verifier-value")
	require.NotEmpty(t, challenge)
	require.NotContains(t, challenge, "=")
}
