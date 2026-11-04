package article

import "encoding/json"

type SearchPage struct {
	IDs      []int64
	PITID    string
	LastSort []json.RawMessage
	HasMore  bool
}
