package main

import "testing"

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
