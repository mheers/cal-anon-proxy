package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDownload(t *testing.T) {
	calProxy := NewCalProxy(ReadConfig())
	src1 := calProxy.config.Srcs()[0]
	events, err := calProxy.download(src1)
	require.NoError(t, err)

	require.Len(t, events, 2)
}

func TestTZs(t *testing.T) {
	tzid := "W. Europe Standard Time"
	_, err := time.LoadLocation(translateTZ(tzid))
	require.NoError(t, err)
}
