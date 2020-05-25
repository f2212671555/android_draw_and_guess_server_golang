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
	RoomId           string           `json:"roomId,omitempty"`
	RoomName         string           `json:"roomName,omitempty"`
	Users            map[string]*User `json:"users,omitempty"`
	CurrentDrawOrder int              `json:"-"`
	TopicDetail      *TopicDetail     `json:"topicDetail,omitempty"`
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
	Result   *bool  `json:"result,omitempty"`
}

type Message struct {
	Type    string `json:"type,omitempty"`
	User    *User  `json:"user,omitempty"`
	Message string `json:"message,omitempty"`
	Result  *bool  `json:"result,omitempty"`
}

type RoomBean struct {
	RoomId    string     `json:"roomId,omitempty"`
	RoomName  string     `json:"roomName,omitempty"`
	UserBeans []UserBean `json:"users,omitempty"`
	Result    *bool      `json:"result,omitempty"`
}

type UserBean struct {
	RoomId   string `json:"roomId,omitempty"`
	UserId   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
}

type UserJoinRoomBean struct {
	UserId   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
	RoomId   string `json:"roomId,omitempty"`
	Result   *bool  `json:"result,omitempty"`
}

var topics = make(map[string]*Topic)  // store topics
var roomsMap = make(map[string]*Room) // store rooms with roomId
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
	currentRoom, roomExist := roomsMap[currentRoomId] // check room exist , get room
	if roomExist == false {
		return
	}
	currentUser, userExist := currentRoom.Users[currentUserId] // check user is login
	if userExist == false {
		return
	}
	upgrader := &websocket.Upgrader{

		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil) // get *conn
	currentUser.DrawConn = conn
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
		currentUser.DrawConn = nil
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
		for _, user := range roomUsers {
			// if userId != currentUserId { // do not send msg to (s)hseself
			if user.DrawConn == nil {
				log.Println("user DrawConn is nil!!")
				break
			}
			err = user.DrawConn.WriteMessage(mtype, msg)
			if err != nil {
				log.Println("write:", err)
				break
			}
			// }
		}
	}
}

func roomWsHandler(w http.ResponseWriter, r *http.Request) {

	currentRoomId := strings.Split(r.URL.Path, "/ws/room/")[1]
	currentUserId := r.URL.Query().Get("userId")
	if currentUserId == "" {
		return
	}
	currentRoom, roomExist := roomsMap[currentRoomId]
	if roomExist == false {
		return
	}
	currentUser, userExist := currentRoom.Users[currentUserId] // check user is login
	if userExist == false {
		return
	}
	upgrader := &websocket.Upgrader{

		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil) // get *conn
	currentUser.RoomConn = conn
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
		currentUser.RoomConn = nil
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

			if currentRoom.TopicDetail != nil {
				currentTopic := currentRoom.TopicDetail.Topic
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
	result := true
	room, roomExist := roomsMap[roomId]
	if roomId == "" || !roomExist {
		result = false
	}
	roomBean := RoomBean{"", "", nil, &result}
	if result {
		roomBean.RoomId = room.RoomId
		roomBean.RoomName = room.RoomName
	}
	jsonBytes, err := json.Marshal(roomBean)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprint(w, string(jsonBytes))
}

func roomStartGameHandler(w http.ResponseWriter, r *http.Request) {
	category, topic := randomTopic()
	roomId := r.URL.Query().Get("roomId")
	room, roomExist := roomsMap[roomId]
	result := true
	if roomId == "" || !roomExist {
		result = false
	}
	topicDetail := &TopicDetail{"", "", "", &result}
	if result {
		userId := userToDrawDispatcher(room)
		topicDetail.Category = category
		topicDetail.Topic = topic
		topicDetail.UserId = userId
		topicDetail.Result = &result

		room.TopicDetail = topicDetail
	}

	jsonBytes, err := json.Marshal(topicDetail)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprint(w, string(jsonBytes))

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
	result := true
	room, roomExist := roomsMap[roomId]
	if roomExist == false {
		result = false
	}
	topicDetail := TopicDetail{"", "", "", &result}
	if result {
		if room.TopicDetail != nil {
			topicDetail.Category = room.TopicDetail.Category
			topicDetail.Topic = room.TopicDetail.Topic
			topicDetail.UserId = room.TopicDetail.UserId
		}
	}
	jsonBytes, err := json.Marshal(topicDetail)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprint(w, string(jsonBytes))

}

func roomListHandler(w http.ResponseWriter, r *http.Request) {

	roomBeans := make([]RoomBean, 0, len(roomsMap))
	for _, room := range roomsMap {

		userBeans := make([]UserBean, 0, len(room.Users))
		for _, user := range room.Users {
			userBean := UserBean{user.RoomId, user.UserId, user.UserName}
			userBeans = append(userBeans, userBean)
		}
		roomBean := RoomBean{room.RoomId, room.RoomName, userBeans, nil}
		roomBeans = append(roomBeans, roomBean)
	}

	jsonBytes, err := json.Marshal(roomBeans)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprint(w, string(jsonBytes))
}

func roomCreateHandler(w http.ResponseWriter, r *http.Request) {

	roomBean := &RoomBean{}
	result := true
	err := json.NewDecoder(r.Body).Decode(roomBean)
	if err != nil {
		println(err)
		return
	}
	roomName := roomBean.RoomName
	if roomName == "" {
		result = false
		println("roomName is empty!!")
	}

	respRoomBean := &RoomBean{"", roomName, nil, &result}
	if result {
		roomId := generateRoomId()
		respRoomBean.RoomId = roomId
		room := &Room{roomId, roomName, make(map[string]*User), 1, nil}
		roomsMap[roomId] = room
	}

	jsonBytes, err := json.Marshal(respRoomBean)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprintln(w, string(jsonBytes))
}

func roomJoinHandler(w http.ResponseWriter, r *http.Request) {

	userJoinRoomBean := &UserJoinRoomBean{}
	err := json.NewDecoder(r.Body).Decode(userJoinRoomBean)
	if err != nil {
		println(err)
		return
	}
	result := true

	room, roomExist := roomsMap[userJoinRoomBean.RoomId]
	if roomExist == false {
		result = false
	}
	userJoinRoomBean.Result = &result
	if result {
		userJoinRoomBean.UserId = generateUserId()
		tmpUser := &User{userJoinRoomBean.RoomId, userJoinRoomBean.UserId,
			userJoinRoomBean.UserName, nil, nil, len(room.Users) + 1}
		room.Users[userJoinRoomBean.UserId] = tmpUser
	}

	jsonBytes, err := json.Marshal(userJoinRoomBean)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprintln(w, string(jsonBytes))
}

func roomQuitHandler(w http.ResponseWriter, r *http.Request) {
	result := true
	userId := r.URL.Query().Get("userId")
	if userId == "" {
		result = false
		println("user id is empty!!")
	}
	roomId := r.URL.Query().Get("roomId")
	room, roomExist := roomsMap[roomId]
	if roomExist == false {
		result = false
		println("this room is not exist!!")
	} else {
		_, usertExist := room.Users[userId]
		if usertExist == false {
			result = false
			println("this user is not exist!!")
		}
	}

	userJoinRoomBean := &UserJoinRoomBean{"", "", "", &result}
	if result {
		userJoinRoomBean.RoomId = roomId
		userJoinRoomBean.UserId = userId
		userJoinRoomBean.UserName = room.Users[userId].UserName
		userJoinRoomBean.Result = &result
		delete(roomsMap[roomId].Users, userId)
	}

	jsonBytes, err := json.Marshal(userJoinRoomBean)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprintln(w, string(jsonBytes))
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
