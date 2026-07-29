package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceVersionHandlesMissingTimestamps(t *testing.T) {
	version := sourceVersion("doc-1", nil, "/quick-start", nil)
	if version == "" {
		t.Fatal("sourceVersion() returned an empty hash")
	}
	if version != sourceVersion("doc-1", nil, "/quick-start", nil) {
		t.Fatal("sourceVersion() is not stable for missing fields")
	}
	if version == sourceVersion("doc-1", nil, "/getting-started", nil) {
		t.Fatal("sourceVersion() did not change when the URL changed")
	}
}

func TestSearchResultHits(t *testing.T) {
	hits, err := searchResultHits([]any{map[string]any{
		"id":      "quick-start",
		"title":   "快速开始",
		"content": "入门文档",
		"url":     "/quick-start",
	}})
	if err != nil {
		t.Fatalf("searchResultHits() error = %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "quick-start" || hits[0].URL != "/quick-start" {
		t.Fatalf("searchResultHits() = %#v", hits)
	}
}

func TestWriteErrorUsesPlatformEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, http.StatusBadRequest, "keyword is required")

	var response errorResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Code != http.StatusBadRequest || response.Message != "keyword is required" || response.Data != nil {
		t.Fatalf("unexpected error response: %#v", response)
	}
}
