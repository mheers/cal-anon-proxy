//go:build !prod
// +build !prod

package main

import (
	"io/fs"

	"github.com/mheers/cal-anon-proxy/internal/embedded"
)

func GetStaticAssets() fs.FS {
	return embedded.NewOsFs()
}
