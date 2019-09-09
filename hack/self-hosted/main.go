package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	jose "gopkg.in/square/go-jose.v2"
)

type KeyResponse struct {
	Keys []jose.JSONWebKey `json:"keys"`
}

func readKey(keyID, filename string) ([]byte, error) {
	var response []byte
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return response, errors.WithMessage(err, "error reading file")
	}

	block, _ := pem.Decode(content)
	if block == nil {
		return response, errors.Errorf("Error decoding PEM file %s", filename)
	}

	pubKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return response, errors.Wrapf(err, "Error parsing key content of %s", filename)
	}
	switch pubKey.(type) {
	case *rsa.PublicKey:
	default:
		return response, errors.New("Public key was not RSA")
	}

	var alg jose.SignatureAlgorithm
	switch pubKey.(type) {
	case *rsa.PublicKey:
		alg = jose.RS256
	default:
		return response, fmt.Errorf("invalid public key type %T, must be *rsa.PrivateKey", pubKey)
	}

	var keys []jose.JSONWebKey
	keys = append(keys, jose.JSONWebKey{
		Key:       pubKey,
		KeyID:     keyID,
		Algorithm: string(alg),
		Use:       "sig",
	})

	keyResponse := KeyResponse{Keys: keys}
	return json.MarshalIndent(keyResponse, "", "    ")
}

func main() {
	kid := flag.String("kid", "", "The Key ID")
	keyFile := flag.String("key", "", "The public key input file in PKCS8 format")
	flag.Parse()

	output, err := readKey(*kid, *keyFile)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	fmt.Println(string(output))
}
