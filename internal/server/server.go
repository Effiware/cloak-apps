package server

import (
	"fmt"
	"log"
	"net/http"
	"time"

	_ "github.com/effiware/cloak-apps/internal/docs"
)

type HdaAndApi struct{}

func HttpServer(host string, port int, timeout int) *http.Server {
	hdaAndApi := &HdaAndApi{}
	readTimeout, writeTimout, idleTimeout := time.Duration(timeout), time.Duration(3*timeout), time.Duration(6*timeout)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", host, port),
		Handler:      hdaAndApi.RegisterRoutes(),
		IdleTimeout:  idleTimeout * time.Second,
		ReadTimeout:  readTimeout * time.Second,
		WriteTimeout: writeTimout * time.Second,
	}

	//server.RegisterOnShutdown(onServerShutdown)
	return server
}

func onServerShutdown() {
	// Add graceful shutdown logic if needed
	log.Println("Server shutdown complete")
}
