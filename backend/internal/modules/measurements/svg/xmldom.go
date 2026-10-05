package svg

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

// A minimal XML tree with libxml2-compatible serialization. The legacy
// fill service edits the pointed SVGs through PHP's DOMDocument (libxml2)
// and returns DOMDocument::saveXML output, so the port has to reproduce
// libxml2's writer: namespace declarations before the other attributes,
// attribute whitespace normalized on parse, empty elements self-closed and
// libxml2's escaping rules for text and attribute values.

type nodeKind int

const (
	elementNode nodeKind = iota
	textNode
	commentNode
	procInstNode
	directiveNode
)

type attr struct {
	name  string // qualified name as written (prefix:local)
	value string
}

type node struct {
	kind     nodeKind
	name     string // element tag (qualified) or processing instruction target
	nsDecls  []attr // xmlns / xmlns:p declarations, in source order
	attrs    []attr // other attributes, in source order
	text     string // text, comment, PI or directive content
	parent   *node
	children []*node
}

// document is a parsed XML file: the prolog nodes and the root element.
type document struct {
	root *node
}

func qualified(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	return n.Space + ":" + n.Local
}

// normalizeAttr applies XML attribute-value normalization for literal
// whitespace (a parser replaces tab, LF and CR with a space).
func normalizeAttr(v string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\n', '\r':
			return ' '
		}
		return r
	}, v)
}

var errNoRoot = errors.New("svg: document has no root element")

// parseXML builds the tree. RawToken keeps prefixes as written; the SVG
// assets declare only the default namespace.
func parseXML(src string) (*document, error) {
	dec := xml.NewDecoder(strings.NewReader(src))
	dec.Strict = true
	doc := &document{}
	var cur *node
	for {
		tok, err := dec.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			el := &node{kind: elementNode, name: qualified(t.Name), parent: cur}
			for _, a := range t.Attr {
				name := qualified(a.Name)
				v := attr{name: name, value: normalizeAttr(a.Value)}
				if name == "xmlns" || strings.HasPrefix(name, "xmlns:") {
					el.nsDecls = append(el.nsDecls, v)
					continue
				}
				el.attrs = append(el.attrs, v)
			}
			if cur == nil {
				if doc.root != nil {
					return nil, errors.New("svg: more than one root element")
				}
				doc.root = el
			} else {
				cur.children = append(cur.children, el)
			}
			cur = el
		case xml.EndElement:
			if cur == nil {
				return nil, errors.New("svg: unbalanced end element")
			}
			cur = cur.parent
		case xml.CharData:
			if cur != nil {
				cur.children = append(cur.children, &node{kind: textNode, text: string(t), parent: cur})
			}
		case xml.Comment:
			if cur != nil {
				cur.children = append(cur.children, &node{kind: commentNode, text: string(t), parent: cur})
			}
		case xml.ProcInst:
			if cur != nil {
				cur.children = append(cur.children, &node{kind: procInstNode, name: t.Target, text: string(t.Inst), parent: cur})
			}
		case xml.Directive:
			// DOCTYPE in the prolog; never serialized (saveXML of an element).
		}
	}
	if doc.root == nil {
		return nil, errNoRoot
	}
	return doc, nil
}

func (n *node) attr(name string) (string, bool) {
	for _, a := range n.attrs {
		if a.name == name {
			return a.value, true
		}
	}
	return "", false
}

// setAttr is DOMElement::setAttribute: replace in place or append.
func (n *node) setAttr(name, value string) {
	for i := range n.attrs {
		if n.attrs[i].name == name {
			n.attrs[i].value = value
			return
		}
	}
	n.attrs = append(n.attrs, attr{name: name, value: value})
}

// walk visits the subtree below n in document order (n excluded).
func (n *node) walk(fn func(*node)) {
	for _, c := range n.children {
		if c.kind != elementNode {
			continue
		}
		fn(c)
		c.walk(fn)
	}
}

// byID is XPath //*[@id="id"] item(0): the first element in document order.
func (d *document) byID(id string) *node {
	if v, ok := d.root.attr("id"); ok && v == id {
		return d.root
	}
	var found *node
	d.root.walk(func(c *node) {
		if found != nil {
			return
		}
		if v, ok := c.attr("id"); ok && v == id {
			found = c
		}
	})
	return found
}

// allByID returns every element with the id in document order.
func (d *document) allByID(id string) []*node {
	var out []*node
	if v, ok := d.root.attr("id"); ok && v == id {
		out = append(out, d.root)
	}
	d.root.walk(func(c *node) {
		if v, ok := c.attr("id"); ok && v == id {
			out = append(out, c)
		}
	})
	return out
}

// serialize is DOMDocument::saveXML($node) for an element.
func serialize(n *node) string {
	var b bytes.Buffer
	writeNode(&b, n)
	return b.String()
}

func writeNode(b *bytes.Buffer, n *node) {
	switch n.kind {
	case textNode:
		escapeText(b, n.text)
	case commentNode:
		b.WriteString("<!--")
		b.WriteString(n.text)
		b.WriteString("-->")
	case procInstNode:
		b.WriteString("<?")
		b.WriteString(n.name)
		if n.text != "" {
			b.WriteByte(' ')
			b.WriteString(n.text)
		}
		b.WriteString("?>")
	case elementNode:
		b.WriteByte('<')
		b.WriteString(n.name)
		for _, a := range n.nsDecls {
			writeAttr(b, a)
		}
		for _, a := range n.attrs {
			writeAttr(b, a)
		}
		if len(n.children) == 0 {
			b.WriteString("/>")
			return
		}
		b.WriteByte('>')
		for _, c := range n.children {
			writeNode(b, c)
		}
		b.WriteString("</")
		b.WriteString(n.name)
		b.WriteByte('>')
	}
}

func writeAttr(b *bytes.Buffer, a attr) {
	b.WriteByte(' ')
	b.WriteString(a.name)
	b.WriteString(`="`)
	escapeAttr(b, a.value)
	b.WriteByte('"')
}

// escapeText follows libxml2's xmlEscapeContent.
func escapeText(b *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '\r':
			b.WriteString("&#13;")
		default:
			b.WriteByte(c)
		}
	}
}

// escapeAttr follows libxml2's attribute serialization.
func escapeAttr(b *bytes.Buffer, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		case '\n':
			b.WriteString("&#10;")
		case '\r':
			b.WriteString("&#13;")
		case '\t':
			b.WriteString("&#9;")
		default:
			b.WriteByte(c)
		}
	}
}
