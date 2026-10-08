// This loopback-only SSH fixture exercises the desktop's native PTY without
// using a real server, user credentials, or a system shell.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

func main() {
	dir := flag.String("dir", "", "temporary fixture directory")
	flag.Parse()
	if *dir == "" {
		panic("fixture directory is required")
	}
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	_, clientKey, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	hostSigner, err := ssh.NewSignerFromKey(hostKey)
	must(err)
	clientSigner, err := ssh.NewSignerFromKey(clientKey)
	must(err)
	privateBlock, err := ssh.MarshalPrivateKey(clientKey, "terminal fixture")
	must(err)
	keyPath := filepath.Join(*dir, "identity")
	must(os.WriteFile(keyPath, pem.EncodeToMemory(privateBlock), 0600))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	must(os.WriteFile(filepath.Join(*dir, "known_hosts"), []byte(fmt.Sprintf("[127.0.0.1]:%d %s", port, ssh.MarshalAuthorizedKey(hostSigner.PublicKey()))), 0600))
	must(os.WriteFile(filepath.Join(*dir, "known_hosts_wrong"), []byte(fmt.Sprintf("[127.0.0.1]:%d %s", port, ssh.MarshalAuthorizedKey(clientSigner.PublicKey()))), 0600))
	config := &ssh.ServerConfig{PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if conn.User() != "terminal-smoke" || string(key.Marshal()) != string(clientSigner.PublicKey().Marshal()) {
			return nil, fmt.Errorf("fixture authentication rejected")
		}
		return &ssh.Permissions{}, nil
	}, PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if conn.User() != "terminal-smoke" || string(password) != "fixture-password" {
			return nil, fmt.Errorf("fixture authentication rejected")
		}
		return &ssh.Permissions{}, nil
	}}
	config.AddHostKey(hostSigner)
	must(json.NewEncoder(os.Stdout).Encode(map[string]any{"port": port}))
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go serve(conn, config)
	}
}

func serve(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()
	server, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer server.Close()
	go ssh.DiscardRequests(requests)
	for pending := range channels {
		if pending.ChannelType() != "session" {
			_ = pending.Reject(ssh.UnknownChannelType, "sessions only")
			continue
		}
		channel, reqs, err := pending.Accept()
		if err != nil {
			continue
		}
		go session(channel, reqs)
	}
}

func session(channel ssh.Channel, requests <-chan *ssh.Request) {
	defer channel.Close()
	var mu sync.Mutex
	cols, rows := uint32(0), uint32(0)
	term := ""
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for request := range requests {
			ok := false
			switch request.Type {
			case "pty-req":
				var size struct {
					Term                      string
					Cols, Rows, Width, Height uint32
					Modes                     string
				}
				if ssh.Unmarshal(request.Payload, &size) == nil {
					mu.Lock()
					cols, rows = size.Cols, size.Rows
					term = size.Term
					mu.Unlock()
					ok = true
				}
			case "window-change":
				var size struct{ Cols, Rows, Width, Height uint32 }
				if ssh.Unmarshal(request.Payload, &size) == nil {
					mu.Lock()
					cols, rows = size.Cols, size.Rows
					mu.Unlock()
					ok = true
				}
			case "shell":
				select {
				case <-started:
				default:
					close(started)
				}
				ok = true
			}
			if request.WantReply {
				_ = request.Reply(ok, nil)
			}
		}
	}()
	select {
	case <-started:
	case <-done:
		return
	}
	_, _ = channel.Write([]byte("fixture-ready\r\n"))
	buffer := make([]byte, 1024)
	var line strings.Builder
	for {
		n, err := channel.Read(buffer)
		if err != nil {
			return
		}
		for _, b := range buffer[:n] {
			if b != '\r' && b != '\n' {
				if line.Len() < 256 {
					line.WriteByte(b)
				}
				continue
			}
			command := line.String()
			line.Reset()
			if command == "exit" {
				_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{0}))
				return
			}
			if command == "probe" {
				mu.Lock()
				message := fmt.Sprintf("user=terminal-smoke cols=%d rows=%d term=%s\r\n", cols, rows, term)
				mu.Unlock()
				_, _ = channel.Write([]byte(message))
			}
		}
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
