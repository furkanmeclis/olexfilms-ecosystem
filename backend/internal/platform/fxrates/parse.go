// Package fxrates fetches daily exchange rates from TCMB and ECB, stores
// them with manual overrides and resolves the rate an order or a journal
// entry freezes (rate_snapshot, K7).
package fxrates

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"golang.org/x/text/encoding/charmap"
)

// Sources of a stored rate. Resolution order: manual > tcmb > ecb.
const (
	SourceManual = "manual"
	SourceTCMB   = "tcmb"
	SourceECB    = "ecb"
)

// Scale is the number of fraction digits stored (NUMERIC(20,10)).
const Scale = 10

// ErrParse: the provider document could not be read.
var ErrParse = errors.New("fxrates: parse failed")

// Rate is "1 Base = Rate Quote".
type Rate struct {
	Base  string
	Quote string
	Rate  string
}

// Day is one provider publication.
type Day struct {
	Source string
	Date   time.Time
	Rates  []Rate
}

type tcmbDoc struct {
	XMLName    xml.Name `xml:"Tarih_Date"`
	Date       string   `xml:"Date,attr"`
	Tarih      string   `xml:"Tarih,attr"`
	Currencies []struct {
		Code         string `xml:"CurrencyCode,attr"`
		Unit         string `xml:"Unit"`
		ForexSelling string `xml:"ForexSelling"`
	} `xml:"Currency"`
}

// ParseTCMB reads a TCMB "kurlar" XML (today.xml or YYYYMM/DDMMYYYY.xml).
// The rate is ForexSelling / Unit (JPY is quoted per 100); currencies
// without a forex selling price (XDR) are skipped. Every rate is CODE→TRY.
func ParseTCMB(r io.Reader) (Day, error) {
	var doc tcmbDoc
	dec := xml.NewDecoder(r)
	dec.CharsetReader = charsetReader
	if err := dec.Decode(&doc); err != nil {
		return Day{}, fmt.Errorf("%w: tcmb: %v", ErrParse, err)
	}
	date, err := tcmbDate(doc.Date, doc.Tarih)
	if err != nil {
		return Day{}, err
	}
	out := Day{Source: SourceTCMB, Date: date}
	for _, c := range doc.Currencies {
		code := strings.ToUpper(strings.TrimSpace(c.Code))
		if !validCode(code) || code == "TRY" {
			continue
		}
		selling := strings.TrimSpace(c.ForexSelling)
		if selling == "" {
			continue
		}
		price, ok := new(big.Rat).SetString(selling)
		if !ok || price.Sign() <= 0 {
			continue
		}
		unit := big.NewRat(1, 1)
		if u := strings.TrimSpace(c.Unit); u != "" {
			parsed, ok := new(big.Rat).SetString(u)
			if !ok || parsed.Sign() <= 0 {
				continue
			}
			unit = parsed
		}
		out.Rates = append(out.Rates, Rate{Base: code, Quote: "TRY", Rate: FormatRat(new(big.Rat).Quo(price, unit))})
	}
	if len(out.Rates) == 0 {
		return Day{}, fmt.Errorf("%w: tcmb: no rates", ErrParse)
	}
	return out, nil
}

// charsetReader accepts UTF-8 and the Turkish ISO-8859-9 / windows-1254
// encodings older TCMB archive files declare.
func charsetReader(label string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "", "utf-8", "utf8":
		return input, nil
	case "iso-8859-9", "latin5":
		return charmap.ISO8859_9.NewDecoder().Reader(input), nil
	case "windows-1254", "cp1254":
		return charmap.Windows1254.NewDecoder().Reader(input), nil
	default:
		return nil, fmt.Errorf("unsupported charset %q", label)
	}
}

func tcmbDate(date, tarih string) (time.Time, error) {
	if d, err := time.Parse("01/02/2006", strings.TrimSpace(date)); err == nil {
		return d, nil
	}
	if d, err := time.Parse("02.01.2006", strings.TrimSpace(tarih)); err == nil {
		return d, nil
	}
	return time.Time{}, fmt.Errorf("%w: tcmb: missing date", ErrParse)
}

type ecbDoc struct {
	Cube struct {
		Days []struct {
			Time  string `xml:"time,attr"`
			Rates []struct {
				Currency string `xml:"currency,attr"`
				Rate     string `xml:"rate,attr"`
			} `xml:"Cube"`
		} `xml:"Cube"`
	} `xml:"Cube"`
}

// ParseECB reads the ECB eurofxref daily XML. Every rate is EUR→CODE. With
// several days (the 90-day file) the latest day is returned.
func ParseECB(r io.Reader) (Day, error) {
	var doc ecbDoc
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return Day{}, fmt.Errorf("%w: ecb: %v", ErrParse, err)
	}
	var out Day
	for _, d := range doc.Cube.Days {
		date, err := time.Parse("2006-01-02", strings.TrimSpace(d.Time))
		if err != nil {
			continue
		}
		if !out.Date.IsZero() && !date.After(out.Date) {
			continue
		}
		day := Day{Source: SourceECB, Date: date}
		for _, c := range d.Rates {
			code := strings.ToUpper(strings.TrimSpace(c.Currency))
			v, ok := new(big.Rat).SetString(strings.TrimSpace(c.Rate))
			if !validCode(code) || code == "EUR" || !ok || v.Sign() <= 0 {
				continue
			}
			day.Rates = append(day.Rates, Rate{Base: "EUR", Quote: code, Rate: FormatRat(v)})
		}
		out = day
	}
	if out.Date.IsZero() || len(out.Rates) == 0 {
		return Day{}, fmt.Errorf("%w: ecb: no rates", ErrParse)
	}
	return out, nil
}

func validCode(c string) bool {
	if len(c) != 3 {
		return false
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// FormatRat renders a rate with Scale fraction digits (rounded half away
// from zero) and trims trailing zeros.
func FormatRat(r *big.Rat) string {
	s := r.FloatString(Scale)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	return s
}

// ParseRate validates a decimal rate string (> 0) and returns it normalized.
func ParseRate(raw string) (string, error) {
	v, ok := new(big.Rat).SetString(strings.TrimSpace(raw))
	if !ok || v.Sign() <= 0 {
		return "", fmt.Errorf("fxrates: rate must be a positive decimal")
	}
	s := FormatRat(v)
	if s == "0" {
		return "", fmt.Errorf("fxrates: rate is below the stored precision")
	}
	return s, nil
}
