package main

import "os"

func main() {
	_ = os.Getenv("DATABASE_URL")
}
