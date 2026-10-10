package node

import (
	"testing"

	"github.com/InazumaV/V2bX/api/panel"
)

type delRecordingCore struct {
	fakeCore
	deleted []panel.UserInfo
}

func (d *delRecordingCore) DelUsers(users []panel.UserInfo, _ string, _ *panel.NodeInfo) error {
	d.deleted = append(d.deleted, users...)
	return nil
}

func TestUserDeltaRemoveByIDResolvesUUID(t *testing.T) {
	c := newTestController(t, "http://127.0.0.1:1", &fakeCore{})
	core := &delRecordingCore{}
	c.server = core
	c.userList = []panel.UserInfo{{Id: 1, Uuid: "u1"}, {Id: 2, Uuid: "u2"}}
	c.rebuildUIDToUUID()

	c.applyUserDelta("remove", []panel.UserInfo{{Id: 2}})

	if len(core.deleted) != 1 || core.deleted[0].Uuid != "u2" {
		t.Fatalf("core should delete u2 by resolved uuid, got %+v", core.deleted)
	}
	if len(c.userList) != 1 || c.userList[0].Id != 1 {
		t.Fatalf("user list not updated: %+v", c.userList)
	}

	c.applyUserDelta("add", []panel.UserInfo{{Id: 2, Uuid: "u2"}})
	if len(c.userList) != 2 {
		t.Fatalf("re-add after remove should succeed: %+v", c.userList)
	}

	core.deleted = nil
	c.applyUserDelta("remove", []panel.UserInfo{{Id: 99}})
	if len(core.deleted) != 0 {
		t.Fatalf("unknown user must not hit core: %+v", core.deleted)
	}
}
