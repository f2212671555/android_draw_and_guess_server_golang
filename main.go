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

type Room struct {
	Id               string           `json:"roomId,omitempty"`
	Name             string           `json:"roomName,omitempty"`
	Users            map[string]*User `json:"users,omitempty"`
	CurrentDrawOrder int              `json:"-"`
}

type User struct {
	RoomId    string          `json:"roomId,omitempty"`
	UserId    string          `json:"userId,omitempty"`
	UserName  string          `json:"userName,omitempty"`
	DrawConn  *websocket.Conn `json:"DrawConn,omitempty"`
	RoomConn  *websocket.Conn `json:"RoomConn,omitempty"`
	DrawOrder int             `json:"-"`
}

type TopicDetail struct {
	Category string `json:"category,omitempty"`
	Topic    string `json:"topic,omitempty"`
	UserId   string `json:"userId,omitempty"`
}

type Message struct {
	Type    string `json:"type,omitempty"`
	User    *User  `json:"user,omitempty"`
	Message string `json:"message,omitempty"`
	Result  *bool  `json:"result,omitempty"`
}

type UserJoinRoom struct {
	UserId   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
	RoomId   string `json:"roomId,omitempty"`
	Result   *bool  `json:"result,omitempty"`
}

var topics = make(map[string]*Topic)          // store topics
var roomTopic = make(map[string]*TopicDetail) // store rooms' topic
var roomsMap = make(map[string]*Room)         // store rooms with roomId
var port = "8899"

func main() {

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/topic/", topicHandler)
	http.HandleFunc("/ws/draw/", drawWsHandler)
	http.HandleFunc("/ws/room/", roomWsHandler)
	http.HandleFunc("/room/", roomHandler) // create, list room .etc

	log.Println("server start at :8899")

	if v := os.Getenv("PORT"); len(v) > 0 {
		port = v
	}
	log.Fatal(http.ListenAndServe(":"+port, nil))

}

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

	currentRoomId := strings.Split(r.URL.Path, "/ws/draw/")[1] // get room id
	currentUserId := r.URL.Query().Get("userId")               // get user id
	if currentUserId == "" {                                   // check userId empty
		return
	}
	_, roomExist := roomsMap[currentRoomId] // check room exist , get room
	if roomExist == false {
		return
	}

	upgrader := &websocket.Upgrader{

		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil) // get *conn
	log.Println("connect !!")

	if err != nil {
		log.Println("upgrade:", err)
		return
	}

	// welcomeMessage := &Message{"", client.User, "HI", nil}
	// respMsg, err := json.Marshal(welcomeMessage)
	// if err != nil {
	// 	return
	// }
	// conn.WriteMessage(1, respMsg)

	defer func() {
		log.Println("disconnect !!")
		conn.Close()
	}()

	for {
		mtype, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("receive: %s\n", msg)

		currentRoom, exist := roomsMap[currentRoomId] // get current room
		if exist == false {
			return
		}

		roomUsers := currentRoom.Users // get the users in this room
		for userId, user := range roomUsers {
			if userId != currentUserId { // do not send msg to (s)hseself
				if user.DrawConn == nil {
					log.Println("user DrawConn is nil!!")
					break
				}
				err = user.DrawConn.WriteMessage(mtype, msg)
				if err != nil {
					log.Println("write:", err)
					break
				}
			}
		}
	}
}

func roomWsHandler(w http.ResponseWriter, r *http.Request) {

	currentRoomId := strings.Split(r.URL.Path, "/ws/room/")[1]
	currentUserId := r.URL.Query().Get("userId")
	if currentUserId == "" {
		return
	}
	_, roomExist := roomsMap[currentRoomId]
	if roomExist == false {
		return
	}

	upgrader := &websocket.Upgrader{

		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	log.Println("connect !!")

	// welcomeMessage := &Message{"", client.User, "HI", nil}
	// respMsg, err := json.Marshal(welcomeMessage)
	// if err != nil {
	// 	return
	// }
	// conn.WriteMessage(1, respMsg)

	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer func() {
		log.Println("disconnect !!")
		conn.Close()
	}()

	for {
		mtype, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("receive: %s\n", msg)

		currentRoom, exist := roomsMap[currentRoomId]
		if exist == false {
			return
		}

		roomUsers := currentRoom.Users // get the users in this room
		for _, user := range roomUsers {
			// if userId != currentUserId { // do not send msg to (s)hseself
			if user.RoomConn == nil {
				break
			}
			msgStr := string(msg)
			result := false
			currentRoomTopicDetail, topicExist := roomTopic[currentRoomId]
			if topicExist {
				currentTopic := currentRoomTopicDetail.Topic
				if currentTopic == msgStr {
					result = true
				}
			}

			respMsgStruct := &Message{"", user, msgStr, &result}
			respMsg, err := json.Marshal(respMsgStruct)
			err = user.RoomConn.WriteMessage(mtype, respMsg)
			if err != nil {
				log.Println("write:", err)
				break
			}
			// }
		}
	}
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
	roomUsers := room.Users
	for _, user := range roomUsers {
		if user.DrawOrder == room.CurrentDrawOrder {
			targetUserId = user.UserId
		}
	}
	room.CurrentDrawOrder += 1
	room.CurrentDrawOrder %= len(roomUsers)
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
	room := &Room{roomId, roomName, make(map[string]*User), 1}
	jsonBytes, err := json.Marshal(room)
	if err != nil {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}
	roomsMap[roomId] = room
	fmt.Fprintln(w, string(jsonBytes))
}

func roomJoinHandler(w http.ResponseWriter, r *http.Request) {

	userJoinRoom := &UserJoinRoom{}
	err := json.NewDecoder(r.Body).Decode(userJoinRoom)

	if err != nil {
		println(err)
		return
	}
	result := false
	userJoinRoom.UserId = generateUserId()
	room, roomExist := roomsMap[userJoinRoom.RoomId] //
	if roomExist {

		jsonBytes, err := json.Marshal(userJoinRoom)
		if err != nil {
			println(err)
			return
		}
		result = true
		tmpUser := &User{userJoinRoom.RoomId, userJoinRoom.UserId,
			userJoinRoom.UserName, nil, nil, len(room.Users) + 1}
		room.Users[userJoinRoom.UserId] = tmpUser
		userJoinRoom.Result = &result
		fmt.Fprintln(w, string(jsonBytes))
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
	room, roomExist := roomsMap[roomId]
	if roomExist == false {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}
	_, usertExist := room.Users[userId]
	if usertExist == false {
		fmt.Fprintln(w, http.StatusBadRequest)
		return
	}
	delete(roomsMap[roomId].Users, userId)
	fmt.Fprintln(w, http.StatusOK)
	return

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
