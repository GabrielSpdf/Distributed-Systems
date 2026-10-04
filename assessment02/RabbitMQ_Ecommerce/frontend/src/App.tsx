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
  createOrder,
  type User,
  type Product,
} from './api'
import Orders from './Orders'
import Catalog from './Catalog'
import Cart from './Cart'
import type { CartItem } from './cartTypes'

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
  const [cartItems, setCartItems] = useState<CartItem[]>([])
  const [ordersRefreshKey, setOrdersRefreshKey] = useState(0)
  const [creatingOrder, setCreatingOrder] = useState(false)
  const [orderError, setOrderError] = useState('')
  const [orderMessage, setOrderMessage] = useState('')
  const [orderMessageVisible, setOrderMessageVisible] = useState(false)

  useEffect(() => {
    getCurrentUser()
      .then(setUser)
      .catch(() => undefined)
      .finally(() => setCheckingSession(false))
  }, [])

  useEffect(() => {
    if (!orderMessage) {
      setOrderMessageVisible(false)
      return
    }

    setOrderMessageVisible(true)

    const fadeTimer = window.setTimeout(() => {
      setOrderMessageVisible(false)
    }, 4600)

    const clearTimer = window.setTimeout(() => {
      setOrderMessage('')
    }, 5000)

  return () => {
    window.clearTimeout(fadeTimer)
    window.clearTimeout(clearTimer)
  }
}, [orderMessage])

  function changeMode(nextMode: AuthMode) {
    setMode(nextMode)
    setError('')
    setPassword('')
  }

  function addToCart(product: Product) {
    if (creatingOrder) {
      return
    }

    if (product.quantity <= 0) {
      return
    }

    setOrderMessage('')
    setOrderError('')

    setCartItems((currentItems) => {
      const existingItem = currentItems.find(
        (item) => item.product.id === product.id,
      )

      if (!existingItem) {
        return [
          ...currentItems,
          { product, quantity: 1 },
        ]
      }

      if (existingItem.quantity >= product.quantity) {
        return currentItems
      }

      return currentItems.map((item) =>
        item.product.id === product.id
          ? {
              product,
              quantity: item.quantity + 1,
            }
          : item,
      )
    })
  }

  function removeFromCart(productID: string) {
    setCartItems((currentItems) =>
      currentItems.filter(
        (item) => item.product.id !== productID,
      ),
    )
  }

  function changeCartQuantity(productID: string, change: number) {
    setCartItems((currentItems) =>
      currentItems.map((item) => {
        if (item.product.id !== productID) {
          return item
        }

        const nextQuantity = item.quantity + change

        if (
          nextQuantity < 1 ||
          nextQuantity > item.product.quantity
        ) {
          return item
        }

        return {
          ...item,
          quantity: nextQuantity,
        }
      }),
    )
  }

  async function handleCreateOrder() {
    if (cartItems.length === 0 || creatingOrder) {
      return
    }

    setCreatingOrder(true)
    setOrderError('')
    setOrderMessage('')

    try {
      const order = await createOrder({
        items: cartItems.map((item) => ({
          product_id: item.product.id,
          quantity: item.quantity,
        })),
      })

      setCartItems([])
      setOrdersRefreshKey((value) => value + 1)
      setOrderMessage(
        `Pedido ${order.id} registrado. Aguardando processamento.`,
      )
    } catch (requestError) {
      setOrderError(
        requestError instanceof Error
          ? requestError.message
          : 'Não foi possível criar o pedido',
      )
    } finally {
      setCreatingOrder(false)
    }
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
      setCartItems([])
      setMode('login')
      setName('')
      setEmail('')
      setPassword('')
      setError('')
      setOrderMessage('')
      setOrderError('')
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
              disabled={loading || creatingOrder}
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
            <button
              type="button"
              onClick={() =>
                document.getElementById('catalog')?.scrollIntoView({
                  behavior: 'smooth',
                })
              }
            >
              Explorar produtos
            </button>
            <button
              className="secondary"
              type="button"
              onClick={() => {
                setOrdersRefreshKey((value) => value + 1)

                document.getElementById('orders')?.scrollIntoView({
                  behavior: 'smooth',
                })
              }}
            >
              Meus pedidos
            </button>
          </div>
        </header>

        <div className="shopping-layout">
          <Catalog
            onAddToCart={addToCart}
            cartItems={cartItems}
          />

          <aside className="cart-sidebar" aria-label="Resumo do carrinho">
            <Cart
              items={cartItems}
              onRemove={removeFromCart}
              onQuantityChange={changeCartQuantity}
              onCreateOrder={handleCreateOrder}
              creatingOrder={creatingOrder}
              error={orderError}
              message={orderMessage}
              messageVisible={orderMessageVisible}
            />
          </aside>
        </div>

        <Orders refreshKey={ordersRefreshKey} />

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