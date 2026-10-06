package main

import (
	"net/http"
	"os"
)

func main() {
	_ = os.Getenv("DATABASE_URL")
	_ = os.Getenv("JWT_SECRET")
	http.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
