package main

import (
	"gunship2000-assets/game/replay"
	"log"
	"os"
)

func main() {
	s := replay.Store{Root: os.Args[1]}
	if e := s.MigrateCompanions(); e != nil {
		log.Fatal(e)
	}
}
