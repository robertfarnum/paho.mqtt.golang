package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eclipse/paho.mqtt.golang/netconn"
	"github.com/eclipse/paho.mqtt.golang/packets"
	"github.com/gorilla/websocket"
)

func StartTCPServer(listen string, server string, isDebug bool, isTrace bool) {
	listener, err := net.Listen("tcp", listen)
	if err != nil {
		panic(err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(err)
		}

		go serve(conn, server, isTrace)
	}
}

func StartWebsocketServer(wsListen string, server string, isDebug bool, isTrace bool) {
	http.HandleFunc("/mqtt", func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{
			Subprotocols: []string{
				"mqtt",
			},
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}

		defer conn.Close()

		connector := &netconn.WebsocketConnector{
			Conn: conn,
		}
		go serve(connector, server, isTrace)
	})

	log.Fatal(http.ListenAndServe(wsListen, nil))
}

func StartServer(tcpListen string, wssListen string, mqttServer string, isDebug bool, isTrace bool) {
	debug = isDebug

	go StartTCPServer(tcpListen, mqttServer, isDebug, isTrace)

	StartWebsocketServer(wssListen, mqttServer, isDebug, isTrace)
}

func proxyStream(reader io.Reader, writer io.Writer, dumper func(*bytes.Buffer), rec Recorder) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
		if rec != nil {
			rec.Close()
		}
	}()

	for {
		cp, err := packets.ReadPacket(reader)
		if err != nil {
			fmt.Println(err.Error())
			break
		}

		buff := new(bytes.Buffer)
		err = cp.Write(buff)
		if err != nil {
			fmt.Println(err.Error())
			break
		}

		d := bytes.NewBuffer(buff.Bytes())
		dumper(d)

		// record the PDU
		if rec != nil {
			rec.SaveMessage(NewMqttMessage(time.Now().UnixNano(), buff.Bytes()))
		}

		count, err := buff.WriteTo(writer)
		if err != nil {
			fmt.Println(err.Error())
			break
		}

		if debug {
			fmt.Printf("Wrote %d bytes\n", count)
		}
	}
}

// serve a connected MQTT client
func serve(conn net.Conn, server string, trace bool) error {

	if debug {
		fmt.Printf("new connection: %v\n", conn.RemoteAddr())
		fmt.Printf("Connecting to: %s\n", server)
	}

	var rec Recorder = nil
	if trace {
		var err error
		name := strings.Replace(conn.RemoteAddr().String(), ":", "-", -1)
		rec, err = NewFileRecorder(name + ".trace")
		if err != nil {
			fmt.Print(err.Error())
			return err
		}
	}

	// first open a connection to the remote broker
	uri, err := url.Parse(server)
	if err != nil {
		return err
	}

	rConn, err := netconn.OpenConnection(uri, nil, time.Duration(time.Second*10), http.Header{}, nil)
	if err != nil {
		return err
	}
	defer rConn.Close()

	//  reverse proxy
	go proxyStream(rConn, conn, func(b *bytes.Buffer) {
		cp, err := packets.ReadPacket(b)
		if err != nil {
			fmt.Printf("RCVD ERROR: %v\n", err)
		}
		fmt.Printf("RCVD: %s\n", cp.String())
	}, rec)

	// forward proxy
	proxyStream(conn, rConn, func(b *bytes.Buffer) {
		cp, err := packets.ReadPacket(b)
		if err != nil {
			fmt.Printf("SENT ERROR: %v\n", err)
		}
		fmt.Printf("SENT: %s\n", cp.String())
	}, nil)

	err = conn.Close()
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	err = rConn.Close()
	if err != nil {
		fmt.Println(err.Error())
		return err
	}

	return nil
}
