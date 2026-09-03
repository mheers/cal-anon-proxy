package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"time"

	// Embed the IANA timezone database: the runtime image (alpine) ships no
	// /usr/share/zoneinfo, and processEvents/toTZ/ServeEventsJSON call
	// time.LoadLocation — without this they fail with "invalid location name".
	_ "time/tzdata"

	"github.com/go-chi/chi/v5"
	hConfig "github.com/maddalax/htmgo/framework/config"
	"github.com/maddalax/htmgo/framework/h"
	"github.com/maddalax/htmgo/framework/service"
	"github.com/mheers/cal-anon-proxy/__htmgo"
	"github.com/mheers/cal-anon-proxy/pages"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "cal-anon-proxy",
		Short: "Anonymizing CalDAV proxy",
	}

	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "Start the CalDAV proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			runServer()
			return nil
		},
	}

	rootCmd.AddCommand(serverCmd)
	rootCmd.AddCommand(newGoogleCloneCmd())
	rootCmd.AddCommand(newGoogleLoginCmd())

	if err := rootCmd.Execute(); err != nil {
		logrus.Error(err)
		os.Exit(1)
	}
}

func runServer() {
	base := ReadConfig()

	tenants, err := ResolveTenants(base)
	if err != nil {
		logrus.Fatal(err)
	}

	runtimes := make([]*TenantRuntime, 0, len(tenants))
	for _, t := range tenants {
		runtimes = append(runtimes, buildTenantRuntime(t))
	}

	if IsMultiTenant(tenants) {
		names := make([]string, 0, len(tenants))
		for _, t := range tenants {
			names = append(names, t.Name)
		}
		logrus.Infof("multitenant mode: %s (legacy routes alias %q)", strings.Join(names, ", "), tenants[0].Name)
	}

	refreshAll := func() {
		for _, rt := range runtimes {
			updateEvents(rt.Proxy, rt.Dav, rt.Tenant.Name)
		}
	}

	go func() {
		// Guard against SRC_UPDATE_INTERVAL being unset or empty (e.g. passed
		// as an empty string by docker-compose): time.NewTicker panics on <= 0.
		interval := base.SrcUpdateInterval
		if interval <= 0 {
			interval = 5
		}

		refreshAll()

		ticker := time.NewTicker(time.Duration(interval) * time.Minute)
		for range ticker.C {
			refreshAll()
		}
	}()

	locator := service.NewLocator()
	cfg := hConfig.Get()

	h.Start(h.AppOpts{
		ServiceLocator: locator,
		LiveReload:     true,
		Register: func(app *h.App) {
			sub, err := fs.Sub(GetStaticAssets(), "assets/dist")

			if err != nil {
				panic(err)
			}

			http.FileServerFS(sub)

			// change this in htmgo.yml (public_asset_path)
			app.Router.Handle(fmt.Sprintf("%s/*", cfg.PublicAssetPath),
				http.StripPrefix(cfg.PublicAssetPath, http.FileServerFS(sub)))

			registerRoutes(app.Router, runtimes)
		},
	})
}

// TenantRuntime is the isolated serving state of one tenant: its own source
// proxy (with its own per-source last-known-good cache) and its own CalDAV
// handler. A failing source in one tenant never affects another tenant.
type TenantRuntime struct {
	Tenant  *Tenant
	Proxy   *CalProxy
	Dav     *CalDavHandler
	Handler http.Handler
}

func buildTenantRuntime(t *Tenant) *TenantRuntime {
	prefix := "/caldav/"
	if t.Name != "" {
		prefix = "/" + t.Name + "/caldav/"
	}
	proxy := NewCalProxy(t.Config)
	proxy.tenant = t.Name
	dav := NewCalDavHandler(prefix)

	var handler http.Handler = dav.HTTPHandler()
	if t.Config.DstAuthEnabled {
		handler = (&auth{
			username: t.Config.DstUsername,
			password: t.Config.DstPassword,
		}).middleware(dav)
	}

	return &TenantRuntime{Tenant: t, Proxy: proxy, Dav: dav, Handler: handler}
}

// registerRoutes wires every tenant under its own path prefix. In
// single-tenant (legacy) mode the routes are exactly today's: /caldav/,
// /calendar.ics, /events.json and the WebUI at /. In multi-tenant mode each
// tenant serves /{name}, /{name}/calendar.ics, /{name}/events.json and
// /{name}/caldav/, while the legacy paths alias the first tenant so existing
// CalDAV clients keep working.
func registerRoutes(router *chi.Mux, runtimes []*TenantRuntime) {
	if !isMultiRuntime(runtimes) {
		rt := runtimes[0]
		router.Handle("/caldav/", rt.Handler)
		router.Handle("/caldav/*", rt.Handler)
		router.HandleFunc("/calendar.ics", rt.Dav.ServeICS)
		router.HandleFunc("/events.json", rt.Dav.ServeEventsJSON)

		__htmgo.Register(router)
		return
	}

	for _, rt := range runtimes {
		base := "/" + rt.Tenant.Name
		router.Handle(base+"/caldav/", rt.Handler)
		router.Handle(base+"/caldav/*", rt.Handler)
		router.HandleFunc(base+"/calendar.ics", rt.Dav.ServeICS)
		router.HandleFunc(base+"/events.json", rt.Dav.ServeEventsJSON)

		page := tenantPageHandler(rt.Tenant)
		router.Get(base, page)
		router.Get(base+"/", page)
	}

	// Legacy alias for existing clients: everything the single-tenant
	// server used to serve now answers with the first tenant's data.
	first := runtimes[0]
	router.Handle("/caldav/", first.Handler)
	router.Handle("/caldav/*", first.Handler)
	router.HandleFunc("/calendar.ics", first.Dav.ServeICS)
	router.HandleFunc("/events.json", first.Dav.ServeEventsJSON)
	router.Get("/", tenantPageHandler(first.Tenant))

	__htmgo.RegisterPartials(router)
}

func isMultiRuntime(runtimes []*TenantRuntime) bool {
	return !(len(runtimes) == 1 && runtimes[0].Tenant.Name == "")
}

// tenantPageHandler renders the tenant WebUI, or 404 when the tenant's WebUI
// is toggled off. CalDAV/ICS/JSON endpoints are unaffected by the toggle.
func tenantPageHandler(t *Tenant) http.HandlerFunc {
	feed := "/events.json"
	if t.Name != "" {
		feed = "/" + t.Name + "/events.json"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if !t.WebUIEnabled {
			http.NotFound(w, r)
			return
		}
		cc := r.Context().Value(h.RequestContextKey).(*h.RequestContext)
		h.HtmlView(w, pages.TenantIndexPage(cc, t.Name, feed))
	}
}

func updateEvents(proxy *CalProxy, calDavHandler *CalDavHandler, tenant string) {
	events, err := proxy.downloadAll()
	if err != nil {
		logrus.Error(err)
		return
	}

	if tenant != "" {
		logrus.Infof("tenant %q: downloaded %d events", tenant, len(events))
	} else {
		logrus.Infof("Downloaded %d events", len(events))
	}
	events = proxy.compactEvents(events)
	calDavHandler.SetEvents(events)
}
