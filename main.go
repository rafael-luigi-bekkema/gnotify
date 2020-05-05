package main

import (
	"container/list"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	serviceName       = "org.freedesktop.Notifications"
	path              = "/org/freedesktop/Notifications"
	defaultExpiration = 5000
)

const (
	CapabilityBody = "body"
)

type Notification struct {
	id                    int32
	sender, summary, body string
	icon                  string
	actions               []string
	hints                 map[string]interface{}
	expires               *time.Time
}

type Receiver struct {
	counter       int32
	notifications *list.List
	lock          sync.Mutex
}

func NewReceiver() *Receiver {
	return &Receiver{
		notifications: list.New(),
	}
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

	noti := Notification{
		id:      id,
		sender:  sender,
		summary: summary,
		body:    body,
		actions: actions,
		hints:   hints,
	}

	if expires == -1 {
		expires = defaultExpiration
	}

	if expires > 0 {
		t := time.Now().Add(time.Millisecond * time.Duration(expires))
		noti.expires = &t
	}

	r.lock.Lock()
	r.notifications.PushBack(&noti)
	r.lock.Unlock()

	log.Printf("Message received from %s, %s %s", sender, summary, body)
	return id, nil
}

func (r *Receiver) GetCapabilities() ([]string, *dbus.Error) {
	return []string{CapabilityBody}, nil
}

func (r *Receiver) CloseNotification(id int32) *dbus.Error {
	return nil
}

func (r *Receiver) outputter() {
	t := time.NewTicker(time.Second)
	for range t.C {
		var output strings.Builder
		r.lock.Lock()
		for item := r.notifications.Front(); item != nil; item = item.Next() {
			noti := item.Value.(*Notification)
			if noti.expires != nil && noti.expires.Before(time.Now()) {
				r.notifications.Remove(item)
				continue
			}
			output.WriteString(noti.summary)
			if noti.body != "" {
				output.WriteString(fmt.Sprintf(" // %s", noti.body))
			}
			output.WriteString(" :: ")

		}
		fmt.Printf("%s%s\n", output.String(), time.Now().Format(time.Stamp))
		r.lock.Unlock()
	}
}

func main() {
	//f, _ := os.OpenFile("/tmp/gnotifyd.log", os.O_CREATE|os.O_WRONLY, 0755)
	//defer f.Close()
	//log.SetOutput(f)

	conn, err := dbus.SessionBus()
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	receiver := NewReceiver()
	conn.Export(receiver, path, serviceName)

	reply, err := conn.RequestName(serviceName, dbus.NameFlagDoNotQueue)
	if err != nil {
		panic(err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		log.Println("name already taken")
		os.Exit(1)
	}
	signal := make(chan *dbus.Signal)
	conn.Signal(signal)

	go receiver.outputter()

	log.Println(fmt.Sprintf("Listening on %s / %s ...", serviceName, path))
	select {
	case s := <-signal:
		log.Println(s)
	}
}
