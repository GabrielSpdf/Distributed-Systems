package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"RabbitMQ_Ecommerce/utils/storage"
)

func TestWriteFileSafelyReplacesExistingContent(
	t *testing.T,
) {
	temporaryDirectory := t.TempDir()

	filePath := filepath.Join(
		temporaryDirectory,
		"data.json",
	)

	if err := os.WriteFile(
		filePath,
		[]byte(`{"value":"old"}`),
		0644,
	); err != nil {
		t.Fatalf(
			"erro ao preparar arquivo de teste: %v",
			err,
		)
	}

	expectedContent := []byte(
		"{\n  \"value\": \"new\"\n}\n",
	)

	if err := storage.WriteFileSafely(
		filePath,
		expectedContent,
		0644,
	); err != nil {
		t.Fatalf(
			"erro ao substituir arquivo: %v",
			err,
		)
	}

	receivedContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf(
			"erro ao ler arquivo substituído: %v",
			err,
		)
	}

	if string(receivedContent) != string(expectedContent) {
		t.Fatalf(
			"conteúdo recebido %q; esperado %q",
			string(receivedContent),
			string(expectedContent),
		)
	}

	temporaryFiles, err := filepath.Glob(
		filepath.Join(
			temporaryDirectory,
			".data.json-*.tmp",
		),
	)
	if err != nil {
		t.Fatalf(
			"erro ao procurar arquivos temporários: %v",
			err,
		)
	}

	if len(temporaryFiles) != 0 {
		t.Fatalf(
			"arquivos temporários não foram removidos: %v",
			temporaryFiles,
		)
	}
}
