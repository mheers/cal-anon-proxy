package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	hConfig "github.com/maddalax/htmgo/framework/config"
	"github.com/maddalax/htmgo/framework/h"
	"github.com/maddalax/htmgo/framework/service"
	"github.com/mheers/cal-anon-proxy/__htmgo"
	"github.com/sirupsen/logrus"
)

func main() {
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

	r := mux.NewRouter()
	r.Use(tracingMiddleware)

	r.PathPrefix("/caldav/").Handler(handler)

	go func() {
		updateEvents(proxy, calDavHandler)

		ticker := time.NewTicker(time.Duration(config.SrcUpdateInterval) * time.Minute)
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

			__htmgo.Register(app.Router)
		},
	})

	s := &http.Server{
		Addr:           ":8086",
		Handler:        r,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	fmt.Printf("Listening on %s\n", s.Addr)
	logrus.Fatal(s.ListenAndServe())
}

func updateEvents(proxy *CalProxy, calDavHandler *CalDavHandler) {
	events, err := proxy.downloadAll()
	if err != nil {
		logrus.Error(err)
		return
	}

	fmt.Printf("Downloaded %d events\n", len(events))
	calDavHandler.SetEvents(events)
}
