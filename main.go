package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"

	// Embed the IANA timezone database: the runtime image (alpine) ships no
	// /usr/share/zoneinfo, and processEvents/toTZ/ServeEventsJSON call
	// time.LoadLocation — without this they fail with "invalid location name".
	_ "time/tzdata"

	hConfig "github.com/maddalax/htmgo/framework/config"
	"github.com/maddalax/htmgo/framework/h"
	"github.com/maddalax/htmgo/framework/service"
	"github.com/mheers/cal-anon-proxy/__htmgo"
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
	config := ReadConfig()

	proxy := NewCalProxy(config)

	calDavHandler := NewCalDavHandler("/caldav/")
	handler := calDavHandler.HTTPHandler()

	if config.DstAuthEnabled {
		a := auth{
			username: config.DstUsername,
			password: config.DstPassword,
		}
		handler = a.middleware(calDavHandler)
	}

	go func() {
		// Guard against SRC_UPDATE_INTERVAL being unset or empty (e.g. passed
		// as an empty string by docker-compose): time.NewTicker panics on <= 0.
		interval := config.SrcUpdateInterval
		if interval <= 0 {
			interval = 5
		}

		updateEvents(proxy, calDavHandler)

		ticker := time.NewTicker(time.Duration(interval) * time.Minute)
		for range ticker.C {
			updateEvents(proxy, calDavHandler)
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

			app.Router.Handle("/caldav/", handler)
			app.Router.HandleFunc("/calendar.ics", calDavHandler.ServeICS)
			app.Router.HandleFunc("/events.json", calDavHandler.ServeEventsJSON)

			__htmgo.Register(app.Router)
		},
	})
}

func updateEvents(proxy *CalProxy, calDavHandler *CalDavHandler) {
	events, err := proxy.downloadAll()
	if err != nil {
		logrus.Error(err)
		return
	}

	logrus.Infof("Downloaded %d events", len(events))
	events = proxy.compactEvents(events)
	calDavHandler.SetEvents(events)
}
