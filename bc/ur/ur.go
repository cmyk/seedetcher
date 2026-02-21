// Package ur implements the Uniform Resources (UR) encoding
// specified in [BCR-2020-005].
//
// [BCR-2020-005]: https://github.com/BlockchainCommons/Research/blob/master/papers/bcr-2020-005-ur.md
package ur

import (
	"errors"
	"fmt"
	"strings"

	"seedetcher.com/bc/bytewords"
	"seedetcher.com/bc/fountain"
)

type Data struct {
	Data      []byte
	Threshold int
	Shards    int
}

// Split implements SeedHammer-style fragment assignments over UR fountain
// fragments for selected m-of-n schemes.
func Split(data Data, keyIdx int) (urs []string) {
	var shares [][]int
	var seqLen int
	n, m := data.Shards, data.Threshold
	switch {
	case n-m <= 1:
		// Optimal: 1 part per share, seqLen m.
		seqLen = m
		if keyIdx < m {
			shares = [][]int{{keyIdx}}
		} else {
			all := make([]int, 0, m)
			for i := range m {
				all = append(all, i)
			}
			shares = [][]int{all}
		}
	case n == 4 && m == 2:
		// Optimal, but 2 parts per share.
		seqLen = m * 2
		switch keyIdx {
		case 0:
			shares = [][]int{{0}, {1}}
		case 1:
			shares = [][]int{{2}, {3}}
		case 2:
			shares = [][]int{{0, 2}, {1, 3}}
		case 3:
			shares = [][]int{{0, 2, 1}, {1, 3, 2}}
		}
	case n == 5 && m == 3:
		// Optimal, but 2 parts per share.
		seqLen = m * 2
		second := []int{
			n,
			(keyIdx + n - 1) % n,
			(keyIdx + 1) % n,
		}
		shares = [][]int{{keyIdx}, second}
	default:
		// Fallback: full data per share.
		seqLen = 1
		shares = [][]int{{0}}
	}
	check := fountain.Checksum(data.Data)
	for _, frag := range shares {
		seqNum := fountain.SeqNumFor(seqLen, check, frag)
		qr := strings.ToUpper(Encode("crypto-output", data.Data, seqNum, seqLen))
		urs = append(urs, qr)
	}
	return
}

func Encode(_type string, message []byte, seqNum, seqLen int) string {
	if seqLen == 1 {
		return fmt.Sprintf("ur:%s/%s", _type, bytewords.Encode(message))
	}
	data := fountain.Encode(message, seqNum, seqLen)
	return fmt.Sprintf("ur:%s/%d-%d/%s", _type, seqNum, seqLen, bytewords.Encode(data))
}

type Decoder struct {
	typ  string
	data []byte

	fountain fountain.Decoder
}

func (d *Decoder) Progress() float32 {
	if d.data != nil {
		return 1
	}
	return d.fountain.Progress()
}

func (d *Decoder) Result() (string, []byte, error) {
	if d.data != nil {
		return d.typ, d.data, nil
	}
	v, err := d.fountain.Result()
	if v == nil {
		return "", nil, err
	}
	return d.typ, v, err
}

func (d *Decoder) Add(ur string) error {
	ur = strings.ToLower(ur)
	const prefix = "ur:"
	if !strings.HasPrefix(ur, prefix) {
		return errors.New("ur: missing ur: prefix")
	}
	ur = ur[len(prefix):]
	parts := strings.SplitN(ur, "/", 3)
	if len(parts) < 2 {
		return errors.New("ur: incomplete UR")
	}
	typ := parts[0]
	if d.typ != "" && d.typ != typ {
		return errors.New("ur: incompatible fragment")
	}
	d.typ = typ
	var seqAndLen string
	var fragment string
	if len(parts) == 2 {
		fragment = parts[1]
	} else {
		seqAndLen, fragment = parts[1], parts[2]
	}
	enc, err := bytewords.Decode(fragment)
	if err != nil {
		return fmt.Errorf("ur: invalid fragment: %w", err)
	}
	if seqAndLen != "" {
		var seq, n int
		if _, err := fmt.Sscanf(seqAndLen, "%d-%d", &seq, &n); err != nil {
			return fmt.Errorf("ur: invalid sequence %q", seqAndLen)
		}
		if err := d.fountain.Add(enc); err != nil {
			return err
		}
	} else {
		d.data = enc
	}
	return nil
}
