package crypto

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/big"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ripemd160"
)

// PublicKeys is a list of public keys.
type PublicKeys []*PublicKey

func (keys PublicKeys) Len() int      { return len(keys) }
func (keys PublicKeys) Swap(i, j int) { keys[i], keys[j] = keys[j], keys[i] }
func (keys PublicKeys) Less(i, j int) bool {
	if keys[i].X.Cmp(keys[j].X) == -1 {
		return true
	}
	if keys[i].X.Cmp(keys[j].X) == 1 {
		return false
	}
	if keys[i].X.Cmp(keys[j].X) == 0 {
		return false
	}

	return keys[i].Y.Cmp(keys[j].Y) == -1
}

// PublicKey represents a public key and provides a high level
// API around the ECPoint.
type PublicKey struct {
	ECPoint
}

// NewPublicKeyFromString return a public key created from the
// given hex string.
func NewPublicKeyFromString(s string) (*PublicKey, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}

	pubKey := new(PublicKey)
	if err := pubKey.DecodeBinary(bytes.NewReader(b)); err != nil {
		return nil, err
	}

	return pubKey, nil
}

// Bytes returns the byte array representation of the public key.
func (p *PublicKey) Bytes() []byte {
	if p.IsInfinity() {
		return []byte{0x00}
	}

	var (
		x       = p.X.Bytes()
		paddedX = append(bytes.Repeat([]byte{0x00}, 32-len(x)), x...)
		prefix  = byte(0x03)
	)

	if p.Y.Bit(0) == 0 {
		prefix = byte(0x02)
	}

	return append([]byte{prefix}, paddedX...)
}

// NewPublicKeyFromRawBytes returns a NEO PublicKey from the ASN.1 serialized keys.
func NewPublicKeyFromRawBytes(data []byte) (*PublicKey, error) {
	var (
		err    error
		pubkey interface{}
	)
	if pubkey, err = x509.ParsePKIXPublicKey(data); err != nil {
		return nil, err
	}
	pk, ok := pubkey.(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("given bytes aren't ECDSA public key")
	}
	key := PublicKey{
		ECPoint{
			X: pk.X,
			Y: pk.Y,
		},
	}
	return &key, nil
}

// DecodeBytes decodes a PublicKey from the given slice of bytes.
func (p *PublicKey) DecodeBytes(data []byte) error {
	l := len(data)

	switch prefix := data[0]; prefix {
	// Infinity
	case 0x00:
		p.ECPoint = ECPoint{}
	// Compressed public keys
	case 0x02, 0x03:
		if l < 33 {
			return errors.Errorf("bad binary size(%d)", l)
		}

		c := NewEllipticCurve()
		var err error
		p.ECPoint, err = c.Decompress(new(big.Int).SetBytes(data[1:]), uint(prefix&0x1))
		if err != nil {
			return err
		}
	case 0x04:
		if l < 66 {
			return errors.Errorf("bad binary size(%d)", l)
		}
		p.X = new(big.Int).SetBytes(data[2:34])
		p.Y = new(big.Int).SetBytes(data[34:66])
	default:
		return errors.Errorf("invalid prefix %d", prefix)
	}

	return nil
}

// DecodeBinary decodes a PublicKey from the given io.Reader.
func (p *PublicKey) DecodeBinary(r io.Reader) error {
	var prefix, size uint8

	if err := binary.Read(r, binary.LittleEndian, &prefix); err != nil {
		return err
	}

	// Infinity
	switch prefix {
	case 0x00:
		p.ECPoint = ECPoint{}
		return nil
	// Compressed public keys
	case 0x02, 0x03:
		size = 32
	case 0x04:
		size = 65
	default:
		return errors.Errorf("invalid prefix %d", prefix)
	}

	data := make([]byte, size+1) // prefix + size

	if _, err := io.ReadFull(r, data[1:]); err != nil {
		return err
	}

	data[0] = prefix

	return p.DecodeBytes(data)
}

// EncodeBinary encodes a PublicKey to the given io.Writer.
func (p *PublicKey) EncodeBinary(w io.Writer) error {
	return binary.Write(w, binary.LittleEndian, p.Bytes())
}

func (p *PublicKey) Signature() ([]byte, error) {
	b := p.Bytes()
	b = append([]byte{0x21}, b...)
	b = append(b, 0xAC)

	sha := sha256.New()
	sha.Write(b)
	hash := sha.Sum(nil)

	ripemd := ripemd160.New()
	ripemd.Reset()
	ripemd.Write(hash)
	hash = ripemd.Sum(nil)

	return hash, nil
}

func (p *PublicKey) Address() (string, error) {
	var (
		err error
		b   []byte
	)
	if b, err = p.Signature(); err != nil {
		return "", err
	}

	b = append([]byte{0x17}, b...)

	sha := sha256.New()
	sha.Write(b)
	hash := sha.Sum(nil)

	sha.Reset()
	sha.Write(hash)
	hash = sha.Sum(nil)

	b = append(b, hash[0:4]...)

	address := Base58Encode(b)
	return address, nil
}
