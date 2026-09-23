package arin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRPKIRRDPRefresh(t *testing.T) {
	for _, mode := range []string{"initial", "deltas", "gap", "delta_failure", "all_failure", "new_session", "unchanged", "same_serial", "rollback", "conflicting_serial", "poll_limit"} {
		t.Run(mode, func(t *testing.T) {
			session := rrdpTestSession
			if mode == "new_session" {
				session = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
			}
			snapshot := []byte(strings.Replace(string(rrdpSnapshotTestBody(`<publish uri="`+rrdpTestObjectURI+`">bmV3</publish>`)), rrdpTestSession, session, 1))
			delta2 := rrdpDeltaTestBody("2", rrdpDeltaPublish(rrdpTestObjectURI, []byte("old"), []byte("middle")))
			delta3 := rrdpDeltaTestBody("3", rrdpDeltaPublish(rrdpTestObjectURI, []byte("middle"), []byte("new")))
			var mu sync.Mutex
			counts := map[string]int{}
			var server *httptest.Server
			var oldRef rrdpFileReference
			server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				counts[r.URL.Path]++
				mu.Unlock()
				switch r.URL.Path {
				case "/notification.xml":
					if mode == "unchanged" {
						if r.Header.Get("If-Modified-Since") == "" {
							t.Error("missing conditional header")
						}
						w.WriteHeader(304)
						return
					}
					w.Header().Set("Last-Modified", cmsTrustNow().Format(http.TimeFormat))
					serial := "3"
					ref := rrdpFileReference{URI: server.URL + "/snapshot.xml", Hash: sha256.Sum256(snapshot)}
					if mode == "same_serial" {
						serial = "1"
						ref = oldRef
					}
					if mode == "conflicting_serial" {
						serial = "1"
					}
					if mode == "rollback" {
						serial = "1"
					}
					children := fmt.Sprintf(`<snapshot uri="%s" hash="%x"/>`, ref.URI, ref.Hash)
					if serial == "3" {
						if mode != "gap" {
							children += fmt.Sprintf(`<delta serial="2" uri="%s/delta2.xml" hash="%x"/>`, server.URL, sha256.Sum256(delta2))
						}
						children += fmt.Sprintf(`<delta serial="3" uri="%s/delta3.xml" hash="%x"/>`, server.URL, sha256.Sum256(delta3))
					}
					body := strings.Replace(string(rrdpNotificationTestBody(serial, children)), rrdpTestSession, session, 1)
					_, _ = w.Write([]byte(body))
				case "/delta2.xml":
					_, _ = w.Write(delta2)
				case "/delta3.xml":
					if mode == "delta_failure" || mode == "all_failure" {
						w.WriteHeader(404)
					} else {
						_, _ = w.Write(delta3)
					}
				case "/snapshot.xml":
					if mode == "all_failure" {
						w.WriteHeader(404)
					} else {
						_, _ = w.Write(snapshot)
					}
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			uri := server.URL + "/notification.xml"
			oldRef = rrdpFileReference{URI: server.URL + "/old-snapshot.xml", Hash: sha256.Sum256([]byte("old snapshot"))}
			base := &rrdpCache{Repository: rrdpRepository{NotificationURI: uri, Session: rrdpTestSession, Serial: "1", Objects: map[string][]byte{rrdpTestObjectURI: []byte("old")}}, Snapshot: oldRef, LastModified: cmsTrustNow().Add(-time.Hour), LastAttempt: cmsTrustNow().Add(-time.Minute)}
			if mode == "initial" {
				base = nil
			}
			if mode == "rollback" {
				base.Repository.Serial = "2"
			}
			if mode == "poll_limit" {
				base.LastAttempt = cmsTrustNow().Add(-59 * time.Second)
			}
			client := rrdpHTTPClient{Transport: server.Client().Transport}
			got, err := client.Refresh(context.Background(), uri, base, cmsTrustNow())
			failed := mode == "all_failure" || mode == "rollback" || mode == "conflicting_serial" || mode == "poll_limit"
			if (err != nil) != failed {
				t.Fatalf("expected failure=%v, err=%v", failed, err)
			}
			if base != nil && string(base.Repository.Objects[rrdpTestObjectURI]) != "old" {
				t.Fatal("previous cache mutated")
			}
			keep := failed || mode == "unchanged" || mode == "same_serial"
			expected := "new"
			if keep {
				expected = "old"
			}
			if string(got.Repository.Objects[rrdpTestObjectURI]) != expected {
				t.Fatal("wrong retained/refreshed object")
			}
			if mode != "poll_limit" && !got.LastAttempt.Equal(cmsTrustNow()) {
				t.Fatal("attempt timestamp lost")
			}
			mu.Lock()
			defer mu.Unlock()
			if mode == "poll_limit" {
				if len(counts) != 0 {
					t.Fatal("polled too soon")
				}
				return
			}
			if counts["/notification.xml"] != 1 {
				t.Fatal("notification retrieved more than once")
			}
			switch mode {
			case "deltas":
				if counts["/delta2.xml"] != 1 || counts["/delta3.xml"] != 1 || counts["/snapshot.xml"] != 0 {
					t.Fatal("complete delta chain not preferred")
				}
			case "initial", "gap", "new_session":
				if counts["/snapshot.xml"] != 1 || counts["/delta2.xml"] != 0 || counts["/delta3.xml"] != 0 {
					t.Fatal("snapshot selection failed")
				}
			case "delta_failure", "all_failure":
				if counts["/delta2.xml"] != 1 || counts["/delta3.xml"] != 1 || counts["/snapshot.xml"] != 1 {
					t.Fatal("delta failure did not fall back")
				}
			case "unchanged", "same_serial", "rollback", "conflicting_serial":
				if len(counts) != 1 {
					t.Fatal("unnecessary object download")
				}
			}
			if base != nil {
				got.Repository.Objects[rrdpTestObjectURI][0] = 'X'
				if string(base.Repository.Objects[rrdpTestObjectURI]) != "old" {
					t.Fatal("returned objects alias previous cache")
				}
			}
		})
	}
}
