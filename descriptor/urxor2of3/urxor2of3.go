package urxor2of3

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"seedetcher.com/bc/ur"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/legacy"
)

var ErrInsufficientShares = errors.New("insufficient shares")

const (
	MinShares = 2
)

// SplitDescriptor returns one UR share per key for canonical sortedmulti
// descriptors when the selected UR/XOR scheme encodes one fragment per share.
// For schemes that require multiple fragments per share (for example 3-of-5),
// use SplitDescriptorForShare.
func SplitDescriptor(desc *urtypes.OutputDescriptor) ([]string, error) {
	if desc == nil {
		return nil, fmt.Errorf("descriptor is nil")
	}
	shares := make([]string, 0, len(desc.Keys))
	for i := range desc.Keys {
		frags, err := SplitDescriptorForShare(desc, i)
		if err != nil {
			return nil, err
		}
		if len(frags) != 1 {
			return nil, fmt.Errorf("scheme requires %d fragments per share for %d-of-%d", len(frags), desc.Threshold, len(desc.Keys))
		}
		shares = append(shares, frags[0])
	}
	return shares, nil
}

// SplitDescriptorForShare returns UR share fragment(s) for a specific cosigner index.
// For 2-of-3 this is one fragment. For some schemes (e.g. 3-of-5) this can be two.
func SplitDescriptorForShare(desc *urtypes.OutputDescriptor, keyIdx int) ([]string, error) {
	payload, err := canonicalURPayload(desc)
	if err != nil {
		return nil, err
	}
	if keyIdx < 0 || keyIdx >= len(desc.Keys) {
		return nil, fmt.Errorf("invalid key index %d", keyIdx)
	}
	return ur.Split(ur.Data{
		Data:      payload,
		Threshold: desc.Threshold,
		Shards:    len(desc.Keys),
	}, keyIdx), nil
}

// Combine recovers the canonical descriptor payload from 2 or more UR shares.
func Combine(shares []string) ([]byte, error) {
	if len(shares) < MinShares {
		return nil, ErrInsufficientShares
	}
	var d ur.Decoder
	seqLen := 0
	for _, s := range shares {
		typ, _, n, ok := ParseShare(s)
		if !ok || typ != "crypto-output" || n < 2 {
			return nil, fmt.Errorf("invalid ur share")
		}
		if seqLen == 0 {
			seqLen = n
		} else if seqLen != n {
			return nil, fmt.Errorf("mixed ur share set")
		}
		if err := d.Add(s); err != nil {
			return nil, err
		}
	}
	typ, payload, err := d.Result()
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, ErrInsufficientShares
	}
	if typ != "crypto-output" {
		return nil, fmt.Errorf("unexpected ur type: %s", typ)
	}
	if _, err := urtypes.Parse("crypto-output", payload); err != nil {
		return nil, fmt.Errorf("recovered payload parse failed: %w", err)
	}
	return payload, nil
}

// ParseShare extracts UR multipart metadata for a share. ok=false means invalid
// or non-multipart UR payload.
func ParseShare(raw string) (typ string, seqNum int, seqLen int, ok bool) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if !strings.HasPrefix(s, "ur:") {
		return "", 0, 0, false
	}
	parts := strings.SplitN(s[len("ur:"):], "/", 3)
	if len(parts) != 3 {
		return "", 0, 0, false
	}
	typ = parts[0]
	var n, m int
	if _, err := fmt.Sscanf(parts[1], "%d-%d", &n, &m); err != nil {
		return "", 0, 0, false
	}
	if n <= 0 || m <= 0 {
		return "", 0, 0, false
	}
	return typ, n, m, true
}

func canonicalURPayload(desc *urtypes.OutputDescriptor) ([]byte, error) {
	if desc == nil {
		return nil, fmt.Errorf("descriptor is nil")
	}
	if desc.Type != urtypes.SortedMulti {
		return nil, fmt.Errorf("ur/xor supports sortedmulti only")
	}
	canonical := canonicalizeSortedMultiDescriptor(desc)
	normalized := legacy.NormalizeDescriptorForLegacyUR(canonical)
	return normalized.Encode(), nil
}

func canonicalizeSortedMultiDescriptor(desc *urtypes.OutputDescriptor) urtypes.OutputDescriptor {
	out := *desc
	out.Keys = make([]urtypes.KeyDescriptor, len(desc.Keys))
	for i, k := range desc.Keys {
		kc := k
		kc.KeyData = append([]byte(nil), k.KeyData...)
		kc.ChainCode = append([]byte(nil), k.ChainCode...)
		kc.DerivationPath = append(urtypes.Path(nil), k.DerivationPath...)
		kc.Children = append([]urtypes.Derivation(nil), k.Children...)
		out.Keys[i] = kc
	}
	normalizeSortedMultiChildren(&out)
	sort.Slice(out.Keys, func(i, j int) bool {
		return bytes.Compare(keyDescriptorSortKey(out.Keys[i]), keyDescriptorSortKey(out.Keys[j])) < 0
	})
	return out
}

func normalizeSortedMultiChildren(desc *urtypes.OutputDescriptor) {
	const (
		changeStart = uint32(0)
		changeEnd   = uint32(1)
	)
	for i := range desc.Keys {
		if len(desc.Keys[i].Children) != 0 {
			continue
		}
		desc.Keys[i].Children = []urtypes.Derivation{
			{Type: urtypes.RangeDerivation, Index: changeStart, End: changeEnd},
			{Type: urtypes.WildcardDerivation},
		}
	}
}

func keyDescriptorSortKey(k urtypes.KeyDescriptor) []byte {
	out := make([]byte, 0, 128+len(k.KeyData)+len(k.ChainCode))
	var b4 [4]byte
	binary.BigEndian.PutUint32(b4[:], k.MasterFingerprint)
	out = append(out, b4[:]...)
	binary.BigEndian.PutUint32(b4[:], k.ParentFingerprint)
	out = append(out, b4[:]...)
	if k.Network != nil {
		out = append(out, []byte(k.Network.Name)...)
	}
	out = append(out, 0x00)

	binary.BigEndian.PutUint32(b4[:], uint32(len(k.DerivationPath)))
	out = append(out, b4[:]...)
	for _, p := range k.DerivationPath {
		binary.BigEndian.PutUint32(b4[:], p)
		out = append(out, b4[:]...)
	}

	binary.BigEndian.PutUint32(b4[:], uint32(len(k.Children)))
	out = append(out, b4[:]...)
	for _, c := range k.Children {
		out = append(out, byte(c.Type))
		if c.Hardened {
			out = append(out, 1)
		} else {
			out = append(out, 0)
		}
		binary.BigEndian.PutUint32(b4[:], c.Index)
		out = append(out, b4[:]...)
		binary.BigEndian.PutUint32(b4[:], c.End)
		out = append(out, b4[:]...)
	}

	binary.BigEndian.PutUint32(b4[:], uint32(len(k.KeyData)))
	out = append(out, b4[:]...)
	out = append(out, k.KeyData...)
	binary.BigEndian.PutUint32(b4[:], uint32(len(k.ChainCode)))
	out = append(out, b4[:]...)
	out = append(out, k.ChainCode...)
	return out
}
