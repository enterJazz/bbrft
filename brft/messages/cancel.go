package messages

type CancelReason uint8

const (
	CancelReasonUndefined CancelReason = iota
	CancelReasonChecksumInvalid
	CancelReasonInvalidOffset
	CancelReasonNotEnoughSpace
	CancelReasonTimeout
	CancelReasonInvalidFlags
	CancelReasonResumeNoChecksum
	CancelReasonInvalidChecksumAlgo
	CancelReasonFileNotFound
	// ...
)

type Cancel struct {
	StreamID uint16
	Reason   CancelReason

	Raw []byte
}

func (m *Cancel) Marshal() []byte {

	return nil
}

func (m *Cancel) Unmarshal(data []byte) {

}
