package main

import (
	"bufio"
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

	//	clientlog := slog.With("component", "clientloop")

	for {

		select {
		case input := <-inputchan:
			//	client.Cmd.Privmsg(event.Source.Name, input)
			fmt.Printf("INPUT=%s\n", input)
		default:

			//			clientlog.Debug("Took mutex")
			for msg := range EventChan {
				evmutex.Lock()
				fmt.Printf("<%s> %s\n", msg.Source.Name, msg.Params[1])
				evmutex.Unlock()
			}
			evmutex.Unlock()
			//			clientlog.Debug("released mutex")

		}

	}
}

func iotask(r io.Reader, c chan string) {

	scanner := bufio.NewReader(r)

	for {
		b, e := scanner.Peek(1)
		if errors.Is(e, io.EOF) {
			fmt.Println("got EOF")
			continue
		} else if b[0] == '\n' || b[0] == '\r' {
			for k, ex := scanner.Peek(1); k[0] < 20; k, ex = scanner.Peek(1) {
				if ex != nil {
					panic(ex)
				}
				fmt.Printf("Skipping '%d'\n", b[0])
				bb, _ := scanner.ReadByte()
				fmt.Printf("Skipped '%d'\n", bb)
			}
			fmt.Println("HELLO")
			slog.Debug("reading...")
			evmutex.Lock()
			fmt.Print("? ")

			s, e := scanner.ReadString('\n')
			if e == nil {
				go func() {
					slog.Debug("pump to channel", "line", s)
					c <- s
				}()
				slog.Debug("...")
			} else {
				panic(e)
			}
			evmutex.Unlock()
			slog.Debug("Unlocked.")
		} else {
			_, _ = scanner.ReadByte()
		}
	}

}
