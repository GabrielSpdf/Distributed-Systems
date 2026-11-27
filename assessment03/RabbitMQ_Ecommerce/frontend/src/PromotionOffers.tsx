import { useEffect, useState } from 'react'
import { createClient } from 'graphql-ws'
import type { PromotionCategory } from './api'

type Offer = {
  eventId: string
  productName: string
  category: PromotionCategory
  originalPrice: number
  promotionalPrice: number
  discountPercentage: number
}

type ConnectionState = 'connecting' | 'connected' | 'reconnecting' | 'disconnected'

const connectionLabels: Record<ConnectionState, string> = {
  connecting: 'Conectando às ofertas…',
  connected: 'Recebendo ofertas das suas categorias de interesse.',
  reconnecting: 'Reconectando às ofertas…',
  disconnected: 'Ofertas desconectadas. Recarregue a página para reconectar.',
}

const categoryLabels: Record<PromotionCategory, string> = {
  alimentos: 'Alimentos',
  limpeza: 'Limpeza',
  eletronicos: 'Eletrônicos',
}

const currency = new Intl.NumberFormat('pt-BR', {
  style: 'currency',
  currency: 'BRL',
})

const discount = new Intl.NumberFormat('pt-BR', {
  style: 'percent',
  maximumFractionDigits: 1,
})

export default function PromotionOffers() {
  const [offers, setOffers] = useState<Offer[]>([])
  const [connection, setConnection] = useState<ConnectionState>('connecting')

  useEffect(() => {
    let active = true

    const url = new URL('/graphql', window.location.href)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'

    const client = createClient({
      url: url.href,
      retryAttempts: 8,
      connectionAckWaitTimeout: 5_000,
      on: {
        connected: () => {
          if (active) setConnection('connected')
        },
        closed: () => {
          if (active) setConnection('reconnecting')
        },
      },
    })

    const unsubscribe = client.subscribe<{ promocaoDisponivel: Offer }>(
      {
        query: `
          subscription PromotionOffers {
            promocaoDisponivel {
              eventId
              productName
              category
              originalPrice
              promotionalPrice
              discountPercentage
            }
          }
        `,
      },
      {
        next: (result) => {
          if (!active) return
          if (result.errors?.length) {
            setConnection('disconnected')
            return
          }

          const offer = result.data?.promocaoDisponivel
          if (!offer) return

          setOffers((current) => {
            if (current.some((item) => item.eventId === offer.eventId)) {
              return current
            }
            return [offer, ...current].slice(0, 10)
          })
        },
        error: () => {
          if (active) setConnection('disconnected')
        },
        complete: () => {
          if (active) setConnection('disconnected')
        },
      },
    )

    return () => {
      active = false
      unsubscribe()
      void client.dispose()
    }
  }, [])

  return (
    <section className="promotion-preferences" aria-labelledby="live-offers-title">
      <h2 id="live-offers-title">Ofertas em tempo real</h2>
      <p role="status">{connectionLabels[connection]}</p>

      <div role="log" aria-label="Ofertas recebidas" aria-relevant="additions">
        {offers.length > 0 ? (
          <ul>
            {offers.map((offer) => (
              <li key={offer.eventId}>
                <strong>{offer.productName}</strong>
                {' · '}{categoryLabels[offer.category] ?? offer.category}
                {' · De '}<del>{currency.format(offer.originalPrice)}</del>
                {' por '}<strong>{currency.format(offer.promotionalPrice)}</strong>
                {' · '}{discount.format(offer.discountPercentage / 100)} de desconto
              </li>
            ))}
          </ul>
        ) : (
          <p>Nenhuma oferta recebida nesta sessão.</p>
        )}
      </div>
    </section>
  )
}