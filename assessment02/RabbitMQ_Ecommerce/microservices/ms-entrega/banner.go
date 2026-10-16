package msentrega

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const bannerLine = "=================================================================="

// printBanner destaca um evento no terminal, no mesmo formato dos demais
// serviços. Apenas exibição: não altera o processamento.
func printBanner(format string, args ...any) {
	title := fmt.Sprintf(format, args...)
	padding := max((utf8.RuneCountInString(bannerLine)-utf8.RuneCountInString(title))/2, 0)

	fmt.Println(bannerLine)
	fmt.Println(strings.Repeat(" ", padding) + title)
	fmt.Println(bannerLine)
}
