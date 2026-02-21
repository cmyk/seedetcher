package printer

import (
	"fmt"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/urxor2of3"
)

func descriptorShardQRPayloadsForShare(desc *urtypes.OutputDescriptor, totalShares, keyIdx int) ([]string, error) {
	if desc == nil {
		return nil, fmt.Errorf("descriptor is nil")
	}
	if totalShares <= 0 {
		return nil, fmt.Errorf("invalid share count: %d", totalShares)
	}
	if keyIdx < 0 || keyIdx >= totalShares {
		return nil, fmt.Errorf("invalid key index: %d", keyIdx)
	}
	threshold := desc.Threshold
	if threshold == 1 && totalShares == 1 {
		qr := createDescriptorQR(desc)
		if qr == "" {
			return nil, fmt.Errorf("empty descriptor QR content")
		}
		return []string{qr}, nil
	}
	if threshold < 2 || threshold > totalShares {
		return nil, fmt.Errorf("invalid descriptor threshold %d for %d shares", threshold, totalShares)
	}
	if desc.Type == urtypes.SortedMulti && threshold >= 2 && urxor2of3.SupportsScheme(threshold, totalShares) {
		if payloads, err := urxor2of3.SplitDescriptorForShare(desc, keyIdx); err == nil {
			return payloads, nil
		}
	}
	// Interoperable fallback: full descriptor UR on each plate.
	qr := createDescriptorQR(desc)
	if qr == "" {
		return nil, fmt.Errorf("empty descriptor QR content")
	}
	return []string{qr}, nil
}

func descriptorShardQRCodes(desc *urtypes.OutputDescriptor, totalShares int) ([]string, error) {
	out := make([]string, totalShares)
	for i := 0; i < totalShares; i++ {
		payloads, err := descriptorShardQRPayloadsForShare(desc, totalShares, i)
		if err != nil {
			return nil, err
		}
		if len(payloads) != 1 {
			return nil, fmt.Errorf("share %d has %d fragments (expected 1)", i+1, len(payloads))
		}
		out[i] = payloads[0]
	}
	return out, nil
}
