package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/typester/go-chatwork"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

var addr = flag.String("addr", "127.0.0.1:6667", "address:port, default: 127.0.0.1:6667")

type IRCMsg struct {
	Prefix  string
	Command string
	Args    []string
}

func fixch(ch string) string {
	if len(ch) == 0 {
		return "mychat"
	} else {
		return strings.Replace(ch, " ", "_", -1)
	}
}

func parseMsg(line string) (*IRCMsg, error) {
	msg := &IRCMsg{}
	msg.Args = make([]string, 0)

	tokens := strings.SplitN(line, " ", 15)
	if len(tokens) < 1 {
		return nil, fmt.Errorf("invalid irc message: %s\n", line)
	}

	if strings.HasPrefix(tokens[0], ":") {
		if len(tokens) < 2 {
			return nil, fmt.Errorf("invalid irc message: %s\n", line)
		}
		msg.Prefix = strings.Replace(tokens[0], ":", "", 1)
		msg.Command = tokens[1]
		tokens = tokens[2:]
	} else {
		if len(tokens) < 1 {
			return nil, fmt.Errorf("invalid irc message: %s\n", line)
		}
		msg.Prefix = ""
		msg.Command = tokens[0]
		tokens = tokens[1:]
	}

	for i := range tokens {
		token := tokens[i]
		if strings.HasPrefix(token, ":") {
			tokens[i] = strings.Replace(tokens[i], ":", "", 1)
			msg.Args = append(msg.Args, strings.Join(tokens[i:], " "))
			break
		} else {
			msg.Args = append(msg.Args, token)
		}
	}

	return msg, nil
}

func Handle(conn net.Conn) {
	nick := ""
	user := ""
	pass := ""
	var client *chatwork.Chatwork

	buf := bufio.NewReader(conn)

	done := make(chan bool)
	defer func() {
		conn.Close()
		done <- true
	}()

	for {
		line, _, err := buf.ReadLine()
		if err != nil {
			log.Printf("Read Error: %s\n", err)
			fmt.Fprintf(conn, "ERROR :%s\n", err.Error())
			return
		}

		msg, err := parseMsg(string(line))
		if err != nil {
			log.Printf("Error: %s\n", err)
			fmt.Fprintf(conn, "ERROR :%s\n", err.Error())
			return
		}

		if msg.Command != "PASS" {
			log.Printf("msg:%+v, args:%d\n", msg, len(msg.Args))
		}

		switch msg.Command {
		case "PASS":
			pass = msg.Args[0]
		case "NICK":
			nick = msg.Args[0]
		case "USER":
			user = msg.Args[0]

			// start chatwork
			client, err = chatwork.New(user, pass)
			if err != nil {
				log.Printf("Error: %s\n", err)
				fmt.Fprintf(conn, "ERROR :%s\n", err.Error())
				return
			}

			go func() {
				if err := client.Login(); err != nil {
					log.Printf("login failed: %s\n", err)
					fmt.Fprintf(conn, "ERROR :%s\n", err.Error())
					conn.Close()
					return
				}

				fmt.Fprintf(conn, "001 %s :Welcome to the Internet Relay Network\n", nick)
				fmt.Fprintf(conn, "376 %s :End of MOTD\n", nick)

				for _, room := range client.Rooms() {
					ch := fixch(room.Name)
					log.Printf("send join: #%s\n", ch)
					fmt.Fprintf(conn, ":%s JOIN :#%s\n", nick, ch)
					fmt.Fprintf(conn, "331 %s #%s :No topic is set\n", nick, ch)
					fmt.Fprintf(conn, "353 %s = #%s :%s\n", nick, ch, nick)
					fmt.Fprintf(conn, "366 %s #%s :End of NAMES list\n", nick, ch)
				}

				for {
					select {
					case <-done:
						return
					default:
						time.Sleep(time.Second * 5)
					}

					updates, err := client.GetUpdate()
					if err != nil {
						log.Printf("Error: %s\n", err)
						fmt.Fprintf(conn, "ERROR :%s\n", err.Error())
						conn.Close()
						return
					}

					for i := range updates {
						chat := updates[i]
						fmt.Println(chat)
						n := fixch(chat.Person.Name)
						c := fixch(chat.Room.Name)
						msgs := strings.Split(chat.Message, "\n")

						for j := range msgs {
							fmt.Fprintf(conn, ":%s PRIVMSG #%s :%s\n", n, c, msgs[j])
						}
					}
				}
			}()

		case "PRIVMSG":
			ch := strings.Replace(msg.Args[0], "#", "", 1)
			msg := msg.Args[1]

			go func() {
				for id, room := range client.Rooms() {
					fmt.Printf("search room: %s, %s", fixch(room.Name), ch)

					if fixch(room.Name) == ch {
						rid, err := strconv.Atoi(id)
						if err != nil {
							log.Printf("Error: %s\n", err)
							return
						}

						err = client.SendChat(int64(rid), msg)
						if err != nil {
							log.Printf("Error: %s\n", err)
							fmt.Fprintf(conn, "ERROR :%s\n", err.Error())
							return
						}
						break
					}
				}

			}()

		default:
			log.Printf("unknown cmd: %s\n", msg.Command)
		}
	}
}

func main() {
	flag.Parse()

	l, err := net.Listen("tcp", *addr)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}

		go Handle(conn)
	}
}
