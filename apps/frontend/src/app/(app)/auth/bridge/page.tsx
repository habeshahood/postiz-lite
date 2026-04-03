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
      // Set auth everywhere — window global, cookie, localStorage, AND URL hash (survives full page load)
      (window as any).__POSTIZ_AUTH = token
      document.cookie = `auth=${token}; path=/; max-age=${60 * 60 * 24}; SameSite=None; Secure`
      try { localStorage.setItem('auth', token) } catch {}
      // Use window.location for a hard navigation with token in hash (survives Brave iframe storage blocking)
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
