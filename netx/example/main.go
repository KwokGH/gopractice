package main

import (
	"flag"
	"fmt"
)

func main() {
	var network string
	var app string
	flag.StringVar(&network, "n", "tcp", "tcp/udp，默认为tcp")
	flag.StringVar(&app, "a", "server", "server/client/client_sp，默认为server")
	flag.Parse()

	if network == "tcp" {
		if app == "server" {
			Server()
		} else if app == "client" {
			Client()
		} else if app == "client_sp" {
			ClientTestStickyPacket()
		} else {
			fmt.Sprintln("参数不正确")
		}
	}

	if network == "udp" {
		if app == "server" {
			ServerUDP()
		} else if app == "client" {
			ClientUDP()
		} else {
			fmt.Sprintln("参数不正确")
		}
	}
}
