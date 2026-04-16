package panel

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/InazumaV/V2bX/conf"
)

func TestGetUserListForcesPeriodicFullRefresh(t *testing.T) {
	var current time.Time
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		switch requests {
		case 1:
			if got := r.Header.Get("If-None-Match"); got != "" {
				t.Fatalf("first request should not send If-None-Match, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("ETag", "etag-1")
			_, _ = w.Write([]byte(`{"users":[{"id":1,"uuid":"u1"}]}`))
		case 2:
			if got := r.Header.Get("If-None-Match"); got != "etag-1" {
				t.Fatalf("second request should reuse etag, got %q", got)
			}
			w.WriteHeader(http.StatusNotModified)
		case 3:
			if got := r.Header.Get("If-None-Match"); got != "" {
				t.Fatalf("third request should bypass etag after refresh window, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("ETag", "etag-2")
			_, _ = w.Write([]byte(`{"users":[{"id":2,"uuid":"u2"}]}`))
		default:
			t.Fatalf("unexpected request count: %d", requests)
		}
	}))
	defer server.Close()

	client, err := New(&conf.ApiConfig{
		APIHost:  server.URL,
		NodeID:   1,
		Key:      "test",
		NodeType: "vmess",
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	client.nowFunc = func() time.Time { return current }
	client.userListForceRefreshAfter = time.Minute

	current = time.Unix(0, 0)
	users, err := client.GetUserList()
	if err != nil {
		t.Fatalf("first get user list: %v", err)
	}
	if len(users) != 1 || users[0].Uuid != "u1" {
		t.Fatalf("unexpected first users: %#v", users)
	}

	current = current.Add(30 * time.Second)
	users, err = client.GetUserList()
	if err != nil {
		t.Fatalf("second get user list: %v", err)
	}
	if users != nil {
		t.Fatalf("second get user list should be not modified, got %#v", users)
	}

	current = current.Add(2 * time.Minute)
	users, err = client.GetUserList()
	if err != nil {
		t.Fatalf("third get user list: %v", err)
	}
	if len(users) != 1 || users[0].Uuid != "u2" {
		t.Fatalf("unexpected third users: %#v", users)
	}
}
