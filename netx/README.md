## 在不同平台上的支持
Go 语言为了提高在不同操作系统上的 I/O 操作性能，使用平台特定的函数实现了多个版本的网络轮询模块。
* src/runtime/netpoll_epoll.go
* src/runtime/netpoll_kqueue.go
* src/runtime/netpoll_solaris.go
* src/runtime/netpoll_windows.go
* src/runtime/netpoll_aix.go
* src/runtime/netpoll_fake.go


## 参考资源
* [Go语言基础之网络编程](https://www.liwenzhou.com/posts/Go/15_socket/)