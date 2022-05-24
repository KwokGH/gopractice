package source

import (
	"errors"
	"gopractice/reflectlite"
	"sync"
	"sync/atomic"
	"time"
)

// Context 这四个方法都是幂等的，连续多次调用同一个方法，返回的结果都是相同的。
type Context interface {
	// Deadline 返回 context 被取消的时间，如果没有设置截止时间，ok 返回 false。
	Deadline() (deadline time.Time, ok bool)
	// Done 返回一个只读的 channel，
	// 当 Context 被主动取消或者超时自动取消时，该 Context 及其派生的 Context 的 done channel 将会被关闭，
	// 读取一个关闭的 channel 会读出相应类型的零值，正好利用这点，与 select 配合使用，实现协程控制或者超时退出等。
	Done() <-chan struct{}
	// Err 返回一个 error 对象，
	// 当 channel 没有被 close 的时候，返回 nil，
	// 如果 channel 被 close, 返回 channel 被 close 的原因。
	Err() error
	// Value 获取设置的 key 对应的 value，如果不存在则返回 nil。
	Value(key any) any
}

// 如果一个 Context 类型实现了上面定义的两个方法，该 Context 就是一个可取消的 Context。
type canceler interface {
	cancel(removeFromParent bool, err error)
	Done() <-chan struct{}
}

type stringer interface {
	String() string
}

// 通常用于创建 root Context，标准库中 context.Background() 和 context.TODO() 返回的就是这个
// emptyCtx 不能取消、不能传值且没有 deadline。
type emptyCtx int

func (*emptyCtx) Deadline() (deadline time.Time, ok bool) {
	return
}
func (*emptyCtx) Done() <-chan struct{} {
	return nil
}
func (*emptyCtx) Err() error {
	return nil
}
func (*emptyCtx) Value(key any) any {
	return nil
}

// 两者都是不可取消的 Context，通常都是放在 main 函数或者最顶层使用。
var (
	background = new(emptyCtx)
	todo       = new(emptyCtx)
)

func Background() Context {
	return background
}
func TODO() Context {
	return todo
}

// 可取消的 Context，实现了 canceler 接口
type cancelCtx struct {
	Context

	// 用于保护结构体中的字段，在访问修改的时候进行加锁处理，防止并发 data race 冲突。
	mu sync.Mutex
	// 存储的value是chan struct{}类型，配合 close(done) 实现信息通知，当一个 channel 被关闭之后，它返回的是该类型零值，此处是 struct{}。
	done atomic.Value
	// 保存可取消的子节点，cancelCtx 可以级联成一个树形结构。
	children map[canceler]struct{}
	// 当 done 没有关闭时，err 返回 nil，
	// 当 done 被关闭时，err 返回非空值，内容是被关闭的原因，是主动 cancel 还是 timeout 取消，
	// 这些错误信息都是 context 包内部定义的
	err error
}

var cancelCtxKey int

// Value cancelCtxKey 是一个 Context 包内部变量，
// 将 key 与 &cancelCtxKey 比较，相等的话就返回 *cancelCtx，
// 即 cancelCtx 的自身地址；否则继续递归。
func (c *cancelCtx) Value(key any) any {
	if key == &cancelCtxKey {
		return c
	}

	return value(c.Context, key)
}

func value(c Context, key any) any {
	for {
		switch ctx := c.(type) {
		case *valueCtx:
			if key == ctx.key {
				return ctx.val
			}
			c = ctx.Context
		case *cancelCtx:
			if key == &cancelCtxKey {
				return c
			}
			c = ctx.Context
		case *timerCtx:
			if key == &cancelCtxKey {
				return &ctx.cancelCtx
			}
			c = ctx.Context
		case *emptyCtx:
			return nil
		default:
			return c.Value(key)
		}
	}
	return nil
}

// Done c.done 是“懒汉式”初始化，只有调用了 Done() 方法的时候才会被创建。
func (c *cancelCtx) Done() <-chan struct{} {
	d := c.done.Load()
	if d != nil {
		return d.(chan struct{})
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	d = c.done.Load()
	if d == nil {
		d = make(chan struct{})
		c.done.Store(d)
	}

	return d.(chan struct{})
}

func (c *cancelCtx) Err() error {
	c.mu.Lock()
	err := c.err
	c.mu.Unlock()
	return err
}
func (c *cancelCtx) String() string {
	return contextName(c.Context) + ".WithCancel"
}
func contextName(c Context) string {
	if s, ok := c.(stringer); ok {
		return s.String()
	}
	return reflectlite.TypeOf(c).String()
}

// closedchan 一个可重用的已关闭的通道
var closedchan = make(chan struct{})

func init() {
	close(closedchan)
}

func (c *cancelCtx) cancel(removeFromParent bool, err error) {
	if err == nil {
		panic("context: internal error: missing cancel error")
	}

	c.mu.Lock()
	// 再次判断，防止重复取消
	if c.err != nil {
		c.mu.Unlock()
		return // already canceled
	}
	c.err = err

	// 如果 c.done 还未初始化，说明 Done() 方法还未被调用，这时候直接将 c.done 赋值一个已关闭的 channel
	// 此时Done() 方法被调用的时候不会阻塞直接返回 struct{}
	d, _ := c.done.Load().(chan struct{})
	if d == nil {
		c.done.Store(closedchan)
	} else {
		close(d)
	}

	// 如果有子节点，递归对子节点进行 cancel 操作
	for child := range c.children {
		// 在父锁的范围内，递归调用子节点的cancel
		child.cancel(false, err)
	}
	c.children = nil
	c.mu.Unlock()

	if removeFromParent {
		// 将本节点从它的父节点中删除
		removeChild(c.Context, c)
	}
}

func removeChild(parent Context, child canceler) {
	p, ok := parentCancelCtx(parent)
	if !ok {
		return
	}

	p.mu.Lock()
	if p.children != nil {
		delete(p.children, child)
	}
	p.mu.Unlock()
}

func parentCancelCtx(parent Context) (*cancelCtx, bool) {
	// 从 parent 开始向上寻找第一个可取消的 *cancelCtx
	// 如果 parent done 为 nil 表示是不可取消的 Context；
	// 如果 parent done 为 closedchan 表示 Context 已经被取消了，这两种情况都直接返回。
	done := parent.Done()
	if done == closedchan || done == nil {
		return nil, false
	}

	// 递归向上查询第一个 *cancelCtx
	// parent.Value(&cancelCtxKey) 递归向上查找节点是不是 cancelCtx。
	p, ok := parent.Value(&cancelCtxKey).(*cancelCtx)
	if !ok {
		return nil, false
	}

	// 注意这里 p.done==done 的判断，是防止下面的情况，parent.Done() 找到的可取消 Context 是我们自定义的可取消Context,
	// 这样 parent.Done() 返回的 done 和 cancelCtx 肯定不在一个同级，它们的 done 肯定是不同的。这种情况也返回 nil。
	pdone, _ := p.done.Load().(chan struct{})
	if pdone != done {
		return nil, false
	}

	return p, true
}

// WithValue valueCtx 是一个 k-v Context，只能使用 WithValue() 函数创建，返回 *valueCtx
// 因为 c.Context 指向父节点。并且只能向上查询，父节点没法获取子节点存储的值，子节点却可以获取父节点的值
// 另外递归向上只能查找 “直系” Context，也就是说可以无限递归查找 parent Context 是否包含这个 key，但是无法查找兄弟 Context 是否包含
func WithValue(parent Context, key, val any) Context {
	if parent == nil {
		panic("cannot create context from nil parent")
	}
	if key == nil {
		panic("nil key")
	}
	// 可比较是必须的
	if !reflectlite.TypeOf(key).Comparable() {
		panic("key is not comparable")
	}

	return &valueCtx{parent, key, val}
}

type valueCtx struct {
	Context
	key, val any
}

func stringify(v any) string {
	switch s := v.(type) {
	case stringer:
		return s.String()
	case string:
		return s
	}
	return "<not Stringer>"
}
func (c *valueCtx) String() string {
	return contextName(c.Context) + ".WithValue(type " +
		reflectlite.TypeOf(c.key).String() +
		", val " + stringify(c.val) + ")"
}

func (c *valueCtx) Value(key any) any {
	if c.key == key {
		return c.val
	}
	return value(c.Context, key)
}

type CancelFunc func()

func WithCancel(parent Context) (ctx Context, cancel CancelFunc) {
	if parent == nil {
		panic("cannot create context from nil parent")
	}

	c := newCancelCtx(parent)
	propagateCancel(parent, &c)
	return &c, func() {
		c.cancel(true, Canceled)
	}
}

func propagateCancel(parent Context, child canceler) {
	done := parent.Done()
	if done == nil {
		return // parent is never canceled
	}

	// 如果 done channel 不是 nil，说明 parent Context 是一个可以取消的 Context
	// 这里立即判断一下 done channel 是否可读取
	// 如果可以读取的话说明 parent Context 已经被取消了，那么应该立即取消 child Context
	select {
	case <-done:
		// parent is already canceled
		child.cancel(false, parent.Err())
		return
	default:
	}

	if p, ok := parentCancelCtx(parent); ok {
		p.mu.Lock()
		if p.err != nil {
			// parent has already been canceled
			child.cancel(false, p.err)
		} else {
			if p.children == nil {
				p.children = make(map[canceler]struct{})
			}
			// 将子节点挂靠到父节点上，形成级联关系
			p.children[child] = struct{}{}
		}
		p.mu.Unlock()
	} else {
		atomic.AddInt32(&goroutines, +1)
		// 代码走到这里，说明向上无法找到可取消的 *cancelCtx，这种情况可能是自定义实现的 Context 类型
		// 这种情况下无法通过 parent Context 的 children map 建立关联，只能通过创建一个 goroutine 来完成及联取消的操作
		go func() {
			select {
			// 这里的 parent.Done() 不能省略，当 parent context 取消时，需要取消下面的 child cotext
			// 如果省略了就不能级联取消 child context
			case <-parent.Done():
				child.cancel(false, parent.Err())
			case <-child.Done():
				// 当 child 取消时，goroutine 退出，防止泄露
			}
		}()
	}
}

// goroutines counts the number of goroutines ever created; for testing.
var goroutines int32

func newCancelCtx(parent Context) cancelCtx {
	return cancelCtx{Context: parent}
}

func WithTimeout(parent Context, timeout time.Duration) (Context, CancelFunc) {
	return WithDeadline(parent, time.Now().Add(timeout))
}

func WithDeadline(parent Context, d time.Time) (Context, CancelFunc) {
	if parent == nil {
		panic("cannot create context from nil parent")
	}

	if cur, ok := parent.Deadline(); ok && cur.Before(d) {
		return WithCancel(parent)
	}

	c := &timerCtx{
		cancelCtx: newCancelCtx(parent),
		deadline:  d,
	}
	propagateCancel(parent, c)
	dur := time.Until(d)
	if dur <= 0 {
		c.cancel(true, DeadlineExceeded)
		return c, func() {
			c.cancel(false, Canceled)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil {
		c.timer = time.AfterFunc(dur, func() {
			c.cancel(true, DeadlineExceeded)
		})
	}

	return c, func() {
		c.cancel(true, Canceled)
	}
}

var DeadlineExceeded error = deadlineExceededError{}
var Canceled = errors.New("context canceled")

type deadlineExceededError struct{}

func (deadlineExceededError) Error() string   { return "context deadline exceeded" }
func (deadlineExceededError) Timeout() bool   { return true }
func (deadlineExceededError) Temporary() bool { return true }

type timerCtx struct {
	cancelCtx
	timer *time.Timer

	deadline time.Time
}

func (c *timerCtx) Deadline() (deadline time.Time, ok bool) {
	return c.deadline, true
}
func (c *timerCtx) String() string {
	return contextName(c.cancelCtx.Context) + ".WithDeadline(" +
		c.deadline.String() + " [" +
		time.Until(c.deadline).String() + "])"
}
func (c *timerCtx) cancel(removeFromParent bool, err error) {
	// 调用cancelCtx的取消方法，取消子节点
	c.cancelCtx.cancel(false, err)
	if removeFromParent {
		// 将当前的 *timerCtx 从父节点移除掉
		removeChild(c.cancelCtx.Context, c)
	}
	c.mu.Lock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()
}
