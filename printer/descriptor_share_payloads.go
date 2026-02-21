package printer

import (
	"fmt"
	"math"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/shard"
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
	if desc.Type == urtypes.SortedMulti && threshold >= 2 && supportsURXORScheme(threshold, totalShares) {
		if payloads, err := urxor2of3.SplitDescriptorForShare(desc, keyIdx); err == nil {
			return payloads, nil
		}
	}
	if threshold > math.MaxUint8 {
		return nil, fmt.Errorf("descriptor threshold too large: %d", threshold)
	}
	if totalShares > math.MaxUint8 {
		return nil, fmt.Errorf("descriptor share count too large: %d", totalShares)
	}
	payload := desc.Encode()
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty descriptor payload")
	}
	opts := shard.SplitOptions{
		Threshold: uint8(threshold),
		Total:     uint8(totalShares),
	}
	if setID, ok := forcedDescriptorShardSetID(); ok {
		opts.SetID = setID
	}
	shares, err := shard.SplitPayloadBytes(payload, opts)
	if err != nil {
		return nil, fmt.Errorf("split descriptor payload: %w", err)
	}
	for i, sh := range shares {
		if int(sh.Index) == keyIdx+1 {
			enc, err := shard.Encode(sh)
			if err != nil {
				return nil, fmt.Errorf("encode share %d: %w", i+1, err)
			}
			return []string{enc}, nil
		}
	}
	return nil, fmt.Errorf("share %d not found", keyIdx+1)
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

func supportsURXORScheme(threshold, totalShares int) bool {
	switch {
	case totalShares-threshold <= 1:
		return true
	case totalShares == 4 && threshold == 2:
		return true
	case totalShares == 5 && threshold == 3:
		return true
	default:
		return false
	}
}
