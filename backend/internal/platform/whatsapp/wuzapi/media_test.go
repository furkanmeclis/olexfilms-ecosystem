package wuzapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/furkanmeclis/olexfilms-ecosystem/backend/internal/platform/whatsapp"
)

const sampleImage = `{"type":"Message","token":"user-token","event":{"Info":{"ID":"3EB0IMG","Chat":"905551234567@s.whatsapp.net","Sender":"905551234567@s.whatsapp.net","Timestamp":"2026-10-01T12:00:00+03:00","Type":"media"},"Message":{"imageMessage":{"caption":"cam","mimetype":"image/jpeg","URL":"https://mmg.whatsapp.net/x","directPath":"/v/t62/x","mediaKey":"a2V5","fileEncSHA256":"ZW5j","fileSHA256":"c2hh","fileLength":2048}}}}`

// The webhook keeps the download reference and size of inbound media.
func TestParseWebhookMediaReference(t *testing.T) {
	c := New(Config{HMACKey: testKey})
	c.SetUserToken("user-token")
	evs, err := c.ParseWebhook(signed(sampleImage), []byte(sampleImage))
	if err != nil || len(evs) != 1 || evs[0].Media == nil {
		t.Fatalf("parse: %+v %v", evs, err)
	}
	m := evs[0].Media
	if m.Type != "image" || m.Size != 2048 || m.URL != "" {
		t.Fatalf("media = %+v", m)
	}
	var ref mediaRef
	if err := json.Unmarshal(m.Download, &ref); err != nil || ref.DirectPath != "/v/t62/x" || ref.MediaKey != "a2V5" || ref.FileLength != 2048 {
		t.Fatalf("download ref = %+v %v", ref, err)
	}
}

func TestDownloadMedia(t *testing.T) {
	jpeg := []byte("\xFF\xD8\xFF\xE0 image bytes")
	var gotPath string
	var gotRef mediaRef
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("token") != "user-token" {
			w.WriteHeader(401)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotRef)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "success": true, "data": map[string]any{
			"Data": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg), "Mimetype": "image/jpeg",
		}})
	}))
	defer srv.Close()
	c := New(Config{BaseURL: srv.URL})
	c.SetUserToken("user-token")
	ref, _ := json.Marshal(mediaRef{DirectPath: "/v/x", MediaKey: "a2V5", FileLength: int64(len(jpeg))})
	data, err := c.DownloadMedia(context.Background(), whatsapp.InboundMedia{Type: "image", Download: ref}, whatsapp.MaxMediaBytes)
	if err != nil || !bytes.Equal(data, jpeg) {
		t.Fatalf("download: %q %v", data, err)
	}
	if gotPath != "/chat/downloadimage" || gotRef.DirectPath != "/v/x" || gotRef.MediaKey != "a2V5" {
		t.Fatalf("request: %s %+v", gotPath, gotRef)
	}

	// Declared larger than the limit: no request.
	gotPath = ""
	big, _ := json.Marshal(mediaRef{DirectPath: "/v/x", FileLength: 20 << 20})
	if _, err := c.DownloadMedia(context.Background(), whatsapp.InboundMedia{Type: "document", Download: big}, whatsapp.MaxMediaBytes); !errors.Is(err, whatsapp.ErrMediaTooLarge) || gotPath != "" {
		t.Fatalf("too large: %v path=%q", err, gotPath)
	}
	// Downloaded content over the limit.
	if _, err := c.DownloadMedia(context.Background(), whatsapp.InboundMedia{Type: "image", Download: ref}, 4); !errors.Is(err, whatsapp.ErrMediaTooLarge) {
		t.Fatalf("over max: %v", err)
	}
	// No reference at all.
	if _, err := c.DownloadMedia(context.Background(), whatsapp.InboundMedia{Type: "image"}, 100); !errors.Is(err, whatsapp.ErrMediaNoDownload) {
		t.Fatalf("no reference: %v", err)
	}
}
