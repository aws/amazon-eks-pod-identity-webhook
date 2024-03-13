package main

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/go-jose/go-jose/v4"
	"github.com/pkg/errors"
)

// copied from kubernetes/kubernetes#78502
func keyIDFromPublicKey(publicKey interface{}) (string, error) {
	publicKeyDERBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("failed to serialize public key to DER format: %v", err)
	}

	hasher := crypto.SHA256.New()
	hasher.Write(publicKeyDERBytes)
	publicKeyDERHash := hasher.Sum(nil)

	keyID := base64.RawURLEncoding.EncodeToString(publicKeyDERHash)

	return keyID, nil
}

type KeyResponse struct {
	Keys []jose.JSONWebKey `json:"keys"`
}

func readKey(filename string) ([]byte, error) {
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

	kid, err := keyIDFromPublicKey(pubKey)
	if err != nil {
		return response, err
	}

	var keys []jose.JSONWebKey
	keys = append(keys, jose.JSONWebKey{
		Key:       pubKey,
		KeyID:     kid,
		Algorithm: string(alg),
		Use:       "sig",
	})

	keyResponse := KeyResponse{Keys: keys}
	return json.MarshalIndent(keyResponse, "", "    ")
}

func main() {
	keyFile := flag.String("key", "", "The public key input file in PKCS8 format")
	flag.Parse()

	output, err := readKey(*keyFile)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	fmt.Println(string(output))
}
