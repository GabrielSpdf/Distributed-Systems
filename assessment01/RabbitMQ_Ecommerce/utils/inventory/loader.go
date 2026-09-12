package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"RabbitMQ_Ecommerce/utils/events"
)

type Data struct {
	Products []events.Product  `json:"products"`
	Stock    map[string]int    `json:"stock"`
}

func Load(fileName string) (Data, error) {
	filePath := filepath.Join("data", fileName)

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return Data{}, fmt.Errorf(
			"erro ao ler arquivo %s: %w",
			filePath,
			err,
		)
	}

	var inventoryData Data

	if err := json.Unmarshal(fileData, &inventoryData); err != nil {
		return Data{}, fmt.Errorf(
			"erro ao desserializar arquivo %s: %w",
			filePath,
			err,
		)
	}

	return inventoryData, nil
}

func SaveStock(
	fileName string,
	stock map[string]int,
) error {
	filePath := filepath.Join("data", fileName)

	inventoryData, err := Load(fileName)
	if err != nil {
		return fmt.Errorf(
			"erro ao carregar estoque para atualização: %w",
			err,
		)
	}

	inventoryData.Stock = stock

	jsonData, err := json.MarshalIndent(
		inventoryData,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao serializar produtos e estoque: %w",
			err,
		)
	}

	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf(
			"erro ao atualizar arquivo %s: %w",
			filePath,
			err,
		)
	}

	return nil
}