package webhook

import (
	"strings"
	"testing"
)

const testSecret = "segredo-de-teste-com-mais-de-32-bytes"

func TestSignProducesPrefixedHexDigest(t *testing.T) {
	signature := Sign(testSecret, []byte(`{"status":"APPROVED"}`))

	if !strings.HasPrefix(signature, SignaturePrefix) {
		t.Fatalf("assinatura sem prefixo: %s", signature)
	}

	if len(signature) != len(SignaturePrefix)+64 {
		t.Fatalf("tamanho inesperado: %d", len(signature))
	}

	if Sign(testSecret, []byte(`{"status":"APPROVED"}`)) != signature {
		t.Fatal("assinatura deve ser determinística")
	}
}

func TestVerifyAcceptsValidSignature(t *testing.T) {
	body := []byte(`{"status":"APPROVED"}`)

	if !Verify(testSecret, body, Sign(testSecret, body)) {
		t.Fatal("assinatura válida rejeitada")
	}
}

func TestVerifyRejectsInvalidSignatures(t *testing.T) {
	body := []byte(`{"status":"APPROVED"}`)
	valid := Sign(testSecret, body)

	cases := map[string]struct {
		secret    string
		body      []byte
		signature string
	}{
		"corpo alterado": {testSecret, []byte(`{"status":"REFUSED"}`), valid},
		"segredo errado": {"outro-segredo-com-mais-de-32-bytes!!", body, valid},
		"sem prefixo":    {testSecret, body, strings.TrimPrefix(valid, SignaturePrefix)},
		"vazia":          {testSecret, body, ""},
		"hex inválido":   {testSecret, body, SignaturePrefix + "zz"},
		"segredo vazio":  {"", body, Sign("", body)},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if Verify(testCase.secret, testCase.body, testCase.signature) {
				t.Fatal("assinatura inválida aceita")
			}
		})
	}
}

func TestValidateSecretRequiresMinimumLength(t *testing.T) {
	if err := ValidateSecret("curto"); err == nil {
		t.Fatal("segredo curto aceito")
	}

	if err := ValidateSecret(testSecret); err != nil {
		t.Fatalf("segredo válido rejeitado: %v", err)
	}
}
