package storage_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/storage"
)

func TestIsSVG(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{`<svg xmlns="http://www.w3.org/2000/svg"/>`, true},
		{"\xef\xbb\xbf  <?xml version=\"1.0\"?>\n<!-- x -->\n<svg/>", true},
		{`<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "x"><SVG/>`, true},
		{"\x89PNG\r\n\x1a\n", false},
		{"<html><body>hi</body></html>", false},
		{"", false},
	} {
		if got := storage.IsSVG([]byte(tc.in)); got != tc.want {
			t.Errorf("IsSVG(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeSVGKeepsLogo(t *testing.T) {
	t.Parallel()
	in := `<?xml version="1.0" encoding="UTF-8"?>
<!-- Generator: Illustrator -->
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd" [
	<!ENTITY ns_svg "http://www.w3.org/2000/svg">
	<!ENTITY ns_xlink "http://www.w3.org/1999/xlink">
]>
<svg version="1.1" xmlns="&ns_svg;" xmlns:xlink="&ns_xlink;" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape"
     viewBox="0 0 100 50" width="100" height="50" inkscape:version="1.0">
  <defs>
    <linearGradient id="g1"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></linearGradient>
    <style>.a{fill:url(#g1)} .b > path {opacity:.5}</style>
  </defs>
  <title>Nissan &amp; Co</title>
  <a href="https://nissan.example"><g class="a"><path d="M0 0L10 10Z" fill="url(#g1)" style="stroke:url('#g1')"/></g></a>
  <use xlink:href="#g1" x="1"/>
  <image href="data:image/png;base64,iVBORw0KGgo=" width="1" height="1"/>
  <text xml:space="preserve">NISSAN</text>
</svg>`
	out, err := storage.SanitizeSVG([]byte(in))
	if err != nil {
		t.Fatalf("SanitizeSVG: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" version="1.1" viewBox="0 0 100 50"`,
		`<linearGradient id="g1">`, `<stop offset="0" stop-color="#fff">`,
		`fill="url(#g1)"`, `style="stroke:url(&#39;#g1&#39;)"`,
		`.a{fill:url(#g1)} .b &gt; path {opacity:.5}`,
		`<title>Nissan &amp; Co</title>`,
		`<path d="M0 0L10 10Z"`, // <a> unwrapped, children kept
		`<use xlink:href="#g1" x="1">`,
		`<image href="data:image/png;base64,iVBORw0KGgo="`,
		`<text xml:space="preserve">NISSAN</text>`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
	for _, bad := range []string{"<a", "nissan.example", "inkscape", "DOCTYPE", "<!--", "<?xml"} {
		if strings.Contains(s, bad) {
			t.Errorf("output keeps %q:\n%s", bad, s)
		}
	}
	// Idempotent: a stored logo sanitizes to itself (stable migrator hash).
	again, err := storage.SanitizeSVG(out)
	if err != nil || string(again) != s {
		t.Errorf("second pass differs (err %v):\n%s\n---\n%s", err, s, again)
	}
}

func TestSanitizeSVGStripsMalicious(t *testing.T) {
	t.Parallel()
	in := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" onload="alert(1)" ONCLICK="x()">
  <script>alert('script')</script>
  <script xlink:href="https://evil.example/x.js"/>
  <foreignObject><iframe xmlns="http://www.w3.org/1999/xhtml" src="javascript:alert(2)"/><body onload="alert(3)"/></foreignObject>
  <iframe src="https://evil.example"/>
  <embed src="https://evil.example/e.swf"/>
  <object data="https://evil.example/o"/>
  <html:script xmlns:html="http://www.w3.org/1999/xhtml">alert(4)</html:script>
  <animate attributeName="href" to="javascript:alert(5)"/>
  <set attributeName="onmouseover" to="alert(6)"/>
  <style>@import url("https://evil.example/a.css"); .x{background:url(https://evil.example/t.png)} .y{fill:url(//evil.example/p)}</style>
  <style>.z{background:\75rl(https://evil.example)}</style>
  <g onmouseover="alert(7)" fill="url(https://evil.example/#a)" stroke="javascript:alert(8)">
    <use href="https://evil.example/s.svg#a"/>
    <use xlink:href="//evil.example/s.svg#a"/>
    <use href="  java&#x09;script:alert(9)"/>
    <image href="data:text/html;base64,PHNjcmlwdD4="/>
    <image xlink:href="data:image/svg+xml;base64,PHN2Zz4="/>
    <image href="https://evil.example/i.png"/>
    <rect width="1" height="1" style="fill:url(https://evil.example/f)"/>
    <path d="M0 0" filter="url(#ok)"/>
  </g>
</svg>`
	out, err := storage.SanitizeSVG([]byte(in))
	if err != nil {
		t.Fatalf("SanitizeSVG: %v", err)
	}
	s := strings.ToLower(string(out))
	for _, bad := range []string{
		"<script", "alert", "onload", "onclick", "onmouseover", "foreignobject", "<iframe", "<embed",
		"<object", "<animate", "<set", "@import", "evil.example", "javascript", "data:text", "svg+xml;base64", "\\75rl",
	} {
		if strings.Contains(s, bad) {
			t.Errorf("output keeps %q:\n%s", bad, out)
		}
	}
	for _, want := range []string{`filter="url(#ok)"`, `<rect width="1" height="1" style="fill:none">`, `<g>`} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestSanitizeSVGRejects(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]string{
		"empty":        "",
		"html root":    `<html><svg/></html>`,
		"foreign root": `<svg xmlns="http://www.w3.org/1999/xhtml"></svg>`,
		"not xml":      `<svg><g></svg>`,
		"unterminated": `<svg xmlns="http://www.w3.org/2000/svg"><g>`,
		"two roots":    `<svg/><svg/>`,
		"external entity": `<!DOCTYPE svg [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<svg xmlns="http://www.w3.org/2000/svg"><text>&xxe;</text></svg>`,
		"nested entity": `<!DOCTYPE svg [<!ENTITY a "x"><!ENTITY b "&a;&a;">]>
<svg xmlns="http://www.w3.org/2000/svg"><text>&b;</text></svg>`,
		"deep": strings.Repeat("<g>", 300) + strings.Repeat("</g>", 300),
	} {
		if name == "deep" {
			in = `<svg xmlns="http://www.w3.org/2000/svg">` + in + `</svg>`
		}
		if _, err := storage.SanitizeSVG([]byte(in)); err == nil {
			t.Errorf("%s: want error", name)
		} else if name != "empty" && !errors.Is(err, storage.ErrInvalidSVG) {
			t.Errorf("%s: err = %v, want ErrInvalidSVG", name, err)
		}
	}
	big := `<svg xmlns="http://www.w3.org/2000/svg"><desc>` + strings.Repeat("a", storage.MaxLogoBytes) + `</desc></svg>`
	if _, err := storage.SanitizeSVG([]byte(big)); err == nil {
		t.Error("oversized SVG must fail")
	}
}

func TestMIMEFromLogoKeySVG(t *testing.T) {
	t.Parallel()
	if got := storage.MIMEFromLogoKey("vehicle-brands/x/logo-abc.svg"); got != storage.SVGMIME {
		t.Fatalf("svg key mime = %q", got)
	}
	// Uploads of other logo kinds still refuse SVG.
	if _, err := storage.DetectLogoMIME("image/svg+xml", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)); err == nil {
		t.Fatal("DetectLogoMIME must keep refusing SVG")
	}
}
