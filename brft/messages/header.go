package messages

// ProcolType defines the protocol type of the packet
type Version uint8

const (
	VersionBRFTv0 Version = (iota + 1) << 5 // 0010 0000
	// VersionBRFTv1                                // 0100 0000
	// 0110 0000 ...

	VersionMask uint8 = 0b11100000
)

func (pt Version) Valid() bool {
	return pt == VersionBRFTv0
}

// MessageType defines the type of message within the given protocol
type MessageType uint8

const (
	MessageTypeFileReq MessageType = iota + 1
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

func (mt MessageType) String() string {
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
