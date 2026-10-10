package userconn

import (
	"net"
	"sync"
)

// CloseUsers 关闭 conns（net.Conn -> 用户名）中属于 names 的连接，只遍历一次，
// 批量删除时为 O(连接数)，而不是 O(用户数 × 连接数)。
func CloseUsers(conns *sync.Map, names map[string]struct{}) {
	if len(names) == 0 {
		return
	}
	conns.Range(func(key, value any) bool {
		name, _ := value.(string)
		if _, ok := names[name]; ok {
			if conn, ok := key.(net.Conn); ok {
				_ = conn.Close()
			}
			conns.Delete(key)
		}
		return true
	})
}
