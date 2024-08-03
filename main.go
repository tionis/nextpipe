package main

import (
	"context"
	_ "embed"
	"errors"
	"github.com/gorilla/mux"
	"github.com/hiddeco/sshsig"
	"github.com/urfave/cli/v2"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	app := &cli.App{
		Name:  "patchwork",
		Usage: "patchwork communication server",
		Commands: []*cli.Command{
			{
				Name:    "start",
				Aliases: []string{"s"},
				Usage:   "start the patchwork server",
				Action: func(c *cli.Context) error {
					startServer()
					return nil
				},
			},
			{
				Name:    "parseSSHSig",
				Aliases: []string{"p"},
				Usage:   "parse an SSH signature",
				Action: func(c *cli.Context) error {
					var sigBytes []byte
					if c.NArg() == 0 || c.Args().First() == "-" {
						// read from stdin
						sigBytes = make([]byte, 0)
						buf := make([]byte, 1024)
						for {
							n, err := os.Stdin.Read(buf)
							if err != nil {
								break
							}
							sigBytes = append(sigBytes, buf[:n]...)
						}
					} else {
						// read from file
						sigBytes = make([]byte, 0)
						file, err := os.Open(c.Args().First())
						if err != nil {
							log.Fatalf("Error opening file: %v", err)
						}
						defer func(file *os.File) {
							err := file.Close()
							if err != nil {
								log.Fatalf("Error closing file: %v", err)
							}
						}(file)
						buf := make([]byte, 1024)
						for {
							n, err := file.Read(buf)
							if err != nil {
								break
							}
							sigBytes = append(sigBytes, buf[:n]...)
						}
					}
					sig, err := sshsig.Unarmor(sigBytes)
					if err != nil {
						log.Fatalf("Error parsing SSH signature: %v", err)
					}
					log.Printf("Parsed SSH signature: %v", sig)
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func startServer() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	logLevel := slog.LevelInfo
	switch strings.ToUpper(os.Getenv("LOG_LEVEL")) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	}
	loggerOpts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: logLevel == slog.LevelDebug,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, loggerOpts))

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	srv := getHTTPServer(logger.WithGroup("http"))
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Error starting http server: %v", err)
		}
	}()

	stopLoop := false
	for !stopLoop {
		logger.Debug("Waiting for signal")
		sig := <-c
		switch sig {
		case os.Interrupt, os.Kill:
			logger.Info("Shutting down Patchwork")
			err := srv.Shutdown(ctx)
			if err != nil {
				logger.Error("Error shutting down http server: %v", err)
			}
			logger.Info("Stopped http server")
			stopLoop = true
		default:
			logger.Info("Received unknown signal: %v", sig)
		}
	}

	//wg.Wait()
	logger.Info("Starting shutdown of remaining contexts")
	<-ctx.Done()
	logger.Info("Patchwork stopped")
}

func getHTTPServer(logger *slog.Logger) *http.Server {
	server := &server{
		logger:             logger,
		channels:           make(map[string]*patchChannel),
		channelsMutex:      sync.RWMutex{},
		githubUserKeyMap:   make(map[string]sshPubKeyListEntry),
		githubUserKeyMutex: sync.RWMutex{},
	}

	router := mux.NewRouter()

	router.HandleFunc("/.well-known", server.wellKnownHandler)
	router.HandleFunc("/.well-known/{path:.*}", server.wellKnownHandler)
	router.HandleFunc("/", server.indexHandler)
	router.HandleFunc("/water.css", server.waterHandler)
	router.HandleFunc("/favicon.ico", server.faviconHandler)
	router.HandleFunc("/patchwork.sh", server.patchworkShHandler)
	router.HandleFunc("/robots.txt", server.robotsHandler)

	router.HandleFunc("/huproxy/{host}/{port}", server.huproxyHandler)
	router.HandleFunc("/p/{path:.*}", server.publicHandler)
	router.HandleFunc("/u/{username}/{path:.*}", server.userHandler)
	router.HandleFunc("/w/{pubkey}/{path:.*}", server.webCryptoHandler)
	router.HandleFunc("/k/{pubkey}/{path:.*}", server.keyHandler)
	router.HandleFunc("/g/{gistId}/{path:.*}", server.gistHandler)

	router.HandleFunc("/healthz", server.statusHandler)
	router.HandleFunc("/status", server.statusHandler)

	http.Handle("/", router)

	logger.Info("Starting Patchwork on :8080")
	return &http.Server{
		Addr: ":8080",
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		Handler:      router,
	}
}
