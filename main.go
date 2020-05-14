package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
)

type Client struct {
	RoomId   string          `json:"roomId,omitempty"`
	User     *User           `json:"user,omitempty"`
	DrawConn *websocket.Conn `json:"DrawConn,omitempty"`
	RoomConn *websocket.Conn `json:"RoomConn,omitempty"`
}

type Room struct {
	Id         string             `json:"id,omitempty"`
	Name       string             `json:"name,omitempty"`
	ClientsMap map[string]*Client `json:"usersMap,omitempty"`
}

type User struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

var clients = make(map[*websocket.Conn]*Client)

func main() {

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/ws/draw/", drawWsHandler)
	http.HandleFunc("/ws/room/", roomWsHandler)
	http.HandleFunc("/room/", roomHandler)          // create, list room .etc
	http.HandleFunc("/newDrawWs", newDrawWsHandler) // test new ws dynamically
	http.HandleFunc("/test/", testHandler)          // test print

	log.Println("server start at :8899")
	port := "8899"
	if v := os.Getenv("PORT"); len(v) > 0 {
		port = v
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))

}

func drawWsHandler(w http.ResponseWriter, r *http.Request) {

	upgrader := &websocket.Upgrader{

		CheckOrigin: func(r *http.Request) bool { return true },
	}
	roomId := strings.Split(r.URL.Path, "/ws/draw/")[1]
	currentRoom, roomErr := roomsMap[roomId]
	if roomErr == false {
		return
	}

	userId := r.URL.Query().Get("userId")
	userName := r.URL.Query().Get("userName")
	if userId == "" || userName == "" {
		return
	}

	currentClientsMap := currentRoom.ClientsMap
	_, userErr := currentClientsMap[userId]
	if userErr == false {
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	log.Println("connect !!")

	roomsMap[roomId].ClientsMap[userId].DrawConn = conn
	clients[conn] = new(Client)
	clients[conn].RoomId = roomId
	clients[conn].User = &User{userId, userName}
	clients[conn].DrawConn = conn

	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer func() {
		log.Println("disconnect !!")
		delete(clients, conn)
		roomsMap[roomId].ClientsMap[userId].DrawConn = nil
		if len(roomsMap[roomId].ClientsMap) == 0 {
			delete(roomsMap, roomId)
		}
		conn.Close()
	}()

	for {
		mtype, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("receive: %s\n", msg)

		currentRoom, exist := roomsMap[roomId]
		if exist == false {
			return
		}
		clientsMap := currentRoom.ClientsMap // get the users in this room
		for _, client := range clientsMap {
			if client.User.Id != clients[conn].User.Id {
				respMsg := clients[conn].User.Name + " say::" + string(msg)
				if client.DrawConn == nil {
					break
				}
				err = client.DrawConn.WriteMessage(mtype, []byte(respMsg))
				if err != nil {
					log.Println("write:", err)
					break
				}
			}

		}

		if err != nil {
			log.Println("write:", err)
			break
		}
	}

}

func roomWsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader := &websocket.Upgrader{

		CheckOrigin: func(r *http.Request) bool { return true },
	}
	roomId := strings.Split(r.URL.Path, "/ws/room/")[1]
	currentRoom, roomErr := roomsMap[roomId]
	if roomErr == false {
		return
	}

	userId := r.URL.Query().Get("userId")
	userName := r.URL.Query().Get("userName")
	if userId == "" || userName == "" {
		return
	}

	currentClientsMap := currentRoom.ClientsMap
	_, userErr := currentClientsMap[userId]
	if userErr == false {
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	log.Println("connect !!")

	roomsMap[roomId].ClientsMap[userId].RoomConn = conn
	clients[conn] = new(Client)
	clients[conn].RoomId = roomId
	clients[conn].User = &User{userId, userName}
	clients[conn].RoomConn = conn

	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer func() {
		log.Println("disconnect !!")
		delete(clients, conn)
		roomsMap[roomId].ClientsMap[userId].RoomConn = nil
		if len(roomsMap[roomId].ClientsMap) == 0 {
			delete(roomsMap, roomId)
		}
		conn.Close()
	}()

	for {
		mtype, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("receive: %s\n", msg)

		currentRoom, exist := roomsMap[roomId]
		if exist == false {
			return
		}
		clientsMap := currentRoom.ClientsMap // get the users in this room
		for _, client := range clientsMap {
			if client.User.Id != clients[conn].User.Id {
				respMsg := clients[conn].User.Name + " say::" + string(msg)
				if client.RoomConn == nil {
					break
				}
				err = client.RoomConn.WriteMessage(mtype, []byte(respMsg))
				if err != nil {
					log.Println("write:", err)
					break
				}
			}

		}

		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func newDrawWsHandler(w http.ResponseWriter, r *http.Request) {
	roomId := generateRoomId()
	println(roomId)
	roomsMap[roomId] = &Room{roomId, "test", make(map[string]*Client)}
	fmt.Fprint(w, roomId)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		errorHandler(w, r, http.StatusNotFound)
		return
	}
	http.HandleFunc("/ws/draw/", drawWsHandler)
	http.HandleFunc("/ws/room/", roomWsHandler)
	http.HandleFunc("/room/", roomHandler) // create, list room .etc
	fmt.Fprint(w, "/ws/draw/<roomId>?userId=<userId> for ")
	fmt.Fprint(w, "/ws/room/<roomId>?userId=<userId> for ")
	fmt.Fprint(w, "/room/list for list rooms")
	fmt.Fprint(w, "/room/create for create a room")
	fmt.Fprint(w, "/room/join?userId=<userId>&roomId<roomId> for user to join a room")
	fmt.Fprint(w, "/room/quit?userId=<userId>&roomId<roomId> for user to quit a room")
}

func errorHandler(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)
	if status == http.StatusNotFound {
		fmt.Fprint(w, "custom 404")
	}
}

func roomHandler(w http.ResponseWriter, r *http.Request) {

	if r.URL.Path == "/room/list" {
		roomListHandler(w, r)
		return
	} else if r.URL.Path == "/room/create" {
		roomCreateHandler(w, r)
		return
	} else if r.URL.Path == "/room/join" {
		roomJoinHandler(w, r)
		return
	} else if r.URL.Path == "/room/quit" {
		roomQuitHandler(w, r)
	}
}

func roomListHandler(w http.ResponseWriter, r *http.Request) {

	rooms := make([]Room, 0, len(roomsMap))
	for _, room := range roomsMap {
		rooms = append(rooms, *room)
	}
	jsonString, err := json.Marshal(rooms)
	if err != nil {
		println(err)
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonString))
}

var roomsMap = make(map[string]*Room)

func roomCreateHandler(w http.ResponseWriter, r *http.Request) {

	var roomTmp Room
	err := json.NewDecoder(r.Body).Decode(&roomTmp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	roomName := roomTmp.Name
	roomId := generateRoomId()
	room := &Room{roomId, roomName, make(map[string]*Client)}
	roomsMap[roomId] = room
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "/room/create")
	fmt.Fprintln(w, "create room success!!")
}

func roomJoinHandler(w http.ResponseWriter, r *http.Request) {

	userId := r.URL.Query().Get("userId")
	userName := r.URL.Query().Get("userName")
	roomId := r.URL.Query().Get("roomId")
	user := &User{userId, userName}

	result := joinRoomById(user, roomId)
	if result {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "/room/join")
		fmt.Fprintln(w, "join room success!!")
	} else {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "/room/join")
		fmt.Fprintln(w, "join room fail!!")
	}

}
func roomQuitHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.URL.Query().Get("userId")
	roomId := r.URL.Query().Get("roomId")
	quitRoomById(userId, roomId)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "/room/quit")
}

func joinRoomById(user *User, roomId string) bool {
	room, roomErr := roomsMap[roomId]
	if roomErr == false {
		return false
	}
	_, userErr := room.ClientsMap[user.Id]
	if userErr == true {
		return false
	}
	room.ClientsMap[user.Id] = &Client{roomId, user, nil, nil}
	return true
}

func quitRoomById(userID string, roomId string) {
	delete(roomsMap[roomId].ClientsMap, userID)
}

func generateUserId() string {
	return generateUuId()
}

func generateRoomId() string {
	return generateUuId()
}

func generateUuId() string {
	uuid := uuid.NewV4()
	return uuid.String()
}

func testHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/test/1" {
		w.WriteHeader(http.StatusOK)

		jsonString, _ := json.Marshal(roomsMap)
		fmt.Fprintln(w, string(jsonString))
	} else {
		w.WriteHeader(http.StatusOK)

		fmt.Fprintln(w, "[")
		index := 0
		for _, value := range clients {
			if index > 0 {
				fmt.Fprintln(w, ",")
			}
			jsonString, _ := json.Marshal(value)
			fmt.Fprintln(w, string(jsonString))
			index++
		}
		fmt.Fprintln(w, "]")
	}

}
