package messages

type StopTransmissionReason uint8

const (
	StopTransmissionReasonUndefined StopTransmissionReason = iota
	StopTransmissionReasonChecksumInvalid
	StopTransmissionReasonInvalidOffset
	StopTransmissionReasonNotEnoughSpace
	StopTransmissionReasonTimeout
	// ...
)

type StopTransmission struct {
	StreamID uint16
	Reason   StopTransmissionReason

	Raw []byte
}

func (m *StopTransmission) Marshal() []byte {

	return nil
}

func (m *StopTransmission) Unmarshal(data []byte) {

}
