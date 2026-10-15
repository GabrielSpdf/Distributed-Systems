package promotions

import "testing"

func TestValidateCategories(t *testing.T) {
	tests := []struct {
		name       string
		categories []string
		wantError  bool
	}{
		{"uma categoria", []string{"alimentos"}, false},
		{"duas categorias", []string{"limpeza", "eletronicos"}, false},
		{"todas as categorias", []string{
			"alimentos", "limpeza", "eletronicos",
		}, false},
		{"lista ausente", nil, true},
		{"lista vazia", []string{}, true},
		{"categoria desconhecida", []string{"roupas"}, true},
		{"categoria vazia", []string{""}, true},
		{"espaços extras", []string{" alimentos "}, true},
		{"letras maiúsculas", []string{"ALIMENTOS"}, true},
		{"categoria repetida", []string{"limpeza", "limpeza"}, true},
		{"mais de três categorias", []string{
			"alimentos", "limpeza", "eletronicos", "roupas",
		}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCategories(test.categories)

			if (err != nil) != test.wantError {
				t.Fatalf(
					"erro=%v; esperava erro=%v",
					err,
					test.wantError,
				)
			}
		})
	}
}
