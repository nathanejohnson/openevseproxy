package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var upstream string
var listenAddress string
var length304 int
var bodyText304 string

func main() {
	fs := flag.NewFlagSet("mongooseproxy", flag.ExitOnError)
	fs.StringVar(&upstream, "upstream", "http://openevse/", "upstream address")
	fs.StringVar(&listenAddress, "listen", ":8080", "listen address")
	fs.IntVar(&length304, "length-304", 0, "Content-Length value for 304 responses")
	fs.StringVar(&bodyText304, "body-text-304", "", "body text to send on 304")
	fs.Parse(os.Args[1:])
	mp, err := NewMongooseProxy(nil, upstream, bodyText304, length304)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s := &http.Server{
		Addr:    listenAddress,
		Handler: mp,
	}
	go func() {
		err := s.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
		stop()
	}()
	<-ctx.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
