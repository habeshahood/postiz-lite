'use client'

import { Suspense, useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'

function BridgeInner() {
  const searchParams = useSearchParams()
  const router = useRouter()

  useEffect(() => {
    const token = searchParams.get('token')
    const redirect = searchParams.get('redirect') || '/launches'
    const embedded = searchParams.get('embedded')

    if (token) {
      // Set auth everywhere — window global, cookie, localStorage (Brave may block cookie/storage)
      ;(window as any).__POSTIZ_AUTH = token
      document.cookie = `auth=${token}; path=/; max-age=${60 * 60 * 24}; SameSite=None; Secure`
      try { localStorage.setItem('auth', token) } catch {}
      const sep = redirect.includes('?') ? '&' : '?'
      const dest = `/s${redirect}${sep}loggedAuth=${encodeURIComponent(token)}${embedded ? '&embedded=true' : ''}`
      window.location.href = dest
    } else {
      router.replace('/auth/login')
    }
  }, [searchParams, router])

  // Blank while redirecting — no flash of content
  return null
}

export default function AuthBridgePage() {
  return (
    <Suspense fallback={null}>
      <BridgeInner />
    </Suspense>
  )
}
