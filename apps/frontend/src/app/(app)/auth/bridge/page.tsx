'use client'

import { useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'

export default function AuthBridgePage() {
  const searchParams = useSearchParams()
  const router = useRouter()

  useEffect(() => {
    const token = searchParams.get('token')
    const redirect = searchParams.get('redirect') || '/launches'
    const embedded = searchParams.get('embedded')

    if (token) {
      // Window global survives client-side navigation even when cookies/localStorage are blocked (e.g. Brave iframe)
      (window as any).__POSTIZ_AUTH = token
      document.cookie = `auth=${token}; path=/; max-age=${60 * 60 * 24}; SameSite=None; Secure`
      try { localStorage.setItem('auth', token) } catch {}
      // Carry embedded flag through navigation
      const suffix = embedded ? (redirect.includes('?') ? '&embedded=true' : '?embedded=true') : ''
      router.replace(redirect + suffix)
    } else {
      router.replace('/auth/login')
    }
  }, [searchParams, router])

  return (
    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}>
      <p>Connecting to scheduler...</p>
    </div>
  )
}
