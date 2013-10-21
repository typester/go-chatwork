package chatwork

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Chatwork struct {
	username, password string
	client             *http.Client

	chatworkId string
	lastId     string
	token      string

	people map[string]*Person
	rooms  map[string]*Room
}

type Person struct {
	Id           int64  `json:"aid"`
	CwId         string `json:"cwid"`
	Name         string `json:"name"`
	Organization string `json:"onm"`
}

type Room struct {
	Name       string           `json:"n"`
	Type       int64            `json:"t"`
	LastUpdate int64            `json:"lt"`
	ReadNum    int64            `json:"r"`
	ChatNum    int64            `json:"c"`
	Member     map[string]int64 `json:"m"`
}

type Chat struct {
	Id      int64
	Message string
	Person  *Person
	Room    *Room
	Time    time.Time
}

type StatusResponser interface {
	Success() bool
}

type CommonResponse struct {
	StatusResponser
	Status Status `json:"status"`
}

func (c *CommonResponse) Success() bool {
	return c.Status.Success
}

type Status struct {
	Success bool `json:"success"`
}

func New(username, password string) (*Chatwork, error) {
	cw := &Chatwork{
		username: username,
		password: password,
		client:   &http.Client{},

		people: map[string]*Person{},
		rooms:  map[string]*Room{},
	}

	jar, err := cookiejar.New(&cookiejar.Options{})
	if err != nil {
		return nil, err
	}

	cw.client.Jar = jar

	return cw, nil
}

func cmd(cmd string) string {
	return "https://www.chatwork.com/gateway.php?_v=2.52&_av=4&cmd=" + cmd
}

func (cw *Chatwork) post(command string, param interface{}, res StatusResponser) (err error) {
	data, err := json.Marshal(param)
	if err != nil {
		return
	}

	v := url.Values{"pdata": []string{string(data)}}
	if len(cw.token) > 0 {
		v["_t"] = []string{cw.token}
	}

	req, err := http.NewRequest("POST", cmd(command), strings.NewReader(v.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	//req.Header.Set("User-Agent", "Robson ChatWork Mobile/undefined iosv7.0.2 (iPhone App iPhone6,1)")
	//req.Header.Set("X-Requested-With", "XMLHttpRequest")

	//r, err := cw.client.PostForm(cmd(command), v)
	r, err := cw.client.Do(req)
	if err != nil {
		return
	}

	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return
	}
	r.Body.Close()

	if len(os.Getenv("DEBUG")) > 0 {
		fmt.Printf("%s\n", command)
		fmt.Println(string(content))
	}

	err = json.Unmarshal(content, &res)
	if err != nil {
		return
	}

	if !res.Success() {
		err = fmt.Errorf("response status is fail: %s\n", res)
		return
	}

	return
}

func (cw *Chatwork) Login() error {
	type LoginData struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		Type        string `json:"type"`
		WithProfile int    `json:"with_profile"`
	}

	type Result struct {
		Token      string            `json:"token"`
		MyId       string            `json:"myid"`
		Rooms      map[string]Room   `json:"room_dat"`
		People     map[string]Person `json:"contact_dat"`
		AnnounceId int64             `json:"announce_id"`
		LastId     string            `json:"last_id"`
	}

	type LoginResponse struct {
		CommonResponse
		Result Result
	}

	var res LoginResponse
	err := cw.post("api_login", &LoginData{cw.username, cw.password, "mobile", 1}, &res)
	if err != nil {
		return err
	}

	cw.lastId = res.Result.LastId
	cw.token = res.Result.Token

	for id, person := range res.Result.People {
		cw.people[id] = &Person{}
		*(cw.people[id]) = person
	}
	for id, room := range res.Result.Rooms {
		cw.rooms[id] = &Room{}
		*(cw.rooms[id]) = room
	}

	return nil
}

func (cw *Chatwork) GetUpdate() ([]*Chat, error) {
	type GetUpdateData struct {
		LastId string `json:"last_id"`
	}

	type UpdateRoom struct {
		P  int64 `json:"p"`
		Ld int64 `json:"ld"`
		I  int64 `json:"i"`
	}

	type UpdateInfo struct {
		Num int64 `json:"num"`
		//Rooms map[string]UpdateRoom `json:"room"`
		Rooms interface{} `json:"room"`
	}

	type GetUpdateResult struct {
		LastId     string     `json:"last_id"`
		UpdateInfo UpdateInfo `json:"update_info"`
	}

	type GetUpdateResponse struct {
		CommonResponse
		Result GetUpdateResult `json:"result"`
	}

	var res GetUpdateResponse
	err := cw.post("get_update", &GetUpdateData{cw.lastId}, &res)
	if err != nil {
		return nil, err
	}

	cw.lastId = res.Result.LastId

	updatedRoom := map[string]*Room{}
	if rooms, ok := res.Result.UpdateInfo.Rooms.(map[string]interface{}); ok {
		for id, _ := range rooms {
			room, ok := cw.rooms[id]
			if ok {
				updatedRoom[id] = room
			}
		}
	}

	var updates []*Chat
	for id, room := range updatedRoom {
		type UnknownParam struct {
			C int64 `json:"c"`
			U int64 `json:"u"`
			T int64 `json:"t"`
			L int64 `json:"l"`
		}

		type RoomInfoRequest struct {
			I map[string]UnknownParam `json:"i"`
		}

		p := map[string]UnknownParam{}
		p[id] = UnknownParam{
			C: room.ChatNum,
			U: 20,
			T: room.LastUpdate,
			L: 0,
		}

		type ChatRaw struct {
			Id  int64  `json:"id"`
			Aid int64  `json:"aid"`
			Msg string `json:"msg"`
			Tm  int64  `json:"tm"`
			Utm int64  `json:"utm"`
		}

		type RoomInfo struct {
			Room
			Chats []ChatRaw `json:"chat_list"`
		}

		type RoomInfoResult struct {
			Rooms map[string]RoomInfo `json:"room_dat"`
		}

		type RoomInfoResponse struct {
			CommonResponse
			Result RoomInfoResult
		}

		var roomInfo RoomInfoResponse
		err := cw.post("get_room_info", &RoomInfoRequest{I: p}, &roomInfo)

		if err != nil {
			return nil, err
		}

		info, ok := roomInfo.Result.Rooms[id]
		if ok {
			var lastUpdate int64 = 0

			for i := range info.Chats {
				chat := info.Chats[i]

				if chat.Tm < room.LastUpdate {
					continue
				}

				epoch := time.Now().Unix()
				if epoch-60 > chat.Tm {
					continue
				}

				pid := strconv.Itoa(int(chat.Aid))
				person, okp := cw.people[pid]
				if !okp {

				}

				u := &Chat{
					Id:      chat.Id,
					Message: chat.Msg,
					Room:    room,
					Person:  person,
					Time:    time.Unix(chat.Tm, 0),
				}
				updates = append(updates, u)

				if chat.Tm > lastUpdate {
					lastUpdate = chat.Tm
				}
			}

			if lastUpdate > room.LastUpdate {
				room.LastUpdate = lastUpdate
			}
		}
	}

	return updates, nil
}
