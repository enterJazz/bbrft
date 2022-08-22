package messages

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/cryptobyte"
)

type MetaResp struct {
	// Items are the items for all the requested files
	Items []MetaItem
}

// NewMetaResp creates a new MetaResp. The number of items will be deeduced from the items. NOTE that the maximum number
// of items in one resp is limited to 255 and that there might also be responses with 0 items
func NewMetaResp(items []MetaItem) (*MetaResp, error) {
	// TODO: we could also do the splitting of MetaResponses in here and return []*MetaResp
	numItems := len(items)
	if numItems > 255 {
		return nil, errors.New("to many items")
	}

	return &MetaResp{
		Items: items,
	}, nil
}

func (m *MetaResp) Marshal() ([]byte, error) {

	// again, make sure that there are not too many items
	numItems := len(m.Items)
	if numItems > 255 {
		return nil, errors.New("to many items")
	}

	bytes := make([]byte, 0, len(m.Items)*40) // some guess about the average item size
	for _, item := range m.Items {
		b, err := item.Marshal()
		if err != nil {
			return nil, fmt.Errorf("unable to marshal item: %s", err)
		}

		bytes = append(bytes, b...)
	}

	outLen := 1 + len(bytes)
	b := cryptobyte.NewFixedBuilder(make([]byte, 0, outLen))

	b.AddUint8(uint8(numItems))
	b.AddBytes(bytes)

	return b.Bytes()
}

func (m *MetaResp) Unmarshal(data []byte, extended bool) error {
	s := cryptobyte.String(data)

	var numItems uint8
	if !s.ReadUint8(&numItems) {
		return ErrReadFailed
	}

	// NOTE: This is designed badly, since we cannot infer here how long an item
	// will be
	items := make([]MetaItem, 0, numItems)
	for i := 0; i < int(numItems); i++ {
		item := &MetaItem{}
		err := item.UnmarshalWithString(&s, extended)
		if err != nil {
			return fmt.Errorf("unabel to unmarshal item: %s", err)
		}

		items = append(items, *item)
	}

	m.Items = items
	return nil
}