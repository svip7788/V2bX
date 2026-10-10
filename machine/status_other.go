//go:build !linux

package machine

import "github.com/InazumaV/V2bX/api/panel"

type statusCollector struct{}

func newStatusCollector() *statusCollector { return &statusCollector{} }

func (c *statusCollector) collect() *panel.MachineStatus { return &panel.MachineStatus{} }
