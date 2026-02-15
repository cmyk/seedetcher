package shard

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"seedetcher.com/bc/urtypes"
	"seedetcher.com/nonstandard"
)

const (
	Version        = 1
	Prefix         = "SE1:"
	setIDLen       = 16
	walletIDLen    = 4
	minThreshold   = 2
	maxShareCount  = 255
	minContainerSz = 2 + 1 + setIDLen + walletIDLen + 1 + 1 + 1 + 1 + 1 + 2 + 4
)

type NetworkHint uint8

const (
	NetworkUnknown NetworkHint = iota
	NetworkMainnet
	NetworkTestnet
	NetworkSignet
	NetworkRegtest
)

type ScriptHint uint8

const (
	ScriptUnknown ScriptHint = iota
	ScriptWPKH
	ScriptWSH
	ScriptTR
	ScriptSortedMulti
)

type Share struct {
	Version   uint8
	SetID     [setIDLen]byte
	WalletID  [walletIDLen]byte
	Network   NetworkHint
	Script    ScriptHint
	Threshold uint8
	Total     uint8
	Index     uint8 // 1-based
	Data      []byte
}

type SplitOptions struct {
	Threshold uint8
	Total     uint8
	SetID     [setIDLen]byte // optional override; zero means deterministic from payload+t+n
}

var checksumRE = regexp.MustCompile(`(?i)^(.*)#([0-9a-z]{8})$`)

func CanonicalizeDescriptor(desc string) (string, error) {
	clean := strings.TrimSpace(desc)
	clean = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, clean)
	m := checksumRE.FindStringSubmatch(clean)
	if m == nil {
		return "", errors.New("descriptor must include checksum")
	}
	body := m[1]
	checksum := strings.ToLower(m[2])
	canonical := body + "#" + checksum
	if _, err := nonstandard.OutputDescriptor([]byte(canonical)); err != nil {
		return "", fmt.Errorf("invalid descriptor: %w", err)
	}
	return canonical, nil
}

func Split(descriptor string, opts SplitOptions) ([]Share, error) {
	canonical, err := CanonicalizeDescriptor(descriptor)
	if err != nil {
		return nil, err
	}
	network, script, err := inferHints(canonical)
	if err != nil {
		return nil, err
	}
	return splitPayloadBytes([]byte(canonical), opts, network, script)
}

// SplitPayload splits an arbitrary payload into sharded shares.
// This is used for transport payloads such as UR strings.
func SplitPayload(payload string, opts SplitOptions) ([]Share, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, errors.New("empty payload")
	}
	return splitPayloadBytes([]byte(payload), opts, NetworkUnknown, ScriptUnknown)
}

// SplitPayloadBytes splits arbitrary bytes into sharded shares.
func SplitPayloadBytes(payload []byte, opts SplitOptions) ([]Share, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}
	return splitPayloadBytes(payload, opts, NetworkUnknown, ScriptUnknown)
}

func splitPayloadBytes(payload []byte, opts SplitOptions, network NetworkHint, script ScriptHint) ([]Share, error) {
	if opts.Total < minThreshold || opts.Total > maxShareCount {
		return nil, fmt.Errorf("invalid total shares: %d", opts.Total)
	}
	if opts.Threshold < minThreshold || opts.Threshold > opts.Total {
		return nil, fmt.Errorf("invalid threshold: %d", opts.Threshold)
	}
	payload = normalizeSplitPayload(payload)
	setID := opts.SetID
	if setID == [setIDLen]byte{} {
		setID = DeriveSetID(payload, opts.Threshold, opts.Total)
	}
	walletID := walletIDForPayload(payload)
	splitSeed := deriveSplitSeed(payload, opts.Threshold, opts.Total)

	parts, err := splitBytes(payload, int(opts.Total), int(opts.Threshold), splitSeed)
	if err != nil {
		return nil, err
	}
	shares := make([]Share, 0, opts.Total)
	for i := 0; i < int(opts.Total); i++ {
		shares = append(shares, Share{
			Version:   Version,
			SetID:     setID,
			WalletID:  walletID,
			Network:   network,
			Script:    script,
			Threshold: opts.Threshold,
			Total:     opts.Total,
			Index:     uint8(i + 1),
			Data:      parts[i],
		})
	}
	return shares, nil
}

// DeriveSetID deterministically derives the descriptor shard set identifier from
// payload bytes and split parameters.
func DeriveSetID(payload []byte, threshold, total uint8) [setIDLen]byte {
	payload = normalizeSplitPayload(payload)
	h := sha256.New()
	_, _ = h.Write([]byte("SE1-SETID-v1"))
	_, _ = h.Write([]byte{threshold, total})
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(payload)))
	_, _ = h.Write(l[:])
	_, _ = h.Write(payload)
	sum := h.Sum(nil)
	var out [setIDLen]byte
	copy(out[:], sum[:setIDLen])
	return out
}

func deriveSplitSeed(payload []byte, threshold, total uint8) [32]byte {
	h := sha256.New()
	_, _ = h.Write([]byte("SE1-SPLIT-v1"))
	_, _ = h.Write([]byte{threshold, total})
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(payload)))
	_, _ = h.Write(l[:])
	_, _ = h.Write(payload)
	sum := h.Sum(nil)
	var out [32]byte
	copy(out[:], sum[:32])
	return out
}

func Combine(shares []Share) (string, error) {
	payload, err := combinePayload(shares)
	if err != nil {
		return "", err
	}
	canonical := string(payload)
	if _, err := CanonicalizeDescriptor(canonical); err != nil {
		return "", fmt.Errorf("reconstructed descriptor failed canonical validation: %w", err)
	}
	return canonical, nil
}

// CombinePayload reconstructs raw payload text without enforcing descriptor
// canonicalization rules.
func CombinePayload(shares []Share) (string, error) {
	payload, err := combinePayload(shares)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// CombinePayloadBytes reconstructs raw payload bytes without descriptor
// canonicalization checks.
func CombinePayloadBytes(shares []Share) ([]byte, error) {
	return combinePayload(shares)
}

func combinePayload(shares []Share) ([]byte, error) {
	if len(shares) == 0 {
		return nil, errors.New("no shares")
	}
	base := shares[0]
	if base.Version != Version {
		return nil, fmt.Errorf("unsupported version: %d", base.Version)
	}
	if base.Threshold < minThreshold || base.Total < base.Threshold {
		return nil, errors.New("invalid threshold/total in shares")
	}

	seen := make(map[uint8]bool)
	unique := make([]Share, 0, len(shares))
	for _, s := range shares {
		if err := validateShareMatch(base, s); err != nil {
			return nil, err
		}
		if seen[s.Index] {
			return nil, fmt.Errorf("duplicate share index: %d", s.Index)
		}
		seen[s.Index] = true
		unique = append(unique, s)
	}

	if len(unique) < int(base.Threshold) {
		return nil, fmt.Errorf("insufficient shares: have %d need %d", len(unique), base.Threshold)
	}

	sort.Slice(unique, func(i, j int) bool { return unique[i].Index < unique[j].Index })
	parts := make([][]byte, len(unique))
	x := make([]byte, len(unique))
	for i, s := range unique {
		parts[i] = s.Data
		x[i] = s.Index
	}
	payload, err := combineBytes(parts, x)
	if err != nil {
		return nil, err
	}
	if walletIDForPayload(payload) != base.WalletID {
		return nil, errors.New("wallet_id mismatch after reconstruction")
	}
	return payload, nil
}

func Encode(share Share) (string, error) {
	b, err := MarshalBinary(share)
	if err != nil {
		return "", err
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	return Prefix + enc, nil
}

func Decode(encoded string) (Share, error) {
	if !strings.HasPrefix(encoded, Prefix) {
		return Share{}, errors.New("invalid prefix")
	}
	payload := strings.TrimPrefix(encoded, Prefix)
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(payload))
	if err != nil {
		return Share{}, fmt.Errorf("base32 decode: %w", err)
	}
	return UnmarshalBinary(b)
}

func MarshalBinary(share Share) ([]byte, error) {
	if share.Version != Version {
		return nil, fmt.Errorf("unsupported version: %d", share.Version)
	}
	if share.Threshold < minThreshold || share.Total < share.Threshold {
		return nil, errors.New("invalid threshold/total")
	}
	if share.Index == 0 || share.Index > share.Total {
		return nil, errors.New("invalid share index")
	}
	if len(share.Data) == 0 {
		return nil, errors.New("empty share data")
	}
	if len(share.Data) > 0xFFFF {
		return nil, errors.New("share data too large")
	}

	headLen := 2 + 1 + setIDLen + walletIDLen + 1 + 1 + 1 + 1 + 1 + 2
	buf := make([]byte, 0, headLen+len(share.Data)+4)
	buf = append(buf, 'S', 'E')
	buf = append(buf, share.Version)
	buf = append(buf, share.SetID[:]...)
	buf = append(buf, share.WalletID[:]...)
	buf = append(buf, byte(share.Network))
	buf = append(buf, byte(share.Script))
	buf = append(buf, share.Threshold)
	buf = append(buf, share.Total)
	buf = append(buf, share.Index)
	var l [2]byte
	binary.BigEndian.PutUint16(l[:], uint16(len(share.Data)))
	buf = append(buf, l[:]...)
	buf = append(buf, share.Data...)
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], crc32.ChecksumIEEE(buf))
	buf = append(buf, c[:]...)
	return buf, nil
}

func UnmarshalBinary(b []byte) (Share, error) {
	if len(b) < minContainerSz {
		return Share{}, errors.New("container too short")
	}
	if b[0] != 'S' || b[1] != 'E' {
		return Share{}, errors.New("invalid magic")
	}
	gotCRC := binary.BigEndian.Uint32(b[len(b)-4:])
	wantCRC := crc32.ChecksumIEEE(b[:len(b)-4])
	if gotCRC != wantCRC {
		return Share{}, errors.New("checksum mismatch")
	}
	off := 2
	s := Share{}
	s.Version = b[off]
	off++
	if s.Version != Version {
		return Share{}, fmt.Errorf("unsupported version: %d", s.Version)
	}
	copy(s.SetID[:], b[off:off+setIDLen])
	off += setIDLen
	copy(s.WalletID[:], b[off:off+walletIDLen])
	off += walletIDLen
	s.Network = NetworkHint(b[off])
	off++
	s.Script = ScriptHint(b[off])
	off++
	s.Threshold = b[off]
	off++
	s.Total = b[off]
	off++
	s.Index = b[off]
	off++
	dataLen := int(binary.BigEndian.Uint16(b[off : off+2]))
	off += 2
	if dataLen <= 0 || off+dataLen+4 != len(b) {
		return Share{}, errors.New("invalid data length")
	}
	s.Data = append([]byte(nil), b[off:off+dataLen]...)
	if s.Threshold < minThreshold || s.Total < s.Threshold {
		return Share{}, errors.New("invalid threshold/total")
	}
	if s.Index == 0 || s.Index > s.Total {
		return Share{}, errors.New("invalid share index")
	}
	return s, nil
}

func validateShareMatch(base, s Share) error {
	if s.Version != base.Version {
		return errors.New("share version mismatch")
	}
	if s.SetID != base.SetID {
		return errors.New("set_id mismatch")
	}
	if s.WalletID != base.WalletID {
		return errors.New("wallet_id mismatch")
	}
	if s.Network != base.Network {
		return errors.New("network hint mismatch")
	}
	if s.Script != base.Script {
		return errors.New("script hint mismatch")
	}
	if s.Threshold != base.Threshold || s.Total != base.Total {
		return errors.New("threshold/total mismatch")
	}
	if len(s.Data) != len(base.Data) {
		return errors.New("share data length mismatch")
	}
	return nil
}

func walletIDForPayload(payload []byte) [walletIDLen]byte {
	h := sha256.Sum256(payload)
	var out [walletIDLen]byte
	copy(out[:], h[:walletIDLen])
	return out
}

func inferHints(canonical string) (NetworkHint, ScriptHint, error) {
	desc, err := nonstandard.OutputDescriptor([]byte(canonical))
	if err != nil {
		return NetworkUnknown, ScriptUnknown, fmt.Errorf("descriptor parse for hints: %w", err)
	}
	network := NetworkUnknown
	if len(desc.Keys) > 0 && desc.Keys[0].Network != nil {
		switch desc.Keys[0].Network.Name {
		case "mainnet":
			network = NetworkMainnet
		case "testnet3":
			network = NetworkTestnet
		case "signet":
			network = NetworkSignet
		case "regtest":
			network = NetworkRegtest
		}
	}
	script := ScriptUnknown
	s := strings.ToLower(desc.Script.String())
	switch {
	case strings.Contains(s, "p2wpkh"):
		script = ScriptWPKH
	case strings.Contains(s, "p2wsh"):
		script = ScriptWSH
	case strings.Contains(s, "p2tr"):
		script = ScriptTR
	}
	if strings.EqualFold(desc.Type.String(), "SortedMulti") {
		script = ScriptSortedMulti
	}
	return network, script, nil
}

func normalizeSplitPayload(payload []byte) []byte {
	v, err := urtypes.Parse("crypto-output", payload)
	if err != nil {
		return payload
	}
	desc, ok := v.(urtypes.OutputDescriptor)
	if !ok {
		return payload
	}
	if desc.Type == urtypes.SortedMulti && len(desc.Keys) > 1 {
		normalizeSortedMultiChildren(&desc)
		sort.Slice(desc.Keys, func(i, j int) bool {
			return bytes.Compare(keyDescriptorSortKey(desc.Keys[i]), keyDescriptorSortKey(desc.Keys[j])) < 0
		})
	}
	return desc.Encode()
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
