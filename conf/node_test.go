package conf

import (
	"encoding/json"
	"testing"
)

func TestNodeConfigUnmarshalSupportsCommaSeparatedNodeIDs(t *testing.T) {
	var node NodeConfig
	err := json.Unmarshal([]byte(`{
		"ApiHost": "https://example.com",
		"ApiKey": "test",
		"NodeID": "1167, 1168,1169",
		"NodeType": "vless",
		"Core": "xray"
	}`), &node)
	if err != nil {
		t.Fatalf("unmarshal node config failed: %v", err)
	}

	got := node.ApiConfig.GetNodeIDs()
	want := []int{1167, 1168, 1169}
	if len(got) != len(want) {
		t.Fatalf("expected %d node ids, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected node id %d at index %d, got %d", want[i], i, got[i])
		}
	}
	if node.ApiConfig.NodeID != 1167 {
		t.Fatalf("expected primary node id 1167, got %d", node.ApiConfig.NodeID)
	}
}

func TestNodeConfigExpandedNodes(t *testing.T) {
	node := NodeConfig{
		ApiConfig: ApiConfig{
			APIHost:  "https://example.com",
			Key:      "test",
			NodeType: "vless",
		},
	}
	node.ApiConfig.SetNodeIDs([]int{1167, 1168, 1169})

	expanded, err := node.ExpandedNodes()
	if err != nil {
		t.Fatalf("expand node config failed: %v", err)
	}
	if len(expanded) != 3 {
		t.Fatalf("expected 3 expanded nodes, got %d", len(expanded))
	}
	for i, nodeID := range []int{1167, 1168, 1169} {
		if expanded[i].ApiConfig.NodeID != nodeID {
			t.Fatalf("expected expanded node id %d at index %d, got %d", nodeID, i, expanded[i].ApiConfig.NodeID)
		}
		got := expanded[i].ApiConfig.GetNodeIDs()
		if len(got) != 1 || got[0] != nodeID {
			t.Fatalf("expected expanded node ids [%d], got %v", nodeID, got)
		}
	}
}
