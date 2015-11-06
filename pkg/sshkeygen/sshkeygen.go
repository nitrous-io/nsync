package sshkeygen

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"

	"golang.org/x/crypto/ssh"
)

var (
	ErrInvalidPrivateKey = errors.New("invalid private key")
)

func Generate(bits int, name string) (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return "", "", err
	}

	privateKeyDer := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privateKeyDer,
	}
	privateKeyPem := string(pem.EncodeToMemory(&privateKeyBlock))

	publicKeyStr, err := generatePubKey(privateKey, name)
	if err != nil {
		return "", "", err
	}

	return privateKeyPem, publicKeyStr, nil
}

func GeneratePubKeyUsingPrivKeyPEM(privKey []byte, name string) (string, error) {
	p, _ := pem.Decode(privKey)

	if p.Type != "RSA PRIVATE KEY" {
		return "", ErrInvalidPrivateKey
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(p.Bytes)
	if err != nil {
		return "", err
	}

	return generatePubKey(privateKey, name)
}

func generatePubKey(privateKey *rsa.PrivateKey, name string) (string, error) {
	publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", err
	}

	return strings.Replace(string(ssh.MarshalAuthorizedKey(publicKey)), "\n", " "+name, -1), nil
}
