package arin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestRPKIRRDPSnapshotSpool(t *testing.T) {
	for _, mode := range []string{"valid", "hash", "duplicate", "truncated", "trailing", "cancel", "read_error", "missing_directory"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			content := `<publish uri="rsync://repo.example/module/a.cer">YWJj</publish>`
			if mode == "duplicate" {
				content += content
			}
			body := rrdpSnapshotTestBody(content)
			if mode == "truncated" {
				body = body[:len(body)-1]
			}
			if mode == "trailing" {
				body = append(body, []byte("bad trailing data")...)
			}
			n := &rrdpNotification{Session: rrdpTestSession, Serial: "3", Snapshot: rrdpFileReference{Hash: sha256.Sum256(body)}}
			if mode == "hash" {
				n.Snapshot.Hash[0] ^= 1
			}
			if mode == "cancel" {
				cancel()
			}
			var input io.Reader = bytes.NewReader(body)
			if mode == "read_error" {
				input = io.MultiReader(input, errorRRDPReader{})
			}
			dir := directory
			if mode == "missing_directory" {
				dir += "/missing"
			}
			spool, err := spoolRRDPSnapshot(ctx, dir, input, n)
			if mode != "valid" {
				if err == nil || spool != nil {
					t.Fatal("invalid snapshot exposed a spool")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { spool.Close() })
				info, err := spool.file.Stat()
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("spool not private")
				}
				for i := 0; i < 2; i++ {
					data, err := spool.ReadObject("rsync://repo.example/module/a.cer")
					if err != nil || string(data) != "abc" {
						t.Fatalf("spooled data: %v", err)
					}
					data[0] = 'x'
				}
				if _, err := spool.ReadObject("absent"); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("missing object not distinguished")
				}
				if _, err := spool.file.WriteAt([]byte("x"), 0); err != nil {
					t.Fatal(err)
				}
				if _, err := spool.ReadObject("rsync://repo.example/module/a.cer"); err == nil {
					t.Fatal("corrupt object accepted")
				}
				if err := spool.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := spool.ReadObject("rsync://repo.example/module/a.cer"); err == nil {
					t.Fatal("closed spool readable")
				}
			}
			files, err := os.ReadDir(directory)
			if err != nil || len(files) != 0 {
				t.Fatal("snapshot staging file leaked")
			}
		})
	}
}

type errorRRDPReader struct{}

func (errorRRDPReader) Read([]byte) (int, error) { return 0, errors.New("injected read failure") }

func TestRPKIRRDPStreamLimits(t *testing.T) {
	body := rrdpSnapshotTestBody(`<publish uri="rsync://repo.example/module/a.cer">YWJj</publish>`)
	for _, tc := range []struct {
		name   string
		limits rrdpStreamLimits
		valid  bool
	}{
		{"exact", rrdpStreamLimits{int64(len(body)), 3, 1}, true},
		{"encoded", rrdpStreamLimits{int64(len(body)) - 1, 3, 1}, false},
		{"decoded", rrdpStreamLimits{int64(len(body)), 2, 1}, false},
		{"invalid", rrdpStreamLimits{int64(len(body)), 3, 0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := streamRRDPObjectFile(bytes.NewReader(body), rrdpTestSession, "3", sha256.Sum256(body), false, tc.limits, func(rrdpChange) error { return nil })
			if (err == nil) != tc.valid {
				t.Fatalf("err=%v", err)
			}
		})
	}
	// Oversized tokens must be stopped while encoding/xml reads them, not after
	// it allocates the complete comment or attribute.
	for _, prefix := range []string{"<!--", `<snapshot arbitrary="`} {
		raw := prefix + strings.Repeat("a", (8<<20)+8192)
		input := &countRRDPReader{input: strings.NewReader(raw)}
		err := streamRRDPObjectFile(input, rrdpTestSession, "3", sha256.Sum256([]byte(raw)), false, rrdpDiskLimits, func(rrdpChange) error { return nil })
		if err == nil {
			t.Fatal("oversized token accepted")
		}
		if input.count >= int64(len(raw)) {
			t.Fatal("oversized token fully buffered before rejection")
		}
	}
}

func TestRPKIRRDPSnapshotSpoolBeyondMapLimit(t *testing.T) {
	var content strings.Builder
	for i := 0; i < 10001; i++ {
		fmt.Fprintf(&content, `<publish uri="rsync://repo.example/module/%d.cer">YQ==</publish>`, i)
	}
	body := rrdpSnapshotTestBody(content.String())
	n := &rrdpNotification{Session: rrdpTestSession, Serial: "3", Snapshot: rrdpFileReference{Hash: sha256.Sum256(body)}}
	spool, err := spoolRRDPSnapshot(context.Background(), privateExchangeDir(t), bytes.NewReader(body), n)
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	if len(spool.entries) != 10001 || spool.size != 10001 {
		t.Fatal("large object index truncated")
	}
}

func TestRPKIRRDPSnapshotSpoolHTTPS(t *testing.T) {
	body := rrdpSnapshotTestBody(`<publish uri="rsync://repo.example/module/a.cer">YWJj</publish>`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("unexpected method or credentials")
		}
		switch r.URL.Path {
		case "/valid":
			w.(http.Flusher).Flush() // Unknown response length exercises streaming EOF.
			w.Write(body)
		case "/oversized":
			w.Header().Set("Content-Length", "2147483649")
		case "/truncated":
			w.Header().Set("Content-Length", fmt.Sprint(len(body)+1))
			w.Write(body)
		case "/downgrade":
			http.Redirect(w, r, "http://127.0.0.1:1/snapshot", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotModified)
		}
	}))
	defer server.Close()
	for _, mode := range []string{"valid", "oversized", "truncated", "downgrade", "304", "untrusted"} {
		t.Run(mode, func(t *testing.T) {
			directory := privateExchangeDir(t)
			client := rrdpHTTPClient{Transport: server.Client().Transport}
			if mode == "untrusted" {
				client.Transport = nil
			}
			n := &rrdpNotification{Session: rrdpTestSession, Serial: "3", Snapshot: rrdpFileReference{URI: server.URL + "/" + mode, Hash: sha256.Sum256(body)}}
			spool, err := client.FetchSnapshotSpool(context.Background(), directory, n)
			if mode == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if err := spool.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || spool != nil {
				t.Fatal("invalid response exposed repository")
			}
			files, err := os.ReadDir(directory)
			if err != nil || len(files) != 0 {
				t.Fatal("failed HTTP request leaked staging file")
			}
		})
	}
}

// Cross both former repository limits without constructing the full XML or
// decoded repository in memory. Native snapshots are larger still.
func TestRPKIRRDPSnapshotSpoolBeyondByteLimits(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(make([]byte, 1<<20))
	source := func() io.Reader {
		readers := []io.Reader{strings.NewReader(fmt.Sprintf(`<snapshot xmlns="%s" version="1" session_id="%s" serial="3">`, rrdpNamespace, rrdpTestSession))}
		for i := 0; i < 97; i++ {
			readers = append(readers, strings.NewReader(fmt.Sprintf(`<publish uri="rsync://repo.example/module/%d.cer">`, i)), strings.NewReader(encoded), strings.NewReader(`</publish>`))
		}
		readers = append(readers, strings.NewReader(`</snapshot>`))
		return io.MultiReader(readers...)
	}
	hash := sha256.New()
	size, err := io.Copy(hash, source())
	if err != nil || size <= 128<<20 {
		t.Fatal("fixture does not exceed former download limit")
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	n := &rrdpNotification{Session: rrdpTestSession, Serial: "3", Snapshot: rrdpFileReference{Hash: digest}}
	spool, err := spoolRRDPSnapshot(context.Background(), privateExchangeDir(t), source(), n)
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	if spool.size != 97<<20 || len(spool.entries) != 97 {
		t.Fatal("large snapshot truncated")
	}
	data, err := spool.ReadObject("rsync://repo.example/module/96.cer")
	if err != nil || len(data) != 1<<20 {
		t.Fatal("last streamed object unavailable")
	}
}

type countRRDPReader struct {
	input io.Reader
	count int64
}

func (r *countRRDPReader) Read(p []byte) (int, error) {
	n, err := r.input.Read(p)
	r.count += int64(n)
	return n, err
}
