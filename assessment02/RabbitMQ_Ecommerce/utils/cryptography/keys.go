package cryptography

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const RSAKeySize = 2048

// GenerateKeyPair gera e salva um novo par de chaves RSA.
func GenerateKeyPair(directory string) error {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf(
			"erro ao criar diretório de chaves %s: %w",
			directory,
			err,
		)
	}

	privateKey, err := rsa.GenerateKey(
		rand.Reader,
		RSAKeySize,
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao gerar chave privada RSA: %w",
			err,
		)
	}

	if err := privateKey.Validate(); err != nil {
		return fmt.Errorf(
			"chave privada RSA inválida: %w",
			err,
		)
	}

	privateKeyPath := filepath.Join(
		directory,
		"private.pem",
	)

	publicKeyPath := filepath.Join(
		directory,
		"public.pem",
	)

	if err := verifyKeyFilesDoNotExist(
		privateKeyPath,
		publicKeyPath,
	); err != nil {
		return err
	}

	if err := savePrivateKey(
		privateKeyPath,
		privateKey,
	); err != nil {
		return err
	}

	if err := savePublicKey(
		publicKeyPath,
		&privateKey.PublicKey,
	); err != nil {
		return err
	}

	return nil
}

func verifyKeyFilesDoNotExist(paths ...string) error {
	for _, path := range paths {
		_, err := os.Stat(path)

		if err == nil {
			return fmt.Errorf(
				"arquivo de chave já existe: %s",
				path,
			)
		}

		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf(
				"erro ao verificar arquivo %s: %w",
				path,
				err,
			)
		}
	}

	return nil
}

func savePrivateKey(
	filePath string,
	privateKey *rsa.PrivateKey,
) error {
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(
		privateKey,
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao converter chave privada: %w",
			err,
		)
	}

	privateKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privateKeyBytes,
		},
	)

	if privateKeyPEM == nil {
		return fmt.Errorf(
			"erro ao codificar chave privada em PEM",
		)
	}

	if err := os.WriteFile(
		filePath,
		privateKeyPEM,
		0600,
	); err != nil {
		return fmt.Errorf(
			"erro ao salvar chave privada em %s: %w",
			filePath,
			err,
		)
	}

	return nil
}

func savePublicKey(
	filePath string,
	publicKey *rsa.PublicKey,
) error {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(
		publicKey,
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao converter chave pública: %w",
			err,
		)
	}

	publicKeyPEM := pem.EncodeToMemory(
		&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: publicKeyBytes,
		},
	)

	if publicKeyPEM == nil {
		return fmt.Errorf(
			"erro ao codificar chave pública em PEM",
		)
	}

	if err := os.WriteFile(
		filePath,
		publicKeyPEM,
		0644,
	); err != nil {
		return fmt.Errorf(
			"erro ao salvar chave pública em %s: %w",
			filePath,
			err,
		)
	}

	return nil
}

// LoadPrivateKey carrega uma chave privada RSA armazenada em PEM.
func LoadPrivateKey(filePath string) (*rsa.PrivateKey, error) {
	keyData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao ler chave privada %s: %w",
			filePath,
			err,
		)
	}

	pemBlock, _ := pem.Decode(keyData)
	if pemBlock == nil {
		return nil, fmt.Errorf(
			"arquivo %s não contém um bloco PEM válido",
			filePath,
		)
	}

	parsedKey, err := x509.ParsePKCS8PrivateKey(
		pemBlock.Bytes,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao interpretar chave privada %s: %w",
			filePath,
			err,
		)
	}

	privateKey, ok := parsedKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf(
			"chave contida em %s não é uma chave RSA",
			filePath,
		)
	}

	return privateKey, nil
}

// LoadPublicKey carrega uma chave pública RSA armazenada em PEM.
func LoadPublicKey(filePath string) (*rsa.PublicKey, error) {
	keyData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao ler chave pública %s: %w",
			filePath,
			err,
		)
	}

	pemBlock, _ := pem.Decode(keyData)
	if pemBlock == nil {
		return nil, fmt.Errorf(
			"arquivo %s não contém um bloco PEM válido",
			filePath,
		)
	}

	parsedKey, err := x509.ParsePKIXPublicKey(
		pemBlock.Bytes,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao interpretar chave pública %s: %w",
			filePath,
			err,
		)
	}

	publicKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf(
			"chave contida em %s não é uma chave RSA",
			filePath,
		)
	}

	return publicKey, nil
}

type PublicKeyRegistry map[string]*rsa.PublicKey

func LoadPublicKeyRegistry(
	keyPaths map[string]string,
) (PublicKeyRegistry, error) {
	registry := make(
		PublicKeyRegistry,
		len(keyPaths),
	)

	for producer, keyPath := range keyPaths {
		publicKey, err := LoadPublicKey(keyPath)
		if err != nil {
			return nil, fmt.Errorf(
				"erro ao carregar chave pública de %s: %w",
				producer,
				err,
			)
		}

		registry[producer] = publicKey
	}

	return registry, nil
}
