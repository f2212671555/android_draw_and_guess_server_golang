package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"

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
	Id               string             `json:"roomId,omitempty"`
	Name             string             `json:"roomName,omitempty"`
	ClientsMap       map[string]*Client `json:"usersMap,omitempty"`
	RoomJoinLock     bool               `json:"-"`
	CurrentDrawOrder int                `json:"-"`
}

type User struct {
	Id        string `json:"userId,omitempty"`
	Name      string `json:"userName,omitempty"`
	DrawOrder int    `json:"-"`
}

var clients = make(map[*websocket.Conn]*Client)
var port = "8899"

func main() {

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/topic/", topicHandler)
	http.HandleFunc("/ws/draw/", drawWsHandler)
	http.HandleFunc("/ws/room/", roomWsHandler)
	http.HandleFunc("/room/", roomHandler)          // create, list room .etc
	http.HandleFunc("/newDrawWs", newDrawWsHandler) // test new ws dynamically
	http.HandleFunc("/test/", testHandler)          // test print

	log.Println("server start at :8899")

	if v := os.Getenv("PORT"); len(v) > 0 {
		port = v
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))

}

var topics = make(map[string]*Topic)
var roomTopic = make(map[string]*TopicDetail)

func init() {
	category := &Category{}
	err := getSampleTopicFile("config.json", category)
	if err != nil {
		log.Println("category config not exist!!")
		return
	}
	for _, category := range category.Category {
		topic := &Topic{}
		err = getSampleTopicFile(category+".json", topic)
		if err != nil {
			log.Println("category: " + category + "not exist!!")
			return
		}
		topics[category] = topic
	}
	log.Println("topics load success!!")
}

func randomTopic() (category string, topic string) {
	tmpCategory := ""
	for randomCategory := range topics {
		tmpCategory = randomCategory
		break
	}
	tmpTopics := topics[tmpCategory].Topics
	tmpTopic := ""
	rand.Seed(time.Now().Unix())
	tmpTopic = tmpTopics[rand.Intn(len(tmpTopics))]
	return tmpCategory, tmpTopic
}

func topicHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.URL.Path == "/topic/list" {
		jsonString, err := json.Marshal(topics)
		if err != nil {
			fmt.Fprint(w, "{}")
			return
		}
		fmt.Fprint(w, string(jsonString))
	} else if r.URL.Path == "/topic/random" {
		category, topic := randomTopic()
		text := `{"category":"` + category + `","` + `topic":"` + topic + `"}`
		fmt.Fprint(w, text)
	}

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
	if userId == "" {
		return
	}

	currentClientsMap := currentRoom.ClientsMap
	client, userErr := currentClientsMap[userId]
	if userErr == false {
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	log.Println("connect !!")

	roomsMap[roomId].ClientsMap[userId].DrawConn = conn
	clients[conn] = new(Client)
	clients[conn].RoomId = roomId
	clients[conn].User = &User{client.User.Id, client.User.Name, client.User.DrawOrder}
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
				if client.DrawConn == nil {
					break
				}
				err = client.DrawConn.WriteMessage(mtype, msg)
				if err != nil {
					log.Println("write:", err)
					break
				}
			}
		}
	}
}

type Message struct {
	User    *User  `json:"user,omitempty"`
	Message string `json:"message,omitempty"`
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
	if userId == "" {
		return
	}

	currentClientsMap := currentRoom.ClientsMap
	client, userErr := currentClientsMap[userId]
	if userErr == false {
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	log.Println("connect !!")
	conn.WriteMessage(1, []byte("HI"))
	roomsMap[roomId].ClientsMap[userId].RoomConn = conn
	clients[conn] = new(Client)
	clients[conn].RoomId = roomId
	clients[conn].User = &User{client.User.Id, client.User.Name, client.User.DrawOrder}
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
				msgStr := string(msg)

				respMsgStruct := &Message{clients[conn].User, msgStr}
				if client.RoomConn == nil {
					break
				}
				currentRoomTopicDetail, topicExist := roomTopic[roomId]
				currentTopic := currentRoomTopicDetail.Topic
				if topicExist && msgStr == currentTopic {
				} else {
					respMsg, err := json.Marshal(respMsgStruct)

					err = client.RoomConn.WriteMessage(mtype, respMsg)
					if err != nil {
						log.Println("write:", err)
						break
					}
				}
			}
		}
	}
}

func newDrawWsHandler(w http.ResponseWriter, r *http.Request) {
	roomId := generateRoomId()
	println(roomId)
	roomsMap[roomId] = &Room{roomId, "test", make(map[string]*Client), false, 0}
	fmt.Fprint(w, roomId)
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.URL.Path != "/" {
		errorHandler(w, r, http.StatusNotFound)
		return
	}

	t, err := template.ParseFiles("./template/html/home.html")
	if t != nil {
		t.Execute(w, nil)
	} else {
		println(err.Error())
	}

	// fmt.Fprintln(w, "/ws/draw/${roomId}?userId=${userId} for <br>")
	// fmt.Fprintln(w, "/ws/room/${roomId}?userId=${userId} for<br>")
	// fmt.Fprintln(w, "/room/list for list rooms<br>")
	// fmt.Fprintln(w, "/room/create for create a room<br>")
	// fmt.Fprintln(w, "/room/join?userId=${userId}&roomId${roomId} for user to join a room<br>")
	// fmt.Fprintln(w, "/room/quit?userId=${userId}&roomId${roomId} for user to quit a room<br>")
	// for topicCategory, topic := range topics {
	// 	text := "Topic Category: " + topicCategory
	// 	jsonString, _ := json.Marshal(topic)
	// 	fmt.Fprintln(w, text)
	// 	fmt.Fprintln(w, string(jsonString))
	// 	println(string(jsonString) + "<br>")
	// }
}

func errorHandler(w http.ResponseWriter, r *http.Request, status int) {
	w.WriteHeader(status)
	if status == http.StatusNotFound {
		fmt.Fprint(w, "custom 404")
	}
}

func roomHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/room/list" {
		roomListHandler(w, r)
		return
	} else if r.URL.Path == "/room/users" {
		roomUsersHandler(w, r)
		return
	} else if r.URL.Path == "/room/create" {
		roomCreateHandler(w, r)
		return
	} else if r.URL.Path == "/room/join" {
		roomJoinHandler(w, r)
		return
	} else if r.URL.Path == "/room/quit" {
		roomQuitHandler(w, r)
		return
	} else if r.URL.Path == "/room/startDraw" {
		roomStartGameHandler(w, r)
		return
	} else if r.URL.Path == "/room/topic" {
		roomTopicHandler(w, r)
		return
	}

}

func roomUsersHandler(w http.ResponseWriter, r *http.Request) {
	roomId := r.URL.Query().Get("roomId")
	room, roomExist := roomsMap[roomId]
	if roomId == "" || !roomExist {
		fmt.Fprint(w, http.StatusBadRequest)
	} else {
		jsonBytes, err := json.Marshal(room)
		if err != nil {
			fmt.Fprint(w, http.StatusBadRequest)
		}
		fmt.Fprint(w, string(jsonBytes))
	}
}

type TopicDetail struct {
	Category string `json:"category,omitempty"`
	Topic    string `json:"topic,omitempty"`
	UserId   string `json:"userId,omitempty"`
}

func roomStartGameHandler(w http.ResponseWriter, r *http.Request) {
	category, topic := randomTopic()
	roomId := r.URL.Query().Get("roomId")
	room, roomExist := roomsMap[roomId]
	if roomId == "" || !roomExist {
		fmt.Fprint(w, http.StatusBadRequest)
	} else {
		userId := userToDrawDispatcher(room)
		topicDetail := &TopicDetail{category, topic, userId}
		jsonBytes, err := json.Marshal(topicDetail)
		if err != nil {
			fmt.Fprint(w, http.StatusBadRequest)
		}
		roomTopic[roomId] = topicDetail
		fmt.Fprint(w, string(jsonBytes))
	}
}
func userToDrawDispatcher(room *Room) string {

	targetUserId := ""
	clientsMap := room.ClientsMap
	for _, client := range clientsMap {
		if client.User.DrawOrder == room.CurrentDrawOrder {
			targetUserId = client.User.Id
		}
	}
	room.CurrentDrawOrder += 1
	room.CurrentDrawOrder %= len(clientsMap)
	return targetUserId
}

func roomTopicHandler(w http.ResponseWriter, r *http.Request) {
	roomId := r.URL.Query().Get("roomId")
	topicDetail := roomTopic[roomId]
	jsonBytes, err := json.Marshal(topicDetail)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprint(w, string(jsonBytes))
}

func roomListHandler(w http.ResponseWriter, r *http.Request) {

	rooms := make([]Room, 0, len(roomsMap))
	for _, room := range roomsMap {
		rooms = append(rooms, *room)
	}
	jsonBytes, err := json.Marshal(rooms)
	if err != nil {
		println(err)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, string(jsonBytes))
}

var roomsMap = make(map[string]*Room)

func roomCreateHandler(w http.ResponseWriter, r *http.Request) {

	roomTmp := &Room{}
	err := json.NewDecoder(r.Body).Decode(roomTmp)
	if err != nil {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}
	roomName := roomTmp.Name
	if roomName == "" {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}
	roomId := generateRoomId()
	room := &Room{roomId, roomName, make(map[string]*Client), false, 1}
	jsonBytes, err := json.Marshal(room)
	if err != nil {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}
	roomsMap[roomId] = room
	fmt.Fprintln(w, string(jsonBytes))
}

type UserJoinRoom struct {
	UserId   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
	RoomId   string `json:"roomId,omitempty"`
}

func roomJoinHandler(w http.ResponseWriter, r *http.Request) {

	userJoinRoom := &UserJoinRoom{}
	err := json.NewDecoder(r.Body).Decode(userJoinRoom)
	if err != nil {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}
	userJoinRoom.UserId = generateUserId()
	result := joinRoomById(userJoinRoom.RoomId)
	if result {
		jsonBytes, err := json.Marshal(userJoinRoom)
		if err != nil {
			fmt.Fprintln(w, http.StatusBadRequest)
			return
		}
		tmpUser := &User{userJoinRoom.UserId, userJoinRoom.UserName, len(roomsMap[userJoinRoom.RoomId].ClientsMap) + 1}
		roomsMap[userJoinRoom.RoomId].ClientsMap[tmpUser.Id] = &Client{userJoinRoom.RoomId, tmpUser, nil, nil}
		fmt.Fprintln(w, string(jsonBytes))
		return
	} else {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}

}
func roomQuitHandler(w http.ResponseWriter, r *http.Request) {
	userId := r.URL.Query().Get("userId")
	if userId == "" {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}
	roomId := r.URL.Query().Get("roomId")
	result := quitRoomById(userId, roomId)
	if result {
		delete(roomsMap[roomId].ClientsMap, userId)
		fmt.Fprintln(w, http.StatusOK)
		return
	} else {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}

}

func joinRoomById(roomId string) bool {
	room, roomExist := roomsMap[roomId]
	if !roomExist || room.RoomJoinLock {
		return false
	}
	return true
}

func quitRoomById(userId string, roomId string) bool {
	room, roomExist := roomsMap[roomId]
	if roomExist == false {
		return false
	}
	_, usertExist := room.ClientsMap[userId]
	if usertExist == false {
		return false
	}
	return true
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

type Category struct {
	Category []string `json:"category,omitempty"`
}

type Topic struct {
	Topics []string `json:"topics,omitempty"`
}

func getSampleTopicFile(fileName string, v interface{}) error {
	file, err := os.Open("sample/topic/" + fileName)
	if err != nil {
		log.Println(err)
		return err
	}
	defer file.Close()
	err = json.NewDecoder(file).Decode(v)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}
