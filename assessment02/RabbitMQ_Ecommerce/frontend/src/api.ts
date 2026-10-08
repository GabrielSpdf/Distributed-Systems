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

export function getProducts(
  filters: ProductFilters = {},
): Promise<ProductsResponse> {
  const query = new URLSearchParams()

  if (filters.category) {
    query.set('category', filters.category)
  }

  if (filters.available !== undefined) {
    query.set('available', String(filters.available))
  }

  const queryString = query.toString()
  const path = queryString
    ? `/api/products?${queryString}`
    : '/api/products'

  return request<ProductsResponse>(path, 'GET')
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

export type OrderLink = {
  href: string
  method: string
}

export type Order = {
  id: string
  status: string
  items: OrderItem[]
  total: number
  currency: string
  checkout_url?: string
  created_at: string
  updated_at: string
  _links: Record<string, OrderLink>
}

export type OrdersResponse = {
  orders: Order[]
  total: number
}

export function createOrder(
  input: CreateOrderInput,
): Promise<Order> {
  return request<Order>(
    '/api/orders',
    'POST',
    input,
  )
}

export function getOrders(): Promise<OrdersResponse> {
  return request<OrdersResponse>(
    '/api/orders',
    'GET',
  )
}

export function getOrderDetails(
  link: OrderLink,
): Promise<Order> {
  return request<Order>(link.href, link.method)
}

export function cancelOrder(link: OrderLink): Promise<void> {
  return request<void>(link.href, 'DELETE')
}