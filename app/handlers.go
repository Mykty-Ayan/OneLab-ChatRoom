package app

import (
	"ZakirAvrora/ChatRoom/internals/models"
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

var ErrRoomNoExist = errors.New("room does not exist")

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func (app *Application) homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.InfoLog.Printf("wrong url path %v from %v ", r.URL.Path, r.RemoteAddr)
		app.notFound(w)
		return
	}

	if r.Method == http.MethodGet {
		if err := TemplateParseAndExecute(w, "public/home.html", app.Server.Rooms); err != nil {
			app.ErrorLog.Println(err.Error())
			app.serverError(w, err)
		}
	} else if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			app.serverError(w, err)
			return
		}

		roomName := strings.TrimSpace(r.Form["name"][0])
		cap := r.Form["capacity"][0]
		capInt, err := strconv.Atoi(cap)
		if err != nil || capInt < 1 || roomName == "" {
			app.badRequest(w)
			return
		}

		app.Server.Mu.RLock()
		if _, ok := app.Server.Rooms[roomName]; ok {
			app.InfoLog.Println("Room already exist")
			http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
			return
		}
		app.Server.Mu.RUnlock()

		app.Server.CreateNewRoom(roomName, capInt)
		http.Redirect(w, r, r.URL.Path, http.StatusSeeOther)
	} else {
		app.InfoLog.Printf("wrong method %v from %v: ", r.Method, r.RemoteAddr)
		app.methodNotAllowed(w)
	}
}

func (app *Application) roomHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/room" {
		app.InfoLog.Printf("wrong url path %v from %v ", r.URL.Path, r.RemoteAddr)
		app.notFound(w)
		return
	}

	if r.Method != http.MethodGet {
		app.InfoLog.Printf("wrong method %v from %v: ", r.Method, r.RemoteAddr)
		app.methodNotAllowed(w)
		return
	}

	if err := TemplateParseAndExecute(w, "public/index.html", nil); err != nil {
		app.ErrorLog.Println(err.Error())
		app.serverError(w, err)
	}
}
func (app *Application) wsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ws" {
		app.InfoLog.Printf("wrong url path %v from %v ", r.URL.Path, r.RemoteAddr)
		app.notFound(w)
		return
	}

	if r.Method != http.MethodGet {
		app.InfoLog.Printf("wrong method %v from %v: ", r.Method, r.RemoteAddr)
		app.methodNotAllowed(w)
		return
	}

	if err := r.ParseForm(); err != nil {
		app.serverError(w, err)
		return
	}

	name := GetNick(r)
	room, err := GetRoom(app, r)
	if err != nil {
		if errors.Is(err, ErrRoomNoExist) {
			app.badRequest(w)
		} else {
			app.serverError(w, err)
		}
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.serverError(w, err)
		return
	}
	client := models.NewClient("Anonymous", conn, app.Server.Rooms["general"])
	app.Server.Rooms["general"].Register <- client

	client := models.NewClient(name, conn, room)

	go client.WritePump()
	go client.ReadPump()
	client.Room.Broadcast <- models.Message{From: client, Msg: []byte(models.MsgUserIn(client))}
	client.Room.Register <- client
}

func TemplateParseAndExecute(w http.ResponseWriter, path string, data interface{}) error {
	tmpl, err := template.ParseFiles(path)
	if err != nil {
		return err
	}

	if err := tmpl.Execute(w, data); err != nil {
		return err
	}
	return nil
}

func GetNick(r *http.Request) string {
	nick := r.Form["nick"]
	var name string

	if len(nick) == 0 || strings.TrimSpace(nick[0]) == "" {
		name = "Anonymous"
	} else {
		name = nick[0]
	}
	return name
}

func GetRoom(app *Application, r *http.Request) (*models.ChatRoom, error) {
	roomName := r.Form["room"]
	var room *models.ChatRoom

	if len(roomName) == 0 || strings.TrimSpace(roomName[0]) == "" {
		room = app.Server.Rooms["general"]
	} else {
		app.Server.Mu.RLock()
		r, ok := app.Server.Rooms[roomName[0]]
		app.Server.Mu.RUnlock()
		if ok {
			room = r
		} else {
			return nil, ErrRoomNoExist
		}

	}

	return room, nil
}
