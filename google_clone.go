package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	googleAuthModeOAuth = "oauth"
	googleAuthModeSSO   = "sso"
	googleSSOScopes     = "https://www.googleapis.com/auth/cloud-platform,https://www.googleapis.com/auth/calendar"

	clonedByMarker = "clonedBy"
	clonedByValue  = "cal-anon-proxy"
)

type googleCloneConfig struct {
	AuthMode         string
	ClientID         string
	ClientSecret     string
	RefreshToken     string
	SourceCalendarID string
	DestCalendarID   string
	DaysPast         int
	DaysFuture       int
	Interval         time.Duration
	WipeDestination  bool
	AllowEmptySource bool
	DryRun           bool
}

type googleLoginConfig struct {
	ClientID     string
	ClientSecret string
	NoBrowser    bool
	Timeout      time.Duration
}

func newGoogleCloneCmd() *cobra.Command {
	cfg := googleCloneConfig{
		AuthMode:         envOr("GOOGLE_AUTH_MODE", googleAuthModeOAuth),
		ClientID:         envOr("GOOGLE_CLIENT_ID", ""),
		ClientSecret:     envOr("GOOGLE_CLIENT_SECRET", ""),
		RefreshToken:     envOr("GOOGLE_REFRESH_TOKEN", ""),
		SourceCalendarID: envOr("GOOGLE_SOURCE_CALENDAR_ID", ""),
		DestCalendarID:   envOr("GOOGLE_DEST_CALENDAR_ID", ""),
		DaysPast:         envOrInt("GOOGLE_SYNC_DAYS_PAST", 30),
		DaysFuture:       envOrInt("GOOGLE_SYNC_DAYS_FUTURE", 365),
		Interval:         envOrDuration("GOOGLE_SYNC_INTERVAL", 0),
		WipeDestination:  envOrBool("GOOGLE_WIPE_DESTINATION", true),
		AllowEmptySource: envOrBool("GOOGLE_ALLOW_EMPTY_SOURCE", false),
		DryRun:           envOrBool("GOOGLE_DRY_RUN", false),
	}

	cmd := &cobra.Command{
		Use:   "google-clone",
		Short: "Clone events from one Google calendar to another",
		Long:  "Syncs events from a source Google calendar (including subscribed calendars shared to your account) into a destination Google calendar.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return runGoogleClone(ctx, cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.AuthMode, "auth-mode", cfg.AuthMode, "Google auth mode: oauth or sso")
	cmd.Flags().StringVar(&cfg.ClientID, "client-id", cfg.ClientID, "Google OAuth client ID")
	cmd.Flags().StringVar(&cfg.ClientSecret, "client-secret", cfg.ClientSecret, "Google OAuth client secret")
	cmd.Flags().StringVar(&cfg.RefreshToken, "refresh-token", cfg.RefreshToken, "Google OAuth refresh token")
	cmd.Flags().StringVar(&cfg.SourceCalendarID, "source-calendar-id", cfg.SourceCalendarID, "Source Google calendar ID (shared/subscribed calendar)")
	cmd.Flags().StringVar(&cfg.DestCalendarID, "dest-calendar-id", cfg.DestCalendarID, "Destination Google calendar ID")
	cmd.Flags().IntVar(&cfg.DaysPast, "days-past", cfg.DaysPast, "How many days in the past to sync")
	cmd.Flags().IntVar(&cfg.DaysFuture, "days-future", cfg.DaysFuture, "How many days in the future to sync")
	cmd.Flags().DurationVar(&cfg.Interval, "interval", cfg.Interval, "Repeat sync interval (e.g. 15m). 0 means run once")
	cmd.Flags().BoolVar(&cfg.WipeDestination, "wipe-destination", cfg.WipeDestination, "Delete destination events in sync window before inserting fresh copies")
	cmd.Flags().BoolVar(&cfg.AllowEmptySource, "allow-empty-source", cfg.AllowEmptySource, "Allow syncing when the source calendar returns 0 events (otherwise the run aborts to protect destination data)")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", cfg.DryRun, "Log what would happen without writing destination events")

	return cmd
}

func newGoogleLoginCmd() *cobra.Command {
	cfg := googleLoginConfig{
		ClientID:     envOr("GOOGLE_CLIENT_ID", ""),
		ClientSecret: envOr("GOOGLE_CLIENT_SECRET", ""),
		Timeout:      5 * time.Minute,
	}

	cmd := &cobra.Command{
		Use:   "google-login",
		Short: "Login to Google and print a refresh token for google-clone",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGoogleLogin(cmd.Context(), cfg, cmd.OutOrStdout())
		},
	}

	cmd.Flags().StringVar(&cfg.ClientID, "client-id", cfg.ClientID, "Google OAuth client ID (Desktop app recommended)")
	cmd.Flags().StringVar(&cfg.ClientSecret, "client-secret", cfg.ClientSecret, "Google OAuth client secret (optional for PKCE)")
	cmd.Flags().BoolVar(&cfg.NoBrowser, "no-browser", cfg.NoBrowser, "Print login URL without attempting to open a browser")
	cmd.Flags().DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "Login timeout")

	return cmd
}

func runGoogleLogin(ctx context.Context, cfg googleLoginConfig, out io.Writer) error {
	if strings.TrimSpace(cfg.ClientID) == "" {
		return fmt.Errorf("missing required Google login configuration: client-id / GOOGLE_CLIENT_ID")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()

	redirectURL := fmt.Sprintf("http://%s/oauth2/callback", listener.Addr().String())
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{calendar.CalendarScope},
	}

	state, err := randomBase64URL(24)
	if err != nil {
		return err
	}
	verifier, err := randomBase64URL(48)
	if err != nil {
		return err
	}
	challenge := pkceChallenge(verifier)

	authURL := oauthCfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	if !cfg.NoBrowser {
		_ = openBrowser(authURL)
	}
	fmt.Fprintf(out, "Open this URL and complete login:\n%s\n\n", authURL)

	timeoutCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	resultCh := make(chan callbackResult, 1)

	server := &http.Server{Handler: newOAuthCallbackHandler(state, resultCh)}

	go func() {
		_ = server.Serve(listener)
	}()
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer shutdownCancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	var result callbackResult
	select {
	case result = <-resultCh:
	case <-timeoutCtx.Done():
		return fmt.Errorf("google login timed out after %s", cfg.Timeout)
	}
	if result.err != nil {
		return result.err
	}

	token, err := oauthCfg.Exchange(timeoutCtx, result.code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return err
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return fmt.Errorf("no refresh token returned by Google; ensure you use a Desktop OAuth client and allow offline access")
	}

	fmt.Fprintf(out, "GOOGLE_REFRESH_TOKEN=%s\n", token.RefreshToken)
	return nil
}

type callbackResult struct {
	code string
	err  error
}

func newOAuthCallbackHandler(state string, resultCh chan<- callbackResult) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/callback" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("state") != state {
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
			select {
			case resultCh <- callbackResult{err: fmt.Errorf("invalid oauth state")}:
			default:
			}
			return
		}
		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			http.Error(w, oauthErr, http.StatusBadRequest)
			select {
			case resultCh <- callbackResult{err: fmt.Errorf("google login failed: %s", oauthErr)}:
			default:
			}
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing authorization code", http.StatusBadRequest)
			select {
			case resultCh <- callbackResult{err: fmt.Errorf("missing authorization code")}:
			default:
			}
			return
		}
		_, _ = io.WriteString(w, "Google login complete. Return to the terminal.\n")
		select {
		case resultCh <- callbackResult{code: code}:
		default:
		}
	}
}

func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func openBrowser(url string) error {
	cmds := [][]string{{"xdg-open", url}, {"open", url}}
	for _, cmd := range cmds {
		if err := exec.Command(cmd[0], cmd[1:]...).Start(); err == nil {
			return nil
		}
	}
	return fmt.Errorf("could not open browser automatically")
}

func runGoogleClone(ctx context.Context, cfg googleCloneConfig) error {
	cfg = normalizeGoogleCloneConfig(cfg)

	if err := validateGoogleCloneConfig(cfg); err != nil {
		return err
	}

	svc, err := newGoogleCalendarService(ctx, cfg)
	if err != nil {
		return err
	}

	syncNow := func() error {
		now := time.Now()
		windowStart := now.AddDate(0, 0, -cfg.DaysPast)
		windowEnd := now.AddDate(0, 0, cfg.DaysFuture)

		inserted, deleted, err := cloneGoogleCalendarWindow(ctx, svc, cfg, windowStart, windowEnd)
		if err != nil {
			return err
		}
		logrus.Infof("google-clone completed: inserted=%d deleted=%d", inserted, deleted)
		return nil
	}

	if cfg.Interval <= 0 {
		if err := syncNow(); err != nil {
			return wrapGoogleCloneError(err, cfg)
		}
		return nil
	}

	if err := syncNow(); err != nil {
		return wrapGoogleCloneError(err, cfg)
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logrus.Info("google-clone stopped")
			return nil
		case <-ticker.C:
			if err := syncNow(); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				logrus.Error(err)
			}
		}
	}
}

func normalizeGoogleCloneConfig(cfg googleCloneConfig) googleCloneConfig {
	cfg.SourceCalendarID = normalizeGoogleCalendarID(cfg.SourceCalendarID)
	cfg.DestCalendarID = normalizeGoogleCalendarID(cfg.DestCalendarID)
	return cfg
}

func normalizeGoogleCalendarID(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "@") {
		return v
	}

	decode := func(encoding *base64.Encoding) (string, bool) {
		decoded, err := encoding.DecodeString(v)
		if err != nil {
			return "", false
		}
		decodedStr := strings.TrimSpace(string(decoded))
		if decodedStr == "" || !strings.Contains(decodedStr, "@") {
			return "", false
		}
		return decodedStr, true
	}

	if decoded, ok := decode(base64.RawURLEncoding); ok {
		return decoded
	}
	if decoded, ok := decode(base64.URLEncoding); ok {
		return decoded
	}
	if decoded, ok := decode(base64.RawStdEncoding); ok {
		return decoded
	}
	if decoded, ok := decode(base64.StdEncoding); ok {
		return decoded
	}

	return v
}

func wrapGoogleCloneError(err error, cfg googleCloneConfig) error {
	var gErr *googleapi.Error
	if !strings.EqualFold(strings.TrimSpace(cfg.AuthMode), googleAuthModeSSO) {
		return err
	}

	if !errors.As(err, &gErr) {
		return err
	}

	insufficient := gErr.Code == 403 && strings.Contains(strings.ToLower(gErr.Message), "insufficient")
	if !insufficient {
		return err
	}

	return fmt.Errorf("%w\nSSO token lacks required scopes. Re-authenticate ADC with: gcloud auth application-default login --scopes=%s", err, googleSSOScopes)
}

func validateGoogleCloneConfig(cfg googleCloneConfig) error {
	authMode, err := normalizeGoogleAuthMode(cfg.AuthMode)
	if err != nil {
		return err
	}

	missing := []string{}
	if authMode == googleAuthModeOAuth {
		if cfg.ClientID == "" {
			missing = append(missing, "client-id / GOOGLE_CLIENT_ID")
		}
		if cfg.RefreshToken == "" {
			missing = append(missing, "refresh-token / GOOGLE_REFRESH_TOKEN")
		}
	}
	if cfg.SourceCalendarID == "" {
		missing = append(missing, "source-calendar-id / GOOGLE_SOURCE_CALENDAR_ID")
	}
	if cfg.DestCalendarID == "" {
		missing = append(missing, "dest-calendar-id / GOOGLE_DEST_CALENDAR_ID")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required Google clone configuration: %s", strings.Join(missing, ", "))
	}

	if cfg.DaysPast < 0 {
		return fmt.Errorf("days-past must be >= 0")
	}
	if cfg.DaysFuture <= 0 {
		return fmt.Errorf("days-future must be > 0")
	}

	return nil
}

func normalizeGoogleAuthMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		mode = googleAuthModeOAuth
	}

	switch mode {
	case googleAuthModeOAuth, googleAuthModeSSO:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid auth mode %q (supported: %s, %s)", raw, googleAuthModeOAuth, googleAuthModeSSO)
	}
}

func newGoogleCalendarService(ctx context.Context, cfg googleCloneConfig) (*calendar.Service, error) {
	authMode, err := normalizeGoogleAuthMode(cfg.AuthMode)
	if err != nil {
		return nil, err
	}

	if authMode == googleAuthModeSSO {
		client, err := newSSOHTTPClient(ctx)
		if err != nil {
			return nil, err
		}
		return calendar.NewService(ctx, option.WithHTTPClient(client), option.WithScopes(calendar.CalendarScope))
	}

	oauthCfg := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes: []string{
			calendar.CalendarScope,
		},
	}

	tokenSource := oauthCfg.TokenSource(ctx, &oauth2.Token{RefreshToken: cfg.RefreshToken})
	client := oauth2.NewClient(ctx, tokenSource)

	svc, err := calendar.NewService(ctx, option.WithHTTPClient(client), option.WithScopes(calendar.CalendarScope))
	if err != nil {
		return nil, err
	}

	return svc, nil
}

// gcloudBinary is a seam for tests to avoid executing the real gcloud binary.
var gcloudBinary = "gcloud"

type gcloudTokenSource struct{}

func (s *gcloudTokenSource) Token() (*oauth2.Token, error) {
	cmd := exec.Command(gcloudBinary, "auth", "print-access-token", "--scopes="+googleSSOScopes)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("gcloud auth print-access-token failed: %v: %s", err, msg)
		}
		return nil, fmt.Errorf("gcloud auth print-access-token failed: %v", err)
	}

	token := strings.TrimSpace(string(out))
	if token == "" {
		return nil, fmt.Errorf("gcloud returned empty access token")
	}

	return &oauth2.Token{
		AccessToken: token,
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(50 * time.Minute),
	}, nil
}

func newSSOHTTPClient(ctx context.Context) (*http.Client, error) {
	gcloudTS := &gcloudTokenSource{}
	_, gcloudErr := gcloudTS.Token()
	if gcloudErr == nil {
		logrus.Debug("Using gcloud access token source for Google SSO")
		return oauth2.NewClient(ctx, oauth2.ReuseTokenSource(nil, gcloudTS)), nil
	}

	defaultTS, adcErr := google.DefaultTokenSource(ctx, calendar.CalendarScope)
	if adcErr != nil {
		logrus.Debugf("Could not initialize ADC token source: %v", adcErr)
		return nil, fmt.Errorf(
			"no SSO credentials available: gcloud probe failed (%v) and Application Default Credentials are unavailable (%v); run 'gcloud auth application-default login --scopes=%s' or use --auth-mode oauth",
			gcloudErr, adcErr, googleSSOScopes,
		)
	}
	logrus.Debug("Using ADC token source for Google SSO")
	return oauth2.NewClient(ctx, defaultTS), nil
}

func cloneGoogleCalendarWindow(ctx context.Context, svc *calendar.Service, cfg googleCloneConfig, windowStart, windowEnd time.Time) (inserted int, deleted int, err error) {
	sourceEvents, err := listCalendarEvents(ctx, svc, cfg.SourceCalendarID, windowStart, windowEnd)
	if err != nil {
		return 0, 0, err
	}

	// Safety guard: an empty source listing usually means a misconfiguration or a
	// transient API problem. Wiping the destination on top of it would silently
	// destroy the mirror. Abort unless explicitly overridden.
	if len(sourceEvents) == 0 && !cfg.AllowEmptySource {
		return 0, 0, fmt.Errorf(
			"source calendar %s returned 0 events in window %s..%s; refusing to touch destination calendar %s (use --allow-empty-source / GOOGLE_ALLOW_EMPTY_SOURCE=true to override)",
			cfg.SourceCalendarID,
			windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339),
			cfg.DestCalendarID,
		)
	}

	if cfg.WipeDestination {
		destinationEvents, err := listCalendarEvents(ctx, svc, cfg.DestCalendarID, windowStart, windowEnd)
		if err != nil {
			return 0, 0, err
		}

		// First sync (no cloned events found yet): full wipe of the window to
		// establish a clean mirror. Subsequent syncs only remove previously
		// cloned events so manually created destination events survive.
		var toDelete []*calendar.Event
		var tagged []*calendar.Event
		for _, event := range destinationEvents {
			if isClonedEvent(event) {
				tagged = append(tagged, event)
			}
		}
		if len(tagged) > 0 {
			toDelete = tagged
		} else {
			toDelete = destinationEvents
		}

		for _, event := range toDelete {
			if cfg.DryRun {
				deleted++
				continue
			}
			if err := svc.Events.Delete(cfg.DestCalendarID, event.Id).SendUpdates("none").Context(ctx).Do(); err != nil {
				return inserted, deleted, err
			}
			deleted++
		}
	}

	for _, event := range sourceEvents {
		if event.Status == "cancelled" {
			continue
		}

		clone := cloneEventForDestination(event)
		if cfg.DryRun {
			inserted++
			continue
		}

		if _, err := svc.Events.Insert(cfg.DestCalendarID, clone).SendUpdates("none").Context(ctx).Do(); err != nil {
			return inserted, deleted, err
		}
		inserted++
	}

	return inserted, deleted, nil
}

func isClonedEvent(event *calendar.Event) bool {
	return event.ExtendedProperties != nil &&
		event.ExtendedProperties.Private != nil &&
		event.ExtendedProperties.Private[clonedByMarker] == clonedByValue
}

func listCalendarEvents(ctx context.Context, svc *calendar.Service, calendarID string, windowStart, windowEnd time.Time) ([]*calendar.Event, error) {
	events := make([]*calendar.Event, 0)
	pageToken := ""

	for {
		call := svc.Events.List(calendarID).
			ShowDeleted(false).
			SingleEvents(true).
			OrderBy("startTime").
			TimeMin(windowStart.Format(time.RFC3339)).
			TimeMax(windowEnd.Format(time.RFC3339)).
			MaxResults(2500).
			Context(ctx)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, err
		}

		events = append(events, resp.Items...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	return events, nil
}

func cloneEventForDestination(source *calendar.Event) *calendar.Event {
	clone := &calendar.Event{
		Summary:                 source.Summary,
		Description:             source.Description,
		Location:                source.Location,
		Start:                   cloneEventDateTime(source.Start),
		End:                     cloneEventDateTime(source.End),
		Recurrence:              append([]string(nil), source.Recurrence...),
		Transparency:            source.Transparency,
		Visibility:              source.Visibility,
		ColorId:                 source.ColorId,
		GuestsCanInviteOthers:   source.GuestsCanInviteOthers,
		GuestsCanModify:         source.GuestsCanModify,
		GuestsCanSeeOtherGuests: source.GuestsCanSeeOtherGuests,
		AnyoneCanAddSelf:        source.AnyoneCanAddSelf,
		Status:                  "confirmed",
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				clonedByMarker: clonedByValue,
			},
		},
	}
	if source.Id != "" {
		clone.ExtendedProperties.Private["sourceEventId"] = source.Id
	}

	if source.Reminders != nil {
		overrides := make([]*calendar.EventReminder, 0, len(source.Reminders.Overrides))
		for _, rem := range source.Reminders.Overrides {
			if rem == nil {
				continue
			}
			copied := *rem
			overrides = append(overrides, &copied)
		}
		clone.Reminders = &calendar.EventReminders{
			UseDefault: source.Reminders.UseDefault,
			Overrides:  overrides,
		}
	}

	return clone
}

func cloneEventDateTime(dt *calendar.EventDateTime) *calendar.EventDateTime {
	if dt == nil {
		return nil
	}
	return &calendar.EventDateTime{
		Date:     dt.Date,
		DateTime: dt.DateTime,
		TimeZone: dt.TimeZone,
	}
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envOrInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func envOrDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func envOrBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}
