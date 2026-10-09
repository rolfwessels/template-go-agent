package agent

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

func readFetchLimit(r io.Reader, limit int) ([]byte, bool, error) {
	raw, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	truncated := len(raw) > limit
	if truncated {
		raw = raw[:limit]
	}
	return raw, truncated, err
}

func capFetchText(s string, limit int) (string, bool) {
	s = strings.ToValidUTF8(s, "�")
	if len(s) <= limit {
		return s, false
	}
	end := limit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end], true
}

// MIME support deliberately excludes PDF, images, SVG, and other binaries.
func textualFetchType(kind string) bool {
	switch kind {
	case "text/html", "application/xhtml+xml", "text/plain", "text/markdown", "text/csv", "text/xml", "application/xml", "application/rss+xml", "application/atom+xml", "application/json", "text/json", "application/yaml", "text/yaml":
		return true
	}
	return strings.HasSuffix(kind, "+json")
}

func fetchReadableContent(raw []byte, contentType string, finalURL *url.URL, maxDecoded, maxText int) (body, title, kind string, truncated bool, err error) {
	if contentType == "" && len(raw) > 0 {
		prefix := raw
		if len(prefix) > 512 {
			prefix = prefix[:512]
		}
		contentType = http.DetectContentType(prefix)
	}
	kind, params, parseErr := mime.ParseMediaType(contentType)
	// HEAD, 204, 304 and zero-length responses contain metadata only. There
	// is nothing to sniff, decode or classify as unsupported content.
	if len(raw) == 0 {
		if parseErr != nil && contentType != "" {
			kind = "unknown"
		}
		return "", "", kind, false, nil
	}
	if parseErr != nil {
		return "Unsupported content type.", "", "unknown", false, nil
	}
	if !textualFetchType(kind) {
		return "Unsupported content type: " + kind + ".", "", kind, false, nil
	}
	var reader io.Reader = bytes.NewReader(raw)
	if kind == "text/html" || kind == "application/xhtml+xml" {
		reader, err = charset.NewReader(reader, contentType)
	} else if label := params["charset"]; label != "" {
		reader, err = charset.NewReaderLabel(label, reader)
	}
	if err != nil {
		return "", "", kind, false, fetchPolicyError("unsupported_charset")
	}
	decoded, cut, readErr := readFetchLimit(reader, maxDecoded)
	if readErr != nil {
		return "", "", kind, false, fetchPolicyError("decode_failed")
	}
	truncated = cut
	if kind == "text/html" || kind == "application/xhtml+xml" {
		body, title, cut, err = extractFetchHTML(decoded, finalURL, maxText)
	} else {
		body, cut = capFetchText(string(decoded), maxText)
	}
	return body, title, kind, truncated || cut, err
}

func extractFetchHTML(raw []byte, base *url.URL, limit int) (string, string, bool, error) {
	root, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return "", "", false, fetchPolicyError("html_parse_failed")
	}
	var title string
	// Find the title separately so early text truncation cannot lose metadata.
	stack := []*html.Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.Type == html.ElementNode && n.Data == "title" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					title += c.Data
				}
			}
			break
		}
		for c := n.LastChild; c != nil; c = c.PrevSibling {
			stack = append(stack, c)
		}
	}
	title, titleCut := capFetchText(strings.Join(strings.Fields(title), " "), min(limit, 1024))
	var out strings.Builder
	cut := titleCut
	appendText := func(s string) {
		if out.Len() > limit {
			cut = true
			return
		}
		// Append at most limit+1 bytes; keep allocation bounded even for long links.
		remaining := limit + 1 - out.Len()
		if len(s) > remaining {
			s = s[:remaining]
			cut = true
		}
		out.WriteString(s)
	}
	var walk func(*html.Node, int)
	walk = func(n *html.Node, depth int) {
		if out.Len() > limit {
			cut = true
			return
		}
		if depth > 512 {
			cut = true
			return
		}
		block := false
		if n.Type == html.ElementNode {
			switch n.Data {
			case "head", "script", "style", "form", "nav", "footer", "header", "aside", "template", "noscript", "iframe", "object", "embed", "svg", "input", "button", "select", "textarea":
				return
			case "h1", "h2", "h3", "h4", "h5", "h6", "p", "div", "section", "article", "main", "ul", "ol", "li", "br", "table", "tr", "blockquote", "pre":
				block = true
			}
			// Drop common navigation/advertising boilerplate and hidden content.
			for _, a := range n.Attr {
				if a.Key == "hidden" || a.Key == "aria-hidden" && a.Val == "true" {
					return
				}
				if a.Key == "role" {
					switch a.Val {
					case "navigation", "banner", "contentinfo", "complementary":
						return
					}
				}
				if a.Key == "class" || a.Key == "id" {
					for _, word := range strings.Fields(strings.ToLower(a.Val)) {
						switch word {
						case "nav", "navbar", "sidebar", "footer", "cookie-banner", "advertisement", "ads":
							return
						}
					}
				}
			}
		}
		if block {
			appendText("\n")
			if n.Data == "li" {
				appendText("- ")
			}
		}
		if n.Type == html.TextNode {
			text := strings.Join(strings.Fields(n.Data), " ")
			if text != "" {
				appendText(text + " ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, depth+1)
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					link, err := url.Parse(a.Val)
					if err == nil {
						link = base.ResolveReference(link)
						if (link.Scheme == "http" || link.Scheme == "https") && link.User == nil {
							appendText("(" + link.String() + ") ")
						}
					}
					break
				}
			}
		}
		if block {
			appendText("\n")
		}
	}
	walk(root, 0)
	// Keep paragraph boundaries while removing indentation and empty blocks.
	lines := strings.Split(out.String(), "\n")
	clean := lines[:0]
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			clean = append(clean, line)
		}
	}
	body, bodyCut := capFetchText(strings.Join(clean, "\n"), limit)
	return body, title, cut || bodyCut, nil
}
