// sshapp is an SSH server that executes a configurable command whenever a user
// connects. Built with charmbracelet/wish v2. Useful for turning any CLI tool
// into an SSH-accessible app.
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"

	"charm.land/wish/v2"
	"github.com/charmbracelet/ssh"
	"github.com/creack/pty"
)

func main() {
	host := flag.String("host", "0.0.0.0", "SSH server host")
	port := flag.Int("port", 2222, "SSH server port")
	command := flag.String("cmd", "", "Command to execute on SSH connect")
	hostKeyPath := flag.String("host-key", "", "Path to SSH host key (generated if not exists)")
	flag.Parse()

	if *command == "" {
		log.Fatal("--cmd is required")
	}

	// Default host key location if not specified.
	if *hostKeyPath == "" {
		*hostKeyPath = filepath.Join(os.TempDir(), "sshapp_host_key")
	}
	if err := ensureHostKey(*hostKeyPath); err != nil {
		log.Fatalf("host key: %v", err)
	}

	srv, err := wish.NewServer(
		wish.WithAddress(fmt.Sprintf("%s:%d", *host, *port)),
		wish.WithHostKeyPath(*hostKeyPath),
		// Intercept every SSH session and run --cmd instead of a shell.
		wish.WithMiddleware(func(next ssh.Handler) ssh.Handler {
			return func(s ssh.Session) {
				cmd := exec.CommandContext(s.Context(), "sh", "-c", *command)
				cmd.Env = os.Environ()

				// Check if the SSH client requested a PTY (most do by default).
				ptyReq, winCh, isPty := s.Pty()
				if isPty {
					// Propagate the client's TERM so TUI apps work correctly.
					cmd.Env = append(cmd.Env, "TERM="+ptyReq.Term)

					// Allocate a real PTY for the child process — required by
					// terminal apps like htop, btop, vim, etc.
					f, err := pty.StartWithSize(cmd, &pty.Winsize{
						Rows: uint16(ptyReq.Window.Height),
						Cols: uint16(ptyReq.Window.Width),
					})
					if err != nil {
						fmt.Fprintf(s.Stderr(), "Error: %v\n", err)
						s.Exit(1)
						return
					}
					defer f.Close()

					// Forward terminal resize events from the SSH client to
					// the PTY so the app's layout adjusts live.
					go func() {
						for win := range winCh {
							pty.Setsize(f, &pty.Winsize{
								Rows: uint16(win.Height),
								Cols: uint16(win.Width),
							})
						}
					}()

					// Bidirectional I/O: SSH stdin → PTY, PTY stdout → SSH.
					go io.Copy(f, s)
					io.Copy(s, f)
				} else {
					// No PTY requested — pipe stdio directly (non-interactive).
					cmd.Stdin = s
					cmd.Stdout = s
					cmd.Stderr = s.Stderr()
					cmd.Run()
				}
			}
		}),
	)
	if err != nil {
		log.Fatal(err)
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Printf("SSH server listening on %s:%d", *host, *port)
	go func() {
		<-ctx.Done()
		log.Println("Shutting down...")
		srv.Close()
	}()

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// ensureHostKey generates an RSA host key at path if one doesn't already exist.
func ensureHostKey(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	return pem.Encode(f, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}
