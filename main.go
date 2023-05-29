package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
)

const (
	defaultServer = "localhost:4040"
	defaultFile   = ""
	usageServer   = "Server address to bind to. Ex: localhost:4040"
	usageFile     = "File or folder to upload. Ex: /tmp/filename.pdf OR /tmp"
	usageMonitor  = "Keep monitoring the Path folder for changes. Ex: true"
)

func main() {
	var path, serverAddr string
	var monitor bool

	cmd := flag.NewFlagSet("tcpClient", flag.ExitOnError)
	cmd.StringVar(&serverAddr, "server", defaultServer, usageServer)
	cmd.StringVar(&path, "path", defaultFile, usageFile)
	cmd.BoolVar(&monitor, "monitor", false, usageMonitor)

	err := cmd.Parse(os.Args[1:])
	checkError(err)

	upload(path, serverAddr)

	if monitor {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatal("Monitoring folder error: ", err)
		}

		defer func(watcher *fsnotify.Watcher) {
			_ = watcher.Close()
		}(watcher)

		done := make(chan bool)
		go func(srv string) {
			defer close(done)

			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						return
					}
					if event.Op == fsnotify.Create {
						go upload(event.Name, srv)
					}
				case err, ok := <-watcher.Errors:
					if !ok {
						return
					}
					log.Println("error:", err)
				}
			}
		}(serverAddr)

		err = watcher.Add(path)
		if err != nil {
			log.Fatal(" -------------------------------------------------------------------- Add failed:", err)
		}
		<-done
	}
}

func upload(path, serverAddr string) {
	fInfo, err := os.Stat(path)
	checkError(err)

	if fInfo.IsDir() {
		uploadFolder(path, "", serverAddr)
	} else {
		uploadFile(filepath.Dir(path), fInfo.Name(), serverAddr)
	}
}

func uploadFolder(path, name, serverAddr string) {
	fullPath := path
	if len(name) > 0 {
		fullPath = fmt.Sprintf("%s/%s", path, name)
	}
	path = strings.TrimRight(path, "/")
	di, err := os.ReadDir(fullPath)
	checkError(err)

	if len(di) == 0 {
		fmt.Printf("[client] No files in %s \n", fullPath)
		return
	}

	for _, item := range di {
		itemName := item.Name()
		if len(name) > 0 {
			itemName = fmt.Sprintf("%s/%s", name, item.Name())
		}
		if item.IsDir() {
			go uploadFolder(path, itemName, serverAddr)
			continue
		}
		uploadFile(path, itemName, serverAddr)
	}
}

func uploadFile(folder, srcFile, serverAddr string) {
	// connect to server
	conn, err := net.Dial("tcp", serverAddr)
	checkError(err)
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			checkError(err)
		}
	}(conn)

	fullPath := fmt.Sprintf("%s/%s", folder, srcFile)

	fmt.Println(fullPath)

	//try to open file to make sure it exists and is readable
	fi, err := os.Open(fullPath)
	checkError(err)
	defer func(fi *os.File) {
		err := fi.Close()
		if err != nil {
			checkError(err)
		}
	}(fi)

	onlyName := cleanName(srcFile)
	bName := []byte(onlyName)

	// write length of filename
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint16(bs, uint16(len(bName)))
	_ = binary.Write(conn, binary.LittleEndian, bs)

	// write filename
	_, err = conn.Write(bName)
	checkError(err)

	// write file content
	_, err = io.Copy(conn, fi)
	checkError(err)
}

func checkError(err error) {
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func cleanName(fName string) string {
	return strings.ReplaceAll(fName, " ", "_")
}
