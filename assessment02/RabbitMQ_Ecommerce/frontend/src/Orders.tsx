import { useEffect, useRef, useState } from 'react'
import { getOrders, type Order, getOrderDetails, cancelOrder } from './api'

type OrdersProps = {
  refreshKey: number
}

const currency = new Intl.NumberFormat('pt-BR', {
  style: 'currency',
  currency: 'BRL',
})

const dateFormat = new Intl.DateTimeFormat('pt-BR', {
  dateStyle: 'short',
  timeStyle: 'short',
})

const statusLabels: Record<string, string> = {
  PENDENTE: 'Aguardando processamento',
  CANCELADO: 'Cancelado',
  ESTOQUE_INDISPONIVEL: 'Estoque indisponível',
  ESTOQUE_CONFIRMADO: 'Estoque confirmado',
  AGUARDANDO_PAGAMENTO: 'Aguardando pagamento',
  PAGAMENTO_RECUSADO: 'Pagamento recusado',
  PAGAMENTO_APROVADO: 'Pagamento aprovado',
  ENVIADO: 'Enviado',
  FALHA_NO_PROCESSAMENTO: 'Falha no processamento',
}

function Orders({ refreshKey }: OrdersProps) {
  const [orders, setOrders] = useState<Order[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)
  const [selectedOrder, setSelectedOrder] = useState<Order | null>(null)
  const [loadingDetails, setLoadingDetails] = useState(false)
  const [detailsError, setDetailsError] = useState('')
  const [detailsOrderID, setDetailsOrderID] = useState<string | null>(null)
  const [cancelingOrderID, setCancelingOrderID] = useState<string | null>(null)
  const [confirmationOrder, setConfirmationOrder] = useState<Order | null>(null)
  const confirmationDialog = useRef<HTMLDialogElement>(null)
  const cancellationInFlight = useRef(false)
  const [cancelFeedback, setCancelFeedback] = useState<{
    orderID: string
    message: string
    isError: boolean
  } | null>(null)

  useEffect(() => {
    const dialog = confirmationDialog.current
    if (!dialog) return

    if (confirmationOrder && !dialog.open) {
      dialog.showModal()
    } else if (!confirmationOrder && dialog.open) {
      dialog.close()
    }
  }, [confirmationOrder])

  useEffect(() => {
    let active = true

    setLoading(true)
    setError('')

    getOrders()
      .then((result) => {
        if (active) {
          setOrders(result.orders)
        }
      })
      .catch((requestError: unknown) => {
        if (active) {
          setError(
            requestError instanceof Error
              ? requestError.message
              : 'Não foi possível consultar os pedidos',
          )
        }
      })
      .finally(() => {
        if (active) {
          setLoading(false)
        }
      })

    return () => {
      active = false
    }
  }, [refreshKey, reload])

  async function showDetails(order: Order) {
    const link = order._links.self

    if (!link || loadingDetails) {
      return
    }

    setDetailsOrderID(order.id)
    setLoadingDetails(true)
    setDetailsError('')
    setSelectedOrder(null)

    try {
      const details = await getOrderDetails(link)
      setSelectedOrder(details)
    } catch (requestError) {
      setDetailsError(
        requestError instanceof Error
          ? requestError.message
          : 'Não foi possível consultar os detalhes',
      )
    } finally {
      setLoadingDetails(false)
    }
  }

  async function handleCancelOrder(order: Order) {
    const link = order._links.cancel

    if (!link || link.method !== 'DELETE' || cancellationInFlight.current || loadingDetails) {
      return
    }

    cancellationInFlight.current = true
    setCancelingOrderID(order.id)
    setCancelFeedback(null)

    try {
      await cancelOrder(link)

      setSelectedOrder((current) =>
        current?.id === order.id ? null : current,
      )

      setCancelFeedback({
        orderID: order.id,
        message: 'Pedido cancelado. A liberação do estoque será processada.',
        isError: false,
      })
    } catch (requestError) {
      setCancelFeedback({
        orderID: order.id,
        message:
          requestError instanceof Error
            ? requestError.message
            : 'Não foi possível cancelar o pedido',
        isError: true,
      })
    } finally {
      cancellationInFlight.current = false
      setCancelingOrderID(null)
      setReload((value) => value + 1)
    }
  }

  return (
    <section
      id="orders"
      className="orders"
      aria-labelledby="orders-title"
    >
      <div className="section-heading">
        <div>
          <span className="eyebrow">Acompanhe suas compras</span>
          <h2 id="orders-title">Meus pedidos</h2>
        </div>

        <button
          className="secondary compact"
          type="button"
          disabled={loading || cancelingOrderID !== null}
          onClick={() => setReload((value) => value + 1)}
        >
          Atualizar
        </button>
      </div>

      {loading ? (
        <div className="catalog-state" role="status">
          <div className="loader" />
          <p>Carregando pedidos...</p>
        </div>
      ) : error ? (
        <div className="catalog-state">
          <p className="form-error" role="alert">{error}</p>
        </div>
      ) : orders.length === 0 ? (
        <div className="catalog-state">
          <p>Você ainda não possui pedidos.</p>
        </div>
      ) : (
        <div className="orders-list">
          {orders.map((order) => (
            <article className="order-card" key={order.id}>
              <div className="order-heading">
                <div>
                  <h3>{order.id}</h3>
                  <p>
                    {dateFormat.format(new Date(order.created_at))}
                  </p>
                </div>

                <span className="order-status">
                  {statusLabels[order.status] ?? order.status}
                </span>
              </div>

              <ul className="order-items">
                {order.items.map((item) => (
                  <li key={item.product_id}>
                    <span>
                      {item.quantity} × {item.product_name}
                    </span>
                    <span>
                      {currency.format(item.unit_price)} por unidade
                    </span>
                  </li>
                ))}
              </ul>

              <div className="order-total">
                <span>Total</span>
                <strong>{currency.format(order.total)}</strong>
              </div>
              <div className="order-actions">
                {order._links.self && selectedOrder?.id !== order.id && (
                  <button
                    className="secondary compact"
                    type="button"
                    disabled={loadingDetails || cancelingOrderID !== null}
                    onClick={() => showDetails(order)}
                  >
                    Ver detalhes
                  </button>
                )}
                {order._links.cancel?.method === 'DELETE' && (
                  <button
                    className="secondary compact"
                    type="button"
                    disabled={cancelingOrderID !== null || loadingDetails}
                    onClick={() => setConfirmationOrder(order)}
                  >
                    {cancelingOrderID === order.id ? 'Cancelando...' : 'Cancelar pedido'}
                  </button>
                )}
              </div>

              {detailsOrderID === order.id && loadingDetails && (
                <p className="cart-feedback" role="status">
                  Consultando detalhes...
                </p>
              )}

              {cancelFeedback?.orderID === order.id && (
                <p
                  className={
                    cancelFeedback.isError
                      ? 'form-error cart-feedback'
                      : 'cart-feedback'
                  }
                  role={cancelFeedback.isError ? 'alert' : 'status'}
                >
                  {cancelFeedback.message}
                </p>
              )}

              {detailsOrderID === order.id && detailsError && (
                <p className="form-error cart-feedback" role="alert">
                  {detailsError}
                </p>
              )}
              {selectedOrder?.id === order.id && (
                <div className="order-detail-panel">
                  <div className="order-heading">
                    <h3>Detalhes de {selectedOrder.id}</h3>
                    <button
                      className="secondary compact"
                      type="button"
                      onClick={() => setSelectedOrder(null)}
                    >
                      Fechar
                    </button>
                  </div>

                  <p>
                    Situação:{' '}
                    {statusLabels[selectedOrder.status] ?? selectedOrder.status}
                  </p>

                  <p>
                    Última atualização:{' '}
                    {dateFormat.format(new Date(selectedOrder.updated_at))}
                  </p>

                  <p>
                    Total: {currency.format(selectedOrder.total)}
                  </p>
                </div>
              )}
            </article>
          ))}
        </div>
      )}
      <dialog
        ref={confirmationDialog}
        className="order-cancel-dialog"
        aria-labelledby="cancel-order-title"
        aria-describedby="cancel-order-description"
        onCancel={() => setConfirmationOrder(null)}
      >
        <span className="eyebrow">Cancelar pedido</span>
        <h2 id="cancel-order-title">
          Deseja cancelar o pedido {confirmationOrder?.id}?
        </h2>
        <p id="cancel-order-description">
          O pedido continuará no seu histórico. Se houver estoque reservado,
          sua liberação será solicitada.
        </p>
        <div className="order-cancel-actions">
          <button
            className="secondary"
            type="button"
            autoFocus
            onClick={() => setConfirmationOrder(null)}
          >
            Voltar
          </button>
          <button
            className="danger"
            type="button"
            disabled={!confirmationOrder || cancelingOrderID !== null}
            onClick={() => {
              if (!confirmationOrder) return
              const order = confirmationOrder
              setConfirmationOrder(null)
              void handleCancelOrder(order)
            }}
          >
            Confirmar cancelamento
          </button>
        </div>
      </dialog>
    </section>
  )
}

export default Orders
