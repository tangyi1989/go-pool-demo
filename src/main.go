package main

import (
	"bytes"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"gopkg.in/fatih/pool.v2"
)

var (
	serverId int = 0
)

const (
	LISTEN_PORT int = 23333
)

// --------------------------------- server ------------------------------------

type EchoServer struct {
	id       int
	listener *net.Listener
	clientId int
	lock     sync.Mutex
	wg       sync.WaitGroup
	clients  map[int]*net.Conn
}

func handleClient(clientId int, client *net.Conn, server *EchoServer) {
	defer func() {
		(*client).Close()

		server.lock.Lock()
		delete(server.clients, clientId)
		server.lock.Unlock()

		log.Printf("server[%d] client[%d] closed.", server.id, clientId)
	}()

	log.Printf("server[%d] client[%d] conencted .", server.id, clientId)
	for {
		buf := make([]byte, 256)
		n, err := (*client).Read(buf)

		if err != nil {
			break
		}

		log.Printf("server[%d] client[%d] read: %s", server.id,
			clientId, buf[:n])

		(*client).Write(buf[:n])
	}

}

func newEchoServer() (*EchoServer, error) {
	listener, err := net.Listen("tcp", ":"+strconv.Itoa(LISTEN_PORT))

	if err != nil {
		return nil, err
	}

	serverId += 1
	server := &EchoServer{
		id:       serverId,
		clientId: 1,
		listener: &listener,
		clients:  make(map[int]*net.Conn),
	}

	server.wg.Add(1)
	go func() {
		log.Printf("server[%d] Listening %v", server.id, listener.Addr())

		defer func() {
			log.Printf("server[%d] Stop listener %v", server.id,
				listener.Addr())
			for _, conn := range server.clients {
				(*conn).Close()
			}
			server.wg.Done()
		}()

		for {
			client, err := listener.Accept()
			if err != nil {
				log.Printf("server[%d] Listner accept error: %v",
					server.id, err)
				break
			}

			server.lock.Lock()
			server.clients[server.clientId] = &client
			server.lock.Unlock()

			go handleClient(server.clientId, &client, server)
			server.clientId += 1
		}
	}()

	return server, nil
}

// --------------------------------- client ------------------------------------

type Conn struct {
	net.Conn
	connected   bool
	readBufChan chan []byte
	err         error
	buf         []byte
}

var (
	Pool pool.Pool
)

func connFactory() (net.Conn, error) {
	c, err := net.Dial("tcp", "localhost:"+strconv.Itoa(LISTEN_PORT))
	if err != nil {
		return nil, err
	}

	readBufChan := make(chan []byte, 1024)
	conn := &Conn{c, true, readBufChan, nil, make([]byte, 0)}

	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := c.Read(buf)
			if err == nil {
				readBufChan <- buf[:n]
			} else {
				if len(buf) > 0 {
					readBufChan <- buf[:n]
				}

				conn.err = err
				conn.connected = false
				readBufChan <- nil
				break
			}
		}
	}()

	return conn, nil
}

func initPool() {
	p, err := pool.NewChannelPool(0, 10, connFactory)
	if err != nil {
		log.Fatal(err)
	}

	Pool = p
}

func (c *Conn) Read(buf []byte) (int, error) {
	if c.buf == nil {
		return 0, c.err
	}

	for i := 0; ; {
		if l := len(c.buf); l > 0 {
			n := len(buf)
			if l >= len(buf)-i {
				n = l
			}

			copy(buf[i:], c.buf[:n])
			c.buf = c.buf[n:]
			i += n
		}

		if i == len(buf) {
			return i, nil
		}

		c.buf = <-c.readBufChan
		if c.buf == nil {
			return i, c.err
		}
	}
}

func doEcho() error {
	var conn *Conn
	var pc *pool.PoolConn
	for {
		c, err := Pool.Get()
		if err != nil {
			return err
		}

		pc, _ = c.(*pool.PoolConn)
		conn, _ = pc.Conn.(*Conn)

		if !conn.connected {
			pc.MarkUnusable()

			conn.Close()
			continue
		}
		break
	}

	defer pc.Close()
	s := []byte("hello, world!")
	for i := 0; i < len(s); {
		n, err := conn.Write(s[i:])
		if err != nil {
			return err
		}
		i += n
	}

	buf := make([]byte, len(s))
	_, err := conn.Read(buf)
	if err != nil {
		pc.MarkUnusable()
		return err
	}

	if bytes.Compare(buf, s) != 0 {
		log.Printf("sent: %v, read: %v", buf, s)
		log.Fatal("ECHO not equal.")
	}

	return nil
}

// --------------------------------- utils -------------------------------------

func handleSignal() {
	sigc := make(chan os.Signal)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP,
		syscall.SIGTERM)

	for {
		s := <-sigc

		switch s {
		case syscall.SIGINT:
			os.Exit(0)
		case syscall.SIGQUIT:
			os.Exit(0)
		case syscall.SIGTERM:
			os.Exit(0)
		case syscall.SIGHUP:

		default:

		}
	}
}

// --------------------------------- main -------------------------------------

func initServer() {
	go func() {
		for {
			server, err := newEchoServer()

			if err != nil {
				log.Fatal(err)
				break
			}

			time.Sleep(10 * time.Second)
			(*server.listener).Close()

			server.wg.Wait()

			time.Sleep(3 * time.Second)
		}
	}()
}

func main() {
	initPool()
	initServer()

	for i := 0; i < 10; i++ {
		go func() {
			for {
				log.Printf("doEcho return: %v", doEcho())
				time.Sleep(1 * time.Second)
			}
		}()
	}

	handleSignal()
}
