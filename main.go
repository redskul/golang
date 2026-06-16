package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"hana-viewer/web"
)

func main() {
	traceDir := flag.String("dir", "", "Path to HANA trace directory to scan (e.g. /usr/sap/HDB/HDB00/trace). If empty, runs in demo mode with sample data.")
	flag.Parse()

	srv, err := web.NewServer(*traceDir)
	if err != nil {
		log.Fatalf("failed to init server: %v", err)
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	addr := ":8080"
	fmt.Printf("HANA Diagnostic Viewer running at http://localhost%s\n", addr)
	fmt.Println("  Dashboard : http://localhost:8080/")
	fmt.Println("  Log Viewer: http://localhost:8080/logs")
	fmt.Println("  Crash #0  : http://localhost:8080/crash/0")
	fmt.Println("  OOM #0    : http://localhost:8080/oom/0")
	fmt.Println("  JSON API  : http://localhost:8080/api/report")

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
