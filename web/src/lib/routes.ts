function normalizeWebPath(value: string | undefined, fallback: string) {
  const path = (value || fallback).trim()
  return `/${path.replace(/^\/+|\/+$/g, '')}`
}

export const OPERATOR_PATH = normalizeWebPath(import.meta.env.VITE_OPERATOR_PATH, '/ops-7f3c')
export const OPERATOR_API_PATH = (import.meta.env.VITE_OPERATOR_API_PATH || 'cp-7f3c').trim().replace(/^\/+|\/+$/g, '')
