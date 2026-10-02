import {
  useEffect,
  useState,
  type FormEvent,
} from 'react'
import {
  getCurrentUser,
  login,
  logout,
  registerUser,
  type User,
} from './api'

const services = [
  { name: 'API Gateway', detail: 'REST, autenticação e SSE' },
  { name: 'Estoque', detail: 'Catálogo e reservas' },
  { name: 'Pagamento', detail: 'Cobranças e webhook' },
  { name: 'Entrega', detail: 'Nota fiscal e despacho' },
  { name: 'Promoções', detail: 'Interesses e e-mails' },
]

type AuthMode = 'login' | 'register'

function App() {
  const [mode, setMode] = useState<AuthMode>('login')
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [user, setUser] = useState<User | null>(null)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [checkingSession, setCheckingSession] = useState(true)

  useEffect(() => {
    getCurrentUser()
      .then(setUser)
      .catch(() => undefined)
      .finally(() => setCheckingSession(false))
  }, [])

  function changeMode(nextMode: AuthMode) {
    setMode(nextMode)
    setError('')
    setPassword('')
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setLoading(true)
    setError('')

    try {
      if (mode === 'register') {
        await registerUser({
          name,
          email,
          password,
        })
      }

      const authenticatedUser = await login({
        email,
        password,
      })

      setUser(authenticatedUser)
      setPassword('')
    } catch (requestError) {
      setError(
        requestError instanceof Error
          ? requestError.message
          : 'Não foi possível concluir a operação',
      )
    } finally {
      setLoading(false)
    }
  }

  async function handleLogout() {
    setLoading(true)

    try {
      await logout()
      setUser(null)
      setMode('login')
      setName('')
      setEmail('')
      setPassword('')
      setError('')
    } finally {
      setLoading(false)
    }
  }

  if (checkingSession) {
    return (
      <main className="center-state">
        <div className="loader" />
        <p>Verificando sua sessão...</p>
      </main>
    )
  }

  if (user) {
    return (
      <main className="dashboard">
        <nav className="topbar">
          <a className="brand" href="/" aria-label="Página inicial">
            <span className="brand-mark">DS</span>
            <span>Event Market</span>
          </a>

          <div className="user-menu">
            <div>
              <strong>{user.name}</strong>
              <span>{user.email}</span>
            </div>

            <button
              className="secondary compact"
              type="button"
              onClick={handleLogout}
              disabled={loading}
            >
              Sair
            </button>
          </div>
        </nav>

        <header className="dashboard-hero">
          <span className="eyebrow">Área do cliente</span>
          <h1>
            Olá, {user.name.split(' ')[0]}.
            <br />
            Boas compras.
          </h1>
          <p>
            Acompanhe produtos, pedidos e pagamentos em uma
            experiência integrada aos microsserviços.
          </p>

          <div className="actions">
            <button type="button">Explorar produtos</button>
            <button className="secondary" type="button">
              Meus pedidos
            </button>
          </div>
        </header>

        <section aria-labelledby="services-title">
          <div className="section-heading">
            <div>
              <span className="eyebrow">Arquitetura</span>
              <h2 id="services-title">Serviços conectados</h2>
            </div>
            <span className="status">
              Sessão autenticada
            </span>
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

  return (
    <main className="auth-page">
      <section className="auth-intro">
        <a className="brand" href="/" aria-label="Página inicial">
          <span className="brand-mark">DS</span>
          <span>Event Market</span>
        </a>

        <div className="intro-copy">
          <span className="eyebrow">Sistemas Distribuídos</span>
          <h1>Uma compra, vários serviços conectados.</h1>
          <p>
            Um e-commerce acadêmico construído com React, Go,
            RabbitMQ, PostgreSQL, REST e SSE.
          </p>
        </div>

        <div className="technology-list">
          <span>REST</span>
          <span>RabbitMQ</span>
          <span>SSE</span>
          <span>PostgreSQL</span>
        </div>
      </section>

      <section className="auth-panel">
        <div className="auth-card">
          <span className="eyebrow">
            {mode === 'login' ? 'Bem-vindo de volta' : 'Nova conta'}
          </span>

          <h2>
            {mode === 'login'
              ? 'Entre para continuar'
              : 'Crie seu acesso'}
          </h2>

          <p className="form-description">
            {mode === 'login'
              ? 'Acompanhe seus pedidos e pagamentos em tempo real.'
              : 'Cadastre-se para começar a explorar o catálogo.'}
          </p>

          <div className="mode-switch" aria-label="Tipo de acesso">
            <button
              className={mode === 'login' ? 'active' : ''}
              type="button"
              onClick={() => changeMode('login')}
            >
              Entrar
            </button>
            <button
              className={mode === 'register' ? 'active' : ''}
              type="button"
              onClick={() => changeMode('register')}
            >
              Criar conta
            </button>
          </div>

          <form onSubmit={handleSubmit}>
            {mode === 'register' && (
              <label>
                Nome
                <input
                  type="text"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Seu nome completo"
                  autoComplete="name"
                  required
                />
              </label>
            )}

            <label>
              E-mail
              <input
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="voce@exemplo.com"
                autoComplete="email"
                required
              />
            </label>

            <label>
              Senha
              <input
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="Mínimo de 8 caracteres"
                autoComplete={
                  mode === 'login'
                    ? 'current-password'
                    : 'new-password'
                }
                minLength={8}
                required
              />
            </label>

            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}

            <button
              className="submit-button"
              type="submit"
              disabled={loading}
            >
              {loading
                ? 'Aguarde...'
                : mode === 'login'
                  ? 'Entrar'
                  : 'Criar minha conta'}
            </button>
          </form>

          <p className="security-note">
            Sua sessão é protegida por cookie seguro e senha
            armazenada com hash.
          </p>
        </div>
      </section>
    </main>
  )
}

export default App