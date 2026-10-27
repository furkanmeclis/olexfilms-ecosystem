package storage

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SVG support is limited to car brand logos (TEC-1020): legacy brands ship
// vector logos and the decision was to keep them vector, not rasterize. Every
// SVG is rebuilt through SanitizeSVG before it reaches storage, and it is
// served with SVGContentSecurityPolicy so even a sanitizer miss cannot run
// script or load anything. Other logo kinds stay raster-only.

// SVGMIME is the Content-Type of stored SVG logos.
const SVGMIME = "image/svg+xml"

// SVGContentSecurityPolicy is sent with every stored SVG: no script, no
// network, inline styles and data: images only.
const SVGContentSecurityPolicy = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox"

// ErrInvalidSVG is returned for bytes that are not a well-formed SVG document.
var ErrInvalidSVG = errors.New("logo must be a valid SVG document")

const (
	svgNS   = "http://www.w3.org/2000/svg"
	xlinkNS = "http://www.w3.org/1999/xlink"
	xmlNS   = "http://www.w3.org/XML/1998/namespace"

	svgMaxDepth = 256
)

// svgElements is the element allowlist. Anything else (script, foreignObject,
// iframe, embed, object, animate/set, metadata, editor namespaces …) is
// dropped together with its subtree.
var svgElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "symbol": true, "use": true,
	"title": true, "desc": true, "switch": true, "style": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true,
	"polyline": true, "polygon": true, "text": true, "tspan": true, "textPath": true,
	"linearGradient": true, "radialGradient": true, "stop": true,
	"clipPath": true, "mask": true, "pattern": true, "marker": true, "image": true,
	"filter": true, "feBlend": true, "feColorMatrix": true, "feComponentTransfer": true,
	"feComposite": true, "feConvolveMatrix": true, "feDiffuseLighting": true,
	"feDisplacementMap": true, "feDistantLight": true, "feDropShadow": true,
	"feFlood": true, "feFuncA": true, "feFuncB": true, "feFuncG": true, "feFuncR": true,
	"feGaussianBlur": true, "feImage": true, "feMerge": true, "feMergeNode": true,
	"feMorphology": true, "feOffset": true, "fePointLight": true,
	"feSpecularLighting": true, "feSpotLight": true, "feTile": true, "feTurbulence": true,
}

// svgUnwrap elements are removed but their children are kept (a logo
// wrapped in a link still renders, the link does not).
var svgUnwrap = map[string]bool{"a": true}

var (
	svgAttrName   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	svgEntityDecl = regexp.MustCompile(`<!ENTITY\s+([A-Za-z_][A-Za-z0-9_.-]*)\s+(?:"([^"<&%]*)"|'([^'<&%]*)')\s*>`)
	svgDataImage  = regexp.MustCompile(`^data:image/(png|jpeg|jpg|gif|webp);base64,[A-Za-z0-9+/=\s]*$`)
	cssImport     = regexp.MustCompile(`(?i)@import[^;]*;?`)
	cssURL        = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)]*))\s*\)`)
)

// IsSVG reports whether body looks like an SVG/XML document (by content, not
// by a declared type). SanitizeSVG still verifies the root element.
func IsSVG(body []byte) bool {
	b := bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))
	b = bytes.TrimLeft(b, " \t\r\n")
	if len(b) == 0 || b[0] != '<' {
		return false
	}
	if len(b) > 4096 {
		b = b[:4096]
	}
	return bytes.Contains(bytes.ToLower(b), []byte("<svg"))
}

// SanitizeSVG parses body as XML and re-serializes an allowlisted subset:
// only SVG elements from svgElements, no event handlers, no foreign
// namespaces, href only to local fragments (and data:image raster for
// <image>/<feImage>), CSS without @import or external url(). It fails when
// the document is not well-formed or its root is not <svg>.
func SanitizeSVG(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return nil, ErrInvalidSVG
	}
	if len(body) > MaxLogoBytes {
		return nil, fmt.Errorf("logo must be at most 2 MiB")
	}
	body = bytes.TrimPrefix(body, []byte("\xef\xbb\xbf"))

	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = true
	dec.CharsetReader = svgCharsetReader
	// Simple internal-subset entities (Illustrator's &ns_svg; etc.). Only
	// literal values without '&' are taken, so expansion cannot recurse.
	if ents := svgEntityDecl.FindAllSubmatch(body, -1); len(ents) > 0 {
		dec.Entity = map[string]string{}
		for _, m := range ents {
			v := string(m[2])
			if len(m[3]) > 0 {
				v = string(m[3])
			}
			dec.Entity[string(m[1])] = v
		}
	}

	var (
		out      bytes.Buffer
		stack    []svgFrame // one frame per open source element
		skip     int        // >0 while inside a dropped subtree
		rootSeen bool
		rootDone bool
		inStyle  bool
		styleBuf strings.Builder
	)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSVG, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if rootDone {
				return nil, fmt.Errorf("%w: content after the root element", ErrInvalidSVG)
			}
			if len(stack) >= svgMaxDepth {
				return nil, fmt.Errorf("%w: nesting too deep", ErrInvalidSVG)
			}
			if !rootSeen {
				rootSeen = true
				if t.Name.Local != "svg" || !svgElementNS(t.Name.Space) {
					return nil, fmt.Errorf("%w: root element is not <svg>", ErrInvalidSVG)
				}
			}
			if skip > 0 {
				skip++
				stack = append(stack, svgFrame{})
				continue
			}
			local := t.Name.Local
			switch {
			case svgElementNS(t.Name.Space) && svgElements[local]:
				writeSVGStart(&out, t, len(stack) == 0)
				stack = append(stack, svgFrame{written: true, name: local})
				if local == "style" {
					inStyle = true
					styleBuf.Reset()
				}
			case svgElementNS(t.Name.Space) && svgUnwrap[local]:
				stack = append(stack, svgFrame{})
			default:
				skip = 1
				stack = append(stack, svgFrame{})
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, ErrInvalidSVG
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if skip > 0 {
				skip--
				continue
			}
			if f.written {
				if f.name == "style" {
					inStyle = false
					writeEscaped(&out, sanitizeCSS(styleBuf.String()))
				}
				out.WriteString("</" + f.name + ">")
			}
			if len(stack) == 0 {
				rootDone = true
			}
		case xml.CharData:
			if skip > 0 || len(stack) == 0 {
				continue
			}
			if inStyle {
				styleBuf.Write(t)
				continue
			}
			writeEscaped(&out, string(t))
		}
		// Comments, processing instructions and directives (DOCTYPE) are
		// dropped.
	}
	if !rootDone {
		return nil, fmt.Errorf("%w: no complete <svg> root", ErrInvalidSVG)
	}
	if out.Len() > MaxLogoBytes {
		return nil, fmt.Errorf("logo must be at most 2 MiB")
	}
	return out.Bytes(), nil
}

type svgFrame struct {
	written bool
	name    string
}

func svgElementNS(space string) bool { return space == "" || space == svgNS }

func writeSVGStart(out *bytes.Buffer, t xml.StartElement, root bool) {
	local := t.Name.Local
	out.WriteString("<" + local)
	if root {
		out.WriteString(` xmlns="` + svgNS + `" xmlns:xlink="` + xlinkNS + `"`)
	}
	seen := map[string]bool{}
	for _, a := range t.Attr {
		name, ok := svgAttrOutName(a.Name)
		if !ok || seen[name] {
			continue
		}
		val, ok := sanitizeSVGAttr(local, name, a.Value)
		if !ok {
			continue
		}
		seen[name] = true
		out.WriteString(" " + name + `="`)
		writeEscaped(out, val)
		out.WriteString(`"`)
	}
	out.WriteString(">")
}

// svgAttrOutName maps a parsed attribute name to its output name, or false
// to drop it (namespace declarations, foreign namespaces, event handlers).
func svgAttrOutName(n xml.Name) (string, bool) {
	local := n.Local
	if !svgAttrName.MatchString(local) {
		return "", false
	}
	switch n.Space {
	case "":
		if local == "xmlns" {
			return "", false
		}
	case xlinkNS, "xlink":
		if local != "href" && local != "title" {
			return "", false
		}
		local = "xlink:" + local
	case xmlNS, "xml":
		if local != "space" && local != "lang" {
			return "", false
		}
		local = "xml:" + local
	default:
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(n.Local), "on") {
		return "", false
	}
	return local, true
}

// sanitizeSVGAttr checks one attribute value; false drops the attribute.
func sanitizeSVGAttr(element, name, value string) (string, bool) {
	compact := strings.ToLower(stripSpaceCtl(value))
	if strings.Contains(compact, "javascript:") || strings.Contains(compact, "vbscript:") ||
		strings.Contains(compact, "data:text") || strings.Contains(compact, "expression(") {
		return "", false
	}
	if name == "href" || name == "xlink:href" {
		v := strings.TrimSpace(value)
		if strings.HasPrefix(v, "#") {
			return v, true
		}
		if (element == "image" || element == "feImage") && svgDataImage.MatchString(v) {
			return v, true
		}
		return "", false
	}
	if name == "style" {
		return sanitizeCSS(value), true
	}
	if strings.Contains(compact, "url(") && !onlyLocalURLs(value) {
		return "", false
	}
	return value, true
}

// sanitizeCSS removes @import and replaces every url() that is not a local
// fragment or a raster data: image. CSS escapes could hide either, so a
// stylesheet with a backslash is dropped.
func sanitizeCSS(css string) string {
	if strings.Contains(css, `\`) {
		return ""
	}
	low := strings.ToLower(stripSpaceCtl(css))
	if strings.Contains(low, "javascript:") || strings.Contains(low, "expression(") ||
		strings.Contains(low, "-moz-binding") || strings.Contains(low, "behavior:") {
		return ""
	}
	css = cssImport.ReplaceAllString(css, "")
	return cssURL.ReplaceAllStringFunc(css, func(m string) string {
		if onlyLocalURLs(m) {
			return m
		}
		return "none"
	})
}

func onlyLocalURLs(s string) bool {
	for _, m := range cssURL.FindAllStringSubmatch(s, -1) {
		u := strings.TrimSpace(m[1] + m[2] + m[3])
		if strings.HasPrefix(u, "#") || svgDataImage.MatchString(u) {
			continue
		}
		return false
	}
	return true
}

func stripSpaceCtl(s string) string {
	return strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

func writeEscaped(out *bytes.Buffer, s string) {
	_ = xml.EscapeText(out, []byte(s))
}

// svgCharsetReader accepts UTF-8/ASCII and Latin-1 declared encodings.
func svgCharsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return input, nil
	case "iso-8859-1", "latin1", "latin-1", "windows-1252", "cp1252":
		raw, err := io.ReadAll(io.LimitReader(input, MaxLogoBytes+1))
		if err != nil {
			return nil, err
		}
		buf := make([]byte, 0, len(raw))
		for _, c := range raw {
			buf = utf8.AppendRune(buf, rune(c))
		}
		return bytes.NewReader(buf), nil
	}
	return nil, fmt.Errorf("unsupported SVG encoding %q", charset)
}
