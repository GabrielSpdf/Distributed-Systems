package cryptography

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// SignContent assina um conteúdo com uma chave privada RSA.
func SignContent(
	content []byte,
	privateKey *rsa.PrivateKey,
) (string, error) {
	hash := sha256.Sum256(content)

	signature, err := rsa.SignPSS(
		rand.Reader,
		privateKey,
		crypto.SHA256,
		hash[:],
		nil,
	)
	if err != nil {
		return "", fmt.Errorf(
			"erro ao assinar conteúdo: %w",
			err,
		)
	}

	return base64.StdEncoding.EncodeToString(signature), nil
}

// VerifyContent verifica uma assinatura RSA-PSS.
func VerifyContent(
	content []byte,
	signatureBase64 string,
	publicKey *rsa.PublicKey,
) error {
	signature, err := base64.StdEncoding.DecodeString(
		signatureBase64,
	)
	if err != nil {
		return fmt.Errorf(
			"assinatura não está em Base64 válido: %w",
			err,
		)
	}

	hash := sha256.Sum256(content)

	if err := rsa.VerifyPSS(
		publicKey,
		crypto.SHA256,
		hash[:],
		signature,
		nil,
	); err != nil {
		return fmt.Errorf(
			"assinatura inválida: %w",
			err,
		)
	}

	return nil
}
