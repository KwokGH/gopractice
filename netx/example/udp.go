package main

import (
	"fmt"
	"net"
)

// 服务端
func ServerUDP() {
	listen, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: 3000,
		Zone: "",
	})
	if err != nil {
		fmt.Println("监听失败 ", err)
		return
	}
	defer listen.Close()

	i := 0
	for {
		var data [1024]byte
		n, addr, err := listen.ReadFromUDP(data[:])
		if err != nil {
			fmt.Println("读取数据失败 ", err)
			continue
		}
		i++
		fmt.Printf("data:%v addr:%v count:%v seq:%d \n", string(data[:n]), addr, n, i)

		_, err = listen.WriteToUDP([]byte("我收到了"), addr)
		if err != nil {
			fmt.Println("写入数据失败 ", err)
			continue
		}
	}
}

func ClientUDP() {
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: 3000,
		Zone: "",
	})
	if err != nil {
		fmt.Println("连接服务端失败，err:", err)
		return
	}

	defer conn.Close()

	for i := 0; i < 20; i++ {
		_, err = conn.Write([]byte("hello server"))
		if err != nil {
			fmt.Println("发送数据失败，err:", err)
			return
		}
	}

	//data := make([]byte, 4096)
	//n, remoteAddr, err := conn.ReadFromUDP(data) // 接收数据
	//if err != nil {
	//	fmt.Println("接收数据失败，err:", err)
	//	return
	//}
	//fmt.Printf("recv:%v addr:%v count:%v\n", string(data[:n]), remoteAddr, n)
}
