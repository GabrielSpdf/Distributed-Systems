import type { CartItem } from './cartTypes'

type CartProps = {
    items: CartItem[]
    onRemove: (productID: string) => void
    onQuantityChange: (productID: string, change: number) => void
    onCreateOrder: () => void
    creatingOrder: boolean
    error: string
    message: string
    messageVisible: boolean
}

const currency = new Intl.NumberFormat('pt-BR', {
  style: 'currency',
  currency: 'BRL',
})

function Cart({
  items,
  onRemove,
  onQuantityChange,
  onCreateOrder,
  creatingOrder,
  error,
  message,
  messageVisible,
}: CartProps) {
  const totalCents = items.reduce(
    (total, item) =>
      total + Math.round(item.product.price * 100) * item.quantity,
    0,
  )

  return (
    <section id="cart" className="cart" aria-labelledby="cart-title">
      <div className="section-heading">
        <div>
          <span className="eyebrow">Sua seleção</span>
          <h2 id="cart-title">Carrinho</h2>
        </div>
      </div>

      {items.length === 0 ? (
        <div className="catalog-state">
          <p>Seu carrinho está vazio. Explore o catálogo para começar.</p>
        </div>
      ) : (
        <>
          <ul className="cart-list">
            {items.map((item) => (
              <li className="cart-item" key={item.product.id}>
                <div>
                  <h3>{item.product.name}</h3>
                  <p>
                    {item.quantity} unidade(s) ×{' '}
                    {currency.format(item.product.price)}
                  </p>
                  <div className="quantity-control">
                        <button
                            type="button"
                            aria-label={`Diminuir quantidade de ${item.product.name}`}
                            disabled={creatingOrder || item.quantity <= 1}
                            onClick={() => onQuantityChange(item.product.id, -1)}
                        >
                            −
                        </button>

                        <span>{item.quantity}</span>

                        <button
                            type="button"
                            aria-label={`Aumentar quantidade de ${item.product.name}`}
                            disabled={creatingOrder || item.quantity >= item.product.quantity}
                            onClick={() => onQuantityChange(item.product.id, 1)}
                        >
                            +
                        </button>
                    </div>
                </div>

                <strong>
                  {currency.format(
                    Math.round(item.product.price * 100) *
                      item.quantity / 100,
                  )}
                </strong>

                <button
                  className="secondary compact"
                  type="button"
                  aria-label={`Remover ${item.product.name} do carrinho`}
                  disabled={creatingOrder}
                  onClick={() => onRemove(item.product.id)}
                >
                  Remover
                </button>
              </li>
            ))}
          </ul>

          <div className="cart-summary">
            <span>Total estimado</span>
            <strong>{currency.format(totalCents / 100)}</strong>
          </div>

          <button
            className="submit-button"
            type="button"
            onClick={onCreateOrder}
            disabled={creatingOrder}
            >
                {creatingOrder ? 'Registrando pedido...' : 'Criar pedido'}
          </button>
        </>
      )}

      {error && (
        <p className="form-error cart-feedback" role="alert">
            {error}
        </p>
        )}

        {message && (
        <p
            className={`cart-feedback cart-confirmation ${
            messageVisible ? 'is-visible' : ''
            }`}
            role="status"
        >
            {message}
        </p>
      )}
    </section>
  )
}

export default Cart