package accountinganalysis

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func DecodeProposal(raw []byte) (Proposal, error) {
	var proposal Proposal
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return Proposal{}, fmt.Errorf("%w: %v", ErrInvalidProposal, err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return Proposal{}, fmt.Errorf("%w: trailing JSON", ErrInvalidProposal)
	}
	return proposal, nil
}
