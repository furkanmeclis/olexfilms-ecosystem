package whatsapp

import (
	"bytes"
	"context"
	"errors"
	"path"
	"strings"
)

// MaxMediaBytes is the largest media file stored or sent (WhatsApp's own
// document limit is higher; 16 MB matches its image/audio/video limit).
const MaxMediaBytes = 16 << 20

// Media errors.
var (
	ErrMediaTooLarge   = errors.New("whatsapp: media larger than 16 MB")
	ErrMediaType       = errors.New("whatsapp: unsupported media type")
	ErrMediaMismatch   = errors.New("whatsapp: media content does not match its file type")
	ErrMediaNoDownload = errors.New("whatsapp: media has no download reference")
)

// Sniffed media types. Only these are stored or sent.
const (
	MimeJPEG = "image/jpeg"
	MimePNG  = "image/png"
	MimeWebP = "image/webp"
	MimePDF  = "application/pdf"
	MimeOGG  = "audio/ogg"
)

// SniffMedia returns the media type from the leading bytes (magic
// numbers), or "" when the content is none of the supported types. The
// declared mime type and the file name are never trusted.
func SniffMedia(data []byte) string {
	switch {
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return MimeJPEG
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return MimePNG
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return MimeWebP
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return MimePDF
	case bytes.HasPrefix(data, []byte("OggS")):
		return MimeOGG
	}
	return ""
}

// extMime maps file extensions to the type their content must have.
var extMime = map[string]string{
	".jpg": MimeJPEG, ".jpeg": MimeJPEG, ".png": MimePNG, ".webp": MimeWebP,
	".pdf": MimePDF, ".ogg": MimeOGG, ".oga": MimeOGG, ".opus": MimeOGG,
}

// ValidateMedia checks size and content of a media file: it must be at most
// MaxMediaBytes, sniff as a supported type, and match the type its file
// name extension (when it has a known one) promises. So a "report.pdf"
// that is not a PDF is rejected even if it is a valid image.
func ValidateMedia(data []byte, fileName string) (string, error) {
	if len(data) > MaxMediaBytes {
		return "", ErrMediaTooLarge
	}
	mime := SniffMedia(data)
	if want, ok := extMime[strings.ToLower(path.Ext(strings.TrimSpace(fileName)))]; ok && want != mime {
		return "", ErrMediaMismatch
	}
	if mime == "" {
		return "", ErrMediaType
	}
	return mime, nil
}

// IsImageMime reports whether mime is a sniffed image type (sent as image).
func IsImageMime(mime string) bool {
	return mime == MimeJPEG || mime == MimePNG || mime == MimeWebP
}

// MediaDownloader fetches the bytes of inbound media (wuzapi decrypts it
// from the WhatsApp CDN). It returns ErrMediaTooLarge without reading the
// whole file when the media is larger than max.
type MediaDownloader interface {
	DownloadMedia(ctx context.Context, m InboundMedia, max int64) ([]byte, error)
}
