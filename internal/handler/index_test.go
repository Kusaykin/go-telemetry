package handler_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexListsAllMetrics(t *testing.T) {
	store := newFakeStorage()
	store.gauges["Alloc"] = 12.5
	store.gauges["Sys"] = 1234567890
	store.counters["PollCount"] += 5
	store.counters["PollCount"] += 10

	rec := do(store, http.MethodGet, "/")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/html; charset=utf-8", rec.Header().Get("Content-Type"))

	body := rec.Body.String()
	assert.Contains(t, body, "Alloc: 12.5")
	assert.Contains(t, body, "Sys: 1234567890")
	assert.Contains(t, body, "PollCount: 15")
	assert.Contains(t, body, "<html")
}

func TestIndexEmptyStorage(t *testing.T) {
	rec := do(newFakeStorage(), http.MethodGet, "/")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<html")
}

func TestIndexSortsNames(t *testing.T) {
	store := newFakeStorage()
	store.gauges["Sys"] = 1
	store.gauges["Alloc"] = 2
	store.gauges["Mallocs"] = 3

	body := do(store, http.MethodGet, "/").Body.String()

	assert.Less(t, strings.Index(body, "Alloc:"), strings.Index(body, "Mallocs:"))
	assert.Less(t, strings.Index(body, "Mallocs:"), strings.Index(body, "Sys:"))
}
