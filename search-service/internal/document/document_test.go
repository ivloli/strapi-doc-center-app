package document

import "testing"

func TestPlainText(t *testing.T) {
	got := PlainText("# Hello [documentation](https://example.test)\n\n![diagram](image.png) **world**")
	want := "Hello documentation diagram world"
	if got != want {
		t.Fatalf("PlainText() = %q, want %q", got, want)
	}
}
