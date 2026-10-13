package incremental

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // the PDF standard security handler is defined with MD5
	"crypto/rand"
	"crypto/rc4" //nolint:gosec // the PDF standard security handler is defined with RC4
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"hash"

	"github.com/pkg/errors"
)

// cryptMethod is how strings or streams are encrypted.
type cryptMethod int

const (
	methodNone cryptMethod = iota
	methodRC4
	methodAESV2
	methodAESV3
)

// passwordPad pads a password to 32 bytes for revisions 2 to 4.
var passwordPad = []byte{
	0x28, 0xbf, 0x4e, 0x5e, 0x4e, 0x75, 0x8a, 0x41, 0x64, 0x00, 0x4e, 0x56, 0xff, 0xfa, 0x01, 0x08,
	0x2e, 0x2e, 0x00, 0xb6, 0xd0, 0x68, 0x3e, 0x80, 0x2f, 0x0c, 0xa9, 0xfe, 0x64, 0x53, 0x69, 0x7a,
}

// security decrypts the objects of a PDF encrypted with the standard
// security handler, and encrypts the objects of an update the same way.
// It is opened with the empty user password, so it only exists for a
// permission-restricted PDF: one that opens without a password.
type security struct {
	key []byte
	str cryptMethod
	stm cryptMethod
	// encryptMetadata is false when the document metadata stream is
	// stored unencrypted.
	encryptMetadata bool
	// dictRef is the encryption dictionary when it is an indirect object;
	// it is never encrypted.
	dictRef *Ref
}

// openSecurity authenticates the empty user password against the
// encryption dictionary enc and derives the file's key.
func (d *Document) openSecurity(enc Object) (*security, error) {
	o, err := d.Resolve(enc)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the encryption dictionary")
	}
	dict, ok := o.(Dict)
	if !ok {
		return nil, errors.New("the encryption dictionary is not a dictionary")
	}
	if f, _ := dict["Filter"].(Name); f != "Standard" {
		return nil, errors.Errorf("unsupported security handler %q", f)
	}
	v, _ := dict["V"].(Integer)
	r, _ := dict["R"].(Integer)
	sec := &security{encryptMetadata: true}
	// /EncryptMetadata is only defined from version 4.
	if b, ok := dict["EncryptMetadata"].(Bool); ok && v >= 4 {
		sec.encryptMetadata = bool(b)
	}
	if ref, ok := enc.(Ref); ok {
		sec.dictRef = &ref
	}

	switch v {
	case 1, 2:
		sec.str, sec.stm = methodRC4, methodRC4
	case 4, 5:
		if sec.str, err = cryptFilter(dict, "StrF"); err != nil {
			return nil, err
		}
		if sec.stm, err = cryptFilter(dict, "StmF"); err != nil {
			return nil, err
		}
	default:
		return nil, errors.Errorf("unsupported encryption version %d", v)
	}

	ownerKey, _ := dict["O"].(String)
	userKey, _ := dict["U"].(String)
	switch {
	case r >= 2 && r <= 4:
		for _, m := range []cryptMethod{sec.str, sec.stm} {
			if m == methodAESV3 {
				return nil, errors.Errorf("AES-256 is not defined for revision %d", r)
			}
		}
		p, ok := dict["P"].(Integer)
		if !ok {
			return nil, errors.New("the encryption dictionary has no /P")
		}
		var id []byte
		if ids, ok := d.trailer["ID"].(Array); ok && len(ids) > 0 {
			id, _ = ids[0].(String)
		}
		// /P is a 32-bit field, written signed or unsigned.
		perms := uint32(p) //nolint:gosec // truncating to 32 bits is intended
		sec.key, err = userKeyR4(int(r), keyLength(dict, int(v), int(r)), ownerKey, userKey, perms, id, sec.encryptMetadata)
	case r == 5 || r == 6:
		for _, m := range []cryptMethod{sec.str, sec.stm} {
			if m == methodRC4 || m == methodAESV2 {
				return nil, errors.Errorf("only AES-256 is defined for revision %d", r)
			}
		}
		ue, _ := dict["UE"].(String)
		sec.key, err = userKeyR6(int(r), userKey, ue)
	default:
		return nil, errors.Errorf("unsupported security handler revision %d", r)
	}
	if err != nil {
		return nil, err
	}
	return sec, nil
}

// cryptFilter returns the method of the crypt filter the encryption
// dictionary names under key (/StrF or /StmF).
func cryptFilter(dict Dict, key Name) (cryptMethod, error) {
	name, _ := dict[key].(Name)
	if name == "" || name == "Identity" {
		return methodNone, nil
	}
	filters, _ := dict["CF"].(Dict)
	f, ok := filters[name].(Dict)
	if !ok {
		return 0, errors.Errorf("crypt filter %q is not defined", name)
	}
	switch cfm, _ := f["CFM"].(Name); cfm {
	case "", "None":
		return methodNone, nil
	case "V2":
		return methodRC4, nil
	case "AESV2":
		return methodAESV2, nil
	case "AESV3":
		return methodAESV3, nil
	default:
		return 0, errors.Errorf("unsupported crypt filter method %q", cfm)
	}
}

// keyLength returns the length in bytes of the key for revisions 2 to 4.
func keyLength(dict Dict, v, r int) int {
	if r == 2 {
		return 5
	}
	bits := 40
	if l, ok := dict["Length"].(Integer); ok {
		bits = int(l)
	} else if v == 4 {
		bits = 128
	}
	if v == 4 {
		// The crypt filter's length is in bytes or, from some writers, in
		// bits.
		// Either filter can be /Identity, so use the one that is not.
		filters, _ := dict["CF"].(Dict)
		name, _ := dict["StmF"].(Name)
		if name == "" || name == "Identity" {
			name, _ = dict["StrF"].(Name)
		}
		if f, ok := filters[name].(Dict); ok {
			if l, ok := f["Length"].(Integer); ok && l > 0 {
				bits = int(l)
				if l <= 16 {
					bits = int(l) * 8
				}
			}
			if cfm, _ := f["CFM"].(Name); cfm == "AESV2" {
				bits = 128
			}
		}
	}
	if bits < 40 || bits > 128 || bits%8 != 0 {
		bits = 128
	}
	return bits / 8
}

// userKeyR4 derives the key for revisions 2 to 4 from the empty user
// password (Algorithm 2 of ISO 32000-2) and checks it against /U
// (Algorithms 4 and 5).
func userKeyR4(r, n int, ownerKey, userKey []byte, p uint32, id []byte, encryptMetadata bool) ([]byte, error) {
	if len(ownerKey) < 32 || len(userKey) < 32 {
		return nil, errors.New("/O or /U is too short")
	}
	h := md5.New() //nolint:gosec // required by the specification
	h.Write(passwordPad)
	h.Write(ownerKey[:32])
	_ = binary.Write(h, binary.LittleEndian, p)
	h.Write(id)
	if r >= 4 && !encryptMetadata {
		h.Write([]byte{0xff, 0xff, 0xff, 0xff})
	}
	key := h.Sum(nil)
	if r >= 3 {
		for i := 0; i < 50; i++ {
			sum := md5.Sum(key[:n]) //nolint:gosec // required by the specification
			key = sum[:]
		}
	}
	key = key[:n]

	var check, want []byte
	if r == 2 {
		check = rc4Crypt(key, passwordPad)
		want = userKey[:32]
	} else {
		h := md5.New() //nolint:gosec // required by the specification
		h.Write(passwordPad)
		h.Write(id)
		check = rc4Crypt(key, h.Sum(nil))
		xored := make([]byte, len(key))
		for i := 1; i <= 19; i++ {
			for j := range key {
				xored[j] = key[j] ^ byte(i)
			}
			check = rc4Crypt(xored, check)
		}
		want = userKey[:16]
	}
	if !bytes.Equal(check, want) {
		return nil, errors.New("the PDF needs a password to open")
	}
	return key, nil
}

// userKeyR6 derives the key for revisions 5 and 6 (AES-256) from the empty
// user password and checks it against /U (Algorithms 2.A and 11 of ISO
// 32000-2).
func userKeyR6(r int, userKey, ue []byte) ([]byte, error) {
	if len(userKey) < 48 || len(ue) < 32 {
		return nil, errors.New("/U or /UE is too short")
	}
	if !bytes.Equal(hashR6(r, userKey[32:40]), userKey[:32]) {
		return nil, errors.New("the PDF needs a password to open")
	}
	block, err := aes.NewCipher(hashR6(r, userKey[40:48]))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	key := make([]byte, 32)
	cipher.NewCBCDecrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(key, ue[:32])
	return key, nil
}

// hashR6 hashes the empty user password with salt: plain SHA-256 for
// revision 5, Algorithm 2.B for revision 6.
func hashR6(r int, salt []byte) []byte {
	k := sha256.Sum256(salt)
	if r == 5 {
		return k[:]
	}
	key := k[:]
	var e []byte
	for i := 0; i < 64 || int(e[len(e)-1]) > i-32; i++ {
		// The password and user data are empty, so each of the 64
		// repetitions is just the key.
		k1 := bytes.Repeat(key, 64)
		// A 16-byte key never fails.
		block, _ := aes.NewCipher(key[:16])
		e = make([]byte, len(k1))
		cipher.NewCBCEncrypter(block, key[16:32]).CryptBlocks(e, k1)
		sum := 0
		for _, b := range e[:16] {
			sum += int(b)
		}
		var h hash.Hash
		switch sum % 3 {
		case 0:
			h = sha256.New()
		case 1:
			h = sha512.New384()
		default:
			h = sha512.New()
		}
		h.Write(e)
		key = h.Sum(nil)
	}
	return key[:32]
}

func rc4Crypt(key, data []byte) []byte {
	c, err := rc4.NewCipher(key) //nolint:gosec // required by the specification
	if err != nil {
		// Keys are always 5 to 16 bytes.
		panic(err)
	}
	out := make([]byte, len(data))
	c.XORKeyStream(out, data)
	return out
}

// objectKey returns the key for the strings or streams of object ref.
func (s *security) objectKey(ref Ref, m cryptMethod) []byte {
	if m == methodAESV3 {
		return s.key
	}
	h := md5.New() //nolint:gosec // required by the specification
	h.Write(s.key)
	h.Write([]byte{byte(ref.Num), byte(ref.Num >> 8), byte(ref.Num >> 16), byte(ref.Gen), byte(ref.Gen >> 8)})
	if m == methodAESV2 {
		h.Write([]byte("sAlT"))
	}
	return h.Sum(nil)[:min(len(s.key)+5, 16)]
}

func (s *security) decrypt(ref Ref, m cryptMethod, data []byte) ([]byte, error) {
	switch m {
	case methodNone:
		return data, nil
	case methodRC4:
		return rc4Crypt(s.objectKey(ref, m), data), nil
	case methodAESV2, methodAESV3:
	}
	// Some writers store an empty string as just the IV.
	if len(data) <= aes.BlockSize {
		return nil, nil
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, errors.Errorf("object %d has invalid AES data", ref.Num)
	}
	block, err := aes.NewCipher(s.objectKey(ref, m))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := make([]byte, len(data)-aes.BlockSize)
	cipher.NewCBCDecrypter(block, data[:aes.BlockSize]).CryptBlocks(out, data[aes.BlockSize:])
	// Readers keep data whose padding is invalid as it is.
	pad := int(out[len(out)-1])
	if pad < 1 || pad > aes.BlockSize {
		return out, nil
	}
	return out[:len(out)-pad], nil
}

func (s *security) encrypt(ref Ref, m cryptMethod, data []byte) ([]byte, error) {
	switch m {
	case methodNone:
		return data, nil
	case methodRC4:
		return rc4Crypt(s.objectKey(ref, m), data), nil
	case methodAESV2, methodAESV3:
	}
	block, err := aes.NewCipher(s.objectKey(ref, m))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	pad := aes.BlockSize - len(data)%aes.BlockSize
	plain := append(append([]byte{}, data...), bytes.Repeat([]byte{byte(pad)}, pad)...)
	out := make([]byte, aes.BlockSize+len(plain))
	if _, err := rand.Read(out[:aes.BlockSize]); err != nil {
		return nil, errors.WithStack(err)
	}
	cipher.NewCBCEncrypter(block, out[:aes.BlockSize]).CryptBlocks(out[aes.BlockSize:], plain)
	return out, nil
}

// crypt returns o, the value of object ref, with its strings and stream
// data passed through fn: decrypted when read, or encrypted when written.
// A cross-reference stream and the encryption dictionary are never
// encrypted, and the document metadata stream is not when the file says
// so.
func (s *security) crypt(ref Ref, o Object, fn func(Ref, cryptMethod, []byte) ([]byte, error)) (Object, error) {
	if s.dictRef != nil && *s.dictRef == ref {
		return o, nil
	}
	if st, ok := o.(*Stream); ok {
		switch t, _ := st.Dict["Type"].(Name); {
		case t == "XRef":
			return o, nil
		case t == "Metadata" && !s.encryptMetadata:
			dict, err := s.cryptValue(ref, st.Dict, fn)
			if err != nil {
				return nil, err
			}
			return &Stream{Dict: dict.(Dict), Data: st.Data}, nil
		}
		if hasCryptFilter(st.Dict["Filter"]) {
			return nil, errors.Errorf("stream %d names its own crypt filter, which is not supported", ref.Num)
		}
		dict, err := s.cryptValue(ref, st.Dict, fn)
		if err != nil {
			return nil, err
		}
		data, err := fn(ref, s.stm, st.Data)
		if err != nil {
			return nil, err
		}
		return &Stream{Dict: dict.(Dict), Data: data}, nil
	}
	return s.cryptValue(ref, o, fn)
}

func (s *security) cryptValue(ref Ref, o Object, fn func(Ref, cryptMethod, []byte) ([]byte, error)) (Object, error) {
	switch v := o.(type) {
	case String:
		b, err := fn(ref, s.str, v)
		if err != nil {
			return nil, err
		}
		return String(b), nil
	case Array:
		out := make(Array, len(v))
		for i, item := range v {
			c, err := s.cryptValue(ref, item, fn)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case Dict:
		out := make(Dict, len(v))
		for k, item := range v {
			c, err := s.cryptValue(ref, item, fn)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	}
	return o, nil
}

func hasCryptFilter(filter Object) bool {
	switch f := filter.(type) {
	case Name:
		return f == "Crypt"
	case Array:
		for _, n := range f {
			if n == Name("Crypt") {
				return true
			}
		}
	}
	return false
}
