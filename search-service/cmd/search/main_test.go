package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestSourceVersionHandlesMissingTimestamps(t *testing.T) {
	version := sourceVersion("doc-1", nil)
	if version == "" {
		t.Fatal("sourceVersion() returned an empty hash")
	}
	if version != sourceVersion("doc-1", nil) {
		t.Fatal("sourceVersion() is not stable for missing fields")
	}
	updatedAt := "2026-08-03T00:00:00Z"
	if version == sourceVersion("doc-1", &updatedAt) {
		t.Fatal("sourceVersion() did not change when the document update time changed")
	}
}

func TestDocumentIndexIDSupportsChineseDocID(t *testing.T) {
	docID := "API KEY使用指引"
	indexID := documentIndexID(docID)
	if indexID != documentIndexID(docID) {
		t.Fatal("documentIndexID() is not stable")
	}
	if !regexp.MustCompile(`^doc_[a-f0-9]{64}$`).MatchString(indexID) {
		t.Fatalf("documentIndexID() = %q, want a Meilisearch-safe identifier", indexID)
	}
	if indexID == documentIndexID("API KEY使用指引 2") {
		t.Fatal("documentIndexID() did not change for a different docId")
	}
}

func TestSearchResultHits(t *testing.T) {
	hits, err := searchResultHits([]any{map[string]any{
		"id":      "doc_hash",
		"docId":   "test-kirito",
		"title":   "快速开始",
		"content": "入门文档与管理说明",
		"_formatted": map[string]any{
			"title":   "快速开始",
			"content": "入门文档与<mark>管理</mark>说明",
		},
	}}, "https://help.test.starviewcloud.com")
	if err != nil {
		t.Fatalf("searchResultHits() error = %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "doc_hash" || hits[0].URL != "https://help.test.starviewcloud.com/test-kirito" {
		t.Fatalf("searchResultHits() = %#v", hits)
	}
	if hits[0].Summary != "入门文档与管理说明" || hits[0].Highlight.Summary != "入门文档与<mark>管理</mark>说明" {
		t.Fatalf("unexpected summary fields: %#v", hits[0])
	}
}

func TestRequestBaseURLUsesForwardedHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:38987/search/list", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Forwarded-Host", "help.test.starviewcloud.com")
	if got := requestBaseURL(request); got != "https://help.test.starviewcloud.com" {
		t.Fatalf("requestBaseURL() = %q", got)
	}
}

func TestSearchSuggestionsDeduplicatesDocumentIDs(t *testing.T) {
	suggestions, err := searchSuggestions([]any{
		map[string]any{"docId": "test-kirito", "title": "域名管理"},
		map[string]any{"docId": "test-kirito", "title": "域名管理"},
	}, "https://help.test.starviewcloud.com")
	if err != nil {
		t.Fatalf("searchSuggestions() error = %v", err)
	}
	if len(suggestions) != 1 || suggestions[0].URL != "https://help.test.starviewcloud.com/test-kirito" {
		t.Fatalf("searchSuggestions() = %#v", suggestions)
	}
}

func TestApifoxDocumentReturnsOpenAPIV3JSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/apifox/openapi.json", nil)
	(&service{}).apifoxDocument(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var document map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&document); err != nil {
		t.Fatalf("decode OpenAPI document: %v", err)
	}
	if document["openapi"] != "3.0.3" {
		t.Fatalf("openapi = %#v, want 3.0.3", document["openapi"])
	}
	if _, found := document["paths"].(map[string]any)["/search/suggestions/list"]; !found {
		t.Fatalf("OpenAPI document does not contain the suggestions endpoint")
	}
}

func TestApifoxUIUsesOpenAPIV3Document(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/apifox/index.html", nil)
	(&service{}).routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "openapi.json") {
		t.Fatal("Apifox UI does not load the OpenAPI 3.0 document")
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
