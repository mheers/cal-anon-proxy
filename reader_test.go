package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownload(t *testing.T) {
	calProxy := NewCalProxy(ReadConfig())
	src1 := calProxy.config.Srcs()[0]
	events, err := calProxy.download(src1)
	require.NoError(t, err)

	require.Len(t, events, 2)
}
