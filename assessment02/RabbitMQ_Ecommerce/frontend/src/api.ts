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