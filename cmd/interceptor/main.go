package main

import (
	"github.com/despreston/interceptor/server"
	"log"
)

func main() {
	s := server.New("0.0.0.0", 3000)

	if err := s.Start(); err != nil {
		log.Fatalf("ERROR: Failed to start server. %v", err.Error())
	}
}
