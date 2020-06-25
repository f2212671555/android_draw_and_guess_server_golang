package main

import (
	"drawAndGuessServer/data/structure/bean"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

	"drawAndGuessServer/data/structure/linkedlist"
)

type Room struct {
	RoomId           string                     `json:"roomId,omitempty"`
	RoomName         string                     `json:"roomName,omitempty"`
	Users            *linkedlist.UserLinkedList `json:"users,omitempty"`
	CurrentDrawOrder int                        `json:"currentDrawOrder"`
	NextDrawOrder    int                        `json:"nextDrawOrder"`
	TopicDetail      *TopicDetail               `json:"topicDetail,omitempty"`
}

type TopicDetail struct {
	Category          string `json:"category,omitempty"`
	Topic             string `json:"topic,omitempty"`
	CurrentDrawUserId string `json:"currentDrawUserId,omitempty"`
	NextDrawUserId    string `json:"nextDrawUserId,omitempty"`
	Result            *bool  `json:"result,omitempty"`
}

type Message struct {
	Type     string `json:"type,omitempty"`
	UserId   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
	RoomId   string `json:"roomId,omitempty"`
	Message  string `json:"message,omitempty"`
	Result   *bool  `json:"result,omitempty"`
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
	Role     string `json:"role,omitempty"`
}

type UserJoinRoomBean struct {
	UserId   string `json:"userId,omitempty"`
	UserName string `json:"userName,omitempty"`
	RoomId   string `json:"roomId,omitempty"`
	RoomName string `json:"roomName,omitempty"`
	Result   *bool  `json:"result,omitempty"`
	Role     string `json:"role,omitempty"`
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
	http.HandleFunc("/.well-known/assetlinks.json", appLinkHandler)
	// http.Handle("/public/", http.FileServer(http.Dir("./public/picture/")))

	if v := os.Getenv("PORT"); len(v) > 0 {
		port = v
	}
	log.Println("server start at :" + port)
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
	currentRoom := currentRoomInterface.(*Room)
	currentUser, userExist := currentRoom.Users.LookFor(currentUserId) // check user is join
	if userExist == false {
		return
	}

	upgrader := &websocket.Upgrader{

		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil) // get *conn
	conn.SetPingHandler(func(s string) error {
		log.Println("drawWsHandler get ping!!!")
		conn.WriteMessage(websocket.PongMessage, []byte("pong"))
		return nil
	})
	currentUser.DrawConn = conn
	log.Println("drawWsHandler connect !!")

	if err != nil {
		log.Println("upgrade:", err)
		return
	}

	defer func() {
		log.Println("drawWsHandler disconnect !!")
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
		// concurrent loop - begin
		roomUsers.RLock()
		userNode := roomUsers.Head()
		if userNode != nil {
			for {
				if userNode.Content().UserId != currentUserId { // do not send msg to (s)hseself
					if userNode.Content().DrawConn == nil {
						log.Println("user DrawConn is nil!!")
						break
					}
					err = userNode.Content().DrawConn.WriteMessage(mtype, msg)
					if err != nil {
						log.Println("write:", err)
						break
					}
				}
				if userNode.Next() == nil {
					break
				}
				userNode = userNode.Next()
			}
		}
		roomUsers.RUnlock()
		// concurrent loop - end
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
	currentUser, userExist := currentRoom.Users.LookFor(currentUserId) // check user is join
	if userExist == false {
		return
	}

	result := true
	currentUser.Ready = &result
	upgrader := &websocket.Upgrader{

		CheckOrigin: func(r *http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil) // get *conn
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	conn.SetPingHandler(func(s string) error {
		log.Println("roomWsHandler get ping!!!")
		conn.WriteMessage(websocket.PongMessage, []byte("pong"))
		return nil
	})

	currentUser.RoomConn = conn
	log.Println("roomWsHandler connect !!")

	// send others you join
	sendAction(currentUser, "join")

	defer func() {
		log.Println("roomWsHandler disconnect !!")
		currentUser.RoomConn = nil
		// send others you quit
		sendAction(currentUser, "quit")
		// user quit room
		roomInterface, roomExist := roomsMap.Get(currentRoomId)
		if roomExist {
			room := roomInterface.(*Room)
			// adjust drawOrder
			adjustDrawOrder(room, currentUser.DrawOrder)
			if room.TopicDetail != nil {
				if room.TopicDetail.CurrentDrawUserId == currentUserId {
					room.TopicDetail.NextDrawUserId = getNextDrawOrderUserId(room)
				}
			}
			//remove user form room's user map
			room.Users.Remove(currentUserId)
			if room.Users.Size() == 0 {
				roomsMap.Remove(currentRoomId)
			}
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

		currentRoomInterface, exist := roomsMap.Get(currentRoomId)
		if exist == false {
			break
		}
		reqMessage := &Message{}
		err = json.Unmarshal(msg, reqMessage)
		if err != nil {
			println(err)
			break
		}
		currentRoom := currentRoomInterface.(*Room)

		if reqMessage.Type == "answer" { // answer question
			checkAnswer(currentRoom, reqMessage, mtype)
		} else if reqMessage.Type == "ready" {
			roomUsers := currentRoom.Users
			user, exist := roomUsers.LookFor(currentUserId)
			if !exist {
				return
			}

			setReadyFlag(user)
			var result = checkAllReadyFlag(currentRoom)
			if result {
				clearAllReadyFlag(currentRoom)
				sendNextDrawTopicDetail(currentRoom, mtype)
			}
		} else if reqMessage.Type == "startDraw" {
			// clearAllReadyFlag(currentRoom)
			result := true
			reqMessage.Result = &result
			sendReqMessage(reqMessage, currentRoom, mtype)
		} else {
			result := true
			reqMessage.Result = &result
			sendReqMessage(reqMessage, currentRoom, mtype)
		}
	}
}

func sendNextDrawTopicDetail(room *Room, mtype int) {
	userId := room.TopicDetail.NextDrawUserId // get the next draw userId in this room
	roomUsers := room.Users
	user, exist := roomUsers.LookFor(userId)
	if !exist {
		return //----
	}

	result := true
	reqMessage := &Message{"nextDraw", userId, user.UserName, room.RoomId, "", &result}
	sendReqMessageTo(reqMessage, user, mtype)
}

func sendReqMessageTo(reqMessage *Message, user *bean.User, mtype int) {

	if user.RoomConn != nil {
		respMsg, err := json.Marshal(reqMessage)
		err = user.RoomConn.WriteMessage(mtype, respMsg)
		if err != nil {
			log.Println("write:", err)
			return
		}
	}
}

func sendReqMessage(reqMessage *Message, room *Room, mtype int) {
	// concurrent loop - begin
	room.Users.RLock()
	userNode := room.Users.Head()
	for {
		sendReqMessageTo(reqMessage, userNode.Content(), mtype)
		if userNode.Next() == nil {
			break
		}
		userNode = userNode.Next()
	}
	room.Users.RUnlock()
	// concurrent loop - end
}

func checkAnswer(room *Room, reqMessage *Message, mtype int) {

	currentTopic := room.TopicDetail.Topic
	result := false
	if currentTopic == reqMessage.Message {
		result = true
	}
	reqMessage.Result = &result
	sendReqMessage(reqMessage, room, mtype)

}

func setReadyFlag(user *bean.User) {
	flag := true
	user.Ready = &flag
}

func checkAllReadyFlag(room *Room) bool {
	// concurrent loop - begin
	room.Users.RLock()
	defer room.Users.RUnlock()
	node := room.Users.Head() // get the users in this room
	for {
		if *(node.Content().Ready) == false {
			return false
		}
		if node.Next() == nil {
			break
		}
		node = node.Next()
	}
	return true
	// concurrent loop - end
}

func clearAllReadyFlag(room *Room) {
	// concurrent loop - begin
	room.Users.RLock()
	defer room.Users.RUnlock()
	node := room.Users.Head() // get the users in this room
	flag := false
	for {
		node.Content().Ready = &flag
		if node.Next() == nil {
			break
		}
		node = node.Next()
	}
	// concurrent loop - end
}

func sendAction(currentUser *bean.User, action string) {
	currentRoomInterface, exist := roomsMap.Get(currentUser.RoomId)
	if exist == false {
		return
	}
	currentRoom := currentRoomInterface.(*Room)
	// concurrent loop - begin
	currentRoom.Users.RLock()
	defer currentRoom.Users.RUnlock()
	node := currentRoom.Users.Head() // get the users in this room
	for {
		if node.Content().RoomConn != nil {
			result := false
			reqMessage := &Message{action, currentUser.UserId, currentUser.UserName, currentUser.RoomId, "", &result}
			respMsg, err := json.Marshal(reqMessage)
			err = node.Content().RoomConn.WriteMessage(1, respMsg)
			if err != nil {
				log.Println("write:", err)
				return
			}
		}
		if node.Next() == nil {
			break
		}
		node = node.Next()
	}
	// concurrent loop - end
}

func adjustDrawOrder(room *Room, quitUserDrawOrder int) {
	// concurrent loop - begin
	room.Users.RLock()
	defer room.Users.RUnlock()
	node := room.Users.Head() // get the users in this room
	for {
		if node.Content().DrawOrder > quitUserDrawOrder {
			node.Content().DrawOrder -= 1
		}
		if node.Next() == nil {
			break
		}
		node = node.Next()
	}
	// concurrent loop - end
}

func appLinkHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	file, err := os.Open(".well-known/assetlinks.json")
	if err != nil {
		log.Println(err)
		return
	}
	defer file.Close()
	byteValue, _ := ioutil.ReadAll(file)

	fmt.Fprint(w, string(byteValue))
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
	} else if r.URL.Path == "/room/cleanAll" {
		roomCleanAllHandler(w, r)
		return
	} else if r.URL.Path == "/room/listAll" {
		roomListAllHandler(w, r)
		return
	}

}
func roomListAllHandler(w http.ResponseWriter, r *http.Request) {

	jsonBytes, err := json.Marshal(roomsMap)
	if err != nil {
		println(err)
		return
	}
	fmt.Fprint(w, string(jsonBytes))
}
func roomCleanAllHandler(w http.ResponseWriter, r *http.Request) {
	roomsMap = cmap.New()
	fmt.Fprint(w, "room Clean all!!")
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
		// concurrent loop - begin
		room.Users.RLock()
		userNode := room.Users.Head() // get the users in this room
		for {
			user := userNode.Content()
			userBean := UserBean{user.RoomId, user.UserId, user.UserName, user.Role}
			roomBean.UserBeans = append(roomBean.UserBeans, userBean)
			if userNode.Next() == nil {
				break
			}
			userNode = userNode.Next()
		}
		room.Users.RUnlock()
		// concurrent loop - end
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
	result := true
	if roomId == "" || !roomExist {
		result = false
	}
	topicDetail := &TopicDetail{"", "", "", "", &result}
	if result {
		room := roomInterface.(*Room)

		if room.Users.Size() == 0 {
			result = false
		} else {
			userId := userToDrawDispatcher(room)
			topicDetail.Category = category
			topicDetail.Topic = topic
			topicDetail.CurrentDrawUserId = userId
			topicDetail.NextDrawUserId = getNextDrawOrderUserId(room)
		}

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

	room.NextDrawOrder = room.CurrentDrawOrder + 1
	if room.Users.Size() == 0 {
		return ""
	}
	room.NextDrawOrder %= room.Users.Size() // next draw order

	room.CurrentDrawOrder = room.NextDrawOrder
	targetUserId := ""
	// concurrent loop - begin
	room.Users.RLock()
	defer room.Users.RUnlock()
	node := room.Users.Head() // get the users in this room
	for {
		if node.Content().DrawOrder == room.CurrentDrawOrder {
			targetUserId = node.Content().UserId
			room.TopicDetail.CurrentDrawUserId = targetUserId
		}
		if node.Next() == nil {
			break
		}
		node = node.Next()
	}
	// concurrent loop - end
	return targetUserId
}

func getNextDrawOrderUserId(room *Room) string {

	targetUserId := ""
	roomUsers := room.Users
	room.NextDrawOrder = room.CurrentDrawOrder + 1
	if roomUsers.Size() == 0 {
		return ""
	}
	room.NextDrawOrder %= roomUsers.Size() // next draw order
	// concurrent loop - begin
	room.Users.RLock()
	defer room.Users.RUnlock()
	node := room.Users.Head() // get the users in this room
	for {
		if node.Content().DrawOrder == room.NextDrawOrder {
			targetUserId = node.Content().UserId
			room.TopicDetail.NextDrawUserId = targetUserId
		}
		if node.Next() == nil {
			break
		}
		node = node.Next()
	}
	// concurrent loop - end
	return targetUserId
}

func roomTopicHandler(w http.ResponseWriter, r *http.Request) {
	roomId := r.URL.Query().Get("roomId")
	result := true
	roomInterface, roomExist := roomsMap.Get(roomId)
	if roomExist == false {
		result = false

	}
	topicDetail := &TopicDetail{"", "", "", "", &result}

	if result {
		room := roomInterface.(*Room)
		if room.TopicDetail != nil && room.Users.Size() > 0 {
			topicDetail = room.TopicDetail
		} else {
			result = false
		}
	}
	topicDetail.Result = &result
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
		userBeans := make([]UserBean, 0, room.Users.Size())
		// concurrent loop - begin
		room.Users.RLock()
		node := room.Users.Head() // get the users in this room
		if node != nil {
			for {
				user := node.Content()
				userBean := UserBean{user.RoomId, user.UserId, user.UserName, user.Role}
				userBeans = append(userBeans, userBean)
				if node.Next() == nil {
					break
				}
				node = node.Next()
			}
		}
		room.Users.RUnlock()
		// concurrent loop - end
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
		room := &Room{roomId, roomName, &linkedlist.UserLinkedList{}, -1, 0, &TopicDetail{}}
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
	if userJoinRoomBean.UserName == "" {
		result = false
	}
	userJoinRoomBean.Result = &result
	if result {
		room := roomInterface.(*Room)
		userJoinRoomBean.UserId = generateUserId()
		userJoinRoomBean.RoomName = room.RoomName
		tmpUser := &bean.User{RoomId: userJoinRoomBean.RoomId, UserId: userJoinRoomBean.UserId,
			UserName: userJoinRoomBean.UserName, DrawConn: nil, RoomConn: nil, DrawOrder: room.Users.Size(), Ready: &result, Role: userJoinRoomBean.Role}
		room.Users.Append(tmpUser)
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
		_, usertExist := room.Users.LookFor(userId)
		if usertExist == false {
			result = false
			println("this user is not exist!!")
		}
	}

	userJoinRoomBean := &UserJoinRoomBean{"", "", "", "", &result, ""}
	if result {
		room := roomInterface.(*Room)
		userJoinRoomBean.RoomId = roomId
		userJoinRoomBean.UserId = userId
		user, _ := room.Users.LookFor(userId)
		userJoinRoomBean.UserName = user.UserName
		userJoinRoomBean.RoomName = room.RoomName
		userJoinRoomBean.Result = &result
		room.Users.Remove(userId)
		if room.Users.Size() == 0 {
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
