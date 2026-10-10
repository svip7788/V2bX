package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func TestMachineNodeClientUsesV2Paths(t *testing.T) {
	var gotPath string
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	api := &conf.ApiConfig{APIHost: srv.URL, Key: "tk", NodeType: "vless", Timeout: 5}
	api.SetNodeIDs([]int{9})
	c, err := NewMachineNode(api, 4)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.GetUserList(t.Context())
	if gotPath != "/api/v2/server/user" {
		t.Fatalf("path = %s", gotPath)
	}
	if gotQuery["machine_id"][0] != "4" || gotQuery["node_id"][0] != "9" || gotQuery["token"][0] != "tk" {
		t.Fatalf("query = %v", gotQuery)
	}
	if _, ok := gotQuery["node_type"]; ok {
		t.Fatalf("node_type should be removed: %v", gotQuery)
	}
}

func TestMachineClient(t *testing.T) {
	var statusBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("machine_id") != "2" || r.URL.Query().Get("token") != "tk" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch r.URL.Path {
		case "/api/v2/server/machine/nodes":
			_, _ = io.WriteString(w, `{"nodes":[{"id":1,"type":"hysteria","node_type":"hysteria2","name":"a"}],"base_config":{"push_interval":60,"pull_interval":30}}`)
		case "/api/v2/server/machine/status":
			_ = json.NewDecoder(r.Body).Decode(&statusBody)
			_, _ = io.WriteString(w, `{"data":true}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	m, err := NewMachineClient(&conf.MachineConfig{APIHost: srv.URL, MachineID: 2, Token: "tk"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := m.FetchNodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 || resp.Nodes[0].NodeType != "hysteria2" || resp.BaseConfig.PullInterval != 30 {
		t.Fatalf("nodes = %+v", resp)
	}
	s := &MachineStatus{CPU: 12.5, Mem: Usage{Total: 100, Used: 40}}
	s.V2bX.Status = "active"
	if err := m.ReportStatus(s); err != nil {
		t.Fatal(err)
	}
	if statusBody["cpu"].(float64) != 12.5 || statusBody["mem"].(map[string]any)["total"].(float64) != 100 {
		t.Fatalf("status body = %v", statusBody)
	}
	if _, ok := statusBody["net"]; ok {
		t.Fatalf("net should be omitted when nil: %v", statusBody)
	}

	bad, _ := NewMachineClient(&conf.MachineConfig{APIHost: srv.URL, MachineID: 2, Token: "wrong"})
	if _, err := bad.FetchNodes(); err == nil {
		t.Fatal("expected error for bad token")
	}
}
