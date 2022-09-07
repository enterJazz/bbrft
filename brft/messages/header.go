package messages

import "golang.org/x/crypto/cryptobyte"

// ProcolType defines the protocol type of the packet
type Version uint8

const (
	VersionBRFTv0 Version = (iota + 1) << 5 // 0010 0000
	// VersionBRFTv1                                // 0100 0000
	// 0110 0000 ...

	VersionMask uint8 = 0b11100000
)

func (v Version) Valid() bool {
	return v == VersionBRFTv0
}

// MessageType defines the type of message within the given protocol
type MessageType uint8

const (
	MessageTypeFileReserved MessageType = iota
	MessageTypeFileReq
	MessageTypeFileResp
	MessageTypeData
	MessageTypeStartTransmission
	MessageTypeClose
	MessageTypeMetaDataReq
	MessageTypeMetaDataResp

	MessageTypeMask uint8 = 0b00011111
)

var messageTypesString = map[MessageType]string{
	MessageTypeFileReq:           "File Request",
	MessageTypeFileResp:          "File Response",
	MessageTypeData:              "Data",
	MessageTypeStartTransmission: "Start Transmission",
	MessageTypeClose:             "Close",
	MessageTypeMetaDataReq:       "Metadata Request",
	MessageTypeMetaDataResp:      "Metadata Response",
}

func (mt MessageType) Name() string {
	return messageTypesString[mt]
}

// var AllMessageTypes = []MessageType{
// 	MessageTypeFileReq,
// 	MessageTypeFileResp,
// 	MessageTypeData,
// 	MessageTypeStartTransmission,
// 	MessageTypeStopTransmission,
// 	MessageTypeMetaDataReq,
// 	MessageTypeMetaDataResp,
// }

// func (mt MessageType) Valid() bool {
// 	for _, t := range AllMessageTypes {
// 		if t == mt {
// 			return true
// 		}
// 	}
// 	return false
// }

type PacketHeader struct {
	Version     Version
	MessageType MessageType
}

func NewPacketHeader(b uint8) PacketHeader {
	return PacketHeader{
		Version:     Version(b & VersionMask),
		MessageType: MessageType(b & MessageTypeMask),
	}
}

func headerForMessage(msg BRFTMessage) PacketHeader {
	p := PacketHeader{
		Version: VersionBRFTv0,
	}

	switch msg.(type) {
	case *FileReq:
		p.MessageType = MessageTypeFileReq
	case *FileResp:
		p.MessageType = MessageTypeFileResp
	case *Data:
		p.MessageType = MessageTypeData
	case *StartTransmission:
		p.MessageType = MessageTypeStartTransmission
	case *Close:
		p.MessageType = MessageTypeClose
	case *MetaReq:
		p.MessageType = MessageTypeMetaDataReq
	case *MetaResp:
		p.MessageType = MessageTypeMetaDataResp
	default:
		panic("impossible message type passed to brft header creator")
	}

	return p
}

func writePacketHeader(msg BRFTMessage, s *cryptobyte.Builder) {
	h := headerForMessage(msg)
	s.AddUint8(uint8(h.Version) | uint8(h.MessageType))
}
