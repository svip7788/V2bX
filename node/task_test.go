package node

import "testing"

func TestPeriodicNodeInfoMonitorSkipsWhenWSConnected(t *testing.T) {
	controller := &Controller{
		wsConnected: func() bool {
			return true
		},
	}

	if err := controller.periodicNodeInfoMonitor(); err != nil {
		t.Fatalf("periodicNodeInfoMonitor returned error: %v", err)
	}
}
