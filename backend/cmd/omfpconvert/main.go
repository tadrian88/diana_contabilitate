package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type fragment struct {
	ID          string `json:"id"`
	CitationKey string `json:"citationKey"`
	Heading     string `json:"heading"`
	Text        string `json:"text"`
	ContentHash string `json:"contentHash"`
	Ordinal     int    `json:"ordinal"`
}
type snapshot struct {
	FormatVersion                string         `json:"formatVersion"`
	Importable                   bool           `json:"importable"`
	RequiresOfficialSourceReview bool           `json:"requiresOfficialSourceReview"`
	Source                       map[string]any `json:"source"`
	Version                      map[string]any `json:"version"`
	Fragments                    []fragment     `json:"fragments"`
}

var orderArticle = regexp.MustCompile(`^ART\.\s*([0-9]+)$`)
var regulationPoint = regexp.MustCompile(`^([0-9]{1,3})\.\s*[-‐]\s*(.*)$`)
var accountHeading = regexp.MustCompile(`^Contul\s+([0-9A-Za-z]+)\s+"([^"]+)"$`)

func main() {
	input := flag.String("input", "", "OMFP PDF")
	output := flag.String("output", "", "destination JSON")
	flag.Parse()
	if *input == "" || *output == "" {
		fatal("-input and -output are required")
	}
	pdf, err := os.ReadFile(*input)
	if err != nil {
		fatal(err.Error())
	}
	pdfHash := sha256.Sum256(pdf)
	temporary, err := os.CreateTemp("", "omfp-1802-*.txt")
	if err != nil {
		fatal(err.Error())
	}
	textPath := temporary.Name()
	temporary.Close()
	defer os.Remove(textPath)
	command := exec.Command("pdftotext", "-layout", *input, textPath)
	if raw, runErr := command.CombinedOutput(); runErr != nil {
		fatal(fmt.Sprintf("pdftotext: %v: %s", runErr, raw))
	}
	raw, err := os.ReadFile(textPath)
	if err != nil {
		fatal(err.Error())
	}
	fragments := extract(string(raw))
	if len(fragments) < 750 {
		fatal(fmt.Sprintf("unsafe extraction: only %d fragments", len(fragments)))
	}
	result := snapshot{FormatVersion: "DIANA_LEGISLATION_SOURCE_SNAPSHOT_V1", Importable: false, RequiresOfficialSourceReview: true,
		Source:  map[string]any{"id": "ro-omfp-1802-2014", "kind": "ORDER", "title": "OMFP nr. 1802/2014 pentru aprobarea Reglementărilor contabile", "issuer": "Ministerul Finanțelor Publice", "jurisdiction": "RO", "officialUrl": nil, "sourceFileSha256": hex.EncodeToString(pdfHash[:])},
		Version: map[string]any{"label": "forma publicată în Monitorul Oficial nr. 963/30.12.2014", "publishedAt": "2014-12-30", "effectiveFrom": "2015-01-01", "convertedAt": time.Now().UTC().Format(time.RFC3339), "note": "PDF-ul nu declară un URL oficial și nu este o formă consolidată cu modificările ulterioare."}, Fragments: fragments}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	encoded = append(encoded, '\n')
	if err = os.WriteFile(*output, encoded, 0o644); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("converted %d OMFP fragments to %s\n", len(fragments), *output)
}

func extract(raw string) []fragment {
	raw = strings.ReplaceAll(raw, "\r", "")
	lines := strings.Split(raw, "\n")
	result := []fragment{}
	inAnnex := false
	expectedPoint := 1
	mode := ""
	key, heading := "", ""
	body := []string{}
	flush := func() {
		if key == "" {
			return
		}
		text := normalize(strings.Join(append([]string{heading}, body...), "\n"))
		appendBounded(&result, key, heading, text)
		key, heading, body = "", "", nil
	}
	for _, sourceLine := range lines {
		line := normalize(strings.TrimPrefix(sourceLine, "\f"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "ANEXA 1") {
			flush()
			inAnnex = true
			mode = ""
			continue
		}
		if !inAnnex {
			if match := orderArticle.FindStringSubmatch(line); match != nil {
				flush()
				mode = "order"
				key = "OMFP 1802/2014 art. " + match[1]
				heading = "ART. " + match[1]
				continue
			}
			if mode == "order" {
				body = append(body, line)
			}
			continue
		}
		if expectedPoint <= 597 {
			if match := regulationPoint.FindStringSubmatch(line); match != nil {
				number, _ := strconv.Atoi(match[1])
				if number >= expectedPoint && number <= 597 {
					flush()
					mode = "point"
					key = "OMFP 1802/2014 pct. " + match[1]
					heading = match[1] + ". -"
					expectedPoint = number + 1
					key = "OMFP 1802/2014 pct. " + match[1]
					body = nil
					if match[2] != "" {
						body = append(body, match[2])
					}
					continue
				}
			}
		}
		if expectedPoint > 597 {
			if match := accountHeading.FindStringSubmatch(line); match != nil {
				flush()
				mode = "account"
				key = "OMFP 1802/2014 contul " + match[1]
				heading = "Contul " + match[1] + " \"" + match[2] + "\""
				continue
			}
		}
		if mode != "" {
			body = append(body, line)
		}
	}
	flush()
	for i := range result {
		result[i].Ordinal = i + 1
	}
	return result
}

func appendBounded(result *[]fragment, key, heading, text string) {
	const limit = 12000
	parts := splitBounded(text, limit)
	for index, part := range parts {
		citation := key
		partHeading := heading
		if len(parts) > 1 {
			citation = fmt.Sprintf("%s · fragment %d/%d", key, index+1, len(parts))
			partHeading = fmt.Sprintf("%s [fragment %d/%d]", heading, index+1, len(parts))
		}
		sum := sha256.Sum256([]byte(part))
		idSource := fmt.Sprintf("%s:%d", key, index+1)
		idHash := sha256.Sum256([]byte(idSource))
		*result = append(*result, fragment{ID: "omfp-1802-2014-" + hex.EncodeToString(idHash[:8]), CitationKey: citation, Heading: partHeading, Text: part, ContentHash: hex.EncodeToString(sum[:])})
	}
}
func splitBounded(text string, limit int) []string {
	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}
	parts := []string{}
	for len(runes) > 0 {
		end := min(limit, len(runes))
		if end < len(runes) {
			for candidate := end; candidate > limit/2; candidate-- {
				if runes[candidate-1] == '.' || runes[candidate-1] == ';' {
					end = candidate
					break
				}
			}
		}
		parts = append(parts, strings.TrimSpace(string(runes[:end])))
		runes = runes[end:]
		for len(runes) > 0 && unicode.IsSpace(runes[0]) {
			runes = runes[1:]
		}
	}
	return parts
}
func normalize(value string) string {
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) }), " ")
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
