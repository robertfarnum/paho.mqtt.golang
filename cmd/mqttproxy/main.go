package main

import (
	"flag"
	"fmt"
)

var tcpListen string
var wssListen string
var srv string
var debug bool
var trace bool
var useAWS bool

func main() {
	fmt.Println("mqttproxy: An MQTT proxy")

	flag.StringVar(&tcpListen, "ts", "0.0.0.0:1883", "the MQTT TCP address and port to listen for MQTT proxy clients")
	flag.StringVar(&wssListen, "ws", "0.0.0.0:443", "the MQTT WSS address and port to listen for MQTT proxy clients")

	flag.StringVar(&srv, "server", "", "the MQTT address and port to connect to the MQTT server")
	flag.BoolVar(&debug, "v", false, "dumps verbose debug information")
	flag.BoolVar(&trace, "t", false, "trace every communication to JSON files")
	flag.Parse()

	if debug {
		fmt.Println("verbose mode enabled")
	}

	StartServer(tcpListen, wssListen, srv, debug, trace)
}
