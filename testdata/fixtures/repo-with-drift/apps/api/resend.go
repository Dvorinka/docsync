package main

import "os"

func resendSecret() string {
	return os.Getenv("RESEND_WEBHOOK_SECRET")
}
