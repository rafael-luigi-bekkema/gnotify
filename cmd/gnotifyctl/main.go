package main

import (
	"bufio"
	"container/list"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/rafael-luigi-bekkema/gnotifyd/internal/shared"
)

type State struct {
	notifications *list.List
	lock          sync.Mutex
}

func NewState() *State {
	return &State{
		notifications: list.New(),
	}
}

func (s *State) reader(socketPath string) error {
	c, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("could not connect to socket: %w", err)
	}
	defer c.Close()

	reader := bufio.NewReader(c)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return fmt.Errorf("connection closed: %w", err)
		}
		var noti shared.Notification
		if err := json.Unmarshal(line, &noti); err != nil {
			log.Printf("could not decode: %s", err)
			continue
		}
		s.lock.Lock()
		s.notifications.PushFront(&noti)
		s.lock.Unlock()
	}
}

func (s *State) netLoop() {
	socketPath := shared.GetSocketPath()
	for {
		if err := s.reader(socketPath); err != nil {
			time.Sleep(time.Second * 5)
		}
	}
}

func main() {
	var s = NewState()
	go s.netLoop()

	t := time.NewTicker(time.Second)
	for range t.C {
		var output strings.Builder
		s.lock.Lock()
		for item := s.notifications.Front(); item != nil; item = item.Next() {
			noti := item.Value.(*shared.Notification)
			if noti.Expires != nil && noti.Expires.Before(time.Now()) {
				s.notifications.Remove(item)
				continue
			}
			output.WriteString(noti.Summary)
			if noti.Body != "" {
				output.WriteString(fmt.Sprintf(" // %s", noti.Body))
			}
			output.WriteString(" :: ")

		}
		s.lock.Unlock()

		data := fmt.Sprintf("%s%s\n", output.String(), time.Now().Format(time.Stamp))
		fmt.Print(data)
	}
}
