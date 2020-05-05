package shared

import (
	"os"
	"path"
	"time"
)

func GetSocketPath() string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	} else {
		cacheDir = path.Join(cacheDir, "gnotifyd")
		os.MkdirAll(cacheDir, 0755)
	}
	socketPath := path.Join(cacheDir, "gnotifyd.sock")
	return socketPath
}

type Notification struct {
	ID                    int32
	Sender, Summary, Body string
	Icon                  string
	Actions               []string
	Hints                 map[string]interface{}
	Expires               *time.Time
}
