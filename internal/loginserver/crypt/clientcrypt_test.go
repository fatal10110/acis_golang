package crypt

import (
	"bytes"
	"testing"
)

func TestDecryptFirstRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef")
	for _, n := range []int{1, 7, 8, 9, 186} {
		server, err := NewLoginCrypt(key)
		if err != nil {
			t.Fatal(err)
		}
		payload := testPattern(n)
		buf := server.Encrypt(payload)
		if err := DecryptFirst(buf); err != nil {
			t.Fatalf("len %d: DecryptFirst: %v", n, err)
		}
		if !bytes.Equal(buf[:n], payload) {
			t.Fatalf("len %d: decrypted %x, want %x", n, buf[:n], payload)
		}
	}
	if err := DecryptFirst(make([]byte, 12)); err == nil {
		t.Fatal("DecryptFirst accepted a 12-byte packet")
	}
}

func TestClientLoginCryptRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef")
	server, err := NewLoginCrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClientLoginCrypt(key)
	if err != nil {
		t.Fatal(err)
	}
	_ = server.Encrypt([]byte{0}) // the server's Init consumes the first-packet path

	up := testPattern(21)
	buf := client.Encrypt(up)
	if err := server.Decrypt(buf); err != nil {
		t.Fatalf("server Decrypt of client packet: %v", err)
	}
	if !bytes.Equal(buf[:len(up)], up) {
		t.Fatalf("server read %x, want %x", buf[:len(up)], up)
	}

	down := testPattern(13)
	buf = server.Encrypt(down)
	if err := client.Decrypt(buf); err != nil {
		t.Fatalf("client Decrypt of server packet: %v", err)
	}
	if !bytes.Equal(buf[:len(down)], down) {
		t.Fatalf("client read %x, want %x", buf[:len(down)], down)
	}
}
