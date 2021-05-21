package handlers

import (
	"fmt"
	"log"
	"net/http"
	"sort"

	"github.com/CloudyKit/jet/v6"
	"github.com/gorilla/websocket"
)

// channel for JSON payload
var wsChan = make(chan WsPayload)

// all clients connected to the websocket endpoint mapped to its username
var clients = make(map[WebSocketConnection]string)

var views = jet.NewSet(
	jet.NewOSFileSystemLoader("./html"),
	jet.InDevelopmentMode(),
)

var upgradeConnection = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Home renders the home page
func Home(w http.ResponseWriter, r *http.Request) {
	err := renderPage(w, "home.jet", nil)
	if err != nil {
		log.Println(err)
		return
	}
}

// WebSocketConnection is a wrapper for websocket.Conn
type WebSocketConnection struct {
	*websocket.Conn
}

// WsJsonResponse defines the response sent back from websocket
type WsJsonResponse struct {
	Action         string   `json:"action"`
	Message        string   `json:"message"`
	MessageType    string   `json:"message_type"`
	ConnectedUsers []string `json:"connected_users"`
}

type WsPayload struct {
	Action   string              `json:"action"`
	Username string              `json:"username"`
	Message  string              `json:"message"`
	Conn     WebSocketConnection `json:"-"` // ignored by JSON Marshal/Unmarshal
}

// WsEndPoint upgrades connection to websocket
func WsEndPoint(w http.ResponseWriter, r *http.Request) {
	ws, err := upgradeConnection.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
	}

	log.Println("client connected to endpoint")

	var response WsJsonResponse
	response.Action = "hello_from_server"
	response.Message = `<em><small>connected to server</small></em>`

	conn := WebSocketConnection{
		Conn: ws,
	}
	// save connected client's connection to the map
	clients[conn] = ""

	err = ws.WriteJSON(response)
	if err != nil {
		log.Println(err)
	}

	go ListenForWsPayload(&conn)
}

// ListenForWsPayload listens for JSON payload from websocket clients and send it to wsChan
func ListenForWsPayload(conn *WebSocketConnection) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("error: %v\n", err)
		}
	}()

	var payload WsPayload

	for {
		err := conn.ReadJSON(&payload)
		// err == nil means there is some JSON come in
		if err == nil {
			payload.Conn = *conn
			wsChan <- payload
		} else {
			break
		}
	}
}

// ListenToWsChannel listens to wsChan for payloads ListenForWsPayload sent
func ListenToWsChannel() {
	var response WsJsonResponse

	for e := range wsChan {
		switch e.Action {
		case "username":
			// get a list of all users and sent it back via broadcastToAll
			clients[e.Conn] = e.Username
			users := getUserList()
			response.Action = "list_users"
			response.ConnectedUsers = users
			broadcastToAll(response)

		case "user_left":
			delete(clients, e.Conn)
			users := getUserList()
			response.Action = "list_users"
			response.ConnectedUsers = users
			broadcastToAll(response)

		case "broadcast":
			response.Action = "broadcast"
			response.Message = fmt.Sprintf("<strong>%s</strong>: %s", e.Username, e.Message)
			broadcastToAll(response)
		}

	}
}

func getUserList() []string {
	var userList []string
	for _, username := range clients {
		if username != "" {
			userList = append(userList, username)
		}
	}

	sort.Strings(userList)
	return userList
}

// broadcastToAll send back response to every clients
func broadcastToAll(response WsJsonResponse) {
	for client := range clients {
		err := client.WriteJSON(response)
		// error means client dismissed
		if err != nil {
			log.Println("websocket error")
			_ = client.Close()
			delete(clients, client)
		}
	}
}

func renderPage(w http.ResponseWriter, tmpl string, data jet.VarMap) error {
	view, err := views.GetTemplate(tmpl)
	if err != nil {
		log.Println(err)
		return err
	}

	err = view.Execute(w, data, nil)
	if err != nil {
		log.Println(err)
		return err
	}

	return nil
}
