import { useEffect, useRef, useState } from 'react'
import {
  getPromotionInterests,
  subscribePromotionInterests,
  unsubscribePromotionInterest,
  type PromotionCategory,
} from './api'

const options: { id: PromotionCategory; label: string }[] = [
  { id: 'alimentos', label: 'Alimentos' },
  { id: 'limpeza', label: 'Limpeza' },
  { id: 'eletronicos', label: 'Eletrônicos' },
]

function PromotionPreferences() {
  const [categories, setCategories] = useState<PromotionCategory[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState<PromotionCategory | null>(null)
  const [loadError, setLoadError] = useState('')
  const [actionError, setActionError] = useState('')
  const [feedback, setFeedback] = useState('')
  const [reload, setReload] = useState(0)
  const mutationInFlight = useRef(false)

  useEffect(() => {
    let active = true

    setLoading(true)
    setLoadError('')

    getPromotionInterests()
      .then((result) => {
        if (active) setCategories(result.categories)
      })
      .catch((error: unknown) => {
        if (active) {
          setLoadError(
            error instanceof Error
              ? error.message
              : 'Não foi possível consultar as preferências',
          )
        }
      })
      .finally(() => {
        if (active) setLoading(false)
      })

    return () => {
      active = false
    }
  }, [reload])

  async function toggleCategory(category: PromotionCategory) {
    if (loading || loadError || mutationInFlight.current) return

    mutationInFlight.current = true
    setSaving(category)
    setActionError('')
    setFeedback('')

    const subscribed = categories.includes(category)

    try {
      if (subscribed) {
        await unsubscribePromotionInterest(category)
        setCategories((current) =>
          current.filter((item) => item !== category),
        )
        setFeedback('Interesse cancelado nessa categoria.')
      } else {
        await subscribePromotionInterests([category])
        setCategories((current) =>
          current.includes(category) ? current : [...current, category],
        )
        setFeedback('Preferência salva para essa categoria.')
      }
    } catch (error: unknown) {
      setActionError(
        error instanceof Error
          ? error.message
          : 'Não foi possível alterar a preferência',
      )
    } finally {
      mutationInFlight.current = false
      setSaving(null)
    }
  }

  return (
    <section
      id="promotion-preferences"
      className="promotion-preferences"
      aria-labelledby="promotions-title"
    >
      <div className="section-heading">
        <div>
          <span className="eyebrow">Suas escolhas</span>
          <h2 id="promotions-title">Preferências de promoções</h2>
        </div>
      </div>

      <p className="promotion-preferences-description">
        Escolha as categorias de promoções que deseja receber por e-mail.
      </p>

      {loading ? (
        <p role="status">Consultando suas preferências...</p>
      ) : loadError ? (
        <div>
          <p role="alert">{loadError}</p>
          <button type="button" onClick={() => setReload((value) => value + 1)}>
            Tentar novamente
          </button>
        </div>
      ) : (
        <div className="promotion-preferences-grid">
          {options.map((option) => {
            const subscribed = categories.includes(option.id)

            return (
              <article className="promotion-preference-card" key={option.id}>
                <h3>{option.label}</h3>
                <p>{subscribed ? 'Interesse ativo' : 'Sem inscrição'}</p>
                <button
                  type="button"
                  className={subscribed ? 'secondary' : undefined}
                  disabled={saving !== null}
                  onClick={() => void toggleCategory(option.id)}
                  aria-label={`${
                    subscribed ? 'Cancelar interesse em' : 'Receber promoções de'
                  } ${option.label}`}
                >
                  {saving === option.id
                    ? 'Salvando...'
                    : subscribed
                      ? 'Cancelar interesse'
                      : 'Receber promoções'}
                </button>
              </article>
            )
          })}
        </div>
      )}

      {feedback && <p role="status">{feedback}</p>}
      {actionError && <p role="alert">{actionError}</p>}

      <p className="promotion-preferences-note">
        Salvar a preferência não confirma o envio de um e-mail.
        A integração de envio ainda está em desenvolvimento.
      </p>
    </section>
  )
}

export default PromotionPreferences
