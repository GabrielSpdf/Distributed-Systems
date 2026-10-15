package mockpagamento

import (
	"html/template"
	"log"
	"net/http"
)

type pageView struct {
	Found     bool
	ChargeID  string
	OrderID   string
	Amount    string
	ExpiresAt string
	Status    string
	Expired   bool
	Pending   bool
	Message   string
}

var checkoutTemplate = template.Must(template.New("checkout").Parse(`<!doctype html>
<html lang="pt-BR">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Mock de Pagamento</title>
<style>
  body { font-family: system-ui, sans-serif; background: #f4f5f7; margin: 0; padding: 16px; color: #1f2933; }
  main { max-width: 420px; margin: 48px auto; background: #fff; border-radius: 12px; padding: 24px; box-shadow: 0 2px 12px rgba(0,0,0,.08); }
  h1 { font-size: 1.25rem; margin: 0 0 16px; }
  dl { display: grid; grid-template-columns: auto 1fr; gap: 8px 16px; margin: 0 0 24px; }
  dt { color: #616e7c; }
  dd { margin: 0; font-weight: 600; word-break: break-all; }
  form { display: inline; }
  button { border: 0; border-radius: 8px; padding: 12px 16px; font-size: 1rem; cursor: pointer; color: #fff; margin: 4px 8px 4px 0; }
  .approve { background: #2f855a; }
  .refuse { background: #c53030; }
  .status { padding: 12px; border-radius: 8px; background: #edf2f7; font-weight: 600; }
  .message { margin-top: 16px; color: #9b2c2c; }
</style>
</head>
<body>
<main>
  <h1>Mock de Pagamento</h1>
  {{if .Found}}
  <dl>
    <dt>Pedido</dt><dd>{{.OrderID}}</dd>
    <dt>Valor</dt><dd>{{.Amount}}</dd>
    <dt>Válido até</dt><dd>{{.ExpiresAt}}</dd>
  </dl>
  {{if .Pending}}
  <form method="post" action="/checkout/{{.ChargeID}}/approve"><button class="approve" type="submit">Pagamento Aprovado</button></form>
  <form method="post" action="/checkout/{{.ChargeID}}/refuse"><button class="refuse" type="submit">Pagamento Recusado</button></form>
  {{else if .Expired}}
  <p class="status">Checkout expirado. Volte à loja e faça um novo pedido.</p>
  {{else if eq .Status "APPROVED"}}
  <p class="status">Pagamento aprovado. Você já pode fechar esta aba.</p>
  {{else}}
  <p class="status">Pagamento recusado. Você já pode fechar esta aba.</p>
  {{end}}
  {{end}}
  {{if .Message}}<p class="message">{{.Message}}</p>{{end}}
</main>
</body>
</html>
`))

func renderPage(writer http.ResponseWriter, status int, view pageView) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.WriteHeader(status)

	if err := checkoutTemplate.Execute(writer, view); err != nil {
		log.Printf("[ERRO] Página de checkout: %v", err)
	}
}
