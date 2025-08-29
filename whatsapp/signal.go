package whatsapp

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
)

const compress = true

func signalEncode(obj any) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	if compress {
		b = signalZip(b)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func signalDecode(in string, obj any) {
	b, err := base64.StdEncoding.DecodeString(in)
	if err != nil {
		panic(err)
	}
	if compress {
		b = signalUnzip(b)
	}
	if err := json.Unmarshal(b, obj); err != nil {
		panic(err)
	}
}

func signalZip(in []byte) []byte {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(in); err != nil {
		panic(err)
	}
	if err := gz.Flush(); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func signalUnzip(in []byte) []byte {
	var b bytes.Buffer
	if _, err := b.Write(in); err != nil {
		panic(err)
	}
	r, err := gzip.NewReader(&b)
	if err != nil {
		panic(err)
	}
	res, err := io.ReadAll(r)
	if err != nil {
		panic(err)
	}
	return res
}
