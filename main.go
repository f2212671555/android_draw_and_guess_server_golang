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
	cmap "github.com/orcaman/concurrent-map"
	uuid "github.com/satori/go.uuid"
)

type Room struct {
	RoomId           string             `json:"roomId,omitempty"`
	RoomName         string             `json:"roomName,omitempty"`
	Users            cmap.ConcurrentMap `json:"users,omitempty"`
	CurrentDrawOrder int                `json:"-"`
	TopicDetail      *TopicDetail       `json:"topicDetail,omitempty"`
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

type Category struct {
	Category []string `json:"category,omitempty"`
}

type Topic struct {
	Topics []string `json:"topics,omitempty"`
}

var topics cmap.ConcurrentMap   // store topics
var roomsMap cmap.ConcurrentMap // store rooms with roomId

func initSafeMap() {
	topics = cmap.New()
	roomsMap = cmap.New()
}

var port = "8899"

func main() {

	initSafeMap()
	loading()
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

func loading() {
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
		topics.Set(category, topic)
	}
	log.Println("topics load success!!")
}

func randomTopic() (category string, topic string) {
	tmpCategory := ""
	tmpTopic := ""
	for item := range topics.Iter() { // get category
		tmpCategory = item.Key
		break
	}

	topicsInterface, exist := topics.Get(tmpCategory)
	if exist {
		topic := topicsInterface.(*Topic)
		rand.Seed(time.Now().Unix())
		tmpTopic = topic.Topics[rand.Intn(len(topic.Topics))]
	}

	return tmpCategory, tmpTopic
}

func topicHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.URL.Path == "/topic/list" {
		jsonString, err := json.Marshal(topics)
		if err != nil {
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

	currentRoomInterface, roomExist := roomsMap.Get(currentRoomId) // check room exist , get room
	if roomExist == false {
		return
	}
	currentRoom := currentRoomInterface.(*Room)                             // interface{} to room
	currentUserInterface, userExist := currentRoom.Users.Get(currentUserId) // check user is login
	if userExist == false {
		return
	}
	currentUser := currentUserInterface.(*User)
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

		currentRoomInterface, exist := roomsMap.Get(currentRoomId) // get current room
		if exist == false {
			return
		}
		currentRoom := currentRoomInterface.(*Room)
		roomUsers := currentRoom.Users // get the users in this room
		for item := range roomUsers.Iter() {
			// if userId != currentUserId { // do not send msg to (s)hseself
			userInterface := item.Val
			user := userInterface.(*User)
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

	currentRoomInterface, roomExist := roomsMap.Get(currentRoomId)
	if roomExist == false {
		return
	}
	currentRoom := currentRoomInterface.(*Room)
	currentUserInterface, userExist := currentRoom.Users.Get(currentUserId) // check user is login
	if userExist == false {
		return
	}
	currentUser := currentUserInterface.(*User)
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

		currentRoomInterface, exist := roomsMap.Get(currentRoomId)
		if exist == false {
			return
		}
		currentRoom := currentRoomInterface.(*Room)
		roomUsers := currentRoom.Users // get the users in this room
		for item := range roomUsers.Iter() {
			// if userId != currentUserId { // do not send msg to (s)hseself
			userInterface := item.Val
			user := userInterface.(*User)
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
	roomInterface, roomExist := roomsMap.Get(roomId)
	if roomId == "" || !roomExist {
		result = false
	}
	room := roomInterface.(*Room)
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
	roomInterface, roomExist := roomsMap.Get(roomId)
	room := roomInterface.(*Room)
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
	for item := range roomUsers.Iter() {
		userInterface := item.Val
		user := userInterface.(*User)
		if user.DrawOrder == room.CurrentDrawOrder {
			targetUserId = user.UserId
		}
	}
	room.CurrentDrawOrder += 1
	room.CurrentDrawOrder %= roomUsers.Count()
	return targetUserId
}

func roomTopicHandler(w http.ResponseWriter, r *http.Request) {
	roomId := r.URL.Query().Get("roomId")
	result := true
	roomInterface, roomExist := roomsMap.Get(roomId)
	if roomExist == false {
		result = false
	}
	room := roomInterface.(*Room)
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

	roomBeans := make([]RoomBean, 0, roomsMap.Count())
	for item := range roomsMap.Iter() {
		roomInterface := item.Val
		room := roomInterface.(*Room)
		userBeans := make([]UserBean, 0, room.Users.Count())
		for item := range room.Users.Iter() {
			userInterface := item.Val
			user := userInterface.(*User)
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
		room := &Room{roomId, roomName, cmap.New(), 1, nil}
		roomsMap.Set(roomId, room)
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

	roomInterface, roomExist := roomsMap.Get(userJoinRoomBean.RoomId)
	if roomExist == false {
		result = false
	}

	userJoinRoomBean.Result = &result
	if result {
		room := roomInterface.(*Room)
		userJoinRoomBean.UserId = generateUserId()
		tmpUser := &User{userJoinRoomBean.RoomId, userJoinRoomBean.UserId,
			userJoinRoomBean.UserName, nil, nil, room.Users.Count() + 1}
		room.Users.Set(userJoinRoomBean.UserId, tmpUser)
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
	roomInterface, roomExist := roomsMap.Get(roomId)
	if roomExist == false {
		result = false
		println("this room is not exist!!")
	} else {
		room := roomInterface.(*Room)
		_, usertExist := room.Users.Get(userId)
		if usertExist == false {
			result = false
			println("this user is not exist!!")
		}
	}

	userJoinRoomBean := &UserJoinRoomBean{"", "", "", &result}
	if result {
		room := roomInterface.(*Room)
		userJoinRoomBean.RoomId = roomId
		userJoinRoomBean.UserId = userId
		userInterface, _ := room.Users.Get(userId) //
		user := userInterface.(*User)
		userJoinRoomBean.UserName = user.UserName
		userJoinRoomBean.Result = &result
		room.Users.Remove(userId)
		if room.Users.Count() == 0 {
			roomsMap.Remove(roomId)
		}
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
