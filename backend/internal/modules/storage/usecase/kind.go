package usecase

import (
	"path"
	"strings"
)

func classifyFileKind(name, mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	switch {
	case strings.HasPrefix(mime, "image/"), isExt(ext, "png", "jpg", "jpeg", "gif", "webp", "svg", "bmp", "ico"):
		return "image"
	case strings.HasPrefix(mime, "video/"), isExt(ext, "mp4", "webm", "mov", "mkv", "avi"):
		return "video"
	case strings.HasPrefix(mime, "audio/"), isExt(ext, "mp3", "wav", "ogg", "flac", "m4a"):
		return "audio"
	case mime == "application/pdf", ext == "pdf":
		return "pdf"
	case isExt(ext, "zip", "rar", "7z", "tar", "gz", "tgz", "bz2"):
		return "archive"
	case isExt(ext, "xls", "xlsx", "csv", "ods"):
		return "spreadsheet"
	case isExt(ext, "doc", "docx", "odt", "rtf", "pages"):
		return "document"
	case isExt(ext, "go", "ts", "tsx", "js", "jsx", "json", "yml", "yaml", "toml", "md", "html", "css", "py", "rs", "java", "kt", "sql", "sh"):
		return "code"
	case strings.HasPrefix(mime, "text/"):
		return "code"
	default:
		return "unknown"
	}
}

func isExt(ext string, allowed ...string) bool {
	for _, a := range allowed {
		if ext == a {
			return true
		}
	}
	return false
}

func mimeFromName(name, fallback string) string {
	if fallback != "" && fallback != "application/octet-stream" {
		return fallback
	}
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".json":
		return "application/json"
	case ".txt", ".md":
		return "text/plain"
	case ".html":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "text/javascript"
	case ".zip":
		return "application/zip"
	default:
		if fallback != "" {
			return fallback
		}
		return "application/octet-stream"
	}
}
