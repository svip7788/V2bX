package xray

import (
	"github.com/InazumaV/V2bX/api/panel"
	"github.com/InazumaV/V2bX/common/format"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/anytls"
)

func buildAnyTLSUsers(tag string, userInfo []panel.UserInfo) (users []*protocol.User) {
	users = make([]*protocol.User, len(userInfo))
	for i := range userInfo {
		users[i] = &protocol.User{
			Level:   0,
			Email:   format.UserTag(tag, userInfo[i].Uuid),
			Account: serial.ToTypedMessage(&anytls.Account{Password: userInfo[i].Uuid}),
		}
	}
	return users
}
