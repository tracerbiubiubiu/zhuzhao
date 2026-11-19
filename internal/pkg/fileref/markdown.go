// Package fileref shares the stable Markdown file-reference parser between
// ticket writes and attachment deletion. It performs no I/O or authorization.
package fileref

import (
	"bytes"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var refParser = goldmark.New(goldmark.WithExtensions(extension.Table))

// ExtractFileRefs returns unique stable image object keys in source order.
func ExtractFileRefs(src string) []string {
	doc := refParser.Parser().Parse(text.NewReader([]byte(src)))
	var keys []string
	seen := map[string]struct{}{}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		img, ok := n.(*ast.Image)
		if !ok {
			return ast.WalkContinue, nil
		}
		const prefix = "/api/v1/files/"
		if !bytes.HasPrefix(img.Destination, []byte(prefix)) {
			return ast.WalkContinue, nil
		}
		key := string(bytes.TrimPrefix(img.Destination, []byte(prefix)))
		if key == "" {
			return ast.WalkContinue, nil
		}
		if _, dup := seen[key]; dup {
			return ast.WalkContinue, nil
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
		return ast.WalkContinue, nil
	})
	return keys
}
