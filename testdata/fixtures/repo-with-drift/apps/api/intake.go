package main

import "os"

func intakeToken() string {
	return os.Getenv("INTAKE_TOKEN")
}
