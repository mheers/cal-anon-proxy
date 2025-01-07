module github.com/mheers/cal-anon-proxy

go 1.23.2

require (
	github.com/emersion/go-ical v0.0.0-20240127095438-fc1c9d8fb2b6
	github.com/emersion/go-webdav v0.5.1-0.20240419143909-21f251fa1de2
	github.com/go-chi/chi/v5 v5.2.0
	github.com/maddalax/htmgo/framework v1.0.6
	github.com/mheers/go-tz v0.0.0-20241118104250-bdd693e0a080
	github.com/sethvargo/go-envconfig v1.1.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/teambition/rrule-go v1.8.2 // indirect
	golang.org/x/sys v0.25.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/emersion/go-webdav => github.com/mheers/go-webdav v0.0.0-20240923131844-0fc5736b465f
