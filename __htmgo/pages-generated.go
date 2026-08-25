// Package __htmgo THIS FILE IS GENERATED. DO NOT EDIT.
package __htmgo

import "net/http"
import "github.com/go-chi/chi/v5"
import "github.com/maddalax/htmgo/framework/h"
import "github.com/mheers/cal-anon-proxy/pages"

func RegisterPages(router *chi.Mux) {
	router.Get("/", func(writer http.ResponseWriter, request *http.Request) {
		cc := request.Context().Value(h.RequestContextKey).(*h.RequestContext)
		h.HtmlView(writer, pages.IndexPage(cc))
	})
}
