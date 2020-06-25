package bean

import "github.com/gorilla/websocket"

type User struct {
	RoomId   string          `json:"roomId,omitempty"`
	UserId   string          `json:"userId,omitempty"`
	UserName string          `json:"userName,omitempty"`
	DrawConn *websocket.Conn `json:"DrawConn,omitempty"`
	RoomConn *websocket.Conn `json:"RoomConn,omitempty"`
	Ready    *bool           `json:"ready,omitempty"`
	Role     string          `json:"role,omitempty"`
}
