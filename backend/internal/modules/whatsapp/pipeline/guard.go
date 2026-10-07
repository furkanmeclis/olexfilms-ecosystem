package pipeline

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxPartRunes bounds one outgoing WhatsApp message; longer answers are
// split at paragraph, line or word boundaries.
const MaxPartRunes = 4000

// Secrets never leave: an answer containing one is not sent at all.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{10,}`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{20,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{8,}\.eyJ[A-Za-z0-9_\-]{8,}\.[A-Za-z0-9_\-]{8,}`),
	regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._\-]{16,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(api[_\-]?key|secret|password|access[_\-]?token)\s*[:=]\s*\S{8,}`),
}

// Internal identifiers are masked: numeric *_id values and bare uuids
// (uuids inside a URL stay, e.g. a portal link).
var (
	internalIDPattern = regexp.MustCompile(`(?i)\b[a-z]+_id\s*[:=]\s*"?\d+"?`)
	uuidPattern       = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	urlPattern        = regexp.MustCompile(`https?://[^\s<>()]+`)
)

const redacted = "[…]"

// guardResult is the checked answer.
type guardResult struct {
	Text string
	// Blocked: the answer leaked a secret and must not be sent.
	Blocked bool
	// Redacted counts masked internal identifiers.
	Redacted int
}

// guard checks an AI answer before delivery: secret leaks block it,
// internal identifiers are masked and Markdown becomes WhatsApp formatting.
func guard(answer string) guardResult {
	for _, re := range secretPatterns {
		if re.MatchString(answer) {
			return guardResult{Blocked: true}
		}
	}
	out := guardResult{}
	answer = internalIDPattern.ReplaceAllStringFunc(answer, func(string) string {
		out.Redacted++
		return redacted
	})
	answer = maskUUIDs(answer, &out.Redacted)
	out.Text = strings.TrimSpace(toWhatsApp(answer))
	return out
}

// maskUUIDs replaces uuids outside URLs.
func maskUUIDs(s string, n *int) string {
	urls := urlPattern.FindAllStringIndex(s, -1)
	inURL := func(start, end int) bool {
		for _, u := range urls {
			if start >= u[0] && end <= u[1] {
				return true
			}
		}
		return false
	}
	var sb strings.Builder
	last := 0
	for _, m := range uuidPattern.FindAllStringIndex(s, -1) {
		if inURL(m[0], m[1]) {
			continue
		}
		sb.WriteString(s[last:m[0]])
		sb.WriteString(redacted)
		last = m[1]
		*n++
	}
	sb.WriteString(s[last:])
	return sb.String()
}

var (
	mdBold      = regexp.MustCompile(`\*\*(.+?)\*\*|__(.+?)__`)
	mdStrike    = regexp.MustCompile(`~~(.+?)~~`)
	mdHeading   = regexp.MustCompile(`(?m)^[ \t]*#{1,6}[ \t]+(.+?)[ \t#]*$`)
	mdLink      = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^)\s]+)\)`)
	mdBullet    = regexp.MustCompile(`(?m)^([ \t]*)[-*+][ \t]+`)
	mdRule      = regexp.MustCompile(`(?m)^[ \t]*(-{3,}|\*{3,}|_{3,})[ \t]*$\n?`)
	mdTableRule = regexp.MustCompile(`(?m)^[ \t]*\|?[ \t:]*-{3,}[-| \t:]*$\n?`)
	htmlTag     = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	blankLines  = regexp.MustCompile(`\n{3,}`)
)

// toWhatsApp converts the Markdown the model may produce to WhatsApp
// formatting: **bold** → *bold*, headings → bold lines, [text](url) →
// "text: url", ~~x~~ → ~x~, list markers → •, rules and HTML removed.
func toWhatsApp(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = htmlTag.ReplaceAllString(s, "")
	s = mdRule.ReplaceAllString(s, "")
	s = mdTableRule.ReplaceAllString(s, "")
	s = mdLink.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdLink.FindStringSubmatch(m)
		if strings.TrimSpace(sub[1]) == sub[2] {
			return sub[2]
		}
		return sub[1] + ": " + sub[2]
	})
	// Bullets before bold, so "* item" lists do not become bold markers.
	s = mdBullet.ReplaceAllString(s, "$1• ")
	s = mdBold.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdBold.FindStringSubmatch(m)
		inner := sub[1]
		if inner == "" {
			inner = sub[2]
		}
		return "*" + inner + "*"
	})
	s = mdHeading.ReplaceAllStringFunc(s, func(m string) string {
		sub := mdHeading.FindStringSubmatch(m)
		inner := strings.Trim(sub[1], "*")
		return "*" + inner + "*"
	})
	s = mdStrike.ReplaceAllString(s, "~$1~")
	return blankLines.ReplaceAllString(s, "\n\n")
}

// splitMessage splits text into parts of at most max runes, preferring
// paragraph, then line, then word boundaries.
func splitMessage(text string, max int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	var parts []string
	for utf8.RuneCountInString(text) > max {
		runes := []rune(text)
		window := string(runes[:max])
		cut := -1
		for _, sep := range []string{"\n\n", "\n", " "} {
			if i := strings.LastIndex(window, sep); i > len(window)/3 {
				cut = i
				break
			}
		}
		if cut < 0 {
			cut = len(window)
		}
		parts = append(parts, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		parts = append(parts, text)
	}
	return parts
}
