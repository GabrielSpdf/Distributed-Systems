package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileSafely writes data to a temporary file before replacing the target.
func WriteFileSafely(
	filePath string,
	data []byte,
	permission os.FileMode,
) error {
	directory := filepath.Dir(filePath)

	if err := os.MkdirAll(directory, 0755); err != nil {
		return fmt.Errorf(
			"erro ao criar diretório %s: %w",
			directory,
			err,
		)
	}

	temporaryFile, err := os.CreateTemp(
		directory,
		"."+filepath.Base(filePath)+"-*.tmp",
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao criar arquivo temporário para %s: %w",
			filePath,
			err,
		)
	}

	temporaryPath := temporaryFile.Name()
	replaced := false

	defer func() {
		if !replaced {
			_ = temporaryFile.Close()
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporaryFile.Chmod(permission); err != nil {
		return fmt.Errorf(
			"erro ao definir permissões do arquivo temporário %s: %w",
			temporaryPath,
			err,
		)
	}

	if _, err := temporaryFile.Write(data); err != nil {
		return fmt.Errorf(
			"erro ao gravar arquivo temporário %s: %w",
			temporaryPath,
			err,
		)
	}

	if err := temporaryFile.Sync(); err != nil {
		return fmt.Errorf(
			"erro ao sincronizar arquivo temporário %s: %w",
			temporaryPath,
			err,
		)
	}

	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf(
			"erro ao fechar arquivo temporário %s: %w",
			temporaryPath,
			err,
		)
	}

	if err := os.Rename(
		temporaryPath,
		filePath,
	); err != nil {
		return fmt.Errorf(
			"erro ao substituir arquivo %s: %w",
			filePath,
			err,
		)
	}

	replaced = true

	return nil
}
