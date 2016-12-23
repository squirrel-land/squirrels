package common

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"strings"
)

func GenerateKey(seed string) ([]byte, error) {

	var r io.Reader
	if seed == "" {
		r = rand.Reader
	} else {
		r = NewDetermRand([]byte(seed))
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), r)
	if err != nil {
		return nil, err
	}
	b, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("Unable to marshal ECDSA private key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b}), nil
}

func FingerprintKey(k ssh.PublicKey) string {
	bytes := md5.Sum(k.Marshal())
	strbytes := make([]string, len(bytes))
	for i, b := range bytes {
		strbytes[i] = fmt.Sprintf("%02x", b)
	}
	return strings.Join(strbytes, ":")
}

func ConnectStreams(chans <-chan ssh.NewChannel) {

	for ch := range chans {

		addr := string(ch.ExtraData())

		stream, reqs, err := ch.Accept()
		if err != nil {
			fmt.Println("Failed to accept stream: %s", err)
			continue
		}

		go ssh.DiscardRequests(reqs)
		go handleStream(stream, addr)
	}
}

func handleStream(src io.ReadWriteCloser, remote string) {

	dst, err := net.Dial("tcp", remote)
	if err != nil {
		fmt.Printf("%s", err)
		src.Close()
		return
	}

	fmt.Printf("Open")
	s, r := Pipe(src, dst)
	fmt.Printf("Close (sent %d received %d)", s, r)
}
