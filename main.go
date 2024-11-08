package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
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
