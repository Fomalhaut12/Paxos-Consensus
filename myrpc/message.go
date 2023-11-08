package myrpc

// MessageType example
// type MessageType uint8

// const (
// 	PREPREPARE MessageType = iota
// 	PREPARE
// 	COMMIT
// )

//	func (mt MessageType) String() string {
//		switch mt {
//		case PREPREPARE:
//			return "PREPREPARE"
//		case PREPARE:
//			return "PREPARE"
//		case COMMIT:
//			return "COMMIT"
//		}
//		return "UNKNOW MESSAGETYPE"
//	}
type MsgType uint8

const (
	Prepare MsgType = iota
	Promise
	Propose
	Accept
)

type ConsensusMsg struct {
	// Type MessageType
	Tp   MsgType
	From uint8
	Seq  uint64
	Data []byte
}

type ConsensusMsgReply struct {
}
