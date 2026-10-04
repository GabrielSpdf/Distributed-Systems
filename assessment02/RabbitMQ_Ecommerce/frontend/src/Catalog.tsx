import { useEffect, useState } from 'react'
import { getProducts, type Product } from './api'
import type { CartItem } from './cartTypes'

const currency = new Intl.NumberFormat('pt-BR', {
  style: 'currency',
  currency: 'BRL',
})

type CatalogProps = {
  onAddToCart: (product: Product) => void
  cartItems: CartItem[]
}

function Catalog({ onAddToCart, cartItems }: CatalogProps) {
  const [products, setProducts] = useState<Product[]>([])
  const [category, setCategory] = useState('')
  const [onlyAvailable, setOnlyAvailable] = useState(false)
  const [search, setSearch] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [reload, setReload] = useState(0)

  useEffect(() => {
    let active = true

    setLoading(true)
    setError('')

    getProducts({
      category: category || undefined,
      available: onlyAvailable ? true : undefined,
    })
      .then((result) => {
        if (active) {
          setProducts(result.products)
        }
      })
      .catch((requestError: unknown) => {
        if (active) {
          setError(
            requestError instanceof Error
              ? requestError.message
              : 'Não foi possível carregar o catálogo',
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
  }, [category, onlyAvailable, reload])

  const filteredProducts = products.filter((product) =>
    product.name.toLocaleLowerCase('pt-BR').includes(
      search.trim().toLocaleLowerCase('pt-BR'),
    ),
  )

  return (
    <section id="catalog" className="catalog" aria-labelledby="catalog-title">
      <div className="section-heading">
        <div>
          <span className="eyebrow">Escolha seus produtos</span>
          <h2 id="catalog-title">Catálogo</h2>
        </div>
      </div>

      <div className="catalog-filters">
        <label>
          Buscar produto
          <input
            type="search"
            placeholder="Digite o nome"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>

        <label>
          Categoria
          <select
            value={category}
            onChange={(event) => setCategory(event.target.value)}
          >
            <option value="">Todas as categorias</option>
            <option value="alimentos">Alimentos</option>
            <option value="limpeza">Limpeza</option>
            <option value="eletronicos">Eletrônicos</option>
          </select>
        </label>

        <label className="availability-filter">
          <input
            type="checkbox"
            checked={onlyAvailable}
            onChange={(event) => setOnlyAvailable(event.target.checked)}
          />
          Somente disponíveis
        </label>
      </div>

      {loading ? (
        <div className="catalog-state" role="status">
          <div className="loader" />
          <p>Carregando produtos...</p>
        </div>
      ) : error ? (
        <div className="catalog-state">
          <p className="form-error" role="alert">{error}</p>
          <button
            type="button"
            onClick={() => setReload((value) => value + 1)}
          >
            Tentar novamente
          </button>
        </div>
      ) : filteredProducts.length === 0 ? (
        <div className="catalog-state">
          <p>Nenhum produto encontrado para esses filtros.</p>
        </div>
      ) : (
        <>
          <p className="catalog-count">
            {filteredProducts.length} produto(s) encontrado(s)
          </p>

          <div className="product-grid">
            {filteredProducts.map((product) => {
              const selectedQuantity = cartItems.find(
                (item) => item.product.id === product.id,
              )?.quantity ?? 0

              const remainingQuantity = Math.max(
                0,
                product.quantity - selectedQuantity,
              )

              return (
                <article className="product-card" key={product.id}>
                  <span className="eyebrow">{product.category}</span>
                  <h3>{product.name}</h3>

                  <strong className="product-price">
                    {currency.format(product.price)}
                  </strong>

                  <p className="product-stock">
                    {product.quantity > 0
                      ? `${product.quantity} unidade(s) em estoque`
                      : 'Indisponível'}
                  </p>

                  {selectedQuantity > 0 && (
                    <p className="product-selection">
                      {selectedQuantity} no carrinho ·{' '}
                      {remainingQuantity} para adicionar
                    </p>
                  )}

                  <button
                    type="button"
                    disabled={remainingQuantity === 0}
                    onClick={() => onAddToCart(product)}
                  >
                    {product.quantity <= 0
                      ? 'Indisponível'
                      : remainingQuantity === 0
                        ? 'Limite atingido'
                        : 'Adicionar ao carrinho'}
                  </button>
                </article>
              )
            })}
          </div>
        </>
      )}
    </section>
  )
}

export default Catalog