// Command verify-update-signature verifies one Tauri/Minisign signature with
// the exact public key that will be embedded in official clients.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"bb_erp_echo/internal/update"
)

func main() {
	publicKey := flag.String("public-key", "", "canonical Tauri updater public key envelope")
	file := flag.String("file", "", "file whose signature must be verified")
	signature := flag.String("signature", "", "base64 Tauri/Minisign signature envelope")
	flag.Parse()

	if strings.TrimSpace(*publicKey) == "" || strings.TrimSpace(*file) == "" || strings.TrimSpace(*signature) == "" {
		fmt.Fprintln(os.Stderr, "public-key, file and signature are required")
		os.Exit(2)
	}
	verifier, err := update.NewMinisignVerifier(*publicKey)
	if err == nil {
		err = verifier.VerifyFile(*file, *signature)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "update signature verification failed:", err)
		os.Exit(1)
	}
}
