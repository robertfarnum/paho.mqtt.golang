package main

import (
	"flag"
	"fmt"
	"log"
)

var (
	region  string
	profile string
)

func main() {
	flag.StringVar(&region, "region", "us-west-2", "AWS Region")
	flag.StringVar(&profile, "profile", "saml", "AWS Profile")

	flag.Parse()

	url, err := GetWebsocketUrl(profile, region)
	if err != nil {
		log.Panic(err)
	}
	fmt.Print(url.String())
}
