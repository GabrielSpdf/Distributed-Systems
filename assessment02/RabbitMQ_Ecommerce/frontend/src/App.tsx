const services = [
  { name: 'API Gateway', detail: 'REST, autenticação e SSE' },
  { name: 'Estoque', detail: 'Catálogo e reservas' },
  { name: 'Pagamento', detail: 'Cobranças e webhook' },
  { name: 'Entrega', detail: 'Nota fiscal e despacho' },
  { name: 'Promoções', detail: 'Interesses e e-mails' },
]

function App() {
  return (
    <main>
      <header className="hero">
        <span className="eyebrow">Sistemas Distribuídos</span>
        <h1>E-commerce orientado a eventos</h1>
        <p>
          Aplicação web construída com React, Go, RabbitMQ, PostgreSQL,
          REST e notificações em tempo real por SSE.
        </p>
        <div className="actions">
          <button type="button">Entrar</button>
          <button className="secondary" type="button">
            Criar conta
          </button>
        </div>
      </header>

      <section aria-labelledby="services-title">
        <div className="section-heading">
          <div>
            <span className="eyebrow">Arquitetura</span>
            <h2 id="services-title">Serviços do projeto</h2>
          </div>
          <span className="status">Fundação em desenvolvimento</span>
        </div>

        <div className="service-grid">
          {services.map((service) => (
            <article key={service.name}>
              <div className="service-indicator" />
              <h3>{service.name}</h3>
              <p>{service.detail}</p>
            </article>
          ))}
        </div>
      </section>
    </main>
  )
}

export default App

