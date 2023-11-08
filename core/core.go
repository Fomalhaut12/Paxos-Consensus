package core

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/rpc"
	"reflect"
	"strconv"
	"time"

	"nis3607/mylogger"
	"nis3607/myrpc"
)

type Consensus struct {
	id   uint8
	n    uint8
	port uint64
	seq  uint64
	//BlockChain
	blockChain *BlockChain
	//logger
	logger *mylogger.MyLogger
	//rpc network
	peers []*myrpc.ClientEnd

	data         []byte //acceptor的已接受value
	promiseNum   uint64 //promise的提案编号
	acceptedNum  uint64 //已接受的提案编号
	acceptedData []byte //已接受的提案值
	promised     uint8  //proposer收到的promise数

	//message channel
	msgChan chan *myrpc.ConsensusMsg
	acChan  chan *myrpc.ConsensusMsg
	lrChan  chan *myrpc.ConsensusMsg

	//learn
	cuChose []byte
	cuCount uint8
	lastSeq uint64
}

func InitConsensus(config *Configuration) *Consensus {
	rand.Seed(time.Now().UnixNano())
	c := &Consensus{
		id:         config.Id,
		n:          config.N,
		port:       config.Port,
		seq:        0,
		lastSeq:    0,
		blockChain: InitBlockChain(config.Id, config.BlockSize),
		logger:     mylogger.InitLogger("node", config.Id),
		peers:      make([]*myrpc.ClientEnd, 0),

		msgChan: make(chan *myrpc.ConsensusMsg, 1024),
		acChan:  make(chan *myrpc.ConsensusMsg, 1024),
		lrChan:  make(chan *myrpc.ConsensusMsg, 1024),
	}
	for _, peer := range config.Committee {
		clientEnd := &myrpc.ClientEnd{Port: uint64(peer)}
		c.peers = append(c.peers, clientEnd)
	}
	go c.serve()

	return c
}

func (c *Consensus) serve() {
	// 注册Consensus类型的rpc服务
	rpc.Register(c)
	// 为注册的rpc服务创建HTTP请求处理器
	rpc.HandleHTTP()
	// 在指定端口上开始监听TCP连接
	l, e := net.Listen("tcp", ":"+strconv.Itoa(int(c.port)))
	if e != nil {
		log.Fatal("listen error:", e)
	}
	// 在一个新的goroutine中启动HTTP服务器，接收连接并处理请求
	go http.Serve(l, nil)
}

func (c *Consensus) OnReceiveMessage(args *myrpc.ConsensusMsg, reply *myrpc.ConsensusMsgReply) error {

	c.logger.DPrintf("Invoke RpcExample: receive message from %v at %v", args.From, time.Now().Nanosecond())

	switch args.Tp {
	case myrpc.Prepare:
		c.acChan <- args
	case myrpc.Propose:
		c.acChan <- args
	case myrpc.Promise:
		c.msgChan <- args
	case myrpc.Accept:
		c.lrChan <- args
	}

	return nil
}

func (c *Consensus) broadcastMessage(msg *myrpc.ConsensusMsg) {
	// 创建一个空的reply变量
	reply := &myrpc.ConsensusMsgReply{}
	// 遍历所有的节点，向它们广播消息
	for id := range c.peers {
		c.peers[id].Call("Consensus.OnReceiveMessage", msg, reply)
	}
}

//func (c *Consensus) handleMsgExample(msg *myrpc.ConsensusMsg) {

func (c *Consensus) Run() {
	// wait for other node to start 等待其它节点启动
	time.Sleep(time.Duration(1) * time.Second)
	//init rpc client 初始化rpc客户端
	rand.Seed(time.Now().UnixNano())
	for id := range c.peers {
		c.peers[id].Connect()
	}
	// 启动提议循环
	go c.proposeLoop()
	go c.Run_acceptor()
	c.Run_learner()
	//handle received message
	//for {
	// 从消息通道中读取消息
	//msg := <-c.msgChan
	// 处理消息
	//c.handleMsgExample(msg)
	//}

}

func (c *Consensus) proposeLoop() {
	for {
		// 如果当前节点是主节点
		flag := c.seq % 7
		if flag == uint64(c.id) {
			c.Run_proposer()
		}
	}
}

func (c *Consensus) Run_proposer() {

	//生成新的区块
	block := c.blockChain.getBlock(c.seq)
	pre_msg := &myrpc.ConsensusMsg{
		Tp:   myrpc.Prepare,
		From: c.id,
		Seq:  c.seq,
		Data: block.Data,
	}

	//广播Prepare
	c.broadcastMessage(pre_msg)
	c.data = block.Data

	timeout := time.After(10 * time.Millisecond)
l:
	for {
		select {
		case msg := <-c.msgChan:

			switch msg.Tp {
			case myrpc.Promise:
				c.promised += 1
				if msg.Seq > 0 {
					//c.seq = msg.Seq
					c.data = msg.Data
				}

			default:
				panic("UnSupport message.")
			}
		case <-timeout:

			break l
		}
	}

	if c.promised >= 4 {
		pro_msg := &myrpc.ConsensusMsg{
			Tp:   myrpc.Propose,
			From: c.id,
			Seq:  c.seq,
			Data: c.data,
		}
		c.broadcastMessage(pro_msg)
		c.promised = 0
	}
	time.Sleep(20 * time.Millisecond)

}

func (c *Consensus) Run_acceptor() {
	for {
		timeout := time.After(60 * time.Millisecond)
		select {
		case msg := <-c.acChan:
			switch msg.Tp {
			case myrpc.Prepare:
				if c.promiseNum < msg.Seq {
					//只有当收到的seq大于已promised的数字才promise
					c.promiseNum = msg.Seq
					rep := &myrpc.ConsensusMsg{
						Tp:   myrpc.Promise,
						From: c.id,
						Seq:  c.acceptedNum,
						Data: c.acceptedData,
					}
					reply := &myrpc.ConsensusMsgReply{}
					c.peers[msg.From].Call("Consensus.OnReceiveMessage", rep, reply)
				}
			case myrpc.Propose:
				num := msg.Seq
				if num >= c.promiseNum { //
					c.acceptedNum = num
					c.acceptedData = msg.Data
					c.promiseNum = num
					ac := &myrpc.ConsensusMsg{
						Tp:   myrpc.Accept,
						From: c.id,
						Seq:  c.acceptedNum,
						Data: c.acceptedData,
					}
					c.broadcastMessage(ac)
				}

			}
		case <-timeout:
			t := rand.Intn(7)
			if t == int(c.id) {
				c.seq++
				fmt.Printf("seq increased")
			}
		}

	}
}

func (c *Consensus) Run_learner() {
	for {
		msg := <-c.lrChan
		c.chosen(msg)

	}
}

func (c *Consensus) chosen(msg *myrpc.ConsensusMsg) {

	if !reflect.DeepEqual(c.cuChose, msg.Data) {
		c.cuChose = msg.Data
		c.cuCount = 1
	} else {
		c.cuCount += 1
	}
	if c.cuCount == 4 {
		if msg.Seq != c.lastSeq && (msg.Seq == 0 || msg.Seq > c.lastSeq) {
			block := &Block{
				Seq:  msg.Seq,
				Data: msg.Data,
			}
			// 将构造的Block提交到区块链中
			c.blockChain.commitBlock(block)
			c.lastSeq = msg.Seq
		}
		c.seq = msg.Seq + 1
		c.acceptedNum = 0
		c.acceptedData = []byte{}
		c.cuChose = []byte{}
	}
}
