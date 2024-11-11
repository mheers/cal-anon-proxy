package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/emersion/go-webdav/caldav"
	"github.com/stretchr/testify/require"
)

func TestDownload(t *testing.T) {
	calProxy := NewCalProxy(ReadConfig())
	src1 := calProxy.config.Srcs()[0]
	events, err := calProxy.download(src1)
	require.NoError(t, err)

	// save events to disk as json
	saveEvents(events, "events.json")

	require.Len(t, events, 2)
}

func saveEvents(events []*caldav.CalendarObject, filename string) error {
	jsonB, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filename, jsonB, 0644); err != nil {
		return err
	}

	return nil
}
