// Package webhook assina e valida o corpo do webhook entre o Mock de
// Pagamento e o MS Pagamento com HMAC-SHA256.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	SignatureHeader = "X-Webhook-Signature"
	SignaturePrefix = "sha256="
	minSecretBytes  = 32
)

var ErrSecretTooShort = errors.New(
	"PAYMENT_WEBHOOK_SECRET deve ter pelo menos 32 bytes",
)

// ValidateSecret rejeita segredos curtos, como o Gateway faz com o JWT.
func ValidateSecret(secret string) error {
	if len(secret) < minSecretBytes {
		return ErrSecretTooShort
	}

	return nil
}

// Sign devolve o valor do cabeçalho X-Webhook-Signature para o corpo.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	return SignaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

// Verify compara a assinatura recebida em tempo constante.
func Verify(secret string, body []byte, signature string) bool {
	if secret == "" || !strings.HasPrefix(signature, SignaturePrefix) {
		return false
	}

	received, err := hex.DecodeString(strings.TrimPrefix(signature, SignaturePrefix))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)

	return hmac.Equal(received, mac.Sum(nil))
}
