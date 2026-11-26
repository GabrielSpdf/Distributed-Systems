export type User = {
  id: string
  name: string
  email: string
  created_at: string
}

export type RegisterInput = {
  name: string
  email: string
  password: string
}

export type LoginInput = {
  email: string
  password: string
}

type ErrorResponse = {
  error?: string
}

async function request<T>(
  path: string,
  method: string,
  body?: unknown,
): Promise<T> {
  const response = await fetch(path, {
    method,
    credentials: 'include',
    headers: body
      ? {
          'Content-Type': 'application/json',
        }
      : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })

  if (!response.ok) {
    let message = 'Não foi possível concluir a operação'

    try {
      const errorBody = (await response.json()) as ErrorResponse

      if (errorBody.error) {
        message = errorBody.error
      }
    } catch {
      // A resposta não continha um JSON de erro.
    }

    throw new Error(message)
  }

  if (response.status === 204) {
    return undefined as T
  }

  return response.json() as Promise<T>
}

type GraphQLResponse<T> = {
  data?: T | null
  errors?: { message: string }[]
}

async function requestGraphQL<T>(
  query: string,
  variables: Record<string, unknown> = {},
): Promise<T> {
  const response = await fetch('/graphql', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, variables }),
  })

  const result = (await response.json().catch(() => null)) as GraphQLResponse<T> | null
  const error = result?.errors?.[0]

  if (error) {
    throw new Error(error.message)
  }

  if (!response.ok || result?.data == null) {
    throw new Error('Não foi possível concluir a operação')
  }

  return result.data
}

export function registerUser(input: RegisterInput) {
  return request<User>(
    '/api/auth/register',
    'POST',
    input,
  )
}

export function login(input: LoginInput) {
  return request<User>(
    '/api/auth/login',
    'POST',
    input,
  )
}

export function getCurrentUser() {
  return request<User>('/api/auth/me', 'GET')
}

export function logout() {
  return request<void>('/api/auth/logout', 'POST')
}

export type Product = {
  id: string
  name: string
  category: string
  price: number
  quantity: number
}

export type ProductsResponse = {
  products: Product[]
  total: number
}

export type ProductFilters = {
  category?: string
  available?: boolean
}

export async function getProducts(
  filters: ProductFilters = {},
): Promise<ProductsResponse> {
  const data = await requestGraphQL<{
    produtosDisponiveis: ProductsResponse
  }>(
    `
      query Catalog($category: String, $available: Boolean) {
        produtosDisponiveis(category: $category, available: $available) {
          products { id name category price quantity }
          total
        }
      }
    `,
    {
      category: filters.category ?? null,
      available: filters.available ?? null,
    },
  )

  return data.produtosDisponiveis
}

export type CreateOrderInput = {
  items: {
    product_id: string
    quantity: number
  }[]
}

export type OrderItem = {
  product_id: string
  product_name: string
  quantity: number
  unit_price: number
}

export type Order = {
  id: string
  status: string
  items: OrderItem[]
  total: number
  currency: string
  checkout_url: string | null
  checkout_expires_at: string | null
  invoice_id: string | null
  tracking_code: string | null
  shipped_at: string | null
  created_at: string
  updated_at: string
  canCancel: boolean
}

export type OrdersResponse = {
  orders: Order[]
  total: number
}

export async function createOrder(
  input: CreateOrderInput,
): Promise<Pick<Order, 'id' | 'status'>> {
  const data = await requestGraphQL<{
    criarPedido: Pick<Order, 'id' | 'status'>
  }>(
    `
      mutation CreateOrder($items: [OrderItemInput!]!) {
        criarPedido(items: $items) {
          id
          status
        }
      }
    `,
    {
      items: input.items.map((item) => ({
        productId: item.product_id,
        quantity: item.quantity,
      })),
    },
  )

  return data.criarPedido
}

const orderFields = `
  id status total currency canCancel
  checkout_url: checkoutUrl
  checkout_expires_at: checkoutExpiresAt
  invoice_id: invoiceId
  tracking_code: trackingCode
  shipped_at: shippedAt
  created_at: createdAt
  updated_at: updatedAt
  items {
    product_id: productId
    product_name: productName
    quantity
    unit_price: unitPrice
  }
`

export async function getOrders(): Promise<OrdersResponse> {
  const data = await requestGraphQL<{ pedidos: OrdersResponse }>(`
    query ListOrders {
      pedidos { orders { ${orderFields} } total }
    }
  `)
  return data.pedidos
}

export async function getOrderDetails(id: string): Promise<Order> {
  const data = await requestGraphQL<{ pedido: Order }>(
    `
      query OrderDetails($id: String!) {
        pedido(id: $id) { ${orderFields} }
      }
    `,
    { id },
  )
  return data.pedido
}

export async function cancelOrder(id: string): Promise<void> {
  await requestGraphQL<{ cancelarPedido: boolean }>(
    `
      mutation CancelOrder($id: String!) {
        cancelarPedido(id: $id)
      }
    `,
    { id },
  )
}

export type PromotionCategory =
  | 'alimentos'
  | 'limpeza'
  | 'eletronicos'

export type PromotionInterestsResponse = {
  categories: PromotionCategory[]
  email: string
}

export async function getPromotionInterests(): Promise<PromotionInterestsResponse> {
  const data = await requestGraphQL<{
    interessesPromocao: PromotionInterestsResponse
  }>(`
    query PromotionInterests {
      interessesPromocao { categories email }
    }
  `)

  return data.interessesPromocao
}

export async function subscribePromotionInterests(
  categories: PromotionCategory[],
  email?: string,
): Promise<{ changed: boolean }> {
  const data = await requestGraphQL<{
    registrarInteressePromocao: { changed: boolean }
  }>(
    `
      mutation SubscribePromotion($categories: [String!]!, $email: String) {
        registrarInteressePromocao(categories: $categories, email: $email) {
          changed
        }
      }
    `,
    { categories, email },
  )

  return data.registrarInteressePromocao
}

export async function unsubscribePromotionInterest(
  category: PromotionCategory,
): Promise<void> {
  await requestGraphQL<{ cancelarInteressePromocao: { changed: boolean } }>(
    `
      mutation UnsubscribePromotion($category: String!) {
        cancelarInteressePromocao(category: $category) { changed }
      }
    `,
    { category },
  )
}