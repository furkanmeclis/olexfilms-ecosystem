package sysconfig

import "context"

// Scan groups the scan.* keys read by the universal scan resolver
// (TEC-203).
type Scan struct {
	SKUEnabled              bool
	ShortCodeEnabled        bool
	ShortCodePrefix         string
	BareLocationCodeEnabled bool
}

// Scan returns the scanner settings (defaults on a read error).
func (s *Service) Scan(ctx context.Context) Scan {
	return Scan{
		SKUEnabled:              s.Bool(ctx, KeyScanSKUEnabled),
		ShortCodeEnabled:        s.Bool(ctx, KeyScanShortCodeEnabled),
		ShortCodePrefix:         s.String(ctx, KeyScanShortCodePrefix),
		BareLocationCodeEnabled: s.Bool(ctx, KeyScanBareLocationCodeEnabled),
	}
}
