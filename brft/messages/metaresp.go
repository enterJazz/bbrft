package messages

import (
	"errors"
	"fmt"

	"gitlab.lrz.de/bbrft/cyberbyte"
	"go.uber.org/zap"
)

// for indicating true in byte form for isExtended
const trueByte uint8 = 0x1

// for indicating false in byte form for isExtended
const falseByte uint8 = 0x0

// base length of MetaDataResp
const metaRespBaseLen int = 2

type MetaResp struct {
	// Items are the items for all the requested files
	Items []*MetaItem
	// Indicates wether this response contains an extended meta data item, i.e., an item with a checksum and file size set
	isExtended bool
}

// NewMetaResp creates a new MetaResp. The number of items will be deeduced from the items. NOTE that the maximum number
// of items in one resp is limited to 255 and that there might also be responses with 0 items
func NewMetaResp(items []*MetaItem) (*MetaResp, error) {
	// TODO: we could also do the splitting of MetaResponses in here and return []*MetaResp
	numItems := len(items)
	if numItems > 255 {
		return nil, errors.New("too many items")
	}

	// determine if resp is extended
	// a req ist extended if and only if it contains exactly 1 item and that item's checksum and file size are not null
	var isExtended bool
	if numItems == 1 && (items[0].FileSize != nil && items[0].Checksum != nil) {
		isExtended = true
	} else {
		isExtended = false
	}

	return &MetaResp{
		Items:      items,
		isExtended: isExtended,
	}, nil
}

func (m *MetaResp) baseSize() int {
	return metaRespBaseLen
}

func (m *MetaResp) Name() string {
	return "MetaResp"
}

func (m *MetaResp) Encode(l *zap.Logger) ([]byte, error) {
	// again, make sure that there are not too many items
	numItems := len(m.Items)
	if numItems > 255 {
		return nil, errors.New("to many items")
	}

	bytes := make([]byte, 0, len(m.Items)*40) // some guess about the average item size
	for _, item := range m.Items {
		b, err := item.Encode()
		if err != nil {
			return nil, fmt.Errorf("unable to marshal item: %s", err)
		}

		bytes = append(bytes, b...)
	}

	b := NewFixedBRFTMessageBuilderWithExtra(m, len(bytes))

	b.AddUint8(uint8(numItems))
	if m.isExtended {
		b.AddUint8(trueByte)
	} else {
		b.AddUint8(falseByte)
	}
	b.AddBytes(bytes)
	return b.Bytes()
}

func (m *MetaResp) Decode(l *zap.Logger, s *cyberbyte.String) error {

	var numItems uint8
	if err := s.ReadUint8(&numItems); err != nil {
		return ErrReadFailed
	}

	var isExtendedByte uint8
	if err := s.ReadUint8(&isExtendedByte); err != nil {
		return ErrReadFailed
	}
	// check if ends with 1 ; only last bit counts
	if isExtendedByte%2 == 1 {
		m.isExtended = true
	} else {
		m.isExtended = false
	}

	items := make([]*MetaItem, 0, numItems)
	for i := 0; i < int(numItems); i++ {
		item := &MetaItem{}
		err := item.UnmarshalWithString(s, m.isExtended)
		if err != nil {
			return fmt.Errorf("unable to unmarshal item: %s", err)
		}

		items = append(items, item)
	}

	m.Items = items
	return nil
}

// TODO remove
func (m *MetaResp) SetExtended(val bool) {
	m.isExtended = val
}
