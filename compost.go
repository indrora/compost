package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"sync"

	"github.com/indrora/compost/config"
	"github.com/lrstanley/girc"
)

var evmutex sync.Mutex

func init() {
	evmutex = sync.Mutex{}
}

func main() {

	// Load the configuration file

	slog.SetLogLoggerLevel(slog.LevelDebug)
	slog.Info("Starting compost")

	clientConfig := new(config.Config)

	config_search_paths := []string{
		"$HOME/.config/compost.toml",
		"compost.toml",
	}

	for _, path := range config_search_paths {
		file, err := os.Open(path)
		slog.Debug("config path", "path", path)

		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil {
			defer file.Close()
			clientConfig, err = config.LoadConfig(file)
			if err != nil {
				slog.Info("Failed to load config file", "path", path, "error", err)
				continue
			} else {
				slog.Debug("Loaded config file", "path", path)
				break
			}
		}

		fmt.Println("Unable to find a valid configuration file.")
		os.Exit(1)
	}

	fmt.Printf("nick = %s, realname = %s\n", clientConfig.Nick, clientConfig.Realname)
	for idx, server := range clientConfig.Servers {
		fmt.Printf("%v: name = %s, host = %s, port = %d, tls = %t, join = %s\n", idx, server.Name, server.Host, server.Port, server.UseTls, server.Autojoin)
	}

	serverindex := -1
	n, err := fmt.Scanf("%d", &serverindex)

	if n != 1 || err != nil || serverindex > len(clientConfig.Servers)-1 {
		fmt.Printf("Invalid server index %v\n", serverindex)
		return
	}

	mServer := clientConfig.Servers[serverindex]

	fmt.Println("Starting compost...")

	client := girc.New(girc.Config{
		Server: mServer.Host,
		Port:   mServer.Port,
		Nick:   clientConfig.Nick,
		User:   clientConfig.Username,
		Name:   clientConfig.Realname,
	})

	// set up the channel for comms

	EventChan := make(chan girc.Event)

	client.Handlers.AddBg(girc.ALL_EVENTS, func(c *girc.Client, e girc.Event) {
		//slog.Debug(e.Command, "params", e.Params, "echo", e.Echo)
	})

	client.Handlers.Add(girc.CONNECTED, func(c *girc.Client, e girc.Event) {
		fmt.Printf("JOINing: %v\n", mServer.Autojoin)
		c.Cmd.Join(mServer.Autojoin...)
	})

	client.Handlers.Add(girc.NOTICE, func(c *girc.Client, e girc.Event) {
		fmt.Printf("<%s> %s\n", e.Source.Name, e.Params[1])
	})

	client.Handlers.AddBg(girc.PRIVMSG, func(c *girc.Client, e girc.Event) {
		EventChan <- e
	})

	slog.Info("Connecting to server", "host", mServer.Host)
	go client.Connect()

	// Handle interrupt signal gracefully
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("Received interrupt, shutting down...")
		client.Close()
		os.Exit(0)
	}()

	// Start the I/O task

	inputchan := make(chan string)
	go iotask(os.Stdin, inputchan)

	clientlog := slog.With("component", "clientloop")

	for {
		select {
		case msg := <-EventChan:
			evmutex.Lock()
			clientlog.Debug("Took mutex")
			fmt.Printf("<%s> %s\n", msg.Source.Name, msg.Params[1])
			evmutex.Unlock()
			clientlog.Debug("released mutex")

		case input := <-inputchan:
			//	client.Cmd.Privmsg(event.Source.Name, input)
			fmt.Printf("INPUT=%s\n", input)
		default:
			//			clientlog.Debug("waiting for input...")
		}

	}
}

func iotask(r io.Reader, c chan string) {

	iobuffBytes := make([]byte, 1024)
	iobuff := bytes.NewBuffer(iobuffBytes)
	iobuffdup := io.TeeReader(r, iobuff)

	rReader := bufio.NewReader(iobuff)
	rScanner := bufio.NewScanner(iobuffdup)
	// clear the buffer of anything.

	logger := slog.With("component", "iotask")
	for {
		//logger.Debug("peeking")
		//		logger.Debug("scanner.Buffered()", "buffered", scanner.Buffered())
		if rReader.Buffered() < 1 {
			//			logger.Debug("no buffered data")
			continue
		} else {
			logger.Debug("scanner.Buffered()", "buffered", rReader.Buffered())
			b, err := rReader.Peek(1)
			if err != nil {
				panic(err)
			}
			if b[0] == '\n' || b[0] == '\r' {
				// skip the newline
				rReader.ReadByte()
				logger.Debug("skipping newline")
				continue
			}

			logger.Debug("peeked", "b", b)
			// read the line
			logger.Debug("taking mutex lock")
			evmutex.Lock()
			logger.Debug("got mutex lock")

			if rScanner.Scan() {
				line := rScanner.Text()
				logger.Debug("scanned line", "line", line)
				c <- line
			}

			evmutex.Unlock()
		}
	}
}
