package document

import (
	"regexp"
	"strings"
)

var (
	imagePattern  = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	linkPattern   = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	markupPattern = regexp.MustCompile(`(?s)<[^>]+>|[*_` + "`" + `#>~]`)
	spacePattern  = regexp.MustCompile(`\s+`)
)

// PlainText keeps searchable prose while removing the Markdown syntax stored by Strapi richtext.
func PlainText(markdown string) string {
	value := imagePattern.ReplaceAllString(markdown, "$1")
	value = linkPattern.ReplaceAllString(value, "$1")
	value = markupPattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(spacePattern.ReplaceAllString(value, " "))
}
