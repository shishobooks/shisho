package incremental

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each method encrypts an object so that it decrypts back to itself, and
// the ciphertext differs from the plain text.
func TestSecurity_RoundTrip(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		method cryptMethod
		key    []byte
	}{
		"RC4 40-bit":  {methodRC4, bytes.Repeat([]byte{1}, 5)},
		"RC4 128-bit": {methodRC4, bytes.Repeat([]byte{2}, 16)},
		"AES-128":     {methodAESV2, bytes.Repeat([]byte{3}, 16)},
		"AES-256":     {methodAESV3, bytes.Repeat([]byte{4}, 32)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sec := &security{key: tc.key, str: tc.method, stm: tc.method, encryptMetadata: true}
			ref := Ref{Num: 7}
			in := &Stream{
				Dict: Dict{"T": String("a title"), "A": Array{String(""), Integer(3)}},
				Data: []byte("stream data"),
			}
			enc, err := sec.crypt(ref, in, sec.encrypt)
			require.NoError(t, err)
			encStream := enc.(*Stream)
			assert.NotEqual(t, in.Data, encStream.Data)
			assert.NotEqual(t, in.Dict["T"], encStream.Dict["T"])

			dec, err := sec.crypt(ref, enc, sec.decrypt)
			require.NoError(t, err)
			assert.Equal(t, in, dec)

			if tc.method != methodAESV3 {
				// The other methods derive a key per object, so another
				// object's key does not decrypt it.
				wrong, err := sec.crypt(Ref{Num: 8}, enc, sec.decrypt)
				if err == nil {
					assert.NotEqual(t, in, wrong)
				}
			}
		})
	}
}

// Cross-reference streams, the encryption dictionary, and, when the file
// says so, the metadata stream's data are never encrypted.
func TestSecurity_Exemptions(t *testing.T) {
	t.Parallel()
	encRef := Ref{Num: 9}
	sec := &security{key: bytes.Repeat([]byte{5}, 16), str: methodRC4, stm: methodRC4, dictRef: &encRef}

	xref := &Stream{Dict: Dict{"Type": Name("XRef"), "ID": String("id")}, Data: []byte{1, 2}}
	o, err := sec.crypt(Ref{Num: 3}, xref, sec.encrypt)
	require.NoError(t, err)
	assert.Equal(t, xref, o)

	encDict := Dict{"O": String("owner")}
	o, err = sec.crypt(encRef, encDict, sec.encrypt)
	require.NoError(t, err)
	assert.Equal(t, encDict, o)

	metadata := &Stream{Dict: Dict{"Type": Name("Metadata")}, Data: []byte("<xmp/>")}
	o, err = sec.crypt(Ref{Num: 4}, metadata, sec.encrypt)
	require.NoError(t, err)
	assert.Equal(t, metadata, o)

	sec.encryptMetadata = true
	o, err = sec.crypt(Ref{Num: 4}, metadata, sec.encrypt)
	require.NoError(t, err)
	assert.NotEqual(t, metadata.Data, o.(*Stream).Data)
}

// A stream that names its own crypt filter is refused instead of being
// decrypted with the wrong one.
func TestSecurity_RefusesStreamCryptFilter(t *testing.T) {
	t.Parallel()
	sec := &security{key: bytes.Repeat([]byte{6}, 16), str: methodAESV2, stm: methodAESV2, encryptMetadata: true}
	s := &Stream{Dict: Dict{"Filter": Array{Name("Crypt"), Name("FlateDecode")}}, Data: make([]byte, 32)}
	_, err := sec.crypt(Ref{Num: 2}, s, sec.decrypt)
	require.Error(t, err)
}

// AES data too short to hold more than an IV is empty, as other readers
// take it; data that is not whole blocks fails instead of panicking.
func TestSecurity_InvalidAESData(t *testing.T) {
	t.Parallel()
	sec := &security{key: bytes.Repeat([]byte{7}, 16), str: methodAESV2, stm: methodAESV2}
	for _, data := range [][]byte{nil, make([]byte, 5), make([]byte, 16)} {
		out, err := sec.decrypt(Ref{Num: 1}, methodAESV2, data)
		require.NoError(t, err, "%d bytes", len(data))
		assert.Empty(t, out, "%d bytes", len(data))
	}
	_, err := sec.decrypt(Ref{Num: 1}, methodAESV2, make([]byte, 33))
	require.Error(t, err)
}
