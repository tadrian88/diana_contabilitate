package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

type article struct {
	ID          string `json:"id"`
	Anchor      string `json:"anchor"`
	CitationKey string `json:"citationKey"`
	Heading     string `json:"heading"`
	Text        string `json:"text"`
	ContentHash string `json:"contentHash"`
	Ordinal     int    `json:"ordinal"`
}

type output struct {
	FormatVersion               string            `json:"formatVersion"`
	Importable                  bool              `json:"importable"`
	RequiresEffectiveDateReview bool              `json:"requiresEffectiveDateReview"`
	Source                      map[string]string `json:"source"`
	Snapshot                    map[string]any    `json:"snapshot"`
	Articles                    []article         `json:"articles"`
}

var articleAnchor = regexp.MustCompile(`^A[0-9]`)
var citationPattern = regexp.MustCompile(`^ART\.\s*[0-9]+(?:\^[0-9]+)?`)
var nonID = regexp.MustCompile(`[^a-z0-9]+`)

func main() {
	input := flag.String("input", "", "ANAF HTML file")
	destination := flag.String("output", "", "destination JSON file")
	flag.Parse()
	if *input == "" || *destination == "" {
		fatal("-input and -output are required")
	}
	file, err := os.Open(*input)
	if err != nil {
		fatal(err.Error())
	}
	defer file.Close()
	doc, err := html.Parse(file)
	if err != nil {
		fatal(err.Error())
	}
	articles, detected := extract(doc)
	if len(articles) < 400 {
		fatal(fmt.Sprintf("unsafe extraction: only %d articles", len(articles)))
	}
	result := output{FormatVersion: "DIANA_LEGISLATION_SOURCE_SNAPSHOT_V1", Importable: false, RequiresEffectiveDateReview: true,
		Source:   map[string]string{"id": "ro-legea-227-2015", "kind": "LAW", "title": "Legea nr. 227/2015 privind Codul fiscal", "issuer": "Parlamentul României", "jurisdiction": "RO", "officialUrl": "https://static.anaf.ro/static/10/Anaf/legislatie/Cod_fiscal_norme_2023.htm"},
		Snapshot: map[string]any{"detectedUpdate": detected, "updateActAdoptedAt": "2026-05-04", "updateActPublishedAt": "2026-05-08", "updateActOfficialUrl": "https://legislatie.just.ro/Public/DetaliiDocumentAfis/310375", "convertedAt": time.Now().UTC().Format(time.RFC3339), "effectiveFrom": nil, "note": "Publicarea OUG 38/2026 nu stabilește automat o singură dată de aplicabilitate pentru toate prevederile consolidării; perioadele trebuie verificate înainte de import."}, Articles: articles}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	raw = append(raw, '\n')
	if err = os.WriteFile(*destination, raw, 0o644); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("converted %d Code articles to %s\n", len(articles), *destination)
}

func extract(root *html.Node) ([]article, string) {
	items := []article{}
	detected := ""
	stopped := false
	var current *article
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if stopped {
			return
		}
		if node.Type == html.ElementNode {
			text := normalize(render(node))
			if node.Data == "span" && strings.HasPrefix(strings.ToLower(text), "ultima actualizare:") && len([]rune(text)) < 160 && detected == "" {
				detected = text
			}
			if strings.HasPrefix(text, "NORME METODOLOGICE de aplicare a Legii nr. 227/2015") && len(items) > 0 {
				stopped = true
				return
			}
			if node.Data == "p" && hasClass(node, "stilArticol") {
				if anchor := namedArticleAnchor(node); anchor != "" {
					if current != nil {
						finish(current)
						items = append(items, *current)
					}
					heading := normalize(render(node))
					citation := citationPattern.FindString(heading)
					if citation == "" {
						citation = heading
					}
					slug := strings.Trim(nonID.ReplaceAllString(strings.ToLower(citation), "-"), "-")
					current = &article{ID: "cod-fiscal-227-2015-" + slug, Anchor: anchor, CitationKey: citation, Heading: heading, Ordinal: len(items) + 1, Text: heading}
					return
				}
			}
			if current != nil && ((node.Data == "p" && (hasClass(node, "stilParagraf") || hasClass(node, "stilFormula"))) || node.Data == "table") {
				if text != "" {
					current.Text += "\n" + text
				}
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if current != nil && !stopped {
		finish(current)
		items = append(items, *current)
	} else if current != nil && stopped {
		finish(current)
		items = append(items, *current)
	}
	return items, detected
}

func finish(item *article) {
	item.Text = strings.TrimSpace(item.Text)
	sum := sha256.Sum256([]byte(item.Text))
	item.ContentHash = hex.EncodeToString(sum[:])
}
func namedArticleAnchor(node *html.Node) string {
	var found string
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if found != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "name" && articleAnchor.MatchString(a.Val) {
					found = a.Val
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(node)
	return found
}
func hasClass(node *html.Node, want string) bool {
	for _, a := range node.Attr {
		if a.Key == "class" {
			for _, v := range strings.Fields(a.Val) {
				if v == want {
					return true
				}
			}
		}
	}
	return false
}
func render(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	if node.Type == html.ElementNode && node.Data == "a" {
		var href string
		for _, a := range node.Attr {
			if a.Key == "href" {
				href = a.Val
			}
		}
		if (strings.Contains(href, "#B") || strings.Contains(href, "#NM_")) && strings.EqualFold(normalize(rawText(node)), "Norme metodologice") {
			return ""
		}
	}
	prefix, suffix := "", ""
	if node.Type == html.ElementNode && node.Data == "sup" {
		prefix = "^"
	}
	if node.Type == html.ElementNode && (node.Data == "br" || node.Data == "tr") {
		suffix = "\n"
	}
	var b strings.Builder
	b.WriteString(prefix)
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(render(c))
		if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
			b.WriteString(" | ")
		}
	}
	b.WriteString(suffix)
	return b.String()
}
func rawText(node *html.Node) string {
	if node.Type == html.TextNode {
		return node.Data
	}
	var b strings.Builder
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(rawText(c))
	}
	return b.String()
}
func normalize(value string) string {
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) }), " ")
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
