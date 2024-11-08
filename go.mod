module github.com/mheers/cal-anon-proxy

go 1.23.1

require (
	github.com/emersion/go-ical v0.0.0-20240127095438-fc1c9d8fb2b6
	github.com/emersion/go-webdav v0.5.1-0.20240419143909-21f251fa1de2
	github.com/gorilla/mux v1.8.1
	github.com/sethvargo/go-envconfig v1.1.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.9.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/teambition/rrule-go v1.8.2 // indirect
	golang.org/x/sys v0.25.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/emersion/go-webdav => github.com/mheers/go-webdav v0.0.0-20240923131844-0fc5736b465f
