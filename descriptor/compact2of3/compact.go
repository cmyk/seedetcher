package compact2of3

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"sort"

	"github.com/btcsuite/btcd/chaincfg"
	"seedetcher.com/bc/urtypes"
	"seedetcher.com/descriptor/shard"
)

const (
	Version        = 1
	Prefix         = "SE2:"
	setIDLen       = 16
	walletIDLen    = 4
	keyOrderLen    = 3
	keyRecordLen   = 73 // mfp(4) + parentfp(4) + keydata(33) + chaincode(32)
	minContainerSz = 3 + 1 + setIDLen + walletIDLen + 1 + 1 + 1 + 1 + 1 + 1 + 2 + 2 + 1 + 1 + keyOrderLen*4 + 4
)

var gfExpTable [512]byte
var gfLogTable [256]byte

func init() {
	x := byte(1)
	for i := 0; i < 255; i++ {
		gfExpTable[i] = x
		gfLogTable[x] = byte(i)
		x = gfXtime(x)
	}
	for i := 255; i < len(gfExpTable); i++ {
		gfExpTable[i] = gfExpTable[i-255]
	}
}

type Share struct {
	Version         uint8
	SetID           [setIDLen]byte
	WalletID        [walletIDLen]byte
	Script          uint8
	Network         uint8
	Threshold       uint8
	Total           uint8
	Index           uint8 // 1-based share index
	KeyIndex        uint8 // 1-based key slot carried in KeyRecord
	Path            urtypes.Path
	Children        []urtypes.Derivation
	KeyOrder        [keyOrderLen]uint32
	KeyRecord       [keyRecordLen]byte
	ParityShareData []byte
}

type SplitOptions struct {
	SetID [setIDLen]byte // optional override; zero means deterministic
}

func DeriveSetID(payload []byte) [setIDLen]byte {
	h := sha256.New()
	_, _ = h.Write([]byte("SE2-SETID-v1"))
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(payload)))
	_, _ = h.Write(l[:])
	_, _ = h.Write(payload)
	sum := h.Sum(nil)
	var out [setIDLen]byte
	copy(out[:], sum[:setIDLen])
	return out
}

func SplitDescriptor(desc *urtypes.OutputDescriptor, opts SplitOptions) ([]Share, error) {
	if desc == nil {
		return nil, errors.New("descriptor is nil")
	}
	if desc.Type != urtypes.SortedMulti || desc.Threshold != 2 || len(desc.Keys) != 3 {
		return nil, errors.New("compact SE2 supports sortedmulti 2-of-3 only")
	}
	canonical := canonicalizeSortedMultiDescriptor(desc)
	path, children, err := commonPathChildren(&canonical)
	if err != nil {
		return nil, err
	}
	rec := [3][keyRecordLen]byte{}
	keyOrder := [3]uint32{}
	for i := 0; i < 3; i++ {
		k := canonical.Keys[i]
		keyOrder[i] = k.MasterFingerprint
		r, err := keyRecordFromDescriptor(k)
		if err != nil {
			return nil, fmt.Errorf("key %d: %w", i+1, err)
		}
		rec[i] = r
	}
	parity := xor3(rec[0][:], rec[1][:], rec[2][:])
	canonicalPayload := canonical.Encode()
	setID := opts.SetID
	if setID == [setIDLen]byte{} {
		setID = DeriveSetID(canonicalPayload)
	}
	parityShares, err := shard.SplitPayloadBytes(parity, shard.SplitOptions{
		Threshold: 2,
		Total:     3,
		SetID:     setID,
	})
	if err != nil {
		return nil, fmt.Errorf("split parity: %w", err)
	}
	descriptorWalletID := shard.WalletIDForPayloadBytes(canonicalPayload)
	out := make([]Share, 0, 3)
	for i := 0; i < 3; i++ {
		out = append(out, Share{
			Version:         Version,
			SetID:           setID,
			WalletID:        descriptorWalletID,
			Script:          scriptCode(canonical.Script),
			Network:         networkCode(canonical.Keys[0].Network),
			Threshold:       2,
			Total:           3,
			Index:           uint8(i + 1),
			KeyIndex:        uint8(i + 1),
			Path:            append(urtypes.Path(nil), path...),
			Children:        append([]urtypes.Derivation(nil), children...),
			KeyOrder:        keyOrder,
			KeyRecord:       rec[i],
			ParityShareData: append([]byte(nil), parityShares[i].Data...),
		})
	}
	return out, nil
}

func CombineToDescriptorPayload(shares []Share) ([]byte, error) {
	if len(shares) == 0 {
		return nil, errors.New("no shares")
	}
	base := shares[0]
	if base.Version != Version {
		return nil, fmt.Errorf("unsupported version: %d", base.Version)
	}
	if base.Threshold != 2 || base.Total != 3 {
		return nil, errors.New("unsupported threshold/total for SE2")
	}
	if base.Script == 0 {
		return nil, errors.New("missing script")
	}
	seenShare := map[uint8]bool{}
	seenKey := map[uint8]bool{}
	uniq := make([]Share, 0, len(shares))
	for _, sh := range shares {
		if err := validateCompatible(base, sh); err != nil {
			return nil, err
		}
		if seenShare[sh.Index] {
			return nil, fmt.Errorf("duplicate share index: %d", sh.Index)
		}
		if seenKey[sh.KeyIndex] {
			return nil, fmt.Errorf("duplicate key index: %d", sh.KeyIndex)
		}
		seenShare[sh.Index] = true
		seenKey[sh.KeyIndex] = true
		uniq = append(uniq, sh)
	}
	if len(uniq) < int(base.Threshold) {
		return nil, fmt.Errorf("insufficient shares: have %d need %d", len(uniq), base.Threshold)
	}
	// Any 2 are enough by construction.
	uniq = uniq[:2]
	parts := make([][]byte, 0, len(uniq))
	x := make([]byte, 0, len(uniq))
	for _, sh := range uniq {
		parts = append(parts, append([]byte(nil), sh.ParityShareData...))
		x = append(x, sh.Index)
	}
	parity, err := combineBytes(parts, x)
	if err != nil {
		return nil, fmt.Errorf("combine parity shares: %w", err)
	}
	if len(parity) != keyRecordLen {
		return nil, fmt.Errorf("invalid parity length: %d", len(parity))
	}
	r1 := uniq[0].KeyRecord
	r2 := uniq[1].KeyRecord
	missing := xor3(r1[:], r2[:], parity)
	var records [3][keyRecordLen]byte
	records[uniq[0].KeyIndex-1] = r1
	records[uniq[1].KeyIndex-1] = r2
	var missingIndex uint8
	for i := uint8(1); i <= 3; i++ {
		if i != uniq[0].KeyIndex && i != uniq[1].KeyIndex {
			missingIndex = i
			break
		}
	}
	if missingIndex == 0 {
		return nil, errors.New("failed to determine missing key index")
	}
	copy(records[missingIndex-1][:], missing)

	net, err := decodeNetwork(base.Network)
	if err != nil {
		return nil, err
	}
	script, err := decodeScript(base.Script)
	if err != nil {
		return nil, err
	}
	keys := make([]urtypes.KeyDescriptor, 3)
	for i := 0; i < 3; i++ {
		k, err := keyRecordToDescriptor(records[i], net, append(urtypes.Path(nil), base.Path...), append([]urtypes.Derivation(nil), base.Children...))
		if err != nil {
			return nil, fmt.Errorf("decode key %d: %w", i+1, err)
		}
		if k.MasterFingerprint != base.KeyOrder[i] {
			return nil, fmt.Errorf("key order fingerprint mismatch at index %d", i+1)
		}
		keys[i] = k
	}
	desc := urtypes.OutputDescriptor{
		Script:    script,
		Threshold: 2,
		Type:      urtypes.SortedMulti,
		Keys:      keys,
	}
	if shard.WalletIDForPayloadBytes(desc.Encode()) != base.WalletID {
		return nil, errors.New("wallet_id mismatch after reconstruction")
	}
	return desc.Encode(), nil
}

func combineBytes(parts [][]byte, x []byte) ([]byte, error) {
	if len(parts) == 0 || len(parts) != len(x) {
		return nil, errors.New("invalid shard inputs")
	}
	n := len(parts[0])
	for i := range parts {
		if len(parts[i]) != n {
			return nil, errors.New("share length mismatch")
		}
	}
	out := make([]byte, n)
	xi := make([]byte, len(x))
	for b := 0; b < n; b++ {
		yi := make([]byte, len(parts))
		for i := range parts {
			yi[i] = parts[i][b]
			xi[i] = x[i]
		}
		out[b] = gfInterpolateAtZero(xi, yi)
	}
	return out, nil
}

func gfXtime(x byte) byte {
	if x&0x80 != 0 {
		return (x << 1) ^ 0x1d
	}
	return x << 1
}

func gfMul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return gfExpTable[int(gfLogTable[a])+int(gfLogTable[b])]
}

func gfInv(a byte) byte {
	if a == 0 {
		panic("gf inverse of zero")
	}
	return gfExpTable[255-int(gfLogTable[a])]
}

func gfLagrangeAtZero(x []byte, i int) byte {
	num := byte(1)
	den := byte(1)
	xi := x[i]
	for j := 0; j < len(x); j++ {
		if j == i {
			continue
		}
		xj := x[j]
		num = gfMul(num, xj)
		den = gfMul(den, xj^xi)
	}
	return gfMul(num, gfInv(den))
}

func gfInterpolateAtZero(x []byte, y []byte) byte {
	var acc byte
	for i := 0; i < len(x); i++ {
		li := gfLagrangeAtZero(x, i)
		acc ^= gfMul(y[i], li)
	}
	return acc
}

func Encode(sh Share) (string, error) {
	b, err := MarshalBinary(sh)
	if err != nil {
		return "", err
	}
	return Prefix + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func Decode(encoded string) (Share, error) {
	if len(encoded) < len(Prefix) || encoded[:len(Prefix)] != Prefix {
		return Share{}, errors.New("invalid prefix")
	}
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(encoded[len(Prefix):])
	if err != nil {
		return Share{}, fmt.Errorf("base32 decode: %w", err)
	}
	return UnmarshalBinary(b)
}

func MarshalBinary(sh Share) ([]byte, error) {
	if sh.Version != Version {
		return nil, fmt.Errorf("unsupported version: %d", sh.Version)
	}
	if sh.Threshold != 2 || sh.Total != 3 {
		return nil, errors.New("invalid threshold/total")
	}
	if sh.Index < 1 || sh.Index > 3 {
		return nil, errors.New("invalid share index")
	}
	if sh.KeyIndex < 1 || sh.KeyIndex > 3 {
		return nil, errors.New("invalid key index")
	}
	if len(sh.ParityShareData) == 0 || len(sh.ParityShareData) > 0xFFFF {
		return nil, errors.New("invalid parity share data")
	}
	if len(sh.Path) > 0xFF || len(sh.Children) > 0xFF {
		return nil, errors.New("path/children too long")
	}
	pathLen := len(sh.Path)
	childrenLen := len(sh.Children)
	headLen := 3 + 1 + setIDLen + walletIDLen + 1 + 1 + 1 + 1 + 1 + 1 + 2 + 2 + 1 + 1 + keyOrderLen*4 + pathLen*4 + childrenLen*(1+4+4+1)
	buf := make([]byte, 0, headLen+keyRecordLen+len(sh.ParityShareData)+4)
	buf = append(buf, 'S', 'E', '2')
	buf = append(buf, sh.Version)
	buf = append(buf, sh.SetID[:]...)
	buf = append(buf, sh.WalletID[:]...)
	buf = append(buf, sh.Script, sh.Network, sh.Threshold, sh.Total, sh.Index, sh.KeyIndex)
	var klen [2]byte
	binary.BigEndian.PutUint16(klen[:], keyRecordLen)
	buf = append(buf, klen[:]...)
	var plen [2]byte
	binary.BigEndian.PutUint16(plen[:], uint16(len(sh.ParityShareData)))
	buf = append(buf, plen[:]...)
	buf = append(buf, byte(len(sh.Path)))
	buf = append(buf, byte(len(sh.Children)))
	for _, fp := range sh.KeyOrder {
		var v [4]byte
		binary.BigEndian.PutUint32(v[:], fp)
		buf = append(buf, v[:]...)
	}
	for _, p := range sh.Path {
		var v [4]byte
		binary.BigEndian.PutUint32(v[:], p)
		buf = append(buf, v[:]...)
	}
	for _, c := range sh.Children {
		buf = append(buf, byte(c.Type))
		var v [4]byte
		binary.BigEndian.PutUint32(v[:], c.Index)
		buf = append(buf, v[:]...)
		binary.BigEndian.PutUint32(v[:], c.End)
		buf = append(buf, v[:]...)
		if c.Hardened {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}
	}
	buf = append(buf, sh.KeyRecord[:]...)
	buf = append(buf, sh.ParityShareData...)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(buf))
	buf = append(buf, crc[:]...)
	return buf, nil
}

func UnmarshalBinary(b []byte) (Share, error) {
	if len(b) < minContainerSz {
		return Share{}, errors.New("container too short")
	}
	if b[0] != 'S' || b[1] != 'E' || b[2] != '2' {
		return Share{}, errors.New("invalid magic")
	}
	gotCRC := binary.BigEndian.Uint32(b[len(b)-4:])
	wantCRC := crc32.ChecksumIEEE(b[:len(b)-4])
	if gotCRC != wantCRC {
		return Share{}, errors.New("checksum mismatch")
	}
	sh := Share{}
	off := 3
	sh.Version = b[off]
	off++
	if sh.Version != Version {
		return Share{}, fmt.Errorf("unsupported version: %d", sh.Version)
	}
	copy(sh.SetID[:], b[off:off+setIDLen])
	off += setIDLen
	copy(sh.WalletID[:], b[off:off+walletIDLen])
	off += walletIDLen
	sh.Script = b[off]
	off++
	sh.Network = b[off]
	off++
	sh.Threshold = b[off]
	off++
	sh.Total = b[off]
	off++
	sh.Index = b[off]
	off++
	sh.KeyIndex = b[off]
	off++
	keyLen := int(binary.BigEndian.Uint16(b[off : off+2]))
	off += 2
	parityLen := int(binary.BigEndian.Uint16(b[off : off+2]))
	off += 2
	if keyLen != keyRecordLen || parityLen <= 0 {
		return Share{}, errors.New("invalid key/parity lengths")
	}
	pathLen := int(b[off])
	off++
	childrenLen := int(b[off])
	off++
	for i := 0; i < keyOrderLen; i++ {
		sh.KeyOrder[i] = binary.BigEndian.Uint32(b[off : off+4])
		off += 4
	}
	if off+pathLen*4 > len(b)-4 {
		return Share{}, errors.New("invalid path length")
	}
	sh.Path = make(urtypes.Path, pathLen)
	for i := 0; i < pathLen; i++ {
		sh.Path[i] = binary.BigEndian.Uint32(b[off : off+4])
		off += 4
	}
	if off+childrenLen*(1+4+4+1) > len(b)-4 {
		return Share{}, errors.New("invalid children length")
	}
	sh.Children = make([]urtypes.Derivation, childrenLen)
	for i := 0; i < childrenLen; i++ {
		sh.Children[i].Type = urtypes.DerivationType(b[off])
		off++
		sh.Children[i].Index = binary.BigEndian.Uint32(b[off : off+4])
		off += 4
		sh.Children[i].End = binary.BigEndian.Uint32(b[off : off+4])
		off += 4
		sh.Children[i].Hardened = b[off] == 1
		off++
	}
	if off+keyRecordLen+parityLen+4 != len(b) {
		return Share{}, errors.New("invalid trailing lengths")
	}
	copy(sh.KeyRecord[:], b[off:off+keyRecordLen])
	off += keyRecordLen
	sh.ParityShareData = append([]byte(nil), b[off:off+parityLen]...)
	if sh.Threshold != 2 || sh.Total != 3 || sh.Index < 1 || sh.Index > 3 || sh.KeyIndex < 1 || sh.KeyIndex > 3 {
		return Share{}, errors.New("invalid threshold/total/index fields")
	}
	return sh, nil
}

func validateCompatible(base, sh Share) error {
	if sh.Version != base.Version {
		return errors.New("share version mismatch")
	}
	if sh.SetID != base.SetID {
		return errors.New("set_id mismatch")
	}
	if sh.WalletID != base.WalletID {
		return errors.New("wallet_id mismatch")
	}
	if sh.Script != base.Script || sh.Network != base.Network {
		return errors.New("script/network mismatch")
	}
	if sh.Threshold != base.Threshold || sh.Total != base.Total {
		return errors.New("threshold/total mismatch")
	}
	if len(sh.Path) != len(base.Path) || len(sh.Children) != len(base.Children) {
		return errors.New("path/children mismatch")
	}
	for i := range sh.Path {
		if sh.Path[i] != base.Path[i] {
			return errors.New("path mismatch")
		}
	}
	for i := range sh.Children {
		a, b := sh.Children[i], base.Children[i]
		if a.Type != b.Type || a.Index != b.Index || a.End != b.End || a.Hardened != b.Hardened {
			return errors.New("children mismatch")
		}
	}
	if sh.KeyOrder != base.KeyOrder {
		return errors.New("key order mismatch")
	}
	if len(sh.ParityShareData) != len(base.ParityShareData) {
		return errors.New("parity share length mismatch")
	}
	return nil
}

func commonPathChildren(desc *urtypes.OutputDescriptor) (urtypes.Path, []urtypes.Derivation, error) {
	path := append(urtypes.Path(nil), desc.Keys[0].DerivationPath...)
	children := append([]urtypes.Derivation(nil), desc.Keys[0].Children...)
	for i := 1; i < len(desc.Keys); i++ {
		if len(desc.Keys[i].DerivationPath) != len(path) {
			return nil, nil, errors.New("keys have different derivation paths")
		}
		for j := range path {
			if desc.Keys[i].DerivationPath[j] != path[j] {
				return nil, nil, errors.New("keys have different derivation paths")
			}
		}
		if len(desc.Keys[i].Children) != len(children) {
			return nil, nil, errors.New("keys have different children paths")
		}
		for j := range children {
			a, b := desc.Keys[i].Children[j], children[j]
			if a.Type != b.Type || a.Index != b.Index || a.End != b.End || a.Hardened != b.Hardened {
				return nil, nil, errors.New("keys have different children paths")
			}
		}
	}
	return path, children, nil
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

func keyRecordFromDescriptor(k urtypes.KeyDescriptor) ([keyRecordLen]byte, error) {
	if len(k.KeyData) != 33 || len(k.ChainCode) != 32 {
		return [keyRecordLen]byte{}, errors.New("invalid key data/chain code length")
	}
	var out [keyRecordLen]byte
	binary.BigEndian.PutUint32(out[0:4], k.MasterFingerprint)
	binary.BigEndian.PutUint32(out[4:8], k.ParentFingerprint)
	copy(out[8:41], k.KeyData)
	copy(out[41:73], k.ChainCode)
	return out, nil
}

func keyRecordToDescriptor(rec [keyRecordLen]byte, net *chaincfg.Params, path urtypes.Path, children []urtypes.Derivation) (urtypes.KeyDescriptor, error) {
	return urtypes.KeyDescriptor{
		Network:           net,
		MasterFingerprint: binary.BigEndian.Uint32(rec[0:4]),
		ParentFingerprint: binary.BigEndian.Uint32(rec[4:8]),
		KeyData:           append([]byte(nil), rec[8:41]...),
		ChainCode:         append([]byte(nil), rec[41:73]...),
		DerivationPath:    append(urtypes.Path(nil), path...),
		Children:          append([]urtypes.Derivation(nil), children...),
	}, nil
}

func xor3(a, b, c []byte) []byte {
	n := len(a)
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		out[i] = a[i] ^ b[i] ^ c[i]
	}
	return out
}

func scriptCode(s urtypes.Script) uint8 {
	switch s {
	case urtypes.P2WSH:
		return 1
	case urtypes.P2SH_P2WSH:
		return 2
	default:
		return 0
	}
}

func decodeScript(code uint8) (urtypes.Script, error) {
	switch code {
	case 1:
		return urtypes.P2WSH, nil
	case 2:
		return urtypes.P2SH_P2WSH, nil
	default:
		return urtypes.UnknownScript, fmt.Errorf("unsupported script code: %d", code)
	}
}

func networkCode(net *chaincfg.Params) uint8 {
	if net == nil {
		return 0
	}
	if net.Name == "mainnet" {
		return 1
	}
	return 2
}

func decodeNetwork(code uint8) (*chaincfg.Params, error) {
	switch code {
	case 1:
		return &chaincfg.MainNetParams, nil
	case 2:
		return &chaincfg.TestNet3Params, nil
	default:
		return nil, fmt.Errorf("unsupported network code: %d", code)
	}
}
