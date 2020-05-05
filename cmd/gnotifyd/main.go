package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/rafael-luigi-bekkema/gnotifyd/internal/shared"
)

const (
	serviceName       = "org.freedesktop.Notifications"
	servicePath       = "/org/freedesktop/Notifications"
	defaultExpiration = 5000
)

const (
	CapabilityBody = "body"
)

type Splitter struct {
	subscribers map[chan *shared.Notification]struct{}
	lock        sync.Mutex
}

func (s *Splitter) Subscribe() chan *shared.Notification {
	s.lock.Lock()
	defer s.lock.Unlock()

	c := make(chan *shared.Notification)
	if s.subscribers == nil {
		s.subscribers = make(map[chan *shared.Notification]struct{})
	}
	s.subscribers[c] = struct{}{}
	return c
}

func (s *Splitter) Unsubscribe(c chan *shared.Notification) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.subscribers, c)
}

func (s *Splitter) Send(n *shared.Notification) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for c := range s.subscribers {
		c <- n
	}
}

type Receiver struct {
	counter  int32
	splitter Splitter
}

func NewReceiver() *Receiver {
	return &Receiver{}
}

func (r *Receiver) GetServerInformation() (string, string, string, string, *dbus.Error) {
	return "gnotifyd notification daemon", "github.com/rafael-luigi-bekkema/gnotifyd", "0.1", "1.2", nil
}

func (r *Receiver) Notify(sender string, replaceID int32, icon, summary, body string, actions []string,
	hints map[string]interface{}, expires int) (int32, *dbus.Error) {
	id := replaceID
	if replaceID == 0 {
		id = atomic.AddInt32(&r.counter, 1)
	}

	noti := shared.Notification{
		ID:      id,
		Sender:  sender,
		Summary: summary,
		Body:    body,
		Actions: actions,
		Hints:   hints,
	}

	if expires == -1 {
		expires = defaultExpiration
	}

	if expires > 0 {
		t := time.Now().Add(time.Millisecond * time.Duration(expires))
		noti.Expires = &t
	}

	r.splitter.Send(&noti)
	return id, nil
}

func (r *Receiver) GetCapabilities() ([]string, *dbus.Error) {
	return []string{CapabilityBody}, nil
}

func (r *Receiver) CloseNotification(id int32) *dbus.Error {
	return nil
}

func (r *Receiver) outputter() {
	socketPath := shared.GetSocketPath()
	os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Could not create socket: %s", err)
	}
	defer l.Close()

	for {
		fd, err := l.Accept()
		if err != nil {
			log.Printf("Accept error: %s", err)
			continue
		}
		log.Printf("New connection: %v", fd)
		go func() {
			receiver := r.splitter.Subscribe()
			defer r.splitter.Unsubscribe(receiver)
			for n := range receiver {
				data, err := json.Marshal(n)
				if err != nil {
					log.Printf("could not encode: %s", err)
				}
				data = append(data, '\n')
				_, err = fd.Write(data)
				if err != nil {
					return
				}
			}
		}()
	}
}

func main() {
	//f, _ := os.OpenFile("/tmp/gnotifyd.log", os.O_CREATE|os.O_WRONLY, 0755)
	//defer f.Close()
	//log.SetOutput(f)

	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalf("Could not connect to D-BUS: %s", err)
	}
	defer conn.Close()

	receiver := NewReceiver()
	conn.Export(receiver, servicePath, serviceName)

	reply, err := conn.RequestName(serviceName, dbus.NameFlagDoNotQueue)
	if err != nil {
		log.Fatalf("could not request name: %s", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Fatal("name already taken")
	}
	signal := make(chan *dbus.Signal)
	conn.Signal(signal)

	go receiver.outputter()

	log.Println(fmt.Sprintf("Listening on %s / %s ...", serviceName, servicePath))
	select {
	case s := <-signal:
		log.Println(s)
	}
}
