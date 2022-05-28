package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"
)

// Server tcp 服务端
func Server() {
	// 监听
	listen, err := net.Listen("tcp", "127.0.0.1:8001")
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("服务端已启动。。。")

	defer listen.Close()

	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Println("accept failed, err:", err)
			continue
		}

		// 启动一个goroutine处理连接
		//go process(conn)
		go processCode(conn)
	}
}

// 服务端处理逻辑
func process(conn net.Conn) {
	defer conn.Close()

	for {
		var buf [128]byte
		n, err := conn.Read(buf[:])
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("read from client failed, err:", err)
			break
		}

		recvData := new(dataReq)
		err = json.Unmarshal(buf[:n], recvData)
		if err != nil {
			fmt.Println("json error", err)
			continue
		}
		fmt.Println("收到client端发来的数据：", recvData.Name)

		// 处理逻辑
		//time.Sleep(3 * time.Second)

		// 响应
		//recvStr := fmt.Sprintf("[%s]---你的消息我处理过了", recvStr)
		//conn.Write([]byte(recvStr))
	}

}

// Client 客户端
func Client() {
	conn, err := net.Dial("tcp", "127.0.0.1:8001")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer conn.Close()

	inputReader := bufio.NewReader(os.Stdin)
	for {
		input, _ := inputReader.ReadString('\n') // 读取用户输入
		inputInfo := strings.Trim(input, "\r\n")
		if strings.ToUpper(inputInfo) == "Q" { // 如果输入q就退出
			return
		}
		_, err = conn.Write([]byte(inputInfo)) // 发送数据
		if err != nil {
			fmt.Println("发送数据失败, err:", err)
			return
		}
		buf := [512]byte{}
		n, err := conn.Read(buf[:])
		if err != nil {
			fmt.Println("读取服务器数据失败, err:", err)
			return
		}

		resp := string(buf[:n])
		fmt.Println(resp)
	}
}

type dataReq struct {
	Name string `json:"name"`
}

// ClientTestStickyPacket 复现粘包场景
// 跟粘包关系最大的就是基于字节流这个特点，数据可能被切割和组装成各种数据包，接收端收到这些数据包后没有正确还原原来的消息，因此出现粘包现象。
// ref: https://segmentfault.com/a/1190000039691657
func ClientTestStickyPacket() {
	conn, _ := net.Dial("tcp", "127.0.0.1:8001")
	defer conn.Close()

	for i := 0; i < 20; i++ {
		str, _ := json.Marshal(dataReq{
			Name: fmt.Sprintf("%s-%d", "name", i),
		})
		b, _ := Encode(string(str))
		conn.Write(b)
	}
	time.Sleep(3 * time.Second)
}

// 解决粘包问题
// 出现”粘包”的关键在于接收方不确定将要传输的数据包的大小，因此我们可以对数据包进行封包和拆包的操作。
// 超过一个字节的数据类型在内存中存储的顺序有大小端模式，所以int8不能用

// Encode 编码
func Encode(msg string) ([]byte, error) {
	length := int32(len(msg))
	pkg := new(bytes.Buffer)
	err := binary.Write(pkg, binary.LittleEndian, length)
	if err != nil {
		return nil, err
	}

	err = binary.Write(pkg, binary.LittleEndian, []byte(msg))
	if err != nil {
		return nil, err
	}

	return pkg.Bytes(), nil
}

// Decode 解码
func Decode(reader *bufio.Reader) ([]byte, error) {
	// 读取消息长度
	lenBytes, _ := reader.Peek(4)
	lenBuff := bytes.NewBuffer(lenBytes)
	var length int32
	err := binary.Read(lenBuff, binary.LittleEndian, &length)
	if err != nil {
		return nil, err
	}

	// Buffered返回缓冲中现有的可读取的字节数, 是不是一个完整的消息，不是则直接返回
	if int32(reader.Buffered()) < length+4 {
		return nil, err
	}

	// 读取真正的消息数据
	pack := make([]byte, length+4)
	_, err = reader.Read(pack)
	if err != nil {
		return nil, err
	}

	return pack[4:], nil
}

func processCode(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for {
		b, err := Decode(reader)
		if err != nil {
			return
		}
		recvData := new(dataReq)
		err = json.Unmarshal(b, recvData)
		if err != nil {
			fmt.Println("json error", err)
			continue
		}
		fmt.Println("收到client端发来的数据：", recvData.Name)
	}
}
