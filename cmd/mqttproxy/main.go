package main

import (
	"flag"
	"fmt"
)

var listen string
var srv string
var debug bool
var trace bool

func main() {
	fmt.Println("mqttproxy: An MQTT proxy")

	flag.StringVar(&listen, "listen", "0.0.0.0:1882", "the MQTT address and port to listen for MQTT proxy clients")
	flag.StringVar(&srv, "server", "mqtt://0.0.0.0:1883", "the MQTT address and port to connect to the MQTT server")
	flag.BoolVar(&debug, "v", false, "dumps verbose debug information")
	flag.BoolVar(&trace, "t", false, "trace every communication to JSON files")
	flag.Parse()

	if debug {
		fmt.Println("verbose mode enabled")
	}

	StartServer(listen, srv, debug, trace)
}
