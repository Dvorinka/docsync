package internal

import (
	"encoding/json"
	"io"
)

// WriteJSON encodes v as indented JSON — the agent-consumable contract.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
