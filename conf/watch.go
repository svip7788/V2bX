package conf

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

func (p *Conf) Watch(filePath string, reload func(), watchPaths ...string) error {
	watchedNames := make(map[string]struct{}, len(watchPaths))
	uniqueWatchPaths := make(map[string]struct{}, len(watchPaths))
	for _, watchPath := range watchPaths {
		if watchPath == "" {
			continue
		}
		uniqueWatchPaths[watchPath] = struct{}{}
		watchedNames[filepath.Base(watchPath)] = struct{}{}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("new watcher error: %s", err)
	}
	go func() {
		var pre time.Time
		defer watcher.Close()
		for {
			select {
			case e := <-watcher.Events:
				if e.Has(fsnotify.Chmod) {
					continue
				}
				if pre.Add(10 * time.Second).After(time.Now()) {
					continue
				}
				pre = time.Now()
				go func() {
					time.Sleep(5 * time.Second)
					name := filepath.Base(strings.TrimSuffix(e.Name, "~"))
					if _, ok := watchedNames[name]; ok {
						log.Printf("watched file %s changed, reloading...", name)
					} else {
						log.Println("config file changed, reloading...")
					}
					next := New()
					err := next.LoadFromPath(filePath)
					if err != nil {
						log.Printf("reload config error: %s", err)
						return
					}
					*p = *next
					reload()
					log.Println("reload config success")
				}()
			case err := <-watcher.Errors:
				if err != nil {
					log.Printf("File watcher error: %s", err)
				}
			}
		}
	}()
	err = watcher.Add(filePath)
	if err != nil {
		return fmt.Errorf("watch file error: %s", err)
	}
	for watchPath := range uniqueWatchPaths {
		err = watcher.Add(watchPath)
		if err != nil {
			return fmt.Errorf("watch extra file error (%s): %s", watchPath, err)
		}
	}
	return nil
}
