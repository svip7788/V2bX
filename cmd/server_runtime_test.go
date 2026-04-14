package cmd

import (
	"testing"

	"github.com/InazumaV/V2bX/conf"
)

func TestCollectNodeGroupsByObject(t *testing.T) {
	first := conf.NodeConfig{
		ApiConfig: conf.ApiConfig{
			APIHost:  "https://panel-a.example.com",
			Key:      "token-a",
			NodeType: "vless",
		},
	}
	first.ApiConfig.SetNodeIDs([]int{1167, 1168, 1169})

	second := conf.NodeConfig{
		ApiConfig: conf.ApiConfig{
			APIHost:  "https://panel-b.example.com",
			Key:      "token-b",
			NodeType: "vmess",
		},
	}
	second.ApiConfig.SetNodeIDs([]int{2201})

	groups, err := collectNodeGroups([]conf.NodeConfig{first, second})
	if err != nil {
		t.Fatalf("collect node groups failed: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].ID != 1 || len(groups[0].Nodes) != 3 {
		t.Fatalf("expected first group id=1 with 3 nodes, got id=%d len=%d", groups[0].ID, len(groups[0].Nodes))
	}
	if groups[1].ID != 2 || len(groups[1].Nodes) != 1 {
		t.Fatalf("expected second group id=2 with 1 node, got id=%d len=%d", groups[1].ID, len(groups[1].Nodes))
	}
	if got := formatNodeIDs(groups[0].Nodes); got != "1167,1168,1169" {
		t.Fatalf("unexpected first group node ids: %s", got)
	}
	if got := formatNodeIDs(groups[1].Nodes); got != "2201" {
		t.Fatalf("unexpected second group node ids: %s", got)
	}
}
