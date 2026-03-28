'use client'

import { useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'

export default function AuthBridgePage() {
  const searchParams = useSearchParams()
  const router = useRouter()

  useEffect(() => {
    const token = searchParams.get('token')
    const redirect = searchParams.get('redirect') || '/launches'

    if (token) {
      // Set the auth cookie that Postiz frontend reads
      document.cookie = `auth=${token}; path=/; max-age=${60 * 60 * 24}; SameSite=None; Secure`
      router.replace(redirect)
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
