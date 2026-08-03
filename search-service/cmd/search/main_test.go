package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
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
