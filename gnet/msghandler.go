package gnet

import (
	"Ginx/gface"
	"Ginx/metrics"
	"errors"
	"fmt"
	"runtime/debug"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type MsgHandle struct {
	//存放每个MsgId 所对应的处理方法的map属性
	Apis map[uint32]gface.IRouter
	//业务工作Worker池的数量
	WorkerPoolSize uint32
	//Worker负责取任务的消息队列
	TaskQueue         []chan gface.IRequest
	workerPoolOnce    sync.Once
	workerPoolStarted atomic.Bool
	workerPoolWait    sync.WaitGroup
	stopSignalOnce    sync.Once
	stopPoolOnce      sync.Once
	stopChan          chan struct{}
	taskQueueLock     sync.RWMutex
	panicHandlerLock  sync.RWMutex
	panicHandler      func(gface.IRequest, interface{}, []byte)
	metrics           *metrics.Metrics
	config            *Config
}

func NewMsgHandle() *MsgHandle {
	return newMsgHandle(nil)
}

func newMsgHandle(config *Config) *MsgHandle {
	settings := effectiveConfig(config)
	return &MsgHandle{
		stopChan:       make(chan struct{}),
		config:         config,
		Apis:           make(map[uint32]gface.IRouter),
		WorkerPoolSize: settings.WorkerPoolSize,
		//一个worker对应一个queue
		TaskQueue: make([]chan gface.IRequest, settings.WorkerPoolSize),
	}
}

// 马上以非阻塞方式处理消息
func (mh *MsgHandle) DoMsgHandler(request gface.IRequest) {
	if request == nil {
		return
	}
	handler, ok := mh.Apis[request.GetMsgID()]
	if !ok {
		fmt.Println("api msgId = ", request.GetMsgID(), " is not FOUND!")
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			if mh.metrics != nil {
				mh.metrics.IncRouterPanics()
			}
			mh.panicHandlerLock.RLock()
			handlerFunc := mh.panicHandler
			mh.panicHandlerLock.RUnlock()
			if handlerFunc != nil {
				handlerFunc(request, recovered, debug.Stack())
				return
			}
			fmt.Println("router panic, msgId = ", request.GetMsgID(), ", panic = ", recovered)
			fmt.Println(string(debug.Stack()))
		}
	}()

	//执行对应处理方法
	handler.PreHandle(request)
	handler.Handle(request)
	handler.PostHandle(request)
}

// SetMetrics 设置路由处理指标收集器。
func (mh *MsgHandle) SetMetrics(value *metrics.Metrics) {
	mh.metrics = value
}

// SetPanicHandler 设置路由 panic 回调，未设置时默认输出消息 ID 和堆栈。
func (mh *MsgHandle) SetPanicHandler(handler func(gface.IRequest, interface{}, []byte)) {
	mh.panicHandlerLock.Lock()
	defer mh.panicHandlerLock.Unlock()
	mh.panicHandler = handler
}

// 为消息添加具体的处理逻辑
func (mh *MsgHandle) AddRouter(msgId uint32, router gface.IRouter) {
	//1 判断当前msg绑定的API处理方法是否已经存在
	if _, ok := mh.Apis[msgId]; ok {
		panic("repeated api , msgId = " + strconv.Itoa(int(msgId)))
	}
	//2 添加msg与api的绑定关系
	mh.Apis[msgId] = router
	fmt.Println("Add api msgId = ", msgId)
}

// 启动一个Worker工作流程
func (mh *MsgHandle) startOneWorker(workerID int, taskQueue chan gface.IRequest) {
	fmt.Println("Worker ID = ", workerID, " is started.")
	//不断的等待队列中的消息
	for request := range taskQueue {
		//有消息则取出队列的Request，并执行绑定的业务方法
		mh.DoMsgHandler(request)
	}
}

// 启动worker工作池
func (mh *MsgHandle) StartWorkerPool() {
	if mh.WorkerPoolSize == 0 {
		return
	}

	mh.workerPoolOnce.Do(func() {
		//遍历需要启动worker的数量，依此启动
		mh.taskQueueLock.Lock()
		select {
		case <-mh.stopChan:
			mh.taskQueueLock.Unlock()
			return
		default:
		}
		for i := 0; i < int(mh.WorkerPoolSize); i++ {
			//一个worker被启动
			//给当前worker对应的任务队列开辟空间
			mh.TaskQueue[i] = make(chan gface.IRequest, effectiveConfig(mh.config).MaxWorkerTaskLen)
		}

		mh.workerPoolWait.Add(int(mh.WorkerPoolSize))
		mh.workerPoolStarted.Store(true)
		mh.taskQueueLock.Unlock()
		for i := 0; i < int(mh.WorkerPoolSize); i++ {
			//启动当前Worker，阻塞的等待对应的任务队列是否有消息传递进来
			go func(workerID int) {
				defer mh.workerPoolWait.Done()
				mh.startOneWorker(workerID, mh.TaskQueue[workerID])
			}(i)
		}
	})
}

func (mh *MsgHandle) StopWorkerPool() {
	mh.beginStop()
	mh.stopPoolOnce.Do(func() {
		mh.taskQueueLock.Lock()
		defer mh.taskQueueLock.Unlock()
		if mh.workerPoolStarted.Load() {
			for _, taskQueue := range mh.TaskQueue {
				close(taskQueue)
			}
			mh.workerPoolStarted.Store(false)
		}
	})
	// 停服后的所有调用者都等待同一组任务，包括零 Worker 模式。
	mh.workerPoolWait.Wait()
}

func (mh *MsgHandle) beginStop() {
	mh.stopSignalOnce.Do(func() { close(mh.stopChan) })
}

// 将消息交给TaskQueue,由worker进行处理
func (mh *MsgHandle) SendMsgToTaskQueue(request gface.IRequest) error {
	mh.taskQueueLock.RLock()
	defer mh.taskQueueLock.RUnlock()
	select {
	case <-mh.stopChan:
		return ErrWorkerStopped
	default:
	}
	if !mh.workerPoolStarted.Load() {
		mh.workerPoolWait.Add(1)
		go func() {
			defer mh.workerPoolWait.Done()
			mh.DoMsgHandler(request)
		}()
		return nil
	}

	//根据ConnID来分配当前的连接应该由哪个worker负责处理
	//轮询的平均分配法则

	//得到需要处理此条连接的workerID
	workerID := request.GetConnection().GetConnId() % mh.WorkerPoolSize
	fmt.Println("Add ConnID=", request.GetConnection().GetConnId(), " request msgID=", request.GetMsgID(), "to workerID=", workerID)
	//将请求消息发送给任务队列
	queue := mh.TaskQueue[workerID]
	waitTime := time.Duration(effectiveConfig(mh.config).WorkerTaskQueueWaitTime) * time.Millisecond
	if waitTime <= 0 {
		select {
		case queue <- request:
			return nil
		default:
			return errors.New("worker task queue is full")
		}
	}

	timer := time.NewTimer(waitTime)
	defer timer.Stop()
	select {
	case queue <- request:
		return nil
	case <-mh.stopChan:
		return ErrWorkerStopped
	case <-gface.RequestContext(request).Done():
		return gface.RequestContext(request).Err()
	case <-timer.C:
		return errors.New("worker task queue wait timeout")
	}
}
