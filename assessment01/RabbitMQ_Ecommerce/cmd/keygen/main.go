package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
)

const keysDirectory = "keys"

var services = []string{
	events.ProducerPrincipal,
	events.ProducerStock,
	events.ProducerPayment,
	events.ProducerDelivery,
}

func main() {
	if err := run(); err != nil {
		log.Fatal("[ERRO] ", err)
	}

	fmt.Println(
		"[SUCESSO] Todas as chaves foram geradas e distribuídas",
	)
}

func run() error {
	if err := verifyKeysDirectoryDoesNotExist(
		keysDirectory,
	); err != nil {
		return err
	}

	temporaryDirectory, err := os.MkdirTemp(
		".",
		".keys-generation-*",
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao criar diretório temporário: %w",
			err,
		)
	}

	// Se alguma etapa falhar, remove somente a pasta temporária.
	shouldRemoveTemporaryDirectory := true

	defer func() {
		if shouldRemoveTemporaryDirectory {
			if err := os.RemoveAll(
				temporaryDirectory,
			); err != nil {
				log.Printf(
					"[AVISO] Não foi possível remover diretório temporário %s: %v",
					temporaryDirectory,
					err,
				)
			}
		}
	}()

	if err := generateServiceKeys(
		temporaryDirectory,
	); err != nil {
		return err
	}

	if err := distributePublicKeys(
		temporaryDirectory,
	); err != nil {
		return err
	}

	if err := os.Rename(
		temporaryDirectory,
		keysDirectory,
	); err != nil {
		return fmt.Errorf(
			"erro ao finalizar diretório de chaves: %w",
			err,
		)
	}

	shouldRemoveTemporaryDirectory = false

	return nil
}

func verifyKeysDirectoryDoesNotExist(
	directory string,
) error {
	_, err := os.Stat(directory)

	if err == nil {
		return fmt.Errorf(
			"o diretório %s já existe; as chaves não serão sobrescritas",
			directory,
		)
	}

	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf(
			"erro ao verificar diretório %s: %w",
			directory,
			err,
		)
	}

	return nil
}

func generateServiceKeys(
	rootDirectory string,
) error {
	for _, service := range services {
		serviceDirectory := filepath.Join(
			rootDirectory,
			service,
		)

		if err := cryptography.GenerateKeyPair(
			serviceDirectory,
		); err != nil {
			return fmt.Errorf(
				"erro ao gerar chaves de %s: %w",
				service,
				err,
			)
		}

		log.Printf(
			"[SUCESSO] Par de chaves de %s gerado",
			service,
		)
	}

	return nil
}

func distributePublicKeys(
	rootDirectory string,
) error {
	for _, consumer := range services {
		publicKeysDirectory := filepath.Join(
			rootDirectory,
			consumer,
			"public_keys",
		)

		if err := os.MkdirAll(
			publicKeysDirectory,
			0755,
		); err != nil {
			return fmt.Errorf(
				"erro ao criar pasta de chaves públicas de %s: %w",
				consumer,
				err,
			)
		}

		for _, producer := range services {
			// Cada serviço já possui sua própria public.pem.
			if producer == consumer {
				continue
			}

			sourcePath := filepath.Join(
				rootDirectory,
				producer,
				"public.pem",
			)

			destinationPath := filepath.Join(
				publicKeysDirectory,
				producer+".pem",
			)

			if err := copyPublicKey(
				sourcePath,
				destinationPath,
			); err != nil {
				return fmt.Errorf(
					"erro ao distribuir chave pública de %s para %s: %w",
					producer,
					consumer,
					err,
				)
			}
		}
	}

	return nil
}

func copyPublicKey(
	sourcePath string,
	destinationPath string,
) error {
	keyData, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf(
			"erro ao ler %s: %w",
			sourcePath,
			err,
		)
	}

	if err := os.WriteFile(
		destinationPath,
		keyData,
		0644,
	); err != nil {
		return fmt.Errorf(
			"erro ao salvar %s: %w",
			destinationPath,
			err,
		)
	}

	return nil
}
