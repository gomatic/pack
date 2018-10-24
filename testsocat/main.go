package main

import (
	"io"
	"log"
	"net"
	"os"
	"os/exec"
)

func main() {
	ln, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Println(err)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go func(conn net.Conn) {
			defer conn.Close()
			cmd := exec.Command("docker", "run", "-i", "--network=host", "alpine/socat", "-", "TCP:localhost:32812")
			cmd.Stdin = conn
			cmd.Stdout = conn
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Println(err)
			}
		}(conn)
	}
}

func mainOLD() {
	// cmd := exec.Command("docker", "run", "alpine/socat", "TCP-LISTEN:80,reuseaddr,fork,su=nobody", "TCP:localhost:32820")
	// cmd.Stdout = os.Stdout
	// cmd.Stderr = os.Stderr
	// cmd.Run()

	r1, w1 := io.Pipe()
	r2, w2 := io.Pipe()
	cmd := exec.Command("docker", "run", "-i", "--network=host", "alpine/socat", "-", "TCP:localhost:32812")
	cmd.Stdin = r1
	cmd.Stdout = w2
	cmd.Stderr = os.Stderr
	go func() {
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
		w2.Close()
	}()

	go func() {
		if _, err := w1.Write([]byte("GET /v2/_catalog HTTP/1.1\nHost: localhost:32812\n\n")); err != nil {
			log.Fatal(err)
		}
		w1.Close()
	}()
	io.Copy(os.Stdout, r2)
}
